// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// sinPreferencias deja el resumen y la barra como los encuentra quien abre Calcus por
// primera vez, antes y después de la prueba: las pruebas comparten una sola app y lo
// recordado por una cambiaría lo que ven las demás.
func sinPreferencias(t *testing.T) {
	t.Helper()
	limpiar := func() {
		if p := prefsCalcus(); p != nil {
			p.RemoveValue(prefResumen)
			p.RemoveValue(prefFxDesplegada)
		}
	}
	limpiar()
	t.Cleanup(limpiar)
}

// TestLasMedidasDelResumenSonLasDeLaWeb son los casos de `leerMedidas` y
// `alternarMedida` de `sheets-resumen-barra.test.tsx`.
func TestLasMedidasDelResumenSonLasDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		guardado string
		quiero   []MedidaResumen
	}{
		{"", []MedidaResumen{MedidaSuma}},
		{"basura,,x", []MedidaResumen{MedidaSuma}},
		{"sum,min,min,average", []MedidaResumen{MedidaPromedio, MedidaMin, MedidaSuma}},
	} {
		if got := LeerMedidas(c.guardado); !slices.Equal(got, c.quiero) {
			t.Errorf("LeerMedidas(%q) = %v y la web da %v", c.guardado, got, c.quiero)
		}
	}
	if got := AlternarMedida([]MedidaResumen{MedidaSuma}, MedidaMax); !slices.Equal(got, []MedidaResumen{MedidaMax, MedidaSuma}) {
		t.Errorf("poner el máximo da %v", got)
	}
	if got := AlternarMedida([]MedidaResumen{MedidaMax, MedidaSuma}, MedidaSuma); !slices.Equal(got, []MedidaResumen{MedidaMax}) {
		t.Errorf("quitar la suma da %v", got)
	}
	if got := AlternarMedida([]MedidaResumen{MedidaSuma}, MedidaSuma); !slices.Equal(got, []MedidaResumen{MedidaSuma}) {
		t.Errorf("quitar la última deja %v: el resumen no puede quedarse vacío", got)
	}
}

