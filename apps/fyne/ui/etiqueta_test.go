// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestEtiquetaMideLoQueTag: las cifras son las de Tag.tsx con el rem de 16, escritas a
// mano. Sin texto, para que no dependan de la fuente:
//
//   - sin aspa: 1 + 10 (`px-2.5`) + 10 + 1 = 22 de ancho; 18 (línea de `text-sm`) + 2×2
//     (`py-0.5`) + 2×1 (borde) = 24 de alto.
//   - con aspa: 1 + 10 + 4 (`gap-1`) + 24 (`IconButton sm`) + 4 (`pe-1`) + 1 = 44, y el
//     MISMO alto: el `-my-1` deja la diana en 24 − 8 = 16, menos que la línea.
//   - con punto: 8 (`h-2 w-2`) + 4 (`gap-1`) más.
//   - densa: línea de `text-xs` (16) y `gap-1.5` (6).
func TestEtiquetaMideLoQueTag(t *testing.T) {
	defer TactilDePrueba(false)()

	m := MedidaEtiqueta("", false, false, false, 0)
	exacto(t, "ancho sin aspa", m.Width, 22)
	exacto(t, "alto sin aspa", m.Height, 24)

	m = MedidaEtiqueta("", false, true, false, 0)
	exacto(t, "ancho con aspa", m.Width, 44)
	exacto(t, "alto con aspa (el chip no crece por el aspa)", m.Height, 24)

	m = MedidaEtiqueta("", true, true, false, 0)
	exacto(t, "ancho con punto y aspa", m.Width, 56)

	m = MedidaEtiqueta("", true, true, true, 0)
	exacto(t, "ancho densa con punto y aspa", m.Width, 1+10+8+6+6+24+4+1)
	exacto(t, "alto densa", m.Height, 16+4+2)

	// El texto se trunca en el `max-w-48` (192): un nombre larguísimo no estira el chip.
	largo := strings.Repeat("Proveedores del proyecto ", 8)
	m = MedidaEtiqueta(largo, false, false, false, 0)
	exacto(t, "ancho con el texto en su tope", m.Width, 1+10+192+10+1)
}

// TestEtiquetaElAspaTieneDianaDe24Y44: la diana del aspa es la de WCAG 2.5.8 con ratón
// y la de 44 con el dedo, y se MIDE en el widget montado, no en la constante. Con el dedo
// el chip sí crece: 44 − 8 = 36 de aspa + 4 + 2 = 42.
func TestEtiquetaElAspaTieneDianaDe24Y44(t *testing.T) {
	for _, c := range []struct {
		tactil      bool
		lado, chipH float32
	}{{false, 24, 24}, {true, 44, 42}} {
		func() {
			defer TactilDePrueba(c.tactil)()
			e := NuevaEtiqueta("Trabajo").ConAspa("Quitar la etiqueta Trabajo", func() {})
			w := test.NewWindow(e)
			defer w.Close()
			e.Resize(e.MinSize())
			exacto(t, "alto del chip", e.Size().Height, c.chipH)
			a := e.Aspa()
			exacto(t, "ancho del aspa", a.Size().Width, c.lado)
			exacto(t, "alto del aspa", a.Size().Height, c.lado)
			if a.Size().Width < 24 || a.Size().Height < 24 {
				t.Errorf("el aspa mide %v: por debajo de los 24 de WCAG 2.5.8", a.Size())
			}
			// Pegada al borde derecho con su `pe-1`, y centrada en el alto.
			exacto(t, "x del aspa", a.Position().X, e.Size().Width-tema.Borde-4-c.lado)
			exacto(t, "y del aspa", a.Position().Y, (e.Size().Height-c.lado)/2)
		}()
	}
}

