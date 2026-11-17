// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// TestigoTrabajo is a non-secret witness of exactly one persisted login. Its
// identity hash includes issuer/client/person/organisation; refresh keeps revision.
type TestigoTrabajo struct {
	Almacen string
	Acceso  ajustes.AccesoSesion
}

var ErrTrabajoSesion = errors.New("auth: device work belongs to a previous session")

func (a *AlmacenSesion) publicarAccesoTrabajo(t Tokens) error {
	snapshot := IdentitySnapshot{a.emisor, a.cliente, t.Subject, organizacionDe(t)}
	identity := snapshot.IdentidadSesion()
	active := t.Refresh != "" && identity != ""
	contexto := ""
	if active {
		contexto = string(snapshot.contextoSesion())
	} else {
		identity = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return ajustes.PonerAccesoSesionContextoCon(ctx, a.cuenta, identity, contexto, active)
}

// TestigoTrabajo is captured under the short shared session lock. It never holds
// that lock over application IO or network, and migrates existing native logins.
//
//nolint:contextcheck // Native secret-store callbacks have their own platform deadline.
func (a *AlmacenSesion) TestigoTrabajo(ctx context.Context) (TestigoTrabajo, error) {
	if available, err := ajustes.EstadoTrabajoSesionCompartido(); err != nil || !available {
		return TestigoTrabajo{}, errors.Join(ajustes.ErrTrabajoCompartidoNoDisponible, err)
	}
	release, err := ajustes.CerrojoSesion(ctx)
	if err != nil {
		return TestigoTrabajo{}, err
	}
	defer release()
	tokens, err := a.Cargar()
	if err != nil || tokens.Refresh == "" {
		return TestigoTrabajo{}, errors.Join(ErrTrabajoSesion, err)
	}
	if err := a.publicarAccesoTrabajo(tokens); err != nil {
		return TestigoTrabajo{}, err
	}
	access, err := ajustes.LeerAccesoSesion(a.cuenta)
	if err != nil {
		return TestigoTrabajo{}, err
	}
	witness := TestigoTrabajo{Almacen: a.cuenta, Acceso: access}
	return witness, witness.Validar()
}
func (w TestigoTrabajo) Validar() error       { return w.validar(false) }
func (w TestigoTrabajo) ValidarCierre() error { return w.validar(true) }
func (w TestigoTrabajo) validar(allowPaused bool) error {
	if available, err := ajustes.EstadoTrabajoSesionCompartido(); err != nil || !available {
		return errors.Join(ajustes.ErrTrabajoCompartidoNoDisponible, err)
	}
	actual, err := ajustes.LeerAccesoSesion(w.Almacen)
	if err != nil || !actual.Activo || (!allowPaused && actual.Suspendido) || actual.Identidad != w.Acceso.Identidad || actual.Revision != w.Acceso.Revision {
		return errors.Join(ErrTrabajoSesion, err)
	}
	return nil
}

func (w TestigoTrabajo) Compartido(ctx context.Context) (func(), error) {
	return ajustes.CerrojoTrabajoSesion(ctx, w.Almacen, w.Acceso, false)
}

func (w TestigoTrabajo) Exclusivo(ctx context.Context) (func(), error) {
	return ajustes.CerrojoTrabajoSesion(ctx, w.Almacen, w.Acceso, true)
}

func (w TestigoTrabajo) Pausar(ctx context.Context, paused bool) error {
	if available, err := ajustes.EstadoTrabajoSesionCompartido(); err != nil || !available {
		return errors.Join(ajustes.ErrTrabajoCompartidoNoDisponible, err)
	}
	return ajustes.SuspenderAccesoSesion(ctx, w.Almacen, w.Acceso, paused)
}
