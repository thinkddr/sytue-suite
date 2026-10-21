// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/bases/db"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

var (
	ana  = domain.Actor{Sub: "ana", Tenant: "acme"}
	bea  = domain.Actor{Sub: "bea", Tenant: "acme"}
	cleo = domain.Actor{Sub: "cleo", Tenant: "acme"}
	gus  = domain.Actor{Sub: "gus", Tenant: "globex"}
)

type entorno struct {
	t    *testing.T
	h    *pgtest.Harness
	s    *Store
	ctx  context.Context
	base domain.Base
	tabl domain.Tabla
	txt  domain.Campo
	num  domain.Campo
}

// nuevo monta una base de ana (acme) con una tabla, un campo de texto y uno numérico.
func nuevo(t *testing.T) *entorno {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	e := &entorno{t: t, h: h, s: New(h.Pool), ctx: context.Background()}
	var err error
	e.base, err = e.s.CrearBase(e.ctx, ana, "Clientes", "")
	e.ok(err)
	e.tabl, err = e.s.CrearTabla(e.ctx, ana, e.base.ID, "Contactos")
	e.ok(err)
	e.txt, err = e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Nombre", Kind: domain.TipoTexto})
	e.ok(err)
	e.num, err = e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Importe", Kind: domain.TipoNumero})
	e.ok(err)
	return e
}

func (e *entorno) ok(err error) {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
}

func esperar(t *testing.T, err, quiero error, qué string) {
	t.Helper()
	if !errors.Is(err, quiero) {
		t.Errorf("%s: error %v, esperado %v", qué, err, quiero)
	}
}

func dato(pares ...string) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	for i := 0; i < len(pares); i += 2 {
		m[pares[i]] = json.RawMessage(pares[i+1])
	}
	return m
}

func (e *entorno) registro(a domain.Actor, tabla string, d map[string]json.RawMessage) domain.Registro {
	e.t.Helper()
	r, err := e.s.CrearRegistro(e.ctx, a, tabla, d, nil)
	e.ok(err)
	return r
}

// El esquema cumple las dos reglas de la suite, comprobadas por catálogo.
func TestEsquema(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, nil)
	pgtest.ExigirPoliticaDeFusion(t, h)
}

func TestAislamientoEntreOrganizaciones(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))

	if bs, err := e.s.ListarBases(e.ctx, gus); err != nil || len(bs) != 0 {
		t.Errorf("gus ve bases ajenas: %v %v", bs, err)
	}
	_, err := e.s.ObtenerBase(e.ctx, gus, e.base.ID)
	esperar(t, err, domain.ErrNoExiste, "obtener base ajena")
	_, err = e.s.ListarTablas(e.ctx, gus, e.base.ID)
	esperar(t, err, domain.ErrNoExiste, "listar tablas ajenas")
	_, err = e.s.ListarRegistros(e.ctx, gus, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "leer registros ajenos")
	_, err = e.s.CrearRegistro(e.ctx, gus, e.tabl.ID, nil, nil)
	esperar(t, err, domain.ErrNoExiste, "crear en tabla ajena")
	_, err = e.s.ActualizarRegistro(e.ctx, gus, r.ID, 1, dato(e.txt.ID, `"x"`), nil)
	esperar(t, err, domain.ErrNoExiste, "actualizar registro ajeno")
	esperar(t, e.s.BorrarRegistro(e.ctx, gus, r.ID, true), domain.ErrNoExiste, "borrar registro ajeno")
	esperar(t, e.s.BorrarTabla(e.ctx, gus, e.tabl.ID), domain.ErrNoExiste, "borrar tabla ajena")
	esperar(t, e.s.BorrarBase(e.ctx, gus, e.base.ID), domain.ErrNoExiste, "borrar base ajena")
	err = e.s.ExportarTabla(e.ctx, gus, e.tabl.ID, func(Exportacion) error { return nil })
	esperar(t, err, domain.ErrNoExiste, "exportar tabla ajena")
	_, err = e.s.ListarAuditoria(e.ctx, gus, e.base.ID, 0, 10)
	esperar(t, err, domain.ErrNoExiste, "auditoría ajena")

	// Ni siquiera el propietario de una base de globex puede enlazar con la tabla de acme.
	bg, err := e.s.CrearBase(e.ctx, gus, "Suya", "")
	e.ok(err)
	tg, err := e.s.CrearTabla(e.ctx, gus, bg.ID, "T")
	e.ok(err)
	_, err = e.s.CrearCampo(e.ctx, gus, tg.ID, domain.EntradaCampo{Name: "L", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	esperar(t, err, domain.ErrNoExiste, "enlace a tabla de otra organización")

	// Conceder a otra organización como principal «tenant» no vale.
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "tenant", PrincipalID: "globex", Role: domain.RolLector})
	esperar(t, err, domain.ErrInvalido, "compartir con otra organización")
}

