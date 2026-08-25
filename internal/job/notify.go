package job

import (
	"context"
	"errors"
)

// Notifier is Planty's direct notification channel. Scheduled jobs depend on
// this interface rather than Home Assistant so alerts can never silently route
// through the house when APNs is unavailable.
type Notifier interface {
	Send(ctx context.Context, title, body string, extra map[string]any) error
}

func notify(ctx context.Context, n Notifier, title, body string, extra map[string]any) error {
	if n == nil {
		return errors.New("push notifications are not configured")
	}
	payload := make(map[string]any, len(extra)+1)
	for key, value := range extra {
		payload[key] = value
	}
	if _, ok := payload["screen"]; !ok {
		payload["screen"] = "today"
	}
	return n.Send(ctx, title, body, payload)
}
