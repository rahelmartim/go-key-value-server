//go:build js && wasm

// Command kvwasm runs the kvserver package in the browser, for the web demo.
// Each virtual client connects through net.Pipe, so the demo runs the same
// server code as the TCP version. It exposes a global "kv" object to
// JavaScript:
//
//	kv.connect(onMessage) -> client ID
//	kv.send(clientID, line)
//	kv.disconnect(clientID)
//	kv.snapshot() -> { key: value }
//	kv.onEvent = function (event) { ... }
package main

import (
	"bufio"
	"io"
	"net"
	"sync"
	"syscall/js"

	"github.com/rahelmartim/go-key-value-server/kvserver"
)

func main() {
	server := kvserver.New()
	api := js.Global().Get("Object").New()

	var mu sync.Mutex
	conns := map[int]net.Conn{}

	server.OnEvent = func(e kvserver.Event) {
		handler := api.Get("onEvent")
		if handler.Type() != js.TypeFunction {
			return
		}
		handler.Invoke(map[string]any{
			"kind":       string(e.Kind),
			"clientId":   e.ClientID,
			"key":        e.Key,
			"value":      e.Value,
			"recipients": e.Recipients,
		})
	}

	api.Set("connect", js.FuncOf(func(_ js.Value, args []js.Value) any {
		serverSide, clientSide := net.Pipe()
		id := server.ServeConn(serverSide)
		mu.Lock()
		conns[id] = clientSide
		mu.Unlock()

		onMessage := args[0]
		go func() {
			reader := bufio.NewReader(clientSide)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				onMessage.Invoke(id, line)
			}
		}()
		return id
	}))

	api.Set("send", js.FuncOf(func(_ js.Value, args []js.Value) any {
		mu.Lock()
		conn := conns[args[0].Int()]
		mu.Unlock()
		if conn != nil {
			line := args[1].String() + "\n"
			// net.Pipe blocks until the server reads, so write outside the JS call.
			go io.WriteString(conn, line)
		}
		return nil
	}))

	api.Set("disconnect", js.FuncOf(func(_ js.Value, args []js.Value) any {
		id := args[0].Int()
		mu.Lock()
		conn := conns[id]
		delete(conns, id)
		mu.Unlock()
		if conn != nil {
			conn.Close()
		}
		return nil
	}))

	api.Set("snapshot", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		// Snapshot waits on a channel, which a JS callback must not do, so the
		// result comes back as a Promise.
		return promise(func() any {
			result := map[string]any{}
			for key, value := range server.Snapshot() {
				result[key] = value
			}
			return result
		})
	}))

	js.Global().Set("kv", api)
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("kv-ready"))
	select {}
}

// promise runs fn on a goroutine and returns a JS Promise for its result.
func promise(fn func() any) js.Value {
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve := args[0]
		go func() {
			resolve.Invoke(fn())
			executor.Release()
		}()
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}