// Las claves foráneas compuestas son lo que impide un enlace entre organizaciones
// aunque el código se equivocara: la comprobación de una FK se salta la RLS.
func TestClaveForaneaCompuestaImpideEnlaceEntreOrganizaciones(t *testing.T) {
	e := nuevo(t)
	rAcme := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))
	bg, _ := e.s.CrearBase(e.ctx, gus, "Suya", "")
	tg, _ := e.s.CrearTabla(e.ctx, gus, bg.ID, "T")
	tg2, _ := e.s.CrearTabla(e.ctx, gus, bg.ID, "T2")
	lg, err := e.s.CrearCampo(e.ctx, gus, tg.ID, domain.EntradaCampo{Name: "L", Kind: domain.TipoRelacion, LinkTableID: tg2.ID})
	e.ok(err)
	rg := e.registro(gus, tg.ID, nil)

	err = e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(e.ctx, tx, "globex"); err != nil {
			return err
		}
		_, err := tx.Exec(e.ctx, `INSERT INTO base_links (tenant, field_id, from_record, from_table_id, to_record, to_table_id)
			VALUES ('globex', $1, $2, $3, $4, $5)`, lg.ID, rg.ID, tg.ID, rAcme.ID, tg2.ID)
		return err
	})
	if err == nil {
		t.Fatal("un enlace de globex a un registro de acme se ha insertado")
	}
	// Y con la RLS: no puede ni escribir una fila con el tenant de otro.
	err = e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(e.ctx, tx, "globex"); err != nil {
			return err
		}
		_, err := tx.Exec(e.ctx, `INSERT INTO bases (tenant, name, owner_sub) VALUES ('acme', 'x', 'gus')`)
		return err
	})
	if err == nil {
		t.Fatal("la RLS dejó escribir una base con el tenant de otra organización")
	}
}

