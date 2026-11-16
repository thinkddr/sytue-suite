// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrTrabajoNativoPendiente = errors.New("ajustes: native private work remains pending or uncertain")

// MetadatosTrabajoNativo must inspect or remove only this UID's exact scope.
// Contar reads hashed path metadata, without opening a WAL or decrypting bodies.
// Desechar removes both journals and retained source blocks; it is called only
// for a durable tombstone or the user's explicit discard of this same login.
type MetadatosTrabajoNativo struct {
	Contar   func(storage.Scope) (int, error)
	Desechar func(context.Context, storage.Scope) error
}

func (m MetadatosTrabajoNativo) validar(scope storage.Scope) error {
	if !scope.Valid() || m.Contar == nil || m.Desechar == nil {
		return ErrProtocoloTrabajoSesion
	}
	return nil
}

func (m MetadatosTrabajoNativo) contar(scope storage.Scope) (int, error) {
	count, err := m.Contar(scope)
	if err != nil {
		return count, err
	}
	if count < 0 || count > 1000000 {
		return count, ErrProtocoloTrabajoSesion
	}
	return count, nil
}

// PrepararAmbito verifies stable metadata under EX before any private WAL is
// opened. A prior discard is applied and acknowledged first, including staging.
func (t *TrabajoNativo) PrepararAmbito(ctx context.Context, scope storage.Scope, metadata MetadatosTrabajoNativo) error {
	if err := metadata.validar(scope); err != nil {
		return err
	}
	lease, err := t.Arrendar(ctx, true)
	if err != nil {
		return err
	}
	defer lease.Cerrar()
	return t.prepararAmbitoCon(ctx, scope, metadata, lease)
}

func (t *TrabajoNativo) prepararAmbitoCon(ctx context.Context, scope storage.Scope, metadata MetadatosTrabajoNativo, lease *ArrendamientoTrabajoNativo) error {
	state, err := t.Resumen(ctx, &scope, false)
	if err != nil {
		return err
	}
	if state.Desecho != "" && !state.DesechoAplicado {
		if err := metadata.Desechar(ctx, scope); err != nil {
			return err
		}
		count, err := metadata.contar(scope)
		if err != nil {
			return err
		}
		if count != 0 {
			return ErrTrabajoNativoPendiente
		}
		_, err = t.ConfirmarDesecho(ctx, scope, lease, state.Desecho, 0, false)
		return err
	}
	count, err := metadata.contar(scope)
	if err != nil {
		return err
	}
	if state.Incierto || state.Pendientes != count {
		_, err = t.Reconciliar(ctx, scope, lease, count)
	}
	return err
}

type cicloArrendamientoNativo struct {
	trabajo   *TrabajoNativo
	scope     storage.Scope
	metadata  MetadatosTrabajoNativo
	prepare   sync.Mutex
	preparado bool
	activos   atomic.Int32
}

// ArrendamientoPrivado retains a single prepared scope across nested native jobs.
// Dirty is durable before the returned release can expose application/WAL IO.
// Report cleanup holds the existing SH lease and never takes the OIDC mutex.
func (t *TrabajoNativo) ArrendamientoPrivado(scope storage.Scope, metadata MetadatosTrabajoNativo) func(context.Context) (func(), error) {
	cycle := &cicloArrendamientoNativo{trabajo: t, scope: scope, metadata: metadata}
	return cycle.adquirir
}

func (c *cicloArrendamientoNativo) preparar(ctx context.Context) error {
	c.prepare.Lock()
	defer c.prepare.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.preparado || c.activos.Load() > 0 {
		return nil
	}
	if err := c.trabajo.PrepararAmbito(ctx, c.scope, c.metadata); err != nil {
		return err
	}
	c.preparado = true
	return nil
}

func (c *cicloArrendamientoNativo) adquirir(ctx context.Context) (func(), error) {
	if err := c.metadata.validar(c.scope); err != nil {
		return nil, err
	}
	if err := c.preparar(ctx); err != nil {
		return nil, err
	}
	lease, err := c.compartidoPreparado(ctx)
	if err != nil {
		return nil, err
	}
	operation, err := c.trabajo.Iniciar(ctx, c.scope, lease)
	if err != nil {
		lease.Cerrar()
		c.prepare.Lock()
		c.preparado = false
		c.prepare.Unlock()
		return nil, err
	}
	c.activos.Add(1)
	var once sync.Once
	return func() { once.Do(func() { c.terminar(ctx, lease, operation) }) }, nil
}

