// Package websocketrwc wraps Gorilla websocket in an io.ReadWriteCloser
// compatible interface, allowing it to be used as a net/rpc or similar
// connection.  Only the server side is currently implemented.
package websocketrwc

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultHandshakeTimeout = 10 * time.Second
	defaultBufferSize       = 4096
)

var (
	// ErrClosing is returned for operations during close phase.
	ErrClosing = errors.New(`Closing`)
	// DefaultUpgrader will be used if a nil upgrader is passed to Upgrade().
	DefaultUpgrader = &websocket.Upgrader{
		HandshakeTimeout:  defaultHandshakeTimeout,
		ReadBufferSize:    defaultBufferSize,
		WriteBufferSize:   defaultBufferSize,
		CheckOrigin:       func(r *http.Request) bool { return true },
	}
	// WriteTimeout determines the period of time a write will wait to complete
	// before producing an error.
	WriteTimeout = 10 * time.Second
	// PongTimeout determines the period of time a connection will wait for a
	// Pong response to Pings sent to the client.
	PongTimeout = 60 * time.Second
	// PingInterval determins the interval at which Pings are sent to the
	// client.
	PingInterval = (PongTimeout * 9) / 10
)

// safeBuffer adds thread-safety to *bytes.Buffer
type safeBuffer struct {
	buf *bytes.Buffer
	sync.Mutex
}

// Read reads the next len(p) bytes from the buffer or until the buffer is drained.
func (s *safeBuffer) Read(p []byte) (int, error) {
	s.Lock()
	defer s.Unlock()
	return s.buf.Read(p)
}

// Write appends the contents of p to the buffer.
func (s *safeBuffer) Write(p []byte) (int, error) {
	s.Lock()
	defer s.Unlock()
	return s.buf.Write(p)
}

// Len returns the number of bytes of the unread portion of the buffer.
func (s *safeBuffer) Len() int {
	s.Lock()
	defer s.Unlock()
	return s.buf.Len()
}

// Reset resets the buffer to be empty.
func (s *safeBuffer) Reset() {
	s.Lock()
	s.buf.Reset()
	s.Unlock()
}

// Conn wraps gorilla websocket to provide io.ReadWriteCloser.
type Conn struct {
	ws     *websocket.Conn
	buf    *safeBuffer
	done   chan struct{}
	wmutex sync.Mutex
	rmutex sync.Mutex
}

// Read implements io.Reader by wrapping websocket messages in a buffer.
func (c *Conn) Read(p []byte) (n int, err error) {
	if c.buf.Len() == 0 {
		var r io.Reader
		c.buf.Reset()
		c.rmutex.Lock()
		defer c.rmutex.Unlock()
		select {
		case <-c.done:
			err = ErrClosing
		default:
			err = c.ws.SetReadDeadline(time.Now().Add(PongTimeout))
			if err == nil {
				_, r, err = c.ws.NextReader()
			}
		}
		if err != nil {
			return n, err
		}
		_, err = io.Copy(c.buf, r)
		if err != nil {
			return n, err
		}
	}

	return c.buf.Read(p)
}

// Write implements io.Writer and sends binary messages only.
func (c *Conn) Write(p []byte) (n int, err error) {
	return c.write(websocket.BinaryMessage, p)
}

// write wraps the websocket writer.
func (c *Conn) write(messageType int, p []byte) (n int, err error) {
	c.wmutex.Lock()
	defer c.wmutex.Unlock()
	select {
	case <-c.done:
		err = ErrClosing
	default:
		err = c.ws.SetWriteDeadline(time.Now().Add(WriteTimeout))
		if err == nil {
			err = c.ws.WriteMessage(messageType, p)
		}
	}
	if err == nil {
		n = len(p)
	}
	return n, err
}

// Close implements io.Closer and closes the underlying connection.
func (c *Conn) Close() error {
	c.rmutex.Lock()
	c.wmutex.Lock()
	defer func() {
		c.rmutex.Unlock()
		c.wmutex.Unlock()
	}()
	select {
	case <-c.done:
		return ErrClosing
	default:
		close(c.done)
	}
	return c.ws.Close()
}

// pinger sends ping messages on an interval for client keep-alive.
func (c *Conn) pinger() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if _, err := c.write(websocket.PingMessage, []byte{}); err != nil {
				_ = c.Close()
			}
		}
	}
}

// newSafeBuffer instantiates a new safeBuffer
func newSafeBuffer() *safeBuffer {
	return &safeBuffer{
		buf: bytes.NewBuffer(nil),
	}
}

// Upgrade a HTTP connection to return the wrapped Conn.
func Upgrade(w http.ResponseWriter, r *http.Request, h http.Header, upgrader *websocket.Upgrader) (*Conn, error) {
	if upgrader == nil {
		upgrader = DefaultUpgrader
	}
	ws, err := upgrader.Upgrade(w, r, h)
	if err != nil {
		return nil, err
	}

	conn := &Conn{
		ws:   ws,
		buf:  newSafeBuffer(),
		done: make(chan struct{}),
	}

	// Set read deadline to detect failed clients.
	if err = conn.ws.SetReadDeadline(time.Now().Add(PongTimeout)); err != nil {
		return nil, err
	}
	// Reset read deadline on Pong.
	conn.ws.SetPongHandler(func(string) error {
		return conn.ws.SetReadDeadline(time.Now().Add(PongTimeout))
	})

	// Start ping loop for client keep-alive.
	go conn.pinger()

	return conn, nil
}