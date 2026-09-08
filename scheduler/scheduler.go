// Package scheduler — минимальный аналог cron: одна задача на фиксированное
// время суток.
package scheduler

import (
	"context"
	"log"
	"time"
)

// RunDaily будит job раз в сутки в ближайшие hour:min (сегодня, если время ещё
// не прошло, иначе завтра) и повторяет вечно. Ошибку job логирует и не падает —
// следующий запуск всё равно наступит через сутки. Блокирует вызывающую
// горутину, поэтому запускать через go scheduler.RunDaily(...).
func RunDaily(ctx context.Context, hour, min int, job func(context.Context) error) {
	for {
		wait := time.Until(nextRun(hour, min))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		if err := job(ctx); err != nil {
			log.Printf("scheduler: задача завершилась с ошибкой: %v", err)
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
