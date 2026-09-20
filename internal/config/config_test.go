package config_test

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// examplePath is the committed file that documents every variable, relative to this package.
const examplePath = "../../.env.example"

// ctlOnlyVars are read by ./ctl.sh rather than by the binary, so they appear in the example file
// without appearing on the Config struct.
//
// The list is here, short and explicit, rather than inferred from a section heading in the file:
// a heuristic exemption grows quietly, and an exemption that grows quietly is how a variable stops
// being checked. Adding one means editing this line.
var ctlOnlyVars = map[string]bool{
	"GORETROTV_PORT": true,
}

// setEnv installs a complete, valid environment and clears everything else the binary reads, so a
// test never passes because of a variable that happened to be exported in the shell that ran it.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, v := range config.Variables() {
		if value, ok := env[v.Name]; ok {
			t.Setenv(v.Name, value)
			continue
		}
		// t.Setenv restores the previous value when the test ends, which is what makes clearing
		// safe to do here rather than in a defer the next reader has to find.
		t.Setenv(v.Name, "")
	}
}

// validEnv is a complete, minimal environment: every required variable and nothing else, so a
// test that removes one is testing the removal rather than an unrelated omission.
func validEnv() map[string]string {
	env := make(map[string]string)
	for _, v := range config.Variables() {
		if !v.Required {
			continue
		}
		// Derived from the declaration rather than listed by hand: a new required variable must
		// break the tests that assert refusal, not the ones that assert success.
		env[v.Name] = "set-for-test"
	}
	return env
}

// TestEveryVariableTheBinaryReadsIsDocumented is TC-1.9's second clause: asserted by walking the
// config struct, not by eye.
//
// It walks in both directions. A variable on the struct and not in the file is an undocumented
// setting; a variable in the file and not on the struct is a line an operator will set and the
// binary will ignore, which is worse, because it looks like it worked.
func TestEveryVariableTheBinaryReadsIsDocumented(t *testing.T) {
	t.Parallel()

	declared := config.Variables()
	// Two guards before any coverage claim, because they catch different failures.
	//
	// The first is that the instrument found its subject at all: a walk returning nothing would
	// satisfy "every variable is documented" vacuously, which is the green that means nothing.
	if err := instrument.MustFind("config completeness", "env-tagged Config fields", len(declared), 5); err != nil {
		t.Fatalf("%v", err)
	}
	// The second is stronger and is the one that is easy to miss: the DRIVER must reach every
	// field, not merely some. A field the walk never visits is invisible to every assertion
	// below while they all keep reporting success - the same defect as a snapshot mutator that
	// leaves a field at its fresh value and so compares it against itself.
	fields := reflect.TypeOf(config.Config{}).NumField()
	if len(declared) != fields {
		t.Fatalf("the walk produced %d variables for %d struct fields; the %d it did not reach are "+
			"invisible to the coverage check below, which would still pass",
			len(declared), fields, fields-len(declared))
	}

	documented := parseExample(t)
	if err := instrument.MustFind("config completeness", "variables in "+examplePath, len(documented), len(declared)); err != nil {
		t.Fatalf("%v", err)
	}

	census := instrument.NewCensus("config completeness", "declared variables")
	census.Declare("documented", "undocumented")

	for _, v := range declared {
		census.Examine()
		finding := "documented"
		if !documented[v.Name] {
			finding = "undocumented"
			t.Errorf("%s (Config.%s) is read by the binary but is not in %s", v.Name, v.Field, examplePath)
		}
		if err := census.Record(finding); err != nil {
			t.Fatalf("census: %v", err)
		}
	}

	known := make(map[string]bool, len(declared))
	for _, v := range declared {
		known[v.Name] = true
	}
	var stray []string
	for name := range documented {
		if !known[name] && !ctlOnlyVars[name] {
			stray = append(stray, name)
		}
	}
	sort.Strings(stray)
	for _, name := range stray {
		t.Errorf("%s is in %s but nothing on Config reads it; an operator would set it and the binary would ignore it", name, examplePath)
	}

	res, err := census.Result()
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	t.Logf("%s", res)
}

// TestTheExampleFileMarksTheRequiredVariables keeps the file honest about which lines an operator
// cannot leave out.
func TestTheExampleFileMarksTheRequiredVariables(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Clean(examplePath))
	if err != nil {
		t.Fatalf("read %s: %v", examplePath, err)
	}
	text := string(body)

	required := 0
	for _, v := range config.Variables() {
		if !v.Required {
			continue
		}
		required++
		if !strings.Contains(text, v.Name) {
			t.Errorf("required variable %s is not in %s", v.Name, examplePath)
		}
	}
	if required == 0 {
		t.Fatal("no variable is required, so the refusal path below is unreachable and this file's Required column means nothing")
	}
	if !strings.Contains(text, "Required") {
		t.Errorf("%s does not mark which variables are required", examplePath)
	}
}

