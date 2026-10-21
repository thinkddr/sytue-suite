// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaCabeceraDeVistaMide48 es el `min-h-12` de SectionHeader con la raíz de 16.
// El relleno `py-2.5 px-4` son 10px 16px.
//
// Los 48 no salen del contenido: el título en `sytue-h4` (26) con `py-2.5` y el
// filete suman 47 y no llegan. Es el mínimo el que manda, y por eso una cabecera
// con solo un título y otra con un título y un enlace miden lo mismo.
func TestLaCabeceraDeVistaMide48(t *testing.T) {
	c := NuevaCabeceraApp("Recibidos")
	if got := MedidaCabeceraApp(c).Height; got != 48 {
		t.Errorf("la cabecera mide %v y `min-h-12` son 48", got)
	}
	if got := c.MinSize().Height; got != 48 {
		t.Errorf("MinSize() da %v y `min-h-12` son 48", got)
	}
	// Sin el `min-h-12`, el contenido daría 47: un píxel menos.
	if quiero := tema.TextoLG.Linea + 2*tema.Espacio(2.5) + tema.Borde; quiero != 47 {
		t.Errorf("el contenido de la cabecera suma %v y son 47", quiero)
	}
}

// TestLaCabeceraCreceConUnBotonDe32 es la cabecera de Calendar con `h-8`: mete un
// botón de 32 y sube a 53 (32 + 2×10 + 1). Es la prueba de que el `min-h-12` es un
// MÍNIMO y no un alto fijo.
func TestLaCabeceraCreceConUnBotonDe32(t *testing.T) {
	boton := NuevoBoton("Hoy", nil).Con(Secundario, TallaSM) // `h-8` = 32
	c := NuevaCabeceraApp("Septiembre de 2026", boton)
	if got := MedidaCabeceraApp(c).Height; got != 53 {
		t.Errorf("la cabecera con un botón de 32 mide %v y las clases de la web dan 53", got)
	}
}

// TestLasDosCabecerasNoMidenIgual es el detalle que se pierde al portar y que deja las
// vistas desalineadas entre sí:
//
//	cabecera de VISTA     48  — `min-h-12` con `py-2.5`, sobre fondo `surface`
//	cabecera de TARJETA   51  — `py-3` sobre `sytue-h4`, sin mínimo, con su filete
//
// Son dos componentes distintos con dos rellenos distintos, y quien los iguale
// «porque son la misma cosa» mueve 3 px todas las tarjetas de la suite.
func TestLasDosCabecerasNoMidenIgual(t *testing.T) {
	vista := MedidaCabeceraApp(NuevaCabeceraApp("Recibidos")).Height
	tarjeta := AltoCabecera("Cambios sin guardar")
	if vista != 48 {
		t.Errorf("la cabecera de vista mide %v y son 48", vista)
	}
	if tarjeta != 51 {
		t.Errorf("la cabecera de tarjeta mide %v y `py-3` sobre `sytue-h4` con su filete son 51", tarjeta)
	}
	if vista == tarjeta {
		t.Error("las dos cabeceras miden lo mismo, y en la web no lo miden")
	}
}

// TestLaCabeceraDeVistaLlevaSuFileteAbajo: `border-b` de un píxel en `--color-border`,
// y DENTRO del alto, que es lo que hace `box-sizing: border-box`.
func TestLaCabeceraDeVistaLlevaSuFileteAbajo(t *testing.T) {
	c := NuevaCabeceraApp("Recibidos")
	m := fyne.NewSize(400, MedidaCabeceraApp(c).Height)
	c.Resize(m)
	c.Refresh()
	img := pinta(t, m.Width+10, m.Height+10, c)

	// La última fila de la cabecera es el filete; la anterior, la superficie.
	//
	// El vector se deriva del CSS actual, de forma independiente del lector de
	// tokens: `rgba(16, 20, 26, .12)` sobre blanco da (226, 228, 228).
	// cerca conserva el margen de un canal por la cuantización del alfa.
	borde := color.NRGBA{R: 226, G: 228, B: 228, A: 255}
	y := int(m.Height*escala) - 1
	if got := px(img, 20, y); !cerca(got, borde) {
		t.Errorf("la última fila pinta %v y el filete sobre la superficie se mide %v", got, borde)
	}
	if got, quiero := px(img, 20, y-4), tema.Color("surface", tema.Claro); !cerca(got, quiero) {
		t.Errorf("dentro de la cabecera se pinta %v y su fondo es %v", got, quiero)
	}
}

