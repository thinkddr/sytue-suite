// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LA BARRA ÚNICA DEL TELÉFONO, como la web (TopBar.tsx con `movil` y BarraMovil.tsx):
// por debajo de 768 la barra móvil del armazón sube a la barra de la suite —una sola
// barra, no dos apiladas— y la suite se pliega tras la entrada de la derecha, el avatar
// de la cuenta. Al tocarla se despliega con un ✕ delante; desde 768 nada cambia.
//
// VISTO EN ROJO: con `unica` siempre a false en el montaje (la barra móvil vuelve a ir
// debajo, a 60 de alto, y falla «una sola barra»), y con el `AlPulsar` de la cuenta sin
// el desvío a `desplegar` (el primer toque abría el menú de cuenta y falla «despliega»).
func TestLaBarraUnicaDelTelefono(t *testing.T) {
	defer TactilDePrueba(true)()
	a := NuevoArmazon(contenidoDePrueba())
	a.Menu, a.Titulo = NuevoLateral("Menú", nil), "Bandeja"
	b, parar := BarraSuiteDePrueba(traductorPrueba(t), iconos.Mail)
	t.Cleanup(parar)
	w := test.NewWindow(b.Montar(a))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 700))

	// Una sola barra: la de la app va arriba del todo, mide lo que la de la suite y lleva
	// el ☰ y el título; la hoja empieza justo debajo.
	nav := a.barra
	if nav == nil || !nav.enSuite || nav.Position() != fyne.NewPos(0, 0) || nav.Size().Height != AltoBarraSuite() {
		t.Fatalf("una sola barra: la móvil está en %v y mide %v (quiero arriba y %v)",
			nav.Position(), nav.Size(), AltoBarraSuite())
	}
	if a.Contenido.Position().Y != AltoBarraSuite() {
		t.Errorf("el contenido empieza en %v y la barra mide %v", a.Contenido.Position().Y, AltoBarraSuite())
	}
	// El cajón arranca BAJO la barra única, que sigue viva (con el ☰ para cerrarlo).
	a.CajonAbierto = true
	a.Refresh()
	if y := a.Menu.Position().Y; y != AltoBarraSuite() {
		t.Errorf("el cajón arranca en y=%v y tapa la barra (mide %v)", y, AltoBarraSuite())
	}
	a.CajonAbierto = false
	a.Refresh()
	// La cara de la app: de la suite solo la entrada, a la derecha.
	for nombre, o := range map[string]fyne.CanvasObject{
		"la marca": b.marca, "el buscador": b.buscador.campo, "la campana": b.avisos.boton,
	} {
		if o.Visible() {
			t.Errorf("plegada, %s no se ve", nombre)
		}
	}
	entrada := b.cuenta.boton
	if !entrada.Visible() || entrada.Position().X+entrada.Size().Width > 390 ||
		!strings.HasPrefix(entrada.Nombre, "Sytue:") {
		t.Errorf("la entrada a la suite: visible=%v en %v, «%s»", entrada.Visible(), entrada.Position(), entrada.Nombre)
	}
	// Las acciones de la app no llegan a la entrada.
	if nav.reservaSuite < entrada.Size().Width {
		t.Errorf("la barra de la app reserva %v para una entrada de %v", nav.reservaSuite, entrada.Size().Width)
	}

	// Al tocarla se despliega la suite, con el ✕ delante; el ✕ la pliega.
	test.Tap(entrada)
	// Desplegada: buscador, campana y cuenta tras el ✕, y NINGÚN selector de apps ni la
	// marca que en la web lo abre (dueño: el selector es solo de la web).
	if !b.suiteAbierta || b.marca.Visible() || !b.buscador.campo.Visible() ||
		!b.avisos.boton.Visible() || !b.cerrarSuite.Visible() {
		t.Fatalf("la entrada despliega la suite: abierta=%v marca=%v buscador=%v campana=%v ✕=%v",
			b.suiteAbierta, b.marca.Visible(), b.buscador.campo.Visible(), b.avisos.boton.Visible(), b.cerrarSuite.Visible())
	}
	if b.buscador.campo.Position().X <= b.cerrarSuite.Position().X+b.cerrarSuite.Size().Width {
		t.Error("el buscador va tras el ✕")
	}
	test.Tap(b.cerrarSuite)
	if b.suiteAbierta || b.buscador.campo.Visible() {
		t.Error("el ✕ pliega la suite")
	}

	// Desde 768, la barra de siempre y la móvil escondida.
	w.Resize(fyne.NewSize(1280, 800))
	if nav.Visible() || !b.marca.Visible() || b.cerrarSuite.Visible() || b.tituloApp.Visible() {
		t.Errorf("en ancho nada cambia: móvil=%v marca=%v ✕=%v título=%v",
			nav.Visible(), b.marca.Visible(), b.cerrarSuite.Visible(), b.tituloApp.Visible())
	}
}

