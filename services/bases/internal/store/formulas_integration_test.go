// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

func ref(ids ...string) string {
	s := "{" + ids[0]
	if len(ids) > 1 {
		s += "." + ids[1]
	}
	return s + "}"
}

func (e *entorno) formula(a domain.Actor, tabla, nombre, expr string, tipo domain.Tipo) domain.Campo {
	e.t.Helper()
	c, err := e.s.CrearCampo(e.ctx, a, tabla, domain.EntradaCampo{Name: nombre, Kind: domain.TipoFormula, Formula: expr, Result: tipo})
	e.ok(err)
	return c
}

// valor lee el valor guardado de un campo de un registro, como JSON ("" = vacío).
func (e *entorno) valor(tenant, registro, campo string) (v string) {
	e.t.Helper()
	e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(e.ctx, tx, tenant); err != nil {
			return err
		}
		return tx.QueryRow(e.ctx, `SELECT coalesce((data -> $2::text)::text, '') FROM base_records WHERE id = $1`, registro, campo).Scan(&v)
	}))
	return v
}

func (e *entorno) vaciarCola() {
	e.t.Helper()
	for i := 0; i < 100; i++ {
		n, err := e.s.RecalcularPendientes(e.ctx)
		e.ok(err)
		if n == 0 {
			return
		}
	}
	e.t.Fatal("la cola de recálculos no se vacía")
}

// Una fórmula sobre los campos del propio registro se calcula al crear y al cambiar, se
// filtra y se ordena como su resultado, y nadie la escribe desde fuera.
func TestFormulaDelRegistro(t *testing.T) {
	e := nuevo(t)
	viejo := e.registro(ana, e.tabl.ID, dato(e.num.ID, `10`))
	doble := e.formula(ana, e.tabl.ID, "Doble", ref(e.num.ID)+" * 2", domain.TipoNumero)
	if doble.Formula == "" || doble.Result != domain.TipoNumero {
		t.Errorf("campo devuelto: %+v", doble)
	}
	// Los registros que ya existían se calculan al crear la fórmula.
	if v := e.valor("acme", viejo.ID, doble.ID); v != "20" {
		t.Errorf("registro anterior a la fórmula: %q", v)
	}
	r := e.registro(ana, e.tabl.ID, dato(e.num.ID, `4`))
	if string(r.Data[doble.ID]) != "8" {
		t.Errorf("al crear: %s", r.Data[doble.ID])
	}
	r2, err := e.s.ActualizarRegistro(e.ctx, ana, r.ID, r.Rev, dato(e.num.ID, `5`), nil)
	e.ok(err)
	if string(r2.Data[doble.ID]) != "10" || r2.Rev != r.Rev+1 {
		t.Errorf("al cambiar: %s (rev %d)", r2.Data[doble.ID], r2.Rev)
	}
	_, err = e.s.ActualizarRegistro(e.ctx, ana, r.ID, r2.Rev, dato(doble.ID, `999`), nil)
	esperar(t, err, domain.ErrInvalido, "escribir un campo de fórmula")
	_, err = e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, dato(doble.ID, `999`), nil)
	esperar(t, err, domain.ErrInvalido, "crear con un campo de fórmula")

	v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Grandes", domain.VistaRejilla, domain.ConfigVista{
		Filters: []domain.Filtro{{Field: doble.ID, Op: "gt", Value: json.RawMessage(`15`)}},
		Sort:    []domain.Orden{{Field: doble.ID, Dir: "desc"}},
	})
	e.ok(err)
	pag, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, v.ID, 0, "")
	e.ok(err)
	if len(pag.Records) != 1 || pag.Records[0].ID != viejo.ID || pag.Recalculating {
		t.Errorf("filtro sobre la fórmula: %d registros", len(pag.Records))
	}
	// Cambiar la expresión recalcula; el campo del que depende no se puede borrar.
	e.ok(e.s.CambiarFormula(e.ctx, ana, doble.ID, ref(e.num.ID)+" * 3"))
	if v := e.valor("acme", viejo.ID, doble.ID); v != "30" {
		t.Errorf("tras cambiar la fórmula: %q", v)
	}
	esperar(t, e.s.BorrarCampo(e.ctx, ana, e.num.ID), domain.ErrReferenciado, "borrar un campo que usa una fórmula")
	e.ok(e.s.BorrarCampo(e.ctx, ana, doble.ID))
	e.ok(e.s.BorrarCampo(e.ctx, ana, e.num.ID))
}

