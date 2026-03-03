package endpoint

import (
	"time"

	"github.com/wlame/rls/config"
)

// Response is the JSON payload returned by every rate-limited endpoint.
type Response struct {
	OK          bool    `json:"ok"`
	Endpoint    string  `json:"endpoint"`
	QueuedForMs int64   `json:"queued_for_ms"`
	QueueDepth  int     `json:"queue_depth"`
	Rate        float64 `json:"rate"`
	Unit        string  `json:"unit"`
	Scheduler   string  `json:"scheduler"`
	Algorithm   string  `json:"algorithm"`
}

// buildResponse constructs a Response from the endpoint config, current queue depth,
// and the time the ticket was enqueued.
func buildResponse(cfg config.EndpointConfig, queueDepth int, enqueuedAt time.Time) Response {
	return Response{
		OK:          true,
		Endpoint:    cfg.Path,
		QueuedForMs: time.Since(enqueuedAt).Milliseconds(),
		QueueDepth:  queueDepth,
		Rate:        cfg.Rate,
		Unit:        cfg.Unit,
		Scheduler:   cfg.Scheduler,
		Algorithm:   cfg.Algorithm,
	}
}
