// Package config loads the binary's settings from the environment.
//
// Every setting the binary reads is a field on Config with an `env` tag, so the set of variables
// is derivable from the struct rather than from memory. That is deliberate: a variable the binary
// reads but nobody documented is how a deployment acquires an undeclared dependency on a default.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the complete set of environment settings the binary reads.
type Config struct {
	Env         string `env:"GORETROTV_ENV"`
	ServiceName string `env:"GORETROTV_SERVICE_NAME"`
	HTTPAddr    string `env:"GORETROTV_HTTP_ADDR"`
	LogLevel    string `env:"GORETROTV_LOG_LEVEL"`
	LogFormat   string `env:"GORETROTV_LOG_FORMAT"`
	EnablePprof bool   `env:"GORETROTV_ENABLE_PPROF"`
}

// Defaults are the values used when a variable is absent from the environment.
//
// HTTPAddr binds the loopback interface. The recorded decision is that developer surfaces are not
// exposed (CLAUDE.md -> Deployment and access); a container publishes this port through an explicit
// host mapping, so loopback is the safe default for every other way the binary gets started.
var Defaults = Config{
	Env:         "development",
	ServiceName: "goretrotv",
	HTTPAddr:    "127.0.0.1:8099",
	LogLevel:    "info",
	LogFormat:   "json",
	EnablePprof: false,
}

// Load reads the environment over the defaults.
func Load() (*Config, error) {
	cfg := Defaults

	cfg.Env = stringVar("GORETROTV_ENV", cfg.Env)
	cfg.ServiceName = stringVar("GORETROTV_SERVICE_NAME", cfg.ServiceName)
	cfg.HTTPAddr = stringVar("GORETROTV_HTTP_ADDR", cfg.HTTPAddr)
	cfg.LogLevel = stringVar("GORETROTV_LOG_LEVEL", cfg.LogLevel)
	cfg.LogFormat = stringVar("GORETROTV_LOG_FORMAT", cfg.LogFormat)

	enablePprof, err := boolVar("GORETROTV_ENABLE_PPROF", cfg.EnablePprof)
	if err != nil {
		return nil, err
	}
	cfg.EnablePprof = enablePprof

	return &cfg, nil
}

func stringVar(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return fallback
}

func boolVar(name string, fallback bool) (bool, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %q is not a boolean: %w", name, v, err)
	}
	return parsed, nil
}
