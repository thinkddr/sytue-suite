// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElBotonMideLoQueDiceMedir es EL guardián del patrón, y el que hay que copiar para
// cada widget nuevo.
//
// Comprueba tres cosas distintas, y las tres hacen falta:
//
//  1. El alto declarado es el de la web. El número viene de Button.tsx (`h-9` = 2,25rem
//     con el rem de 14 = 31,5 px) y no de esta implementación.
//  2. Lo declarado y lo que el widget le pide a Fyne son lo mismo (`MinSize`).
//  3. Lo declarado y lo PINTADO son lo mismo. Sin esto, un widget puede decir 32 y
//     dejar 33 filas de tinta, que es justo el fallo que nadie ve hasta comparar
//     capturas.
func TestElBotonMideLoQueDiceMedir(t *testing.T) {
	// `h-9` de Button.tsx son 2,25rem, y con el rem de 14 de esta suite, 31,5 px de
	// CSS. Sin redondear: a ×2 son 63 filas clavadas (TestLaGeometriaEsExactaADoble).
	const altoWeb = 2.25 * tema.Rem

	b := NuevoBoton("Guardar cambios", nil)
	declarada := MedidaBoton(b.Texto, b.Talla, b.Ancho)
	if declarada.Height != altoWeb {
		t.Errorf("MedidaBoton dice %v de alto y `h-9` con el rem de 14 son %v", declarada.Height, float32(altoWeb))
	}
	if got := b.MinSize(); got != declarada {
		t.Errorf("MinSize() = %v y MedidaBoton dice %v: hay dos cuentas donde tiene que haber una", got, declarada)
	}

	// Y lo PINTADO, a ×2, donde el medio píxel de `h-9` es exacto. El botón principal
	// es relleno opaco sobre blanco, así que se rastrea su color directamente.
	b.Resize(declarada)
	img := pinta(t, declarada.Width+20, declarada.Height+20, b)
	x0, x1, y0, y1 := caja(img, tema.Color("primary", tema.Claro))
	mismaCaja(t, "el botón", declarada, x0, x1, y0, y1)
}

// TestElBotonSecundarioEsAzulComoElPrimario fija la acción azul de Button.tsx.
func TestElBotonSecundarioEsAzulComoElPrimario(t *testing.T) {
	b := NuevoBoton("Cancelar", nil).Con(Secundario, TallaMD)
	m := b.MinSize()
	b.Resize(m)
	img := pinta(t, m.Width+10, m.Height+10, b)
	for _, x := range []int{0, int(tema.Borde * escala)} {
		quiero := tema.Color("primary", tema.Claro)
		if x == 0 {
			quiero = BordeSobre("border-strong", "primary", tema.Claro)
		}
		if got := px(img, x, int(m.Height*escala/2)); !cerca(got, quiero) {
			t.Errorf("el secundario pinta %v; Button.tsx usa el azul %v", got, quiero)
		}
	}
}

// TestElBotonInerteSePintaAMitadDeOpacidad es `disabled:opacity-50`, y de paso fija que
// un botón inerte no responde ni al puntero ni a la pulsación.
func TestElBotonInerteSePintaAMitadDeOpacidad(t *testing.T) {
	pulsado := false
	b := NuevoBoton("Enviar", func() { pulsado = true })
	b.Inerte = true
	m := MedidaBoton(b.Texto, b.Talla, b.Ancho)
	b.Resize(m)
	img := pinta(t, m.Width+10, m.Height+10, b)

	lleno := tema.Color("primary", tema.Claro)
	mitad := color.NRGBA{R: lleno.R, G: lleno.G, B: lleno.B, A: lleno.A / 2}
	sobreSuperficie := func(c color.NRGBA) color.NRGBA {
		a := float64(c.A) / 255
		fondo := tema.Color("surface", tema.Claro)
		m := func(v, f uint8) uint8 { return uint8(float64(f) + (float64(v)-float64(f))*a + 0.5) }
		return color.NRGBA{R: m(c.R, fondo.R), G: m(c.G, fondo.G), B: m(c.B, fondo.B), A: 255}
	}
	if got := px(img, int(m.Width*escala/2), int(m.Height*escala/2)); !cerca(got, sobreSuperficie(mitad)) {
		t.Errorf("el botón inerte pinta %v y al 50 %% sobre surface son %v", got, sobreSuperficie(mitad))
	}

	test.Tap(b)
	if pulsado {
		t.Error("un botón inerte ha llamado a AlPulsar")
	}
}

