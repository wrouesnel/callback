package connect

import (
	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/callback/api/apisettings"
	"github.com/wrouesnel/callback/util/websocketrwc"
	"github.com/wrouesnel/go.log"
	"net/http"
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
			ReadBufferSize:  settings.ReadBufferSize,
			WriteBufferSize: settings.WriteBufferSize,
		}

		incomingConn, uerr := websocketrwc.Upgrade(w, r, nil, &upgrader)
		if uerr != nil {
			log.Errorln("Websocket upgrade failed:", uerr)
			return
		}
		log.Infoln("Connection upgrade successful.")

		log.Infoln("Connection upgrade successful. Registering callback session.")
		errCh := settings.ConnectionManager.ClientConnection(callbackId, r.RemoteAddr, incomingConn)

		err := <-errCh
		if err != nil {
			log.Errorln("Callback session error:", err)
		} else {
			log.Infoln("Callback session ended normally.")
		}
	}
}