// TestElResumenSeEligeEnSuMenuYSeRecuerda: la suma sola por defecto; pulsar el resumen
// abre las seis medidas con su cifra y su tic, marcar dos las añade en el orden de Excel
// sin cerrar el menú, y otro lienzo abierto después las conserva.
func TestElResumenSeEligeEnSuMenuYSeRecuerda(t *testing.T) {
	sinPreferencias(t)
	c := lienzoTraducido(t)
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	c.Libro.Escribir(0, 0, "1")
	c.Libro.Escribir(0, 1, "2")
	c.Libro.Escribir(0, 2, "x")
	c.Rejilla.Seleccionar(0, 0, false)
	c.Rejilla.Seleccionar(0, 2, true)
	p := c.Pestanas
	if got := p.Resumen(); got != "Suma: 3" {
		t.Fatalf("sin elegir nada el resumen es %q y debe ser la suma sola", got)
	}
	if p.resumen.Hidden || p.resumen.AccessibilityLabel() != "Suma: 3" {
		t.Fatalf("el botón del resumen no está o se anuncia como %q", p.resumen.AccessibilityLabel())
	}
	// Va pegado a la derecha, a `px-2` del borde, como el `<output>` de antes.
	if fin, borde := p.resumen.Position().X+p.resumen.Size().Width, p.Size().Width-MargenPestanas; fin != borde {
		t.Errorf("el resumen acaba en x=%v y la barra en %v", fin, borde)
	}
	test.Tap(p.resumen)
	m := p.MenuResumen()
	if m == nil {
		t.Fatal("pulsar el resumen no abre su menú")
	}
	textos := make([]string, 0, len(MedidasResumen))
	for _, e := range elementosDe(m) {
		textos = append(textos, e.Texto)
	}
	quiero := []string{"Promedio: 1.5", "Recuento: 3", "Recuento numérico: 2", "Mín.: 1", "Máx.: 2", "Suma: 3"}
	if !slices.Equal(textos, quiero) {
		t.Errorf("el menú ofrece %q y la web %q", textos, quiero)
	}
	// La suma, única elegida, va marcada y con la tinta de las demás (apagada salía en
	// gris, a ~2,5:1); pulsarla no la quita.
	if e := elementoMenu(t, m, "Suma: 3"); !e.Marcada || e.Inerte {
		t.Error("la suma, única elegida, va marcada y sin apagar")
	}
	test.Tap(elementoMenu(t, m, "Suma: 3"))
	if got := p.Resumen(); got != "Suma: 3" {
		t.Errorf("pulsar la única elegida deja el resumen en %q", got)
	}
	test.Tap(elementoMenu(t, m, "Máx.: 2"))
	test.Tap(elementoMenu(t, m, "Recuento: 3"))
	if got := p.Resumen(); got != "Recuento: 3 · Máx.: 2 · Suma: 3" {
		t.Errorf("con recuento y máximo el resumen es %q", got)
	}
	if p.MenuResumen() == nil {
		t.Fatal("marcar una medida cierra el menú, y lo normal es marcar varias")
	}
	if e := elementoMenu(t, p.MenuResumen(), "Máx.: 2"); !e.Marcada {
		t.Error("el máximo elegido no lleva su tic")
	}
	if e := elementoMenu(t, p.MenuResumen(), "Suma: 3"); e.Inerte {
		t.Error("con tres elegidas, la suma ya se puede quitar")
	}
	guarda(t, w.Canvas().Capture()) // con `SYTUE_CAPTURA`, el menú abierto
	p.MenuResumen().Cerrar()

	otro := lienzoTraducido(t)
	if got := otro.Pestanas.Medidas(); !slices.Equal(got, []MedidaResumen{MedidaRecuento, MedidaMax, MedidaSuma}) {
		t.Errorf("otro libro abierto después enseña %v: lo elegido no se ha recordado", got)
	}
}

