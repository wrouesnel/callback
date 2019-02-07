// apisettings package implements internal structs which configure the HTTP api.

package apisettings

import (
	"github.com/jinzhu/gorm"
	"github.com/wrouesnel/callback/connman"
	"net/url"
	"path/filepath"
	"time"
)

const (
	// Imported by other utilities so we can track the latest API automatically
	CallbackLatestApi = "v1"
)

// APISettings contains settings needed for the API to operate
type APISettings struct {
	// ConnectionManager holds a handle to the backend connection manager
	ConnectionManager *connman.ConnectionManager
	// DbConn is the gorm database connection
	DbConn *gorm.DB

	// ContextPath is any URL-prefix being passed by a reverse proxy.
	ContextPath string
	// StaticProxy if set is the backend static resources should be returned from
	StaticProxy *url.URL

	// Websocket read buffer size
	ReadBufferSize int
	// Websocket write buffer size
	WriteBufferSize int

	// Websocket handshake timeouts
	HandshakeTimeout time.Duration
}

// WrapPath wraps a given URL string in the context path
func (api *APISettings) WrapPath(path string) string {
	return filepath.Join(api.ContextPath, path)
}
