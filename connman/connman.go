// connman implements the callback connection manager.

package connman

import (
	"github.com/hashicorp/yamux"
	"github.com/wrouesnel/callback/util"
	"github.com/wrouesnel/go.log"
	"io"
	"sync"
	"time"
)

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
	// callbackSessions holds the currently active muxes.
	callbackSessions map[string]*callbackSession
	callbackMtx      sync.RWMutex

	clientSessions map[string]*ClientSessionDesc
	clientMtx      sync.RWMutex

	proxyBufferSize int
}

// ClientSessionDesc holds connection information for a client session.
type ClientSessionDesc struct {
	// Establishment time
	ConnectedAt time.Time `json:"connected_at"`
	// Connection details
	RemoteAddr string `json:"remote_addr"`
	// Connection Target
	CallbackId string `json:"callback_id"`
	// Connection tallies
	BytesOut uint64 `json:"bytes_out"`
	BytesIn  uint64 `json:"bytes_in"`
}

// CallbackSessionDesc holds connection infromation for a callback reverse proxy
// session
type CallbackSessionDesc struct {
	// Establishment time
	ConnectedAt time.Time `json:"connected_at"`
	// Connection details
	RemoteAddr string `json:"remote_addr"`
	// Number of clients
	NumClients uint `json:"num_clients"`
}

// callbackSession holds the actual internal state of a session
type callbackSession struct {
	muxClient *yamux.Session
	*sync.Mutex
	// resultCh holds the channel which communicates connection failure/termination
	// to the underlying websocket. We send an error when we fail to connect,
	// to signal the underlying request to finish and allow a reset.
	resultCh chan<- error
	CallbackSessionDesc
}

// NewConnMan initializes a new connection manager
func NewConnectionManager(proxyBufferSize int) *ConnectionManager {
	return &ConnectionManager{
		callbackSessions: make(map[string]*callbackSession),
		clientSessions:   make(map[string]*ClientSessionDesc),
		proxyBufferSize:  proxyBufferSize,
	}
}

// ListCallbackSessions returns a list of the callback session descriptions
// currently enabled.
func (this *ConnectionManager) ListCallbackSessions() map[string]CallbackSessionDesc {
	this.callbackMtx.RLock()
	defer this.callbackMtx.RUnlock()

	ret := make(map[string]CallbackSessionDesc, len(this.callbackSessions))

	for k, v := range this.callbackSessions {
		ret[k] = v.CallbackSessionDesc
	}

	return ret
}

// ListClientSessions returns a list of the callback session descriptions
// currently enabled.
func (this *ConnectionManager) ListClientSessions() []ClientSessionDesc {
	this.clientMtx.RLock()
	defer this.clientMtx.RUnlock()

	ret := make([]ClientSessionDesc, len(this.clientSessions))
	for _, v := range this.clientSessions {
		ret = append(ret, *v)
	}

	return ret
}

// CallbackConnection sets up a new callback connection using the given
// callbackId and an incomingConn object. The remoteAddr is informational and
// should be any relevant string which identifies the callback origin.
// doneCh is optional, but recommended, and should be a channel which will close
// when the underlying connection is disconnected (this allows pre-emptive
// detection of connection failure).
func (this *ConnectionManager) CallbackConnection(callbackId string, remoteAddr string, incomingConn io.ReadWriteCloser, doneCh <-chan struct{}) <-chan error {

	log := log.With("remote_addr", remoteAddr).With("callback_id", callbackId)
	errCh := make(chan error)

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
				errCh <- &ErrSessionExists{callbackId}
				return
			}
		}

		// Setup a mux session on the websocket
		log.Debugln("Setting up mux connection")
		muxSession, merr := yamux.Client(incomingConn, nil)
		if merr != nil {
			log.Errorln("Could not setup mux session:", merr)
			errCh <- merr
			return
		}

		sessionData := CallbackSessionDesc{
			ConnectedAt: time.Now(),
			RemoteAddr:  remoteAddr,
		}

		newSession := &callbackSession{
			muxClient:           muxSession,
			Mutex:               &sync.Mutex{},
			resultCh:            errCh,
			CallbackSessionDesc: sessionData,
		}

		// Start a watcher goroutine to detect if
		go func() {

		}()

		this.callbackSessions[callbackId] = newSession

		log.Infoln("Established callback mux session.")
	}()

	return errCh
}

// ClientConnection attempts to connect to the callback reverse proxy session given by callbackId.
// Blocks until the connection is finished (should be called by a goroutine).
func (this *ConnectionManager) ClientConnection(callbackId string, remoteAddr string, incomingConn io.ReadWriteCloser) <-chan error {
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

		// We do, try and dial the client.
		reverseConnection, err := session.muxClient.Open()
		if err != nil {
			log.Errorln("Establishing reverse connection failed:", err)
			errCh <- err
			close(errCh)
			return
		}

		sessionData := &ClientSessionDesc{
			ConnectedAt: time.Now(),
			RemoteAddr:  remoteAddr,
			CallbackId:  callbackId,
			BytesOut:    0,
			BytesIn:     0,
		}

		this.clientMtx.Lock()
		this.clientSessions[callbackId] = sessionData
		this.clientMtx.Unlock()

		// TODO: these seem unnecessary...
		session.Lock()
		session.NumClients += 1
		session.Unlock()

		log.Infoln("Client connected to session.")

		errCh := util.HandleProxy(log, this.proxyBufferSize, incomingConn, reverseConnection, nil)
		cerr := <-errCh
		if cerr != io.EOF || cerr != nil {
			log.Errorln("Client disconnected from session due to error.")
			// TODO: trigger a disconnect of a bad mux ?
			//session.resultCh <- cerr
		}

		log.Infoln("Client disconnected.")

		this.clientMtx.Lock()
		delete(this.clientSessions, callbackId)
		this.clientMtx.Unlock()

		// TODO: these seem unnecessary...
		session.Lock()
		session.NumClients -= 1
		session.Unlock()
	}()

	return errCh
}