func TestACL(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))

	// Sin ninguna entrada: la base no existe para bea.
	_, err := e.s.ObtenerBase(e.ctx, bea, e.base.ID)
	esperar(t, err, domain.ErrNoExiste, "bea sin acceso, obtener")
	_, err = e.s.ListarRegistros(e.ctx, bea, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "bea sin acceso, leer")
	_, err = e.s.ListarTablas(e.ctx, bea, e.base.ID)
	esperar(t, err, domain.ErrNoExiste, "bea sin acceso, tablas")
	if bs, _ := e.s.ListarBases(e.ctx, bea); len(bs) != 0 {
		t.Error("bea ve la base sin permiso")
	}

	// Lector.
	p, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolLector})
	e.ok(err)
	if _, err := e.s.ListarRegistros(e.ctx, bea, e.tabl.ID, "", 0, ""); err != nil {
		t.Errorf("lector lee: %v", err)
	}
	_, err = e.s.CrearRegistro(e.ctx, bea, e.tabl.ID, nil, nil)
	esperar(t, err, domain.ErrSinPermiso, "lector crea registro")
	_, err = e.s.ActualizarRegistro(e.ctx, bea, r.ID, 1, dato(e.txt.ID, `"x"`), nil)
	esperar(t, err, domain.ErrSinPermiso, "lector edita registro")
	esperar(t, e.s.BorrarRegistro(e.ctx, bea, r.ID, false), domain.ErrSinPermiso, "lector borra registro")
	_, err = e.s.CrearCampo(e.ctx, bea, e.tabl.ID, domain.EntradaCampo{Name: "X", Kind: domain.TipoTexto})
	esperar(t, err, domain.ErrSinPermiso, "lector crea campo")
	_, err = e.s.CrearVista(e.ctx, bea, e.tabl.ID, "V", domain.VistaRejilla, domain.ConfigVista{})
	esperar(t, err, domain.ErrSinPermiso, "lector crea vista")
	_, err = e.s.ListarPermisos(e.ctx, bea, e.base.ID)
	esperar(t, err, domain.ErrSinPermiso, "lector lista permisos")
	_, err = e.s.ConcederPermiso(e.ctx, bea, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	esperar(t, err, domain.ErrSinPermiso, "lector se sube el rol")
	_, err = e.s.ListarAuditoria(e.ctx, bea, e.base.ID, 0, 10)
	esperar(t, err, domain.ErrSinPermiso, "lector lee la auditoría")
	esperar(t, e.s.BorrarBase(e.ctx, bea, e.base.ID), domain.ErrSinPermiso, "lector borra la base")
	if err := e.s.ExportarTabla(e.ctx, bea, e.tabl.ID, func(Exportacion) error { return nil }); err != nil {
		t.Errorf("lector exporta lo que puede leer: %v", err)
	}

	// Editor: registros y vistas sí; esquema, permisos y auditoría no.
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	e.ok(err)
	if _, err := e.s.CrearRegistro(e.ctx, bea, e.tabl.ID, dato(e.txt.ID, `"Bea"`), nil); err != nil {
		t.Errorf("editor crea: %v", err)
	}
	if _, err := e.s.CrearVista(e.ctx, bea, e.tabl.ID, "V", domain.VistaRejilla, domain.ConfigVista{}); err != nil {
		t.Errorf("editor crea vista: %v", err)
	}
	_, err = e.s.CrearCampo(e.ctx, bea, e.tabl.ID, domain.EntradaCampo{Name: "X", Kind: domain.TipoTexto})
	esperar(t, err, domain.ErrSinPermiso, "editor crea campo")
	esperar(t, e.s.BorrarTabla(e.ctx, bea, e.tabl.ID), domain.ErrSinPermiso, "editor borra tabla")
	esperar(t, e.s.BorrarRegistro(e.ctx, bea, r.ID, true), domain.ErrSinPermiso, "editor fuerza el borrado")
	_, _, err = e.s.ImportarTabla(e.ctx, bea, e.base.ID, "T", domain.TablaCSV{})
	esperar(t, err, domain.ErrSinPermiso, "editor importa")

	// Revocar surte efecto al instante.
	e.ok(e.s.RevocarPermiso(e.ctx, ana, e.base.ID, p.ID)) // el de lector, ya sustituido por editor (misma fila)
	_, err = e.s.ListarRegistros(e.ctx, bea, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "tras revocar")

	// Acceso a UNA tabla: cleo solo ve y escribe en la suya.
	t2, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Segunda")
	e.ok(err)
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: t2.ID, PrincipalKind: "user", PrincipalID: "cleo", Role: domain.RolEditor})
	e.ok(err)
	if _, err := e.s.CrearRegistro(e.ctx, cleo, t2.ID, nil, nil); err != nil {
		t.Errorf("cleo escribe en su tabla: %v", err)
	}
	_, err = e.s.ListarRegistros(e.ctx, cleo, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "cleo en la otra tabla")
	ts, err := e.s.ListarTablas(e.ctx, cleo, e.base.ID)
	e.ok(err)
	if len(ts) != 1 || ts[0].ID != t2.ID {
		t.Errorf("cleo solo debe ver su tabla: %+v", ts)
	}
	// Un permiso de tabla de OTRA base no se puede conceder desde esta.
	b2, _ := e.s.CrearBase(e.ctx, ana, "Otra", "")
	_, err = e.s.ConcederPermiso(e.ctx, ana, b2.ID, domain.Permiso{TableID: t2.ID, PrincipalKind: "user", PrincipalID: "cleo", Role: domain.RolLector})
	esperar(t, err, domain.ErrNoExiste, "permiso con tabla de otra base")

	// Toda la organización como lectora.
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "tenant", PrincipalID: "acme", Role: domain.RolLector})
	e.ok(err)
	if _, err := e.s.ListarRegistros(e.ctx, cleo, e.tabl.ID, "", 0, ""); err != nil {
		t.Errorf("la organización lee: %v", err)
	}
}

