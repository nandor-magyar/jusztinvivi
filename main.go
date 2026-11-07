package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed all:public
var embedded embed.FS

var version = "dev"

const siteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

const csp = "default-src 'self'; img-src 'self'; style-src 'self'; font-src 'self'; script-src 'self'; " +
	"base-uri 'self'; form-action 'none'; frame-ancestors 'none'; object-src 'none'"

// Turnstile loads on the contact pages only
const contactCSP = "default-src 'self'; img-src 'self'; style-src 'self'; font-src 'self'; " +
	"script-src 'self' https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; " +
	"base-uri 'self'; form-action 'none'; frame-ancestors 'none'; object-src 'none'"

var contactPages = []string{"/kapcsolat/", "/en/contact/"}

const defaultCache = "public, max-age=0, s-maxage=600, stale-while-revalidate=3600"

var cacheRules = []struct {
	re    *regexp.Regexp
	value string
}{
	{regexp.MustCompile(`^/version\.txt$`), "no-store"},
	{regexp.MustCompile(`\.min\.[0-9a-f]{64}\.(css|js)$`), "public, max-age=31536000, immutable"},
	{regexp.MustCompile(`^/fonts/`), "public, max-age=31536000, immutable"},
	{regexp.MustCompile(`_hu[0-9a-z]+\.(jpg|webp)$`), "public, max-age=31536000, immutable"},
	{regexp.MustCompile(`\.(png|jpe?g|webp|avif|gif|svg|ico)$`), "public, max-age=86400, s-maxage=604800"},
}

var contentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".json":  "application/json",
	".xml":   "application/xml; charset=utf-8",
	".txt":   "text/plain; charset=utf-8",
	".svg":   "image/svg+xml; charset=utf-8",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".gif":   "image/gif",
	".ico":   "image/x-icon",
	".woff2": "font/woff2",
	".woff":  "font/woff",
}

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz on the local server and exit")
	flag.Parse()

	addr := ":" + env("PORT", "8080")
	if *healthcheck {
		os.Exit(probe("http://127.0.0.1" + addr + "/healthz"))
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	public, err := fs.Sub(embedded, "public")
	if err != nil {
		fatal("embedded site", err)
	}
	site, err := newSite(public)
	if err != nil {
		fatal("index site", err)
	}

	contact := &contact{
		secret: os.Getenv("TURNSTILE_SECRET"),
		email:  os.Getenv("CONTACT_EMAIL"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
	if !contact.configured() {
		slog.Warn("contact disabled, set TURNSTILE_SECRET and CONTACT_EMAIL")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, "ok\n")
	})
	mux.Handle("POST /api/contact", contact)
	mux.Handle("/", site)

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(secure(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			slog.Error("shutdown", "err", err)
		}
	}()

	slog.Info("listening", "addr", addr, "version", version)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		fatal("listen", err)
	}
	<-done
}

type site struct {
	fs    fs.FS
	etags map[string]string
	dirs  map[string]bool
}

func newSite(fsys fs.FS) (*site, error) {
	s := &site{fs: fsys, etags: map[string]string{}, dirs: map[string]bool{}}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			s.dirs[name] = true
			return nil
		}
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		s.etags[name] = `"` + hex.EncodeToString(h.Sum(nil)[:12]) + `"`
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := s.etags["index.html"]; !ok {
		return nil, errors.New("no index.html, run hugo before go build")
	}
	return s, nil
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	switch {
	case name == "":
		name = "index.html"
	case s.dirs[name]:
		if !strings.HasSuffix(r.URL.Path, "/") {
			target := "/" + name + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		name += "/index.html"
	}
	if _, ok := s.etags[name]; !ok {
		notFound := "404.html"
		if strings.HasPrefix(name, "en/") {
			notFound = "en/404.html"
		}
		if _, ok := s.etags[notFound]; !ok {
			notFound = "404.html"
		}
		s.serve(w, r, notFound, http.StatusNotFound)
		return
	}
	s.serve(w, r, name, http.StatusOK)
}

func (s *site) serve(w http.ResponseWriter, r *http.Request, name string, status int) {
	h := w.Header()
	h.Set("Content-Type", contentType(name))
	h.Set("Cache-Control", cacheControl("/"+name))
	src := name
	if _, ok := s.etags[name+".gz"]; ok {
		h.Add("Vary", "Accept-Encoding")
		if acceptsGzip(r) {
			src = name + ".gz"
			h.Set("Content-Encoding", "gzip")
		}
	}
	f, err := s.fs.Open(src)
	if err != nil {
		slog.Error("open", "name", src, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if status != http.StatusOK {
		if fi, err := f.Stat(); err == nil {
			h.Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
		}
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			io.Copy(w, f)
		}
		return
	}
	h.Set("ETag", s.etags[src])
	http.ServeContent(w, r, "", time.Time{}, f.(io.ReadSeeker))
}

func contentType(name string) string {
	if t, ok := contentTypes[path.Ext(name)]; ok {
		return t
	}
	return "application/octet-stream"
}

func cacheControl(urlPath string) string {
	for _, rule := range cacheRules {
		if rule.re.MatchString(urlPath) {
			return rule.value
		}
	}
	return defaultCache
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		coding, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		q, ok := strings.CutPrefix(strings.TrimSpace(params), "q=")
		if !ok {
			return true
		}
		v, err := strconv.ParseFloat(q, 64)
		return err == nil && v > 0
	}
	return false
}

type contact struct {
	secret string
	email  string
	client *http.Client
}

func (c *contact) configured() bool {
	return c.secret != "" && c.email != ""
}

func (c *contact) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !c.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "contact not configured"})
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || len(req.Token) > 2048 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	ok, err := c.verify(r.Context(), req.Token)
	if err != nil {
		slog.Error("siteverify", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "verification unavailable"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "verification failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": c.email})
}

func (c *contact) verify(ctx context.Context, token string) (bool, error) {
	form := url.Values{"secret": {c.secret}, "response": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, siteverifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, errors.New("siteverify status " + resp.Status)
	}
	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return false, err
	}
	if !out.Success {
		slog.Info("turnstile rejected", "errors", out.ErrorCodes)
	}
	return out.Success, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Strict-Transport-Security", "max-age=31536000")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		h.Set("Content-Security-Policy", csp)
		for _, p := range contactPages {
			if strings.HasPrefix(r.URL.Path, p) {
				h.Set("Content-Security-Policy", contactCSP)
			}
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(start))
	})
}

func probe(target string) int {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
