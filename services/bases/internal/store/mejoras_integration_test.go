// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"encoding/json"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Duplicar copia valores y enlaces en un registro NUEVO de la misma tabla, sin los
// adjuntos; quien solo lee no puede, y la copia cuenta para el límite de la tabla.
func TestDuplicarRegistro(t *testing.T) {
	e := nuevo(t)
	pedidos, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Pedidos")
	e.ok(err)
	cli, err := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "Cliente", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	e.ok(err)
	nota, err := e.s.CrearCampo(e.ctx, ana, pedidos.ID, domain.EntradaCampo{Name: "Nota", Kind: domain.TipoTexto})
	e.ok(err)
	c1, err := e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, dato(e.txt.ID, `"Panadería"`), nil)
	e.ok(err)
	p1, err := e.s.CrearRegistro(e.ctx, ana, pedidos.ID, dato(nota.ID, `"urgente"`), map[string][]string{cli.ID: {c1.ID}})
	e.ok(err)

	copia, err := e.s.DuplicarRegistro(e.ctx, ana, p1.ID)
	e.ok(err)
	if copia.ID == p1.ID || copia.TableID != pedidos.ID {
		t.Fatalf("la copia es otro registro de la misma tabla: %+v", copia)
	}
	if string(copia.Data[nota.ID]) != `"urgente"` || len(copia.Links[cli.ID]) != 1 || copia.Links[cli.ID][0] != c1.ID {
		t.Errorf("la copia no lleva valores y enlaces: %+v", copia)
	}
	_, err = e.s.DuplicarRegistro(e.ctx, gus, p1.ID)
	esperar(t, err, domain.ErrNoExiste, "otra organización")
}

// El pie suma TODO lo que pasa la vista, no la ventana cargada, y respeta la búsqueda.
func TestResumirVista(t *testing.T) {
	e := nuevo(t)
	for _, f := range []struct{ nombre, importe string }{{`"Ana"`, `10`}, {`"Bea"`, `5.5`}, {`"Cleo"`, `null`}} {
		_, err := e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, dato(e.txt.ID, f.nombre, e.num.ID, f.importe), nil)
		e.ok(err)
	}
	r, err := e.s.Resumir(e.ctx, ana, e.tabl.ID, "", LecturaAdHoc{})
	e.ok(err)
	num := r.Fields[e.num.ID]
	if r.Count != 3 || num.Filled != 2 || num.Sum == nil || *num.Sum != 15.5 || *num.Min != 5.5 || *num.Max != 10 {
		t.Errorf("resumen: %+v %+v", r, num)
	}
	r, err = e.s.Resumir(e.ctx, ana, e.tabl.ID, "", LecturaAdHoc{Search: "bea"})
	e.ok(err)
	if r.Count != 1 || *r.Fields[e.num.ID].Sum != 5.5 {
		t.Errorf("con búsqueda: %+v", r)
	}
	_, err = e.s.Resumir(e.ctx, gus, e.tabl.ID, "", LecturaAdHoc{})
	esperar(t, err, domain.ErrNoExiste, "otra organización")
}

// Exportar una vista baja lo que la vista enseña; sin vista, la tabla entera.
func TestExportarVista(t *testing.T) {
	e := nuevo(t)
	for _, n := range []string{`1`, `20`, `300`} {
		_, err := e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, dato(e.num.ID, n), nil)
		e.ok(err)
	}
	v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Grandes", domain.VistaRejilla, domain.ConfigVista{
		Filters: []domain.Filtro{{Field: e.num.ID, Op: "gt", Value: json.RawMessage(`10`)}},
	})
	e.ok(err)
	contar := func(vista string) int {
		n := 0
		e.ok(e.s.ExportarVista(e.ctx, ana, e.tabl.ID, vista, LecturaAdHoc{}, func(ex Exportacion) error { n += len(ex.Records); return nil }))
		return n
	}
	if got := contar(v.ID); got != 2 {
		t.Errorf("la vista exporta %d registros, quería 2", got)
	}
	if got := contar(""); got != 3 {
		t.Errorf("sin vista exporta %d, quería 3", got)
	}
}