// pedidosYLineas monta Pedidos —enlace→ Líneas(Importe) con «Total = SUM({Líneas.Importe})».
func pedidosYLineas(e *entorno) (pedidos, lineas domain.Tabla, enlace, importe, total domain.Campo) {
	var err error
	lineas, err = e.s.CrearTabla(e.ctx, ana, e.base.ID, "Líneas")
	e.ok(err)
	importe, err = e.s.CrearCampo(e.ctx, ana, lineas.ID, domain.EntradaCampo{Name: "Importe", Kind: domain.TipoNumero})
	e.ok(err)
	pedidos, err = e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	e.ok(err)
	enlace, err = e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "Líneas", Kind: domain.TipoRelacion, LinkTableID: lineas.ID})
	e.ok(err)
	total = e.formula(ana, pedidos.ID, "Total", "SUM("+ref(enlace.ID, importe.ID)+") + 0 * "+ref(enlace.ID), domain.TipoNumero)
	return pedidos, lineas, enlace, importe, total
}

// Los agregados de enlaces se rehacen cuando cambia un registro enlazado, cuando cambian
// los enlaces y cuando el enlazado se borra; y una fórmula que agrega OTRA fórmula, en
// cadena por las tablas.
func TestFormulaConAgregadosDeEnlaces(t *testing.T) {
	e := nuevo(t)
	pedidos, lineas, enlace, importe, total := pedidosYLineas(e)
	l1 := e.registro(ana, lineas.ID, dato(importe.ID, `10`))
	l2 := e.registro(ana, lineas.ID, dato(importe.ID, `5`))
	p, err := e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{enlace.ID: {l1.ID, l2.ID}})
	e.ok(err)
	if string(p.Data[total.ID]) != "15" {
		t.Fatalf("total al crear: %s", p.Data[total.ID])
	}
	// Tercer nivel: Clientes —enlace→ Pedidos, «Facturado = SUM({Pedidos.Total})».
	enlaceCP, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Pedidos", Kind: domain.TipoRelacion, LinkTableID: pedidos.ID})
	e.ok(err)
	facturado := e.formula(ana, e.tabl.ID, "Facturado", "SUM("+ref(enlaceCP.ID, total.ID)+")", domain.TipoNumero)
	cli, err := e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, nil, map[string][]string{enlaceCP.ID: {p.ID}})
	e.ok(err)
	if string(cli.Data[facturado.ID]) != "15" {
		t.Fatalf("facturado al crear: %s", cli.Data[facturado.ID])
	}
	comprobar := func(cuando, quiero string) {
		t.Helper()
		if v := e.valor("acme", p.ID, total.ID); v != quiero {
			t.Errorf("total %s: %q, esperado %s", cuando, v, quiero)
		}
		if v := e.valor("acme", cli.ID, facturado.ID); v != quiero {
			t.Errorf("facturado %s: %q, esperado %s", cuando, v, quiero)
		}
	}
	_, err = e.s.ActualizarRegistro(e.ctx, ana, l1.ID, l1.Rev, dato(importe.ID, `100`), nil)
	e.ok(err)
	comprobar("tras cambiar una línea", "105")
	// El recálculo no es una edición: no sube la versión del pedido.
	p2, err := e.s.ActualizarRegistro(e.ctx, ana, p.ID, p.Rev, nil, map[string][]string{enlace.ID: {l1.ID}})
	e.ok(err)
	if string(p2.Data[total.ID]) != "100" {
		t.Errorf("total tras quitar un enlace: %s", p2.Data[total.ID])
	}
	comprobar("tras quitar un enlace", "100")
	e.ok(e.s.BorrarRegistro(e.ctx, ana, l1.ID, true))
	comprobar("tras borrar la línea enlazada", "0")
}

