// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ApplicationSession binds a native presenter's whole lifetime to one persisted
// login incarnation. Refresh preserves it; logout and another login invalidate
// it. The short AuthorizationIntent is used only when this binding is captured.
type ApplicationSession struct {
	leaseOnce          sync.Once
	acquire            offline.WorkLease
	life               context.Context
	identity           auth.IdentitySnapshot
	scope              storage.Scope
	witness            auth.TestigoTrabajo
	current            func() (auth.IdentitySnapshot, bool)
	register           func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error)
	registerTransition func(offline.PrivateWork, auth.TestigoTrabajo, sesion.PrepararTransicionPrivada) (func(), error)
	http               api.Session
}

type applicationSessionKey struct{}

// Capture requires every native work/HTTP hook. There is no local, anonymous or
// nil-lease fallback when the OS cannot provide the suite's shared work witness.
func (f Factory[D]) Capture(ctx context.Context) (*ApplicationSession, error) {
	intent := auth.RequestAuthorizationIntent(ctx)
	owner := intent.Identity()
	scope := storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: f.Product}
	if storage.NativeReturnApp(f.Product, f.NativeConsumer) == "" || !scope.Valid() || owner.Issuer == "" || owner.Client == "" || !f.configured() {
		return nil, ErrPrivateResources
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !intent.Current() {
		return nil, auth.ErrAuthorizationChanged
	}
	witness, err := f.CaptureWork(ctx, scope)
	if err != nil {
		return nil, err
	}
	if !intent.Current() || witness.Acceso.Identidad != owner.IdentidadSesion() {
		return nil, auth.ErrAuthorizationChanged
	}
	// The persisted witness and bound HTTP replace the action intent. Keeping its
	// revision in a long context would reject an ordinary refresh from another app.
	life := auth.WithoutAuthorizationIntent(ctx)
	session := &ApplicationSession{life: life, identity: owner, scope: scope, witness: witness, current: f.CurrentIdentity, register: f.RegisterWork, registerTransition: f.RegisterTransition}
	if err := session.Check(life); err != nil {
		return nil, err
	}
	release, err := session.WorkLease()(life)
	if err != nil {
		return nil, err
	}
	defer release()
	session.http, err = f.BindHTTP(life, witness)
	if err != nil {
		return nil, err
	}
	if nilReference(session.http) || !scopedHTTP(session.http) {
		return nil, ErrPrivateResources
	}
	if err := session.Check(life); err != nil {
		return nil, err
	}
	session.life = context.WithValue(life, applicationSessionKey{}, session)
	return session, nil
}

func (f Factory[D]) configured() bool {
	return f.CaptureWork != nil && f.RegisterWork != nil && f.CurrentIdentity != nil && f.BindHTTP != nil
}

// SessionHTTP is for storage State/Bootstrap callbacks before Resources.Ready.
// A missing or stale native binding is an error; callers never substitute an
// anonymous client or the next account's ordinary shared session.
func SessionHTTP(ctx context.Context) (api.Session, error) {
	session, _ := ctx.Value(applicationSessionKey{}).(*ApplicationSession)
	if session == nil {
		return nil, ErrPrivateResources
	}
	if err := session.Check(ctx); err != nil {
		return nil, err
	}
	return session.HTTP(), nil
}

func scopedHTTP(session api.Session) bool {
	_, ok := session.(interface {
		AccessTokenScoped(context.Context) (string, *auth.AuthorizationScope, error)
	})
	return ok
}

func (s *ApplicationSession) Context() context.Context           { return s.life }
func (s *ApplicationSession) PrivateStorageScope() storage.Scope { return s.scope }
func (s *ApplicationSession) HTTP() api.Session                  { return s.http }
func (s *ApplicationSession) WorkLease() offline.WorkLease {
	s.leaseOnce.Do(func() { s.acquire = offline.ReentrantWorkLease(sesion.ArrendamientoTrabajoPrivado(s.witness, s.scope)) })
	return s.acquire
}

// Check reads the non-secret persistent witness around the atomic identity
// snapshot. It neither acquires authorization nor holds an OIDC lock during IO.
func (s *ApplicationSession) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.life.Err(); err != nil {
		return err
	}
	if err := s.witness.Validar(); err != nil {
		return err
	}
	owner, inside := s.current()
	if !inside || owner != s.identity {
		return auth.ErrAuthorizationChanged
	}
	return s.witness.Validar()
}

func (s *ApplicationSession) Register(ctx context.Context, work offline.PrivateWork) (func(), error) {
	if nilReference(work) || work.PrivateStorageScope() != s.scope {
		return nil, ErrPrivateResources
	}
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	unregister, err := s.register(work, s.witness)
	if err != nil {
		return nil, err
	}
	if unregister == nil {
		return nil, ErrPrivateResources
	}
	if err := s.Check(ctx); err != nil {
		unregister()
		return nil, err
	}
	return unregister, nil
}

// RegisterTransition retains the original work witness; it never recaptures a
// later session with the same subject and organization.
func (s *ApplicationSession) RegisterTransition(ctx context.Context, work offline.PrivateWork, prepare sesion.PrepararTransicionPrivada) (func(), error) {
	if s.registerTransition == nil || nilReference(work) || work.PrivateStorageScope() != s.scope || prepare == nil {
		return nil, ErrPrivateResources
	}
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	unregister, err := s.registerTransition(work, s.witness, prepare)
	if err != nil {
		return nil, err
	}
	if unregister == nil {
		return nil, ErrPrivateResources
	}
	if err := s.Check(ctx); err != nil {
		unregister()
		return nil, err
	}
	return unregister, nil
}
