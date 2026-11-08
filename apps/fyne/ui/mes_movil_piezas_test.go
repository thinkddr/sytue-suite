// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las piezas que el mes del teléfono de Calendar le pide al catálogo. Sus parejas en la
// web son `mobileBarCenter`/`mobileBarActions`/`mobileNavNotice` de AppShell (con sus
// tests en AppShell.test.tsx), `SidebarItem` rama y `useEstrecha`.

// TestLaBarraMovilCentraElTituloYPoneLasAcciones: el título en `text-md` centrado en el
// ancho ENTERO (no en el hueco entre botones), las acciones a la derecha con el borde en
// el canalón y la corrección táctil de la hamburguesa, y entre ellas `gap-1`.
//
// Con el DEDO, que es donde los botones crecen a 44 y la corrección se nota.
//
// VISTO EN ROJO centrando el título en el hueco entre el ☰ y las acciones (su centro cae
// en 174), y colocando la última acción sin la corrección táctil (acaba en 374).
func TestLaBarraMovilCentraElTituloYPoneLasAcciones(t *testing.T) {
	t.Cleanup(TactilDePrueba(true))
	b := NuevaBarraMovil()
	b.Titulo, b.TituloCentrado, b.AlMenu = "noviembre", true, func() {}
	hoy := NuevoBotonBarraMovil(iconos.Calendario, "Hoy", nil)
	mas := NuevoBotonBarraMovil(iconos.Mas, "Crear", nil)
	b.Acciones = []fyne.CanvasObject{hoy, mas}
	b.Resize(fyne.NewSize(390, AltoBarraMovil))
	r := test.WidgetRenderer(b).(*barraMovilRenderer)
	r.Layout(b.Size())

	if r.titulo.TextSize != tema.TextoMD.Tam {
		t.Errorf("el título centrado mide %v y es `text-md` (%v)", r.titulo.TextSize, tema.TextoMD.Tam)
	}
	ancho := Medir(r.titulo.Text, tema.TextoMD, SemiNegrita).Width
	if centro := r.titulo.Position().X + ancho/2; centro < 194 || centro > 196 {
		t.Errorf("el centro del título está en %v y va en el de la barra (195)", centro)
	}
	lado := LadoBotonBarraMovil()
	derecha := 390 - max(canalonBarraMovil-max(lado-ladoSoloIcono(TallaSM), 0)/2, 0)
	if lado <= ladoSoloIcono(TallaSM) {
		t.Fatalf("con el dedo el botón mide %v y tendría que crecer", lado)
	}
	if x := mas.Position().X + mas.Size().Width; x != derecha {
		t.Errorf("la última acción acaba en %v y va en el canalón, como el ☰ (%v)", x, derecha)
	}
	if hueco := mas.Position().X - (hoy.Position().X + hoy.Size().Width); hueco != huecoAccionesMovil {
		t.Errorf("entre las acciones hay %v y es `gap-1` (%v)", hueco, huecoAccionesMovil)
	}
	if hoy.Trazo != r.boton.Trazo || mas.TokenAro != r.boton.TokenAro {
		t.Errorf("las acciones no toman los tonos de la barra: %q/%q frente a %q/%q", hoy.Trazo, mas.TokenAro, r.boton.Trazo, r.boton.TokenAro)
	}
}

// TestLaBarraMovilSinNovedadesQuedaComoEstaba: sin TituloCentrado, Acciones ni Aviso,
// el título sigue en `text-sm` junto al botón y no hay punto. Es el «byte a byte como
// hoy» del diseño, del lado de Fyne.
func TestLaBarraMovilSinNovedadesQuedaComoEstaba(t *testing.T) {
	b := NuevaBarraMovil()
	b.Titulo, b.AlMenu, b.RotuloAbrir = "Calendario", func() {}, "Abrir navegación"
	b.Resize(fyne.NewSize(390, AltoBarraMovil))
	r := test.WidgetRenderer(b).(*barraMovilRenderer)
	r.Layout(b.Size())
	if r.titulo.TextSize != tema.TextoSM.Tam || r.titulo.Position().X != 16+32+6 {
		t.Errorf("título de %v en x=%v: quiero `text-sm` tras el botón (54)", r.titulo.TextSize, r.titulo.Position().X)
	}
	if r.punto.Visible() || r.boton.Rotulo != "Abrir navegación" {
		t.Errorf("sin aviso: punto %v, nombre %q", r.punto.Visible(), r.boton.Rotulo)
	}
}

