// Package kvserver implements a key-value database server over TCP.
//
// Clients send text commands, one per line. A put stores a value. A get
// sends the current value of a key to every connected client, not only to
// the client that asked. See parseCommand for the protocol.
//
// One goroutine owns all server state: the store and the set of clients.
// Other goroutines talk to it through channels, so the package uses no locks
// for this state.
package kvserver

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
)

// outboxSize is the number of messages that the server queues for one
// client. When a slow client's queue is full, the server drops new
// messages for that client, so that one slow client cannot block the others.
const outboxSize = 500

// ErrClosed is returned by Start when the server is already closed.
var ErrClosed = errors.New("kvserver: server is closed")

// KeyValueServer is a centralized key-value database server.
type KeyValueServer interface {
	// Start listens on localhost:port and accepts clients in the background.
	// It does not block.
	Start(port int) error
	// Count returns the number of connected clients.
	Count() int
	// Close disconnects all clients and stops the server.
	Close()
}

// EventKind identifies an event that the server reports to Server.OnEvent.
type EventKind string

const (
	EventConnect    EventKind = "connect"
	EventDisconnect EventKind = "disconnect"
	EventPut        EventKind = "put"
	EventGet        EventKind = "get"
	EventDrop       EventKind = "drop"
)

// Event describes one thing that happened in the server.
type Event struct {
	Kind     EventKind
	ClientID int
	Key      string
	Value    string
	// Recipients is the number of clients that received a get result.
	Recipients int
}

// Server is the KeyValueServer implementation.
type Server struct {
	// OnEvent, if set, receives every event. It runs on the goroutine that
	// owns the server state, so it must return quickly. Set it before Start.
	OnEvent func(Event)

	register   chan *client
	unregister chan *client
	requests   chan request
	queries    chan func(*state)
	quit       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
	startOnce  sync.Once
	listenerMu sync.Mutex
	listener   net.Listener
	nextID     int
	idMu       sync.Mutex
}

type request struct {
	from *client
	cmd  command
}

type state struct {
	store   map[string][]byte
	clients map[*client]struct{}
}

type client struct {
	id     int
	conn   net.Conn
	outbox chan []byte
}

var _ KeyValueServer = (*Server)(nil)

// New returns a server that is ready to start.
func New() *Server {
	return &Server{
		register:   make(chan *client),
		unregister: make(chan *client),
		requests:   make(chan request),
		queries:    make(chan func(*state)),
		quit:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Start listens on localhost:port. It implements KeyValueServer.
func (s *Server) Start(port int) error {
	return s.Listen(net.JoinHostPort("localhost", strconv.Itoa(port)))
}

// Listen listens on a TCP address, for example ":9999", and accepts clients
// in the background. It does not block.
func (s *Server) Listen(addr string) error {
	if s.isClosed() {
		return ErrClosed
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("kvserver: listen on %s: %w", addr, err)
	}
	s.listenerMu.Lock()
	s.listener = listener
	s.listenerMu.Unlock()

	s.run()
	go s.acceptLoop(listener)
	return nil
}

// ServeConn adds one client connection to the server and returns its client
// ID. Use it to serve connections that do not come from a TCP listener, for
// example one end of net.Pipe.
func (s *Server) ServeConn(conn net.Conn) int {
	s.run()
	s.idMu.Lock()
	s.nextID++
	c := &client{id: s.nextID, conn: conn, outbox: make(chan []byte, outboxSize)}
	s.idMu.Unlock()

	select {
	case s.register <- c:
		go s.readLoop(c)
		go c.writeLoop()
	case <-s.quit:
		conn.Close()
	}
	return c.id
}

// Count returns the number of connected clients.
func (s *Server) Count() int {
	var count int
	s.query(func(st *state) { count = len(st.clients) })
	return count
}

// Snapshot returns a copy of all stored key-value pairs.
func (s *Server) Snapshot() map[string]string {
	snapshot := map[string]string{}
	s.query(func(st *state) {
		for key, value := range st.store {
			snapshot[key] = string(value)
		}
	})
	return snapshot
}

// Close disconnects all clients and stops all background goroutines.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.quit)
		s.listenerMu.Lock()
		if s.listener != nil {
			s.listener.Close()
		}
		s.listenerMu.Unlock()
	})
	s.startOnce.Do(func() { close(s.done) }) // The loop never started.
	<-s.done
}

func (s *Server) isClosed() bool {
	select {
	case <-s.quit:
		return true
	default:
		return false
	}
}

// query runs fn on the goroutine that owns the state, and waits for it.
func (s *Server) query(fn func(*state)) {
	finished := make(chan struct{})
	select {
	case s.queries <- func(st *state) { fn(st); close(finished) }:
		<-finished
	case <-s.quit:
	}
}

// run starts the state loop once.
func (s *Server) run() {
	s.startOnce.Do(func() { go s.loop() })
}

// loop owns the store and the client set.
func (s *Server) loop() {
	defer close(s.done)
	st := &state{store: map[string][]byte{}, clients: map[*client]struct{}{}}

	for {
		select {
		case c := <-s.register:
			st.clients[c] = struct{}{}
			s.emit(Event{Kind: EventConnect, ClientID: c.id})

		case c := <-s.unregister:
			if _, ok := st.clients[c]; ok {
				delete(st.clients, c)
				close(c.outbox)
				s.emit(Event{Kind: EventDisconnect, ClientID: c.id})
			}

		case req := <-s.requests:
			s.handle(st, req)

		case fn := <-s.queries:
			fn(st)

		case <-s.quit:
			for c := range st.clients {
				c.conn.Close()
				close(c.outbox)
			}
			return
		}
	}
}

func (s *Server) handle(st *state, req request) {
	key := req.cmd.key
	switch req.cmd.op {
	case "put":
		st.store[key] = req.cmd.value
		s.emit(Event{Kind: EventPut, ClientID: req.from.id, Key: key, Value: string(req.cmd.value)})

	case "get":
		value := st.store[key]
		message := make([]byte, 0, len(key)+len(value)+2)
		message = append(append(append(append(message, key...), ','), value...), '\n')

		recipients := 0
		for c := range st.clients {
			select {
			case c.outbox <- message:
				recipients++
			default:
				s.emit(Event{Kind: EventDrop, ClientID: c.id, Key: key})
			}
		}
		s.emit(Event{
			Kind: EventGet, ClientID: req.from.id, Key: key,
			Value: string(value), Recipients: recipients,
		})
	}
}

func (s *Server) emit(event Event) {
	if s.OnEvent != nil {
		s.OnEvent(event)
	}
}

func (s *Server) acceptLoop(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return // The listener was closed.
		}
		s.ServeConn(conn)
	}
}

// readLoop parses commands from one client until the connection closes.
func (s *Server) readLoop(c *client) {
	defer func() {
		c.conn.Close()
		select {
		case s.unregister <- c:
		case <-s.quit:
		}
	}()

	reader := bufio.NewReader(c.conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd, ok := parseCommand(line)
		if !ok {
			continue // Ignore lines that do not match the protocol.
		}
		select {
		case s.requests <- request{from: c, cmd: cmd}:
		case <-s.quit:
			return
		}
	}
}

// writeLoop sends queued messages to one client until its outbox closes.
func (c *client) writeLoop() {
	for message := range c.outbox {
		if _, err := c.conn.Write(message); err != nil {
			c.conn.Close()
			for range c.outbox {
				// Drain, so that the state loop never blocks on this client.
			}
			return
		}
	}
}
