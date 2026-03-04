package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

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
	flag.Parse()

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
		// Still apply CLI overrides (port/host) on top of defaults.
		if mergeErr := config.MergeOverrides(cfg, overrides); mergeErr != nil {
			log.Fatalf("%s  invalid flags: %v", now(), mergeErr)
		}
	}

	if *interactive {
		logWriter, logCh := tui.LogSink(256)
		log.SetOutput(logWriter)
		events := make(chan endpoint.Event, 256)
		srv, err := server.New(*cfg, endpoint.WithEventSink(events))
		if err != nil {
			log.Fatalf("%s  create server: %v", now(), err)
		}
		go func() {
			if err := srv.Start(); err != nil {
				log.Printf("%s  server stopped: %v", now(), err)
			}
		}()
		if err := tui.Run(cfg, events, tui.DotThresholds{Warn: *tuiWarn, Crit: *tuiCrit}, logCh); err != nil {
			log.Fatalf("%s  tui error: %v", now(), err)
		}
		srv.Shutdown() //nolint:errcheck
		return
	}

	srv, err := server.New(*cfg)
	if err != nil {
		log.Fatalf("%s  create server: %v", now(), err)
	}

	log.Printf("%s  rls listening on %s:%d", now(), cfg.Server.Host, cfg.Server.Port)
	if err := srv.Start(); err != nil {
		log.Fatalf("%s  server error: %v", now(), err)
	}
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
