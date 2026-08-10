// Command zeptohttpd is a tiny static HTTP server intended for development
// and operation checks.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

// Build information, overridable with -ldflags "-X main.version=...".
var version = "dev"

// Config is the on-disk configuration file format.
type Config struct {
	// ContentTypes maps a URL path prefix to the Content-Type header that
	// should be sent for requests below that prefix. The longest matching
	// prefix wins.
	ContentTypes map[string]string
}

// loadConfig reads the configuration file. A missing file is not an error:
// the server runs with defaults so the image is usable without a config.
func loadConfig(name string) (Config, error) {
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", name, err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", name, err)
	}
	return config, nil
}

// contentTypeFor returns the configured Content-Type for a URL path, using the
// longest matching prefix so that overlapping rules resolve deterministically.
func (c Config) contentTypeFor(urlPath string) string {
	best, matched := "", -1
	for prefix, contentType := range c.ContentTypes {
		if len(prefix) > matched && strings.HasPrefix(urlPath, prefix) {
			best, matched = contentType, len(prefix)
		}
	}
	return best
}

// hasDotSegment reports whether any element of the path starts with a dot,
// which keeps files such as .git/ or .env from being served by accident.
func hasDotSegment(urlPath string) bool {
	for _, elem := range strings.Split(path.Clean(urlPath), "/") {
		if strings.HasPrefix(elem, ".") && elem != "." && elem != ".." {
			return true
		}
	}
	return false
}

// newHandler serves dir as a static site according to config.
func newHandler(config Config, dir string, dotfiles bool) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !dotfiles && hasDotSegment(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if contentType := config.contentTypeFor(r.URL.Path); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		fileServer.ServeHTTP(w, r)
	})
}

// statusRecorder captures the response status so it can be logged.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// withLogging logs one line per request. %q quotes the path so that a crafted
// URL cannot inject newlines into the log.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		log.Printf("%s %q %d %s", r.Method, r.URL.RequestURI(), rec.status, time.Since(start).Round(time.Microsecond))
	})
}

// newServer builds an http.Server with timeouts that protect against clients
// that open a connection and then stall (Slowloris).
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
		ErrorLog:          log.Default(),
	}
}

// run starts the server and blocks until ctx is cancelled, then shuts down
// gracefully so in-flight responses are allowed to finish.
func run(ctx context.Context, server *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		log.Printf("zeptohttpd %s listening on %s", version, server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Print("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

// env returns the value of key, or fallback when it is unset or empty.
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	addr := flag.String("addr", env("ZEPTO_ADDR", ":80"), "address to listen on (env ZEPTO_ADDR)")
	dir := flag.String("dir", env("ZEPTO_DIR", "public"), "directory to serve (env ZEPTO_DIR)")
	configPath := flag.String("config", env("ZEPTO_CONFIG", "config.json"), "path to config.json (env ZEPTO_CONFIG)")
	dotfiles := flag.Bool("dotfiles", false, "serve paths containing dot-prefixed elements such as .git")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)

	if *showVersion {
		fmt.Println(version)
		return
	}

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(*dir); err != nil {
		log.Fatalf("serve directory unavailable: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := newServer(*addr, withLogging(newHandler(config, *dir, *dotfiles)))
	if err := run(ctx, server); err != nil {
		log.Fatal(err)
	}
}
