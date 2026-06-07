package main

import (
	"io"
	"log"
	"net"
)

const PORT = ":8080"

func main() {
	// requirements:
	// 1. echo service; resend received data
	// 2. can handle at least 5 simultaneous connections
	// 3. close socket after receiving EOF
	//
	// RFC 862

	log.SetFlags(log.LstdFlags)

	log.Printf("TCP server listening at port %s\n", PORT)
	conn, err := net.Listen("tcp", PORT)
	if err != nil {
		log.Fatalf("ERROR: couldn't establish connection: %v", err)
	}
	defer conn.Close()

	for {
		conn, err := conn.Accept()
		if err != nil {
			// continue to accept connections after a failed attempt
			continue
		}

		go func(c net.Conn) {
			defer c.Close()

			_, err := io.Copy(c, c)
			if err == io.EOF {
				log.Printf("INFO: received EOF. Closing connection...")
			} else if err != nil {
				log.Fatalf("ERROR: copy error: %v", err)
			}
		}(conn)
	}
}
