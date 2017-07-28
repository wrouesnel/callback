package util

import (
	"github.com/wrouesnel/go.log"
	"io"
)

type readWriteCloser struct {
	io.Reader
	io.Writer
	closeFn func() error
}

func (sc *readWriteCloser) Close() error {
	return sc.closeFn()
}

// NewReadWriteCloser combines a reader and writer into a ReadWriteCloser closed
// by the given closeFn.
func NewReadWriteCloser(r io.Reader, w io.Writer, closeFn func() error) io.ReadWriteCloser {
	return &readWriteCloser{r, w, closeFn}
}

// HandleProxy connects an incoming io.ReadWriteCloser to and outgoing
// io.ReadWriteCloser and sets up copy pipes between them. It returns a channel
// which yields the exit status as an error type - nil is returned if the
// connection closes normally.
func HandleProxy(log log.Logger, bufferSize int, incoming, outgoing io.ReadWriteCloser, shutdownCh <-chan struct{}) <-chan error {
	resultCh := make(chan error)

	go func() {
		defer func() { LogErr(log, incoming.Close()) }()
		defer func() { LogErr(log, outgoing.Close()) }()

		var proxyErr error

		// Forward data between connections
		// TODO: possibly allow shutting down the pipes.
		closedSrcDest := pipe(log, bufferSize, incoming, outgoing, shutdownCh)
		closedDestSrc := pipe(log, bufferSize, outgoing, incoming, shutdownCh)
		for {
			select {
			case sderr := <-closedSrcDest:
				if proxyErr == nil {
					proxyErr = sderr
				}
				closedSrcDest = nil
			case dserr := <-closedDestSrc:
				if proxyErr == nil {
					proxyErr = dserr
				}
				closedDestSrc = nil
			}
			if closedDestSrc == nil && closedSrcDest == nil {
				log.Debugln("All connections finished")
				break
			}
		}
		log.Debugln("Proxy session finished")
		resultCh <- proxyErr
		close(resultCh)
	}()

	return resultCh
}

// ErrIncompleteWrite is returned when a proxy tunnel write returns less written
// bytes then the input data.
type ErrIncompleteWrite struct{}

// Error implements error
func (err ErrIncompleteWrite) Error() string {
	return "incomplete write"
}

// pipe sets up a goroutine which proxies data between an io.Reader and io.Writer
// using a buffer of bufferSize.
// TODO: it feels like there should be windowing being done here?
// TODO: this function could be a lot cleaner
func pipe(log log.Logger, bufferSize int, src io.Reader, dst io.Writer, shutdownCh <-chan struct{}) <-chan error {
	closeCh := make(chan error)

	go func() {
		data := make([]byte, bufferSize)
		for {
			select {
			case <-shutdownCh:
				closeCh <- nil
				close(closeCh)
				log.Debugln("Pipe process shutting down on user request")
				return
			default:
				readBytes, rerr := src.Read(data)
				if rerr != nil {
					if rerr != io.EOF {
						log.Errorln("read error:", rerr)
						closeCh <- rerr
					} else {
						// EOF is a "normal" channel close
						closeCh <- nil
					}
					close(closeCh)
					return
				}
				writtenBytes, werr := dst.Write(data[:readBytes])
				if werr != nil {
					if werr != io.EOF {
						log.Errorln("write error:", werr)
					} else {
						// EOF is a "normal" channel close (probably should never
						// happen on a write?)
						closeCh <- nil
					}
					close(closeCh)
					return
				}
				if writtenBytes != readBytes {
					ierr := &ErrIncompleteWrite{}
					log.Errorln("write error:", ierr)
					closeCh <- ierr
					close(closeCh)
					return
				}
			}
		}
	}()

	return closeCh
}

// LogErr consumes an error and writes it as a log message.
func LogErr(log log.Logger, err error) {
	if err != nil {
		log.Errorln(err)
	}
}
