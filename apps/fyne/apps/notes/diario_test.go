// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package notes

import (
	"reflect"
	"testing"
	"time"

	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestCapturaRapidaRespetaMarcadoresYEscapaHTML(t *testing.T) {
	for _, c := range []struct {
		texto, tipo, titulo string
		hecha               bool
	}{
		{"- [X] Comprar pan", "text", "Comprar pan", true},
		{"[ ] Revisar", "text", "Revisar", false},
		{"Llamar", "todo", "Llamar", false},
	} {
		in, ok := capturaRapida(c.texto, c.tipo)
		if !ok || in.Kind != "todo" || in.Title != c.titulo || in.Checked == nil || *in.Checked != c.hecha {
			t.Errorf("%q: %+v", c.texto, in)
		}
	}
	in, ok := capturaRapida("<img src=x onerror=alert(1)>", "text")
	if !ok || in.Content != "<p>&lt;img src=x onerror=alert(1)&gt;</p>" {
		t.Fatalf("HTML no escapado: %+v", in)
	}
	if _, ok := capturaRapida("  ", "text"); ok {
		t.Fatal("el vacío crea una nota")
	}
}

func TestEnfoquesMantienenAntepasadosYTerminanCiclos(t *testing.T) {
	notas := []Nota{{ID: "group", Tipo: "text"}, {ID: "todo", Tipo: "todo", Grupo: "group"}, {ID: "done", Tipo: "todo", Marcada: true}, {ID: "pin", Tipo: "text", Fijada: true, Grupo: "loop"}, {ID: "loop", Tipo: "text", Grupo: "pin"}}
	for foco, esperado := range map[string][]string{"pending": {"group", "todo"}, "done": {"done"}, "pinned": {"pin", "loop"}} {
		var ids []string
		for _, n := range enfocarNotas(notas, foco, false) {
			ids = append(ids, n.ID)
		}
		if !reflect.DeepEqual(ids, esperado) {
			t.Errorf("%s: %v, quería %v", foco, ids, esperado)
		}
	}
}

func TestLaSintaxisDeTwos(t *testing.T) {
	for texto, titulo := range map[string]string{"- Comprar pan": "Comprar pan", "[] Revisar": "Revisar"} {
		in, ok := capturaRapida(texto, "text")
		if !ok || in.Kind != "todo" || in.Title != titulo || in.Checked == nil || *in.Checked {
			t.Errorf("%q: %+v", texto, in)
		}
	}
	if in, _ := capturaRapida("---", "text"); in.Kind != "text" || in.Content != "<hr>" {
		t.Errorf("---: %+v", in)
	}
	for _, texto := range []string{"-5 grados", "- uno\n- dos", "----", "-\nComprar"} {
		if in, _ := capturaRapida(texto, "text"); in.Kind != "text" {
			t.Errorf("%q no es texto: %+v", texto, in)
		}
	}
}

func TestDdYTtSeExpandenAlTeclearlos(t *testing.T) {
	ahora := time.Date(2026, 10, 2, 9, 5, 0, 0, time.Local)
	for de, a := range map[string]string{
		"dd ": "2026-10-02 ", "Llamar tt ": "Llamar 09:05 ", "add ": "add ", "dd": "dd", "dd y más ": "dd y más ",
	} {
		if x := expandirAtajos(de, ahora); x != a {
			t.Errorf("%q: %q, quería %q", de, x, a)
		}
	}
}

func TestBorrarNoExpande(t *testing.T) {
	ahora := time.Date(2026, 10, 2, 9, 5, 0, 0, time.Local)
	for _, c := range [][3]string{{"dd x", "dd ", "dd "}, {"dd", "dd ", "2026-10-02 "}, {"", "dd ", "dd "}} {
		if x := expandirCaptura(c[0], c[1], ahora); x != c[2] {
			t.Errorf("%q → %q: %q, quería %q", c[0], c[1], x, c[2])
		}
	}
}

// El diario de otra persona compartido conmigo no se abre por «Hoy»: se queda en el
// árbol y no pone punto.
func TestElArbolYLaSemanaConDiarios(t *testing.T) {
	yo := func(s string) bool { return s == "yo" }
	cs := []Cuaderno{
		{ID: "a", Notas: 3},
		{ID: "d1", Notas: 2, Crudo: api.Notebook{DailyDate: "2026-10-01", OwnerSub: "yo"}},
		{ID: "d2", Crudo: api.Notebook{DailyDate: "2026-10-02", OwnerSub: "yo"}},
		{ID: "d3", Crudo: api.Notebook{DailyDate: "2026-10-03", OwnerSub: "yo"}},
		{ID: "hijo", Padre: "d2"},
		{ID: "ajeno", Notas: 5, Crudo: api.Notebook{DailyDate: "2026-10-03", OwnerSub: "otra"}},
	}
	ids := make([]string, 0, len(cs))
	for _, c := range cuadernosDelArbol(cs, yo) {
		ids = append(ids, c.ID)
	}
	if !reflect.DeepEqual(ids, []string{"a", "d2", "hijo", "ajeno"}) {
		t.Errorf("árbol: %v", ids)
	}
}

func TestLosAtajosDelDiarioSoloValenConUnDiaAbierto(t *testing.T) {
	p, _ := montarCaptura(t, 1280, 800, "list")
	var dia string
	hoy := 0
	p.lateral.AlDia = func(d string) { dia = d }
	p.lateral.AlHoy = func() { hoy++ }
	if p.atajoDiario("next") || p.atajoDiario("capture") {
		t.Fatal("un cuaderno normal no tiene días")
	}
	if !p.atajoDiario("today") || hoy != 1 {
		t.Fatal("⌘D abre hoy desde cualquier sitio")
	}
	p.activo = DeCuaderno(api.Notebook{ID: "d", Layout: "list", DailyDate: "2026-10-31"})
	p.vista = VistaCuaderno
	if !p.atajoDiario("next") || dia != "2026-11-01" {
		t.Fatalf("siguiente: %q", dia)
	}
	if !p.atajoDiario("previous") || dia != "2026-10-30" {
		t.Fatalf("anterior: %q", dia)
	}
	pop := widget.NewPopUp(widget.NewLabel("menú"), p.win.Canvas())
	pop.Show()
	if p.atajoDiario("next") || p.atajoDiario("today") || hoy != 1 {
		t.Fatal("con algo encima no se cambia el día de detrás")
	}
	pop.Hide()
}

// Renombrar un texto sin título del diario saca el campo: en línea, la cabecera era el
// cuerpo y el campo no aparecía nunca.
func TestRenombrarUnTextoSinTituloDelDiario(t *testing.T) {
	tj := NuevaTarjeta(trInterno(t), DeNota(api.Note{ID: "n", Kind: "text", Content: "<p>hola</p>"}))
	tj.Diario = true
	if !tj.enLinea() {
		t.Fatal("un texto sin título del diario va en línea")
	}
	tj.Renombrar = NuevoCampoEnLinea("", "", PYCampoTarjeta, "bg", 0)
	if _, titulo, _, _ := tj.cabecera(tema.Claro); titulo != tj.Renombrar {
		t.Fatalf("renombrando, la cabecera no lleva el campo: %T", titulo)
	}
}

// Las mismas tres comprobaciones que client/diario.test.ts.
func TestLasCanceladasNoSonPendientesYOcultarConservaAntepasados(t *testing.T) {
	notas := []Nota{
		{ID: "padre", Tipo: "todo", Marcada: true},
		{ID: "hija", Tipo: "todo", Grupo: "padre"},
		{ID: "cancelada", Tipo: "todo", Cancelada: true},
		{ID: "hecha", Tipo: "todo", Marcada: true},
		{ID: "texto", Tipo: "text"},
	}
	for _, c := range []struct {
		foco     string
		ocultar  bool
		esperado []string
	}{
		{"pending", false, []string{"padre", "hija"}},
		{"done", false, []string{"padre", "hecha"}},
		{"all", true, []string{"padre", "hija", "texto"}},
	} {
		var ids []string
		for _, n := range enfocarNotas(notas, c.foco, c.ocultar) {
			ids = append(ids, n.ID)
		}
		if !reflect.DeepEqual(ids, c.esperado) {
			t.Errorf("%s/%v: %v, quería %v", c.foco, c.ocultar, ids, c.esperado)
		}
	}
	if trasladosDe([]byte(`{"carry_count":3}`)) != 3 || trasladosDe([]byte(`{"carry_count":"3"}`)) != 0 {
		t.Error("carry_count mal leído")
	}
}
