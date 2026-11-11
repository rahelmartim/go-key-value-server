// Command kvserver runs the key-value server on a TCP address.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/rahelmartim/go-key-value-server/kvserver"
)

func main() {
	addr := flag.String("addr", "localhost:9999", "TCP address to listen on")
	verbose := flag.Bool("v", false, "log every server event")
	flag.Parse()

	server := kvserver.New()
	if *verbose {
		server.OnEvent = logEvent
	}
	if err := server.Listen(*addr); err != nil {
		log.Fatal(err)
	}
	log.Printf("kvserver listening on %s", *addr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Print("shutting down")
	server.Close()
}

func logEvent(e kvserver.Event) {
	switch e.Kind {
	case kvserver.EventPut:
		log.Printf("client %d: put %q = %q", e.ClientID, e.Key, e.Value)
	case kvserver.EventGet:
		log.Printf("client %d: get %q = %q, sent to %d clients", e.ClientID, e.Key, e.Value, e.Recipients)
	case kvserver.EventDrop:
		log.Printf("client %d: dropped a message, because its queue is full", e.ClientID)
	default:
		log.Printf("client %d: %s", e.ClientID, e.Kind)
	}
}
