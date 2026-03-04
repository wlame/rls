// Package attach provides IPC for mirroring a running rls process.
package attach

import "encoding/json"

// MsgType identifies the kind of wire message.
type MsgType string

const (
	MsgConfig MsgType = "config"
	MsgState  MsgType = "state"
	MsgEvent  MsgType = "event"
	MsgLog    MsgType = "log"
)

// EndpointSnapshot carries the queue depth for a single endpoint.
type EndpointSnapshot struct {
	Path       string `json:"path"`
	QueueDepth int    `json:"queue_depth"`
}

// WireMsg is the JSONL envelope sent over the Unix socket.
type WireMsg struct {
	Type MsgType         `json:"type"`
	Data json.RawMessage `json:"data"`
}