func (c *cicloArrendamientoNativo) compartidoPreparado(ctx context.Context) (*ArrendamientoTrabajoNativo, error) {
	for range 2 {
		lease, err := c.trabajo.Arrendar(ctx, false)
		if err != nil {
			return nil, err
		}
		state, err := c.trabajo.Resumen(ctx, &c.scope, false)
		if err != nil {
			lease.Cerrar()
			return nil, err
		}
		if state.Desecho == "" || state.DesechoAplicado {
			return lease, nil
		}
		lease.Cerrar()
		if c.activos.Load() != 0 {
			return nil, ErrTrabajoNativoPendiente
		}
		c.prepare.Lock()
		c.preparado = false
		c.prepare.Unlock()
		if err := c.preparar(ctx); err != nil {
			return nil, err
		}
	}
	return nil, ErrTrabajoNativoPendiente
}

func (c *cicloArrendamientoNativo) terminar(parent context.Context, lease *ArrendamientoTrabajoNativo, operation *OperacionTrabajoNativo) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	count, err := c.metadata.contar(c.scope)
	_, _ = operation.Terminar(ctx, count, err != nil)
	cancel()
	lease.Cerrar()
	if c.activos.Add(-1) != 0 || err != nil {
		return
	}
	// This is after SH, so logout EX never waits for a reporter to reacquire SH.
	// Under contention, a retained count remains conservative until the next EX.
	stable, cancel := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer cancel()
	_ = c.trabajo.PrepararAmbito(stable, c.scope, c.metadata)
}

// PrepararCierre holds EX until the caller commits or aborts credential teardown.
// Other UIDs are represented by durable counts/tombstones, never deleted here.
// A failed attempt returns no lease and leaves credentials and dirty work intact.
func (t *TrabajoNativo) PrepararCierre(ctx context.Context, scopes []storage.Scope, metadata MetadatosTrabajoNativo, discard bool) (func(), error) {
	seen := map[storage.Scope]bool{}
	for _, scope := range scopes {
		if err := metadata.validar(scope); err != nil {
			return nil, err
		}
		if seen[scope] {
			return nil, ErrProtocoloTrabajoSesion
		}
		seen[scope] = true
	}
	if len(scopes) > 256 {
		return nil, ErrProtocoloTrabajoSesion
	}
	lease, err := t.Arrendar(ctx, true)
	if err != nil {
		return nil, err
	}
	if err := t.prepararCierreCon(ctx, scopes, metadata, discard, lease); err != nil {
		lease.Cerrar()
		return nil, err
	}
	return lease.Cerrar, nil
}

func (t *TrabajoNativo) prepararCierreCon(ctx context.Context, scopes []storage.Scope, metadata MetadatosTrabajoNativo, discard bool, lease *ArrendamientoTrabajoNativo) error {
	for _, scope := range scopes {
		state, err := t.Resumen(ctx, &scope, false)
		if err != nil {
			return err
		}
		count, err := metadata.contar(scope)
		if err != nil {
			return err
		}
		// Cleanup remains in the captured credential teardown. A failed revocation
		// must not erase local WAL merely because the close was prepared.
		if (state.Desecho == "" || state.DesechoAplicado) && (state.Incierto || state.Pendientes != count) {
			if _, err := t.Reconciliar(ctx, scope, lease, count); err != nil {
				return err
			}
		}
	}
	total, err := t.Resumen(ctx, nil, false)
	if err != nil {
		return err
	}
	if total.Pendientes == 0 && !total.Incierto {
		return nil
	}
	if !discard {
		return ErrTrabajoNativoPendiente
	}
	requested, err := t.PedirDesecho(ctx, lease)
	if err != nil {
		return err
	}
	if !requested.TodoConDesecho {
		return ErrTrabajoNativoPendiente
	}
	return nil
}
