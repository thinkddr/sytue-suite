// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// accionDe es una acción de la cabecera de ese ancho, con el alto táctil.
func accionDe(ancho float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.NRGBA{A: 255})
	r.SetMinSize(fyne.NewSize(ancho, 44))
	return r
}

// cabeceraEnTelefono monta la cabecera bajo la barra de la suite en un teléfono de 390,
// que es donde `RepiteLaBarra` esconde el título.
func cabeceraEnTelefono(t *testing.T, c *CabeceraApp) {
	t.Helper()
	restaurar := TactilDePrueba(true)
	t.Cleanup(func() { restaurar() })
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Contacts)
	t.Cleanup(b.Parar)
	w := test.NewTempWindow(t, b.Montar(container.NewVBox(c)))
	w.Resize(fyne.NewSize(390, 700))
	c.Ancho = 390
	if c.tituloVisible() != "" {
		t.Fatal("en el teléfono, bajo la barra única, el título sigue a la vista")
	}
}

// TestSinTituloLaCabeceraNoReservaSuLinea: con el título escondido por la barra única
// (`RepiteLaBarra`), SectionHeader se queda en `max-md:min-h-0 max-md:justify-end`: solo
// las acciones, en una línea y a la derecha. Aquí reservaba la línea del título (~40 px
// vacíos sobre las acciones en el teléfono) y, si no cabían, las bajaba a una tira debajo
// (auditoría del 2026-11-24, Contacts). Sin acciones, no ocupa nada.
// VISTO EN ROJO con el reparto de siempre: 99 de alto donde cabe en 65, y 48 sin nada.
func TestSinTituloLaCabeceraNoReservaSuLinea(t *testing.T) {
	pocas := NuevaCabeceraApp("Contactos", accionDe(44), accionDe(160))
	pocas.RepiteLaBarra = true
	cabeceraEnTelefono(t, pocas)
	una := 44 + 2*cabeceraPYv + tema.Borde
	if got := MedidaCabeceraApp(pocas).Height; got != una {
		t.Errorf("sin título y con las acciones cabiendo mide %v y es una línea de acciones, %v", got, una)
	}

	muchas := NuevaCabeceraApp("Contactos", accionDe(140), accionDe(120), accionDe(170))
	muchas.RepiteLaBarra = true
	cabeceraEnTelefono(t, muchas)
	if got := MedidaCabeceraApp(muchas).Height; got != una {
		t.Errorf("sin título y con la tira mide %v y es una sola línea, %v", got, una)
	}
	tam := fyne.NewSize(390, una)
	muchas.Resize(tam)
	r := test.WidgetRenderer(muchas).(*cabeceraRenderer)
	r.Layout(tam)
	if r.tira == nil || !r.desliza {
		t.Fatal("las acciones que no caben no van en la tira")
	}
	if y := r.tira.Position().Y; y != cabeceraPYv {
		t.Errorf("la tira está en y=%v y, sin título, va en la primera línea (%v)", y, cabeceraPYv)
	}

	sola := NuevaCabeceraApp("Contactos")
	sola.RepiteLaBarra = true
	cabeceraEnTelefono(t, sola)
	if got := MedidaCabeceraApp(sola).Height; got != 0 {
		t.Errorf("sin título ni acciones mide %v y es `max-md:hidden`", got)
	}
}

// TestLaCabeceraMedidaAntesDeMontarSabeQueVaSinTitulo: Contacts mide la cabecera para su
// `franjas` ANTES de colgarla, sin lienzo. Con el título contado, la franja guardaba el
// alto de dos líneas y la cabecera, ya sin título, se pintaba centrada en ese hueco: la
// línea vacía seguía ahí. Se mide con la ventana de la app que lleva la barra de la suite.
// VISTO EN ROJO sin `lienzoConBarraSuite`: 99 en vez de 65.
func TestLaCabeceraMedidaAntesDeMontarSabeQueVaSinTitulo(t *testing.T) {
	montada := NuevaCabeceraApp("Contactos")
	montada.RepiteLaBarra = true
	cabeceraEnTelefono(t, montada) // la ventana de la app, a 390 y con la barra
	// El driver de pruebas da a todo objeto el lienzo de la ÚLTIMA ventana, colgado o no:
	// una ventana más, sin barra, hace de «todavía sin montar».
	otra := test.NewTempWindow(t, canvas.NewRectangle(color.NRGBA{}))
	otra.Resize(fyne.NewSize(390, 100))

	suelta := NuevaCabeceraApp("Contactos", accionDe(140), accionDe(120), accionDe(170))
	suelta.RepiteLaBarra = true
	suelta.Ancho = 390
	if got, una := MedidaCabeceraApp(suelta).Height, 44+2*cabeceraPYv+tema.Borde; got != una {
		t.Errorf("medida sin montar da %v y, sin título, es una línea de acciones: %v", got, una)
	}
}

// TestLasAccionesQueNoCabenNoSeSalenDeLaColumna: con puntero, si una acción sola —un
// grupo de la app, como el de Contacts— es más ancha que la columna de 320, la envoltura
// no puede repartirla y se salía por encima del lector (auditoría del 2026-11-24). Va en
// la tira, que la recorta a la columna y se desliza; y lo escondido se difumina.
// VISTO EN ROJO sin `algunaNoCabe`: la acción colgaba suelta, más ancha que la columna.
func TestLasAccionesQueNoCabenNoSeSalenDeLaColumna(t *testing.T) {
	restaurar := TactilDePrueba(false)
	t.Cleanup(func() { restaurar() })
	grupo := accionDe(380)
	c := NuevaCabeceraApp("Contactos", grupo)
	c.Ancho = 320
	tam := fyne.NewSize(320, MedidaCabeceraApp(c).Height)
	c.Resize(tam)
	r := test.WidgetRenderer(c).(*cabeceraRenderer)
	r.Layout(tam)
	if r.tira == nil || !r.desliza {
		t.Fatal("la acción que no cabe cuelga suelta de la cabecera, sin tira que la recorte")
	}
	if p, s := r.tira.Position(), r.tira.Size(); p.X+s.Width > tam.Width-cabeceraPX+0.5 {
		t.Errorf("la tira (%v, %v) se sale de la columna de %v", p, s, tam.Width)
	}
	tr := test.WidgetRenderer(r.tira).(*tiraDeslizableRenderer)
	if f := tr.fundidos[1]; f == nil || f.Hidden {
		t.Error("lo que queda a la derecha no se difumina")
	}
	if f := tr.fundidos[0]; f == nil || !f.Hidden {
		t.Error("a la izquierda no hay nada escondido y se difumina igual")
	}
	r.tira.mover(1000)
	if tr.fundidos[1].Hidden != true || tr.fundidos[0].Hidden {
		t.Error("desplazada hasta el final, el difuminado no cambia de lado")
	}
}