// TestAMissingRequiredVariableRefusesTheBoot is TC-1.9's first clause.
func TestAMissingRequiredVariableRefusesTheBoot(t *testing.T) {
	for _, v := range config.Variables() {
		if !v.Required {
			continue
		}
		t.Run(v.Name, func(t *testing.T) {
			env := validEnv()
			delete(env, v.Name)
			setEnv(t, env)

			cfg, err := config.Load()
			if err == nil {
				t.Fatalf("Load succeeded without %s and returned %+v; a silent default is the failure this prevents", v.Name, cfg)
			}
			if cfg != nil {
				t.Errorf("Load returned a config alongside its error: %+v", cfg)
			}
			if !strings.Contains(err.Error(), v.Name) {
				t.Errorf("error %q does not name %s; an operator cannot act on a refusal that does not say what is missing", err, v.Name)
			}
		})
	}
}

// TestAnEmptyRequiredVariableIsAlsoMissing closes the gap between "not exported" and "exported as
// nothing", which look identical to an operator and must behave identically here.
func TestAnEmptyRequiredVariableIsAlsoMissing(t *testing.T) {
	setEnv(t, map[string]string{"GORETROTV_ENV": "   "})

	if cfg, err := config.Load(); err == nil {
		t.Fatalf("Load accepted a blank required variable and returned %+v", cfg)
	}
}

// TestEveryMissingRequiredVariableIsReportedAtOnce spares an operator one restart per mistake.
func TestEveryMissingRequiredVariableIsReportedAtOnce(t *testing.T) {
	var required []string
	for _, v := range config.Variables() {
		if v.Required {
			required = append(required, v.Name)
		}
	}
	setEnv(t, nil)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load succeeded with every required variable absent")
	}
	for _, name := range required {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestLoadAppliesDefaultsAndOverrides(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "set-for-test" {
		t.Errorf("Env = %q, want set-for-test", cfg.Env)
	}
	if cfg.ServiceName != config.Defaults.ServiceName {
		t.Errorf("ServiceName = %q, want the default %q", cfg.ServiceName, config.Defaults.ServiceName)
	}
	if cfg.HTTPAddr != config.Defaults.HTTPAddr {
		t.Errorf("HTTPAddr = %q, want the default %q", cfg.HTTPAddr, config.Defaults.HTTPAddr)
	}
	if cfg.EnablePprof {
		t.Error("EnablePprof defaulted to true; a developer surface must be off unless asked for")
	}

	env := validEnv()
	env["GORETROTV_ENV"] = "development"
	env["GORETROTV_SERVICE_NAME"] = "goretrotv-oracle"
	env["GORETROTV_HTTP_ADDR"] = "127.0.0.1:9000"
	env["GORETROTV_LOG_LEVEL"] = "debug"
	env["GORETROTV_LOG_FORMAT"] = "text"
	env["GORETROTV_ENABLE_PPROF"] = "true"
	setEnv(t, env)

	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{
		Env:                  "development",
		FirmwareDir:          env["GORETROTV_FIRMWARE_DIR"],
		WebDir:               "web",
		ServiceName:          "goretrotv-oracle",
		HTTPAddr:             "127.0.0.1:9000",
		AllowNonLoopbackBind: false,
		LogLevel:             "debug",
		LogFormat:            "text",
		EnablePprof:          true,
		BroadcastDate:        config.Defaults.BroadcastDate,
	}
	if *cfg != want {
		t.Errorf("Load = %+v, want %+v", *cfg, want)
	}
}

// TestTheDefaultListenerBindsLoopback guards the recorded security decision at the level it is
// actually made. p1-security audits the same thing from outside; this fails first and cheaper.
func TestTheDefaultListenerBindsLoopback(t *testing.T) {
	t.Parallel()

	if !strings.HasPrefix(config.Defaults.HTTPAddr, "127.0.0.1:") {
		t.Errorf("the default listener is %q; developer surfaces bind to localhost (CLAUDE.md -> Deployment and access)", config.Defaults.HTTPAddr)
	}
}

func TestNonLoopbackListenerRequiresExplicitContainerOverride(t *testing.T) {
	cases := []struct {
		name, addr, override string
		allowed              bool
	}{
		{"IPv4 loopback", "127.0.0.1:8099", "", true},
		{"IPv6 loopback", "[::1]:8099", "", true},
		{"localhost", "localhost:8099", "", true},
		{"empty host", ":8099", "", false},
		{"IPv4 any", "0.0.0.0:8099", "", false},
		{"IPv6 any", "[::]:8099", "", false},
		{"public IP", "192.0.2.1:8099", "", false},
		{"DNS name", "example.test:8099", "", false},
		{"malformed", "not-an-address", "", false},
		{"container override", ":8099", "true", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := validEnv()
			env["GORETROTV_HTTP_ADDR"] = tc.addr
			env["GORETROTV_BIND_ALL_INTERFACES"] = tc.override
			setEnv(t, env)
			cfg, err := config.Load()
			if tc.allowed && err != nil {
				t.Fatalf("Load rejected %q with override %q: %v", tc.addr, tc.override, err)
			}
			if !tc.allowed && err == nil {
				t.Fatalf("Load accepted %q without an override: %+v", tc.addr, cfg)
			}
			if !tc.allowed && tc.addr != "not-an-address" && !strings.Contains(err.Error(), "localhost only") {
				t.Errorf("refusal %q does not name the loopback rule", err)
			}
		})
	}
}