// Sin armazón con barra (el Hub, los editores), la barra única lleva el nombre del
// producto y la entrada: nunca se queda vacía.
func TestLaBarraUnicaSinBarraDeAppLlevaElProducto(t *testing.T) {
	defer TactilDePrueba(true)()
	b, parar := BarraSuiteDePrueba(traductorEs(t), iconos.Mail)
	t.Cleanup(parar)
	w := test.NewWindow(b.Montar(contenidoDePrueba()))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 700))
	// El nombre de MARCA, sin traducir: «Mail» (y no «Correo») en cualquier idioma.
	if !b.tituloApp.Visible() || b.tituloApp.Text != "Mail" {
		t.Errorf("sin barra de app, el título es la marca del producto: visible=%v «%s»",
			b.tituloApp.Visible(), b.tituloApp.Text)
	}
	// La marca en el sitio del ☰ y el título donde empieza con ☰.
	if !b.marca.Visible() || b.marca.conNombre || !b.cuenta.boton.Visible() {
		t.Error("plegada: la marca (sin nombre) en el sitio del ☰ y la entrada a la derecha")
	}
	if x := b.tituloApp.Position().X; x != canalonBarraUnica+LadoBotonBarraMovil()+huecoBarraUnica {
		t.Errorf("el título empieza en %v y con ☰ empieza en %v", x, canalonBarraUnica+LadoBotonBarraMovil()+huecoBarraUnica)
	}
}

// En Fyne NO hay selector de aplicaciones (dueño, 2026-11-07: «el app switcher NO debe
// existir en las apps Fyne, solo en web»): la marca de la barra es la cara del
// producto y no se pulsa ni despliega nada, en ninguna de las caras ni a ningún ancho.
func TestFyneNoTieneSelectorDeApps(t *testing.T) {
	b, parar := BarraSuiteDePrueba(traductorPrueba(t), iconos.Mail)
	t.Cleanup(parar)
	var o fyne.CanvasObject = b.marca
	if _, es := o.(fyne.Tappable); es {
		t.Error("la marca de la barra se puede pulsar: sería un selector de apps")
	}
	for _, obj := range test.WidgetRenderer(b).Objects() {
		if _, es := obj.(fyne.Tappable); es && obj != fyne.CanvasObject(b.cuenta.boton) &&
			obj != fyne.CanvasObject(b.avisos.boton) && obj != fyne.CanvasObject(b.cerrarSuite) &&
			obj != fyne.CanvasObject(b.buscador.limpiar) && obj != fyne.CanvasObject(b.buscador.campo) {
			t.Errorf("la barra monta un pulsable que no es buscador, campana, cuenta ni ✕: %T", obj)
		}
	}
}

// NINGUNA FRANJA CLARA entre la barra única y el contenido (dueño, 2026-11-07: «evita al
// 100 % franjas blancas que queden feas»; el precedente es la de 44 px que la web tuvo
// entre la topbar y el cajón). En Marco y en Carril, en claro y en oscuro, con la hoja y
// con el cajón abierto: bajo la barra, en los dos bordes y en el centro, cada píxel claro
// tiene que ser el de la barra o el de lo que sigue debajo (la hoja, el contenido).
//
// VISTO EN ROJO separando 12 px el contenido de la barra móvil del armazón: sale la banda
// del fondo de la página entre la barra de color y el contenido, en Carril claro.
func TestLaBarraUnicaNoDejaFranjasClaras(t *testing.T) {
	defer TactilDePrueba(true)()
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirForma("") })
	for _, forma := range []string{"marco", "carril"} {
		for _, modo := range []string{"claro", "oscuro"} {
			tema.ElegirForma(forma)
			tema.Elegir(modo)
			for _, cajon := range []bool{false, true} {
				a := NuevoArmazon(contenidoDePrueba())
				a.Menu, a.Titulo, a.CajonAbierto = NuevoLateral("Menú", nil), "Bandeja", cajon
				b, parar := BarraSuiteDePrueba(traductorPrueba(t), iconos.Mail)
				w := test.NewWindow(b.Montar(a))
				w.SetPadded(false)
				w.Resize(fyne.NewSize(390, 500))
				img := w.Canvas().Capture()
				k := float32(img.Bounds().Dx()) / 390
				alto := AltoBarraSuite()
				for _, x := range []float32{2, 195, 387} {
					barra := px(img, int(x*k), int((alto-3)*k))
					debajo := px(img, int(x*k), int((alto+40)*k))
					for _, dy := range []float32{1, 5, 10, 15} {
						p := px(img, int(x*k), int((alto+dy)*k))
						if luminancia(p) < 0.72 || cercaDe(p, barra, 14) || cercaDe(p, debajo, 14) {
							continue
						}
						t.Errorf("%s/%s cajón=%v: banda clara en (%v,%v) %v entre la barra %v y lo de debajo %v",
							forma, modo, cajon, x, alto+dy, p, barra, debajo)
						break
					}
				}
				w.Close()
				parar()
			}
		}
	}
}

// traductorEs es el catálogo común en castellano: con él, «Correo» saldría si alguien
// volviera a traducir el nombre de marca.
func traductorEs(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	return tr
}
