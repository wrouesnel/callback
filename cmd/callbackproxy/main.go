package main

import (
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"flag"
	"github.com/wrouesnel/go.log"
	"syscall"
	"fmt"
	"net/url"
	"os/signal"
	"strings"
	"github.com/gorilla/websocket"
	"net/http"
	"github.com/wrouesnel/callback/util"
	"io"
)

// Version is set by the Makefile
var Version = "0.0.0.dev"

const (
	CallbackApiPath = "api/v1/connect"
)

var (
	app = kingpin.New("callbackreverse", "Simple websocket stdio proxy client for callback server")

	callbackServer = app.Flag("server", "Callback Server to connect to").URL()
	connectTimeout = app.Flag("timeout", "Connection timeout").Default("5s").Duration()

    stripSuffix = app.Flag("strip-suffix", "Suffix to remove from the supplied callback Id").String()

    callbackId = app.Arg("callbackId", "ID of the endpoint on the callback server to connect to").String()

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

	apiUrl, err := url.Parse(fmt.Sprintf("%s/%s",CallbackApiPath, *callbackId))
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
		Proxy: http.ProxyFromEnvironment,
		HandshakeTimeout: *connectTimeout,
		// TODO: what do you set the buffers to when you are going to mux over it
	}

	wconn, _, err := wDialer.Dial(apiUri.String(), nil)
	if err != nil {
		log.Fatalln("Failed to connect to callback server:", err)
	}
	defer wconn.Close()

	log := log.With("remote_addr", wconn.RemoteAddr())

	stdio := util.NewReadWriteCloser(os.Stdin, os.Stdout, func() error {
		log.Infoln("Close called on stdio")
		return nil
	})

	resultCh := util.HandleProxy(log, *proxyBufferSize, stdio, wconn.UnderlyingConn())
	resultErr := <-resultCh
	if resultErr != nil {
		if resultErr != io.EOF {
			log.Errorln("Connection closed with error:", resultErr)
		} else {
			log.Debugln("Connection closed with EOF")
		}
	} else {
		log.Debugln("Connection closed without error.")
	}

	os.Exit(0)
}
