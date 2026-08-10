# zeptohttpd

A lightweight static HTTP server intended for development and operation checks.
The container image is a `scratch` image containing a single static binary
(~9 MB) and is available on [Docker Hub](https://hub.docker.com/r/morststs/zeptohttpd).

## Quick start

```sh
docker run --rm -p 8080:80 -v "$PWD/site:/public:ro" morststs/zeptohttpd
```

Everything under `/public` is served. `GET` and `HEAD` are answered; any other
method gets `405`. Query strings are ignored — only the URL path selects a file.

## Configuration

| Flag | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `-addr` | `ZEPTO_ADDR` | `:80` | Listen address |
| `-dir` | `ZEPTO_DIR` | `public` | Directory to serve |
| `-config` | `ZEPTO_CONFIG` | `config.json` | Path to the config file |
| `-dotfiles` | — | `false` | Serve paths whose elements start with `.` (e.g. `.git`, `.env`) |
| `-version` | — | — | Print the version and exit |

A missing config file is not an error — the server starts with defaults.

### config.json

`ContentTypes` maps a URL path prefix to the `Content-Type` to send for
requests under it. The **longest matching prefix wins**, so overlapping rules
are resolved deterministically:

```json
{
    "ContentTypes": {
        "/api/": "application/json",
        "/api/text/": "text/plain; charset=utf-8"
    }
}
```

Paths without a rule fall back to Go's normal extension/sniffing behaviour.

```sh
docker run --rm -p 8080:80 \
  -v "$PWD/site:/public:ro" \
  -v "$PWD/config.json:/config.json:ro" \
  morststs/zeptohttpd
```

## Notes on running

- The container runs as UID/GID `65532` (non-root). Docker allows binding port
  80 from a non-root user by default; on platforms that do not (some Kubernetes
  setups), use `-e ZEPTO_ADDR=:8080` and expose 8080 instead.
- `SIGTERM`/`SIGINT` trigger a graceful shutdown with a 10 s drain, so
  `docker stop` returns immediately instead of waiting out the kill timeout.
- Read/write/idle timeouts are set on the server, so a stalled client cannot
  hold a connection open indefinitely.
- Dot-prefixed paths are hidden by default to avoid serving `.git/`, `.env`,
  and similar files if they end up inside the served directory. Pass
  `-dotfiles` if you need `.well-known/`.
- One line is logged per request: method, quoted request URI, status, duration.

## Supported platforms

`linux/amd64`, `linux/arm64`, `linux/arm/v7`, `linux/arm/v6`, `linux/arm/v5`, `linux/386`.

## Development

```sh
cd src
go vet ./...
go test ./...
```

Build the image locally (tests run inside the build):

```sh
docker build -t zeptohttpd:dev .
```

Multi-platform build:

```sh
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v6,linux/arm/v5,linux/386 \
  --build-arg VERSION="$(git describe --tags --always)" -t zeptohttpd:dev .
```

## License

Apache-2.0. See [LICENSE](LICENSE).
