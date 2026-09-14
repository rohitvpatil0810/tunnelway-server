# Tunnelway Server

Tunnelway Server is the public-facing relay for Tunnelway. It accepts WebSocket connections from Tunnelway agents, assigns each agent a subdomain, and streams HTTP requests received on that subdomain to the agent's local service. The agent then streams the local service response back through the server.

The server is intentionally small and stateful in memory. It does not store application data, terminate TLS itself, authenticate agents, or provide a database-backed tunnel registry.

## How It Works

```text
                              HTTP request
Client  ->  <subdomain>.<main-domain>  ->  Tunnelway Server
                                             |
                                             | WebSocket binary frames
                                             v
                                      Tunnelway Agent
                                             |
                                             v
                                      Local service
```

1. An agent opens a WebSocket connection to `/_ws/agent`.
2. The server creates a tunnel session and sends the assigned public subdomain as JSON.
3. A client sends an HTTP request to that subdomain.
4. The server looks up the matching in-memory session.
5. The server sends request metadata and body chunks to the agent over the WebSocket.
6. The agent forwards the request to its configured local port.
7. The agent sends response metadata and body chunks back to the server.
8. The server writes the response to the original HTTP client.

By default, a newly connected agent receives a random four-character slug, for example:

```text
abcd.localtest.me:6000
```

An agent can also reconnect with its previous slug using the `agent_id` query parameter. If the session still exists, the server replaces the old WebSocket connection and preserves the public subdomain.

## Requirements

- Go 1.25.3 or newer
- A network-reachable host for the server
- DNS for the configured main domain, unless using a development domain such as `localtest.me`
- A Tunnelway agent configured with a WebSocket URL pointing to this server

The server and agent modules are separate Go modules. Run Go commands from `tunnelway-server/`.

## Quick Start

### Run locally

From this directory:

```bash
go run ./cmd/tunnelway-server
```

The default server listens on `0.0.0.0:6000` and uses `localtest.me:6000` as its main domain.

Start an agent against the local server, then expose a local service on port `3000`:

```bash
tunnelway setup
# Enter: ws://localhost:6000

tunnelway --port 3000
```

The agent prints the assigned public URL. With the defaults, it will look similar to:

```text
http://abcd.localtest.me:6000
```

Because `localtest.me` resolves to loopback, this is useful for local development. For a remote server, use a domain that resolves to the server's public address.

### Build the binary

```bash
go build -o tunnelway-server ./cmd/tunnelway-server
./tunnelway-server
```

## Configuration

Configuration is loaded from a `.env` file in the process working directory when present, followed by environment variables. Environment variables take precedence over values loaded from `.env`.

| Variable                | Default             | Description                                                 |
| ----------------------- | ------------------- | ----------------------------------------------------------- |
| `TUNNELWAY_LISTEN_ADDR` | `0.0.0.0:6000`      | TCP address and port used by the HTTP and WebSocket server. |
| `TUNNELWAY_MAIN_DOMAIN` | `localtest.me:6000` | Base domain included in assigned tunnel URLs.               |

Example:

```bash
TUNNELWAY_LISTEN_ADDR=0.0.0.0:8080 \
TUNNELWAY_MAIN_DOMAIN=tunnels.example.com \
go run ./cmd/tunnelway-server
```

A `.env` file can contain the same values:

```dotenv
TUNNELWAY_LISTEN_ADDR=0.0.0.0:6000
TUNNELWAY_MAIN_DOMAIN=localtest.me:6000
```

### Domain and DNS setup

For a production domain such as `tunnels.example.com`, create a wildcard DNS record pointing to the server:

```text
*.tunnels.example.com  A  <server-ip>
```

The server does not provision DNS records. The wildcard record must exist before clients can resolve generated tunnel subdomains.

When TLS is terminated by a reverse proxy, set the main domain to the externally visible host, for example `tunnels.example.com`, and forward both normal HTTP traffic and WebSocket upgrades to the server's listen address.

## HTTP API

### Public tunnel requests

All routes other than `/_ws/agent` are treated as public tunnel traffic. The server selects a session from the request host.

```text
<slug>.<main-domain>/<path>?<query>
```

Example:

```bash
curl -i http://abcd.localtest.me:6000/api/health
```

The path, query string, method, request headers, and request body are forwarded to the agent. The agent's response status, headers, and body are returned to the original client.

There is no separate health endpoint. A request to a host without a connected tunnel returns:

```http
HTTP/1.1 404 Not Found

Tunnel is not connected.
```

If an agent disconnects while a request is in flight, the request returns an internal server error. Requests are also limited by a 60-minute server-side timeout.

### Agent WebSocket endpoint

```text
GET /_ws/agent
```

The endpoint upgrades the connection to WebSocket and accepts binary Tunnelway frames after the connection is established.

Optional reconnect query parameter:

```text
GET /_ws/agent?agent_id=abcd
```

On a new session, the server sends a JSON text message:

