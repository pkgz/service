# service

Helper functions for service in golang.

## Installation
```bash
go get github.com/pkgz/service
```

## Usage

### Bootstrap a service

`Init` parses CLI flags / environment variables into your args struct (which must embed `ARGS`),
sets up the global logger, and returns a context that is cancelled on `SIGINT`/`SIGTERM`.

```go
type Args struct {
    service.ARGS
    DSN string `long:"dsn" env:"DSN" description:"database connection string"`
}

func main() {
    var args Args
    ctx, cancel, err := service.Init(&args)
    if err != nil {
        log.Fatal(err)
    }
    defer cancel()

    // ... run your service, respecting ctx cancellation ...
    <-ctx.Done()
}
```

If you only need the signal-aware context, use `service.ContextWithCancel()` directly.

### Connectors

Each connector dials the backend and verifies connectivity (ping/ready) before returning:

```go
mongo, err := service.NewMongo(ctx, "mongodb://localhost:27017")
redis, err := service.NewRedis(ctx, []string{"localhost:6379"}, "" /* password */)
influx, err := service.NewInflux(ctx, "http://localhost:8086", token, nil)
```

### Authenticator

`NewAuthenticator` performs an OAuth2 client-credentials flow against `AUTH_HOST`, fetches the
JWKS public key, and transparently refreshes the access token in the background until `ctx` is
cancelled. Configure it via `AUTH_HOST`, `AUTH_CLIENT_ID`, `AUTH_CLIENT_SECRET` (and optionally
`JWKS_KEY_ID`).

```go
auth, err := service.NewAuthenticator(ctx)
if err != nil {
    log.Fatal(err)
}

token := auth.Token()              // current access token
payload, err := auth.Verify(jwt)   // validate a JWT against the auth service's public key
```

### IDs

```go
id := service.UUID()              // UUID v4
short := service.ShortUUID()      // short, URL-friendly id
```

## Licence
[MIT License](https://github.com/pkgz/service/blob/master/LICENSE)