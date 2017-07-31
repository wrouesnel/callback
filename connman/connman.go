// connman implements the callback connection manager.

package connman

import (
	"github.com/hashicorp/yamux"
	"github.com/wrouesnel/callback/util"
	"github.com/wrouesnel/go.log"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type ErrSessionDisconnected struct {
	callbackId string
}

func (err ErrSessionDisconnected) Error() string {
	return "callback session disconnected externally"
}

type ErrSessionExists struct {
	callbackId string
}

func (err ErrSessionExists) Error() string {
	return "callback session already exists"
}

type ErrSessionUnknown struct {
	callbackId string
}

func (err ErrSessionUnknown) Error() string {
	return "callback session does not exist"
}

type ConnectionManager struct {
	// These counters generate sequence numbers for the session event streams,
	// to allow clients to detect missed updates.
	callbackSessionEventCounter uint32
	clientSessionEventCounter   uint32

	// callbackSessions holds the currently active muxes.
	callbackSessions map[string]*callbackSession
	callbackMtx      sync.RWMutex

	clientSessions map[string]*ClientSessionDesc
	clientMtx      sync.RWMutex

	clientSubscribers      map[<-chan ClientSessionDesc]chan<- ClientSessionDesc
	clientSubscribersMutex sync.RWMutex

	callbackSubscribers      map[<-chan CallbackSessionDesc]chan<- CallbackSessionDesc
	callbackSubscribersMutex sync.RWMutex

	proxyBufferSize int
}

// ClientSessionDesc holds connection information for a client session.
type ClientSessionDesc struct {
	// Connection tallies
	BytesOut uint64 `json:"bytes_out"`
	BytesIn  uint64 `json:"bytes_in"`
	// Establishment time
	ConnectedAt time.Time `json:"connected_at"`
	// Connection details
	RemoteAddr string `json:"remote_addr"`
	// Connection Target
	CallbackId string `json:"callback_id"`
}

// copy makes a thread-safe copy ClientSessionDesc.
func (cb *ClientSessionDesc) copy() ClientSessionDesc {
	result := ClientSessionDesc{}
	result.BytesOut = atomic.LoadUint64(&cb.BytesOut)
	result.BytesIn = atomic.LoadUint64(&cb.BytesIn)

	result.ConnectedAt = cb.ConnectedAt
	result.RemoteAddr = cb.RemoteAddr
	result.CallbackId = cb.CallbackId
	return result
}

// CallbackSessionDesc holds connection infromation for a callback reverse proxy
// session
type CallbackSessionDesc struct {
	// Establishment time
	ConnectedAt time.Time `json:"connected_at"`
	// Connection details
	RemoteAddr string `json:"remote_addr"`
	// Number of clients
	NumClients uint32 `json:"num_clients"`
}

// ConnMannEventType maps string types to event descriptors used by the connection manager
type EventType string

const (
	EventDisconnected = EventType("disconnected")
	EventConnected    = EventType("connected")
	EventUpdated	  = EventType("updated")
)

// ConnManEventHeader defines the common header used for connection manager events
type ConnManEventHeader struct {
	EventType   EventType `json:"event_type"`
	SequenceNum uint32    `json:"sequence_num"`
}

// ClientConnectionEvent is emitted when an event pertaining to client connections occurs
type ClientConnectionEvent struct {
	ConnManEventHeader `json:",inline"`
	ClientSessionDesc  `json:",inline"`
}

// CallbackConnectionEvent is emitted when an event pertaining to callabck connections occurs
type CallbackConnectionEvent struct {
	ConnManEventHeader  `json:",inline"`
	CallbackSessionDesc `json:",inline"`
}

// CallbackSessionList defines the serialization format for listing callback sessions.
// It includes the sequence_num so it may be reconciled with the event stream.
type CallbackSessionList struct {
	SequenceNum uint32                         `json:"sequence_num"`
	Sessions    map[string]CallbackSessionDesc `json:"sessions"`
}

// ClientSessionList defines the serialization format for listing client sessions.
// It includes the sequence_num so it may be reconciled with the event stream.
type ClientSessionList struct {
	SequenceNum uint32              `json:"sequence_num"`
	Sessions    []ClientSessionDesc `json:"sessions"`
}

// callbackSession holds the actual internal state of a session
type callbackSession struct {
	// context logger for the session
	log log.Logger
	// muxClient holds the yamux client session. This is the actual callback connection.
	muxClient *yamux.Session
	// resultCh holds the channel which communicates connection failure/termination
	// to the underlying websocket. We send an error when we fail to connect,
	// to signal the underlying request to finish and allow a reset.
	resultCh chan<- error
	// doneCh holds a channel which is closed when the callbackSession is shutting down.
	doneCh chan struct{}
	// Mutex to synchronize channel operations
	mtx sync.Mutex
	// desc holds the public accounting data for the session
	desc CallbackSessionDesc
}

// shutdownWatch monitors the given channel for closure, and triggers the clean up of the callbackSession.
// Should be launched as a go-routine.
func (cbs *callbackSession) startShutdownWatch(shutdown <-chan struct{}) {
	go func() {
		<-shutdown
		cbs.log.Errorln("Callback session underlying connection has ended.")
		// Send a normal disconnect anyway, since *maybe* its still possible to close the mux gracefully.
		cbs.Disconnect()
	}()
}

// GetShutdownChannel returns the session's done channel, which will be closed when the session started ending.
// Channel may be nil, in which case it means the session has already closed.
func (cbs *callbackSession) GetShutdownChannel() <-chan struct{} {
	defer cbs.mtx.Unlock()
	cbs.mtx.Lock()
	return cbs.doneCh
}

// Disconnect manually requests a callback session to end. The effect is that resultCh is closed, which should
// signal the underlying connection to terminate. We attempt a mux clean up before hand, but its not guaranteed.
func (cbs *callbackSession) Disconnect() {
	defer cbs.mtx.Unlock()
	cbs.mtx.Lock()

	if cbs.doneCh == nil {
		log.Debugln("Session close already sent.")
		return
	}

	// Close the doneCh to signal watchers we're shutting down
	close(cbs.doneCh)
	cbs.doneCh = nil

	// Close the channel mux
	err := cbs.muxClient.Close()
	if err != nil {
		log.Errorln("Could not gracefully close mux session:", err)
	}

	// Send the channel mux close result to the error channel
	cbs.resultCh <- err
	close(cbs.resultCh)
}

// NewConnMan initializes a new connection manager
func NewConnectionManager(proxyBufferSize int) *ConnectionManager {
	return &ConnectionManager{
		callbackSessions: make(map[string]*callbackSession),
		clientSessions:   make(map[string]*ClientSessionDesc),

		clientSubscribers:   make(map[<-chan ClientSessionDesc]chan<- ClientSessionDesc),
		callbackSubscribers: make(map[<-chan CallbackSessionDesc]chan<- CallbackSessionDesc),

		proxyBufferSize: proxyBufferSize,
	}
}

// ListCallbackSessions returns a list of the callback session descriptions
// currently enabled.
func (this *ConnectionManager) ListCallbackSessions() *CallbackSessionList {
	this.callbackMtx.RLock()
	defer this.callbackMtx.RUnlock()

	ret := make(map[string]CallbackSessionDesc, len(this.callbackSessions))

	for k, v := range this.callbackSessions {
		ret[k] = v.desc
	}

	return &CallbackSessionList{
		SequenceNum: atomic.LoadUint32(&this.callbackSessionEventCounter),
		Sessions:    ret,
	}
}

// SubscribeCallbackEvents returns a channel which yields a stream of events when callback clients
// connect and disconnect.
func (this *ConnectionManager) SubscribeCallbackEvents(buffer int) <-chan CallbackSessionDesc {
	this.callbackSubscribersMutex.Lock()
	defer this.callbackSubscribersMutex.Unlock()

	ch := make(chan CallbackSessionDesc, buffer)
	writeCh := (chan<- CallbackSessionDesc)(ch)
	readCh := (<-chan CallbackSessionDesc)(ch)
	this.callbackSubscribers[readCh] = writeCh

	return readCh
}

// UnsubscribeCallbackEvents closes a callback events channel for a consumer.
func (this *ConnectionManager) UnsubscribeCallbackEvents(ch <-chan CallbackSessionDesc) {
	this.callbackSubscribersMutex.Lock()
	defer this.callbackSubscribersMutex.Unlock()
	writeCh, ok := this.callbackSubscribers[ch]
	if !ok {
		return
	}
	close(writeCh)
	delete(this.callbackSubscribers, ch)
}

// ListClientSessions returns a list of the callback session descriptions
// currently enabled.
func (this *ConnectionManager) ListClientSessions() *ClientSessionList {
	this.clientMtx.RLock()
	defer this.clientMtx.RUnlock()

	ret := make([]ClientSessionDesc, len(this.clientSessions))
	for _, v := range this.clientSessions {
		ret = append(ret, v.copy())
	}

	return &ClientSessionList{
		SequenceNum: atomic.LoadUint32(&this.clientSessionEventCounter),
		Sessions:    ret,
	}
}

// SubscribeCallbackEvents returns a channel which yields a stream of events when callback clients
// connect and disconnect.
func (this *ConnectionManager) SubscribeClientConnectionEvents(buffer int) <-chan ClientSessionDesc {
	this.clientSubscribersMutex.Lock()
	defer this.clientSubscribersMutex.Unlock()

	ch := make(chan ClientSessionDesc, buffer)
	writeCh := (chan<- ClientSessionDesc)(ch)
	readCh := (<-chan ClientSessionDesc)(ch)
	this.clientSubscribers[readCh] = writeCh

	return readCh
}

// UnsubscribeCallbackEvents closes a callback events channel for a consumer.
func (this *ConnectionManager) UnsubscribeClientConnectionEvents(ch <-chan ClientSessionDesc) {
	this.clientSubscribersMutex.Lock()
	defer this.clientSubscribersMutex.Unlock()
	writeCh, ok := this.clientSubscribers[ch]
	if !ok {
		return
	}
	close(writeCh)
	delete(this.clientSubscribers, ch)
}

// internal function for publishing an event record to all subscribers
//func (this *ConnectionManager) publishClientConnectionEvent(data ClientSessionDesc) {
//	this.clientSubscribersMutex.RLock()
//	defer this.clientSubscribersMutex.RUnlock()
//
//	for _, sub := range this.clientSubscribers {
//		select {
//		case sub <- event:
//			continue
//		default:
//			continue
//		}
//	}
//
//	// Increment the event sequence number
//	atomic.AddUint32(&this.clientSessionEventCounter, 1)
//}

// CallbackConnection sets up a new callback connection using the given
// callbackId and an incomingConn object. The remoteAddr is informational and
// should be any relevant string which identifies the callback origin.
// doneCh is optional, but recommended, and should be a channel which will close
// when the underlying connection is disconnected (this allows pre-emptive
// detection of connection failure).
func (this *ConnectionManager) CallbackConnection(callbackId string, remoteAddr string, incomingConn io.ReadWriteCloser, doneCh <-chan struct{}) <-chan error {
	log := log.With("remote_addr", remoteAddr).With("callback_id", callbackId)
	resultCh := make(chan error)

	go func() {
		this.callbackMtx.Lock()
		defer this.callbackMtx.Unlock()

		if callbackSession, found := this.callbackSessions[callbackId]; found {
			// Is the session closed?
			if !callbackSession.muxClient.IsClosed() {
				log.Errorln("Callback session already exists and is active.")
				if ierr := incomingConn.Close(); ierr != nil {
					log.Errorln("Error closing websocket connection:", ierr)
				}
				resultCh <- &ErrSessionExists{callbackId}
				return
			}
			log.Debugln("Callback session exists but was closed. Recreating.")
		}

		// Setup a mux session on the websocket
		log.Debugln("Setting up mux connection")
		muxSession, merr := yamux.Client(incomingConn, nil)
		if merr != nil {
			log.Errorln("Could not setup mux session:", merr)
			resultCh <- merr
			return
		}

		sessionData := CallbackSessionDesc{
			ConnectedAt: time.Now(),
			RemoteAddr:  remoteAddr,
		}

		newSession := &callbackSession{
			log:       log,
			muxClient: muxSession,
			resultCh:  resultCh,
			doneCh:    make(chan struct{}),
			desc:      sessionData,
		}

		log.Debugln("Starting shutdown channel monitoring")
		newSession.startShutdownWatch(doneCh)

		this.callbackSessions[callbackId] = newSession

		// When the channel shuts down it should be automatically removed from the connection manager
		go func() {
			<- doneCh
			log.Infoln("Cleaning up finished callback session.")
			defer this.callbackMtx.Unlock()
			this.callbackMtx.Lock()

			delete(this.callbackSessions, callbackId)
			log.Debugln("Callback session removed from manager.")
		}()

		log.Infoln("Established callback mux session.")
	}()

	return resultCh
}

// DisconnectCallbackConnection forcibly disconnects a callbackId session. It does not necessarily prevent the
// session from immediately reconnecting.
func (this *ConnectionManager) DisconnectCallbackConnection(callbackId string) error {
	this.callbackMtx.RLock()
	defer this.callbackMtx.RUnlock()

	callbackSession, found := this.callbackSessions[callbackId]
	if !found {
		return &ErrSessionUnknown{callbackId}
	}

	callbackSession.Disconnect()
	return nil
}

// ClientConnection attempts to connect to the callback reverse proxy session given by callbackId.
// Blocks until the connection is finished (should be called by a goroutine).
func (this *ConnectionManager) ClientConnection(callbackId string, remoteAddr string, incomingConn io.ReadWriteCloser, doneCh <- chan struct{}) <-chan error {
	log := log.With("remote_addr", remoteAddr).With("callback_id", callbackId)
	errCh := make(chan error)

	go func() {
		this.callbackMtx.RLock()

		// Check if we have a session with that name
		session, found := this.callbackSessions[callbackId]
		if !found {
			log.Errorln("Requested callback session does not exist.")
			errCh <- error(&ErrSessionUnknown{callbackId})
			close(errCh)
			this.callbackMtx.RUnlock()
			return
		}
		this.callbackMtx.RUnlock()

		// Session found, check its not shutting down...
		callbackDoneCh := session.GetShutdownChannel()
		if callbackDoneCh == nil {
			log.Errorln("Requested callback session is already closing.")
			errCh <- error(&ErrSessionDisconnected{callbackId})
			close(errCh)
			return
		}

		// Session seems to be alive, try and dial it. If we fail here we just give up.
		reverseConnection, err := session.muxClient.Open()
		if err != nil {
			log.Errorln("Establishing reverse connection failed:", err)
			errCh <- err
			close(errCh)
			return
		}

		// Setup session metadata.
		sessionData := &ClientSessionDesc{
			ConnectedAt: time.Now(),
			RemoteAddr:  remoteAddr,
			CallbackId:  callbackId,
			BytesOut:    0,
			BytesIn:     0,
		}

		// Add the session to the session list.
		this.clientMtx.Lock()
		this.clientSessions[callbackId] = sessionData
		this.clientMtx.Unlock()

		// Increment target sessions connected session count
		atomic.AddUint32(&session.desc.NumClients, 1)

		log.Infoln("Client connected to session.")

		// shutdownCh needs to combine the client's websocket status and the callback sessions connection status to
		// ensure prompt shutdown of the session is either fails.
		shutdownCh := make(chan struct{})
		go func() {
			select {
			case <- doneCh:
				log.Infoln("Client underlying connection closed.")
			case <- callbackDoneCh:
				log.Infoln("Callback session ended.")
			}
			close(shutdownCh)
		}()

		// Start the proxy session.
		errCh := util.HandleProxy(log, this.proxyBufferSize, incomingConn, reverseConnection, shutdownCh, &sessionData.BytesOut, &sessionData.BytesIn)
		cerr := <-errCh
		if cerr != io.EOF || cerr != nil {
			log.Errorln("Client disconnected from session due to error.")
		}

		log.Infoln("Client disconnected.")

		this.clientMtx.Lock()
		delete(this.clientSessions, callbackId)
		this.clientMtx.Unlock()

		// Decrement target session connected count. Even if the session has disappeared by now, this reference
		// will mean we have something to write to (which will then be GC'd out of existence).
		atomic.AddUint32(&session.desc.NumClients, ^uint32(0))
	}()

	return errCh
}
