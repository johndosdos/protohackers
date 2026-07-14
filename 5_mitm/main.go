package main

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
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

func main() {
	// REVERSE PROXY for budget chat

	if testAddr := os.Getenv("TEST_ADDR"); testAddr != "" {
		ADDR = testAddr
	}
	if upstreamAddr := os.Getenv("UPSTREAM_TEST_ADDR"); upstreamAddr != "" {
		PROTOHACKERS_SERVER_ADDR = upstreamAddr
	}

	// Init waitgroup for goroutines.
	var wg sync.WaitGroup

	// Init logger.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	}))

	// Init proxy.
	listener, err := net.Listen("tcp", ADDR)
	if err != nil {
		logger.Error("TCP listen error", "err", err)
		return
	}
	defer listener.Close()

	logger.Info("Starting proxy server...", "port", ADDR)

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

			logger.Error("failed TCP accept", "err", err)
			continue
		}

		wg.Go(func() {
			handleConn(conn, logger)
		})
	}

	wg.Wait()
	logger.Info("Stopping proxy server...")
}

func handleConn(clientConn net.Conn, logger *slog.Logger) {
	defer clientConn.Close()

	// Forward the connection to the upstream server.
	// Dial the protohackers "Budget Chat" server at
	// "chat.protohackers.com:16963"
	upstreamConn, err := net.Dial("tcp", PROTOHACKERS_SERVER_ADDR)
	if err != nil {
		logger.Error("TCP dial to upstream server failed", "err", err)
		return
	}

	go proxy(clientConn, upstreamConn)
	proxy(upstreamConn, clientConn)
}

func proxy(src, dst net.Conn) {
	defer dst.Close()

	reader := bufio.NewReader(src)
	r, err := regexp.Compile(`^7[a-zA-Z0-9]{25,34}$`)
	if err != nil {
		return
	}

	for {
		message, err := reader.ReadString('\n')
		if len(message) > 0 {
			hasNewline := strings.HasSuffix(message, "\n")
			trimmed := strings.TrimSuffix(message, "\n")
			words := strings.Split(trimmed, " ")

			for i, word := range words {
				if isBoguscoin(r, word) {
					// If string is a boguscoin, replace it with the correct address.
					words[i] = "7YWHMfk9JZe0LM0g1ZauHuiSxhI"
				}
			}

			trimmed = strings.Join(words, " ")
			if hasNewline {
				trimmed += "\n"
			}

			dst.SetWriteDeadline(time.Now().Add(2 * time.Minute))
			if _, err := dst.Write([]byte(trimmed)); err != nil {
				return
			}
		}

		if err != nil {
			return
		}
	}
}

func isBoguscoin(r *regexp.Regexp, str string) bool {
	return r.MatchString(str)
}
