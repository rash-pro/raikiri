package app

import (
	"context"
	"sync"
)

type Adapter interface {
	Start(context.Context)
	Stop()
}

func (a *App) restartAdapters(ctx context.Context) {
	a.stopAdapters()
	runCtx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	cfg := a.config()
	var adapters []Adapter
	if cfg.TwitchChannel != "" {
		adapters = append(adapters, NewTwitchChatAdapter(cfg, a.store, a.logger, a.routeChat))
	}
	if cfg.TwitchChannel != "" && cfg.TwitchClientID != "" {
		adapters = append(adapters, NewTwitchEventSubAdapter(cfg, a.store, a.logger, a.routeEvent))
	}
	if cfg.YouTubeID != "" {
		adapters = append(adapters, NewYouTubeWebAdapter(cfg.YouTubeID, a.logger, a.routeChat, a.routeEvent))
	}
	a.mu.Lock()
	a.adapters = adapters
	a.mu.Unlock()
	for _, adapter := range adapters {
		go adapter.Start(runCtx)
	}
	_ = ctx
}

func (a *App) stopAdapters() {
	a.mu.Lock()
	cancel := a.cancel
	adapters := a.adapters
	a.adapters = nil
	a.cancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	var wg sync.WaitGroup
	for _, adapter := range adapters {
		wg.Add(1)
		go func(adapter Adapter) {
			defer wg.Done()
			adapter.Stop()
		}(adapter)
	}
	wg.Wait()
}
