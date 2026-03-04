package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/wlame/rls/attach"
	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
	"github.com/wlame/rls/server"
	"github.com/wlame/rls/tui"
)

func main() {
	cfgPath := flag.String("config", "rls.yml", "path to config file")
	port := flag.Int("port", 0, "override server port")
	host := flag.String("host", "", "override server host")
	interactive := flag.Bool("interactive", false, "start interactive terminal UI")
	tuiWarn := flag.Duration("tui-warn", 2*time.Second, "dot colour: green below this threshold")
	tuiCrit := flag.Duration("tui-crit", 5*time.Second, "dot colour: yellow below this threshold, red at or above")
	attachPID := flag.Int("attach", 0, "PID of rls process to attach to")
	flag.Parse()

	thresholds := tui.DotThresholds{Warn: *tuiWarn, Crit: *tuiCrit}

	// --- Attach mode: connect to a running rls process ---
	if *attachPID != 0 {
		remoteCfg, stateCh, remoteEvents, remoteLogs, err := attach.Connect(*attachPID)
		if err != nil {
			log.Fatalf("%s  attach: %v", now(), err)
		}

		snapshots := <-stateCh

		if *interactive {
			if err := tui.Run(&remoteCfg, remoteEvents, thresholds, remoteLogs, *attachPID, snapshots); err != nil {
				log.Fatalf("%s  tui error: %v", now(), err)
			}
		} else {
			// Drain events in background.
			go func() { for range remoteEvents {} }()
			for line := range remoteLogs {
				fmt.Printf("[%d] %s\n", *attachPID, line)
			}
		}
		return
	}

	// --- Normal mode: load config and start server ---
	overrides := make(map[string]string)
	if *port != 0 {
		overrides["port"] = fmt.Sprintf("%d", *port)
	}
	if *host != "" {
		overrides["host"] = *host
	}

	cfg, err := loadConfig(*cfgPath, overrides)
	if err != nil {
		log.Printf("%s  warning: %v — using built-in defaults", now(), err)
		cfg = defaultConfig()
		if mergeErr := config.MergeOverrides(cfg, overrides); mergeErr != nil {
			log.Fatalf("%s  invalid flags: %v", now(), mergeErr)
		}
	}

	if *interactive {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		logWriter, rawLogCh := tui.LogSink(256)
		log.SetOutput(logWriter)

		rawEvents := make(chan endpoint.Event, 256)
		srv, err := server.New(*cfg, endpoint.WithEventSink(rawEvents))
		if err != nil {
			log.Fatalf("%s  create server: %v", now(), err)
		}
		go func() {
			if err := srv.Start(); err != nil {
				log.Printf("%s  server stopped: %v", now(), err)
			}
		}()

		// Fanout: TUI + hub each get their own channels.
		tuiEvents, hubEvents := attach.Events2(rawEvents)
		tuiLogs, hubLogs := attach.Logs2(rawLogCh)

		hub := attach.NewHub(*cfg, func() []attach.EndpointSnapshot {
			depths := srv.Registry().QueueDepths()
			snaps := make([]attach.EndpointSnapshot, 0, len(depths))
			for path, depth := range depths {
				snaps = append(snaps, attach.EndpointSnapshot{Path: path, QueueDepth: depth})
			}
			return snaps
		})
		go hub.Run(ctx, hubEvents, hubLogs)
		go attach.Serve(ctx, hub, attach.SocketPath(os.Getpid()))

		if err := tui.Run(cfg, tuiEvents, thresholds, tuiLogs, 0, nil); err != nil {
			log.Fatalf("%s  tui error: %v", now(), err)
		}
		cancel()
		srv.Shutdown() //nolint:errcheck
		return
	}

	// Non-interactive mode.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logWriter, rawLogCh := tui.LogSink(256)
	log.SetOutput(io.MultiWriter(os.Stderr, logWriter))

	rawEvents := make(chan endpoint.Event, 256)
	srv, err := server.New(*cfg, endpoint.WithEventSink(rawEvents))
	if err != nil {
		log.Fatalf("%s  create server: %v", now(), err)
	}

	hub := attach.NewHub(*cfg, func() []attach.EndpointSnapshot {
		depths := srv.Registry().QueueDepths()
		snaps := make([]attach.EndpointSnapshot, 0, len(depths))
		for path, depth := range depths {
			snaps = append(snaps, attach.EndpointSnapshot{Path: path, QueueDepth: depth})
		}
		return snaps
	})
	go hub.Run(ctx, rawEvents, rawLogCh)
	go attach.Serve(ctx, hub, attach.SocketPath(os.Getpid()))

	log.Printf("%s  rls listening on %s:%d", now(), cfg.Server.Host, cfg.Server.Port)
	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("%s  server error: %v", now(), err)
		}
	}()

	<-ctx.Done()
	srv.Shutdown() //nolint:errcheck
}

func loadConfig(path string, overrides map[string]string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if err := config.MergeOverrides(cfg, overrides); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8080
	config.ApplyDefaults(cfg)
	return cfg
}

func now() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

func init() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
}
