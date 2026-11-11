// Command kvclient connects to a kvserver. It sends each line that you type
// and prints each line that the server sends.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

func main() {
	addr := flag.String("addr", "localhost:9999", "address of the kvserver")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(os.Stderr, "Connected to %s. Type put,<key>,<value> or get,<key>.\n", *addr)

	go func() {
		if _, err := io.Copy(os.Stdout, conn); err != nil {
			log.Print(err)
		}
		fmt.Fprintln(os.Stderr, "Server closed the connection.")
		os.Exit(0)
	}()

	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		if _, err := fmt.Fprintln(conn, input.Text()); err != nil {
			log.Fatal(err)
		}
	}
}
