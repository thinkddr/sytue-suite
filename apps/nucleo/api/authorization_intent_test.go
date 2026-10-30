// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type intentTransport func(*http.Request) (*http.Response, error)

func (f intentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func intentTokens(subject string) auth.Tokens {
	return auth.Tokens{
		Issuer: "https://id.example", Cliente: "native", Subject: subject, Organizacion: "org-" + subject,
		Access: "synthetic-" + subject, Refresh: "synthetic-refresh-" + subject, Expiry: time.Now().Add(time.Hour),
	}
}

type intentSessionGate struct {
	*auth.Session
	before  bool
	entered chan struct{}
	release chan struct{}
}

func (s intentSessionGate) AccessTokenScoped(ctx context.Context) (string, *auth.AuthorizationScope, error) {
	if s.before {
		close(s.entered)
		<-s.release
		return s.Session.AccessTokenScoped(ctx)
	}
	token, scope, err := s.Session.AccessTokenScoped(ctx)
	close(s.entered)
	<-s.release
	return token, scope, err
}

func credentialActions() map[string]func(context.Context, *Account) error {
	return map[string]func(context.Context, *Account) error{
		"app-password": func(ctx context.Context, a *Account) error {
			_, err := a.CreateAppPassword(ctx, "Thunderbird")
			return err
		},
		"apple-profile":        func(ctx context.Context, a *Account) error { _, err := a.EnlacePerfilApple(ctx, "Apple"); return err },
		"apple-native-profile": func(ctx context.Context, a *Account) error { _, err := a.PerfilApple(ctx, "Mac"); return err },
	}
}

// The real SDK is held before token acquisition or after authorization, then the
// Session changes. Neither path may send a POST with the next account's token.
func TestCredentialIntentRejectsAccountChangeBeforeTokenOrSend(t *testing.T) {
	for name, action := range credentialActions() {
		for _, before := range []bool{true, false} {
			t.Run(name+map[bool]string{true: "/before-token", false: "/before-send"}[before], func(t *testing.T) {
				s := auth.NewSession(intentTokens("a"), nil, nil)
				ctx := s.WithAuthorizationIntent(context.Background())
				gate := intentSessionGate{s, before, make(chan struct{}), make(chan struct{})}
				var once sync.Once
				t.Cleanup(func() { once.Do(func() { close(gate.release) }) })
				var posts atomic.Int32
				a := NewAccount("https://id.example", gate, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
					posts.Add(1)
					return nil, errors.New("unexpected credential POST")
				})})
				done := make(chan error, 1)
				go func() { done <- action(ctx, a) }()
				select {
				case <-gate.entered:
				case <-time.After(time.Second):
					t.Fatal("authorization did not reach the gate")
				}
				s.Sincronizar(intentTokens("b"))
				once.Do(func() { close(gate.release) })
				select {
				case err := <-done:
					if !errors.Is(err, auth.ErrAuthorizationChanged) || posts.Load() != 0 {
						t.Fatalf("credential intent crossed account change: posts=%d, err=%v", posts.Load(), err)
					}
				case <-time.After(time.Second):
					t.Fatal("request did not complete")
				}
			})
		}
	}
}

func TestCredential401NeverRetriesInAnotherAccount(t *testing.T) {
	for name, action := range credentialActions() {
		t.Run(name, func(t *testing.T) {
			s := auth.NewSession(intentTokens("a"), nil, nil)
			ctx := s.WithAuthorizationIntent(context.Background())
			var headers []string
			a := NewAccount("https://id.example", s, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
				headers = append(headers, r.Header.Get("Authorization"))
				s.Sincronizar(intentTokens("b"))
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})})
			if err := action(ctx, a); !errors.Is(err, auth.ErrAuthorizationChanged) || len(headers) != 1 || headers[0] != "Bearer synthetic-a" {
				t.Fatalf("401 changed the credential owner: headers=%v, err=%v", headers, err)
			}
		})
	}
}

