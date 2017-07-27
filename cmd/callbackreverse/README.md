# Callback Reverse

Callback Reverse implements a simple TCP reverse proxy.

The reverse proxy process connects to a callback server via websockets, and
establishes a `yamux` session over websockets, forwarding incoming connections
to the specified local port.

## Example
```
$ callback-reverse --server http://my-call-back-server --connect 127.0.0.1:22 --id $(hostname -f)
```

This command line would establish a connection to the callback server with the current local
hostname as the callback ID. Incoming connections will be proxied to port 22 - i.e. the callback
server provides a gateway to connect SSH (its recommended usage).