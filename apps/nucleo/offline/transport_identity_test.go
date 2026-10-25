// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

func identityTokens(subject, tenant, token string) auth.Tokens {
	return auth.Tokens{
		Issuer:       "https://issuer.invalid",
		Cliente:      "native",
		Subject:      subject,
		Organizacion: tenant,
		Access:       token,
		Refresh:      "synthetic-refresh-" + token,
		Expiry:       time.Now().Add(time.Hour),
	}
}

// The API client has already obtained Authorization A, but a wrapper delays entry
// to the cache transport until Session has adopted B. No token values enter logs.
func TestAuthorizationBeforeCacheEntryCannotCrossOwner(t *testing.T) {
	for _, mode := range []string{"una", "stream", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			store, _ := abrir(t)
			a := identityTokens("ada", "first", "synthetic-a")
			b := identityTokens("grace", "second", "synthetic-b")
			session := auth.NewSession(a, nil, nil)
			owner := func() string { return session.Subject() + "/" + session.Organizacion() }
			baseErr := errors.New("synthetic network failure")
			cache := &Transport{Store: store, Owner: owner, Product: "mail", Scope: func() string { return "mounted" }}
			cache.Base = streamRoundTrip(func(req *http.Request) (*http.Response, error) {
				if mode == "fallback" {
					return nil, baseErr
				}
				if req.Header.Get("Authorization") != "Bearer "+a.Access {
					return nil, errors.New("request authorization was unexpectedly changed")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("account A")), ContentLength: 9, Request: req}, nil
			})
			const url = "https://service.invalid/api/list"
			if mode == "fallback" {
				if err := store.Write("grace/second", espacioHTTP("mail"), url, respuestaGuardada{Status: http.StatusOK, Header: make(http.Header), Body: []byte("account B")}); err != nil {
					t.Fatal(err)
				}
			}
			entered, resume := make(chan struct{}), make(chan struct{})
			// A real http.Client wrapper must not strip the request's opaque scope.
			client := api.New("https://service.invalid", session, &http.Client{Transport: streamRoundTrip(func(req *http.Request) (*http.Response, error) {
				close(entered)
				<-resume
				return cache.RoundTrip(req)
			})})
			type result struct {
				response *http.Response
				err      error
			}
			done := make(chan result, 1)
			go func() {
				var response *http.Response
				var err error
				if mode == "stream" {
					response, err = client.Stream(context.Background(), http.MethodGet, "/api/list", nil, nil, nil, 0)
				} else {
					response, err = client.Do(context.Background(), api.Request{Path: "/api/list"})
				}
				done <- result{response, err}
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("API request never reached the barrier")
			}
			session.Sincronizar(b)
			close(resume)
			out := <-done
			if mode == "fallback" {
				if out.response != nil {
					_ = out.response.Body.Close()
					t.Fatal("authorization A received the offline response of account B")
				}
				if !errors.Is(out.err, baseErr) {
					t.Fatal("network failure was not preserved")
				}
				return
			}
			if out.err != nil {
				t.Fatal(out.err)
			}
			body, err := io.ReadAll(out.response.Body)
			closed := out.response.Body.Close()
			if err != nil || closed != nil || string(body) != "account A" {
				t.Fatal("network response was disturbed", err, closed)
			}
			var saved respuestaGuardada
			if ok, err := store.Read("grace/second", espacioHTTP("mail"), url, &saved); err != nil || ok {
				t.Fatal("authorization A was persisted under account B", ok, err)
			}
		})
	}
}

type changeOnClose struct {
	io.Reader
	change func()
}

func (c changeOnClose) Close() error { c.change(); return nil }

func TestScopedBodyRequiresCurrentAuthorizationAtEOFAndClose(t *testing.T) {
	for _, stage := range []string{"read", "close", "aba", "header", "removed-header"} {
		t.Run(stage, func(t *testing.T) {
			store, _ := abrir(t)
			a, b := identityTokens("ada", "first", "synthetic-a"), identityTokens("grace", "second", "synthetic-b")
			session := auth.NewSession(a, nil, nil)
			cache := &Transport{Store: store, Owner: func() string { return session.Subject() + "/" + session.Organizacion() }, Product: "mail"}
			cache.Base = streamRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := changeOnClose{Reader: strings.NewReader("account A"), change: func() {
					if stage == "close" || stage == "aba" {
						session.Sincronizar(b)
					}
					if stage == "aba" {
						session.Sincronizar(a)
					}
					if stage == "header" {
						req.Header.Set("Authorization", "Bearer replaced")
					}
					if stage == "removed-header" {
						req.Header.Del("Authorization")
					}
				}}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, ContentLength: 9, Request: req}, nil
			})
			client := api.New("https://service.invalid", session, &http.Client{Transport: cache})
			response, err := client.Do(context.Background(), api.Request{Path: "/api/list"})
			if err != nil {
				t.Fatal(err)
			}
			if stage == "read" {
				session.Sincronizar(b)
			}
			body, err := io.ReadAll(response.Body)
			closed := response.Body.Close()
			if err != nil || closed != nil || string(body) != "account A" {
				t.Fatal("network stream changed", err, closed)
			}
			for _, owner := range []string{"ada/first", "grace/second"} {
				var saved respuestaGuardada
				if ok, err := store.Read(owner, espacioHTTP("mail"), "https://service.invalid/api/list", &saved); err != nil || ok {
					t.Fatal("superseded authorization committed a body", stage, ok, err)
				}
			}
		})
	}
}

