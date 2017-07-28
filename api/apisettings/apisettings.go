// apisettings package implements internal structs which configure the HTTP api.

package apisettings

import (
	"github.com/wrouesnel/callback/connman"
	"net/url"
	"time"
)

const (
	// Imported by other utilities so we can track the latest API automatically
	CallbackLatestApi = "v2"
)

type APISettings struct {
	ConnectionManager *connman.ConnectionManager

	StaticProxy *url.URL

	// Websocket buffer sizes
	ReadBufferSize  int
	WriteBufferSize int

	// Websocket Timeouts
	HandshakeTimeout time.Duration
}
