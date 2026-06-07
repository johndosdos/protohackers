package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net"
	"os"
)

type PrimeRequest struct {
	Method string `json:"method"`
	Number any    `json:"number"`
}

type PrimeResponse struct {
	Method string `json:"method"`
	Prime  bool   `json:"prime"`
}

const PORT = ":8080"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	}))

	// 1. connection is TCP
	// 2. return response object as "true" if request object is a prime number

	listener, err := net.Listen("tcp", PORT)
	if err != nil {
		logger.Error("couldn't listen to TCP port",
			"port", PORT,
			"err", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Info("starting TCP server", "port", PORT)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("failed to accept TCP connection",
				"err", err)
			os.Exit(1)
		}

		go TCPHandler(conn, logger)
	}
}

func TCPHandler(conn net.Conn, logger *slog.Logger) {
	defer conn.Close()

	// var req PrimeRequest
	reader := bufio.NewReader(conn)

	for {
		// Read until \n since this is NDJSON.
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			logger.Info("EOF detected. Parsing next stream.",
				"err", err,
			)
			return
		}

		// Unmarshal into a map so that we can manually check the JSON
		// fields.
		var req map[string]any
		err = json.Unmarshal(line, &req)
		if err != nil {
			logger.Error("Unmarshal error. Parsing next request.",
				"err", err,
			)
			err := encode(conn, writeMalformed())
			if err != nil {
				logger.Error("Encode error to stream",
					"err", err,
				)
			}
			return
		}

		reqMethod := req["method"]
		reqNumber := req["number"]

		if reqMethod != "isPrime" {
			logger.Info("Invalid request method. Returning...",
				"want", "isPrime",
				"got", reqMethod,
			)

			err := encode(conn, writeMalformed())
			if err != nil {
				logger.Error("Encode error to stream", "err", err)
			}

			return
		}

		n, ok := reqNumber.(float64)
		if !ok {
			logger.Info("Invalid request number. Returning...",
				"want", "float64",
				"got", fmt.Sprintf("%T", n),
			)

			err := encode(conn, writeMalformed())
			if err != nil {
				logger.Error("Encode error to stream", "err", err)
			}

			return
		}

		ok = isNumberPrime(n)
		logger.Debug("isNumberPrime value", "val", ok)
		if !ok {
			err := encode(conn, PrimeResponse{
				Method: "isPrime",
				Prime:  ok,
			})
			if err != nil {
				logger.Error("Encoding error", "err", err)
				return
			}

			continue
		}

		err = encode(conn, PrimeResponse{
			Method: "isPrime",
			Prime:  ok,
		})
		if err != nil {
			logger.Error("Encoding error", "err", err)
			return
		}
	}
}

func isNumberPrime(num float64) bool {
	// check if a number is prime or not
	// NOTE: only integers can be prime

	whole, frac := math.Modf(num)
	// Prime numbers start at 2.
	// Floating point values are not considered primes.
	if frac != 0.0 || whole < 2 {
		return false
	}

	// Do a primality test.
	// Cast to int64 to handle larger prime numbers.
	w := int64(whole)
	for i := int64(2); i*i <= w; i++ {
		// If w % i != 0, 'w' still has a smaller factor (??)
		if w%i == 0 {
			return false
		}
	}
	return w >= 2
}

func encode(conn net.Conn, payload any) error {
	var data []byte
	var err error

	switch t := payload.(type) {
	case []byte:
		data = t
	default:
		// If the payload is of PrimeResponse type, Marshal it into a struct.
		data, err = json.Marshal(t)
		if err != nil {
			return err
		}
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return err
	}

	return nil
}

func writeMalformed() []byte {
	return []byte("{bad response\n")
}
