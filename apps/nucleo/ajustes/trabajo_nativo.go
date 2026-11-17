// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"context"
	"os"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// EstadoTrabajoNativo is only local pending metadata, never a key or application
// body. Incierto must be treated as pending even when Pendientes is zero.
type EstadoTrabajoNativo struct {
	Pendientes      int
	Incierto        bool
	Desecho         string
	DesechoAplicado bool
	TodoConDesecho  bool
}

// TrabajoNativo carries one persistent session witness through the existing
// typed broker client. Neither a nil handle nor an old broker permits private IO.
type TrabajoNativo struct {
	cliente clienteTrabajoBroker
	abrir   func(context.Context, string) (*os.File, error)
	cuenta  string
	acceso  AccesoSesion
}

// ArrendamientoTrabajoNativo retains a broker ticket only while its kernel
// description remains open. Tickets cannot be constructed by other packages.
type ArrendamientoTrabajoNativo struct {
	mu        sync.Mutex
	trabajo   *TrabajoNativo
	exclusivo bool
	permiso   string
	soltar    func()
	cerrado   bool
}

// OperacionTrabajoNativo is created only after the broker persisted dirty state.
// A failed or interrupted report retains that dirty state across process death.
type OperacionTrabajoNativo struct {
	trabajo *TrabajoNativo
	ambito  storage.Scope
	lease   *ArrendamientoTrabajoNativo
	id      string
}

// TrabajoSesionNativo returns handled=false only for a platform without this
// protocol or an explicitly known old broker. A failed new broker returns error.
func TrabajoSesionNativo(ctx context.Context, cuenta string, acceso AccesoSesion) (*TrabajoNativo, bool, error) {
	p := pedidoTrabajoBroker{Version: 1, Cuenta: cuenta, Acceso: &acceso}
	if err := p.validar("resumir"); err != nil {
		return nil, true, err
	}
	client, native := clienteTrabajoPlataforma()
	if !native {
		return nil, false, nil
	}
	if client == nil {
		return nil, true, ErrProtocoloTrabajoSesion
	}
	return nuevoTrabajoNativo(ctx, *client, cuenta, acceso)
}

func nuevoTrabajoNativo(ctx context.Context, client clienteTrabajoBroker, cuenta string, acceso AccesoSesion) (*TrabajoNativo, bool, error) {
	t := &TrabajoNativo{cliente: client, abrir: descriptorTrabajoPlataforma, cuenta: cuenta, acceso: acceso}
	_, native, err := t.cliente.aplicar(ctx, "resumir", t.pedido())
	if err != nil || !native {
		return nil, native, err
	}
	return t, true, nil
}

func (t *TrabajoNativo) pedido() pedidoTrabajoBroker {
	return pedidoTrabajoBroker{Version: 1, Cuenta: t.cuenta, Acceso: &t.acceso}
}

func (t *TrabajoNativo) ejecutar(ctx context.Context, action string, p pedidoTrabajoBroker) (resultadoTrabajoBroker, error) {
	result, native, err := t.cliente.aplicar(ctx, action, p)
	if !native {
		return resultadoTrabajoBroker{}, ErrProtocoloTrabajoSesion
	}
	return result, err
}

func estadoNativo(work *registroTrabajoBroker) EstadoTrabajoNativo {
	return EstadoTrabajoNativo{Pendientes: work.Pendientes, Incierto: work.Incierto, Desecho: work.Desecho, DesechoAplicado: work.DesechoAplicado, TodoConDesecho: work.TodoConDesecho}
}

// Resumen reads this package's scope, all packages, or all except this package.
// Scope=nil requests the complete identity; the broker never substitutes zero
// after a malformed or missing reply.
func (t *TrabajoNativo) Resumen(ctx context.Context, scope *storage.Scope, otros bool) (EstadoTrabajoNativo, error) {
	p := t.pedido()
	p.Otros = otros
	if scope != nil {
		fixedScope := *scope
		p.Ambito = &fixedScope
	}
	r, err := t.ejecutar(ctx, "resumir", p)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	return estadoNativo(r.Trabajo), nil
}

func (t *TrabajoNativo) Arrendar(ctx context.Context, exclusive bool) (*ArrendamientoTrabajoNativo, error) {
	p := t.pedido()
	p.Exclusiva = &exclusive
	r, native, err := t.cliente.arrendar(ctx, p, t.abrir)
	if err != nil {
		return nil, err
	}
	if !native {
		return nil, ErrProtocoloTrabajoSesion
	}
	return &ArrendamientoTrabajoNativo{trabajo: t, exclusivo: exclusive, permiso: r.permiso, soltar: r.soltar}, nil
}

