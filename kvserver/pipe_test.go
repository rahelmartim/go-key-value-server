package kvserver

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"
)

func connect(t *testing.T, s *Server) (net.Conn, *bufio.Reader) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	s.ServeConn(serverSide)
	return clientSide, bufio.NewReader(clientSide)
}

func send(t *testing.T, conn net.Conn, line string) {
	t.Helper()
	if _, err := io.WriteString(conn, line); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}

func readLine(t *testing.T, conn net.Conn, reader *bufio.Reader) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return line
}

func TestGetIsSentToEveryClient(t *testing.T) {
	s := New()
	defer s.Close()
	alice, aliceReader := connect(t, s)
	bob, bobReader := connect(t, s)

	send(t, alice, "put,color,blue\n")
	send(t, alice, "get,color\n")

	for name, pair := range map[string]struct {
		conn   net.Conn
		reader *bufio.Reader
	}{"alice": {alice, aliceReader}, "bob": {bob, bobReader}} {
		if got := readLine(t, pair.conn, pair.reader); got != "color,blue\n" {
			t.Errorf("%s received %q, want %q", name, got, "color,blue\n")
		}
	}
}

func TestInvalidLineDoesNotStopTheServer(t *testing.T) {
	s := New()
	defer s.Close()
	conn, reader := connect(t, s)

	send(t, conn, "get\n")
	send(t, conn, "hello\n")
	send(t, conn, "put,k,v\n")
	send(t, conn, "get,k\n")

	if got := readLine(t, conn, reader); got != "k,v\n" {
		t.Errorf("received %q, want %q", got, "k,v\n")
	}
}

func TestSnapshotAndEvents(t *testing.T) {
	s := New()
	events := make(chan Event, 10)
	s.OnEvent = func(e Event) { events <- e }
	defer s.Close()
	conn, _ := connect(t, s)

	send(t, conn, "put,a,1\n")
	want := []EventKind{EventConnect, EventPut}
	for _, kind := range want {
		select {
		case e := <-events:
			if e.Kind != kind {
				t.Fatalf("event %q, want %q", e.Kind, kind)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no %q event", kind)
		}
	}
	if got := s.Snapshot(); got["a"] != "1" || len(got) != 1 {
		t.Errorf("Snapshot() = %v, want map[a:1]", got)
	}
}

func TestCloseDisconnectsClients(t *testing.T) {
	s := New()
	conn, reader := connect(t, s)
	if got := s.Count(); got != 1 {
		t.Fatalf("Count() = %d, want 1", got)
	}

	s.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := reader.ReadString('\n'); err != io.EOF && err != io.ErrClosedPipe {
		t.Errorf("read after Close: %v, want EOF", err)
	}
	if err := s.Start(0); err != ErrClosed {
		t.Errorf("Start after Close: %v, want ErrClosed", err)
	}
}
