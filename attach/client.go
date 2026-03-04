package attach

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// Connect dials the socket of a running rls process and returns its config
// and streaming channels for events and logs.
func Connect(pid int) (config.Config, <-chan endpoint.Event, <-chan string, error) {
	return ConnectAddr(SocketPath(pid))
}

// ConnectAddr connects to an explicit socket path (useful for testing).
func ConnectAddr(socketPath string) (config.Config, <-chan endpoint.Event, <-chan string, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return config.Config{}, nil, nil, fmt.Errorf("attach: dial %s: %w", socketPath, err)
	}

	scanner := bufio.NewScanner(conn)

	// First message must be config.
	if !scanner.Scan() {
		conn.Close()
		return config.Config{}, nil, nil, fmt.Errorf("attach: no config message from %s", socketPath)
	}
	var msg WireMsg
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		conn.Close()
		return config.Config{}, nil, nil, fmt.Errorf("attach: unmarshal config envelope: %w", err)
	}
	if msg.Type != MsgConfig {
		conn.Close()
		return config.Config{}, nil, nil, fmt.Errorf("attach: expected config message, got %q", msg.Type)
	}
	var cfg config.Config
	if err := json.Unmarshal(msg.Data, &cfg); err != nil {
		conn.Close()
		return config.Config{}, nil, nil, fmt.Errorf("attach: unmarshal config data: %w", err)
	}

	// Patch 0.0.0.0 → 127.0.0.1 so Space inject targets the right address.
	if cfg.Server.Host == "0.0.0.0" {
		cfg.Server.Host = "127.0.0.1"
	}

	events := make(chan endpoint.Event, 256)
	logs := make(chan string, 256)

	go func() {
		defer close(events)
		defer close(logs)
		defer conn.Close()

		for scanner.Scan() {
			var m WireMsg
			if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
				continue
			}
			switch m.Type {
			case MsgEvent:
				var ev endpoint.Event
				if err := json.Unmarshal(m.Data, &ev); err != nil {
					continue
				}
				select {
				case events <- ev:
				default:
				}
			case MsgLog:
				var line string
				if err := json.Unmarshal(m.Data, &line); err != nil {
					continue
				}
				select {
				case logs <- line:
				default:
				}
			}
		}
	}()

	return cfg, events, logs, nil
}