// TestSinRotuloElBotonNoCuentaElHuecoDelTexto: un rótulo en `sr-only` sale del flujo
// (`position: absolute`) y el `gap-2` deja de contarlo. Es la cabecera del lector de
// Sytue Mail en el móvil, donde «Responder a todos», «Etiquetar» y «Mover a» se quedan
// en su glifo. Hoja: `h-8 px-3 gap-2` con glifos de 16.
//
// VISTO EN ROJO (2026-09-22): con la cuenta de antes —un `gap` por glifo— salen 44 y 67
// donde la web pinta 37 y 60, y el chevron se pinta 7 px a la derecha.
func TestSinRotuloElBotonNoCuentaElHuecoDelTexto(t *testing.T) {
	px3, gap2 := tema.Espacio(3), tema.Espacio(2)
	if got := MedidaBotonCon("", TallaSM, 0, 1, false, 16).Width; got != 2*px3+16 {
		t.Errorf("un glifo sin rótulo mide %v y son %v (`px-3` y el glifo)", got, 2*px3+16)
	}
	if got := MedidaBotonCon("", TallaSM, 0, 2, false, 16).Width; got != 2*px3+16+gap2+16 {
		t.Errorf("dos glifos sin rótulo miden %v y son %v (un solo `gap-2` entre ellos)", got, 2*px3+16+gap2+16)
	}
	conTexto := MedidaBotonCon("Mover a", TallaSM, 0, 2, false, 16).Width
	if want := Medir("Mover a", tema.TextoSM, PesoRotulo).Width + 2*(16+gap2) + 2*px3; conTexto != want {
		t.Errorf("con rótulo y dos glifos mide %v y son %v: los dos `gap` siguen ahí", conTexto, want)
	}
}

// TestElSecundarioSumaSuBordeAlAncho: es la única variante con `border`, y con
// `box-sizing: border-box` el borde va fuera del `px-*`. Medido en el lector de Sytue
// Mail (arnés sin sesión, 2026-09-22): «Responder a todos» con su glifo en el móvil
// mide 39 como secundario —21 de `px-3`, 16 de glifo y 2 de borde— y «Etiquetar», que
// es fantasma, 60 sin borde.
//
// VISTO EN ROJO (2026-09-22): sin el borde, el secundario de solo glifo sale de 37.
func TestElSecundarioSumaSuBordeAlAncho(t *testing.T) {
	sec := NuevoBoton("", nil).Con(Secundario, TallaSM)
	sec.Glifo, sec.TamIcono = iconos.TrazoConTinta(iconos.Mas, 16, tema.Color("text", tema.Claro)), 16
	if got := sec.MinSize().Width; got != 42 {
		t.Errorf("el secundario de solo glifo mide %v y en la web 39", got)
	}
	fan := NuevoBoton("", nil).Con(Fantasma, TallaSM)
	fan.Glifo, fan.TamIcono = sec.Glifo, 16
	if got := fan.MinSize().Width; got != 40 {
		t.Errorf("el fantasma de solo glifo mide %v y son 37: el borde es solo del secundario", got)
	}
	cuadrado := NuevoBoton("Destacar", nil).Con(Secundario, TallaSM)
	cuadrado.SoloIcono = true
	if got := cuadrado.MinSize().Width; got != 32 {
		t.Errorf("un secundario `iconOnly` mide %v y `w-8` ya contiene el borde: 28", got)
	}
}

