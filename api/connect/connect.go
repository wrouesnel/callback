package connect

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/callback/api/apisettings"
	"github.com/wrouesnel/callback/util/websocketrwc"
	"github.com/wrouesnel/go.log"
	"net/http"
)

// ConnectGet establishes a websocket connection to
func ConnectGet(settings apisettings.APISettings) httprouter.Handle {
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

		incomingConn, uerr, doneCh := websocketrwc.Upgrade(w, r, nil, &upgrader)
		if uerr != nil {
			log.Errorln("Websocket upgrade failed:", uerr)
			return
		}
		log.Infoln("Connection upgrade successful.")

		log.Infoln("Connection upgrade successful. Registering callback session.")
		errCh := settings.ConnectionManager.ClientConnection(callbackId, r.RemoteAddr, incomingConn, doneCh)

		err := <-errCh
		if err != nil {
			log.Errorln("Callback session error:", err)
		} else {
			log.Infoln("Callback session ended normally.")
		}
	}
}

func SessionsGet(settings apisettings.APISettings) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer r.Body.Close()

		var err error

		sessions := settings.ConnectionManager.ListClientSessions()

		out, err := json.Marshal(&sessions)
		if err != nil {
			log.Errorln(err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(out)))

		w.Write(out)
	}
}

// SSE subscription to events dispatched via a channel from the bootmanager.
func Subscribe(settings apisettings.APISettings) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		defer r.Body.Close()

		log := log.With("remote_addr", r.RemoteAddr)

		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Errorln("HTTP.ResponseWriter was not castable as http.Flusher")
		}

		closeNotifier, ok := w.(http.CloseNotifier)
		if !ok {
			log.Errorln("HTTP.ResponseWriter was not castable as http.Flusher")
		}

		closeCh := closeNotifier.CloseNotify()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Subscribe to the notifier
		msgCh := settings.ConnectionManager.SubscribeClientConnectionEvents(1)
		defer settings.ConnectionManager.UnsubscribeClientConnectionEvents(msgCh)

		log.Debugln("New boot event subscriber")

		// Return messages as line-delimited JSON until connection closes or shutdown
		func() {
			// Send all messages we receive
			for {
				select {
				case msg := <-msgCh:
					log.Debugln("Sending boot event data")
					b, err := json.Marshal(&msg)
					if err != nil {
						log.Errorln("Error marshalling JSON to subscriber", err)
						return
					}
					_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", "boot", b)
					if err != nil {
						log.Debugln("Write failed closing subscription:", err)
						return
					}
					flusher.Flush()
				case <-closeCh:
					log.Debugln("Subscriber Client disconnected.")
					return
				}
			}
		}()
	}
}
