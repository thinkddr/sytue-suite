// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// lienzoGrabador es un [Lienzo] que apunta lo que le mandan.
type lienzoGrabador struct{ vistos []string }

func (l *lienzoGrabador) Objeto() fyne.CanvasObject     { return ui.NuevoVacio("", "") }
func (l *lienzoGrabador) Cargar(string) bool            { return true }
func (l *lienzoGrabador) Serializar() string            { return "" }
func (l *lienzoGrabador) Estadisticas() (int, int, int) { return 1, 1, 0 }
func (l *lienzoGrabador) Formato() Formato              { return Formato{} }
func (l *lienzoGrabador) Mandar(c Comando, v string)    { l.vistos = append(l.vistos, string(c)+"="+v) }

// nuevaGrabadora aísla cada test: el grabador es común a todo el proceso.
func nuevaGrabadora(t *testing.T) {
	t.Helper()
	antes := grabadora
	grabadora = &macros{}
	t.Cleanup(func() { grabadora = antes })
}

func grabar(c *Contexto, cmds ...[2]string) {
	grabadora.empezar()
	for _, x := range cmds {
		c.Mandar(Comando(x[0]), x[1])
	}
}

func TestGrabarYReproducir(t *testing.T) {
	nuevaGrabadora(t)
	lz := &lienzoGrabador{}
	c := &Contexto{Tipo: Documento, Lienzo: lz}
	grabar(c, [2]string{"bold", ""}, [2]string{"color", "#DC2626"}, [2]string{"undo", ""}, [2]string{"heading", "h2"})
	pasos, descartados := grabadora.terminar()
	// «undo» no se graba: deshacer corrompería la reproducción.
	if len(pasos) != 3 || descartados != 1 {
		t.Fatalf("pasos=%d descartados=%d; quería 3 y 1", len(pasos), descartados)
	}
	if !grabadora.guardar("Mía", office.Documento, pasos) {
		t.Fatal("no se pudo guardar")
	}
	lz.vistos = nil
	if r, _ := c.reproducirMacro(grabadora.lista()[0]); r != office.Valido {
		t.Fatalf("reproducir: %q", r)
	}
	if want := []string{"bold=", "color=#DC2626", "heading=h2"}; !slices.Equal(lz.vistos, want) {
		t.Errorf("llegaron %v y quería %v, en orden", lz.vistos, want)
	}
	// Y reproducir no se graba a sí misma.
	grabadora.empezar()
	_, _ = c.reproducirMacro(grabadora.lista()[0])
	if p, _ := grabadora.terminar(); len(p) != 0 {
		t.Errorf("reproducir se grabó a sí misma: %v", p)
	}
}

func TestReproducirNoDespachaNadaSiUnPasoFalla(t *testing.T) {
	nuevaGrabadora(t)
	lz := &lienzoGrabador{}
	c := &Contexto{Tipo: Documento, Lienzo: lz}
	m := macro{id: "x", nombre: "X", tipo: office.Documento, pasos: []office.Paso{
		{Editor: office.Documento, Cmd: "bold"},
		{Editor: office.Documento, Cmd: "font", Valor: "Papyrus", ConValor: true},
	}}
	if r, i := c.reproducirMacro(m); r != office.ValorMalo || i != 1 {
		t.Fatalf("veredicto %q en %d; quería valor en 1", r, i)
	}
	if len(lz.vistos) != 0 {
		t.Errorf("se despachó %v antes de saber que fallaba: una macro a medias deja el documento en un estado que nadie pidió", lz.vistos)
	}
}

func TestReproducirNoEditaEnSoloLectura(t *testing.T) {
	nuevaGrabadora(t)
	lz := &lienzoGrabador{}
	c := &Contexto{Tipo: Documento, Lienzo: lz, SoloLectura: true}
	m := macro{id: "x", nombre: "X", tipo: office.Documento, pasos: []office.Paso{{Editor: office.Documento, Cmd: "bold"}}}
	if r, _ := c.reproducirMacro(m); r != office.SoloLectura {
		t.Fatalf("veredicto %q; quería soloLectura", r)
	}
	if len(lz.vistos) != 0 {
		t.Errorf("en solo lectura llegó %v al lienzo", lz.vistos)
	}
}

func TestUnaMacroDeOtroTipoNoSeReproduce(t *testing.T) {
	nuevaGrabadora(t)
	c := &Contexto{Tipo: Hoja, Lienzo: &lienzoGrabador{}}
	m := macro{id: "x", nombre: "X", tipo: office.Documento, pasos: []office.Paso{{Editor: office.Documento, Cmd: "bold"}}}
	if r, _ := c.reproducirMacro(m); r != office.OtroTipo {
		t.Fatalf("veredicto %q; quería tipo", r)
	}
}

func TestLosTechosDeLaGrabadora(t *testing.T) {
	nuevaGrabadora(t)
	c := &Contexto{Tipo: Documento, Lienzo: &lienzoGrabador{}}
	grabadora.empezar()
	for i := 0; i < office.MaxPasos+7; i++ {
		c.Mandar(Negrita, "")
	}
	pasos, descartados := grabadora.terminar()
	if len(pasos) != office.MaxPasos || descartados != 7 {
		t.Errorf("pasos=%d descartados=%d; quería %d y 7", len(pasos), descartados, office.MaxPasos)
	}
	for i := 0; i < office.MaxMacros; i++ {
		if !grabadora.guardar("m", office.Documento, pasos[:1]) {
			t.Fatalf("la macro %d debería caber", i)
		}
	}
	if grabadora.guardar("una más", office.Documento, pasos[:1]) {
		t.Errorf("la macro %d pasa el techo de %d", office.MaxMacros+1, office.MaxMacros)
	}
}

// TestSinGrabarNoSeApuntaNada: el gancho de Mandar fuera de grabación es un `if` que vuelve.
func TestSinGrabarNoSeApuntaNada(t *testing.T) {
	nuevaGrabadora(t)
	c := &Contexto{Tipo: Documento, Lienzo: &lienzoGrabador{}}
	c.Mandar(Negrita, "")
	if len(grabadora.pasos) != 0 {
		t.Errorf("sin grabar se apuntó %v", grabadora.pasos)
	}
}
