package endpoint

import (
	"time"

	"github.com/wlame/rls/config"
)

// Response is the JSON payload returned by every rate-limited endpoint.
// Contains the full resolved configuration, including inherited values.
type Response struct {
	OK            bool    `json:"ok"`
	Endpoint      string  `json:"endpoint"`
	QueuedForMs   int64   `json:"queued_for_ms"`
	QueueDepth    int     `json:"queue_depth"`
	Rate          float64 `json:"rate"`
	Unit          string  `json:"unit"`
	Scheduler     string  `json:"scheduler"`
	Algorithm     string  `json:"algorithm"`
	MaxQueueSize  int     `json:"max_queue_size"`
	Overflow      string  `json:"overflow"`
	BurstSize     int     `json:"burst_size,omitempty"`
	WindowSeconds int     `json:"window_seconds,omitempty"`
	QueueTimeout        float64 `json:"queue_timeout,omitempty"`
	LatencyCompensation float64 `json:"latency_compensation,omitempty"`
	NetworkLatencyMs    *int64  `json:"network_latency_ms,omitempty"`
	Dynamic             bool    `json:"dynamic,omitempty"`
	TokensConsumed      *int   `json:"tokens_consumed,omitempty"`
	TokensRemaining     *int   `json:"tokens_remaining,omitempty"`
	WindowCapacity      *int   `json:"window_capacity,omitempty"`
	WaitingForNextWindow *int  `json:"waiting_for_next_window,omitempty"`
}

// tokenWindowInfo holds optional token window fields for the response.
type tokenWindowInfo struct {
	Consumed  int
	Remaining int
	Capacity  int
	Waiting   int
}

// buildResponse constructs a Response from the endpoint config, current queue depth,
// and the time the ticket was enqueued. All config fields are included — for dynamic
// endpoints this reflects the fully resolved inherited values.
func buildResponse(cfg config.EndpointConfig, queueDepth int, enqueuedAt time.Time, networkLatencyMs *int64, twi *tokenWindowInfo) Response {
	resp := Response{
		OK:                  true,
		Endpoint:            cfg.Path,
		QueuedForMs:         time.Since(enqueuedAt).Milliseconds(),
		QueueDepth:          queueDepth,
		Rate:                cfg.Rate,
		Unit:                cfg.Unit,
		Scheduler:           cfg.Scheduler,
		Algorithm:           cfg.Algorithm,
		MaxQueueSize:        cfg.MaxQueueSize,
		Overflow:            cfg.Overflow,
		BurstSize:           cfg.BurstSize,
		WindowSeconds:       cfg.WindowSeconds,
		QueueTimeout:        cfg.QueueTimeout,
		LatencyCompensation: cfg.LatencyCompensation,
		NetworkLatencyMs:    networkLatencyMs,
		Dynamic:             cfg.Dynamic,
	}
	if twi != nil {
		resp.TokensConsumed = &twi.Consumed
		resp.TokensRemaining = &twi.Remaining
		resp.WindowCapacity = &twi.Capacity
		resp.WaitingForNextWindow = &twi.Waiting
	}
	return resp
}
