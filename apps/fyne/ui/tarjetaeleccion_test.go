// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaTarjetaEleccionMideLoQueMideLaWeb: la opción «CSV» de «Tus datos» mide 59,64 a
// 588 de ancho en producción, y el texto empieza en x = 40 (1 + 10,5 + 18 + 10,5). Una
// descripción que no cabe baja a otra línea y la tarjeta crece lo que mide esa línea.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): sin el `mt-0.5` de la
// descripción la tarjeta mide 57,89 y el test lo dice.
func TestLaTarjetaEleccionMideLoQueMideLaWeb(t *testing.T) {
	c := NuevaTarjetaEleccion("CSV", "Para abrirlo en Excel, Numbers o Google Sheets.", nil).A(588)
	c.Densa = true
	if got := c.MinSize(); got.Width != 588 || got.Height-67.875 > 0.01 || 67.875-got.Height > 0.01 {
		t.Errorf("mide %v y en producción 588 × 59,64", got)
	}
	if x := tema.Borde + 3*0.25*tema.Rem + LadoIconoEleccion + HuecoTarjetaEleccion; x != 43 {
		t.Errorf("el texto empieza en x = %v y en producción en 40", x)
	}
	una := AltoTarjetaEleccion(c, 588)
	if dos := AltoTarjetaEleccion(c, 240); dos-una-descEleccion.Linea > 0.01 || una+descEleccion.Linea-dos > 0.01 {
		t.Errorf("a 200 la descripción va en dos líneas y la tarjeta mide %v; tenía que crecer %v desde %v",
			dos, descEleccion.Linea, una)
	}
}

// TestLaTarjetaEleccionEligeComoUnRadio: pulsar una sin elegir avisa, pulsar la ya
// elegida no, y lo mismo con Espacio e Intro. Al enfocarla sale el anillo.
func TestLaTarjetaEleccionEligeComoUnRadio(t *testing.T) {
	veces := 0
	c := NuevaTarjetaEleccion("JSON", "Para otras aplicaciones y programas.", func() { veces++ }).A(588)
	w := test.NewWindow(c)
	defer w.Close()
	w.Resize(fyne.NewSize(600, 80))

	c.Tapped(nil)
	c.TypedRune(' ')
	c.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if veces != 3 {
		t.Errorf("sin elegir avisa %d veces de 3", veces)
	}
	c.Elegida = true
	c.Tapped(nil)
	if veces != 3 {
		t.Error("pulsar la opción ya elegida vuelve a avisar: un radio no hace eso")
	}
	r := test.WidgetRenderer(c).(*tarjetaEleccionRenderer)
	c.FocusGained()
	if !r.aro.anillo.Visible() {
		t.Error("enfocada no enseña el anillo")
	}
	if !r.tic.Objeto().Visible() || r.chevron.Objeto().Visible() {
		t.Error("elegida tiene que enseñar el tic y no el chevron")
	}
}

// TestLaTarjetaDeEleccionLlevaSuIcono: sin elegir se pinta el `icon` que se le da, y sin
// él el `chevronRight` de la web.
func TestLaTarjetaDeEleccionLlevaSuIcono(t *testing.T) {
	c := NuevaTarjetaEleccion("Solo ver", "", nil)
	if c.icono() != iconos.ChevronDerecha {
		t.Error("sin icono no sale el chevron por defecto")
	}
	c.Icono = iconos.Globo
	if c.icono() != iconos.Globo {
		t.Error("el icono pedido no se pinta")
	}
}

// TestLaInsigniaVaDetrasDelTitulo es el `badge` de ChoiceCard: en la fila del título
// (`flex flex-wrap items-center gap-2`) si cabe detrás, y en la siguiente, con el mismo
// `gap-2`, si no. La fila mide lo que la más alta de las dos, y la línea de `text-base`
// (19,25) le saca 1,75 a la insignia (17,5), que va centrada: medido con Chromium en el
// alta del portal, la insignia de «Para mi organización» cae a 0,875 del borde de su
// fila.
//
// Visto en ROJO a mano (en el frontal): con `filaTitulo` ignorando la insignia, la
// tarjeta mide lo mismo con ella que sin ella y el test lo dice.
func TestLaInsigniaVaDetrasDelTitulo(t *testing.T) {
	const titulo, insignia = "Para mi organización", "Lo más habitual"
	sin := NuevaTarjetaEleccion(titulo, "Invitas a tu equipo.", nil)
	con := NuevaTarjetaEleccion(titulo, "Invitas a tu equipo.", nil)
	con.Insignia = insignia
	ins := MedidaInsignia(insignia)
	cerca := func(a, b float32) bool { return a-b < 0.01 && b-a < 0.01 }
	if got, quiero := AltoTarjetaEleccion(con, 600), AltoTarjetaEleccion(sin, 600)+max(ins.Height-tema.TextoBase.Linea, 0); !cerca(got, quiero) {
		t.Errorf("con la insignia al lado mide %v; quiero %v", got, quiero)
	}
	// Un ancho en el que cabe el título pero no el título con la insignia detrás.
	w := Medir(titulo, tema.TextoBase, SemiNegrita).Width
	ancho := w + 1 + 2*(tema.Borde+4*0.25*tema.Rem) + LadoIconoEleccion + HuecoTarjetaEleccion
	if got, quiero := AltoTarjetaEleccion(con, ancho), AltoTarjetaEleccion(sin, ancho)+HuecoInsignia+ins.Height; !cerca(got, quiero) {
		t.Errorf("sin sitio al lado mide %v; quiero %v (la insignia en su propia fila)", got, quiero)
	}

	con.Ancho = 600
	r := test.WidgetRenderer(con).(*tarjetaEleccionRenderer)
	con.Resize(fyne.NewSize(600, AltoTarjetaEleccion(con, 600)))
	x0 := tema.Borde + 4*0.25*tema.Rem + LadoIconoEleccion + HuecoTarjetaEleccion
	if p := r.ins.Position(); !r.ins.Visible() || !cerca(p.X, x0+w+HuecoInsignia) {
		t.Errorf("la insignia está en x = %v (visible %v); quiero %v", p.X, r.ins.Visible(), x0+w+HuecoInsignia)
	}
	if p := r.ins.Position(); !cerca(p.Y, tema.Borde+4*0.25*tema.Rem+(tema.TextoBase.Linea-ins.Height)/2) {
		t.Errorf("la insignia está en y = %v; tiene que ir centrada en la fila del título", p.Y)
	}
}
