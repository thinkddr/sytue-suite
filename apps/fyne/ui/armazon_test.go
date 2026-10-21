// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElArmazonRepiteElEsqueletoDeMail son las tres columnas medidas en
// AppShell con la raíz de 16, en la misma ventana de 1280×800 del fixture histórico
// `mail/bandeja.ancho.json`, sin la topbar —que en nativo no existe—:
//
//	menú       0 → 260
//	panel    260 → 580   (`md:w-80`: 320, con 319 dentro por su filete)
//	detalle  580 → 1280  (700)
func TestElArmazonRepiteElEsqueletoDeMail(t *testing.T) {
	menu := NuevoLateral("Navegación principal", nil)
	panel, detalle := iconoFalso(), iconoFalso()
	a := NuevoArmazon(detalle)
	a.Menu, a.Panel = menu, panel
	a.Resize(fyne.NewSize(1280, 744))
	_ = test.WidgetRenderer(a)
	a.Refresh()
	pinta(t, 1290, 754, a)

	for _, c := range []struct {
		que  string
		o    fyne.CanvasObject
		x, w float32
	}{
		{"el menú", menu, 0, 260},
		{"el panel", panel, 260, 320},
		{"el detalle", detalle, 580, 700},
	} {
		if got := c.o.Position().X; got != c.x {
			t.Errorf("%s empieza en x=%v y las clases de AppShell lo ponen en %v", c.que, got, c.x)
		}
		if got := c.o.Size().Width; got != c.w {
			t.Errorf("%s mide %v de ancho y las clases de AppShell dan %v", c.que, got, c.w)
		}
		if got := c.o.Size().Height; got != 744 {
			t.Errorf("%s mide %v de alto y la ventana deja 744", c.que, got)
		}
	}
}

// TestElPanelCrecePorEscalones son los tres anchos de la hoja: `md:w-80` hasta 1535,
// `2xl:w-96` desde 1536 y `w-[26rem]` desde 1920. Con el rem de 16 son 320, 384 y 416.
//
// Los escalones existen para que el detalle no se quede en 640 px en un portátil de
// 1280; darle el ancho grande antes de tiempo es justo lo que se quería evitar.
func TestElPanelCrecePorEscalones(t *testing.T) {
	for _, c := range []struct{ ventana, ancho float32 }{
		{1280, 320}, {1535, 320}, {1536, 384}, {1919, 384}, {1920, 416}, {2560, 416},
	} {
		if got := AnchoPanel(c.ventana); got != c.ancho {
			t.Errorf("a %v de ventana el panel mide %v y su escalón son %v", c.ventana, got, c.ancho)
		}
	}
}

// TestEnEstrechoLaListaYElDetalleSeTurnan: por debajo de 768 no caben las tres
// columnas —el menú solo se come 260 de los 320 de un iPhone SE—, así que la lista y
// el detalle se turnan la pantalla entera y el menú pasa a cajón.
//
// Sin el turno, el detalle sería inalcanzable en un teléfono.
func TestEnEstrechoLaListaYElDetalleSeTurnan(t *testing.T) {
	menu := NuevoLateral("Navegación principal", nil)
	panel, detalle := iconoFalso(), iconoFalso()
	a := NuevoArmazon(detalle)
	a.Menu, a.Panel = menu, panel
	a.Resize(fyne.NewSize(390, 844))
	_ = test.WidgetRenderer(a)
	a.Refresh()
	pinta(t, 400, 854, a)

	if !panel.Visible() || detalle.Visible() {
		t.Error("en estrecho y sin detalle abierto tiene que verse la lista y no el detalle")
	}
	if got := panel.Size().Width; got != 390 {
		t.Errorf("la lista mide %v y en el teléfono ocupa la pantalla (390)", got)
	}
	if menu.Visible() {
		t.Error("en estrecho el menú es un cajón y arranca cerrado")
	}

	a.DetalleAbierto = true
	a.Resize(fyne.NewSize(390, 844))
	a.Refresh()
	pinta(t, 400, 854, a)
	if panel.Visible() || !detalle.Visible() {
		t.Error("con el detalle abierto tiene que verse el detalle y no la lista")
	}
	if got := detalle.Position().X; got != 0 {
		t.Errorf("el detalle empieza en x=%v y en el teléfono ocupa la pantalla desde 0", got)
	}

	a.CajonAbierto = true
	a.Resize(fyne.NewSize(390, 844))
	a.Refresh()
	pinta(t, 400, 854, a)
	if !menu.Visible() {
		t.Error("el cajón abierto tiene que enseñar el menú")
	}
	if got := menu.Size().Width; got != 260 {
		t.Errorf("el cajón mide %v y el carril son 260 también en el teléfono", got)
	}
}

