package main

import (
	"flag"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

var hosts = map[string]string{
	"authserver":        "authserver.mojang.com",
	"sessionserver":     "sessionserver.mojang.com",
	"api":               "api.mojang.com",
	"minecraftservices": "api.minecraftservices.com",
}

// The explicit service prefix also supports upstream paths unknown to this binary.
func route(path string) (host, upstreamPath string) {
	first, rest, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if host := hosts[first]; host != "" {
		return host, "/" + rest
	}
	switch first {
	case "authenticate", "refresh", "validate", "signout", "invalidate":
		host = hosts["authserver"]
	case "session", "blockedservers":
		host = hosts["sessionserver"]
	case "profiles", "users", "user":
		host = hosts["api"]
	case "player", "publickeys", "minecraft", "minecraftservices-internal", "entitlements":
		host = hosts["minecraftservices"]
	}
	return host, path
}

type responseLog struct {
	http.ResponseWriter
	status int
	err    error
}

func (w *responseLog) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseLog) WriteHeader(code int) {
	if code >= 200 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	publicURL := flag.String("public-url", "", "external HTTP(S) base URL for discovery endpoints (default: incoming HTTP host)")
	flag.Parse()
	base := strings.TrimRight(*publicURL, "/")
	if *publicURL != "" {
		u, err := url.Parse(base)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			logger.Print("ERROR  invalid -public-url")
			os.Exit(2)
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.ResponseHeaderTimeout = 30 * time.Second
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(p *httputil.ProxyRequest) {
			// The upstream HTTPS connection negotiates HTTP/2 independently.
			p.Out.Header.Del("Upgrade")
			p.Out.Header.Del("Connection")
			p.Out.Header.Del("HTTP2-Settings")
			host, path := route(p.In.URL.EscapedPath())
			p.Out.URL.Scheme = "https"
			p.Out.URL.Host = host
			p.Out.Host = host
			p.Out.URL.RawQuery = p.In.URL.RawQuery
			// Strip only a literal routing prefix, preserving escaped separators.
			if path != p.In.URL.EscapedPath() {
				_, rest, _ := strings.Cut(strings.TrimPrefix(p.In.URL.Path, "/"), "/")
				p.Out.URL.Path = "/" + rest
				p.Out.URL.RawPath = path
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.(*responseLog).err = err
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		host, path := route(r.URL.EscapedPath())
		out := &responseLog{ResponseWriter: w}
		complete := false
		defer func() {
			logRequest(out, complete, r.Method, ip, host, path, time.Since(start))
		}()
		if r.URL.EscapedPath() == discoveryPath {
			host = discoveryHost
			origin := base
			if origin == "" {
				origin = "http://" + r.Host
			}
			if err := serveDiscovery(out, r, origin); err != nil {
				out.err = err
				if out.status == 0 {
					http.Error(out, "Bad Gateway", http.StatusBadGateway)
				}
				return
			}
		} else if host == "" {
			http.NotFound(out, r)
		} else {
			proxy.ServeHTTP(out, r)
		}
		complete = true
	})
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	logger.Printf("%s  Listening on %s", label("MojProxy", "36"), *listen)
	if err := server.ListenAndServe(); err != nil {
		logger.Printf("%s  %q", label("ERROR", "31"), err.Error())
		os.Exit(1)
	}
}
