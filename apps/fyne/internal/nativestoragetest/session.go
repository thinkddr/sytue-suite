// SPDX-License-Identifier: Apache-2.0
package nativestoragetest

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SessionPorts reuses the native Factory tests' synthetic SytueID boundary.
// The persistent witness and its OS leases remain real. Callers isolate the
// preferences directory and supply their own broker; no Matrix verification,
// recovery acknowledgement or storage authority is fabricated here.
type SessionPorts struct {
	CaptureWork     func(context.Context, storage.Scope) (auth.TestigoTrabajo, error)
	RegisterWork    func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error)
	CurrentIdentity func() (auth.IdentitySnapshot, bool)
	BindHTTP        func(context.Context, auth.TestigoTrabajo) (api.Session, error)
}

func NewSessionPorts(ctx context.Context, session *auth.Session, store string, register func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error)) (SessionPorts, error) {
	if session == nil || register == nil {
		return SessionPorts{}, errors.New("isolated synthetic session and explicit broker are required")
	}
	if err := ajustes.PonerAccesoSesionCon(ctx, store, session.SnapshotIdentity().IdentidadSesion(), true); err != nil {
		return SessionPorts{}, err
	}
	return SessionPorts{
		CaptureWork: func(ctx context.Context, _ storage.Scope) (auth.TestigoTrabajo, error) {
			access, err := ajustes.LeerAccesoSesionCon(ctx, store)
			return auth.TestigoTrabajo{Almacen: store, Acceso: access}, errors.Join(err, ctx.Err())
		},
		RegisterWork:    register,
		CurrentIdentity: func() (auth.IdentitySnapshot, bool) { return session.SnapshotIdentity(), session.Subject() != "" },
		BindHTTP:        func(ctx context.Context, _ auth.TestigoTrabajo) (api.Session, error) { return session, ctx.Err() },
	}, nil
}