// El defecto llega a las altas que no traen el campo, también desde un formulario
// público, y «hoy» es el día que dice quien da el alta.
func TestValoresPorDefecto(t *testing.T) {
	e := nuevo(t)
	estado, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Estado", Kind: domain.TipoSeleccion, Choices: []string{"Nuevo", "Hecho"}})
	e.ok(err)
	alta, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Alta", Kind: domain.TipoFecha})
	e.ok(err)
	e.ok(e.s.CambiarDefecto(e.ctx, ana, estado.ID, json.RawMessage(`"Nuevo"`)))
	e.ok(e.s.CambiarDefecto(e.ctx, ana, alta.ID, json.RawMessage(`"today"`)))
	esperar(t, e.s.CambiarDefecto(e.ctx, ana, estado.ID, json.RawMessage(`"Otro"`)), domain.ErrInvalido, "opción que no existe")
	esperar(t, e.s.CambiarDefecto(e.ctx, bea, estado.ID, json.RawMessage(`"Hecho"`)), domain.ErrNoExiste, "quien no es propietaria")

	hoy := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	r, err := e.s.CrearRegistroEl(e.ctx, ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`), nil, hoy)
	e.ok(err)
	if string(r.Data[estado.ID]) != `"Nuevo"` || string(r.Data[alta.ID]) != `"2026-11-25"` {
		t.Errorf("alta con defectos: %v", r.Data)
	}
	// Un null escrito a propósito se respeta.
	r, err = e.s.CrearRegistroEl(e.ctx, ana, e.tabl.ID, dato(estado.ID, `null`), nil, hoy)
	e.ok(err)
	if _, ok := r.Data[estado.ID]; ok {
		t.Errorf("null a propósito: %v", r.Data)
	}
	e.ok(e.s.CambiarDefecto(e.ctx, ana, estado.ID, json.RawMessage(`null`)))
	r, err = e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, nil, nil)
	e.ok(err)
	if _, ok := r.Data[estado.ID]; ok {
		t.Errorf("defecto quitado: %v", r.Data)
	}
}

// Un formulario con un campo obligatorio rechaza el envío que no lo trae, también si se
// llama a la API sin la página; y deja de serlo si lo quitan del formulario.
func TestFormularioConObligatorios(t *testing.T) {
	e := nuevo(t)
	v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Alta", domain.VistaFormulario,
		domain.ConfigVista{Fields: []string{e.txt.ID, e.num.ID}, Required: []string{e.txt.ID}})
	e.ok(err)
	_, token, err := e.s.CrearEnlaceFormulario(e.ctx, ana, v.ID, 10, nil)
	e.ok(err)
	f, err := e.s.FormularioPorToken(e.ctx, token)
	e.ok(err)
	if len(f.Required) != 1 || f.Required[0] != e.txt.ID {
		t.Fatalf("obligatorios públicos: %v", f.Required)
	}
	esperar(t, e.s.EnviarFormulario(e.ctx, token, dato(e.num.ID, `3`)), domain.ErrInvalido, "sin el obligatorio")
	e.ok(e.s.EnviarFormulario(e.ctx, token, dato(e.txt.ID, `"Ana"`)))
}

// «Últimos 7 días» cuenta hoy y los seis de antes, con el día de quien mira.
func TestFiltroDeFechaRelativo(t *testing.T) {
	e := nuevo(t)
	alta, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Alta", Kind: domain.TipoFecha})
	e.ok(err)
	for _, d := range []string{`"2026-11-25"`, `"2026-11-19"`, `"2026-11-18"`, `"2026-12-01"`} {
		_, err := e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, dato(alta.ID, d), nil)
		e.ok(err)
	}
	v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Semana", domain.VistaRejilla, domain.ConfigVista{
		Filters: []domain.Filtro{{Field: alta.ID, Op: domain.OpUltimosDias, Value: json.RawMessage(`7`)}},
	})
	e.ok(err)
	p, err := e.s.ListarRegistrosCon(e.ctx, ana, e.tabl.ID, v.ID, 100, "", LecturaAdHoc{Today: "2026-11-25"})
	e.ok(err)
	if len(p.Records) != 2 {
		t.Errorf("últimos 7 días desde el 25: %d registros, quería 2", len(p.Records))
	}
}
