// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package notes

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La semana de lunes a domingo y el resumen de una línea son los de client/diario.ts.
func TestSemanaYResumenComoLaWeb(t *testing.T) {
	jueves := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	s := semanaDe(jueves)
	if s[0].Format(time.DateOnly) != "2026-09-28" || s[6].Format(time.DateOnly) != "2026-10-04" {
		t.Fatalf("semana %v", s)
	}
	if semanaDe(s[0])[0] != s[0] {
		t.Fatal("el lunes no es el primer día de su semana")
	}
	casos := []struct{ titulo, cuerpo, quiero string }{
		{" Comprar pan ", "<p>x</p>", "Comprar pan"},
		{"", "<h2>Plan</h2><p>Ir &amp; volver&nbsp;<b>hoy</b></p>", "Plan Ir & volver hoy"},
		{"", "<p>&lt;script&gt;</p>", "<script>"},
	}
	for _, c := range casos {
		if got := resumenNota(c.titulo, c.cuerpo, maxResumen); got != c.quiero {
			t.Errorf("resumenNota(%q, %q) = %q", c.titulo, c.cuerpo, got)
		}
	}
	if got := resumenNota("", "<p>"+strings.Repeat("a", 200)+"</p>", 10); got != "aaaaaaaaa…" {
		t.Errorf("recorte: %q", got)
	}
}

// El punto de cada día: el abierto cuenta lo que hay en pantalla; los demás, lo cargado.
func TestMarcasDeLaSemana(t *testing.T) {
	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1", Kind: "todo"},
		api.Note{ID: "b", NotebookID: "c1", Kind: "text"})
	hoy := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	p.dia.marcas = map[string]api.DailyDay{"2026-09-30": {Date: "2026-09-30", Total: 2}}
	// Un diario ajeno no es el mío de ese día: su punto sale de lo cargado (nada).
	if m, _ := p.marcaDia(hoy, hoy); m != 0 {
		t.Fatalf("hoy, diario ajeno: %d", m)
	}
	p.Yo = func() string { return "yo" }
	p.activo.Crudo.OwnerSub, p.lienzo.Fase = "yo", Exito
	if m, s := p.marcaDia(hoy, hoy); m != 2 || !strings.Contains(s, "2") {
		t.Fatalf("hoy: %d %q", m, s)
	}
	// Mientras carga, las notas en pantalla aún son las del día anterior.
	p.lienzo.Fase = Cargando
	if m, _ := p.marcaDia(hoy, hoy); m != 0 {
		t.Fatalf("hoy cargando: %d", m)
	}
	p.lienzo.Fase = Exito
	p.notas[0].Cancelada = true
	if m, _ := p.marcaDia(hoy, hoy); m != 1 {
		t.Fatalf("cancelada cuenta como pendiente: %d", m)
	}
	if m, _ := p.marcaDia(hoy.AddDate(0, 0, -1), hoy); m != 1 {
		t.Fatalf("ayer sin pendientes: %d", m)
	}
	if m, s := p.marcaDia(hoy.AddDate(0, 0, 1), hoy); m != 0 || s != "" {
		t.Fatalf("mañana vacío: %d %q", m, s)
	}
}

// Soltar sobre un día abre su diario SIN trasladar pendientes y mueve allí la nota.
func TestMoverADia(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"POST /daily": `{"id":"c9","title":"2 de octubre","daily_date":"2026-10-02"}`,
	}, api.Note{ID: "a", NotebookID: "c1"}, api.Note{ID: "b", NotebookID: "c1"})
	if err := p.moverADia("a", "2026-10-02"); err != nil {
		t.Fatal(err)
	}
	if d := falso.ultima(t, "POST /daily"); d["date"] != "2026-10-02" || d["carry"] != false {
		t.Errorf("abre el día con %v", d)
	}
	if m := falso.ultima(t, "POST /notes/a/move"); m["notebook_id"] != "c9" {
		t.Errorf("mueve con %v", m)
	}
	if ids(p.notas) != "b" || len(p.cuadernos) != 3 || p.dia.version == 0 {
		t.Fatalf("notas %s, cuadernos %d, versión %d", ids(p.notas), len(p.cuadernos), p.dia.version)
	}
}

// Las capturas del diario por días: Día (con agenda y «Este día»), Semana y Días, en
// claro y en oscuro, a 1280 y a 390. Sin `SYTUE_CAPTURAS` solo comprueban que pintan.
func TestCapturasDelDiarioPorDias(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	hoy := "2026-10-01"
	nota := func(id, dia, kind, titulo, cuerpo string, hecha bool) api.Note {
		return api.Note{ID: id, NotebookID: "d-" + dia, Kind: kind, Title: titulo, Content: cuerpo, Checked: hecha}
	}
	dias := map[string]api.DailyDay{
		"2026-09-28": {Date: "2026-09-28", Total: 2, Pending: 1, Notes: []api.Note{
			nota("a", "2026-09-28", "todo", "Enviar el presupuesto", "", false),
			nota("b", "2026-09-28", "text", "", "<p>Llamar a Marta por la reunión del jueves.</p>", false),
		}},
		hoy: {Date: hoy, Total: 2, Notes: []api.Note{
			nota("c", hoy, "todo", "Revisar el contrato", "", true),
			nota("d", hoy, "text", "", "<p>Ideas para la presentación: tres bloques y una demo.</p>", false),
		}},
		"2026-10-03": {Date: "2026-10-03", Total: 1, Pending: 1, Notes: []api.Note{nota("e", "2026-10-03", "todo", "Comprar fruta", "", false)}},
	}
	casos := []struct {
		nombre      string
		ancho, alto float32
		vista       string
	}{
		{"dia", 1280, 800, VistaDia},
		{"semana", 1280, 800, VistaSem},
		{"dias", 1280, 800, VistaDias},
		{"movil-dia", 390, 844, VistaDia},
		{"movil-semana", 390, 844, VistaSem},
	}
	for _, variante := range []string{"claro", "oscuro"} {
		for _, c := range casos {
			t.Run(variante+"-"+c.nombre, func(t *testing.T) {
				tema.Elegir(variante)
				p, w := montarCaptura(t, c.ancho, c.alto, "list")
				d := DeCuaderno(api.Notebook{ID: "d-" + hoy, Title: "1 de octubre de 2026", Layout: "list", DailyDate: hoy, Color: "#86198f"})
				p.cuadernos, p.activo = append(p.cuadernos, d), d
				p.lienzo.Cuaderno = d
				p.notas = DeNotas(dias[hoy].Notes)
				p.dia.vista, p.dia.dias = c.vista, dias
				p.dia.marcas = map[string]api.DailyDay{"2026-09-28": dias["2026-09-28"], "2026-10-03": dias["2026-10-03"]}
				p.dia.este = []api.DailyDay{{Date: "2025-10-01", Total: 1, Notes: []api.Note{nota("f", "2025-10-01", "text", "", "<p>Primer día en la oficina nueva.</p>", false)}}}
				p.dia.agenda = []api.EventoAgenda{
					{ID: "u1", Nombre: "Festivo local", DiaCompleto: true},
					{ID: "u2", Nombre: "Comité de dirección", Inicio: time.Date(2026, 10, 1, 9, 30, 0, 0, time.Local)},
				}
				p.Suite = &api.Elegir{}
				p.Repintar()
				if w.Canvas().Capture().Bounds().Dx() == 0 {
					t.Fatal("la captura ha salido vacía")
				}
				capturar(t, w, "diario-"+variante+"-"+c.nombre)
			})
		}
	}
}