// Cerrar always releases the kernel lease. An RPC cleanup failure cannot erase
// durable dirty records or leave a dead caller's descriptor blocking recovery.
func (l *ArrendamientoTrabajoNativo) Cerrar() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.cerrado {
		l.mu.Unlock()
		return
	}
	l.cerrado = true
	release := l.soltar
	l.mu.Unlock()
	if release != nil {
		release()
	}
}

func (t *TrabajoNativo) exigirExclusiva(lease *ArrendamientoTrabajoNativo) (string, error) {
	if lease == nil {
		return "", ErrProtocoloTrabajoSesion
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.trabajo != t || !lease.exclusivo || lease.cerrado || lease.soltar == nil || !revisionDrive.MatchString(lease.permiso) {
		return "", ErrProtocoloTrabajoSesion
	}
	return lease.permiso, nil
}

func (t *TrabajoNativo) exigirCompartida(lease *ArrendamientoTrabajoNativo) error {
	if lease == nil {
		return ErrProtocoloTrabajoSesion
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.trabajo != t || lease.exclusivo || lease.cerrado || lease.soltar == nil {
		return ErrProtocoloTrabajoSesion
	}
	return nil
}

// Iniciar requires the caller's SH kernel lease. It returns only after the broker
// durably marks the operation dirty; application/WAL IO must start afterward.
func (t *TrabajoNativo) Iniciar(ctx context.Context, scope storage.Scope, lease *ArrendamientoTrabajoNativo) (*OperacionTrabajoNativo, error) {
	if err := t.exigirCompartida(lease); err != nil {
		return nil, err
	}
	p := t.pedido()
	p.Ambito = &scope
	r, err := t.ejecutar(ctx, "iniciar", p)
	if err != nil {
		return nil, err
	}
	return &OperacionTrabajoNativo{trabajo: t, ambito: scope, lease: lease, id: r.Operacion}, nil
}

// Terminar runs before releasing SH and never acquires another SH lease. A failed
// metadata scan must set failed=true and cannot report a successful zero count.
func (o *OperacionTrabajoNativo) Terminar(ctx context.Context, pending int, failed bool) (EstadoTrabajoNativo, error) {
	if o == nil || o.trabajo == nil || !revisionDrive.MatchString(o.id) {
		return EstadoTrabajoNativo{}, ErrProtocoloTrabajoSesion
	}
	if err := o.trabajo.exigirCompartida(o.lease); err != nil {
		return EstadoTrabajoNativo{}, err
	}
	p := o.trabajo.pedido()
	scope := o.ambito
	p.Ambito = &scope
	p.Operacion = o.id
	p.Fallo = failed
	if !failed {
		p.Pendientes = &pending
	}
	r, err := o.trabajo.ejecutar(ctx, "terminar", p)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	return estadoNativo(r.Trabajo), nil
}

// Reconciliar may lower a conservative count only under EX, after reading stable
// metadata from this UID's exact owner scope. It never clears another UID's WAL.
func (t *TrabajoNativo) Reconciliar(ctx context.Context, scope storage.Scope, lease *ArrendamientoTrabajoNativo, pending int) (EstadoTrabajoNativo, error) {
	permit, err := t.exigirExclusiva(lease)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	p := t.pedido()
	p.Ambito = &scope
	p.Permiso = permit
	p.Pendientes = &pending
	r, err := t.ejecutar(ctx, "reconciliar", p)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	return estadoNativo(r.Trabajo), nil
}

// PedirDesecho writes tombstones for every pending UID. Counts remain pending
// until the owner UID applies its tombstone before WAL access and acknowledges.
func (t *TrabajoNativo) PedirDesecho(ctx context.Context, lease *ArrendamientoTrabajoNativo) (EstadoTrabajoNativo, error) {
	permit, err := t.exigirExclusiva(lease)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	p := t.pedido()
	p.Permiso = permit
	r, err := t.ejecutar(ctx, "desechar", p)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	return estadoNativo(r.Trabajo), nil
}

// ConfirmarDesecho succeeds only after this UID's requested rows and staged data
// were removed and their absence verified. Failure leaves the durable tombstone.
func (t *TrabajoNativo) ConfirmarDesecho(ctx context.Context, scope storage.Scope, lease *ArrendamientoTrabajoNativo, discard string, pending int, failed bool) (EstadoTrabajoNativo, error) {
	permit, err := t.exigirExclusiva(lease)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	if !failed && pending != 0 {
		return EstadoTrabajoNativo{}, ErrProtocoloTrabajoSesion
	}
	p := t.pedido()
	p.Ambito = &scope
	p.Permiso = permit
	p.Desecho = discard
	p.Fallo = failed
	if !failed {
		p.Pendientes = &pending
	}
	r, err := t.ejecutar(ctx, "confirmar-desecho", p)
	if err != nil {
		return EstadoTrabajoNativo{}, err
	}
	return estadoNativo(r.Trabajo), nil
}