// TestLaCabeceraSeRecolocaAlRefrescar: cambiar el título o el texto de una acción
// después del primer pintado tiene que verse. El título se escribe en Layout, y Fyne
// no llama a Layout si el tamaño no cambia.
func TestLaCabeceraSeRecolocaAlRefrescar(t *testing.T) {
	conteo := NuevoRotulo("1 sin leer", tema.TextoXS, Normal, "text-subtle")
	c := NuevaCabeceraApp("Recibidos", conteo)
	w := test.NewWindow(c)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(279, 48))
	w.Canvas().Capture()

	c.Titulo = "Archivo"
	conteo.Texto = "12 conversaciones"
	c.Refresh()
	w.Canvas().Capture()

	var vistos []string
	for _, o := range test.WidgetRenderer(c).Objects() {
		if tx, ok := o.(*canvas.Text); ok {
			vistos = append(vistos, tx.Text)
		}
	}
	for _, o := range test.WidgetRenderer(conteo).Objects() {
		if tx, ok := o.(*canvas.Text); ok {
			vistos = append(vistos, tx.Text)
		}
	}
	if !slices.Contains(vistos, "Archivo") || !slices.Contains(vistos, "12 conversaciones") {
		t.Errorf("tras refrescar la cabecera pinta %q", vistos)
	}
	if got, quiero := conteo.Size().Width, MedidaRotulo(conteo).Width; got != quiero {
		t.Errorf("el conteo nuevo mide %v y se ha quedado con el ancho del viejo, %v", quiero, got)
	}
}

// TestLaCabeceraEnvuelveComoLaWeb es el `flex-wrap` de SectionHeader: en el panel de
// 279 del fixture, el título y las dos acciones de 110×28 no caben en una línea:
// bajan a la segunda y la cabecera mide 83 —1 + 2×10 + 26 + 8 + 28—.
// El alto de 28 es el de las acciones artificiales de este test, no una talla h-8.
func TestLaCabeceraEnvuelveComoLaWeb(t *testing.T) {
	accion := func() fyne.CanvasObject {
		r := canvas.NewRectangle(color.NRGBA{})
		r.SetMinSize(fyne.NewSize(110, 28))
		return r
	}
	c := NuevaCabeceraApp("Contactos", accion(), accion())
	c.Ancho = 279
	if got := MedidaCabeceraApp(c).Height; got != 83 {
		t.Errorf("a 279 la cabecera mide %v y con las acciones en segunda línea son 83", got)
	}
	tam := fyne.NewSize(279, 83)
	c.Resize(tam)
	r := test.WidgetRenderer(c).(*cabeceraRenderer)
	r.Layout(tam)
	if r.titulo.Text != "Contactos" {
		t.Errorf("el título sale %q: con las acciones abajo cabe entero", r.titulo.Text)
	}
	if p := c.Acciones[0].Position(); p != fyne.NewPos(16, 44) {
		t.Errorf("la primera acción está en %v y va a `px-4` en la segunda línea (16, 44)", p)
	}
	c.Ancho = 800
	if got := MedidaCabeceraApp(c).Height; got != 49 {
		t.Errorf("a 800 cabe en una línea y mide %v, no 49", got)
	}
}

