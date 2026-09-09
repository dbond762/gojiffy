// Package scheduler — the smallest thing that acts like cron: one job at a
// fixed time of day.
package scheduler

import (
	"context"
	"log"
	"time"
)

// RunDaily wakes job once a day at the next hour:min (today if that time has
// not passed yet, tomorrow otherwise) and repeats forever. An error out of job
// is logged and brings nothing down — the next run comes round in a day
// regardless. It blocks the calling goroutine, so start it as
// go scheduler.RunDaily(...).
func RunDaily(ctx context.Context, hour, min int, job func(context.Context) error) {
	for {
		wait := time.Until(nextRun(hour, min))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		if err := job(ctx); err != nil {
			log.Printf("scheduler: job finished with an error: %v", err)
		}
	}
}

func nextRun(hour, min int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
