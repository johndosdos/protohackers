package main

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const ADDR = ":8080"

type database struct {
	table map[string]string
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

	_, ok := db.table[key]
	if !ok {
		// Init new kv pair
		db.table[key] = string(val)
		return
	}

	log.Println(db.table)

	// Update to new value
	db.table[key] = string(val)
}

func main() {
	// Initialize logger using log/slog.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	}))

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

	// Cleanup guys.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		udpConn.Close()
	}()

	slog.Info("Starting UDP server...", "addr", udpAddr.String())

	// Init in-memory db.
	db := &database{
		table: map[string]string{},
	}

	// Initialize a buffer with a size of 990 Bytes. As per spec, requests and responses
	// must be shorter than 1000 Bytes.

	// Try 1 KB first.
	buf := make([]byte, 1000)

	for {
		n, addrPort, err := udpConn.ReadFromUDPAddrPort(buf)
		if err != nil {
			logger.Error("read from UDP", "err", err)
			return
		}

		request := bytes.TrimSpace(buf[:n])
		logger.Info("message received", "from", addrPort.String(), slog.String("msg", string(request)))

		pos := bytes.Index(request, []byte("="))
		log.Println(pos)
		if pos == -1 {
			// pos is -1 when "=" is not found. If not found, it's a "retrieve" request.
			kv := db.retrieveReq(string(request))
			_, err := udpConn.WriteToUDPAddrPort([]byte(kv+"\n"), addrPort)
			if err != nil {
				logger.Error("write to UDP after retrieve()", "err", err)
				continue
			}
		} else {
			// else, an "insert" request.
			db.insertReq(string(request[:pos]), string(request[pos+1:]))
		}
	}
}
