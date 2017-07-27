package assets

import (
	"github.com/wrouesnel/callback/api/apisettings"
	"github.com/elazarl/go-bindata-assetfs"
	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/go.log"
	"net/http"
	"net/http/httputil"
)

var rootDir = "assets/static"

// Appends a new static files API to the supplied router
func StaticFiles(settings apisettings.APISettings) http.Handler {
	router := httprouter.New()

	// Static asset handling
	if settings.StaticProxy != nil {
		log.Infoln("Proxying static assets from", settings.StaticProxy)
		revProxy := httputil.NewSingleHostReverseProxy(settings.StaticProxy)
		router.Handler("GET", "/*filepath", revProxy)
	} else {
		router.Handler("GET", "/*filepath",
			http.FileServer(&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, Prefix: ""}))
	}

	return router
}