// TestLaBarraFxSeDespliegaYSeRecuerda: el ▾ del final de la barra la despliega en un
// campo de tres líneas —la rejilla baja lo que crece—, el texto pasa tal cual, Intro
// sigue escribiendo la celda, se recuerda en el dispositivo y el ▴ la recoge.
func TestLaBarraFxSeDespliegaYSeRecuerda(t *testing.T) {
	sinPreferencias(t)
	c := lienzoTraducido(t)
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	largo := "=SUM(A1:A2)+" + strings.Repeat("A3*2+", 30) + "1"
	c.Libro.Escribir(1, 0, largo)
	c.Rejilla.Seleccionar(1, 0, false)
	b := c.Barra
	if b.Desplegada() || b.Size().Height != AltoBarra || b.boton.Rotulo != "Desplegar la barra de fórmulas" {
		t.Fatalf("la barra empieza desplegada (%v), mide %v o su ▾ dice %q",
			b.Desplegada(), b.Size().Height, b.boton.Rotulo)
	}
	// El ▾ es el botón de la cinta, a `px-2` del borde derecho.
	if x, quiero := b.boton.Position().X, b.Size().Width-MargenBarra-editor.LadoBotonCinta; x != quiero {
		t.Errorf("el ▾ cae en x=%v y no en %v", x, quiero)
	}
	test.Tap(b.boton)
	if !b.Desplegada() || b.ed != b.eds[1] || !b.ed.MultiLine {
		t.Fatal("el ▾ no despliega la barra en el campo de varias líneas")
	}
	if b.Size().Height != AltoBarraDesplegada || c.Rejilla.Position().Y != AltoBarraDesplegada {
		t.Errorf("desplegada mide %v y la rejilla empieza en %v; las dos tienen que ser %v",
			b.Size().Height, c.Rejilla.Position().Y, AltoBarraDesplegada)
	}
	if b.ed.Texto() != largo || b.boton.Rotulo != "Recoger la barra de fórmulas" {
		t.Errorf("desplegada lleva %q y su ▴ dice %q", b.ed.Texto(), b.boton.Rotulo)
	}
	// El campo de tres líneas reparte la fórmula: el cursor puesto por posición cae
	// donde se pide aunque esté en la tercera línea visual.
	n := utf8.RuneCountInString(largo) - 3
	b.ed.PonerCursor(n)
	if got := b.ed.CursorTextOffset(); got != n || b.ed.CursorRow == 0 {
		t.Errorf("el cursor pedido en %d cae en %d (fila visual %d)", n, got, b.ed.CursorRow)
	}
	guarda(t, w.Canvas().Capture()) // con `SYTUE_CAPTURA`, la barra desplegada

	// Se recuerda: otro libro la abre desplegada.
	if otro := lienzoTraducido(t); !otro.Barra.Desplegada() {
		t.Error("otro libro abierto después no recuerda la barra desplegada")
	}

	// Intro escribe la celda y baja, como en la de una línea.
	c.Rejilla.Seleccionar(0, 0, false)
	w.Canvas().Focus(b.ed)
	b.ed.PonerTexto("hola")
	b.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := c.Libro.Crudo(0, 0); got != "hola" || strings.Contains(b.ed.Texto(), "\n") {
		t.Errorf("Intro en la barra desplegada deja %q en A1 y %q en la barra", got, b.ed.Texto())
	}
	if s := c.Rejilla.Seleccion(); s.F != 1 {
		t.Errorf("tras Intro la selección está en la fila %d y tiene que bajar a la 2", s.F+1)
	}

	// Ni el tabulador ni lo pegado con saltos de línea meten nada que una celda no
	// tenga: el tabulador pasa el foco y los saltos se pegan como espacios.
	w.Canvas().Focus(b.ed)
	b.ed.PonerTexto("=A1")
	b.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if strings.Contains(b.ed.Texto(), "\t") || w.Canvas().Focused() == b.ed {
		t.Errorf("el tabulador en la barra desplegada deja %q y el foco sigue en ella", b.ed.Texto())
	}
	w.Canvas().Focus(b.ed)
	b.ed.PonerTexto("=")
	b.ed.TypedShortcut(&fyne.ShortcutPaste{Clipboard: portapapelesFijo("A1\r\n+B1\n")})
	if got := b.ed.Texto(); got != "=A1 +B1 " {
		t.Errorf("pegar con saltos de línea en la barra desplegada deja %q", got)
	}

	test.Tap(b.boton)
	if b.Desplegada() || b.ed != b.eds[0] || c.Rejilla.Position().Y != AltoBarra {
		t.Error("el ▴ no recoge la barra")
	}
	if fxDesplegadaRecordada() {
		t.Error("recogida, el dispositivo la sigue recordando desplegada")
	}
}

// TestDesplegarConservaLaFormulaEnCurso: si se estaba escribiendo una fórmula en la
// barra, desplegarla no la da por acabada: el foco, el texto y el cursor pasan al campo
// nuevo y la lista de funciones sigue viva.
func TestDesplegarConservaLaFormulaEnCurso(t *testing.T) {
	sinPreferencias(t)
	c := lienzoTraducido(t)
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	b := c.Barra
	w.Canvas().Focus(b.ed)
	for _, r := range "=SU" {
		b.ed.TypedRune(r)
	}
	if c.fx == nil || !c.fx.barra {
		t.Fatal("escribir «=SU» en la barra no empieza la edición de fórmula")
	}
	b.Desplegar(true)
	if w.Canvas().Focused() != b.ed {
		t.Fatal("al desplegar, el foco no pasa al campo nuevo")
	}
	if c.fx == nil || c.fx.texto != "=SU" || c.fx.cursor != 3 {
		t.Fatalf("al desplegar, la fórmula en curso queda en %+v", c.fx)
	}
	b.ed.TypedRune('M')
	if c.fx == nil || c.fx.texto != "=SUM" {
		t.Errorf("lo tecleado en el campo desplegado no llega a la fórmula: %+v", c.fx)
	}
}
