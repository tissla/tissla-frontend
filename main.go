package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Action struct {
	Label Localized `json:"label"`
	URL   string    `json:"url"`
}
type Item struct {
	ID          string    `json:"id"`
	Order       int       `json:"order"`
	Title       string    `json:"title"`
	Description Localized `json:"description"`
	Images      []string  `json:"images"`
	Tags        []string  `json:"tags"`
	Actions     []Action  `json:"actions"`
	Repo        string    `json:"repo"`
}

func loadItems(dir string) ([]Item, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no portfolio items in %s", dir)
	}
	items := make([]Item, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var item Item
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&item); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return nil, fmt.Errorf("%s: trailing JSON", path)
		}
		if item.ID == "" || item.Title == "" || item.Description.Default() == "" || seen[item.ID] {
			return nil, fmt.Errorf("%s: missing fields or duplicate id", path)
		}
		seen[item.ID] = true
		for _, a := range item.Actions {
			u, e := url.Parse(a.URL)
			if e != nil || u.Scheme != "https" || u.Host == "" || a.Label.Default() == "" {
				return nil, fmt.Errorf("%s: invalid action", path)
			}
		}
		if item.Repo != "" {
			parts := strings.Split(item.Repo, "/")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(item.Repo, "?#% ") {
				return nil, fmt.Errorf("%s: invalid repo", path)
			}
		}
		for _, img := range item.Images {
			if !strings.HasPrefix(img, "/static/img/") || strings.Contains(img, "..") {
				return nil, fmt.Errorf("%s: invalid image", path)
			}
			if _, err := os.Stat(strings.TrimPrefix(img, "/")); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Order == items[j].Order {
			return items[i].ID < items[j].ID
		}
		return items[i].Order < items[j].Order
	})
	return items, nil
}

func newHandler(verifier string, client *http.Client, contentDir string) (http.Handler, error) {
	target, err := url.Parse(verifier)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return nil, fmt.Errorf("invalid VERIFIER_URL")
	}
	items, err := loadItems(contentDir)
	if err != nil {
		return nil, err
	}
	catalog, err := loadCatalog("static/locales")
	if err != nil {
		return nil, err
	}
	funcs := catalog.templateFuncs()
	funcs["asset"] = func(path string) (string, error) {
		data, err := os.ReadFile(strings.TrimPrefix(path, "/"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s?v=%x", path, sha256.Sum256(data)), nil
	}
	tmpl, err := template.New("page.html").Funcs(funcs).ParseFiles("templates/page.html")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	for language, messages := range catalog {
		data, err := json.Marshal(messages)
		if err != nil {
			return nil, err
		}
		mux.HandleFunc("GET /locales/"+language+".json", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(data)
		})
	}
	for path, portfolio := range map[string]bool{"/": false, "/portfolio": true} {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, struct {
			Portfolio bool
			Items     []Item
			Languages Catalog
		}{portfolio, items, catalog}); err != nil {
			return nil, err
		}
		page := append([]byte(nil), buf.Bytes()...)
		pattern := "GET " + path
		if path == "/" {
			pattern = "GET /{$}"
		}
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(page)
		})
	}
	mux.HandleFunc("GET /portfolio/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/portfolio", http.StatusPermanentRedirect)
	})
	files := http.FileServer(http.Dir("static"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, err := os.Stat(filepath.Join("static", filepath.Clean("/"+r.URL.Path)))
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	for public, upstream := range map[string]string{"status": "authenticate", "request-code": "request-code", "verify-code": "verify-code", "logout": "logout"} {
		method := "POST"
		if public == "status" {
			method = "GET"
		}
		mux.HandleFunc(method+" /auth/"+public, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method == "POST" {
				if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
					jsonError(w, 403, "forbidden")
					return
				}
				if origin := r.Header.Get("Origin"); origin != "" {
					u, e := url.Parse(origin)
					if e != nil || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "https" && u.Scheme != "http") {
						jsonError(w, 403, "forbidden")
						return
					}
				}
				if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
					jsonError(w, 415, "JSON required")
					return
				}
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
			if err != nil {
				jsonError(w, 413, "request too large")
				return
			}
			req, err := http.NewRequestWithContext(r.Context(), "POST", strings.TrimRight(target.String(), "/")+"/verify/"+upstream, bytes.NewReader(body))
			if err != nil {
				jsonError(w, 502, "verifier unavailable")
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if cookie, err := r.Cookie("auth_token"); err == nil {
				req.AddCookie(cookie)
			}
			resp, err := client.Do(req)
			if err != nil {
				jsonError(w, 502, "verifier unavailable")
				return
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
			if err != nil || len(data) > 65536 || !json.Valid(data) || resp.StatusCode >= 300 && resp.StatusCode < 400 {
				jsonError(w, 502, "invalid verifier response")
				return
			}
			for _, cookie := range resp.Cookies() {
				if cookie.Name == "auth_token" {
					http.SetCookie(w, cookie)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write(data)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self' https://api.github.com; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	}), nil
}
func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	handler, err := newHandler(env("VERIFIER_URL", "http://tissla-verifier:8080"), client, env("CONTENT_DIR", "content/portfolio"))
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: ":" + env("PORT", "3000"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
