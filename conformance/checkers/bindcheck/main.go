package main

import (
	"encoding/json"
	"os"

	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/gdbstub"
)

func main() {
	for _, v := range config.Variables() {
		_ = os.Unsetenv(v.Name)
	}
	_ = os.Setenv("GORETROTV_ENV", "development")
	_ = os.Setenv("GORETROTV_FIRMWARE_DIR", "./firmware")

	type result struct {
		Name    string `json:"name"`
		Allowed bool   `json:"allowed"`
	}
	cases := []struct {
		name, addr string
		override   bool
	}{
		{"loopback", "127.0.0.1:8099", false},
		{"ipv6-loopback", "[::1]:8099", false},
		{"empty-host", ":8099", false},
		{"ipv4-any", "0.0.0.0:8099", false},
		{"ipv6-any", "[::]:8099", false},
		{"public-ip", "192.0.2.1:8099", false},
		{"container-override", ":8099", true},
	}
	results := make([]result, 0, len(cases))
	for _, tc := range cases {
		_ = os.Setenv("GORETROTV_HTTP_ADDR", tc.addr)
		_ = os.Unsetenv("GORETROTV_BIND_ALL_INTERFACES")
		if tc.override {
			_ = os.Setenv("GORETROTV_BIND_ALL_INTERFACES", "true")
		}
		_, err := config.Load()
		results = append(results, result{Name: tc.name, Allowed: err == nil})
	}
	for _, tc := range []struct {
		name, addr string
	}{
		{"gdb-loopback", "127.0.0.1:0"},
		{"gdb-localhost", "localhost:0"},
		{"gdb-empty-host", ":0"},
		{"gdb-ipv4-any", "0.0.0.0:0"},
	} {
		listener, err := gdbstub.Listen(tc.addr)
		if err == nil {
			if closeErr := listener.Close(); closeErr != nil {
				_, _ = os.Stderr.WriteString(closeErr.Error())
				os.Exit(2)
			}
		}
		results = append(results, result{Name: tc.name, Allowed: err == nil})
	}
	if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
}
