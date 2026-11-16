// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// vidaPantalla binds asynchronous work and callbacks to one mounted presenter.
// It never owns, deletes or closes the repository or its encrypted journal.
type vidaPantalla struct {
	once         sync.Once
	parent       context.Context
	ctx          context.Context
	cancel       context.CancelFunc
	gate         *offline.WorkGate
	work         *notas.LocalWork
	closed       atomic.Bool
	unsubscribe  func()
	dispatch     func(func())
	dispatchWait func(func())
}

func (p *Pantalla) vida() *vidaPantalla {
	v := &p.lifetime
	v.once.Do(func() {
		parent := v.parent
		if parent == nil {
			parent = context.Background()
		}
		v.ctx, v.cancel = context.WithCancel(parent)
		v.gate = offline.NewWorkGate(v.ctx)
		if v.dispatch == nil {
			v.dispatch = fyne.Do
		}
		if v.dispatchWait == nil {
			v.dispatchWait = fyne.DoAndWait
		}
	})
	return v
}

func (p *Pantalla) contextoTrabajo() (context.Context, uint64) {
	v := p.vida()
	if v.work != nil {
		return v.work.Context()
	}
	return v.ctx, 0
}

func (p *Pantalla) vigente() bool {
	v := p.vida()
	ctx, _ := p.contextoTrabajo()
	return !v.closed.Load() && v.ctx.Err() == nil && ctx.Err() == nil
}

func (p *Pantalla) admitir() (context.Context, func(), error) {
	v := p.vida()
	if v.closed.Load() {
		return nil, nil, offline.ErrWorkClosed
	}
	if v.work != nil {
		return v.work.Start()
	}
	return v.gate.Start(v.ctx)
}

// iniciar admits before launching. The caller evaluates and captures each argument
// before this call, just as Go does before a go statement.
func (p *Pantalla) iniciar(fn func()) bool {
	_, done, err := p.admitir()
	if err != nil {
		return false
	}
	go func() { defer done(); fn() }()
	return true
}

func (p *Pantalla) lanzar(fn func() error) { p.iniciar(func() { _ = fn() }) }

func (p *Pantalla) enHilo(fn func()) {
	v := p.vida()
	if !p.vigente() {
		return
	}
	_, generation := p.contextoTrabajo()
	v.dispatch(func() {
		_, current := p.contextoTrabajo()
		if p.vigente() && generation == current {
			fn()
		}
	})
}

func (p *Pantalla) enHiloEsperar(fn func()) {
	v := p.vida()
	if !p.vigente() {
		return
	}
	_, generation := p.contextoTrabajo()
	v.dispatchWait(func() {
		_, current := p.contextoTrabajo()
		if p.vigente() && generation == current {
			fn()
		}
	})
}

// Cerrar detaches on the UI thread without waiting for workers that may themselves
// be waiting for the UI. Reusing this presenter for another identity is forbidden.
func (p *Pantalla) Cerrar() {
	v := p.vida()
	if v.closed.Swap(true) {
		return
	}
	v.cancel()
	if v.work != nil {
		v.work.Stop()
	}
	if v.unsubscribe != nil {
		v.unsubscribe()
		v.unsubscribe = nil
	}
	if p.edicion != nil && p.edicion.reloj != nil {
		p.edicion.reloj.Stop()
	}
	p.cancelarMigracion()
	p.olvidarClaveActiva()
}

// Join is called away from the UI before the host retires the repository/SDK.
// A timeout retains the host's retirement barrier and may be retried.
func (p *Pantalla) Join(ctx context.Context) error {
	if work := p.vida().work; work != nil {
		if err := work.Drain(ctx); err != nil {
			return err
		}
	} else if err := p.vida().gate.Drain(ctx); err != nil {
		return err
	}
	for id, d := range p.erroresEdiciones {
		if err := p.escribirEdicion(ctx, id, d); err != nil {
			return err
		}
		delete(p.erroresEdiciones, id)
	}
	// Do not release the presenter's retirement barrier while its only copy of
	// a blocked edit is still in memory. A failed drain may be retried.
	if s := p.edicion; s != nil && s.bloqueoBorrador {
		switch {
		case s.durable != nil:
			if s.borrador == nil || s.durable.Revision != s.borrador.Revision {
				return notas.ErrEditorPending
			}
			durable, err := s.durable.Status()
			if err != nil {
				return err
			}
			if !durable {
				return notas.ErrEditorPending
			}
		case s.borrador == nil || p.Almacen == nil:
			return offline.ErrEncryptedStore
		default:
			stored, found, err := p.leerEdicion(s.borradorID)
			if err != nil {
				return err
			}
			if !found || stored.Revision != s.borrador.Revision {
				return offline.ErrEncryptedStore
			}
		}
	}
	return nil
}
