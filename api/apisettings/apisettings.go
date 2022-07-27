// apisettings package implements internal structs which configure the HTTP api.

package apisettings

import (
	"net/url"
	"path/filepath"
	"time"

	"github.com/wrouesnel/callback/connman"
)

const (
	// Imported by other utilities so we can track the latest API automatically
	CallbackLatestApi = "v1"
)

type APISettings struct {
	ConnectionManager *connman.ConnectionManager

	// ContextPath is any URL-prefix being passed by a reverse proxy.
	ContextPath string
	StaticProxy *url.URL

	// Websocket buffer sizes
	ReadBufferSize  int
	WriteBufferSize int

	// Websocket Timeouts
	HandshakeTimeout time.Duration
}

// WrapPath wraps a given URL string in the context path
func (api *APISettings) WrapPath(path string) string {
	return filepath.Join(api.ContextPath, path)
}
