# Callback HTTP Server

Callback HTTP server implements the HTTP server which mediates connections.

## Deployment

If deploying behind a proxy, the following headers are supported:

 * `X-Forwarded-For`
 * `X-Forwarded-Scheme`
 * `Forwarded` (the RFC7239 spec)
 
Set `--http.context-path` to a subpath if not deploying on a domain root.