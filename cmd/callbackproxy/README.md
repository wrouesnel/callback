# Callback Proxy

Callback Proxy connects via Callback Server to the given reverse proxy connection
and forwards via stdin/stdout. This makes it suitable for use as an SSH
ProxyCommand.

## Example Usage
```
Host *.callback
  ProxyCommand callbackproxy --strip-suffix=.callback %h
```

This would allow the following ssh command line to work seamlessly:
```
ssh my-callback-url.callback
```
which would connect to whatever forwarder is being mediated via the callback
server.