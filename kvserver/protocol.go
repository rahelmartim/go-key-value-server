package kvserver

import "strings"

// command is one parsed request line from a client.
type command struct {
	op    string // "put" or "get"
	key   string
	value []byte
}

// parseCommand parses one request line. The protocol has two commands:
//
//	put,<key>,<value>\n  stores the value under the key
//	get,<key>\n          sends "<key>,<value>\n" to every connected client
//
// The value of a put can contain commas. The second result is false for a
// line that does not match the protocol.
func parseCommand(line string) (command, bool) {
	parts := strings.SplitN(strings.TrimRight(line, "\r\n"), ",", 3)
	switch {
	case len(parts) == 3 && parts[0] == "put":
		return command{op: "put", key: parts[1], value: []byte(strings.TrimSpace(parts[2]))}, true
	case len(parts) == 2 && parts[0] == "get":
		return command{op: "get", key: strings.TrimSpace(parts[1])}, true
	default:
		return command{}, false
	}
}
