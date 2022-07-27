package callback

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/callback/api/apisettings"
	"github.com/wrouesnel/callback/util/websocketrwc"
	log "github.com/wrouesnel/go.log"
)

// CallbackPosts establishes a persistent websocket connection, and tries to
// back connect a yamux.Client instance to it (expecting a server on the other
// end.
func CallbackGet(settings apisettings.APISettings) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer r.Body.Close()

		log := log.With("remote_addr", r.RemoteAddr)

		callbackId := ps.ByName("callbackId")
		if callbackId == "" {
			log.Errorln("Received request for blank callbackId")
		}

		log.With("callbackid", callbackId)

		var upgrader = websocket.Upgrader{
			ReadBufferSize:  int(settings.ReadBufferSize),
			WriteBufferSize: int(settings.WriteBufferSize),
		}

		incomingConn, uerr, doneCh := websocketrwc.Upgrade(w, r, nil, &upgrader)
		if uerr != nil {
			log.Errorln("Websocket upgrade failed:", uerr)
			return
		}
		log.Infoln("Connection upgrade successful.")

		errCh := settings.ConnectionManager.CallbackConnection(callbackId, r.RemoteAddr, incomingConn, doneCh)

		err := <-errCh
		if err != nil {
			log.Errorln("Callback session error:", err)
		} else {
			log.Infoln("Callback session ended normally.")
		}
	}
}

// SessionsGet returns a list of currently active callback sessions.
func SessionsGet(settings apisettings.APISettings) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer r.Body.Close()

		var err error

		callbackSessions := settings.ConnectionManager.ListCallbackSessions()

		out, err := json.Marshal(&callbackSessions)
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
		msgCh := settings.ConnectionManager.SubscribeCallbackEvents(1)
		defer settings.ConnectionManager.UnsubscribeCallbackEvents(msgCh)

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
