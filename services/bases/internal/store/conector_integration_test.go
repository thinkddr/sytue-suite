// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

func TestConectorLeeLoQueAbarcaYNadaMas(t *testing.T) {
	e := nuevo(t)
	pedidos, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	e.ok(err)
	cli, err := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "Cliente", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	e.ok(err)
	mul, err := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "Etiquetas", Kind: domain.TipoMultiple, Choices: []string{"a", "b"}})
	e.ok(err)
	c1 := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Panadería Aurora"`))
	_, err = e.s.CrearRegistro(e.ctx, ana, pedidos.ID, dato(mul.ID, `["a","b"]`), map[string][]string{cli.ID: {c1.ID}})
	e.ok(err)

	con, token, err := e.s.CrearConexion(e.ctx, ana, pedidos.ID, "", "Calcus", "", 0)
	e.ok(err)
	if !strings.HasPrefix(token, PrefijoTokenConector) || strings.Contains(string(jsonDe(con)), token) {
		t.Fatal("el token no viaja en la conexión")
	}
	l, err := e.s.LeerPorConector(e.ctx, token, "", "", "test")
	e.ok(err)
	if len(l.Rows) != 1 || len(l.Columns) != 2 {
		t.Fatalf("lectura: %+v", l)
	}
	// El enlace sale con el NOMBRE del registro enlazado, no con su id; la múltiple, unida.
	if l.Rows[0][0] != "Panadería Aurora" || l.Rows[0][1] != "a, b" {
		t.Errorf("fila: %+v", l.Rows[0])
	}
	if l.Provenance.TableID != pedidos.ID || l.Provenance.ConnectionID != con.ID || l.Provenance.ViewID != "" {
		t.Errorf("procedencia: %+v", l.Provenance)
	}
	// La conexión de «Pedidos» no lee «Contactos»: no hay parámetro con el que pedir otra tabla.
	for _, fila := range l.Rows {
		if len(fila) != len(l.Columns) {
			t.Errorf("fila desalineada: %v", fila)
		}
	}

	// El hash, no el token, es lo que hay en la base.
	var n int
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM base_connections WHERE token_hash = $1`, token).Scan(&n)
	}))
	if n != 0 {
		t.Error("el token está en claro en la base")
	}

	// La lectura se cuenta.
	cs, err := e.s.ListarConexiones(e.ctx, ana, e.base.ID)
	e.ok(err)
	if len(cs) != 1 || cs[0].Reads != 1 || cs[0].LastReadAt == nil {
		t.Errorf("contador de lecturas: %+v", cs)
	}
}

func TestConectorRevocacionCaducidadYPermisos(t *testing.T) {
	e := nuevo(t)
	_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	e.ok(err)
	_, _, err = e.s.CrearConexion(e.ctx, bea, e.tabl.ID, "", "x", "", 0)
	esperar(t, err, domain.ErrSinPermiso, "un editor concede")
	_, _, err = e.s.CrearConexion(e.ctx, gus, e.tabl.ID, "", "x", "", 0)
	esperar(t, err, domain.ErrNoExiste, "otra organización concede")
	_, _, err = e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "x", "", 999)
	esperar(t, err, domain.ErrInvalido, "caducidad fuera de rango")
	_, _, err = e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "", "", 0)
	esperar(t, err, domain.ErrInvalido, "sin nombre")

	// Una vista de OTRA tabla no acota esta.
	t2, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "T2")
	vOtra, _ := e.s.CrearVista(e.ctx, ana, t2.ID, "V", domain.VistaRejilla, domain.ConfigVista{})
	_, _, err = e.s.CrearConexion(e.ctx, ana, e.tabl.ID, vOtra.ID, "x", "", 0)
	esperar(t, err, domain.ErrNoExiste, "vista de otra tabla")

	con, token, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "Calcus", "", 1)
	e.ok(err)
	if _, err := e.s.LeerPorConector(e.ctx, token, "", "", ""); err != nil {
		t.Fatalf("lee: %v", err)
	}
	esperar(t, e.s.RevocarConexion(e.ctx, bea, con.ID), domain.ErrSinPermiso, "un editor revoca")
	esperar(t, e.s.RevocarConexion(e.ctx, gus, con.ID), domain.ErrNoExiste, "otra organización revoca")
	e.ok(e.s.RevocarConexion(e.ctx, ana, con.ID))
	_, err = e.s.LeerPorConector(e.ctx, token, "", "", "")
	esperar(t, err, domain.ErrNoExiste, "revocada")
	_, _, err = e.s.RotarConexion(e.ctx, ana, con.ID)
	esperar(t, err, domain.ErrNoExiste, "rotar una revocada")

	// Caducada: se fuerza la fecha.
	con2, tok2, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "Calcus", "", 1)
	e.ok(err)
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `UPDATE base_connections SET expires_at = now() - interval '1 minute' WHERE id = $1`, con2.ID)
		return err
	}))
	_, err = e.s.LeerPorConector(e.ctx, tok2, "", "", "")
	esperar(t, err, domain.ErrNoExiste, "caducada")

	// Borrar la tabla se lleva la conexión: el token deja de leer.
	t3, _ := e.s.CrearTabla(e.ctx, ana, e.base.ID, "T3")
	_, tok3, err := e.s.CrearConexion(e.ctx, ana, t3.ID, "", "Calcus", "", 1)
	e.ok(err)
	e.ok(e.s.BorrarTabla(e.ctx, ana, t3.ID))
	_, err = e.s.LeerPorConector(e.ctx, tok3, "", "", "")
	esperar(t, err, domain.ErrNoExiste, "tabla borrada")

	// Un token de formulario no lee, ni uno de conector envía un formulario.
	v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "F", domain.VistaFormulario, domain.ConfigVista{})
	e.ok(err)
	_, tokForm, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 5, nil)
	e.ok(err)
	_, err = e.s.LeerPorConector(e.ctx, tokForm, "", "", "")
	esperar(t, err, domain.ErrNoExiste, "token de formulario en el conector")
	_, err = e.s.LeerPorConector(e.ctx, PrefijoTokenConector+tokForm, "", "", "")
	esperar(t, err, domain.ErrNoExiste, "token de formulario con prefijo")
	esperar(t, e.s.EnviarFormulario(e.ctx, strings.TrimPrefix(token, PrefijoTokenConector), dato(e.txt.ID, `"x"`)), domain.ErrNoExiste, "token de conector en un formulario")
}

// Una conexión es de UN consumidor, de UNA vista y de UNA organización. Cada comprobación de
// aquí se ha visto fallar quitando la condición que vigila en LeerPorConector y CrearConexion.
func TestConectorPorConsumidorVistaYOrganizacion(t *testing.T) {
	e := nuevo(t)
	e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Aurora"`, e.num.ID, `10`))
	e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Bea"`, e.num.ID, `99`))
	caras, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Caras", domain.VistaRejilla,
		domain.ConfigVista{Filters: []domain.Filtro{{Field: e.num.ID, Op: "gt", Value: json.RawMessage(`50`)}}})
	e.ok(err)
	baratas, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Baratas", domain.VistaRejilla,
		domain.ConfigVista{Filters: []domain.Filtro{{Field: e.num.ID, Op: "lt", Value: json.RawMessage(`50`)}}})
	e.ok(err)

	// Los consumidores nuevos exigen una vista, y uno desconocido no existe.
	for _, c := range []string{"drive", "erp", "web"} {
		_, _, err = e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "x", c, 1)
		esperar(t, err, domain.ErrInvalido, c+" sin vista")
	}
	_, _, err = e.s.CrearConexion(e.ctx, ana, e.tabl.ID, caras.ID, "x", "otro", 1)
	esperar(t, err, domain.ErrInvalido, "consumidor desconocido")

	con, token, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, caras.ID, "Informe", "erp", 1)
	e.ok(err)
	_, tokBaratas, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, baratas.ID, "Sitio", "web", 1)
	e.ok(err)

	// Lee SU vista y nada más: ni la otra vista de la misma tabla.
	l, err := e.s.LeerPorConector(e.ctx, token, "erp", "acme", "test")
	e.ok(err)
	if len(l.Rows) != 1 || l.Rows[0][0] != "Bea" {
		t.Fatalf("la conexión de «Caras» lee %v", l.Rows)
	}
	p := l.Provenance
	if p.Consumer != "erp" || p.ViewID != caras.ID || p.ViewName != "Caras" || p.BaseID != e.base.ID || p.GrantedBy != "ana" || p.ConnectionID != con.ID || p.GrantedAt.IsZero() {
		t.Errorf("procedencia: %+v", p)
	}
	l2, err := e.s.LeerPorConector(e.ctx, tokBaratas, "web", "acme", "test")
	e.ok(err)
	if len(l2.Rows) != 1 || l2.Rows[0][0] != "Aurora" || l2.Provenance.ViewID != baratas.ID {
		t.Fatalf("la conexión de «Baratas» lee %v", l2.Rows)
	}

	// Ni otra base: la conexión de una base de la misma dueña no enseña esta.
	otra, err := e.s.CrearBase(e.ctx, ana, "Otra", "")
	e.ok(err)
	tOtra, err := e.s.CrearTabla(e.ctx, ana, otra.ID, "T")
	e.ok(err)
	cOtra, err := e.s.CrearCampo(e.ctx, ana, tOtra.ID, domain.EntradaCampo{Name: "N", Kind: domain.TipoTexto})
	e.ok(err)
	e.registro(ana, tOtra.ID, dato(cOtra.ID, `"de la otra base"`))
	vOtra, err := e.s.CrearVista(e.ctx, ana, tOtra.ID, "V", domain.VistaRejilla, domain.ConfigVista{})
	e.ok(err)
	_, tokOtra, err := e.s.CrearConexion(e.ctx, ana, tOtra.ID, vOtra.ID, "x", "erp", 1)
	e.ok(err)
	l3, err := e.s.LeerPorConector(e.ctx, tokOtra, "erp", "acme", "test")
	e.ok(err)
	if len(l3.Rows) != 1 || l3.Rows[0][0] != "de la otra base" || l3.Provenance.BaseID != otra.ID {
		t.Fatalf("la conexión de la otra base lee %v de %s", l3.Rows, l3.Provenance.BaseID)
	}

	// El token de SyERP no vale en otro consumidor, ni sin organización, ni en otra.
	for nombre, caso := range map[string][2]string{
		"en Sytue Web":        {"web", "acme"},
		"en Drive":            {"drive", "acme"},
		"como Calcus":         {"", ""},
		"como Calcus de acme": {"calcus", "acme"},
		"sin organización":    {"erp", ""},
		"otra organización":   {"erp", "globex"},
	} {
		_, err = e.s.LeerPorConector(e.ctx, token, caso[0], caso[1], "test")
		esperar(t, err, domain.ErrNoExiste, "token de SyERP "+nombre)
	}
	// Y una conexión de Calcus sigue leyendo sin organización, pero no desde otra.
	_, tokCalcus, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "Hoja", "", 1)
	e.ok(err)
	_, err = e.s.LeerPorConector(e.ctx, tokCalcus, "", "", "test")
	e.ok(err)
	_, err = e.s.LeerPorConector(e.ctx, tokCalcus, "", "globex", "test")
	esperar(t, err, domain.ErrNoExiste, "Calcus desde otra organización")
	_, err = e.s.LeerPorConector(e.ctx, tokCalcus, "erp", "acme", "test")
	esperar(t, err, domain.ErrNoExiste, "token de Calcus en SyERP")

	// Los intentos denegados no cuentan como lecturas y quedan en la auditoría.
	cs, err := e.s.ListarConexiones(e.ctx, ana, e.base.ID)
	e.ok(err)
	for _, c := range cs {
		if c.ID == con.ID && (c.Reads != 1 || c.Consumer != "erp") {
			t.Errorf("la conexión de SyERP: %+v", c)
		}
	}
	var denegados, leidos int
	e.ok(e.s.en(e.ctx, ana, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FILTER (WHERE action = 'connector.denied'), count(*) FILTER (WHERE action = 'connector.read' AND after->>'consumer' = 'erp')
			FROM base_audit WHERE base_id = $1`, e.base.ID).Scan(&denegados, &leidos)
	}))
	if denegados != 8 || leidos != 1 {
		t.Errorf("auditoría: %d denegados (8), %d lecturas de SyERP (1)", denegados, leidos)
	}

	// Revocar corta de inmediato, también a los consumidores nuevos.
	e.ok(e.s.RevocarConexion(e.ctx, ana, con.ID))
	_, err = e.s.LeerPorConector(e.ctx, token, "erp", "acme", "test")
	esperar(t, err, domain.ErrNoExiste, "revocada")
}
