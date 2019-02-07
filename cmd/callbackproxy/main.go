package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/wrouesnel/callback/util"
	"github.com/wrouesnel/callback/util/websocketrwc"
	"github.com/wrouesnel/go.log"
	"gopkg.in/alecthomas/kingpin.v2"
	"io"
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
	CallbackApiPath = "api/v1/connect"
)

var (
	app = kingpin.New("callbackproxy", "Simple websocket stdio proxy client for callback server")

	callbackServer = app.Flag("server", "Callback Server to connect to").URL()
	connectTimeout = app.Flag("timeout", "Connection timeout").Default("5s").Duration()

	basicUser     = app.Flag("http.user", "Basic Authentication User to use for connection").Envar("CALLBACKPROXY_USER").String()
	basicPassword = app.Flag("http.password", "Basic Authentication Password to use for connection").Envar("CALLBACKPROXY_PASSWORD").String()

	stripSuffix = app.Flag("strip-suffix", "Suffix to remove from the supplied callback ID").String()
	stripPrefix = app.Flag("strip-prefix", "Prefix to remove from the supplied callback ID").String()

	inputCallbackId = app.Arg("callbackId", "ID of the endpoint on the callback server to connect to").String()

	proxyBufferSize = app.Flag("proxy.buffer-size", "Size in bytes of connection buffers").Default("1024").Int()

	loglevel  = app.Flag("log-level", "Logging Level").Default("info").String()
	logformat = app.Flag("log-format", "If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.").Default("logger:stderr").String()
)

func basicAuthEncode(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

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

	if *inputCallbackId == "" {
		log.Fatalln("Cannot use a blank id")
	}

	callbackId := *inputCallbackId
	// Remove the given suffix
	callbackId = strings.TrimSuffix(callbackId, *stripSuffix)
	// Remove the given prefix
	callbackId = strings.TrimSuffix(callbackId, *stripPrefix)

	// Setup signal wait for shutdown
	signalCh := make(chan os.Signal, 1)
	shutdownCh := make(chan struct{})
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signalCh
		log.Infoln("Received Signal:", sig.String())
		close(shutdownCh)
		return
	}()

	if !strings.HasSuffix((*callbackServer).Path, "/") {
		(*callbackServer).Path = fmt.Sprintf("%s/", (*callbackServer).Path)
	}

	apiUrl, err := url.Parse(fmt.Sprintf("%s/%s", CallbackApiPath, callbackId))
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

	reqHeaders := http.Header{}
	if *basicUser != "" || *basicPassword != "" {
		log.Debugln("Setting HTTP basic auth.")
		reqHeaders.Set("Authorization", "Basic "+basicAuthEncode(*basicUser, *basicPassword))
	}

	wconn, _, err := wDialer.Dial(apiUri.String(), reqHeaders)
	if err != nil {
		log.Fatalln("Failed to connect to callback server:", err)
	}
	defer wconn.Close()

	rwc, wrapErr := websocketrwc.WrapClientWebsocket(wconn)
	if wrapErr != nil {
		log.Fatalln("Error while wrapping websocket:", wrapErr)
	}

	log := log.With("remote_addr", wconn.RemoteAddr())

	stdio := util.NewReadWriteCloser(os.Stdin, os.Stdout, func() error {
		log.Infoln("Close called on stdio")
		return nil
	})

	exitCh := make(chan int)
	// Start proxying
	resultCh := util.HandleProxy(log, *proxyBufferSize, stdio, rwc, shutdownCh, nil, nil)
	// Wait for user shutdown or resultCh
	go func() {
		select {
		case resultErr := <-resultCh:
			if resultErr != nil {
				if resultErr != io.EOF {
					log.Errorln("Connection closed with error:", resultErr)
					exitCh <- 1
				} else {
					log.Debugln("Connection closed with EOF")
					exitCh <- 0
				}
			} else {
				log.Debugln("Connection closed without error.")
				exitCh <- 0
			}
		case <-shutdownCh:
			log.Infoln("Exiting on user request.")
			exitCh <- 0
		}
	}()

	exitCode := <-exitCh
	os.Exit(exitCode)
}