// TestElContenidoRestringidoSeCentraEn1280 es el `mx-auto w-full p-6` con
// `max-width: var(--container-content)`: 24 de margen y nunca más ancho de 1280.
func TestElContenidoRestringidoSeCentraEn1280(t *testing.T) {
	contenido := iconoFalso()
	a := NuevoArmazon(contenido)
	a.Restringido = true

	a.Resize(fyne.NewSize(1000, 600))
	_ = test.WidgetRenderer(a)
	a.Refresh()
	pinta(t, 1010, 610, a)
	if got, quiero := contenido.Size().Width, float32(1000-2*24); got != quiero {
		t.Errorf("en 1000 el contenido mide %v y con el `p-6` son %v", got, quiero)
	}
	if got := contenido.Position().Y; got != 24 {
		t.Errorf("el contenido empieza en y=%v y el `p-6` son 24", got)
	}

	// Muy ancho, se acota y se centra.
	a.Resize(fyne.NewSize(1800, 600))
	a.Refresh()
	pinta(t, 1810, 610, a)
	if got := contenido.Size().Width; got != tema.AnchoContenido {
		t.Errorf("en 1800 el contenido mide %v y el ancho canónico son %v", got, tema.AnchoContenido)
	}
	if got, quiero := contenido.Position().X, (1800-tema.AnchoContenido)/2; got != quiero {
		t.Errorf("el contenido se centra en x=%v y le tocan %v", got, quiero)
	}
}

// TestElArmazonPintaElMenuQueLeCambian: un menú nuevo después del primer pintado tiene
// que estar entre los objetos que se pintan. Mail reconstruye su lateral con cada dato
// que llega, y el carril se quedaba en blanco.
func TestElArmazonPintaElMenuQueLeCambian(t *testing.T) {
	a := NuevoArmazon(canvas.NewRectangle(color.NRGBA{A: 255}))
	a.Menu = NuevoLateral("menú", nil)
	w := test.NewWindow(a)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 744))
	w.Canvas().Capture()

	nuevo := NuevoLateral("otro", nil)
	a.Menu = nuevo
	a.Refresh()
	if !slices.Contains(test.WidgetRenderer(a).Objects(), fyne.CanvasObject(nuevo)) {
		t.Error("el menú nuevo no está entre los objetos del armazón: no se pinta")
	}
}

// TestElCajonDelTelefonoFuncionaSinQueLaAppHagaNada es el cromo móvil de AppShell.tsx:
// en estrecho, con menú y sin barra propia, el armazón pone la suya; la hamburguesa
// abre el cajón, que arranca ARRIBA y tapa la barra; el velo se pulsa y lo cierra; y al
// ensanchar la ventana se cierra solo. VISTO EN ROJO antes: sin barra no había forma de
// abrir el menú (Mail, Notepads, Drive, Calendar, ERP), el velo no se podía pulsar y el
// cajón empezaba una barra móvil más abajo (44 con la raíz de 16).
func TestElCajonDelTelefonoFuncionaSinQueLaAppHagaNada(t *testing.T) {
	menu := NuevoLateral("Navegación principal", nil)
	a := NuevoArmazon(iconoFalso())
	a.Menu = menu
	var avisos []bool
	a.AlCajon = func(abierto bool) { avisos = append(avisos, abierto) }
	a.Resize(fyne.NewSize(360, 740))
	r := test.WidgetRenderer(a).(*armazonRenderer)
	a.Refresh()

	b := a.barra
	if b == nil || !b.Visible() || b.AlMenu == nil {
		t.Fatal("en estrecho y sin barra de la app, el armazón no pone la suya con su hamburguesa")
	}
	b.AlMenu()
	if !a.CajonAbierto || !menu.Visible() {
		t.Fatal("la hamburguesa no abre el cajón")
	}
	if y := menu.Position().Y; y != 0 {
		t.Errorf("el cajón arranca en y=%v y en la web tapa la barra móvil desde arriba (0)", y)
	}
	r.velo.Tapped(nil)
	if a.CajonAbierto || menu.Visible() {
		t.Error("pulsar el velo no cierra el cajón")
	}
	b.AlMenu()
	a.Resize(fyne.NewSize(1280, 800))
	if a.CajonAbierto {
		t.Error("al ensanchar la ventana el cajón sigue abierto")
	}
	if want := []bool{true, false, true, false}; !slices.Equal(avisos, want) {
		t.Errorf("AlCajon ha avisado %v y tenía que ser %v", avisos, want)
	}
}