// TestElMargenDelIconoSeSuma: el ERP pone `<Icon class="mr-1" />` en el glifo de sus
// botones, y ese margen se SUMA al `gap` de la talla. Medido en la ficha del ERP (arnés
// del frontal, 2026-09-23): «UBL», fantasma `sm` con el icono de 14, 66,48; y sin rótulo
// («Rellenar con Sytue AI») el margen no cuenta: 35.
//
// VISTO EN ROJO (frontal, 2026-09-23): sin sumar `MargenIcono`, «UBL» mide 62,98.
func TestElMargenDelIconoSeSuma(t *testing.T) {
	b := NuevoBoton("", nil).Con(Fantasma, TallaSM)
	b.ConIcono(iconos.IA, 14)
	b.MargenIcono = 4
	casi(t, "icono sin rótulo", b.MinSize().Width, 38)

	u := NuevoBoton("UBL", nil).Con(Fantasma, TallaSM)
	u.ConIcono(iconos.Descargar, 14)
	u.MargenIcono = 4
	if d := u.MinSize().Width - 74.016; d < -0.5 || d > 0.5 {
		t.Errorf("«UBL» con `mr-1` mide %v; la web 66,48", u.MinSize().Width)
	}
}

// TestElBotonSeVePulsadoMientrasSeAprieta es `:active`, que nadie ponía: el estado
// existía en `colores` y en cuatro piezas más, y no se veía nunca. VISTO EN ROJO sin
// `MouseDown`: el primario seguía en `primary` con el botón apretado.
func TestElBotonSeVePulsadoMientrasSeAprieta(t *testing.T) {
	b := NuevoBoton("Guardar", nil)
	v := tema.Claro
	b.MouseDown(nil)
	if relleno, _, _ := b.colores(v); relleno != tema.Color("primary-active", v) {
		t.Errorf("apretado, el primario pinta %v y `active:` es `primary-active` %v", relleno, tema.Color("primary-active", v))
	}
	b.MouseUp(nil)
	if relleno, _, _ := b.colores(v); relleno != tema.Color("primary", v) {
		t.Errorf("soltado, el primario pinta %v y en reposo es `primary`", relleno)
	}
	f := NuevaFilaLateral("Recibidos", nil, nil)
	f.MouseDown(nil)
	if fondo, _ := f.colores(v); fondo.A == 0 {
		t.Error("apretada, la fila del menú no pinta el `active:bg-sidebar-active/60`")
	}
}

// TestConElDedoElBotonNoBajaDe44 es el `pointer-coarse:min-h-[max(44px,2.75rem)]` de
// Button.tsx en las cuatro tallas, y el `min-w` en los de solo icono: el botón de menú de
// la barra móvil de Notepads (`sm`, solo icono) mide 28×28 con ratón y 44×44 en un
// teléfono. VISTO EN ROJO sin el suelo: 28×28 también con el dedo.
func TestConElDedoElBotonNoBajaDe44(t *testing.T) {
	if got := MedidaBotonCon("", TallaSM, 0, 1, true, 18); got != fyne.NewSize(32, 32) {
		t.Fatalf("con ratón el `sm` de solo icono mide %v y es `h-8 w-8`, 28×28", got)
	}
	si := true
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = nil })
	if got := MedidaBotonCon("", TallaSM, 0, 1, true, 18); got != fyne.NewSize(44, 44) {
		t.Errorf("con el dedo el `sm` de solo icono mide %v y son 44×44", got)
	}
	for _, talla := range []Talla{TallaXS, TallaSM, TallaMD, TallaLG} {
		if got := MedidaBoton("Guardar", talla, 0).Height; got != 44 {
			t.Errorf("con el dedo la talla %d mide %v de alto y el suelo es 44", talla, got)
		}
	}
}