func TestEnlacesYBorradoSeguro(t *testing.T) {
	e := nuevo(t)
	pedidos, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	e.ok(err)
	cli, err := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "Cliente", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	e.ok(err)
	c1 := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))
	p1, err := e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{cli.ID: {c1.ID}})
	e.ok(err)

	pag, err := e.s.ListarRegistros(e.ctx, ana, pedidos.ID, "", 0, "")
	e.ok(err)
	if len(pag.Records) != 1 || len(pag.Records[0].Links[cli.ID]) != 1 {
		t.Fatalf("enlace no leído: %+v", pag.Records)
	}

	// Lo enlazado no se borra a la ligera.
	esperar(t, e.s.BorrarRegistro(e.ctx, ana, c1.ID, false), domain.ErrReferenciado, "borrar registro enlazado")
	esperar(t, e.s.BorrarTabla(e.ctx, ana, e.tabl.ID), domain.ErrReferenciado, "borrar tabla con enlace entrante")

	// Enlaces inválidos.
	otra, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Otra")
	ro := e.registro(ana, otra.ID, nil)
	_, err = e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{cli.ID: {ro.ID}})
	esperar(t, err, domain.ErrInvalido, "enlace a registro de otra tabla")
	_, err = e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{cli.ID: {uuid.NewString()}})
	esperar(t, err, domain.ErrInvalido, "enlace a registro inexistente")
	_, err = e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{e.txt.ID: {c1.ID}})
	esperar(t, err, domain.ErrInvalido, "enlace en un campo que no es de enlace")
	// Registro de otra BASE de la misma organización.
	b2, _ := e.s.CrearBase(e.ctx, ana, "B2", "")
	t2, _ := e.s.CrearTabla(e.ctx, ana, b2.ID, "T")
	_, err = e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "X", Kind: domain.TipoRelacion, LinkTableID: t2.ID})
	esperar(t, err, domain.ErrNoExiste, "campo de enlace hacia otra base")

	// Forzar el borrado exige ser propietario y quita los enlaces.
	e.ok(e.s.BorrarRegistro(e.ctx, ana, c1.ID, true))
	pag, _ = e.s.ListarRegistros(e.ctx, ana, pedidos.ID, "", 0, "")
	if len(pag.Records) != 1 || len(pag.Records[0].Links[cli.ID]) != 0 || pag.Records[0].ID != p1.ID {
		t.Errorf("el pedido debe seguir sin enlaces: %+v", pag.Records)
	}
	// Sin enlaces entrantes ya se puede borrar la tabla: primero el campo de enlace.
	e.ok(e.s.BorrarCampo(e.ctx, ana, cli.ID))
	e.ok(e.s.BorrarTabla(e.ctx, ana, e.tabl.ID))
}

