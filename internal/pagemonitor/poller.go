package pagemonitor

import (
	"context"
	"log"
	"time"
)

type Notifier interface {
	SendPageMonitorNotification(channelID string, m Monitor, oldContent, newContent string) error
}

type Poller struct {
	service  *Service
	notifier Notifier
	interval time.Duration
}

func NewPoller(service *Service, notifier Notifier, interval time.Duration) *Poller {
	return &Poller{service: service, notifier: notifier, interval: interval}
}

func (p *Poller) Start(ctx context.Context) {
	go p.run(ctx)
}

func (p *Poller) run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	monitors, err := p.service.DueMonitors(ctx, time.Now())
	if err != nil {
		log.Printf("pagemonitor: fetch due monitors error: %v", err)
		return
	}
	for _, m := range monitors {
		p.check(ctx, m)
	}
}

func (p *Poller) check(ctx context.Context, m Monitor) {
	result, err := CheckURL(ctx, m.URL, m.Selector)
	if err != nil {
		log.Printf("pagemonitor: check error for %s: %v", m.URL, err)
		// Still update next_check so we don't hammer a broken URL
		_ = p.service.UpdateContent(ctx, m, "error", m.ContentHash, m.LastContent)
		return
	}

	oldHash := m.ContentHash
	oldContent := m.LastContent

	if err := p.service.UpdateContent(ctx, m, "ok", result.Hash, result.Content); err != nil {
		log.Printf("pagemonitor: update content error for monitor %d: %v", m.ID, err)
	}

	// First check — just baseline, no notification
	if oldHash == "" {
		return
	}
	// No change
	if oldHash == result.Hash {
		return
	}

	if err := p.notifier.SendPageMonitorNotification(m.ChannelID, m, oldContent, result.Content); err != nil {
		log.Printf("pagemonitor: notify error for monitor %d: %v", m.ID, err)
	}
}
