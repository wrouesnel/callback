// connman implements the callback connection manager.

package connman

import (
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/wrouesnel/go.log"
	"sync"
	"github.com/wrouesnel/callback/util"
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
	callbackMtx sync.RWMutex

	clientSessions map[string]*ClientSessionDesc
	clientMtx sync.RWMutex

	proxyBufferSize uint
}

// ClientSessionDesc holds connection information for a client session.
type ClientSessionDesc struct {
	// Establishment time
	ConnectedAt time.Time		`json:"connected_at"`
	// Connection details
	RemoteAddr string			`json:"remote_addr"`
	// Connection Target
	CallbackId string			`json:"callback_id"`
	// Connection tallies
	BytesOut uint64				`json:"bytes_out"`
	BytesIn  uint64				`json:"bytes_in"`
}

// CallbackSessionDesc holds connection infromation for a callback reverse proxy
// session
type CallbackSessionDesc struct {
	// Establishment time
	ConnectedAt time.Time		`json:"connected_at"`
	// Connection details
	RemoteAddr string			`json:"remote_addr"`
	// Number of clients
	NumClients uint				`json:"num_clients"`
}

// callbackSession holds the actual internal state of a session
type callbackSession struct {
	muxClient *yamux.Session
	*sync.Mutex
	CallbackSessionDesc
}

// NewConnMan initializes a new connection manager
func NewConnectionManager(proxyBufferSize uint) *ConnectionManager {
	return &ConnectionManager{
		callbackSessions : make(map[string]*callbackSession),
		clientSessions : make(map[string]*ClientSessionDesc),
		proxyBufferSize: proxyBufferSize,
	}
}

// CallbackConnection takes a callbackId and an established net.Conn object, and sets up the mux and reverse
// proxy system. Blocks until the incomingConn connection closes.
func (this* ConnectionManager) CallbackConnection(callbackId string, incomingConn *websocket.Conn) error {
	this.callbackMtx.Lock()
	defer this.callbackMtx.Unlock()

	sessionData := CallbackSessionDesc{
		ConnectedAt: time.Now(),
		RemoteAddr: incomingConn.RemoteAddr().String(),
	}

	log := log.With("remote_addr", sessionData.RemoteAddr).
		With("callback_id", callbackId)

	if callbackSession, found := this.callbackSessions[callbackId]; found {
		// Is the session closed?
		if !callbackSession.muxClient.IsClosed() {
			log.Errorln("Callback session already exists and is active.")
			if ierr := incomingConn.Close(); ierr != nil {
				log.Errorln("Error closing websocket connection:", ierr)
			}
			return &ErrSessionExists{callbackId}
		}
	}

	// Setup a mux session on the websocket
	log.Debugln("Setting up a callback connection")
	muxSession, merr := yamux.Client(incomingConn.UnderlyingConn(), nil)
	if merr != nil {
		log.Errorln("Could not setup mux session:", merr)
		return merr
	}

	newSession := &callbackSession{
		muxClient: muxSession,
		Mutex: &sync.Mutex{},
		CallbackSessionDesc: sessionData,
	}

	this.callbackSessions[callbackId] = newSession

	log.Infoln("Established callback mux session.")

	return nil
}

// ClientConnection attempts to connect to the callback reverse proxy session given by callbackId.
// Blocks until the connection is finished (should be called by a goroutine).
func (this* ConnectionManager) ClientConnection(callbackId string, incomingConn *websocket.Conn) error {
	this.callbackMtx.RLock()
	defer this.callbackMtx.RUnlock()

	sessionData := ClientSessionDesc{
		ConnectedAt: time.Now(),
		RemoteAddr: incomingConn.RemoteAddr().String(),
		CallbackId: callbackId,
		BytesOut: 0,
		BytesIn: 0,
	}

	log := log.With("remote_addr", sessionData.RemoteAddr).
		       With("callback_id", sessionData.CallbackId)

	// Check if we have a session with that name
	session, found := this.callbackSessions[callbackId]
	if !found {
		log.Errorln("Requested callback session does not exist.")
		return &ErrSessionUnknown{callbackId}
	}

	// We do, try and dial the client.
	reverseConnection, err := session.muxClient.Open()
	if err != nil {
		log.Errorln("Establishing reverse connection failed:", err)
		return err
	}

	// TODO: these seem unnecessary...
	session.Lock()
	session.NumClients += 1
	session.Unlock()

	log.Infoln("Client connected to session.")

	errCh := util.HandleProxy(log, this.proxyBufferSize,
		incomingConn.UnderlyingConn(), reverseConnection)

	log.Infoln("Client disconnected.")

	// TODO: these seem unnecessary...
	session.Lock()
	session.NumClients -= 1
	session.Unlock()

	return <- errCh
}