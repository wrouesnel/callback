package main

import (
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/wrouesnel/callback/util"
	"github.com/wrouesnel/go.log"
	"gopkg.in/alecthomas/kingpin.v2"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// Version is set by the Makefile
var Version = "0.0.0.dev"

const (
	CallbackApiPath = "api/v1/callback"
)

var (
	app = kingpin.New("callbackreverse", "Callback Server Reverse Proxy Client")

	callbackServer = app.Flag("server", "Callback Server to connect to").URL()
	connectTimeout = app.Flag("timeout", "Connection timeout").Default("5s").Duration()

	forwardingAddress = app.Flag("connect", "Address and Port to forward to").String()
	callbackId        = app.Flag("id", "Callback ID to register as").String()

	//forever = app.Flag("forever", "Automatically reconnect on disconnect").Default("true").Bool()

	proxyBufferSize = app.Flag("proxy.buffer-size", "Size in bytes of connection buffers").Default("1024").Int()

	loglevel  = app.Flag("log-level", "Logging Level").Default("info").String()
	logformat = app.Flag("log-format", "If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.").Default("logger:stderr").String()
)

func main() {
	app.Version(Version)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	if err := flag.Set("log.level", *loglevel); err != nil {
		log.Fatalln("Could not set --log-level:", err)
	}
	if err := flag.Set("log.format", *logformat); err != nil {
		log.Fatalln("Could not set --log-format:", err)
	}

	if *callbackServer == nil {
		log.Fatalln("Must specify a callback server to connect to.")
	}

	if *callbackId == "" {
		log.Fatalln("Cannot use a blank id")
	}

	// Setup signal wait for shutdown
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	if !strings.HasSuffix((*callbackServer).Path, "/") {
		(*callbackServer).Path = fmt.Sprintf("%s/", (*callbackServer).Path)
	}

	apiUrl, err := url.Parse(fmt.Sprintf("%s/%s", CallbackApiPath, *callbackId))
	if err != nil {
		log.Fatalln("BUG: CallbackApiPath should always resolve")
	}

	apiUri := (*callbackServer).ResolveReference(apiUrl)
	if err != nil {
		log.Fatalln("Could not construct the callback API path from source URL:", (*callbackServer).String())
	}

	log.Infoln("Callback Server Endpoint:", apiUri.String())

	// Ensure the scheme is set correctly
	if apiUri.Scheme == "http" {
		apiUri.Scheme = "ws"
	}
	if apiUri.Scheme == "https" {
		apiUri.Scheme = "wss"
	}
	if apiUri.Scheme != "wss" && apiUri.Scheme != "ws" {
		log.Fatalln("Unrecognized URI for remote endpoint:", apiUri.Scheme)
	}

	wDialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: *connectTimeout,
		// TODO: what do you set the buffers to when you are going to mux over it
	}

	wconn, _, err := wDialer.Dial(apiUri.String(), nil)
	if err != nil {
		log.Fatalln("Failed to connect to callback server:", err)
	}
	defer wconn.Close()

	// Setup a yamux *server* on the websocket connection
	muxServer, merr := yamux.Server(wconn.UnderlyingConn(), nil)
	if merr != nil {
		log.Fatalln("Could not setup mux session:", merr)
	}

	// Launch the listener
	exitCh := make(chan error)
	go func() {
		for {
			incomingConn, aerr := muxServer.Accept()
			if aerr != nil {
				// TODO: when does a mux actually fail this? What happens with our
				// underlying connection?
				log.Errorln("Error accepting connection on mux:", aerr)
				exitCh <- aerr
				close(exitCh)
				return
			}

			// Update the logger with incoming detail
			log := log.With("incoming_remote_addr", incomingConn.RemoteAddr()).
				With("incoming_local_addr", incomingConn.LocalAddr())

			log.Debugln("Accepting connection on mux")

			outgoingConn, oerr := net.Dial("tcp", *forwardingAddress)
			if oerr != nil {
				log.With("forwarding_addr", *forwardingAddress).
					Errorln("Error establishing outgoing proxy connection")

				if icerr := incomingConn.Close(); icerr != nil {
					log.Errorln("Error while closing incoming mux connection:", icerr)
				}
				// No proxying - skip to continue accepting connections
				continue
			}

			// Update the logger with the outgoing detail
			log = log.With("outgoing_remote_addr", outgoingConn.RemoteAddr()).
				With("outgoing_local_addr", outgoingConn.LocalAddr())

			log.Debugln("Proxy connected.")
			errCh := util.HandleProxy(log, *proxyBufferSize, incomingConn, outgoingConn)
			go func() {
				perr := <-errCh
				if perr != nil {
					if perr != io.EOF {
						log.Errorln("Proxy connection terminated with error:", perr)
					} else {
						log.Debugln("Proxy connection exited normally.")
					}
				} else {
					log.Debugln("Proxy connection exited normally.")
				}
			}()
		}
	}()

	select {
	case sig := <-shutdownCh:
		log.Infoln("Terminating on signal:", sig)
	case eerr := <-exitCh:
		if eerr != nil {
			log.Errorln("Exiting due to error:", eerr)
		}
	}

	if mcerr := muxServer.Close(); mcerr != nil {
		log.Errorln("Got error while closing mux session:", mcerr)
	} else {
		log.Debugln("Mux session closed successfully.")
	}
}
