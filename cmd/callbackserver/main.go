package main

import (
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"net/http"

	"github.com/bakins/logrus-middleware"
	logrusmiddleware "github.com/bakins/logrus-middleware"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"github.com/stanvit/go-forwarded"
	"github.com/wrouesnel/callback/api"
	"github.com/wrouesnel/callback/api/apisettings"
	"github.com/wrouesnel/callback/assets"
	"github.com/wrouesnel/callback/connman"
	log "github.com/wrouesnel/go.log"
	"github.com/wrouesnel/multihttp"
	"gopkg.in/alecthomas/kingpin.v2"
)

// Version is set by the Makefile
var Version = "0.0.0.dev"

var (
	app = kingpin.New("callbackserver", "Callback Websocket Mediation Server")

	listenAddr           = app.Flag("listen.addr", "Port to listen on for API").Default("tcp://0.0.0.0:8080").Strings()
	contextPath          = app.Flag("http.context-path", "Subpath the application is being hosted under").Default("").String()
	allowedForwardedNets = app.Flag("http.local-networks", "Comma separated list of local networks which can set Forwarded headers").Default("127.0.0.0/8").String()

	staticProxy = app.Flag("debug.static-proxy", "URL of a proxy hosting static resources externally").URL()

	proxyBufferSize  = app.Flag("proxy.buffer-size", "Size in bytes of connection buffers").Default("1024").Int()
	handshakeTimeout = app.Flag("proxy.timeout", "Set maximum timeouts for connections").Default("3s").Duration()

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
		ContextPath:       *contextPath,
		StaticProxy:       *staticProxy,
		ReadBufferSize:    *proxyBufferSize,
		WriteBufferSize:   *proxyBufferSize,
		HandshakeTimeout:  *handshakeTimeout,
	}

	// Setup HTTP router
	router := httprouter.New()
	api.NewAPI_v1(settings, router)
	assets.StaticFiles(settings, router)
	//router.Handler("GET","/", http.RedirectHandler(settings.WrapPath("/static"), http.StatusFound))

	log.Debugln("Configuring callbackserver-http logging")
	middlewareLogger := logrusmiddleware.Middleware{
		Name:   "callbackserver",
		Logger: logrus.StandardLogger(),
	}

	handler := http.Handler(middlewareLogger.Handler(router, "callbackserver-http"))

	// Add the Forwarded middleware
	wrapper, werr := forwarded.New(*allowedForwardedNets,
		false, true,
		"X-Forwarded-For", "X-Forwarded-Protocol")
	if werr != nil {
		log.Fatalln("Could not setup forwarding middleware.")
	}

	handler = wrapper.Handler(handler)

	log.Infoln("Starting web interface")
	listeners, err := multihttp.Listen(*listenAddr, handler)
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
