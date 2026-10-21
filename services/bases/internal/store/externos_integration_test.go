// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

var hal = domain.Actor{Sub: "hal", Tenant: "globex"}

// enAcme es gus atendido en la anfitriona: lo que el servicio construye cuando
// Anfitrion contesta "acme".
var gusEnAcme = domain.Actor{Sub: "gus", Tenant: "acme", Externo: true}

func (e *entorno) invitar(sub string, rol domain.Rol, tabla string) domain.Permiso {
	e.t.Helper()
	p, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: tabla, PrincipalKind: "external", PrincipalID: sub, PrincipalLabel: "N. " + sub, Role: rol})
	e.ok(err)
	return p
}

// Compartir con otra organización: la base se queda en la anfitriona, la persona de
// fuera entra con el rol de SU entrada y no ve nada más de esa organización.
func TestCompartirConOtraOrganizacion(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))
	otra, err := e.s.CrearBase(e.ctx, ana, "Nóminas", "")
	e.ok(err)
	tOtra, err := e.s.CrearTabla(e.ctx, ana, otra.ID, "T")
	e.ok(err)
	// «Toda la organización» puede leer Nóminas: gus no es de la organización.
	_, err = e.s.ConcederPermiso(e.ctx, ana, otra.ID, domain.Permiso{PrincipalKind: "tenant", PrincipalID: "acme", Role: domain.RolEditor})
	e.ok(err)

	// Sin invitación no hay anfitriona, y aunque alguien fabricara el actor, no hay acceso.
	if h, err := e.s.Anfitrion(e.ctx, gus, "record", r.ID); err != nil || h != "" {
		t.Fatalf("anfitriona sin invitación: %q %v", h, err)
	}
	_, err = e.s.ListarRegistros(e.ctx, gusEnAcme, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "actor de fuera sin invitación")

	p := e.invitar("gus", domain.RolEditor, "")

	// El servicio encuentra la anfitriona por cualquier recurso de la base compartida…
	for clase, id := range map[string]string{"base": e.base.ID, "table": e.tabl.ID, "record": r.ID} {
		if h, err := e.s.Anfitrion(e.ctx, gus, clase, id); err != nil || h != "acme" {
			t.Errorf("anfitriona por %s: %q %v", clase, h, err)
		}
	}
	// …y también por los de OTRA base de acme: existir no concede nada, lo corta la ACL.
	if h, _ := e.s.Anfitrion(e.ctx, gus, "table", tOtra.ID); h != "acme" {
		t.Errorf("anfitriona por un recurso no compartido: %q", h)
	}
	_, err = e.s.ListarRegistros(e.ctx, gusEnAcme, tOtra.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "otra base de la anfitriona, abierta a toda la organización")
	_, err = e.s.ObtenerBase(e.ctx, gusEnAcme, otra.ID)
	esperar(t, err, domain.ErrNoExiste, "obtener otra base de la anfitriona")
	// Una clase de recurso inventada, o quien ya viene como de fuera, no resuelven nada.
	if h, _ := e.s.Anfitrion(e.ctx, gus, "bases; DROP TABLE bases", r.ID); h != "" {
		t.Errorf("clase inventada: %q", h)
	}
	if h, _ := e.s.Anfitrion(e.ctx, gusEnAcme, "record", r.ID); h != "" {
		t.Errorf("un actor de fuera no vuelve a saltar: %q", h)
	}
	// hal, de la misma organización que gus pero sin invitación: nada.
	if h, _ := e.s.Anfitrion(e.ctx, hal, "record", r.ID); h != "" {
		t.Errorf("anfitriona para quien no está invitado: %q", h)
	}

	// En la anfitriona edita registros y comenta (es editor)…
	if _, err := e.s.ActualizarRegistro(e.ctx, gusEnAcme, r.ID, r.Rev, dato(e.txt.ID, `"x"`), nil); err != nil {
		t.Errorf("invitado editor edita: %v", err)
	}
	if _, _, err := e.s.CrearComentario(e.ctx, gusEnAcme, r.ID, "", "hola desde fuera"); err != nil {
		t.Errorf("invitado editor comenta: %v", err)
	}
	// …pero no gestiona nada: ni esquema, ni permisos, ni auditoría, ni propiedad, ni borrado.
	_, err = e.s.CrearCampo(e.ctx, gusEnAcme, e.tabl.ID, domain.EntradaCampo{Name: "X", Kind: domain.TipoTexto})
	esperar(t, err, domain.ErrSinPermiso, "invitado crea campo")
	_, err = e.s.ListarPermisos(e.ctx, gusEnAcme, e.base.ID)
	esperar(t, err, domain.ErrSinPermiso, "invitado lista permisos")
	_, err = e.s.ConcederPermiso(e.ctx, gusEnAcme, e.base.ID, domain.Permiso{PrincipalKind: "external", PrincipalID: "hal", Role: domain.RolEditor})
	esperar(t, err, domain.ErrSinPermiso, "invitado invita a otro")
	_, err = e.s.ListarAuditoria(e.ctx, gusEnAcme, e.base.ID, 0, 10)
	esperar(t, err, domain.ErrSinPermiso, "invitado lee la auditoría")
	_, err = e.s.TransferirBase(e.ctx, gusEnAcme, e.base.ID, "hal")
	esperar(t, err, domain.ErrSinPermiso, "invitado transfiere")
	esperar(t, e.s.BorrarBase(e.ctx, gusEnAcme, e.base.ID), domain.ErrSinPermiso, "invitado borra la base")

	// Su token de globex, sin pasar por la anfitriona, no abre nada; y un actor de acme
	// con su sub pero NO marcado como de fuera, tampoco (la entrada es de fuera).
	_, err = e.s.ListarRegistros(e.ctx, gus, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "con su organización")
	_, err = e.s.ListarRegistros(e.ctx, domain.Actor{Sub: "gus", Tenant: "acme"}, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "con la anfitriona pero sin ser de fuera")

	// En su lista sale la base compartida, con la anfitriona y su rol; no «Nóminas».
	bs, err := e.s.ListarBases(e.ctx, gus)
	e.ok(err)
	if len(bs) != 1 || bs[0].ID != e.base.ID || bs[0].HostOrg != "acme" || bs[0].Role != domain.RolEditor {
		t.Errorf("lista del invitado: %+v", bs)
	}
	if bs, _ := e.s.ListarBases(e.ctx, hal); len(bs) != 0 {
		t.Errorf("lista de quien no está invitado: %+v", bs)
	}
	if bs, _ := e.s.ListarBases(e.ctx, ana); len(bs) != 2 || bs[0].HostOrg != "" || bs[1].HostOrg != "" {
		t.Errorf("la lista de la propietaria no cambia: %+v", bs)
	}

	// La invitación y lo que hace el invitado quedan en la traza de la anfitriona.
	es, err := e.s.ListarAuditoria(e.ctx, ana, e.base.ID, 0, 100)
	e.ok(err)
	var invitado, edito bool
	for _, x := range es {
		invitado = invitado || (x.Action == "permission.grant" && x.Actor == "ana")
		edito = edito || (x.Action == "record.update" && x.Actor == "gus")
	}
	if !invitado || !edito {
		t.Errorf("traza: invitación=%v edición del invitado=%v", invitado, edito)
	}

	// Revocar corta EN EL ACTO, aunque el servicio aún recuerde la anfitriona.
	e.ok(e.s.RevocarPermiso(e.ctx, ana, e.base.ID, p.ID))
	_, err = e.s.ListarRegistros(e.ctx, gusEnAcme, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "tras revocar")
	if bs, _ := e.s.ListarBases(e.ctx, gus); len(bs) != 0 {
		t.Errorf("tras revocar sigue en su lista: %+v", bs)
	}

	// Invitado a UNA tabla: esa, y no el resto de la base.
	t2, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Segunda")
	e.ok(err)
	e.invitar("gus", domain.RolLector, t2.ID)
	if _, err := e.s.ListarRegistros(e.ctx, gusEnAcme, t2.ID, "", 0, ""); err != nil {
		t.Errorf("invitado a una tabla la lee: %v", err)
	}
	_, err = e.s.ListarRegistros(e.ctx, gusEnAcme, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "invitado a una tabla, en la otra")
	_, err = e.s.CrearRegistro(e.ctx, gusEnAcme, t2.ID, nil, nil)
	esperar(t, err, domain.ErrSinPermiso, "invitado lector escribe")
}

