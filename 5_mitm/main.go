package main

import (
	"bufio"
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ADDR = ":8080"
var PROTOHACKERS_SERVER_ADDR = "chat.protohackers.com:16963"
var boguscoinRe = regexp.MustCompile(`^7[a-zA-Z0-9]{25,34}$`)

func main() {
	// REVERSE PROXY for budget chat

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if testAddr := os.Getenv("TEST_ADDR"); testAddr != "" {
		ADDR = testAddr
	}
	if upstreamAddr := os.Getenv("UPSTREAM_TEST_ADDR"); upstreamAddr != "" {
		PROTOHACKERS_SERVER_ADDR = upstreamAddr
	}

	// Init waitgroup for goroutines.
	var wg sync.WaitGroup

	// Init proxy.
	listener, err := net.Listen("tcp", ADDR)
	if err != nil {
		log.Println("TCP listen error:", err)
		return
	}
	defer listener.Close()

	log.Println("Starting proxy server on port", ADDR)

	// Cleanup guys.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		// Wait for new connections
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}

			log.Println("failed TCP accept:", err)
			continue
		}

		wg.Go(func() {
			handleConn(conn)
		})
	}

	wg.Wait()
	log.Println("Stopping proxy server...")
}

func handleConn(clientConn net.Conn) {
	defer clientConn.Close()

	// Forward the connection to the upstream server.
	// Dial the protohackers "Budget Chat" server at
	// "chat.protohackers.com:16963"
	upstreamConn, err := net.Dial("tcp", PROTOHACKERS_SERVER_ADDR)
	if err != nil {
		log.Println("TCP dial to upstream server failed:", err)
		return
	}

	go proxy(clientConn, upstreamConn)
	proxy(upstreamConn, clientConn)
}

func proxy(src, dst net.Conn) {
	defer dst.Close()

	reader := bufio.NewReader(src)

	for {
		message, err := reader.ReadString('\n')
		if len(message) > 0 {
			trimmed := rewriteMessage(message)

			dst.SetWriteDeadline(time.Now().Add(2 * time.Minute))
			if _, err := dst.Write([]byte(trimmed)); err != nil {
				log.Println("dest write error:", err)
				return
			}
		}

		if err != nil {
			log.Println("reader.ReadString error:", err)
			return
		}
	}
}

func rewriteMessage(message string) string {
	hasNewline := strings.HasSuffix(message, "\n")
	trimmed := strings.TrimSuffix(message, "\n")
	words := strings.Split(trimmed, " ")

	for i, word := range words {
		if boguscoinRe.MatchString(word) {
			// If string is a boguscoin, replace it with the correct address.
			words[i] = "7YWHMfk9JZe0LM0g1ZauHuiSxhI"
		}
	}

	trimmed = strings.Join(words, " ")
	if hasNewline {
		trimmed += "\n"
	}

	return trimmed
}
