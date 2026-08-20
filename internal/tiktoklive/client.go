package tiktoklive

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"raikiri/internal/tiktoklive/internal/events"
	"raikiri/internal/tiktoklive/internal/gifts"
	"raikiri/internal/tiktoklive/internal/rawclient"
)

const defaultRetryWait = 15 * time.Second

type rawSource interface {
	Connect(context.Context) (<-chan events.Event, error)
}

type Client struct {
	logger    *slog.Logger
	newSource func(string) rawSource
	retryWait time.Duration
	now       func() time.Time
}

func New(logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("module", "tiktoklive")
	return &Client{
		logger: logger,
		newSource: func(username string) rawSource {
			return rawclient.NewClient(username, logger)
		},
		retryWait: defaultRetryWait,
		now:       time.Now,
	}
}

// Run listens to a public TikTok LIVE stream until ctx is cancelled.
// Offline, blocked, and transport failures are retried internally. emit is
// called synchronously and in source order, providing backpressure instead of
// silently dropping events. Cancellation is a normal shutdown and returns nil.
func (c *Client) Run(ctx context.Context, username string, emit func(Event)) error {
	username = NormalizeUsername(username)
	if username == "" {
		return errors.New("tiktoklive: username is required")
	}
	if emit == nil {
		return errors.New("tiktoklive: emit callback is required")
	}

	for {
		source := c.newSource(username)
		rawEvents, err := source.Connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Warn("connection failed", "username", username, "error", err, "retry", c.retryWait)
			if !waitForRetry(ctx, c.retryWait) {
				return nil
			}
			continue
		}

		streaks := gifts.NewGiftStreakTracker()
		for incoming := range rawEvents {
			if ctx.Err() != nil {
				return nil
			}
			if event, ok := normalizeEvent(incoming, username, streaks, c.now); ok {
				emit(event)
			}
		}
		if !waitForRetry(ctx, c.retryWait) {
			return nil
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		delay = time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
