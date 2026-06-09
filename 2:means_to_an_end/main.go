package main

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const PORT = ":8080"

type userDB map[int32]int32

const (
	TRANS_INSERT = 'I'
	TRANS_QUERY  = 'Q'
)

func main() {
	// Requirements:
	// Message format is a custom binary format that is 9 bytes long.
	// [0 | 1 2 3 4 | 5 6 7 8]
	// [ t   int32     int32 ]
	//
	// For an Insert action:
	// The first 4 bytes after the type holds the timestamp, followed by the last 4 bytes which holds the price.

	var wg sync.WaitGroup

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clientsDB := make(map[net.Conn]userDB)

	textHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	})
	logger := slog.New(textHandler)

	listener, err := net.Listen("tcp", PORT)
	if err != nil {
		logger.Error("TCP listen failure",
			"port", PORT,
			"err", err,
		)
		os.Exit(1)
	}

	// Listen for the shutdown signal.
	// We need to handle shutdowns gracefully.
	go func() {
		<-ctx.Done()
		logger.Info("Shutting down TCP server...")
		listener.Close()
	}()

	logger.Info("Starting TCP server...",
		"port", PORT,
	)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("Couldn't accept incoming connection",
				"err", err,
			)
			break
		}

		_, ok := clientsDB[conn]
		if !ok {
			clientsDB[conn] = userDB{}
		}
		db := clientsDB[conn]

		wg.Go(func() {
			defer conn.Close()
			connHandler(conn, db, logger)
		})
	}

	wg.Wait()
	listener.Close()
}

func connHandler(conn net.Conn, db userDB, logger *slog.Logger) {
	for { // Parse the transaction type that is 1 byte.
		var transType byte
		err := binary.Read(conn, binary.BigEndian, &transType)
		if errors.Is(err, io.EOF) {
			logger.Info("EOF detected. Returning.")
			return
		}

		switch transType {
		case TRANS_INSERT:
			var ts int32
			var price int32

			err := binary.Read(conn, binary.BigEndian, &ts)
			if errors.Is(err, io.EOF) {
				logger.Info("EOF detected. Returning.")
				return
			}

			err = binary.Read(conn, binary.BigEndian, &price)
			if errors.Is(err, io.EOF) {
				logger.Info("EOF detected. Returning.")
				return
			}

			if _, ok := db[ts]; !ok {
				db[ts] = price
			}

		case TRANS_QUERY:
			var mintime int32
			var maxtime int32

			err := binary.Read(conn, binary.BigEndian, &mintime)
			if errors.Is(err, io.EOF) {
				logger.Info("EOF detected. Returning.")
				return
			}

			err = binary.Read(conn, binary.BigEndian, &maxtime)
			if errors.Is(err, io.EOF) {
				logger.Info("EOF detected. Returning.")
				return
			}

			// Compute the mean price between mintime and maxtime
			var sumPrice int64 = 0
			var transCount int64 = 0
			var meanPrice int32 = 0

			for i := mintime; i <= maxtime; i++ {
				if price, exists := db[i]; exists {
					sumPrice += int64(price)
					transCount++
				}
			}

			// Send the mean price back to the client in 4 bytes.
			if transCount != 0 {
				meanPrice = int32(sumPrice / transCount)
			}

			log.Printf("minTime=%d, maxTime=%d, sumPrice=%d, transCount=%d, meanPrice=%d", mintime, maxtime, sumPrice, transCount, meanPrice)

			err = binary.Write(conn, binary.BigEndian, meanPrice)
			if errors.Is(err, io.EOF) {
				logger.Info("EOF detected. Returning.")
				return
			}

		default:
			return
		}
	}
}
