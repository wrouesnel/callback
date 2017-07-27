Under Development!

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

