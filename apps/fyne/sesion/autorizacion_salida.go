// SPDX-License-Identifier: Apache-2.0
package sesion

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// AutorizacionSalida captures the account displayed when a native action opens.
// No credential read, token renewal or network request runs on the UI thread.
// A zero value cannot close a session or discard work.
type AutorizacionSalida struct {
	suite      *Suite
	witness    auth.TestigoTrabajo
	action     auth.ActionIdentity
	generation string
	private    bool
}

func (s *Suite) AutorizarSalida() (AutorizacionSalida, error) {
	if s == nil || s.Sesion == nil || s.almacen == nil {
		return AutorizacionSalida{}, auth.ErrTrabajoSesion
	}
	action := s.Sesion.SnapshotActionIdentity()
	access, err := ajustes.LeerAccesoSesion(s.almacen.Identificador())
	if err != nil {
		return AutorizacionSalida{}, err
	}
	captured := AutorizacionSalida{suite: s, witness: auth.TestigoTrabajo{Almacen: s.almacen.Identificador(), Acceso: access}, action: action, generation: ajustes.Leer().Sesion, private: access.Activo && ajustes.TrabajoSesionCompartidoDisponible()}
	if err := captured.Vigente(); err != nil {
		return AutorizacionSalida{}, err
	}
	return captured, nil
}

func (a AutorizacionSalida) Vigente() error {
	if a.suite == nil {
		return auth.ErrTrabajoSesion
	}
	current := a.suite.Sesion.SnapshotActionIdentity()
	if a.private {
		if current.Identity.IdentidadSesion() != a.witness.Acceso.Identidad {
			return auth.ErrTrabajoSesion
		}
		return a.witness.Validar()
	}
	access, err := ajustes.LeerAccesoSesion(a.witness.Almacen)
	if err != nil || access != a.witness.Acceso || ajustes.Leer().Sesion != a.generation {
		return errors.Join(auth.ErrTrabajoSesion, err)
	}
	// A failed legacy persistence intentionally clears RAM. The same retained
	// credential can be retried, while a later interactive adoption is rejected.
	if current.Identity.Subject != "" && current != a.action {
		return auth.ErrTrabajoSesion
	}
	return nil
}

func (a AutorizacionSalida) Cerrar(ctx context.Context, discard bool) error {
	if err := a.Vigente(); err != nil {
		return err
	}
	if a.private {
		return a.suite.salirProtegidoConTestigo(ctx, discard, a.witness)
	}
	return a.suite.salirLegadoAutorizado(ctx, discard, a.Vigente)
}

func (a AutorizacionSalida) sesionVinculada(ctx context.Context) (api.Session, error) {
	if err := a.Vigente(); err != nil {
		return nil, err
	}
	if a.private {
		return a.suite.SesionVinculadaPrivada(ctx, a.witness)
	}
	bound := a.suite.autorizacionDriveSistema(a.action.Identity)
	current := bound.vigente
	bound.vigente = func(job context.Context) bool { return ctx.Err() == nil && a.Vigente() == nil && current(job) }
	return bound, nil
}