// TestElSubtituloQueLlegaDespuesSePinta: la cabecera se crea sin subtítulo y el
// recuento llega al cargar. VISTO EN ROJO con el subtítulo creado solo si ya venía: no
// se pintaba nunca.
func TestElSubtituloQueLlegaDespuesSePinta(t *testing.T) {
	c := NuevaCabeceraApp("Recibidos")
	c.Resize(fyne.NewSize(400, 60))
	r := test.WidgetRenderer(c).(*cabeceraRenderer)
	c.Subtitulo = "17 sin leer"
	c.Refresh()
	if r.subtitulo.Hidden || r.subtitulo.Text != "17 sin leer" {
		t.Errorf("el subtítulo tardío sale oculto=%v con %q", r.subtitulo.Hidden, r.subtitulo.Text)
	}
}

// TestLasAccionesDeLaSegundaFilaEnvuelvenEntreSi es el `flex flex-wrap gap-2` del `<div>`
// de las acciones: en un teléfono de 360, tres acciones de 110 no caben en los 328 de la
// fila y la tercera baja a otra línea, a `gap-2`. La cabecera mide entonces 1 + 2×10
// + 26 + 8 + 28 + 8 + 28 = 119. VISTO EN ROJO con una sola línea de acciones, que se
// salía por la derecha.
func TestLasAccionesDeLaSegundaFilaEnvuelvenEntreSi(t *testing.T) {
	accion := func() fyne.CanvasObject {
		r := canvas.NewRectangle(color.NRGBA{})
		r.SetMinSize(fyne.NewSize(110, 28))
		return r
	}
	c := NuevaCabeceraApp("Plantillas del proyecto", accion(), accion(), accion())
	c.Ancho = 360
	if got := MedidaCabeceraApp(c).Height; got != 119 {
		t.Errorf("a 360 la cabecera mide %v y con las acciones en dos líneas son 119", got)
	}
	tam := fyne.NewSize(360, 119)
	c.Resize(tam)
	r := test.WidgetRenderer(c).(*cabeceraRenderer)
	r.Layout(tam)
	if p := c.Acciones[2].Position(); p != fyne.NewPos(16, 44+28+8) {
		t.Errorf("la tercera acción está en %v y va al principio de la línea siguiente (16, 80)", p)
	}
	if p := c.Acciones[1].Position(); p != fyne.NewPos(16+110+8, 44) {
		t.Errorf("la segunda acción está en %v y va junto a la primera a `gap-2`", p)
	}
}

// TestElGlifoDeLaCabeceraEsTextMuted: SectionHeader envuelve el icono en `<span
// class="text-text-muted">` y el glifo hereda `currentColor`. VISTO EN ROJO con el color
// que traía el glifo.
func TestElGlifoDeLaCabeceraEsTextMuted(t *testing.T) {
	c := NuevaCabeceraApp("Contactos")
	c.Glifo = iconos.TrazoConTinta(iconos.Usuarios, 16, color.NRGBA{R: 255, A: 255})
	c.Resize(fyne.NewSize(400, MinCabeceraApp))
	img := pinta(t, 400, MinCabeceraApp, c)
	quiero := tema.Color("text-muted", tema.Claro)
	hay := false
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < int(2*(cabeceraPX+CajaIcono)); x++ {
			if p := px(img, x, y); p.R > 200 && p.G < 80 {
				t.Fatalf("en (%d,%d) queda el rojo del glifo: la cabecera no lo tiñe", x, y)
			}
			if cerca(px(img, x, y), quiero) {
				hay = true
			}
		}
	}
	if !hay {
		t.Error("el glifo de la cabecera no pinta ningún píxel en `text-muted`")
	}
}

