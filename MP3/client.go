package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const defaultServerAddr string = "172.22.156.112:4999" // VM 1

var interrupted bool = false

func readFeedbackService(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for !interrupted {
		feedback, readErr := reader.ReadString('\n')
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Client: Error reading feedback from server\n")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, feedback)
	}
}

func main() {
	// signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		interrupted = true
	}()

	reader := bufio.NewReader(os.Stdin)

	var serverAddr string = defaultServerAddr
	args := os.Args
	if len(args) != 1 {
		serverAddr = args[1]
	}
	conn, dialErr := net.Dial("tcp", serverAddr)
	if dialErr != nil {
		fmt.Fprintf(os.Stderr, "Client: Error connecting to server\n")
		os.Exit(1)
	}

	go readFeedbackService(conn)

	for !interrupted {
		cmd, readErr := reader.ReadString('\n')
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Client: Error reading input\n")
			os.Exit(1)
		}
		_, writeErr := fmt.Fprintf(conn, "%s\n", cmd)
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "Client: Error writing to server\n")
			os.Exit(1)
		}
	}
}
