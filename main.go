package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/wlame/rls/config"
)

func main() {
	// Define flags directly so flag.Parse() picks them up.
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
	}

	fmt.Printf("rls starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	_ = cfg
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
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
}
