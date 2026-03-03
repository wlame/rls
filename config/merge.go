package config

import (
	"flag"
	"fmt"
	"strconv"
)

// ParseFlags parses command-line flags and returns the config file path
// plus a map of override key→value pairs.
func ParseFlags() (configPath string, overrides map[string]string, err error) {
	fs := flag.NewFlagSet("rls", flag.ContinueOnError)
	overrides = make(map[string]string)

	var (
		cfgPath string
		port    int
		host    string
	)

	fs.StringVar(&cfgPath, "config", "rls.yml", "path to config file")
	fs.IntVar(&port, "port", 0, "override server port")
	fs.StringVar(&host, "host", "", "override server host")

	if err = fs.Parse(flag.Args()); err != nil {
		return "", nil, fmt.Errorf("parse flags: %w", err)
	}

	configPath = cfgPath
	if port != 0 {
		overrides["port"] = strconv.Itoa(port)
	}
	if host != "" {
		overrides["host"] = host
	}
	return configPath, overrides, nil
}

// MergeOverrides applies override values from CLI flags into cfg.
// Supported keys: "port", "host".
func MergeOverrides(cfg *Config, overrides map[string]string) error {
	for k, v := range overrides {
		switch k {
		case "port":
			p, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("invalid port %q: %w", v, err)
			}
			cfg.Server.Port = p
		case "host":
			cfg.Server.Host = v
		default:
			return fmt.Errorf("unknown override key %q", k)
		}
	}
	return nil
}
