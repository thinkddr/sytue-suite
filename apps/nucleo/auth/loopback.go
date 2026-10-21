// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// LoopbackCallback configures a single browser return, shared by Sytue OIDC and
// Matrix SSO. Address is an explicit loopback IP; port zero requests an ephemeral port.
// Matrix uses an unguessable Path because m.login.token does not echo OAuth state.
type LoopbackCallback struct {
	Address  string
	Path     string
	Open     func(redirect string) error
	Validate func(url.Values) error
	Success  string
	Failure  string
}

func hostPermitido(hostHeader, addr string) bool {
	if hostHeader == addr {
		return true
	}
	reqHost, reqPort, err := net.SplitHostPort(hostHeader)
	if err != nil {
		reqHost = hostHeader
		reqPort = ""
	}
	_, addrPort, _ := net.SplitHostPort(addr)
	if reqPort != "" && addrPort != "" && reqPort != addrPort {
		return false
	}
	return reqHost == "127.0.0.1" || reqHost == "localhost" || reqHost == "::1" || reqHost == "[::1]"
}

const htmlRespuesta = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sytue</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f172a;color:#f8fafc;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.c{background:#1e293b;padding:2.5rem;border-radius:1rem;box-shadow:0 10px 25px -5px rgba(0,0,0,0.4);text-align:center;max-width:440px}
h1{font-size:1.3rem;margin:0 0 .75rem;color:#38bdf8}
p{font-size:.95rem;color:#94a3b8;margin:0 0 1.5rem;line-height:1.5}
.b{display:inline-block;background:#0284c7;color:#fff;text-decoration:none;padding:.6rem 1.25rem;border-radius:.5rem;font-weight:500;font-size:.9rem;cursor:pointer}
</style>
</head>
<body>
<div class="c">
<h1>%s</h1>
<p>%s</p>
<a class="b" onclick="window.close();return false;">Cerrar pestaña</a>
</div>
%s
</body>
</html>`

// WaitLoopback listens only on loopback, opens the browser after binding, and
// closes both server and listener on return. It never logs callback credentials.
func WaitLoopback(ctx context.Context, options LoopbackCallback) (url.Values, error) {
	host, _, err := net.SplitHostPort(options.Address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() ||
		!strings.HasPrefix(options.Path, "/") || strings.ContainsAny(options.Path, "?#\r\n") || options.Open == nil || options.Validate == nil {
		return nil, errors.New("auth: invalid browser callback configuration")
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", options.Address)
	if err != nil {
		return nil, fmt.Errorf("auth: browser callback listener: %w", err)
	}
	type result struct {
		query url.Values
		err   error
	}
	arrived := make(chan result, 1)
	server := &http.Server{
		ReadHeaderTimeout: 30 * time.Second,
		MaxHeaderBytes:    16 << 10,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; frame-ancestors 'none'")
			w.Header().Set("Connection", "close")

			if r.Method != http.MethodGet || r.URL.Path != options.Path || !hostPermitido(r.Host, ln.Addr().String()) {
				http.NotFound(w, r)
				return
			}
			query := r.URL.Query()
			valErr := options.Validate(query)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if valErr != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, htmlRespuesta, "Error de inicio de sesión", options.Failure, "")
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, htmlRespuesta, "Sesión iniciada", options.Success, "<script>setTimeout(function(){window.open('','_self','');window.close();},1200);</script>")
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			select {
			case arrived <- result{query: query, err: valErr}:
			default:
			}
		}),
	}
	go func() { _ = server.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = ln.Close()
	}()
	redirect := (&url.URL{Scheme: "http", Host: ln.Addr().String(), Path: options.Path}).String()
	if err = options.Open(redirect); err != nil {
		return nil, fmt.Errorf("auth: open browser: %w", err)
	}
	select {
	case got := <-arrived:
		return got.query, got.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