// TestLaBarraMovilTitulaYPoneElBotonComoAppShell: el título sale sin lista y, con
// lista, solo en el detalle; sin menú ni detalle que cerrar, no hay botón.
func TestLaBarraMovilTitulaYPoneElBotonComoAppShell(t *testing.T) {
	b := NuevaBarraMovil()
	b.Titulo, b.AlMenu = "Recibidos", func() {}
	r := test.WidgetRenderer(b).(*barraMovilRenderer)
	b.ConLista = true
	b.Refresh()
	if got := r.tituloVisible(); got != "" {
		t.Errorf("con la lista a la vista el título sobra y sale %q", got)
	}
	b.ConLista = false
	b.Refresh()
	if got := r.tituloVisible(); got != "Recibidos" {
		t.Errorf("sin lista el título es lo único que dice dónde estás y sale %q", got)
	}
	b.AlMenu = nil
	b.Refresh()
	if r.boton.Visible() {
		t.Error("sin menú ni detalle que cerrar, AppShell no pinta botón")
	}
}

// TestElTecladoDelCajon es `useNavDrawer`: al abrir, el foco entra en el menú; el Tab da
// vueltas DENTRO; Escape lo cierra y el foco vuelve al botón que lo abrió. VISTO EN ROJO
// antes: el foco se quedaba fuera, el Tab se escapaba al contenido de debajo del velo y
// Escape no hacía nada.
func TestElTecladoDelCajon(t *testing.T) {
	uno, dos := NuevaFilaLateral("Recibidos", nil, nil), NuevaFilaLateral("Enviados", nil, nil)
	fuera := NuevoBoton("Fuera", nil)
	a := NuevoArmazon(fuera)
	a.Menu = NuevoLateral("Navegación principal", container.New(Columna{}, uno, dos))
	w := test.NewTempWindow(t, a)
	w.Resize(fyne.NewSize(360, 740))
	a.Resize(fyne.NewSize(360, 740))
	a.Refresh()
	c := w.Canvas()

	a.barra.AlMenu()
	if c.Focused() != fyne.Focusable(a.Menu.(*Lateral)) {
		t.Fatalf("al abrir el cajón el foco está en %T y entra en el menú", c.Focused())
	}
	c.Focus(dos)
	c.FocusNext() // el Tab del driver, que con el cajón abierto solo recorre el cajón
	if c.Focused() != fyne.Focusable(uno) {
		t.Errorf("Tab desde la última fila va a %T y tiene que volver a la primera", c.Focused())
	}
	uno.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if a.CajonAbierto || c.Focused() != fyne.Focusable(a.barra.boton) {
		t.Errorf("Escape: cajón abierto=%v y foco en %T; tiene que cerrar y volver al botón",
			a.CajonAbierto, c.Focused())
	}
	c.Focus(uno)
	c.FocusNext()
	if c.Focused() == fyne.Focusable(dos) {
		t.Error("con el cajón cerrado el Tab sigue encerrado en el menú")
	}
}