func TestRegistrosValidacionConflictoYAuditoria(t *testing.T) {
	e := nuevo(t)
	r := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`, e.num.ID, `10`))

	_, err := e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, dato(e.num.ID, `"diez"`), nil)
	esperar(t, err, domain.ErrInvalido, "número con texto")
	_, err = e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, dato("campo-inventado", `"x"`), nil)
	esperar(t, err, domain.ErrInvalido, "campo desconocido")

	r2, err := e.s.ActualizarRegistro(e.ctx, ana, r.ID, r.Rev, dato(e.txt.ID, `"Ana María"`, e.num.ID, `null`), nil)
	e.ok(err)
	if r2.Rev != 2 || r2.Data[e.num.ID] != nil || string(r2.Data[e.txt.ID]) != `"Ana María"` {
		t.Errorf("actualización: %+v", r2)
	}
	_, err = e.s.ActualizarRegistro(e.ctx, ana, r.ID, r.Rev, dato(e.txt.ID, `"viejo"`), nil)
	esperar(t, err, domain.ErrConflicto, "versión vieja")
	_, err = e.s.ActualizarRegistro(e.ctx, ana, r.ID, 0, dato(e.txt.ID, `"x"`), nil)
	esperar(t, err, domain.ErrInvalido, "sin versión")

	// Auditoría: quién, qué campo, antes y después; los valores grandes, acotados.
	grande := `"` + strings.Repeat("z", 5000) + `"`
	_, err = e.s.ActualizarRegistro(e.ctx, ana, r.ID, 2, dato(e.txt.ID, grande), nil)
	e.ok(err)
	es, err := e.s.ListarAuditoria(e.ctx, ana, e.base.ID, 0, 200)
	e.ok(err)
	var visto bool
	for _, x := range es {
		if x.Action == "record.update" && x.FieldID == e.txt.ID && strings.Contains(string(x.After), "truncated") {
			visto = true
			if x.Actor != "ana" || x.RecordID != r.ID || len(x.After) > 100 {
				t.Errorf("entrada: %+v", x)
			}
		}
	}
	if !visto {
		t.Error("falta la entrada acotada del cambio grande")
	}
	pag1, err := e.s.ListarAuditoria(e.ctx, ana, e.base.ID, 0, 3)
	e.ok(err)
	if len(pag1) != 3 {
		t.Fatalf("página de 3: %d", len(pag1))
	}
	pag2, _ := e.s.ListarAuditoria(e.ctx, ana, e.base.ID, pag1[2].ID, 3)
	if len(pag2) == 0 || pag2[0].ID >= pag1[2].ID {
		t.Error("la paginación por id no avanza")
	}
}

func TestVistasYFiltros(t *testing.T) {
	e := nuevo(t)
	sel, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Estado", Kind: domain.TipoSeleccion, Choices: []string{"nuevo", "hecho"}})
	e.ok(err)
	e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"100% Ana"`, e.num.ID, `5`, sel.ID, `"nuevo"`))
	e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Bea"`, e.num.ID, `20`, sel.ID, `"hecho"`))
	e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Cleo"`, e.num.ID, `15`))

	listar := func(cfg domain.ConfigVista) []string {
		v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "V", domain.VistaRejilla, cfg)
		e.ok(err)
		pag, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, v.ID, 0, "")
		e.ok(err)
		out := make([]string, 0, 3)
		for _, r := range pag.Records {
			var s string
			_ = json.Unmarshal(r.Data[e.txt.ID], &s)
			out = append(out, s)
		}
		return out
	}
	j := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	if got := listar(domain.ConfigVista{Filters: []domain.Filtro{{Field: e.num.ID, Op: "gt", Value: j(10)}}, Sort: []domain.Orden{{Field: e.num.ID, Dir: "desc"}}}); strings.Join(got, ",") != "Bea,Cleo" {
		t.Errorf("número > 10 desc: %v", got)
	}
	// `%` no es un comodín: solo coincide con el texto literal.
	if got := listar(domain.ConfigVista{Filters: []domain.Filtro{{Field: e.txt.ID, Op: "contains", Value: j("%")}}}); strings.Join(got, ",") != "100% Ana" {
		t.Errorf("contains %%: %v", got)
	}
	if got := listar(domain.ConfigVista{Filters: []domain.Filtro{{Field: e.txt.ID, Op: "contains", Value: j("BEA")}}}); strings.Join(got, ",") != "Bea" {
		t.Errorf("contains sin mayúsculas: %v", got)
	}
	if got := listar(domain.ConfigVista{Filters: []domain.Filtro{{Field: sel.ID, Op: "empty"}}}); strings.Join(got, ",") != "Cleo" {
		t.Errorf("selección vacía: %v", got)
	}
	if got := listar(domain.ConfigVista{Filters: []domain.Filtro{{Field: sel.ID, Op: "eq", Value: j("hecho")}, {Field: e.num.ID, Op: "lte", Value: j(20)}}}); strings.Join(got, ",") != "Bea" {
		t.Errorf("dos filtros: %v", got)
	}
	// Una vista de otra tabla no se puede usar para leer ésta.
	t2, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "T2")
	vOtra, _ := e.s.CrearVista(e.ctx, ana, t2.ID, "V", domain.VistaRejilla, domain.ConfigVista{})
	_, err = e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, vOtra.ID, 0, "")
	esperar(t, err, domain.ErrNoExiste, "vista de otra tabla")
	// Tablero Kanban por la selección.
	if _, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Tablero", domain.VistaKanban, domain.ConfigVista{GroupBy: sel.ID}); err != nil {
		t.Errorf("kanban: %v", err)
	}
	if _, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Mal", domain.VistaRejilla, domain.ConfigVista{Filters: []domain.Filtro{{Field: uuid.NewString(), Op: "eq", Value: j("x")}}}); !errors.Is(err, domain.ErrInvalido) {
		t.Errorf("filtro sobre campo inexistente: %v", err)
	}
	// Paginación acotada.
	pag, _ := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, "", 2, "")
	if len(pag.Records) != 2 {
		t.Errorf("limit 2: %d", len(pag.Records))
	}
	pag, _ = e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, "", 1000000, "")
	if len(pag.Records) != 3 {
		t.Errorf("limit enorme se acota: %d", len(pag.Records))
	}
}