// TestElAvisoDelMenuPintaElPuntoYCambiaElNombre: con algo pendiente el ☰ cerrado se
// llama como dice el aviso y lleva el punto de 8 en la esquina del glifo, con el color de
// señal; abierto, vuelve a «Cerrar navegación» y sin punto.
//
// VISTO EN ROJO dejando `RotuloAbrir` en el caso del aviso, y sin esconder el punto con
// el cajón abierto.
func TestElAvisoDelMenuPintaElPuntoYCambiaElNombre(t *testing.T) {
	b := NuevaBarraMovil()
	b.AlMenu, b.RotuloAbrir, b.RotuloCerrar = func() {}, "Abrir navegación", "Cerrar navegación"
	b.Aviso = "Abrir menú, 3 pendientes"
	b.Resize(fyne.NewSize(390, AltoBarraMovil))
	r := test.WidgetRenderer(b).(*barraMovilRenderer)
	r.Layout(b.Size())
	if r.boton.Rotulo != b.Aviso || !r.punto.Visible() {
		t.Fatalf("con aviso: nombre %q, punto %v", r.boton.Rotulo, r.punto.Visible())
	}
	if s := r.punto.Size(); s.Width != LadoPuntoAvisoMovil || s.Height != LadoPuntoAvisoMovil {
		t.Errorf("el punto mide %v y es de 8", s)
	}
	cx := r.boton.Position().X + r.boton.Size().Width/2
	if x := r.punto.Position().X + LadoPuntoAvisoMovil; x != cx+DesvioPuntoAvisoMovil {
		t.Errorf("el punto acaba en %v y va a 15 del centro del botón (%v)", x, cx+DesvioPuntoAvisoMovil)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	token := "indicator"
	if tema.EsMarco() {
		token = "sidebar-indicator"
	}
	if r.punto.FillColor != tema.Color(token, v) {
		t.Errorf("el punto es %v y es el color de señal %s", r.punto.FillColor, token)
	}
	b.CajonAbierto = true
	b.Refresh()
	if r.boton.Rotulo != "Cerrar navegación" || r.punto.Visible() {
		t.Errorf("abierto: nombre %q, punto %v", r.boton.Rotulo, r.punto.Visible())
	}
}

// TestAlMedirAvisaSoloCuandoCambiaElAncho: el armazón dice su ancho al recolocarse si
// cambió, y no al recolocar con el mismo (Calendar reconstruye la pantalla al cruzar 640:
// avisar en cada Layout la reconstruiría sin fin).
//
// VISTO EN ROJO quitando la comparación con `anchoMedido`: avisa tres veces.
func TestAlMedirAvisaSoloCuandoCambiaElAncho(t *testing.T) {
	a := NuevoArmazon(canvas.NewRectangle(nil))
	var anchos []float32
	a.AlMedir = func(w float32) { anchos = append(anchos, w) }
	w := test.NewWindow(a)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	if n := len(anchos); n == 0 || anchos[n-1] != 390 {
		t.Fatalf("a 390 AlMedir recibió %v y el último tiene que ser 390", anchos)
	}
	anchos = nil
	a.Refresh()
	test.WidgetRenderer(a).Layout(a.Size())
	w.Resize(fyne.NewSize(1280, 800))
	if len(anchos) != 1 || anchos[0] != 1280 {
		t.Errorf("recolocar a 390 y pasar a 1280 avisó %v y quiero solo [1280]", anchos)
	}
}

// TestSubirRunaLlegaALaRanuraDeLaVentana: la letra que el control enfocado no usa llega
// al `OnTypedRune` de la ventana, que es donde Calendar atiende sus atajos.
func TestSubirRunaLlegaALaRanuraDeLaVentana(t *testing.T) {
	w := test.NewWindow(canvas.NewRectangle(nil))
	t.Cleanup(w.Close)
	var llego []rune
	w.Canvas().SetOnTypedRune(func(r rune) { llego = append(llego, r) })
	SubirRuna(w.Canvas(), 't')
	SubirRuna(nil, 'x') // sin lienzo, nada
	if len(llego) != 1 || llego[0] != 't' {
		t.Errorf("la ranura recibió %q y quiero «t»", string(llego))
	}
}

// TestRamaDesplegable: la fila lleva el galón y no cierra el cajón; plegada va sola y
// desplegada con sus hijos debajo, sangrados `ml-7`; pulsarla avisa al plegador.
//
// VISTO EN ROJO sin `MantieneCajon` y devolviendo los hijos también plegada.
func TestRamaDesplegable(t *testing.T) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	pulsada := 0
	f := NuevaFilaLateral("Recordatorios", nil, nil)
	hijo := canvas.NewRectangle(nil)
	plegada := RamaDesplegable(f, false, func() { pulsada++ }, v, hijo)
	if len(plegada) != 1 || !f.MantieneCajon || f.Cola == nil {
		t.Fatalf("plegada: %d objetos, mantiene el cajón %v, galón %v", len(plegada), f.MantieneCajon, f.Cola != nil)
	}
	f.Tapped(nil)
	if pulsada != 1 {
		t.Error("pulsar la rama no avisó al plegador")
	}
	abierta := RamaDesplegable(f, true, nil, v, hijo)
	if len(abierta) != 2 {
		t.Fatalf("desplegada devuelve %d objetos y son la fila y sus hijos", len(abierta))
	}
	c, _ := abierta[1].(*fyne.Container)
	if c == nil {
		t.Fatal("los hijos no van en un contenedor")
	}
	if m, ok := c.Layout.(Margen); !ok || m.Izq != tema.Espacio(7) {
		t.Errorf("los hijos van con %#v y es `ml-7`", c.Layout)
	}
}

