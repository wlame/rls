package attach

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
)

// SocketPath returns the conventional socket path for a given PID.
func SocketPath(pid int) string {
	return fmt.Sprintf("/tmp/rls-%d.sock", pid)
}

// Serve listens on a Unix domain socket and streams JSONL to each client.
// It removes any stale socket file before binding, and cleans up on return.
func Serve(ctx context.Context, hub *Hub, socketPath string) error {
	// Remove stale socket.
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("attach: listen %s: %w", socketPath, err)
	}
	defer func() {
		ln.Close()
		os.Remove(socketPath)
	}()

	// Close listener when context is done.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	var wg sync.WaitGroup
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Expected when listener is closed.
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			default:
				wg.Wait()
				return fmt.Errorf("attach: accept: %w", err)
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			serveConn(ctx, hub, conn)
		}()
	}
}

func serveConn(ctx context.Context, hub *Hub, conn net.Conn) {
	defer conn.Close()

	ch, unsub := hub.Subscribe()
	defer unsub()

	enc := json.NewEncoder(conn)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if err := enc.Encode(msg); err != nil {
				return
			}
		}
	}
}