// TestElTabDelCajonNoDependeDelControl: con el cajón abierto, el Tab queda dentro también
// desde un `widget.Entry` de Fyne, que no es de la suite y hasta el fork `.5` se escapaba.
func TestElTabDelCajonNoDependeDelControl(t *testing.T) {
	campo := widget.NewEntry()
	uno := NuevaFilaLateral("Recibidos", nil, nil)
	a := NuevoArmazon(NuevoBoton("Fuera", nil))
	a.Menu = NuevoLateral("Navegación principal", container.New(Columna{}, campo, uno))
	w := test.NewTempWindow(t, a)
	w.Resize(fyne.NewSize(360, 740))
	a.Resize(fyne.NewSize(360, 740))
	a.Refresh()
	a.barra.AlMenu()
	c := w.Canvas()
	c.Focus(uno)
	for range 4 {
		c.FocusNext()
		if f := c.Focused(); f != fyne.Focusable(campo) && f != fyne.Focusable(uno) {
			t.Fatalf("con el cajón abierto el Tab ha salido a %T", f)
		}
	}
}

// TestElCajonEntraDesdeLaIzquierda es `sytue-drawer-in`: de `translateX(-100%)` a su
// sitio en 180 ms. El driver de pruebas salta la animación a su final, así que se mira que
// acabe en 0 y que el reparto respete lo que le quede por entrar. VISTO EN ROJO con el
// cajón colocado siempre en 0.
func TestElCajonEntraDesdeLaIzquierda(t *testing.T) {
	menu := NuevoLateral("Navegación principal", nil)
	a := NuevoArmazon(iconoFalso())
	a.Menu = menu
	a.Resize(fyne.NewSize(360, 740))
	test.WidgetRenderer(a)
	a.cambiarCajon(true)
	if a.entrada != 0 || menu.Position().X != 0 {
		t.Fatalf("al acabar la entrada el cajón está en x=%v (le quedan %v)", menu.Position().X, a.entrada)
	}
	a.entrada = -130 // a mitad de camino
	a.Refresh()
	if x := menu.Position().X; x != -130 {
		t.Errorf("a mitad de la entrada el cajón está en x=%v y va a -130", x)
	}
	a.entrada = 0
}

// TestElCajonDelTraslucidoVaSobreUnaBaseOpaca: el carril del traslúcido deja ver un punto
// de lo que tiene debajo. En escritorio es el fondo ambiental; en el cajón del teléfono
// es el contenido, y el texto se colaba entre los rótulos del menú. La web le pone su
// mismo color sobre `neutral-50` (`nav.bg-sidebar` por debajo de 768, appearance.css).
// Con un carril opaco —claro y oscuro— la base no se pinta.
//
// VISTO EN ROJO sin `ponerBase` en el reparto: la base ni se coloca ni se esconde (mide
// 0×0 y sigue a la vista con el carril opaco).
func TestElCajonDelTraslucidoVaSobreUnaBaseOpaca(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for variante, quiero := range map[string]bool{"claro": false, "oscuro": false, "traslucido": true} {
		tema.Elegir(variante)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		menu := NuevoLateral("Navegación principal", nil)
		a := NuevoArmazon(iconoFalso())
		a.Menu = menu
		a.CajonAbierto = true
		a.Resize(fyne.NewSize(360, 740))
		r := test.WidgetRenderer(a).(*armazonRenderer)
		r.Layout(a.Size())
		if r.base.Visible() != quiero {
			t.Errorf("en %s la base del cajón visible=%v y quiero %v", variante, r.base.Visible(), quiero)
		}
		if !quiero {
			continue
		}
		if r.base.Size() != menu.Size() || r.base.FillColor != color.Color(tema.Color("neutral-50", v)) {
			t.Errorf("la base mide %v y pinta %v; quiero el tamaño del menú (%v) y `neutral-50`", r.base.Size(), r.base.FillColor, menu.Size())
		}
		// Cerrado el cajón, fuera.
		a.CajonAbierto = false
		r.Layout(a.Size())
		if r.base.Visible() {
			t.Error("con el cajón cerrado la base sigue a la vista")
		}
	}
}

