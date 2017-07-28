[![Build Status](https://travis-ci.org/wrouesnel/callback.svg?branch=master)](https://travis-ci.org/wrouesnel/callback)
[![Coverage Status](https://coveralls.io/repos/github/wrouesnel/callback/badge.svg?branch=master)](https://coveralls.io/github/wrouesnel/callback?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/wrouesnel/callback)](https://goreportcard.com/report/github.com/wrouesnel/callback)

# Callback

`callback` implements a `yamux` based reverse proxy service to clients. It
accepts connections and upgrades them to websockets to allow reverse proxying
back to the connecting client.

Each connection is then reflected as a forwarding connection endpoint, allowing
users to connect back to the daemons. This implements effective, universal
callback functionality for connecting to unknown hosts.

## API
The API is versioned under the `/api` endpoint. The current API endpoint prefix
is `/api/v1`.

`/callback` : 
    `GET` returns list of all callback sessions
    
`/callback/<identifier name>` : `POST` request to this endpoint initiates
websocket tunnel setup for a client. There is no authentication, identifiers
are handled "first come/first serve".

`/connect` :
    `GET` returns list of all connected user sessions.
    
`/connect/<identifier name>` : `POST` request to this endpoint initiates a
reverse proxy connection via a tunnel setup on the proxy. `404` will be returned
if the tunnel is not known. There is a configurable timeout incase the tunnels
destination has dropped and is waiting for a reconnect - if the server does not
reconnect then the tunnel is dropped.

`/events/callback` : `GET` request serves SSE updating callback events.
`/events/connect`  : `GET` request serves SSE updating connection events.

`/static`          : Static web assets are served under this path.

## Basic Usage

For this example we'll be just proxying to SSH on the host machine, you will
need 3 separate terminals to start it up and watch everything that happens:

Start the server
```bash
$ ./callbackserver  --log-level=debug
```
This starts the server on 0.0.0.0:8080 by default.

Now start a reverse proxy:
```bash
$ ./callbackreverse --log-level=debug --server=http://localhost:8080/ --id=testconnect --connect=127.0.0.1:22 --forever
```
This starts a new session identified by `testconnect`. If you were using another
websocket client, you would proxy to this session via `ws://localhost:8080/api/v1/connect/testconnect`

We can now use `callbackproxy` as an SSH `ProxyCommand` to connect back to the
localserver via the reverse tunnel:
```bash
$ ssh -oProxyCommand='./callbackproxy --log-level=debug --server=http://localhost:8080/ testconnect' $USER@localhost
```
Note `callbackproxy` is fairly loose about how you specify the the URI - using
other proxies might require specifying the URL as `ws`.

## What is this useful for?

I've had more then a few situations arise where being able to have a target
dial back to me, but expose SSH (so I can run a tool like Ansible) would have
been very useful.

Although the [`yamux`](https://github.com/hashicorp/yamux) library from Hashicorp existed, it didn't seem like anyone
had wrapped it into some useful command line utilities - so I created [`reverseit`](https://github.com/wrouesnel/reverseit)
to implement the basic concept.

`callback` is the logical evolution of the idea - rather then terminating
reverse proxy connections via SSH configuration, bypass the whole idea and
inject them directly into a more modern web-stack that would be amenable to
more management.

# TODO
* Web interface
* Add metrics tracking to client sessions
* Tidy up the error handling / reduce nested goroutine usage
* Performance test / optmize the websocket usage