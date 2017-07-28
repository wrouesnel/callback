package connect

import (
	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/callback/api/apisettings"
	"net/http"
	"github.com/wrouesnel/go.log"
	"github.com/gorilla/websocket"
)

// ConnectPost establishes a websocket connection to
func ConnectPost(settings apisettings.APISettings) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer r.Body.Close()

		log := log.With("remote_addr", r.RemoteAddr)

		callbackId := ps.ByName("callbackId")
		if callbackId == "" {
			log.Errorln("Received request for blank callbackId")
		}

		log.With("callbackid", callbackId)

		var upgrader = websocket.Upgrader{
			ReadBufferSize: settings.ReadBufferSize,
			WriteBufferSize: settings.WriteBufferSize,
		}

		incomingConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorln("Websocket upgrade failed:", err)
			http.Error(w, "Websocket upgrade failed", http.StatusInternalServerError)
		}

		log.Infoln("Connection upgrade successful. Registering callback session.")
		cerr := settings.ConnectionManager.ClientConnection(callbackId, incomingConn)
		if cerr != nil {
			log.Errorln("Client session error:", err)
		}
		log.Infoln("Client session ended normally")
	}
}