// TestElTituloLargoUsaElHuecoAntesDeTruncar: a 320 «Σεπτέμβριος 2027» no cabe centrado
// en el ancho entero, pero sí en el hueco entre el ☰ y las acciones: entero, dentro del
// hueco y lo más cerca del centro que deja el lado más ocupado. Si en `text-md` no cabe
// ni en el hueco, baja a `text-sm`; y si tampoco, trunca al hueco. Es la rejilla de tres columnas de AppShell.
// VISTO EN ROJO con el truncado a `ancho − 2·ocupado` (salía «Σεπτέμβριος 20…»).
func TestElTituloLargoUsaElHuecoAntesDeTruncar(t *testing.T) {
	t.Cleanup(TactilDePrueba(true))
	b := NuevaBarraMovil()
	b.Titulo, b.TituloCentrado, b.AlMenu = "Σεπτέμβριος 2027", true, func() {}
	hoy := NuevoBotonBarraMovil(iconos.Calendario, "Hoy", nil)
	mas := NuevoBotonBarraMovil(iconos.Mas, "Crear", nil)
	b.Acciones = []fyne.CanvasObject{hoy, mas}
	b.Resize(fyne.NewSize(320, AltoBarraMovil))
	r := test.WidgetRenderer(b).(*barraMovilRenderer)
	r.Layout(b.Size())
	if r.titulo.Text != "Σεπτέμβριος 2027" {
		t.Fatalf("a 320 el título sale %q y cabe entero en el hueco", r.titulo.Text)
	}
	ancho := Medir(r.titulo.Text, tema.TextoMD, SemiNegrita).Width
	izq := r.boton.Position().X + r.boton.Size().Width + huecoBarraMovil
	dcha := hoy.Position().X - huecoBarraMovil
	if x := r.titulo.Position().X; x < izq-0.5 || x+ancho > dcha+0.5 {
		t.Errorf("el título va de %v a %v y el hueco de %v a %v", x, x+ancho, izq, dcha)
	}
	if x := r.titulo.Position().X; x+ancho < dcha-0.5 {
		t.Errorf("el título acaba en %v: no se acerca al centro todo lo que puede (%v)", x+ancho, dcha)
	}

	if r.titulo.TextSize != tema.TextoMD.Tam {
		t.Errorf("cabiendo en el hueco el título baja a %v", r.titulo.TextSize)
	}

	// Un poco más largo de lo que cabe en `text-md`: baja a `text-sm` y no trunca.
	b.Titulo = "Σεπτέμβριος 2027 · 28"
	r.Layout(b.Size())
	if Medir(b.Titulo, tema.TextoMD, SemiNegrita).Width <= dcha-izq || Medir(b.Titulo, tema.TextoSM, SemiNegrita).Width > dcha-izq {
		t.Fatalf("el caso de prueba no cae entre `text-sm` y `text-md` en %v", dcha-izq)
	}
	if r.titulo.Text != b.Titulo || r.titulo.TextSize != tema.TextoSM.Tam {
		t.Errorf("sin sitio en `text-md` sale %q a %v y es entero en `text-sm`", r.titulo.Text, r.titulo.TextSize)
	}

	b.Titulo = "Un título tan largo que no cabe ni en el hueco entre los botones"
	r.Layout(b.Size())
	if got := Medir(r.titulo.Text, tema.TextoSM, SemiNegrita).Width; got > dcha-izq+0.5 || r.titulo.Text == b.Titulo {
		t.Errorf("sin sitio el título no trunca al hueco: %q mide %v", r.titulo.Text, got)
	}
}