// TestEtiquetaElAspaQuitaYSeNombra: el aspa es un botón con el nombre de la etiqueta que
// quita, y sin `ConAspa` no hay aspa.
func TestEtiquetaElAspaQuitaYSeNombra(t *testing.T) {
	quitadas := 0
	e := NuevaEtiqueta("Trabajo").ConAspa("Quitar la etiqueta Trabajo", func() { quitadas++ })
	w := test.NewWindow(e)
	defer w.Close()
	a := e.Aspa()
	if a.AccessibilityRole() != fyne.AccessibleRoleButton {
		t.Errorf("el aspa tiene el rol %v y es un botón", a.AccessibilityRole())
	}
	if a.AccessibilityLabel() != "Quitar la etiqueta Trabajo" {
		t.Errorf("el aspa se llama %q", a.AccessibilityLabel())
	}
	test.Tap(a)
	if quitadas != 1 {
		t.Errorf("pulsar el aspa quita %d veces", quitadas)
	}
	a.Disable()
	test.Tap(a)
	if quitadas != 1 {
		t.Error("el aspa apagada sigue quitando")
	}

	sin := NuevaEtiqueta("Trabajo")
	if sin.Aspa() != nil {
		t.Error("una etiqueta sin ConAspa tiene aspa")
	}
	for _, o := range test.WidgetRenderer(sin).Objects() {
		if _, ok := o.(*BotonIcono); ok {
			t.Error("una etiqueta sin aspa pinta un botón")
		}
	}
}

// TestEtiquetaSigueLasClasesDeTag: si Tag.tsx cambia sus clases, estas medidas se
// escribieron contra otra hoja y hay que volver a medir.
func TestEtiquetaSigueLasClasesDeTag(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Tag.tsx")
	for _, clase := range []string{
		"rounded-pill inline-flex max-w-full items-center border py-0.5",
		"'ps-2.5 pe-1' : 'px-2.5'",
		"sm: 'gap-1 text-sm'",
		"xs: 'gap-1.5 text-xs'",
		"maxWidth = 'max-w-48'",
		"h-2 w-2 shrink-0 rounded-full",
		`size="sm"`,
		`tone="subtle"`,
		`class="-my-1"`,
		`<Icon name="close" size={12} />`,
		`<span class="inline-flex shrink-0">{trailing}</span>`,
	} {
		if !strings.Contains(fuente, clase) {
			t.Errorf("Tag.tsx ya no contiene %q: o la web cambió o aquí se midió a ojo", clase)
		}
	}
	if !strings.Contains(webref.Read(t, "apps/frontend/packages/ui/src/components/IconButton.tsx"),
		"subtle: 'text-text-muted hover:bg-border hover:text-text'") {
		t.Error("el tono subtle de IconButton.tsx cambió y el aspa de la Etiqueta pinta el de antes")
	}
}

// TestEtiquetaConAnexoLoDejaEntreElTextoYElAspa es el `trailing` de Tag.tsx: la insignia
// va detrás del texto con su `gap-1` y antes del aspa, suma su ancho al chip y, cuando
// falta sitio, el que se recorta es el texto y no la insignia ni el aspa.
func TestEtiquetaConAnexoLoDejaEntreElTextoYElAspa(t *testing.T) {
	defer TactilDePrueba(false)()
	correo := "ana.de.la.fuente@ayuntamiento.example.org"
	ins := NuevaInsignia("Acepta", InsigniaExito)
	e := NuevaEtiqueta(correo).ConAspa("Quitar a "+correo, func() {}).ConAnexo(ins)
	w := test.NewWindow(e)
	defer w.Close()

	sin := MedidaEtiqueta(correo, false, true, false, 0)
	exacto(t, "ancho con anexo", e.MinSize().Width, sin.Width+ins.MinSize().Width+4)
	if e.Anexo() != ins {
		t.Error("Anexo no devuelve lo que se puso con ConAnexo")
	}

	// Estrecho: 160 de chip no dan para el correo entero.
	e.Resize(fyne.NewSize(160, e.MinSize().Height))
	texto := test.WidgetRenderer(e).(*etiquetaRenderer).texto
	if texto.Text == correo {
		t.Error("en 160 px el correo sale entero y debería recortarse")
	}
	a := e.Aspa()
	if d := ins.Position().X + ins.Size().Width; d > a.Position().X+0.01 {
		t.Errorf("la insignia acaba en %v y el aspa empieza en %v: se pisan", d, a.Position().X)
	}
	if ins.Size().Width < ins.MinSize().Width-0.01 {
		t.Errorf("la insignia mide %v y pide %v: se ha encogido en vez del texto", ins.Size().Width, ins.MinSize().Width)
	}
	if d := a.Position().X + a.Size().Width; d > 160+0.01 {
		t.Errorf("el aspa acaba en %v, fuera del chip de 160", d)
	}
	if ins.Position().X < Medir(texto.Text, tema.TextoSM, Normal).Width+tema.Borde+10 {
		t.Errorf("la insignia empieza en %v, encima del texto", ins.Position().X)
	}
}
