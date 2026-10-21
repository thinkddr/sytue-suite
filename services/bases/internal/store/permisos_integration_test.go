// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Un grupo de Sytue ID es un principal de la ACL: aplica a quien lo trae en su token,
// dentro de SU organización, y a nadie más.
func TestGruposComoPrincipal(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))
	deVentas := domain.Actor{Sub: "bea", Tenant: "acme", Groups: []string{"soporte", "ventas"}}
	deOtro := domain.Actor{Sub: "cleo", Tenant: "acme", Groups: []string{"dirección"}}
	deFuera := domain.Actor{Sub: "gus", Tenant: "globex", Groups: []string{"ventas"}}

	p, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "group", PrincipalID: "ventas", Role: domain.RolEditor})
	e.ok(err)

	// Quien trae el grupo: edita y ve la base en su lista con ese rol.
	if _, err := e.s.ActualizarRegistro(e.ctx, deVentas, r.ID, r.Rev, dato(e.txt.ID, `"x"`), nil); err != nil {
		t.Errorf("miembro del grupo edita: %v", err)
	}
	if bs, err := e.s.ListarBases(e.ctx, deVentas); err != nil || len(bs) != 1 || bs[0].Role != domain.RolEditor {
		t.Errorf("miembro del grupo lista: %+v %v", bs, err)
	}
	// Pero no es propietaria: ni esquema ni permisos.
	_, err = e.s.ListarPermisos(e.ctx, deVentas, e.base.ID)
	esperar(t, err, domain.ErrSinPermiso, "miembro del grupo lista permisos")

	// La misma persona SIN el grupo en el token, otro grupo, y el mismo nombre de grupo en otra organización: nada.
	for nombre, a := range map[string]domain.Actor{"sin grupos": bea, "otro grupo": deOtro, "mismo nombre en otra organización": deFuera} {
		_, err := e.s.ListarRegistros(e.ctx, a, e.tabl.ID, "", 0, "")
		esperar(t, err, domain.ErrNoExiste, nombre+", leer")
		_, err = e.s.ObtenerBase(e.ctx, a, e.base.ID)
		esperar(t, err, domain.ErrNoExiste, nombre+", obtener")
		if bs, _ := e.s.ListarBases(e.ctx, a); len(bs) != 0 {
			t.Errorf("%s ve la base en su lista", nombre)
		}
	}

	// Las clases no se confunden: un grupo llamado como un sub no es esa persona, ni al revés.
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "group", PrincipalID: "cleo", Role: domain.RolEditor})
	e.ok(err)
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "dirección", Role: domain.RolEditor})
	e.ok(err)
	_, err = e.s.ListarRegistros(e.ctx, deOtro, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "grupo con nombre de sub, y sub con nombre de grupo")

	// Grupo a nivel de tabla: suma en esa tabla y no abre la base.
	t2, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Segunda")
	e.ok(err)
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: t2.ID, PrincipalKind: "group", PrincipalID: "dirección", Role: domain.RolLector})
	e.ok(err)
	if _, err := e.s.ListarRegistros(e.ctx, deOtro, t2.ID, "", 0, ""); err != nil {
		t.Errorf("grupo con acceso a una tabla la lee: %v", err)
	}
	_, err = e.s.ListarRegistros(e.ctx, deOtro, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "grupo de una tabla en la otra")

	// Revocar el grupo corta en el acto.
	e.ok(e.s.RevocarPermiso(e.ctx, ana, e.base.ID, p.ID))
	_, err = e.s.ListarRegistros(e.ctx, deVentas, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "tras revocar el grupo")

	// Entradas hostiles.
	for _, malo := range []string{"", "a\x00b", "a\nb", strings.Repeat("g", 201)} {
		_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "group", PrincipalID: malo, Role: domain.RolLector})
		esperar(t, err, domain.ErrInvalido, "nombre de grupo no válido")
	}
	for _, clase := range []string{"public", "", "GROUP"} {
		_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: clase, PrincipalID: "x", Role: domain.RolLector})
		esperar(t, err, domain.ErrInvalido, "clase de principal desconocida")
	}
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "group", PrincipalID: "ventas", Role: domain.RolPropietario})
	esperar(t, err, domain.ErrInvalido, "propietario por grupo")

	// Un token con miles de grupos no revienta la consulta: se acota.
	muchos := domain.Actor{Sub: "bea", Tenant: "acme", Groups: make([]string, 5000)}
	for i := range muchos.Groups {
		muchos.Groups[i] = strings.Repeat("g", 150)
	}
	if _, err := e.s.ListarBases(e.ctx, muchos); err != nil {
		t.Errorf("actor con muchos grupos: %v", err)
	}
}