func TestImportarYExportar(t *testing.T) {
	e := nuevo(t)
	csv := "Nombre,Importe,Alta\nAna,12.5,2026-01-02\nBea,,2026-02-03\n=2+2,7,2026-03-04\n"
	tb, err := domain.LeerCSV(strings.NewReader(csv))
	e.ok(err)
	tabla, campos, err := e.s.ImportarTabla(e.ctx, ana, e.base.ID, "Importada", tb)
	e.ok(err)
	if len(campos) != 3 || campos[1].Kind != domain.TipoNumero || campos[2].Kind != domain.TipoFecha {
		t.Errorf("campos: %+v", campos)
	}
	filas := 0
	e.ok(e.s.ExportarTabla(e.ctx, ana, tabla.ID, func(ex Exportacion) error { filas += len(ex.Records); return nil }))
	if filas != 3 {
		t.Errorf("filas importadas: %d", filas)
	}
	es, _ := e.s.ListarAuditoria(e.ctx, ana, e.base.ID, 0, 200)
	var imp, exp bool
	for _, x := range es {
		imp = imp || x.Action == "table.import"
		exp = exp || x.Action == "table.export"
	}
	if !imp || !exp {
		t.Errorf("la importación y la exportación se auditan: import=%v export=%v", imp, exp)
	}
	// Todo o nada: un CSV con una fila que no cabe no deja una tabla a medias.
	_, _, err = e.s.ImportarTabla(e.ctx, ana, e.base.ID, "", tb)
	esperar(t, err, domain.ErrInvalido, "nombre vacío")
	ts, _ := e.s.ListarTablas(e.ctx, ana, e.base.ID)
	for _, x := range ts {
		if x.Name == "" {
			t.Error("tabla a medias")
		}
	}
}

func TestLimites(t *testing.T) {
	e := nuevo(t)
	for i := 2; i <= domain.MaxTablasPorBase; i++ {
		_, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "T")
		e.ok(err)
	}
	_, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "T")
	esperar(t, err, domain.ErrLimite, "tablas por base")
	for i := 0; i < domain.MaxPermisosPorBase; i++ {
		_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "u" + uuid.NewString(), Role: domain.RolLector})
		e.ok(err)
	}
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "uno-mas", Role: domain.RolLector})
	esperar(t, err, domain.ErrLimite, "permisos por base")
}

