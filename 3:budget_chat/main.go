package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const PORT = ":8080"

type room struct {
	users map[net.Conn]*client
	rwmu  sync.RWMutex
}

type client struct {
	room     *room
	conn     net.Conn
	username string
	joined   bool
	send     chan string
}

func (r *room) add(cl *client, username string) error {
	r.rwmu.Lock()
	defer r.rwmu.Unlock()

	for _, u := range r.users {
		if u.username == username {
			return errors.New("username already taken")
		}
	}

	cl.username = username
	r.users[cl.conn] = cl

	return nil
}

func (r *room) join(cl *client, logger *slog.Logger) error {
	r.rwmu.Lock()
	if u, ok := r.users[cl.conn]; ok {
		u.joined = true
	}

	// Gather users currently in the room excluding the current user.
	targets := make([]*client, 0, len(r.users))
	usernames := make([]string, 0, len(r.users))

	for c, u := range r.users {
		if c != cl.conn && u.username != cl.username && u.joined {
			targets = append(targets, u)
			usernames = append(usernames, u.username)
		}
	}
	r.rwmu.Unlock()

	// Announce to the server that user has entered the chat, except the current user.
	for _, c := range targets {
		select {
		case c.send <- fmt.Sprintf("* %s has entered the room\n", cl.username):
		default:
			c.conn.Close()
		}
	}

	// Announce to the current user who're currently in the room.
	select {
	case cl.send <- fmt.Sprintf("* The room contains: %s\n", strings.Join(usernames, ", ")):
	default:
		cl.conn.Close()
	}

	logger.Info("connected users", "users", strings.Join(usernames, ", "))

	return nil
}

func (r *room) leave(conn net.Conn) error {
	defer conn.Close()

	r.rwmu.Lock()
	cl, ok := r.users[conn]
	if !ok {
		r.rwmu.Unlock()
		return errors.New("user connection not found in room map")
	}

	delete(r.users, conn)

	// When there is a network error before the user joins, unlock the mutex and return.
	if !cl.joined {
		r.rwmu.Unlock()
		return nil
	}

	targets := make([]*client, 0, len(r.users))
	for c, u := range r.users {
		if c != conn && u.joined {
			targets = append(targets, u)
		}
	}
	r.rwmu.Unlock()

	for _, c := range targets {
		select {
		case c.send <- fmt.Sprintf("* %s has left the room\n", cl.username):
		default:
			c.conn.Close()
		}
	}

	return nil
}

func main() {
	// recap:
	//
	// 1. separate goroutines for read and write
	// 2. read and write deadlines to prevent blocking and hangs. Use channels alongside it for concurrent writes
	// 3. use mutex sparringly
	// 4. graceful shutdown with proper signal and context cancellation
	// 5. proper error handling and logging

	var wg sync.WaitGroup

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", PORT)
	if err != nil {
		logger.Error("Couldn't listen to TCP port", "port", PORT)
		os.Exit(1)
	}

	logger.Info("Starting TCP chat server...", "port", PORT)

	userMap := make(map[net.Conn]*client)
	r := room{
		users: userMap,
	}

	go func() {
		<-ctx.Done()
		listener.Close()
		r.rwmu.Lock()
		for c := range r.users {
			c.Close()
		}
		r.rwmu.Unlock()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			logger.Error("Coudln't accept connection", "err", err)
			continue
		}

		c := &client{
			room:     &r,
			conn:     conn,
			username: "",
			joined:   false,
			send:     make(chan string, 50),
		}

		wg.Go(func() {
			chatHandler(&r, c, logger)
		})
	}

	wg.Wait()
	logger.Info("Shutting down server...")
}

func chatHandler(rm *room, cl *client, logger *slog.Logger) {
	defer rm.leave(cl.conn)

	scanner := bufio.NewScanner(cl.conn)

	// Remember to set a read deadline before every input read and a write deadline before
	// every network write.
	cl.conn.SetWriteDeadline(time.Now().Add(2 * time.Minute))
	cl.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

	// Send confirmation before accepting client into current list of users.
	introMsg := "Welcome to budgetchat! What shall I call you?"
	_, err := fmt.Fprintf(cl.conn, "%s\n", introMsg)
	if err != nil {
		logger.Error("write error", "err", err)
		return
	}

	// The first message from the client sets the user's name.
	cl.username, err = checkUsername(cl.conn, scanner)
	if err != nil || cl.username == "" {
		logger.Error("checkUsername error", "err", err)
		return
	}

	if err := rm.add(cl, cl.username); err != nil {
		_, err = fmt.Fprintf(cl.conn, "username already taken\n")
		logger.Error("add user to room", "err", err)
		return
	}
	logger.Info("user connected", "user", cl.username)

	// User is connected at this point.
	err = rm.join(cl, logger)
	if err != nil {
		logger.Error("join error", "err", err)
		return
	}

	// Avoid blocking the main operation and launch a separate goroutine that writes to
	// connected users.
	//
	// Read and Write operations must be concurrent, especially when dealing with real-time servers.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go writeLoop(cl, ctx)

	// From here, all subsequent messages from the client are chat messages.
	cl.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

	var message string
	for scanner.Scan() {
		cl.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

		message = scanner.Text()
		if message == "" {
			continue
			// if len(message) < 1000 {
			// 	//
			// }
		}

		// Broadcast message to all connected clients, excluding the current user.
		// NOTE:
		// - Avoid doing network operations inside a mutex because a network call is much slower than a local call. Do the network operation after releasing the mutex

		rm.rwmu.RLock()
		targets := make([]*client, 0, len(rm.users))
		for c, u := range rm.users {
			if c != cl.conn && u.joined {
				targets = append(targets, u)
			}
		}
		rm.rwmu.RUnlock()

		for _, c := range targets {
			select {
			case c.send <- fmt.Sprintf("[%s] %s\n", cl.username, message):
			default:
				c.conn.Close()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Error("scan error", "err", err)
	}
}

func checkUsername(conn net.Conn, scanner *bufio.Scanner) (string, error) {
	if !scanner.Scan() {
		return "", fmt.Errorf("read error: %w", scanner.Err())
	}

	username := scanner.Text()
	if len(username) < 1 {
		_, err := fmt.Fprintf(conn, "Username must be at least 1 character long.\n")
		if err != nil {
			return "", fmt.Errorf("write error: %w", err)
		}
		return "", errors.New("Username must be at least 1 character long")
	}

	for _, char := range username {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') {
			_, err := fmt.Fprintf(conn, "Username is not alphanumeric.\n")
			if err != nil {
				return "", fmt.Errorf("write error: %w", err)
			}
			return "", errors.New("invalid username: username is not alphanumeric")
		}
	}

	return username, nil
}

func writeLoop(cl *client, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-cl.send:
			cl.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprint(cl.conn, msg); err != nil {
				cl.conn.Close()
				return
			}
		}
	}
}