// TestElRotuloTruncadoSeAdaptaAlAncho: con `Truncar` el rótulo no pide ancho propio y
// recorta con «…» lo que no cabe, también tras cambiar el texto y repintar; sin él sigue
// pidiendo su ancho entero (los demás usos no cambian). Es el título de la lista del día
// del mes del teléfono. VISTO EN ROJO con el Layout sin truncar (el texto sale entero).
func TestElRotuloTruncadoSeAdaptaAlAncho(t *testing.T) {
	const largo = "miércoles, 30 de septiembre de 2026"
	entero := NuevoRotulo(largo, tema.TextoMD, SemiNegrita, "text")
	if w := test.WidgetRenderer(entero).MinSize().Width; w != Medir(largo, tema.TextoMD, SemiNegrita).Width {
		t.Errorf("sin Truncar pide %v de ancho y es el del texto", w)
	}
	r := NuevoRotulo(largo, tema.TextoMD, SemiNegrita, "text")
	r.Truncar = true
	rr := test.WidgetRenderer(r)
	if m := rr.MinSize(); m.Width != 0 || m.Height != tema.TextoMD.Linea {
		t.Errorf("truncado pide %v y es 0 × la caja de línea", m)
	}
	r.Resize(fyne.NewSize(120, tema.TextoMD.Linea))
	txt := rr.Objects()[0].(*canvas.Text)
	cabe := func() {
		t.Helper()
		if w := Medir(txt.Text, tema.TextoMD, SemiNegrita).Width; w > 120 || txt.Text == r.Texto {
			t.Errorf("a 120 pinta %q (%v de ancho)", txt.Text, w)
		}
	}
	cabe()
	r.Texto = "jueves, 31 de diciembre de 2026"
	r.Refresh()
	cabe()
	r.Resize(fyne.NewSize(400, tema.TextoMD.Linea))
	if txt.Text != r.Texto {
		t.Errorf("a 400 cabe entero y pinta %q", txt.Text)
	}
}
