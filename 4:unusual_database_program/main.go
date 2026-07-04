package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const ADDR = ":8080"

type database struct {
	table map[string]string
	mu    sync.Mutex
}

func newDB() *database {
	return &database{
		table: map[string]string{},
	}
}

// retrieveReq returns the concatenated key-value pair and a boolean that indicates if the
// key exists. If the key doesn't exists or doesn't have an associated value, it returns an
// empty value in the form of [key=]
func (db *database) retrieveReq(key string) string {
	// "version" is a special key used to get the version number of the database.
	if key == "version" {
		kv := key + "=" + "Ken's key-value store V1.0"
		return kv
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	val, ok := db.table[key]
	if !ok {
		emptyVal := key + "="
		return emptyVal
	}

	kv := key + "=" + val
	return kv
}

func (db *database) insertReq(key, val string) {
	// "version" is a special key used to get the version number of the database.
	if key == "version" {
		return
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Update to new value or create one if key isn't found.
	db.table[key] = string(val)
}

func main() {
	// Initialize logger using log/slog.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	}))

	// Init in-memory db.
	db := newDB()

	// Cleanup guys.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Init UDP listener.
	udpAddr, err := net.ResolveUDPAddr("udp", ADDR)
	if err != nil {
		logger.Error("resolve UDP address", "err", err)
		return
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		logger.Error("listen UDP error", "err", err)
		return
	}
	slog.Info("Starting UDP server...", "addr", udpAddr.String())

	go func() {
		<-ctx.Done()
		udpConn.Close()
		logger.Info("Shutting down server...")
	}()

	for {
		// Initialize a buffer with a size of 990 Bytes. As per spec, requests and responses
		// must be shorter than 1000 Bytes.
		//
		// Try 1 KB first.
		buf := make([]byte, 1000)

		n, addrPort, err := udpConn.ReadFromUDPAddrPort(buf)
		if err != nil {
			logger.Error("read from UDP", "err", err)
			return
		}

		request := buf[:n]
		logger.Info("message received", "from", addrPort.String(), slog.String("msg", string(request)))

		worker(udpConn, addrPort, request, db, logger)
	}
}

func worker(udpConn *net.UDPConn, addrPort netip.AddrPort, request []byte, db *database, logger *slog.Logger) {
	before, after, found := bytes.Cut(request, []byte("="))
	if !found {
		// pos is -1 when "=" is not found. If not found, it's a "retrieve" request.
		kv := db.retrieveReq(string(request))
		_, err := udpConn.WriteToUDPAddrPort([]byte(kv), addrPort)
		if err != nil {
			logger.Error("write to UDP after retrieve()", "err", err)
			return
		}
	} else {
		// else, an "insert" request.
		db.insertReq(string(before), string(after))
	}
}