func TestAnUnparseableValueIsRefusedByName(t *testing.T) {
	env := validEnv()
	env["GORETROTV_ENABLE_PPROF"] = "yes please"
	setEnv(t, env)

	cfg, err := config.Load()
	if err == nil {
		t.Fatalf("Load accepted a non-boolean and returned %+v", cfg)
	}
	for _, want := range []string{"GORETROTV_ENABLE_PPROF", "yes please"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestNoRequiredVariableHasADefault catches the contradiction directly: a default is the guess the
// requirement exists to prevent, and a required variable carrying one would never refuse anything.
func TestNoRequiredVariableHasADefault(t *testing.T) {
	t.Parallel()

	defaults := reflect.ValueOf(config.Defaults)
	for _, v := range config.Variables() {
		if !v.Required {
			continue
		}
		field := defaults.FieldByName(v.Field)
		if !field.IsValid() {
			t.Fatalf("Config.%s is declared but absent from Defaults' type", v.Field)
		}
		if !field.IsZero() {
			t.Errorf("%s is required but Defaults.%s is %v; the default would be the guess the requirement exists to prevent",
				v.Name, v.Field, field.Interface())
		}
	}
}

// TestEveryFieldHasAParser stops a new setting of an unhandled type from being silently left at
// its default. Load errors on one at runtime; this puts the failure at test time instead.
func TestEveryFieldHasAParser(t *testing.T) {
	// Not parallel: it sets the environment, which is process-wide.
	env := make(map[string]string, len(config.Variables()))
	for _, v := range config.Variables() {
		switch v.Kind {
		case "string":
			env[v.Name] = "x"
		case "bool":
			env[v.Name] = "true"
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64":
			env[v.Name] = "1"
		default:
			t.Errorf("%s (Config.%s) is a %s, which config.assign has no parser for", v.Name, v.Field, v.Kind)
		}
	}
	env["GORETROTV_HTTP_ADDR"] = "127.0.0.1:8099"
	// GORETROTV_ENV is a string like any other here; "x" is a value, not a meaningful one.
	setEnvMap(t, env)

	if _, err := config.Load(); err != nil {
		t.Errorf("Load with every variable set: %v", err)
	}
}

func setEnvMap(t *testing.T, env map[string]string) {
	t.Helper()
	for name, value := range env {
		t.Setenv(name, value)
	}
}

func TestVariablesDeclaresWhatTheStructDeclares(t *testing.T) {
	t.Parallel()

	vars := config.Variables()
	structType := reflect.TypeOf(config.Config{})
	if len(vars) != structType.NumField() {
		t.Errorf("Variables() returned %d entries for %d struct fields; a field with no env tag is a setting nobody can set",
			len(vars), structType.NumField())
	}

	seen := make(map[string]bool, len(vars))
	for _, v := range vars {
		if v.Name == "" || v.Field == "" || v.Kind == "" {
			t.Errorf("incomplete Variable: %+v", v)
		}
		if seen[v.Name] {
			t.Errorf("%s is declared twice; one of the two fields will never be read", v.Name)
		}
		seen[v.Name] = true
		if !strings.HasPrefix(v.Name, "GORETROTV_") {
			t.Errorf("%s does not carry the GORETROTV_ prefix", v.Name)
		}
	}
}

// parseExample reads the variable names out of the committed example file.
func parseExample(t *testing.T) map[string]bool {
	t.Helper()

	f, err := os.Open(filepath.Clean(examplePath))
	if err != nil {
		t.Fatalf("open %s: %v", examplePath, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close %s: %v", examplePath, err)
		}
	}()

	found := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, ok := strings.Cut(line, "=")
		if !ok {
			t.Errorf("%s has a line that is neither a comment nor an assignment: %q", examplePath, line)
			continue
		}
		found[strings.TrimSpace(name)] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", examplePath, err)
	}
	return found
}

// TestTheExampleFileCarriesNoSecrets is a cheap guard on a committed file. The real credential
// guard is the pre-commit hook; this one fails in CI too, and a second place to catch it costs
// nothing.
func TestTheExampleFileCarriesNoSecrets(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Clean(examplePath))
	if err != nil {
		t.Fatalf("read %s: %v", examplePath, err)
	}
	for _, suspect := range []string{"PASSWORD=", "SECRET=", "TOKEN=", "API_KEY=", "PRIVATE_KEY"} {
		if strings.Contains(string(body), suspect) {
			t.Errorf("%s contains %q; this file carries variable names and harmless examples only", examplePath, suspect)
		}
	}
}