// TestSeparadorPanelRedimensionable verifica que el separador entre el panel de lista
// y el detalle se puede arrastrar para redimensionar, acota mínimos y máximos,
// responde al teclado y se restablece con doble clic.
func TestSeparadorPanelRedimensionable(t *testing.T) {
	menu := NuevoLateral("Navegación principal", nil)
	panel, detalle := iconoFalso(), iconoFalso()
	a := NuevoArmazon(detalle)
	a.Menu, a.Panel = menu, panel
	a.Resize(fyne.NewSize(1280, 744))
	r := test.WidgetRenderer(a).(*armazonRenderer)
	a.Refresh()

	sep := r.separador
	if sep == nil {
		t.Fatal("el armazón con panel y contenido debe tener un separadorPanel")
	}
	if !sep.Visible() {
		t.Fatal("el separador debe ser visible en escritorio")
	}
	if sep.Cursor() != desktop.HResizeCursor {
		t.Errorf("el cursor del separador debe ser HResizeCursor, obtuve %v", sep.Cursor())
	}

	// Posición inicial: menú 260, panel 320 -> separador centrado en 580 (x=580 - AnchoSeparadorPanel/2)
	xEsperado := float32(580 - AnchoSeparadorPanel/2)
	if sep.Position().X != xEsperado {
		t.Errorf("separador en x=%v, esperado %v", sep.Position().X, xEsperado)
	}

	// 1. Arrastrar hacia la derecha (+50px)
	sep.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(xEsperado+50, 100)},
		Dragged:    fyne.Delta{DX: 50, DY: 0},
	})
	sep.DragEnd()
	if panel.Size().Width != 370 {
		t.Errorf("tras arrastrar +50, el panel debe medir 370, mide %v", panel.Size().Width)
	}
	if detalle.Position().X != 260+370 {
		t.Errorf("tras arrastrar +50, el detalle debe empezar en %v, empieza en %v", 260+370, detalle.Position().X)
	}

	// 2. Acotación al mínimo (AnchoPanelMinimo = 180)
	sep.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, 100)},
		Dragged:    fyne.Delta{DX: -500, DY: 0},
	})
	sep.DragEnd()
	if panel.Size().Width != AnchoPanelMinimo {
		t.Errorf("el panel no debe bajar de AnchoPanelMinimo (180), mide %v", panel.Size().Width)
	}

	// 3. Acotación al máximo (ancho disponible - AnchoContenidoMinimo)
	sep.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(1280, 100)},
		Dragged:    fyne.Delta{DX: 2000, DY: 0},
	})
	sep.DragEnd()
	maxEsperado := a.anchoMaximoPanel()
	if panel.Size().Width != maxEsperado {
		t.Errorf("el panel no debe superar el máximo (%v), mide %v", maxEsperado, panel.Size().Width)
	}

	// 4. Doble clic para restablecer ancho por defecto
	sep.DoubleTapped(&fyne.PointEvent{})
	if panel.Size().Width != 320 {
		t.Errorf("tras doble clic el panel debe volver a 320, mide %v", panel.Size().Width)
	}

	// 5. Teclado: flechas izquierda/derecha, Home, End
	sep.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if panel.Size().Width != 320+24 {
		t.Errorf("tras flecha derecha el panel debe medir 344, mide %v", panel.Size().Width)
	}
	sep.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if panel.Size().Width != 320 {
		t.Errorf("tras flecha izquierda el panel debe medir 320, mide %v", panel.Size().Width)
	}
	sep.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	if panel.Size().Width != AnchoPanelMinimo {
		t.Errorf("tras Home el panel debe medir %v, mide %v", AnchoPanelMinimo, panel.Size().Width)
	}
	sep.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if panel.Size().Width != maxEsperado {
		t.Errorf("tras End el panel debe medir %v, mide %v", maxEsperado, panel.Size().Width)
	}

	// 6. En ventana estrecha, el separador no debe ser visible
	a.Resize(fyne.NewSize(390, 844))
	a.Refresh()
	if sep.Visible() {
		t.Error("en ventana estrecha el separador debe estar oculto")
	}
}