// La política `base_permissions_externo` es la única lectura entre organizaciones: deja
// ver las entradas `external` de UNA persona y nada más, y no deja escribir.
func TestLaLecturaEntreOrganizacionesSoloVeLoDeEsaPersona(t *testing.T) {
	e := nuevo(t)
	e.invitar("gus", domain.RolLector, "")
	e.invitar("hal", domain.RolLector, "")
	e.conceder(bea, domain.RolEditor)

	cuenta := func(sub, consulta string) (n int) {
		t.Helper()
		e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(e.ctx, `SELECT set_config('app.bases_externo', $1, true)`, sub); err != nil {
				return err
			}
			return tx.QueryRow(e.ctx, consulta).Scan(&n)
		}))
		return n
	}
	if n := cuenta("gus", `SELECT count(*) FROM base_permissions`); n != 1 {
		t.Errorf("gus ve %d entradas, esperada solo la suya", n)
	}
	if n := cuenta("bea", `SELECT count(*) FROM base_permissions`); n != 0 {
		t.Errorf("una entrada de persona se ve como de fuera: %d", n)
	}
	if n := cuenta("", `SELECT count(*) FROM base_permissions`); n != 0 {
		t.Errorf("sin persona se ven %d entradas", n)
	}
	for _, tabla := range []string{"bases", "base_tables", "base_records", "base_fields", "base_views", "base_comments", "base_audit"} {
		if n := cuenta("gus", `SELECT count(*) FROM `+tabla); n != 0 {
			t.Errorf("la lectura entre organizaciones ve %d filas de %s", n, tabla)
		}
	}
	// No sirve para escribir ni para borrar: ni subirse el rol ni quitarse de en medio.
	e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.ctx, `SELECT set_config('app.bases_externo', 'gus', true)`); err != nil {
			return err
		}
		tag, err := tx.Exec(e.ctx, `UPDATE base_permissions SET role = 'editor'`)
		if err == nil && tag.RowsAffected() != 0 {
			t.Errorf("la política de lectura dejó actualizar %d filas", tag.RowsAffected())
		}
		return nil
	}))
}