func TestScoped401RetryAndOfflineFallbackKeepTheirOwnAuthorization(t *testing.T) {
	store, _ := abrir(t)
	a := identityTokens("ada", "first", "synthetic-a")
	renewed := a
	renewed.Access = "synthetic-renewed"
	renewals := 0
	session := auth.NewSession(a, func(context.Context, auth.Tokens) (auth.Tokens, error) { renewals++; return renewed, nil }, nil)
	cache := &Transport{Store: store, Owner: func() string { return session.Subject() + "/" + session.Organizacion() }, Product: "mail"}
	var scopes []*auth.AuthorizationScope
	cache.Base = streamRoundTrip(func(req *http.Request) (*http.Response, error) {
		scopes = append(scopes, auth.RequestAuthorizationScope(req.Context()))
		status, text := http.StatusOK, "account A"
		if req.Header.Get("Authorization") == "Bearer "+a.Access {
			status, text = http.StatusUnauthorized, "expired"
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(text)), ContentLength: int64(len(text)), Request: req}, nil
	})
	client := api.New("https://service.invalid", session, &http.Client{Transport: cache})
	response, err := client.Do(context.Background(), api.Request{Path: "/api/list"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(body) != "account A" || renewals != 1 || len(scopes) != 2 || scopes[0] == scopes[1] || scopes[0].Current("Bearer "+a.Access) || !scopes[1].Current("Bearer "+renewed.Access) {
		t.Fatal("401 retry did not acquire its own current witness", err)
	}
	const id = "https://service.invalid/api/list"
	encoded, err := store.b.get(clave("ada/first", espacioHTTP("mail"), id))
	if err != nil || strings.Contains(string(encoded), a.Access) || strings.Contains(string(encoded), renewed.Access) {
		t.Fatal("authorization metadata entered persistent storage", err)
	}
	cache.Base = streamRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("synthetic offline") })
	// A stale incoming context must be replaced by the new acquisition on both paths.
	ctx := auth.WithAuthorizationScope(context.Background(), scopes[0])
	for _, mode := range []string{"una", "stream"} {
		if mode == "stream" {
			response, err = client.Stream(ctx, http.MethodGet, "/api/list", nil, nil, nil, 0)
		} else {
			response, err = client.Do(ctx, api.Request{Path: "/api/list"})
		}
		if err != nil {
			t.Fatal(mode, err)
		}
		body, err = io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || string(body) != "account A" || response.Header.Get("X-Sytue-Offline") != "cache" {
			t.Fatal("valid same-owner fallback was lost", mode, err)
		}
	}
}

type unscopedSession struct{ api.Session }

func TestLegacyAuthorizationKeepsHTTPButNeverUsesPrivateCache(t *testing.T) {
	store, _ := abrir(t)
	a := identityTokens("ada", "first", "synthetic-a")
	session := unscopedSession{auth.NewSession(a, nil, nil)}
	cache := &Transport{Store: store, Owner: func() string { return "ada/first" }, Product: "mail"}
	cache.Base = streamRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer "+a.Access {
			return nil, errors.New("legacy HTTP authorization changed")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("account A")), ContentLength: 9, Request: req}, nil
	})
	client := api.New("https://service.invalid", session, &http.Client{Transport: cache})
	response, err := client.Do(context.Background(), api.Request{Path: "/api/list"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(body) != "account A" {
		t.Fatal("legacy network response changed", err)
	}
	var saved respuestaGuardada
	const id = "https://service.invalid/api/list"
	if ok, err := store.Read("ada/first", espacioHTTP("mail"), id, &saved); err != nil || ok {
		t.Fatal("unscoped response was cached", ok, err)
	}
	if err := store.Write("ada/first", espacioHTTP("mail"), id, respuestaGuardada{Status: http.StatusOK, Header: make(http.Header), Body: []byte("cached")}); err != nil {
		t.Fatal(err)
	}
	networkErr := errors.New("synthetic offline")
	cache.Base = streamRoundTrip(func(*http.Request) (*http.Response, error) { return nil, networkErr })
	response, err = client.Do(context.Background(), api.Request{Path: "/api/list"})
	if response != nil || !errors.Is(err, networkErr) {
		t.Fatal("unscoped authorization used private fallback")
	}
}
