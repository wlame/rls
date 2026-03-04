// Package attach provides IPC for mirroring a running rls process.
package attach

import "encoding/json"

// MsgType identifies the kind of wire message.
type MsgType string

const (
	MsgConfig MsgType = "config"
	MsgEvent  MsgType = "event"
	MsgLog    MsgType = "log"
)

// WireMsg is the JSONL envelope sent over the Unix socket.
type WireMsg struct {
	Type MsgType         `json:"type"`
	Data json.RawMessage `json:"data"`
}
