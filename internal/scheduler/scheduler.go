package scheduler

import (
	"context"
	"log"
	"time"

	"mizubot-go/internal/reminders"
)

type Sender func(channelID, content string) error

type Scheduler struct {
	store   *reminders.Store
	sender  Sender
	ticker  *time.Ticker
	every   time.Duration
	dueLoad int
}

func New(store *reminders.Store, sender Sender, every time.Duration) *Scheduler {
	if every <= 0 {
		every = 10 * time.Second
	}
	return &Scheduler{store: store, sender: sender, every: every, dueLoad: 50}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.ticker = time.NewTicker(s.every)
	go func() {
		defer s.ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.ticker.C:
				s.runOnce(ctx, time.Now().UTC())
			}
		}
	}()
}

func (s *Scheduler) runOnce(ctx context.Context, now time.Time) {
	// normalize to UTC
	now = now.UTC()
	due, err := s.store.Due(ctx, now, s.dueLoad)
	if err != nil {
		log.Printf("scheduler load error: %v", err)
		return
	}
	for _, r := range due {
		// send message
		if err := s.sender(r.ChannelID, r.Message); err != nil {
			log.Printf("send error for reminder %d: %v", r.ID, err)
		}
		// reschedule or delete
		next, repeat, err := reminders.NextAfter(r, now)
		if err != nil {
			log.Printf("reschedule error for reminder %d: %v", r.ID, err)
			// best-effort delete to prevent tight loop
			_ = s.store.DeleteID(ctx, r.ID)
			continue
		}
		if !repeat {
			_ = s.store.DeleteID(ctx, r.ID)
			continue
		}
		_ = s.store.SetNextRun(ctx, r.ID, next)
	}
}
