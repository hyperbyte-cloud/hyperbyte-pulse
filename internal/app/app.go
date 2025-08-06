package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"hyperbyte-proc-monitor/internal/monitor"
	"hyperbyte-proc-monitor/internal/ui"
)

// App represents the main application
type App struct {
	monitor *monitor.Monitor
	ui      *ui.UI
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewApp creates a new application instance
func NewApp() (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create monitor
	mon := monitor.NewMonitor()

	// Create UI
	userInterface := ui.NewUI(mon)

	return &App{
		monitor: mon,
		ui:      userInterface,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

// Run starts the application
func (a *App) Run() error {
	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start monitoring goroutine
	a.wg.Add(1)
	go a.monitoringLoop()

	// Start cleanup goroutine
	a.wg.Add(1)
	go a.cleanupLoop()

	// Handle graceful shutdown
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		a.Stop()
	}()

	// Run UI (blocks until UI is closed)
	err := a.ui.Run(a.ctx)

	// Stop everything and wait for goroutines to finish
	a.Stop()
	a.wg.Wait()

	return err
}

// Stop stops the application
func (a *App) Stop() {
	a.cancel()
	a.ui.Stop()
}

// monitoringLoop continuously updates system and process metrics
func (a *App) monitoringLoop() {
	defer a.wg.Done()

	// Use different update frequencies for different metrics
	systemTicker := time.NewTicker(1 * time.Second)  // System metrics every second
	processTicker := time.NewTicker(2 * time.Second) // Process metrics every 2 seconds
	defer systemTicker.Stop()
	defer processTicker.Stop()

	// Initial update
	if err := a.monitor.UpdateMetrics(a.ctx); err != nil {
		fmt.Printf("Error updating metrics: %v\n", err)
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-systemTicker.C:
			// Update only system metrics (lightweight)
			if err := a.updateSystemMetricsOnly(); err != nil {
				fmt.Printf("Error updating system metrics: %v\n", err)
			}
		case <-processTicker.C:
			// Full update including processes (heavier)
			if err := a.monitor.UpdateMetrics(a.ctx); err != nil {
				fmt.Printf("Error updating metrics: %v\n", err)
			}
		}
	}
}

// updateSystemMetricsOnly updates just the lightweight system metrics
func (a *App) updateSystemMetricsOnly() error {
	// This could be optimized to only update system-level metrics
	// For now, we'll stick with the full update but less frequently
	return nil
}

// cleanupLoop periodically cleans up old metrics data
func (a *App) cleanupLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.monitor.CleanupOldMetrics()
		}
	}
}
