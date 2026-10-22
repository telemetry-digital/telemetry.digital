// Package service wraps the application as a Windows service or systemd unit (kardianos/service, zlib license).
package service

import (
	"context"
	"log/slog"
	"os"

	"github.com/kardianos/service"
)

// RunFunc runs the application until ctx is cancelled.
type RunFunc func(ctx context.Context) error

type program struct {
	run    RunFunc
	log    *slog.Logger
	cancel context.CancelFunc
	done   chan error
}

// exit ends the process when the application returns on its own; a variable for tests.
var exit = os.Exit

func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan error, 1)
	go func() {
		err := p.run(ctx)
		if ctx.Err() == nil {
			// Returned without Stop (e.g. the database was not up yet after a reboot): the service manager only
			// restarts a process that ends, so end it instead of waiting for a stop signal that never comes.
			p.log.Error("server stopped; exiting so the service manager starts it again", "err", err)
			exit(1)
		}
		p.done <- err
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		if err := <-p.done; err != nil && err != context.Canceled {
			p.log.Error("service stopped with error", "err", err)
		}
	}
	return nil
}

// Config returns the service definition. Arguments are passed to the binary on service start.
func Config(configPath string) *service.Config {
	return &service.Config{
		Name:        "ctrl32-telemetry",
		DisplayName: "ctrl32 telemetry",
		Description: "ctrl32 telemetry — data collection platform (MQTT, HTTP, LoRaWAN, OPC UA, Modbus)",
		Arguments:   []string{"serve", "-config", configPath},
		Option: service.KeyValue{
			"Restart":   "always",
			"LogOutput": true,
			"OnFailure": "restart",
		},
	}
}

// AgentConfig returns the definition of the host agent service (Windows: runs as LocalSystem like the app;
// Linux installs use the systemd unit from install.sh instead).
func AgentConfig(configPath string) *service.Config {
	return &service.Config{
		Name:        "ctrl32-telemetry-agent",
		DisplayName: "ctrl32 telemetry agent",
		Description: "ctrl32 telemetry — host agent (service restarts, backups, updates)",
		Arguments:   []string{"agent", "-config", configPath},
		Option: service.KeyValue{
			"Restart":   "always",
			"LogOutput": true,
			"OnFailure": "restart",
		},
	}
}

// RelayConfig returns the definition of the camera relay service at a remote site (spec 018): it runs
// `camera-relay -config path` and is restarted by the service manager when it stops.
func RelayConfig(configPath string) *service.Config {
	return &service.Config{
		Name:        "ctrl32-camera-relay",
		DisplayName: "ctrl32 camera relay",
		Description: "ctrl32 telemetry — forwards local cameras to the server over an encrypted connection",
		Arguments:   []string{"camera-relay", "-config", configPath},
		Option: service.KeyValue{
			"Restart":   "always",
			"LogOutput": true,
			"OnFailure": "restart",
		},
	}
}

// New builds the service handle.
func New(cfg *service.Config, run RunFunc, log *slog.Logger) (service.Service, error) {
	return service.New(&program{run: run, log: log}, cfg)
}

// Control executes install/uninstall/start/stop/restart.
func Control(s service.Service, action string) error {
	return service.Control(s, action)
}

// Interactive reports whether the process runs in a terminal rather than under the service manager.
func Interactive() bool { return service.Interactive() }
