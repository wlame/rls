package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/server"
)

func main() {
	cfgPath := flag.String("config", "rls.yml", "path to config file")
	port := flag.Int("port", 0, "override server port")
	host := flag.String("host", "", "override server host")
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
		log.Printf("warning: %v — using built-in defaults", err)
		cfg = defaultConfig()
		// Still apply CLI overrides (port/host) on top of defaults.
		if mergeErr := config.MergeOverrides(cfg, overrides); mergeErr != nil {
			log.Fatalf("invalid flags: %v", mergeErr)
		}
	}

	srv, err := server.New(*cfg)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	log.Printf("rls listening on %s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := srv.Start(); err != nil {
		log.Fatalf("server error: %v", err)
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

func init() {
	log.SetFlags(log.LstdFlags)
	log.SetOutput(os.Stderr)
}
