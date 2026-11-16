// SPDX-License-Identifier: Apache-2.0
package sesion

import (
	"context"
	"errors"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ArrendamientoTrabajoPrivado binds a single application cycle to the persistent
// native witness. A new Android broker always supplies durable dirty/reporting;
// no nil callback or per-app file can substitute for that authority.
func ArrendamientoTrabajoPrivado(witness auth.TestigoTrabajo, scope storage.Scope) offline.WorkLease {
	var initialize sync.Mutex
	var acquire offline.WorkLease
	return func(ctx context.Context) (func(), error) {
		if !scope.Valid() {
			return nil, auth.ErrTrabajoSesion
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := witness.Validar(); err != nil {
			return nil, err
		}
		initialize.Lock()
		if acquire == nil {
			control, native, err := ajustes.TrabajoSesionNativo(ctx, witness.Almacen, witness.Acceso)
			if err != nil {
				initialize.Unlock()
				return nil, err
			}
			if native && control == nil {
				initialize.Unlock()
				return nil, ajustes.ErrProtocoloTrabajoSesion
			}
			if native {
				acquire = control.ArrendamientoPrivado(scope, metadatosPrivadosUID())
			} else {
				acquire = witness.Compartido
			}
		}
		fixed := acquire
		initialize.Unlock()
		return fixed(ctx)
	}
}

// ArrendamientoTrabajoPrivado checks the original identity around the shared
// lease. It does not retain a Session mutex while waiting for work or doing IO.
func (s *Suite) ArrendamientoTrabajoPrivado(witness auth.TestigoTrabajo, scope storage.Scope) offline.WorkLease {
	acquire := ArrendamientoTrabajoPrivado(witness, scope)
	check := func() error {
		if s == nil || s.Sesion == nil || s.almacen == nil || witness.Almacen != s.almacen.Identificador() || !scope.Valid() {
			return auth.ErrTrabajoSesion
		}
		identity := s.Sesion.SnapshotActionIdentity().Identity
		if identity.Subject != scope.Subject || identity.Organization != scope.Tenant || identity.IdentidadSesion() != witness.Acceso.Identidad {
			return auth.ErrTrabajoSesion
		}
		return nil
	}
	return func(ctx context.Context) (func(), error) {
		if err := check(); err != nil {
			return nil, err
		}
		release, err := acquire(ctx)
		if err != nil {
			return nil, err
		}
		if err := check(); err != nil {
			release()
			return nil, err
		}
		return release, nil
	}
}

func metadatosPrivadosUID() ajustes.MetadatosTrabajoNativo {
	return ajustes.MetadatosTrabajoNativo{Contar: contarAmbitoPrivadoUID, Desechar: descartarAmbitoPrivadoUID}
}

func namespacesPrivadosUID(scope storage.Scope, staging bool) []string {
	namespaces := storage.PrivateJournalNamespaces(scope.Product)
	if staging {
		namespaces = append(namespaces, storage.PrivateStagingNamespaces(scope.Product)...)
	}
	return append(namespaces, storage.PendingUploadsNamespace)
}

func contarAmbitoPrivadoUID(scope storage.Scope) (int, error) {
	if !scope.Valid() {
		return 0, auth.ErrTrabajoSesion
	}
	dir, err := directorioAlmacen()
	if err != nil {
		return 0, err
	}
	owner := scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product
	return offline.CountDirectoryOwner(dir, owner, namespacesPrivadosUID(scope, false)...)
}

func descartarAmbitoPrivadoUID(ctx context.Context, scope storage.Scope) error {
	if !scope.Valid() {
		return auth.ErrTrabajoSesion
	}
	dir, err := directorioAlmacen()
	if err != nil {
		return err
	}
	owner := scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product
	namespaces := namespacesPrivadosUID(scope, true)
	if err := offline.DiscardDirectoryOwnerContext(ctx, dir, owner, namespaces...); err != nil {
		return err
	}
	count, err := offline.CountDirectoryOwner(dir, owner, namespaces...)
	if err != nil || count != 0 {
		return errors.Join(ErrTrabajoPendiente, err)
	}
	return ctx.Err()
}

// pendientesTrabajoNativo reads only broker metadata. UI snapshots have no
// operation context and bound this local query to one second.
//
//nolint:contextcheck // Native UI counters have an independent bounded metadata deadline.
func (s *Suite) pendientesTrabajoNativo(identity auth.IdentitySnapshot) (int, bool, error) {
	if s.almacen == nil || identity.IdentidadSesion() == "" {
		return 0, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	access, err := ajustes.LeerAccesoSesionCon(ctx, s.almacen.Identificador())
	if err != nil {
		return 0, true, err
	}
	if !access.Activo || access.Identidad != identity.IdentidadSesion() {
		return 0, true, auth.ErrTrabajoSesion
	}
	control, native, err := ajustes.TrabajoSesionNativo(ctx, s.almacen.Identificador(), access)
	if err != nil || !native {
		return 0, native, err
	}
	state, err := control.Resumen(ctx, nil, identity.Organization != "")
	count := state.Pendientes
	if state.Incierto && count == 0 {
		count = 1
	}
	return count, true, err
}