// Transferir la propiedad: solo el propietario, a otra persona, y deja traza.
func TestTransferirBase(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, nil)
	e.conceder(bea, domain.RolEditor)

	// Nadie más que la propietaria.
	_, err := e.s.TransferirBase(e.ctx, bea, e.base.ID, "bea")
	esperar(t, err, domain.ErrInvalido, "transferirse a una misma")
	_, err = e.s.TransferirBase(e.ctx, bea, e.base.ID, "cleo")
	esperar(t, err, domain.ErrSinPermiso, "una editora transfiere")
	_, err = e.s.TransferirBase(e.ctx, cleo, e.base.ID, "cleo2")
	esperar(t, err, domain.ErrNoExiste, "sin acceso transfiere")
	_, err = e.s.TransferirBase(e.ctx, gus, e.base.ID, "gus2")
	esperar(t, err, domain.ErrNoExiste, "otra organización transfiere")
	for _, malo := range []string{"", "ana", strings.Repeat("x", 201)} {
		_, err = e.s.TransferirBase(e.ctx, ana, e.base.ID, malo)
		esperar(t, err, domain.ErrInvalido, "destino no válido")
	}
	if b, _ := e.s.ObtenerBase(e.ctx, ana, e.base.ID); b.OwnerSub != "ana" {
		t.Fatalf("un intento fallido cambió la propiedad: %+v", b)
	}

	nombre, err := e.s.TransferirBase(e.ctx, ana, e.base.ID, "bea")
	e.ok(err)
	if nombre != "Clientes" {
		t.Errorf("nombre de la base: %q", nombre)
	}
	// bea es propietaria (y su entrada de editora sobra); ana se queda como editora.
	if b, err := e.s.ObtenerBase(e.ctx, bea, e.base.ID); err != nil || b.Role != domain.RolPropietario || b.OwnerSub != "bea" {
		t.Errorf("la nueva propietaria: %+v %v", b, err)
	}
	ps, err := e.s.ListarPermisos(e.ctx, bea, e.base.ID)
	e.ok(err)
	if len(ps) != 1 || ps[0].PrincipalID != "ana" || ps[0].Role != domain.RolEditor {
		t.Errorf("ACL tras transferir: %+v", ps)
	}
	if _, err := e.s.ActualizarRegistro(e.ctx, ana, r.ID, r.Rev, dato(e.txt.ID, `"x"`), nil); err != nil {
		t.Errorf("la antigua propietaria sigue editando: %v", err)
	}
	_, err = e.s.ListarPermisos(e.ctx, ana, e.base.ID)
	esperar(t, err, domain.ErrSinPermiso, "la antigua propietaria gestiona permisos")
	_, err = e.s.TransferirBase(e.ctx, ana, e.base.ID, "cleo")
	esperar(t, err, domain.ErrSinPermiso, "la antigua propietaria vuelve a transferir")
	esperar(t, e.s.BorrarBase(e.ctx, ana, e.base.ID), domain.ErrSinPermiso, "la antigua propietaria borra la base")

	es, err := e.s.ListarAuditoria(e.ctx, bea, e.base.ID, 0, 50)
	e.ok(err)
	var visto bool
	for _, x := range es {
		if x.Action == "base.transfer" && x.Actor == "ana" && strings.Contains(string(x.Before), `"ana"`) && strings.Contains(string(x.After), `"bea"`) {
			visto = true
		}
	}
	if !visto {
		t.Errorf("la transferencia no está en la traza: %+v", es)
	}
}
