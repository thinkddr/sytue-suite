// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type unscopedNativeSession struct{}

func TestTransitionRegistrationUsesOriginalWitness(t *testing.T) {
	f, ctx, _, _ := factoryFixture(t)
	var captured auth.TestigoTrabajo
	f.RegisterTransition = func(_ offline.PrivateWork, witness auth.TestigoTrabajo, _ sesion.PrepararTransicionPrivada) (func(), error) {
		captured = witness
		return func() {}, nil
	}
	s, err := f.Capture(ctx)
	if err != nil {
		t.Fatal(err)
	}
	work := &cycleRepository{scope: s.PrivateStorageScope()}
	unregister, err := s.RegisterTransition(ctx, work, func(context.Context, func() bool, sesion.EventoTransicionPrivada) (func(sesion.EventoTransicionPrivada), error) {
		return func(sesion.EventoTransicionPrivada) {}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	if captured != s.witness {
		t.Fatal("transition registration replaced the original witness")
	}
	work.scope.Subject = "another-subject"
	if _, err := s.RegisterTransition(ctx, work, func(context.Context, func() bool, sesion.EventoTransicionPrivada) (func(sesion.EventoTransicionPrivada), error) {
		return func(sesion.EventoTransicionPrivada) {}, nil
	}); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("transition accepted another scope", err)
	}
}

func (unscopedNativeSession) AccessToken(context.Context) (string, error) { return "synthetic", nil }
func (unscopedNativeSession) Invalidate(string)                           {}

func TestApplicationSessionRequiresEveryWorkAndHTTPHook(t *testing.T) {
	for _, name := range []string{"capture", "register", "identity", "HTTP", "anonymous HTTP", "unscoped HTTP"} {
		t.Run(name, func(t *testing.T) {
			f, ctx, _, devices := factoryFixture(t)
			switch name {
			case "capture":
				f.CaptureWork = nil
			case "register":
				f.RegisterWork = nil
			case "identity":
				f.CurrentIdentity = nil
			case "HTTP":
				f.BindHTTP = nil
			case "anonymous HTTP":
				f.BindHTTP = func(context.Context, auth.TestigoTrabajo) (api.Session, error) {
					var empty *auth.Session
					return empty, nil
				}
			case "unscoped HTTP":
				f.BindHTTP = func(context.Context, auth.TestigoTrabajo) (api.Session, error) { return unscopedNativeSession{}, nil }
			}
			if r, err := f.Open(ctx, devices); r != nil || !errors.Is(err, ErrPrivateResources) {
				t.Fatal("native work bypassed a required binding", name, err)
			}
			if devices.Legacy.preparations+devices.Isolated.preparations != 0 {
				t.Fatal("SDK owner keys used before native work binding")
			}
		})
	}
}

func TestApplicationSessionSurvivesPeerRefreshAndRejectsEveryNewLogin(t *testing.T) {
	for _, name := range []string{"ABA", "same account login", "logout"} {
		t.Run(name, func(t *testing.T) {
			f, ctx, identity, _ := factoryFixture(t)
			session, err := f.Capture(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if auth.RequestAuthorizationIntent(session.Context()) != nil {
				t.Fatal("long lifetime retained short action intent")
			}
			if h, err := SessionHTTP(session.Context()); err != nil || h != session.HTTP() {
				t.Fatal("bound HTTP was not retained", err)
			}
			if _, err := SessionHTTP(t.Context()); !errors.Is(err, ErrPrivateResources) {
				t.Fatal("missing binding fell back to ordinary HTTP", err)
			}
			tokens := factoryTokens("alice")
			tokens.Refresh = "synthetic-refresh-from-other-app"
			factoryAdopt(t, session.witness, identity, tokens)
			if auth.RequestAuthorizationIntent(ctx).Current() {
				t.Fatal("peer refresh did not retire the original short action")
			}
			if err := session.Check(session.Context()); err != nil {
				t.Fatal("peer refresh retired the persisted native login", err)
			}
			scoped, ok := session.HTTP().(interface {
				AccessTokenScoped(context.Context) (string, *auth.AuthorizationScope, error)
			})
			if !ok {
				t.Fatal("native HTTP lost request proof")
			}
			token, proof, err := scoped.AccessTokenScoped(session.Context())
			if err != nil || proof == nil || !proof.Current("Bearer "+token) {
				t.Fatal("peer refresh could not acquire a fresh scope proof", err)
			}
			switch name {
			case "ABA":
				factoryAdopt(t, session.witness, identity, factoryTokens("bob"))
				factoryAdopt(t, session.witness, identity, factoryTokens("alice"))
			case "same account login":
				if err := ajustes.PonerAccesoSesionCon(t.Context(), session.witness.Almacen, "", false); err != nil {
					t.Fatal(err)
				}
				factoryAdopt(t, session.witness, identity, factoryTokens("alice"))
			case "logout":
				factoryAdopt(t, session.witness, identity, auth.Tokens{})
			}
			if err := session.Check(session.Context()); err == nil {
				t.Fatal("earlier presenter adopted a new login")
			}
			if _, err := SessionHTTP(session.Context()); err == nil {
				t.Fatal("new login remained available through an old context")
			}
		})
	}
}

func TestApplicationSessionCancelsOrRejectsRevocationDuringLeaseAcquisition(t *testing.T) {
	for _, name := range []string{"cancel", "revoke"} {
		t.Run(name, func(t *testing.T) {
			f, ctx, identity, _ := factoryFixture(t)
			witness, err := f.CaptureWork(ctx, storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"})
			if err != nil {
				t.Fatal(err)
			}
			release, err := witness.Exclusivo(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var once sync.Once
			unlock := func() { once.Do(release) }
			defer unlock()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			entered := make(chan struct{})
			capture := f.CaptureWork
			f.CaptureWork = func(ctx context.Context, scope storage.Scope) (auth.TestigoTrabajo, error) {
				w, e := capture(ctx, scope)
				close(entered)
				return w, e
			}
			bound := 0
			bind := f.BindHTTP
			f.BindHTTP = func(ctx context.Context, w auth.TestigoTrabajo) (api.Session, error) { bound++; return bind(ctx, w) }
			done := make(chan error, 1)
			go func() { _, err := f.Capture(ctx); done <- err }()
			<-entered
			if name == "cancel" {
				cancel()
			} else {
				factoryAdopt(t, witness, identity, factoryTokens("bob"))
			}
			unlock()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled/revoked lease was adopted")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("lease acquisition ignored cancellation/revocation")
			}
			if bound != 0 {
				t.Fatal("HTTP was bound before the shared lease was acquired and checked")
			}
		})
	}
}

func TestApplicationSessionRejectsRevocationAfterHTTPAcquisition(t *testing.T) {
	f, ctx, identity, devices := factoryFixture(t)
	bind := f.BindHTTP
	f.BindHTTP = func(ctx context.Context, w auth.TestigoTrabajo) (api.Session, error) {
		client, err := bind(ctx, w)
		factoryAdopt(t, w, identity, factoryTokens("bob"))
		factoryAdopt(t, w, identity, factoryTokens("alice"))
		return client, err
	}
	if r, err := f.Open(ctx, devices); r != nil || err == nil {
		t.Fatal("post-acquisition login race adopted a later session", err)
	}
	if devices.Legacy.preparations+devices.Isolated.preparations != 0 {
		t.Fatal("post-acquisition revocation admitted SDK IO")
	}
}
