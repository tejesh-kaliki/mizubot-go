package animefeed

import (
	"context"
	"log"
	"time"
)

type Poller struct {
	service  *Service
	notifier Notifier
	every    time.Duration
}

func NewPoller(service *Service, notifier Notifier, every time.Duration) *Poller {
	if every <= 0 {
		every = time.Minute
	}
	return &Poller{service: service, notifier: notifier, every: every}
}

func (p *Poller) Start(ctx context.Context) {
	ticker := time.NewTicker(p.every)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := p.service.Sync(ctx, p.notifier); err != nil {
					log.Printf("anime poller error: %v", err)
				}
			}
		}
	}()
}
