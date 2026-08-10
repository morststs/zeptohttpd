package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigMissingFileIsNotAnError(t *testing.T) {
	config, err := loadConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if len(config.ContentTypes) != 0 {
		t.Fatalf("want empty config, got %#v", config)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	name := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, name, "{not json")
	if _, err := loadConfig(name); err == nil {
		t.Fatal("want error for malformed config")
	}
}

func TestLoadConfigParsesContentTypes(t *testing.T) {
	name := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, name, `{"ContentTypes":{"/api/":"application/json"}}`)
	config, err := loadConfig(name)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.ContentTypes["/api/"]; got != "application/json" {
		t.Fatalf("want application/json, got %q", got)
	}
}

func TestContentTypeForLongestPrefixWins(t *testing.T) {
	config := Config{ContentTypes: map[string]string{
		"/a":        "text/plain",
		"/a/b":      "application/json",
		"/a/b/c/d":  "text/csv",
		"/unrelate": "text/html",
	}}
	tests := []struct{ path, want string }{
		{"/a/x", "text/plain"},
		{"/a/b/x", "application/json"},
		{"/a/b/c/d/e", "text/csv"},
		{"/zzz", ""},
	}
	// Repeat: map iteration order is randomised, so a non-deterministic
	// implementation fails this reliably.
	for i := 0; i < 50; i++ {
		for _, tt := range tests {
			if got := config.contentTypeFor(tt.path); got != tt.want {
				t.Fatalf("contentTypeFor(%q) = %q, want %q", tt.path, got, tt.want)
			}
		}
	}
}

func TestHasDotSegment(t *testing.T) {
	tests := map[string]bool{
		"/index.html":      false,
		"/a/b/c.txt":       false,
		"/.git/config":     true,
		"/sub/.env":        true,
		"/..%2f":           true, // odd leading-dot segment, treated as hidden
		"/a/../b":          false,
		"/.well-known/foo": true,
	}
	for urlPath, want := range tests {
		if got := hasDotSegment(urlPath); got != want {
			t.Errorf("hasDotSegment(%q) = %v, want %v", urlPath, got, want)
		}
	}
}

func newTestServer(t *testing.T, config Config, dotfiles bool) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "<h1>hi</h1>")
	writeFile(t, filepath.Join(dir, "data", "payload"), `{"ok":true}`)
	writeFile(t, filepath.Join(dir, ".git", "config"), "secret")
	server := httptest.NewServer(newHandler(config, dir, dotfiles))
	t.Cleanup(server.Close)
	return server, dir
}

func get(t *testing.T, server *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := server.Client().Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestServesIndex(t *testing.T) {
	server, _ := newTestServer(t, Config{}, false)
	resp := get(t, server, "/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("want text/html, got %q", ct)
	}
}

func TestConfiguredContentTypeOverridesSniffing(t *testing.T) {
	config := Config{ContentTypes: map[string]string{"/data/": "application/json"}}
	server, _ := newTestServer(t, config, false)
	resp := get(t, server, "/data/payload")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("want application/json, got %q", ct)
	}
}

func TestQueryParametersAreIgnored(t *testing.T) {
	config := Config{ContentTypes: map[string]string{"/data/": "application/json"}}
	server, _ := newTestServer(t, config, false)
	resp := get(t, server, "/data/payload?v=1&cache=no")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("want application/json, got %q", ct)
	}
}

func TestDotfilesAreHiddenByDefault(t *testing.T) {
	server, _ := newTestServer(t, Config{}, false)
	for _, path := range []string{"/.git/config", "/.git/", "//.git/config"} {
		if resp := get(t, server, path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: want 404, got %d", path, resp.StatusCode)
		}
	}
}

func TestDotfilesCanBeEnabled(t *testing.T) {
	server, _ := newTestServer(t, Config{}, true)
	if resp := get(t, server, "/.git/config"); resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 with -dotfiles, got %d", resp.StatusCode)
	}
}

func TestPathTraversalIsRejected(t *testing.T) {
	server, dir := newTestServer(t, Config{}, false)
	writeFile(t, filepath.Join(filepath.Dir(dir), "outside.txt"), "nope")
	for _, path := range []string{"/../outside.txt", "/..%2foutside.txt", "/data/../../outside.txt"} {
		resp := get(t, server, path)
		if resp.StatusCode == http.StatusOK {
			t.Errorf("GET %s: served a file outside the root", path)
		}
	}
}

func TestNonReadMethodsRejected(t *testing.T) {
	server, _ := newTestServer(t, Config{}, false)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req, err := http.NewRequest(method, server.URL+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d", method, resp.StatusCode)
		}
		if allow := resp.Header.Get("Allow"); allow != "GET, HEAD" {
			t.Errorf("%s: want Allow header, got %q", method, allow)
		}
	}
}

func TestHeadIsAllowed(t *testing.T) {
	server, _ := newTestServer(t, Config{}, false)
	resp, err := server.Client().Head(server.URL + "/index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestServerHasTimeouts(t *testing.T) {
	server := newServer(":0", http.NotFoundHandler())
	if server.ReadHeaderTimeout == 0 || server.WriteTimeout == 0 || server.IdleTimeout == 0 {
		t.Fatalf("server is missing timeouts: %+v", server)
	}
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()

	server := newServer(addr, http.NotFoundHandler())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, server) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if conn, err := net.Dial("tcp", addr); err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server never started listening")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}
}

func TestRunReportsListenError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := newServer(listener.Addr().String(), http.NotFoundHandler())
	if err := run(context.Background(), server); err == nil {
		t.Fatal("want error when the port is already in use")
	}
}

// The repository ships config/config.json; make sure it stays loadable.
func TestShippedConfigIsValid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "config", "config.json"))
	if err != nil {
		t.Skipf("config not present: %v", err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("shipped config.json is invalid: %v", err)
	}
}