```json
{ "subdomain": "abcd.localtest.me:6000" }
```

On a successful reconnection, the server sends:

```json
{ "reconnected": "true", "subdomain": "abcd.localtest.me:6000" }
```

The server accepts WebSocket connections from any origin. Put authentication and origin restrictions in front of the service before exposing it to untrusted clients.

## WebSocket Frame Protocol

Tunnelway request and response data uses binary WebSocket messages. Each message has this layout:

| Offset               |      Size | Field                          |
| -------------------- | --------: | ------------------------------ |
| `0`                  |    1 byte | Frame type                     |
| `1`                  |    1 byte | Request ID length, `N`         |
| `2`                  | `N` bytes | UTF-8 request ID               |
| after request ID     |   4 bytes | Big-endian payload length, `L` |
| after payload length | `L` bytes | Payload                        |

Frame type values are:

| Value | Name                     | Direction       | Payload                 |
| ----: | ------------------------ | --------------- | ----------------------- |
|   `0` | `FrameRequestStart`      | Server to agent | JSON request metadata   |
|   `1` | `FrameRequestBodyChunk`  | Server to agent | Raw request body bytes  |
|   `2` | `FrameRequestBodyEnd`    | Server to agent | Empty                   |
|   `3` | `FrameResponseStart`     | Agent to server | JSON response metadata  |
|   `4` | `FrameResponseBodyChunk` | Agent to server | Raw response body bytes |
|   `5` | `FrameResponseBodyEnd`   | Agent to server | Empty                   |

Request start payload:

```json
{
  "method": "POST",
  "url": "/api/items?limit=10",
  "headers": {
    "Content-Type": ["application/json"]
  }
}
```

Response start payload:

```json
{
  "StatusCode": 200,
  "Headers": {
    "Content-Type": ["application/json"]
  }
}
```

Request and response bodies are streamed in chunks. The server uses a 32 KiB buffer while reading an incoming HTTP request body. Request IDs are generated as UUIDs and are used to associate response frames with the pending public request.

## Docker

Build the image from the server directory:

```bash
docker build -t tunnelway-server .
```

Run it with the default port:

```bash
docker run --rm \
  --name tunnelway-server \
  -p 6000:6000 \
  -e TUNNELWAY_MAIN_DOMAIN=localtest.me:6000 \
  tunnelway-server
```

The image is built in two stages and runs from a minimal `scratch` runtime image as UID/GID `65532:65532`. The container exposes port `6000` and binds to `0.0.0.0:6000` by default.

For a production deployment behind a TLS reverse proxy:

```bash
docker run -d \
  --name tunnelway-server \
  -p 127.0.0.1:6000:6000 \
  -e TUNNELWAY_LISTEN_ADDR=0.0.0.0:6000 \
  -e TUNNELWAY_MAIN_DOMAIN=tunnels.example.com \
  tunnelway-server
```

The reverse proxy must support WebSocket upgrade requests for `/_ws/agent` and forward normal HTTP requests for tunnel subdomains.

## Deployment Notes

- Keep the server and all connected agents on compatible protocol versions.
- Use a wildcard DNS record for the main domain.
- Terminate TLS at a reverse proxy or load balancer and configure agents with a `wss://` URL.
- Ensure the proxy preserves the `Host` header; the server uses it to select the tunnel session.
- Forward WebSocket upgrade headers and use a sufficiently long idle timeout for agent connections.
- Run one server instance per tunnel namespace unless a shared session registry and connection routing layer are added.
- Monitor process restarts: tunnel sessions are held only in memory and disappear when the server stops.
- Protect the agent WebSocket endpoint with network controls or an authenticated proxy before production use.

## Current Limitations

- No authentication or authorization is implemented.
- WebSocket origin checks are permissive.
- Sessions and assigned slugs are stored only in process memory.
- There is no persistent registry, multi-instance coordination, or load-balancing support for active sessions.
- There is no built-in TLS termination.
- There is no built-in health, metrics, or administrative API.
- A reconnect can preserve a session only while the original session remains in the running server process.
- The server currently logs errors but does not expose structured operational metrics.

## Project Layout

```text
cmd/tunnelway-server/main.go       Application entry point
internal/config/config.go           Environment and .env configuration
internal/api/http-router/router.go  Public request fallback routing
internal/api/ws/agent_handler.go    Agent WebSocket upgrade and registration
internal/tunnel/manager.go           Session lookup and public request streaming
internal/tunnel/session.go           WebSocket state and binary frame protocol
pkg/logger/logger.go                Server logging setup
Dockerfile                           Multi-stage container build
```

## Related Component

The companion agent is maintained in the [tunnelway-agent repository](https://github.com/rohitvpatil0810/tunnelway-agent). Its setup flow writes a configuration containing:

```yaml
server_url: wss://your-server.example.com
server_path: /_ws/agent
```

The agent then connects to the server, forwards requests to a local port, and automatically attempts to reconnect using its assigned agent ID.