// Ciclos: ni un campo consigo mismo, ni dos entre sí, ni dando la vuelta por otra tabla.
func TestFormulaSinCiclos(t *testing.T) {
	e := nuevo(t)
	a := e.formula(ana, e.tabl.ID, "A", ref(e.num.ID)+" + 1", domain.TipoNumero)
	b := e.formula(ana, e.tabl.ID, "B", ref(a.ID)+" + 1", domain.TipoNumero)
	esperar(t, e.s.CambiarFormula(e.ctx, ana, a.ID, ref(a.ID)+" + 1"), domain.ErrInvalido, "fórmula que se usa a sí misma")
	esperar(t, e.s.CambiarFormula(e.ctx, ana, a.ID, ref(b.ID)+" + 1"), domain.ErrInvalido, "A usa B, que usa A")

	// Entre tablas: T1.X = SUM({L12.Y}) y T2.Y = SUM({L21.X}).
	t2, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "T2")
	e.ok(err)
	l12, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "A T2", Kind: domain.TipoRelacion, LinkTableID: t2.ID})
	e.ok(err)
	l21, err := e.s.CrearCampo(e.ctx, ana, t2.ID, domain.EntradaCampo{Name: "A T1", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	e.ok(err)
	y := e.formula(ana, t2.ID, "Y", "SUM("+ref(l21.ID, a.ID)+")", domain.TipoNumero)
	esperar(t, e.s.CambiarFormula(e.ctx, ana, a.ID, "SUM("+ref(l12.ID, y.ID)+")"), domain.ErrInvalido, "ciclo entre tablas")
	// Sin cerrar el círculo, sí.
	e.ok(e.s.CambiarFormula(e.ctx, ana, b.ID, "SUM("+ref(l12.ID, y.ID)+")"))
}

// Una fórmula solo alcanza campos de SU tabla y, por un enlace suyo, de la tabla
// enlazada: ni otra tabla, ni otra base, ni otra organización. Y solo la define el
// propietario.
func TestFormulaNoSaleDeSuAlcance(t *testing.T) {
	e := nuevo(t)
	_, lineas, enlace, importe, _ := pedidosYLineas(e)
	crear := func(a domain.Actor, tabla, expr string) error {
		_, err := e.s.CrearCampo(e.ctx, a, tabla, domain.EntradaCampo{Name: "F", Kind: domain.TipoFormula, Formula: expr})
		return err
	}
	esperar(t, crear(ana, e.tabl.ID, ref(importe.ID)), domain.ErrInvalido, "campo de otra tabla, sin enlace")
	esperar(t, crear(ana, e.tabl.ID, "SUM("+ref(enlace.ID, importe.ID)+")"), domain.ErrInvalido, "enlace de otra tabla")
	esperar(t, crear(ana, lineas.ID, "SUM("+ref(enlace.ID, importe.ID)+")"), domain.ErrInvalido, "enlace que no sale de esta tabla")

	otra, err := e.s.CrearBase(e.ctx, ana, "Otra", "")
	e.ok(err)
	tOtra, err := e.s.CrearTabla(e.ctx, ana, otra.ID, "T")
	e.ok(err)
	esperar(t, crear(ana, tOtra.ID, ref(e.num.ID)), domain.ErrInvalido, "campo de otra base")

	bg, err := e.s.CrearBase(e.ctx, gus, "Globex", "")
	e.ok(err)
	tg, err := e.s.CrearTabla(e.ctx, gus, bg.ID, "T")
	e.ok(err)
	esperar(t, crear(gus, tg.ID, ref(e.num.ID)), domain.ErrInvalido, "campo de otra organización")
	esperar(t, crear(gus, e.tabl.ID, "1"), domain.ErrNoExiste, "fórmula en la tabla de otra organización")

	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	e.ok(err)
	esperar(t, crear(bea, e.tabl.ID, "1"), domain.ErrSinPermiso, "un editor define una fórmula")
	f := e.formula(ana, e.tabl.ID, "F", "1", domain.TipoNumero)
	esperar(t, e.s.CambiarFormula(e.ctx, bea, f.ID, "2"), domain.ErrSinPermiso, "un editor cambia una fórmula")
	esperar(t, e.s.CambiarFormula(e.ctx, gus, f.ID, "2"), domain.ErrNoExiste, "otra organización cambia una fórmula")
	esperar(t, e.s.CambiarFormula(e.ctx, ana, e.num.ID, "2"), domain.ErrInvalido, "fórmula en un campo que no lo es")

	for i := 1; i < domain.MaxFormulasPorTabla; i++ {
		e.formula(ana, e.tabl.ID, fmt.Sprint("F", i), "1", domain.TipoNumero)
	}
	esperar(t, crear(ana, e.tabl.ID, "1"), domain.ErrLimite, "más fórmulas de las que caben")
}

// Lo que no cabe en el presupuesto de una petición va a la cola y el bucle lo termina:
// al crear una fórmula en una tabla «grande» y al cambiar un registro del que dependen
// «muchos».
func TestFormulaPorLaCola(t *testing.T) {
	previo := presupuestoRecalculo
	presupuestoRecalculo = 3
	t.Cleanup(func() { presupuestoRecalculo = previo })
	e := nuevo(t)
	pedidos, lineas, enlace, importe, total := pedidosYLineas(e)
	linea := e.registro(ana, lineas.ID, dato(importe.ID, `7`))
	var ps []domain.Registro
	for i := 0; i < 6; i++ {
		p, err := e.s.CrearRegistro(e.ctx, ana, pedidos.ID, nil, map[string][]string{enlace.ID: {linea.ID}})
		e.ok(err)
		ps = append(ps, p)
	}
	// Seis pedidos dependen de la línea y solo caben tres: la tabla va a la cola.
	_, err := e.s.ActualizarRegistro(e.ctx, ana, linea.ID, linea.Rev, dato(importe.ID, `9`), nil)
	e.ok(err)
	pag, err := e.s.ListarRegistros(e.ctx, ana, pedidos.ID, "", 0, "")
	e.ok(err)
	if !pag.Recalculating {
		t.Error("la página no avisa de que hay un recálculo pendiente")
	}
	e.vaciarCola()
	for _, p := range ps {
		if v := e.valor("acme", p.ID, total.ID); v != "9" {
			t.Errorf("pedido %s tras vaciar la cola: %q", p.ID, v)
		}
	}
	if pag, _ := e.s.ListarRegistros(e.ctx, ana, pedidos.ID, "", 0, ""); pag.Recalculating {
		t.Error("la cola vacía sigue avisando")
	}
	// Una fórmula nueva en una tabla con más registros de los que caben.
	conteo := e.formula(ana, pedidos.ID, "Líneas", ref(enlace.ID)+" + 0", domain.TipoNumero)
	e.vaciarCola()
	for _, p := range ps {
		if v := e.valor("acme", p.ID, conteo.ID); v != "1" {
			t.Errorf("fórmula nueva en %s: %q", p.ID, v)
		}
	}
	// La cola es de cada organización: otra no puede anotar una tabla ajena (la clave
	// compuesta con el tenant lo impide al confirmar).
	if err := e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(e.ctx, tx, "globex"); err != nil {
			return err
		}
		_, err := tx.Exec(e.ctx, `INSERT INTO base_recalc (tenant, table_id, base_id) VALUES ('globex', $1, $2)`, pedidos.ID, e.base.ID)
		return err
	}); err == nil {
		t.Error("globex ha encolado una tabla de acme")
	}
}
