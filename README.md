# Go Key-Value Server

[![CI](https://github.com/rahelmartim/go-key-value-server/actions/workflows/ci.yml/badge.svg)](https://github.com/rahelmartim/go-key-value-server/actions/workflows/ci.yml)
[![Live demo](https://img.shields.io/badge/live%20demo-GitHub%20Pages-00ADD8)](https://rahelmartim.github.io/go-key-value-server/)
[![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)

A concurrent key-value database server over TCP, written in Go with goroutines and channels. Clients store values with `put` and read them with `get`. The server sends every `get` result to all connected clients.

Live demo: https://rahelmartim.github.io/go-key-value-server/

The live demo runs the same server package in your browser. It is compiled to WebAssembly, and the virtual clients connect to it through in-memory connections. You can add clients, send commands, and watch each message go to the server and back to every client.

## Protocol

Each command is one line of text that ends with `\n`.

| Client sends | Effect |
| --- | --- |
| `put,<key>,<value>` | Stores the value under the key. The value can contain commas. |
| `get,<key>` | Sends `<key>,<value>` to every connected client. |

The server ignores lines that do not match the protocol.

Example with two clients:

```text
client A → put,color,blue
client B → get,color
client A ← color,blue
client B ← color,blue
```

## Design

One goroutine owns the store and the set of connected clients. Other goroutines send it requests through channels. For this reason, the server needs no locks for its state.

Each client has two goroutines: one reads commands, and one writes messages. Each client also has a queue of 500 messages. When the queue of a slow client is full, the server drops new messages for that client. This way, one slow client cannot block the other clients.

`Close()` closes the listener and every client connection, and it stops all background goroutines.

## Getting started

You need Go 1.24 or later.

1. Start the server.

   ```bash
   go run ./cmd/kvserver -v
   ```

   The `-v` flag logs every event. To listen on a different address, use `-addr`, for example `-addr :9999`.

2. In two or more other terminals, start a client.

   ```bash
   go run ./cmd/kvclient
   ```

3. Type commands in a client, for example `put,color,blue` and then `get,color`. Every client prints `color,blue`.

You can also connect with any TCP tool, for example `nc localhost 9999`.

## Use the package

```go
import "github.com/rahelmartim/go-key-value-server/kvserver"

server := kvserver.New()
server.OnEvent = func(e kvserver.Event) { log.Println(e.Kind, e.Key) }
if err := server.Listen(":9999"); err != nil {
	log.Fatal(err)
}
defer server.Close()
```

`Server` also has `Count()`, which returns the number of connected clients, and `Snapshot()`, which returns a copy of the store. `ServeConn(conn)` serves a connection that does not come from a TCP listener, for example one end of `net.Pipe`. The web demo uses it.

## Commands

| Command | Description |
| --- | --- |
| `make test` | Runs `go vet` and the tests with the race detector. |
| `make build` | Builds `kvserver` and `kvclient` into `bin/`. |
| `make web` | Builds the browser demo into `site/`. |
| `make serve` | Builds the browser demo and serves it on http://localhost:8080. It needs Python 3. To use another port, run `make serve PORT=8090`. |

## Project structure

```text
cmd/
├── kvserver/   # TCP server command
├── kvclient/   # Interactive TCP client command
└── kvwasm/     # WebAssembly entry point for the browser demo
kvserver/       # Server package, protocol, and tests
web/            # Browser demo: HTML, CSS, and JavaScript
```

## Tests

The tests in `kvserver/server_test.go` are the official tests of the course assignment that this project started from. They check many clients, slow clients, and the client count. The other test files check the protocol, the broadcast of `get` results, invalid lines, and `Close()`.

```bash
make test
```

## Deployment

The `Deploy demo to GitHub Pages` workflow builds the demo and publishes it on every push to `master`. The demo is a static site, so it needs no server.

## License

This project uses the MIT license. Read the [LICENSE](LICENSE) file for the full text.
