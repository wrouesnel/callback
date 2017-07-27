package main

import (
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"flag"
	"github.com/wrouesnel/go.log"
	"github.com/wrouesnel/callback/connman"
	"github.com/bakins/logrus-middleware"
	"github.com/wrouesnel/multihttp"
	"syscall"
	"github.com/wrouesnel/callback/api/apisettings"
	"net/http"
	"github.com/wrouesnel/callback/api"
	"github.com/sirupsen/logrus"
	"github.com/wrouesnel/callback/assets"
	"os/signal"
)

// Version is set by the Makefile
var Version = "0.0.0.dev"

var (
	app = kingpin.New("callbackserver", "Callback Websocket Mediation Server")

	listenAddr  = app.Flag("listen.addr", "Port to listen on for API").Default("tcp://0.0.0.0:8080").Strings()
	staticProxy = app.Flag("debug.static-proxy", "URL of a proxy hosting static resources externally").URL()

	proxyBufferSize = app.Flag("proxy.buffer-size", "Size in bytes of connection buffers").Default("1024").Uint()

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

	log.Infoln("Log Level:", *loglevel)
	log.Infoln("Log Format:", *logformat)

	log.Infoln("Starting connection manager")
	connectionManager := connman.NewConnectionManager(*proxyBufferSize)

	settings := apisettings.APISettings{
		ConnectionManager: connectionManager,
		StaticProxy: *staticProxy,
	}

	// Setup HTTP router
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", api.NewAPI_v1(settings)))
	mux.Handle("/static/", http.StripPrefix("/static", assets.StaticFiles(settings)))
	mux.Handle("/", http.RedirectHandler("/static", http.StatusFound))

	// Setup logging middleware with logrus and go.log
	// FIXME: go.log should really provide an answer for this
	middlewareLogger := logrusmiddleware.Middleware{
		Name:   "callbackserver",
		Logger: logrus.StandardLogger(),
	}

	// Use multi-http to listen on all specified interfaces
	log.Infoln("Starting web interface")
	listeners, err := multihttp.Listen(*listenAddr, middlewareLogger.Handler(mux, "callback-api"))
	defer func() {
		for _, l := range listeners {
			if cerr := l.Close(); cerr != nil {
				log.Errorln("Error while closing listeners (ignored):", cerr)
			}
		}
	}()
	if err != nil {
		log.Panicln("Startup failed for a listener:", err)
	}
	for _, addr := range *listenAddr {
		log.Infoln("Listening on", addr)
	}

	// Setup signal wait for shutdown
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-shutdownCh
	log.Infoln("Terminating on signal:", sig)

}