// TestElFantasmaAtenuadoTieneLaTintaDeLaWeb es `ACTION_BTN` de RevisionPanel.tsx (y
// `TEXT_BTN` de CommentsPanel.tsx): rótulo y glifo en `text-muted` en reposo y en `text`
// con el puntero (el glifo se mira en la variante de la app). Los números son de Chromium (`getComputedStyle`, color del botón y
// `stroke` del SVG) sobre `Button size="xs" variant="ghost"` con esa clase, medidos el
// 2026-09-25 en el frontal. Se vio en ROJO sin `Atenuado`: el reposo salía en `text`.
func TestElFantasmaAtenuadoTieneLaTintaDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		v              tema.Variante
		reposo, encima color.NRGBA
	}{
		// `text-muted` y `text` de la paleta petróleo: #43555a / #0f1d20 y #a6b8ba / #e7efef.
		{tema.Claro, color.NRGBA{67, 85, 90, 255}, color.NRGBA{15, 29, 32, 255}},
		{tema.Oscuro, color.NRGBA{166, 184, 186, 255}, color.NRGBA{231, 239, 239, 255}},
	} {
		b := NuevoBoton("Aceptar", nil).Con(Fantasma, TallaXS)
		b.Glifo, b.TamIcono, b.Atenuado = iconos.TrazoConTinta(iconos.Tic, 14, tema.Color("text-muted", c.v)), 14, true
		r := test.WidgetRenderer(b).(*botonRenderer)
		for _, fase := range []struct {
			nombre string
			quiero color.NRGBA
			hacer  func()
		}{
			{"reposo", c.reposo, func() {}},
			{"con el puntero", c.encima, func() { b.MouseIn(nil) }},
			{"de vuelta", c.reposo, func() { b.MouseOut() }},
		} {
			fase.hacer()
			_, texto, _ := b.colores(c.v)
			if texto != fase.quiero {
				t.Errorf("%v, %s: el rótulo pinta %v y la web %v", c.v, fase.nombre, texto, fase.quiero)
			}
			r.Refresh() // el renderer pinta con la variante de la app: el glifo, solo en esa
			if c.v != tema.De(fyne.CurrentApp().Settings().ThemeVariant()) {
				continue
			}
			if g := tintaDe(r.delante.Objeto().(*canvas.Raster)); g != fase.quiero {
				t.Errorf("%v, %s: el glifo pinta %v y la web %v", c.v, fase.nombre, g, fase.quiero)
			}
		}
	}
	if _, texto, _ := NuevoBoton("x", nil).Con(Fantasma, TallaXS).colores(tema.Claro); texto != tema.Color("primary-text", tema.Claro) {
		t.Errorf("sin Atenuado, el fantasma sigue en `text`: %v", texto)
	}
}

// tintaDe es el color del píxel más opaco de un glifo: el trazo sin el antialias.
func tintaDe(r *canvas.Raster) color.NRGBA {
	img := r.Generator(28, 28)
	var mejor color.NRGBA
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if p := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA); p.A > mejor.A {
				mejor = p
			}
		}
	}
	mejor.A = 255
	return mejor
}

// TestLaTallaDeTextoEsElBotonQueLaWebEscribeAMano: el «+ Añadir» del Solver es
// `text-2xs inline-flex h-5 items-center gap-0.5 rounded-sm px-1` con el «+» de 14:
// 17,5 de alto, el rótulo de peso normal y sin el suelo táctil de `Button`.
func TestLaTallaDeTextoEsElBotonQueLaWebEscribeAMano(t *testing.T) {
	b := NuevoBoton("Añadir", nil).Con(Fantasma, TallaTexto).ConIcono(iconos.Mas, 14)
	b.Atenuado = true
	quiero := fyne.NewSize(tema.Espacio(1)*2+14+tema.Espacio(0.5)+Medir("Añadir", tema.Texto2XS, Normal).Width, tema.Espacio(5))
	if got := b.MinSize(); got != quiero {
		t.Errorf("mide %v y la web %v", got, quiero)
	}
	si := true
	tactilDePrueba = &si
	defer func() { tactilDePrueba = nil }()
	if got := b.MinSize().Height; got != tema.Espacio(5) {
		t.Errorf("con el dedo sube a %v: el `<button>` a mano no lleva el `min-h` de Button", got)
	}
	if radioTalla(TallaTexto) != 4 || radioTalla(TallaXS) != 4 {
		t.Error("las dos tallas comparten la esquina de 4 px")
	}
}
