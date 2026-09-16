// Package config loads the binary's settings from the environment.
//
// Every setting the binary reads is a field on Config with an `env` tag, and Load walks those tags
// rather than naming variables a second time in code. That is the point: a hand-written loader and
// a struct definition drift, and the drift is invisible — a field nobody reads, or a variable
// nobody documented, which is how a deployment acquires an undeclared dependency on a default.
// Here the tags are the only list, and the example env file is checked against them by a test that
// walks the same struct.
//
// A variable marked required has no default and no guess. Load refuses to start and names it,
// because the alternative is a process that runs happily under an assumption nobody made
// deliberately. Everything else has a default that is safe to assume, and those defaults live in
// Defaults where they can be read at a glance.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Config is the complete set of environment settings the binary reads.
type Config struct {
	// Env is which deployment this is. Required, and deliberately so: there is no safe guess
	// between development and production, and a box that quietly decided it was a development
	// box is the failure that announces itself last.
	Env string `env:"GORETROTV_ENV,required"`

	// FirmwareDir is where the flash images and their manifest live. Required, and for a
	// sharper reason than Env.
	//
	// firmware.Load reads MANIFEST.md from this same directory and verifies the images against
	// it, so the checksum guard cannot notice a wrong DIRECTORY - a different firmware set
	// arrives with its own manifest and verifies perfectly against itself. The one failure
	// TC-1.4 exists to prevent, silently running a different ROM, is therefore invisible to
	// every check downstream of this value, which means the value itself has to be stated.
	//
	// A relative default would be worse than no default: it resolves against the process's
	// working directory, so the same binary finds different ROMs depending on where it was
	// started. That is exactly how the container works today by accident - the image mounts
	// /firmware and a relative path happens to resolve because distroless sets no WORKDIR.
	FirmwareDir string `env:"GORETROTV_FIRMWARE_DIR,required"`

	// ServiceName labels log lines.
	ServiceName string `env:"GORETROTV_SERVICE_NAME"`

	// HTTPAddr is what the server listens on. The default binds the loopback interface, which is
	// the recorded security decision (CLAUDE.md -> Deployment and access): the gdb stub and the
	// instrument endpoints are the developer surfaces on a public demo host, and loopback is the
	// one real control over them. A container overrides this to bind all interfaces and publishes
	// the port through a loopback-only mapping instead.
	HTTPAddr string `env:"GORETROTV_HTTP_ADDR"`

	// AllowNonLoopbackBind is for the container only. Its port is mapped to host loopback by
	// docker-compose.yml; a native process has no such fence and must leave this false.
	AllowNonLoopbackBind bool `env:"GORETROTV_BIND_ALL_INTERFACES"`

	// LogLevel is one of debug, info, warn, error.
	LogLevel string `env:"GORETROTV_LOG_LEVEL"`

	// LogFormat is json or text.
	LogFormat string `env:"GORETROTV_LOG_FORMAT"`

	// EnablePprof mounts the profiling endpoints. Off by default; a developer surface.
	EnablePprof bool `env:"GORETROTV_ENABLE_PPROF"`
}

// Defaults are the values used when an optional variable is absent from the environment.
//
// Required variables are absent from this struct on purpose. A required variable with a default is
// a contradiction — the default would be the guess the requirement exists to prevent — and a test
// asserts that none of them has one.
var Defaults = Config{
	ServiceName: "goretrotv",
	HTTPAddr:    "127.0.0.1:8099",
	LogLevel:    "info",
	LogFormat:   "json",
	EnablePprof: false,
}

// Variable describes one environment setting, as declared by the struct tags.
//
// It is exported because the completeness test walks it: asserting that the example env file lists
// every variable the binary reads is only meaningful if both sides read the same declaration.
type Variable struct {
	// Name is the environment variable, e.g. GORETROTV_LOG_LEVEL.
	Name string
	// Field is the Go field it fills, for error messages.
	Field string
	// Required says Load refuses to start without it.
	Required bool
	// Kind is the field's type, e.g. "string" or "bool".
	Kind string
}

// Variables lists every environment setting the binary reads, in declaration order.
func Variables() []Variable {
	t := reflect.TypeOf(Config{})
	out := make([]Variable, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, required, ok := parseTag(field)
		if !ok {
			continue
		}
		out = append(out, Variable{
			Name:     name,
			Field:    field.Name,
			Required: required,
			Kind:     field.Type.Kind().String(),
		})
	}
	return out
}

// Load reads the environment over the defaults.
//
// Every missing required variable is reported together rather than one per restart: an operator
// bringing a new host up should learn the whole list on the first attempt.
func Load() (*Config, error) {
	cfg := Defaults
	value := reflect.ValueOf(&cfg).Elem()
	structType := value.Type()

	var missing []string
	var problems []error

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		name, required, ok := parseTag(field)
		if !ok {
			continue
		}

		raw, present := os.LookupEnv(name)
		raw = strings.TrimSpace(raw)
		if !present || raw == "" {
			if required {
				missing = append(missing, name)
			}
			continue
		}

		if err := assign(value.Field(i), raw); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", name, err))
		}
	}

	if len(missing) > 0 {
		problems = append(problems, fmt.Errorf(
			"missing required environment %s: %s (see .env.example)",
			plural(len(missing), "variable", "variables"), strings.Join(missing, ", ")))
	}
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	if err := validateHTTPBind(cfg.HTTPAddr, cfg.AllowNonLoopbackBind); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validateHTTPBind(addr string, allowNonLoopback bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("GORETROTV_HTTP_ADDR: %w", err)
	}
	if allowNonLoopback {
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("GORETROTV_HTTP_ADDR %q: developer surfaces bind to localhost only (CLAUDE.md); set GORETROTV_BIND_ALL_INTERFACES only behind a loopback-only port mapping", addr)
	}
	return nil
}

// parseTag reads the `env:"NAME[,required]"` tag. A field without one is not a setting.
func parseTag(field reflect.StructField) (name string, required, ok bool) {
	tag, ok := field.Tag.Lookup("env")
	if !ok || tag == "" || tag == "-" {
		return "", false, false
	}
	parts := strings.Split(tag, ",")
	name = strings.TrimSpace(parts[0])
	if name == "" {
		return "", false, false
	}
	for _, opt := range parts[1:] {
		if strings.TrimSpace(opt) == "required" {
			required = true
		}
	}
	return name, required, true
}

// assign parses raw into the field.
//
// An unsupported kind is an error rather than a skip. A skipped field would keep its default while
// the environment plainly asked for something else, which is the silent default this whole package
// is arranged to prevent; a test also asserts every current field is of a kind handled here, so
// the failure lands at test time rather than on the demo host.
func assign(field reflect.Value, raw string) error {
	// The default case returns an error rather than skipping, which is the thing the exhaustive
	// linter exists to catch. Listing all twenty-six reflect.Kind values to say "everything else
	// is an error" would bury the four that are actually handled.
	switch field.Kind() { //nolint:exhaustive // the default case errors; see the comment above
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%q is not a boolean: %w", raw, err)
		}
		field.SetBool(v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not an integer: %w", raw, err)
		}
		field.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not an unsigned integer: %w", raw, err)
		}
		field.SetUint(v)
	default:
		return fmt.Errorf("no parser for a %s setting; add one to config.assign rather than leaving the value unread", field.Kind())
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
