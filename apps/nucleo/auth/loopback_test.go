// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoopbackOnlyConsumesExactBrowserReturnAndCloses(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var redirect string
	validations := 0
	query, err := WaitLoopback(ctx, LoopbackCallback{Address: "127.0.0.1:0", Path: "/callback/unpredictable", Success: "done", Failure: "failed", Validate: func(q url.Values) error {
		validations++
		if q.Get("state") != "expected" {
			return errors.New("invalid state")
		}
		return nil
	}, Open: func(target string) error {
		redirect = target
		for _, variation := range []string{"path", "host", "valid"} {
			u, _ := url.Parse(target)
			u.RawQuery = "state=expected"
			if variation == "path" {
				u.Path = "/callback/wrong"
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
			if variation == "host" {
				req.Host = "attacker.example"
			}
			response, requestErr := http.DefaultClient.Do(req)
			if requestErr != nil {
				return requestErr
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			want := http.StatusNotFound
			if variation == "valid" {
				want = http.StatusOK
			}
			if response.StatusCode != want {
				t.Errorf("%s status=%d", variation, response.StatusCode)
			}
			if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" {
				t.Error("credential callback can be cached or leaked")
			}
		}
		return nil
	}})
	if err != nil || query.Get("state") != "expected" || validations != 1 {
		t.Fatalf("query=%v validations=%d err=%v", query, validations, err)
	}
	client := &http.Client{Timeout: time.Second}
	if response, err := client.Get(redirect); err == nil {
		_ = response.Body.Close()
		t.Fatal("browser listener survived completion")
	}
}

func TestLoopbackRejectsNonLoopbackAndCleansUpOnBrowserFailure(t *testing.T) {
	opened := false
	options := LoopbackCallback{Address: "0.0.0.0:0", Path: "/return", Open: func(string) error { opened = true; return nil }, Validate: func(url.Values) error { return nil }}
	if _, err := WaitLoopback(t.Context(), options); err == nil || opened {
		t.Fatal("non-loopback listener accepted")
	}
	var target string
	options.Address = "127.0.0.1:0"
	options.Open = func(value string) error { target = value; return errors.New("browser unavailable") }
	if _, err := WaitLoopback(t.Context(), options); err == nil {
		t.Fatal("browser failure was ignored")
	}
	client := &http.Client{Timeout: time.Second}
	if response, err := client.Get(target); err == nil {
		_ = response.Body.Close()
		t.Fatal("listener leaked after browser failure")
	}
}

func TestLoopbackHandlesBrowserKeepAliveAndGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	query, err := WaitLoopback(ctx, LoopbackCallback{
		Address: "127.0.0.1:0",
		Path:    "/auth/callback",
		Success: "Sesión iniciada con éxito",
		Failure: "Error de autenticación",
		Validate: func(q url.Values) error {
			if q.Get("code") != "secret-code" {
				return errors.New("bad code")
			}
			return nil
		},
		Open: func(redirect string) error {
			go func() {
				defer close(done)
				u, _ := url.Parse(redirect)
				u.RawQuery = "code=secret-code&state=123"
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
				req.Header.Set("Connection", "keep-alive")
				resp, reqErr := http.DefaultClient.Do(req)
				if reqErr != nil {
					t.Errorf("request error: %v", reqErr)
					return
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				if resp.StatusCode != http.StatusOK {
					t.Errorf("status=%d", resp.StatusCode)
				}
				if !strings.Contains(string(body), "Sesión iniciada") {
					t.Errorf("expected HTML body with success message, got: %s", string(body))
				}
				if !resp.Close && resp.Header.Get("Connection") != "close" {
					t.Errorf("expected Connection: close (resp.Close=true), got: %v, header: %s", resp.Close, resp.Header.Get("Connection"))
				}
			}()
			return nil
		},
	})
	if err != nil || query.Get("code") != "secret-code" {
		t.Fatalf("unexpected query=%v err=%v", query, err)
	}
	<-done
}

func TestHostPermitido(t *testing.T) {
	cases := []struct {
		host string
		addr string
		want bool
	}{
		{"127.0.0.1:8921", "127.0.0.1:8921", true},
		{"localhost:8921", "127.0.0.1:8921", true},
		{"127.0.0.1", "127.0.0.1:8921", true},
		{"localhost", "127.0.0.1:8921", true},
		{"[::1]:8921", "127.0.0.1:8921", true},
		{"attacker.example:8921", "127.0.0.1:8921", false},
		{"attacker.example", "127.0.0.1:8921", false},
		{"127.0.0.1:9999", "127.0.0.1:8921", false},
	}
	for _, tc := range cases {
		if got := hostPermitido(tc.host, tc.addr); got != tc.want {
			t.Errorf("hostPermitido(%q, %q) = %v; want %v", tc.host, tc.addr, got, tc.want)
		}
	}
}