func TestBorrarBaseSeLlevaTodoYDejaLaTraza(t *testing.T) {
	e := nuevo(t)
	pedidos, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	cli, _ := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "C", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	c := e.registro(ana, e.tabl.ID, nil)
	_, err := e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{cli.ID: {c.ID}})
	e.ok(err)
	// Con enlaces cruzados, borrar la base entera funciona (en cascada), a diferencia de una tabla suelta.
	e.ok(e.s.BorrarBase(e.ctx, ana, e.base.ID))
	_, err = e.s.ObtenerBase(e.ctx, ana, e.base.ID)
	esperar(t, err, domain.ErrNoExiste, "base borrada")
	var n int
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM base_audit WHERE base_id = $1`, e.base.ID).Scan(&n)
	}))
	if n == 0 {
		t.Error("la traza debe sobrevivir al borrado")
	}
}

// La fusión de organizaciones (Sytue ID, merge.MoveService) reescribe `tenant` tabla a
// tabla en el orden del catálogo, con `app.merge`. Con las claves compuestas inmediatas
// la primera tabla hija rompía; diferidas (migración 0004), todo se mueve y cuadra.
func TestLaFusionDeOrganizacionesMueveTodasLasTablas(t *testing.T) {
	e := nuevo(t)
	pedidos, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	cli, _ := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "C", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	c := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`))
	_, err := e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{cli.ID: {c.ID}})
	e.ok(err)
	v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "F", domain.VistaFormulario, domain.ConfigVista{})
	e.ok(err)
	_, _, err = e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 5, nil)
	e.ok(err)
	_, token, err := e.s.CrearConexion(e.ctx, ana, pedidos.ID, "", "Calcus", "", 0)
	e.ok(err)
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolLector})
	e.ok(err)
	hilo, _, err := e.s.CrearComentario(e.ctx, ana, c.ID, "", "raíz")
	e.ok(err)
	_, _, err = e.s.CrearComentario(e.ctx, ana, c.ID, hilo.ID, "respuesta")
	e.ok(err)
	guion := e.aprobado(ana, soloLeer)
	_, err = e.s.AutorizarEjecucion(e.ctx, ana, e.base.ID, guion.ID, 1)
	e.ok(err)

	// Lo mismo que hace merge.MoveService: todas las tablas con `tenant`, en orden de nombre.
	e.ok(e.h.Pool.InMergeTx(e.ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(e.ctx, `SELECT table_name FROM information_schema.columns
			WHERE table_schema = current_schema() AND column_name = 'tenant' ORDER BY table_name`)
		if err != nil {
			return err
		}
		var tablas []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return err
			}
			tablas = append(tablas, n)
		}
		rows.Close()
		for _, tb := range tablas {
			if _, err := tx.Exec(e.ctx, "UPDATE "+pgx.Identifier{tb}.Sanitize()+" SET tenant = $1 WHERE tenant = $2", "fusionada", "acme"); err != nil {
				return err
			}
		}
		return nil
	}))

	// Todo sigue funcionando en la organización de destino, enlaces y conector incluidos.
	nueva := domain.Actor{Sub: "ana", Tenant: "fusionada"}
	pag, err := e.s.ListarRegistros(e.ctx, nueva, pedidos.ID, "", 0, "")
	e.ok(err)
	if len(pag.Records) != 1 || len(pag.Records[0].Links[cli.ID]) != 1 {
		t.Errorf("tras la fusión: %+v", pag.Records)
	}
	if cs, err := e.s.ListarComentarios(e.ctx, nueva, c.ID); err != nil || len(cs) != 2 {
		t.Errorf("los comentarios tras la fusión: %d %v", len(cs), err)
	}
	if _, err := e.s.LeerPorConector(e.ctx, token, "", "", ""); err != nil {
		t.Errorf("el conector tras la fusión: %v", err)
	}
	// Los scripts se mueven con su base: la versión aprobada sigue ejecutándose y el
	// registro (y con él la cuota) va con la organización de destino.
	if _, err := e.s.AutorizarEjecucion(e.ctx, nueva, e.base.ID, guion.ID, 1); err != nil {
		t.Errorf("el script tras la fusión: %v", err)
	}
	if uso, err := e.s.UsoCuotaScripts(e.ctx, nueva); err != nil || uso.EjecucionesOrg != 2 {
		t.Errorf("el registro de scripts tras la fusión: %+v %v", uso, err)
	}
	if bs, _ := e.s.ListarBases(e.ctx, ana); len(bs) != 0 {
		t.Error("quedan bases con el tenant de origen")
	}
}