// TestLaCabeceraNoApilaAccionesEnTactil: en un teléfono las acciones que no caben
// van en UNA tira deslizable bajo el título, no repartidas en líneas —la misma
// respuesta que la web en móvil—. Con las tres acciones de 110 del test de
// envoltura, la cabecera apilada llega a 119: dos filas de botones sobre la
// lista. VISTO EN ROJO quitando la rama de `esTactil` de `repartir`: volvían las
// líneas apiladas y la tira no aparecía.
func TestLaCabeceraNoApilaAccionesEnTactil(t *testing.T) {
	si := true
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = nil })

	accion := func() fyne.CanvasObject {
		r := canvas.NewRectangle(color.NRGBA{})
		r.SetMinSize(fyne.NewSize(110, 28))
		return r
	}
	c := NuevaCabeceraApp("Plantillas del proyecto", accion(), accion(), accion())
	c.Ancho = 360
	// Dos líneas como mucho: el título y la tira, se vean las acciones o se
	// deslicen. El alto exacto es el de la envoltura de UNA fila de acciones.
	quiero := float32(26 + 8 + 28 + 2*10 + 1)
	if got := MedidaCabeceraApp(c).Height; got != quiero {
		t.Errorf("en táctil la cabecera mide %v y con el título más la tira son %v",
			got, quiero)
	}
	tam := fyne.NewSize(360, quiero)
	c.Resize(tam)
	r := test.WidgetRenderer(c).(*cabeceraRenderer)
	r.Layout(tam)
	if r.tira == nil {
		t.Fatal("en táctil la cabecera no monta la tira deslizable")
	}
	if p := r.tira.Position(); p != fyne.NewPos(16, 26+10+8) {
		t.Errorf("la tira está en %v y va bajo el título a `px-4`", p)
	}
	// Las posiciones de las acciones son RELATIVAS a la tira que las contiene:
	// lo que se comprueba es que su padre es la tira y no la cabecera.
	for _, a := range c.Acciones {
		if !slices.Contains(r.tira.Objetos, a) {
			t.Error("una acción no va dentro de la tira deslizable")
		}
	}
	for _, o := range r.Objects() {
		if slices.Contains(c.Acciones, o) {
			t.Error("una acción cuelga directo de la cabecera y TAMBIÉN de la tira no puede estar")
		}
	}
}

func TestLaCabeceraTactilCambiaDeAnchoSinRecortarAcciones(t *testing.T) {
	antes := tactilDePrueba
	si := true
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = antes })
	acciones := make([]fyne.CanvasObject, 0, 5)
	for range 5 {
		acciones = append(acciones, NuevoBoton("Acción", nil).Con(Secundario, TallaSM))
	}
	c := NuevaCabeceraApp("Contactos", acciones...)
	c.Ancho = 800
	c.Resize(fyne.NewSize(c.Ancho, c.MinSize().Height))
	r := test.WidgetRenderer(c).(*cabeceraRenderer)
	for _, ancho := range []float32{800, 320, 375, 390, 800} {
		c.Ancho = ancho
		c.Resize(fyne.NewSize(ancho, c.MinSize().Height))
		r.Layout(c.Size())
		for _, a := range acciones {
			padre := c.Size()
			if !slices.Contains(r.Objects(), a) {
				if r.tira == nil || !slices.Contains(r.tira.Objetos, a) || !slices.Contains(r.Objects(), fyne.CanvasObject(r.tira)) {
					t.Fatalf("a %g una acción no está en la cabecera ni en su tira", ancho)
				}
				padre = r.tira.Size()
			}
			if a.Size().Height < 44 || a.Position().Y < 0 || a.Position().Y+a.Size().Height > padre.Height {
				t.Errorf("a %g una acción de 44 queda recortada: %v/%v en %v", ancho, a.Position(), a.Size(), padre)
			}
		}
		if ancho < 800 {
			r.tira.Mostrar(acciones[len(acciones)-1])
		} else {
			posiciones := make([]fyne.Position, len(acciones))
			for i, a := range acciones {
				posiciones[i] = a.Position()
			}
			acciones[0].(*Boton).FocusGained()
			acciones[len(acciones)-1].(*Boton).FocusGained()
			for i, a := range acciones {
				if a.Position() != posiciones[i] {
					t.Errorf("a %g el foco en la tira oculta ha movido una acción de %v a %v", ancho, posiciones[i], a.Position())
				}
			}
		}
	}
}