func TestCredential401CanRenewTheSameIdentity(t *testing.T) {
	for name, action := range credentialActions() {
		t.Run(name, func(t *testing.T) {
			s := auth.NewSession(intentTokens("a"), func(context.Context, auth.Tokens) (auth.Tokens, error) {
				tokens := intentTokens("a")
				tokens.Access = "synthetic-renewed-a"
				return tokens, nil
			}, nil)
			ctx := s.WithAuthorizationIntent(context.Background())
			var headers []string
			a := NewAccount("https://id.example", s, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
				headers = append(headers, r.Header.Get("Authorization"))
				status := http.StatusOK
				if len(headers) == 1 {
					status = http.StatusUnauthorized
				}
				body := `{"id":"p","password":"synthetic-password","url":"https://id.example/profile/one-use"}`
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})})
			if err := action(ctx, a); err != nil || len(headers) != 2 || headers[1] != "Bearer synthetic-renewed-a" {
				t.Fatalf("same-identity renewal failed: headers=%v, err=%v", headers, err)
			}
		})
	}
}

func TestAnonymousCoordinatesDoNotNeedAnAuthorizationScope(t *testing.T) {
	s := auth.NewSession(intentTokens("a"), nil, nil)
	ctx := s.WithAuthorizationIntent(context.Background())
	h := &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("public coordinates received authorization")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"domain":"example"}`)), Request: r}, nil
	})}
	if data, err := CoordenadasDe(ctx, "https://id.example", h); err != nil || data.Domain != "example" {
		t.Fatalf("anonymous metadata failed with an intent context: %v", err)
	}
}

func TestCredentialIntentFailsClosedForLegacySessions(t *testing.T) {
	s := auth.NewSession(intentTokens("a"), nil, nil)
	ctx := s.WithAuthorizationIntent(context.Background())
	var posts int
	a := NewAccount("https://id.example", &sesionFalsa{tokens: []string{"synthetic-a"}}, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		posts++
		return nil, errors.New("unexpected credential POST")
	})})
	if _, err := a.CreateAppPassword(ctx, "Thunderbird"); !errors.Is(err, auth.ErrAuthorizationChanged) || posts != 0 {
		t.Fatal("a legacy source silently ignored the bound authorization intent")
	}
}

type foreignIntentSession struct{ *auth.Session }

func (s foreignIntentSession) AccessTokenScoped(context.Context) (string, *auth.AuthorizationScope, error) {
	return s.Session.AccessTokenScoped(context.Background()) // A wrapper that lost the intent.
}

func TestCredentialIntentRejectsAValidWitnessFromAnotherSession(t *testing.T) {
	a := auth.NewSession(intentTokens("a"), nil, nil)
	otherTokens := intentTokens("a") // Same identity, different Session and issued token.
	otherTokens.Access, otherTokens.Refresh = "synthetic-other-session", "synthetic-other-refresh"
	b := auth.NewSession(otherTokens, nil, nil)
	var posts int
	client := NewAccount("https://id.example", foreignIntentSession{b}, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		posts++
		return nil, errors.New("unexpected credential POST")
	})})
	if a.SnapshotIdentity() != b.SnapshotIdentity() {
		t.Fatal("this regression must isolate the Session-instance guard")
	}
	if _, err := client.CreateAppPassword(a.WithAuthorizationIntent(context.Background()), "Outlook"); !errors.Is(err, auth.ErrAuthorizationChanged) || posts != 0 {
		t.Fatal("a valid witness from another Session instance was accepted")
	}
}

func TestIntentWithNilSessionNeverSendsPOSTOrStreamEvenWithAnOldWitness(t *testing.T) {
	for _, stream := range []bool{false, true} {
		s := auth.NewSession(intentTokens("a"), nil, nil)
		ctx := s.WithAuthorizationIntent(context.Background())
		token, scope, err := s.AccessTokenScoped(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ctx = auth.WithAuthorizationScope(ctx, scope)
		var sends int
		c := New("https://id.example", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
			sends++
			return nil, errors.New("unexpected unauthenticated send")
		})})
		header := http.Header{"Authorization": {"Bearer " + token}}
		if stream {
			_, err = c.Stream(ctx, http.MethodPost, "/x", nil, header, strings.NewReader("payload"), 7)
		} else {
			_, err = c.Do(ctx, Request{Method: http.MethodPost, Path: "/x", Header: header, Body: []byte("payload")})
		}
		if !errors.Is(err, auth.ErrAuthorizationChanged) || sends != 0 {
			t.Fatal("nil Session bypassed the intent using a supplied header or an old witness")
		}
	}
}
