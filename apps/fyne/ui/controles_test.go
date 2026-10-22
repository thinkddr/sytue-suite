// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// # Cómo se mide aquí, y la trampa que hay que evitar
//
// Cada cota se compara con **la cifra absoluta de la hoja**, calculada a mano desde
// `rem = 16`, y NUNCA contra lo que el propio widget calcula ni contra el ancho del
// texto que lleva dentro.
//
// La diferencia no es de estilo. Un guardián que comprueba «el ancho menos el rótulo
// son 28» pasa en verde aunque el conformador esté haciendo el texto un 6 % más ancho,
// y entonces el control sale cuatro píxeles más largo que en la web sin que nada lo
// diga. Por eso aquí las cotas atadas son las que NO dependen del texto —altos,
// rellenos, posiciones de icono, lados de casilla, recorrido del pomo— y, para las que
// sí dependen, se ata dónde EMPIEZA la tinta, que también es un número de la hoja.

// casi compara con un margen de una centésima: los números de la hoja son exactos y lo
// único que se admite es el error de coma flotante de las medidas de CSS.
func casi(t *testing.T, que string, dio, laWeb float32) {
	t.Helper()
	d := dio - laWeb
	if d < 0 {
		d = -d
	}
	if d > 0.01 {
		t.Errorf("%s = %v; la web mide %v", que, dio, laWeb)
	}
}

func blanco() color.NRGBA { return color.NRGBA{R: 255, G: 255, B: 255, A: 255} }

// tintaEnBanda devuelve, en px de CSS, la caja de lo pintado que no es ninguno de los
// colores dados, mirando SOLO la franja horizontal [desdeY, hastaY).
//
// La franja no es un capricho: las esquinas redondeadas sí llevan antialias, así que un
// barrido de la captura entera cuenta como «tinta» los píxeles a medio camino entre el
// borde y el fondo y devuelve x = 0 siempre. Midiendo por el centro del control, donde
// los lados son rectos y Fyne no interpola, el número es el de la hoja.
func tintaEnBanda(img image.Image, desdeY, hastaY float32, fondos ...color.NRGBA) (x0, x1, y0, y1 float32) {
	ix0, ix1, iy0, iy1 := 1<<30, -1, 1<<30, -1
	b := img.Bounds()
	for y := max(int(desdeY*escala), 0); y < min(int(hastaY*escala), b.Dy()); y++ {
		for x := range b.Dx() {
			p := px(img, x, y)
			salta := false
			for _, f := range fondos {
				if cerca(p, f) {
					salta = true
				}
			}
			if salta {
				continue
			}
			ix0, ix1, iy0, iy1 = min(ix0, x), max(ix1, x), min(iy0, y), max(iy1, y)
		}
	}
	if ix1 < 0 {
		return -1, -1, -1, -1
	}
	return float32(ix0) / escala, float32(ix1) / escala, float32(iy0) / escala, float32(iy1) / escala
}

// tinta es tintaEnBanda sobre la captura entera.
func tinta(img image.Image, fondos ...color.NRGBA) (x0, x1, y0, y1 float32) {
	return tintaEnBanda(img, 0, float32(img.Bounds().Dy())/escala, fondos...)
}

// colocado pinta un widget en (margen, margen) a la medida dada, sobre un lienzo con
// sitio de sobra alrededor para ver lo que se salga de la caja (el aro).
func colocado(t *testing.T, w fyne.Widget, tam fyne.Size, margen float32) image.Image {
	t.Helper()
	w.Resize(tam)
	w.Move(fyne.NewPos(margen, margen))
	return pinta(t, tam.Width+2*margen, tam.Height+2*margen, w)
}

// --------------------------------------------------------------------------------
// 1. El glifo: la ruta es la de la web, letra por letra
// --------------------------------------------------------------------------------

// El guardián que comparaba los glifos con `Icon.tsx` se ha ido al paquete `iconos`,
// donde vive ahora el catálogo entero: allí compara los ochenta y tres por IGUALDAD de
// conjuntos, no los cuatro que hacían falta aquí (`iconos.TestElCatalogoEsElDeLaWeb`).

// --------------------------------------------------------------------------------
// 2. El aro de foco
// --------------------------------------------------------------------------------

// TestElAroEsElDeLaHoja ata los dos grosores al CSS que los declara. Si alguien engorda
// el aro «para que se vea mejor», esto le manda a `styles.css`, que es donde se decide
// para las dieciséis apps a la vez.
func TestElAroEsElDeLaHoja(t *testing.T) {
	hoja := webref.Read(t, "apps/frontend/packages/ui/src/styles.css")
	for _, c := range []struct{ que, css string }{
		{"el aro pegado", "box-shadow: 0 0 0 2px var(--sytue-focus-ring);"},
		{"el hueco del aro sobre relleno", "0 0 0 1px var(--sytue-surface),"},
		{"el anillo del aro sobre relleno", "0 0 0 3px var(--sytue-focus-ring);"},
	} {
		if !strings.Contains(hoja, c.css) {
			t.Errorf("%s ya no se declara así en styles.css: %q", c.que, c.css)
		}
	}
	if grosorAro != 2 {
		t.Errorf("el aro mide %v y la hoja dice 2 px", float32(grosorAro))
	}
	if ConHueco != 1 {
		t.Errorf("el hueco mide %v y la hoja dice 1 px (3 de sombra menos 2 de anillo)", ConHueco)
	}
}

// TestElAroSePintaFueraDeLaCajaYNoOcupaSitio es la comprobación que decide si el aro se
// puede hacer así en Fyne. `box-shadow` no participa en el layout de CSS, así que el
// widget tiene que seguir midiendo lo mismo con el foco puesto y pintar DOS píxeles más
// allá de su caja por cada lado.
//
// Se mide sobre la casilla, que es donde más se nota: allí el aro rodea el cuadradito
// de 16 y no la fila entera con su etiqueta, igual que en la web, donde lo lleva el
// `<input>` y no el `<label>`.
func TestElAroSePintaFueraDeLaCajaYNoOcupaSitio(t *testing.T) {
	const m = 8
	c := NuevaCasilla("", nil)
	c.SinEtiqueta = true
	declarada := c.MinSize()
	arribaCaja := m + (declarada.Height-LadoEleccion)/2

	// Sin foco: el guardián, invertido. Fuera del cuadradito no hay nada pintado.
	img := colocado(t, c, declarada, m)
	sinFoco, _, _, _ := tintaEnBanda(img, arribaCaja+LadoEleccion/2, arribaCaja+LadoEleccion/2+1, blanco())
	casi(t, "sin foco, la tinta empieza en el borde del cuadradito", sinFoco, m)

	c.FocusGained()
	if enFoco := c.MinSize(); enFoco != declarada {
		t.Errorf("con el foco puesto mide %v y sin él %v: el aro está ocupando sitio en el layout",
			enFoco, declarada)
	}
	img = colocado(t, c, declarada, m)
	conFoco, _, _, _ := tintaEnBanda(img, arribaCaja+LadoEleccion/2, arribaCaja+LadoEleccion/2+1, blanco())
	casi(t, "con foco, la tinta empieza dos píxeles antes", conFoco, m-float32(grosorAro))

	// Y es el color del token, no un azul cualquiera.
	got := px(img, int((m-1.5)*escala), int((arribaCaja+LadoEleccion/2)*escala))
	if quiero := tema.Color("focus-ring", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el aro pinta %v y el token focus-ring es %v", got, quiero)
	}
}

// --------------------------------------------------------------------------------
// 3. La entrada de texto
// --------------------------------------------------------------------------------

// TestLaEntradaMideLoQueLaWeb ata las cotas del campo `md` a la hoja:
//
//   - `h-9` = 2,25rem × 16 = **36** de alto, borde incluido (`box-sizing: border-box`).
//   - `px-3` = 0,75rem × 16 = 12 de relleno, que empieza DESPUÉS del borde: el texto
//     arranca en **13** desde el borde exterior.
func TestLaEntradaMideLoQueLaWeb(t *testing.T) {
	const m, altoWeb, textoEnX = 10, 36, 1 + 12

	e := NuevaEntrada("")
	e.Ancho = 280
	e.PonerTexto("Hxg")
	declarada := MedidaEntrada(e.Talla, e.Ancho, false)
	casi(t, "el alto declarado", declarada.Height, altoWeb)
	if got := e.MinSize(); got != declarada {
		t.Errorf("MinSize() = %v y MedidaEntrada dice %v: hay dos cuentas donde tiene que haber una",
			got, declarada)
	}

	img := colocado(t, e, declarada, m)
	// La caja pintada. Se busca el BORDE, que es lo que la delimita: el relleno es la
	// superficie, del mismo blanco que el lienzo de la captura.
	x0, x1, y0, y1 := caja(img, tema.Color("border-strong", tema.Claro))
	mismaCaja(t, "la entrada", declarada, x0, x1, y0, y1)

	// Y la cota que de verdad importa: dónde empieza a escribirse.
	//
	// Se lee del LAYOUT y no de la tinta, y la diferencia importa: la tinta de una «H»
	// empieza un píxel después del origen del texto —es el avance lateral del glifo, no
	// una desviación del control—, así que medirla contra 13 compararía dos cosas
	// distintas. El píxel se usa para comprobar que lo pintado sigue al layout, que es
	// otra pregunta.
	r, _ := e.CreateRenderer().(*campoRenderer)
	r.Layout(declarada)
	casi(t, "el texto empieza en x", e.env.Position().X, textoEnX)
	casi(t, "y su caja de línea va centrada en la del campo",
		e.env.Position().Y+e.env.Size().Height/2, altoWeb/2)

	tx0, _, _, _ := tintaEnBanda(img, m+altoWeb/2-1, m+altoWeb/2+1,
		blanco(), tema.Color("surface", tema.Claro), tema.Color("border-strong", tema.Claro))
	if d := tx0 - m - textoEnX; d < 0 || d > 2 {
		t.Errorf("la tinta del texto empieza en %v y el origen del texto es %v: lo pintado no sigue al layout",
			tx0-m, float32(textoEnX))
	}
}

// TestLaEntradaSMMideVeinticuatro es la talla del cromo denso: `h-6 min-h-[24px] px-1.5
// text-xs`. Medía 21 (`h-6` con la raíz a 14) hasta el PR #841 de la web.
func TestLaEntradaSMMideVeinticuatro(t *testing.T) {
	const m, altoWeb, textoEnX = 10, 24, 1 + 6

	e := NuevaEntrada("").Con(CampoSM)
	e.Ancho = 200
	e.PonerTexto("Hxg")
	declarada := MedidaEntrada(CampoSM, e.Ancho, false)
	casi(t, "el alto de `min-h-[24px]`", declarada.Height, altoWeb)

	r, _ := e.CreateRenderer().(*campoRenderer)
	r.Layout(declarada)
	casi(t, "el texto empieza en x", e.env.Position().X, textoEnX)

	img := colocado(t, e, declarada, m)
	x0, x1, y0, y1 := caja(img, tema.Color("border-strong", tema.Claro))
	mismaCaja(t, "la entrada sm", declarada, x0, x1, y0, y1)
}

// TestUnCampoCentradoPoneLoEscritoEnMedio es `w-14 px-1.5 text-center`, el número de
// página de CertiPDF: lo escrito deja el mismo hueco a los dos lados, también después de
// teclear, y lo que no cabe empieza donde empieza un campo sin centrar. Aquí sí se mide
// contra el texto, porque centrar ES repartir lo que el texto deja libre.
func TestUnCampoCentradoPoneLoEscritoEnMedio(t *testing.T) {
	p := CampoPropio{MargenX: 0.375 * tema.Rem, MargenY: 0.25 * tema.Rem, Texto: tema.TextoXS, Centrado: true}
	e := NuevaEntrada("").ConPropia(p)
	e.Ancho = 3.5 * tema.Rem
	tam := fyne.NewSize(e.Ancho, p.Alto())
	casi(t, "el alto de `py-1 text-xs` con borde", tam.Height, 26)
	e.Resize(tam)
	borde := tema.Borde + p.MargenX
	mira := func(que, s string) {
		t.Helper()
		izq := e.env.Position().X - borde
		der := tam.Width - borde - e.env.Position().X - Medir(s, tema.TextoXS, Normal).Width
		casi(t, que+": el hueco de la izquierda contra el de la derecha", izq, der)
	}
	e.PonerTexto("7")
	mira("con «7»", "7")
	test.Type(e.editor, "4")
	mira("tras teclear un 4", "74")
	e.PonerTexto("1234567890123")
	casi(t, "lo que no cabe empieza en el relleno", e.env.Position().X, borde)
}

// TestLaEntradaConIconoLoPoneEnLeft3 ata la geometría del `leadingIcon`: el icono va en
// `left-3` —medido desde la caja de BORDE, porque en la web es un `absolute` dentro del
// envoltorio y no relleno— y el texto se aparta hasta `pl-9`.
func TestLaEntradaConIconoLoPoneEnLeft3(t *testing.T) {
	const m, altoWeb, iconoEnX, textoEnX = 10, 36, 12, 1 + 36

	e := NuevaEntrada("")
	e.Icono = iconos.Calendario
	e.Ancho = 280
	e.PonerTexto("Hxg")
	declarada := e.MinSize()
	r, _ := e.CreateRenderer().(*campoRenderer)
	r.Layout(declarada)
	g := r.glifo.Objeto()
	casi(t, "el icono empieza en x", g.Position().X, iconoEnX)
	casi(t, "el icono mide", g.Size().Width, 16)
	casi(t, "y va centrado en la caja", g.Position().Y+g.Size().Height/2, altoWeb/2)
	casi(t, "el texto se aparta hasta `pl-9`", e.env.Position().X, textoEnX)

	// Y en píxeles: por el centro del campo hay tinta del icono antes de `pl-9`.
	img := colocado(t, e, declarada, m)
	tx0, _, _, _ := tintaEnBanda(img, m+altoWeb/2-1, m+altoWeb/2+1,
		blanco(), tema.Color("surface", tema.Claro), tema.Color("border-strong", tema.Claro))
	if tx0-m > textoEnX {
		t.Errorf("no se ha pintado nada del icono: la primera tinta está en %v, ya dentro del texto", tx0-m)
	}
}

// TestElEditorDeFynePintaDondeElNuestro es LA MEDICIÓN que decide envolver
// `widget.Entry` en vez de escribir un editor propio.
//
// Con el cromo apagado por tema —borde, fondo, rellenos y espaciado a cero—, el editor
// de Fyne pone su texto en el MISMO píxel que un `canvas.Text` de este paquete, y su
// `MinSize` es exactamente la caja de línea de la fuente. Si algún día deja de ser
// cierto, esto se pone en rojo: entonces hay que reevaluar la decisión y el comentario
// de `campo.go` que la explica, no ajustar un desplazamiento a mano.
func TestElEditorDeFynePintaDondeElNuestro(t *testing.T) {
	const s = "Hxg"
	ref := Texto(s, tema.TextoRaiz, Normal, color.NRGBA{A: 255})
	ref.Move(fyne.NewPos(10, 10))
	ref.Resize(ref.MinSize())
	rx0, _, ry0, _ := tinta(pinta(t, 300, 60, ref), blanco())

	ed := nuevoEditor(func(bool) {})
	ed.Scroll = fyne.ScrollNone
	ed.Wrapping = fyne.TextWrapOff
	ed.SetText(s)
	env := envolver(ed, &temaEditor{escalon: tema.TextoRaiz})
	env.Move(fyne.NewPos(10, 10))
	env.Resize(fyne.NewSize(280, ref.MinSize().Height))
	ex0, _, ey0, _ := tinta(pinta(t, 300, 60, env), blanco())

	casi(t, "la x del texto del editor de Fyne", ex0, rx0)
	casi(t, "la y del texto del editor de Fyne", ey0, ry0)
	if m := ed.MinSize().Height; m != ref.MinSize().Height {
		t.Errorf("el editor mide %v de alto y la caja de la fuente es %v: ya no se le puede imponer la caja de CSS",
			m, ref.MinSize().Height)
	}
}

// --------------------------------------------------------------------------------
// 4. El área de texto
// --------------------------------------------------------------------------------

// TestElAreaTextoMideCientoSeis es la cuenta que se equivoca todo el mundo.
//
// El alto de un `<textarea rows=4>` son cuatro CAJAS DE LÍNEA —no cuatro alturas de
// letra—, más `py-2` arriba y abajo y el borde: 4 × 22 + 16 + 2 = **106**.
// Usar la caja de la fuente del toolkit deja el área más baja.
func TestElAreaTextoMideCientoSeis(t *testing.T) {
	const m, altoWeb = 10, 4*22 + 2*8 + 2

	a := NuevaAreaTexto("")
	a.Ancho = 320
	declarada := a.MinSize()
	casi(t, "el alto de cuatro filas", declarada.Height, altoWeb)
	casi(t, "y en números redondos", declarada.Height, 106)
	if got := MedidaAreaTexto(a.Filas, a.Ancho); got != declarada {
		t.Errorf("MinSize() = %v y MedidaAreaTexto dice %v", declarada, got)
	}

	a.PonerTexto("Hxg")
	img := colocado(t, a, declarada, m)
	x0, x1, y0, y1 := caja(img, tema.Color("border-strong", tema.Claro))
	mismaCaja(t, "el área de texto", declarada, x0, x1, y0, y1)

	// La primera línea se apoya ARRIBA con `py-2`, no en el centro de las cuatro: su
	// tinta cae dentro de la primera caja de línea.
	_, _, ty0, ty1 := tintaEnBanda(img, m+tema.RadioMD, m+altoWeb-tema.RadioMD,
		blanco(), tema.Color("surface", tema.Claro), tema.Color("border-strong", tema.Claro))
	arriba := float32(tema.Borde + tema.Espacio(2))
	abajo := arriba + tema.TextoBase.Linea
	if ty0-m < arriba || ty1-m > abajo {
		t.Errorf("la primera línea ocupa de %v a %v y su caja de línea va de %v a %v",
			ty0-m, ty1-m, arriba, abajo)
	}
}

// TestElInterlineadoDelAreaEsElDeCSS comprueba la calibración que hace que la CUARTA
// línea caiga donde la pone el navegador. Fyne apila las líneas a la caja de la fuente;
// CSS las apila a 22. Sin corregirlo, el error se acumula en cada línea y el texto se
// despega del fondo del área.
func TestElInterlineadoDelAreaEsElDeCSS(t *testing.T) {
	fuente := altoFuente(tema.TextoBase)
	if fuente >= tema.TextoBase.Linea {
		t.Fatalf("la caja de la fuente (%v) no es menor que la de CSS (%v): la corrección sobra o va al revés",
			fuente, tema.TextoBase.Linea)
	}
	casi(t, "el interlineado que se le impone a Fyne",
		temaEditor{escalon: tema.TextoBase}.Size("lineSpacing"), tema.TextoBase.Linea-fuente)
	casi(t, "y con él, cuatro líneas de CSS", 4*tema.TextoBase.Linea, 88)
}

// --------------------------------------------------------------------------------
// 5. El selector
// --------------------------------------------------------------------------------

// TestElSelectorMideLoQueLaWeb: la misma caja que un campo (`h-9`), el rótulo en `pl-3`
// y el chevron de 16 con su borde derecho a `right-3` del borde de la caja.
func TestElSelectorMideLoQueLaWeb(t *testing.T) {
	const m, altoWeb, rotuloEnX, chevronDerecha = 10, 36, 1 + 12, 12

	s := NuevoSelector([]string{"España", "Portugal"}, nil)
	s.Ancho = 240
	declarada := s.MinSize()
	casi(t, "el alto", declarada.Height, altoWeb)

	img := colocado(t, s, declarada, m)
	x0, x1, y0, y1 := caja(img, tema.Color("border-strong", tema.Claro))
	mismaCaja(t, "el selector", declarada, x0, x1, y0, y1)

	rs, _ := s.CreateRenderer().(*selectorRenderer)
	rs.Layout(declarada)
	casi(t, "el rótulo empieza en x", rs.rotulo.Position().X, rotuloEnX)
	ch := rs.chevron.Objeto()
	casi(t, "el chevron mide", ch.Size().Width, 16)
	casi(t, "y su borde derecho queda a `right-3`",
		declarada.Width-(ch.Position().X+ch.Size().Width), chevronDerecha)
	casi(t, "el chevron va centrado en la caja",
		ch.Position().Y+ch.Size().Height/2, altoWeb/2)

	// Y en píxeles, con la geometría del propio glifo.
	//
	// `right-3` coloca la CAJA de 16 del SVG, no su tinta: el trazo de `chevronDown` va
	// de x=5 a x=19 en la rejilla de 24, así que con el remate redondo la tinta acaba
	// 2,67 px antes del borde derecho de esa caja —(24-19-1)/24 × 16—. El navegador
	// rasteriza el MISMO SVG, así que el hueco también está ahí. Lo que se comprueba es
	// que la tinta cae DENTRO de la caja del chevron y pegada a su borde derecho.
	const sangriaGlifo = (24 - 19 - 1) / 24.0 * 16
	// Se mide toda la altura de la caja del glifo: el extremo de su brazo está
	// en y=8,5 de la rejilla, fuera de una franja de un píxel alrededor de y=12.
	_, tx1, _, _ := tintaEnBanda(img, m+ch.Position().Y, m+ch.Position().Y+ch.Size().Height,
		blanco(), tema.Color("surface", tema.Claro), tema.Color("border-strong", tema.Claro))
	hueco := (m + declarada.Width) - tx1 - chevronDerecha
	if hueco < 0 {
		t.Errorf("la tinta del chevron se sale de su caja por la derecha (%v px)", -hueco)
	}
	if d := hueco - sangriaGlifo; d > 1 || d < -1 {
		t.Errorf("la tinta del chevron acaba a %v de su caja y la ruta de la rejilla de 24 deja %v",
			hueco, float32(sangriaGlifo))
	}
}

// TestElSelectorSMEncogeElChevron: `pr-5`, chevron de 12 y `right-1`.
func TestElSelectorSMEncogeElChevron(t *testing.T) {
	s := NuevoSelector([]string{"A"}, nil).Con(CampoSM)
	s.Ancho = 160
	casi(t, "el alto de `h-6 min-h-[24px]`", s.MinSize().Height, 24)
	casi(t, "el relleno derecho `pr-5`", medidasSelector[CampoSM].derecha, 20)
	casi(t, "el chevron de la talla sm", medidasSelector[CampoSM].chevron, 12)
	casi(t, "y su `right-1`", medidasSelector[CampoSM].chevronX, 4)
}

// TestElSelectorSeManejaConElTeclado: un `<select>` cambia de valor con las flechas sin
// abrir la lista, y eso es lo que tiene quien no usa ratón.
func TestElSelectorSeManejaConElTeclado(t *testing.T) {
	var elegidas []int
	s := NuevoSelector([]string{"Uno", "Dos", "Tres"}, func(i int) { elegidas = append(elegidas, i) })
	for _, k := range []fyne.KeyName{fyne.KeyDown, fyne.KeyDown, fyne.KeyUp, fyne.KeyEnd} {
		s.TypedKey(&fyne.KeyEvent{Name: k})
	}
	if s.Valor() != "Tres" {
		t.Errorf("tras bajar dos, subir una e ir al final, el valor es %q", s.Valor())
	}
	if len(elegidas) != 4 {
		t.Errorf("AlElegir se ha llamado %d veces y ha habido 4 cambios: %v", len(elegidas), elegidas)
	}
	// Y no se sale por los extremos: en la última, bajar no hace nada.
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if s.Elegida != 2 {
		t.Errorf("se ha salido de la lista: Elegida = %d con 3 opciones", s.Elegida)
	}
}

// --------------------------------------------------------------------------------
// 6. Casilla y radio
// --------------------------------------------------------------------------------

// TestLaCasillaMide16EnUnaFilaDe22 ata las dos cotas que se confunden: el cuadradito
// es `h-4 w-4` = **16**, pero la FILA mide la caja de línea de `text-base` = **22**,
// porque el `<label>` es un `inline-flex items-center text-base`. El hueco hasta la
// etiqueta es `gap-2` = **8**.
func TestLaCasillaMide16EnUnaFilaDe22(t *testing.T) {
	const lado, fila, hueco = 16, 22, 8

	c := NuevaCasilla("Recordarme", nil)
	declarada := c.MinSize()
	casi(t, "el alto de la fila", declarada.Height, fila)
	casi(t, "el lado del cuadradito", LadoEleccion, lado)
	casi(t, "el hueco hasta la etiqueta", HuecoEleccion, hueco)
	casi(t, "el ancho, que es cuadradito + hueco + rótulo",
		declarada.Width, lado+hueco+Medir("Recordarme", tema.TextoBase, Normal).Width)

	// Sin marcar: el contorno de Chromium (el `border-border-strong` de Form.tsx no toca un
	// control con `appearance: auto`) sobre su fondo.
	solo := NuevaCasilla("", nil)
	solo.SinEtiqueta = true
	img := colocado(t, solo, solo.MinSize(), 6)
	x0, x1, y0, y1 := caja(img, CasillaNativa(tema.Claro).Borde)
	mismaCaja(t, "el cuadradito", fyne.NewSize(lado, lado), x0, x1, y0, y1)

	// Marcada: cuadrado sólido del acento, del mismo lado.
	solo.Marcada = true
	solo.Refresh()
	img = colocado(t, solo, solo.MinSize(), 6)
	x0, x1, y0, y1 = caja(img, tema.Color("primary", tema.Claro))
	mismaCaja(t, "el cuadradito marcado", fyne.NewSize(lado, lado), x0, x1, y0, y1)
}

// TestElRadioNoSeDesmarcaAlPulsarlo es la diferencia de comportamiento con la casilla:
// `<input type="radio">` no se apaga solo, para eso está el grupo.
func TestElRadioNoSeDesmarcaAlPulsarlo(t *testing.T) {
	cambios := 0
	r := NuevoRadio("Mensual", func(bool) { cambios++ })
	r.Tapped(nil)
	if !r.Marcada || cambios != 1 {
		t.Fatalf("el radio no se ha marcado: marcada=%v cambios=%d", r.Marcada, cambios)
	}
	r.Tapped(nil)
	if !r.Marcada || cambios != 1 {
		t.Errorf("el radio se ha desmarcado al volver a pulsarlo: marcada=%v cambios=%d", r.Marcada, cambios)
	}
	// Y una casilla SÍ se desmarca: el mismo guardián, invertido.
	c := NuevaCasilla("Recordarme", nil)
	c.Tapped(nil)
	c.Tapped(nil)
	if c.Marcada {
		t.Error("una casilla no se ha desmarcado al volver a pulsarla")
	}
}

// --------------------------------------------------------------------------------
// 7. El conmutador
// --------------------------------------------------------------------------------

// TestElConmutadorMideLoQueLaWeb: carril `h-5 w-9` = **36 × 20**, pomo `h-4 w-4` =
// **16** a `top-0.5 left-0.5` = 2, y recorrido `translate-x-4` = **16**.
//
// La cuenta cierra: 2 + 16 + 16 + 2 = 36. Si alguno de los cuatro se toca por su
// cuenta, el pomo encendido se sale del carril o le sobra hueco.
func TestElConmutadorMideLoQueLaWeb(t *testing.T) {
	const ancho, alto, pomo, margen, recorrido = 36, 20, 16, 2, 16

	casi(t, "el ancho del carril", AnchoCarril, ancho)
	casi(t, "el alto del carril", AltoCarril, alto)
	casi(t, "el lado del pomo", LadoPomo, pomo)
	casi(t, "el margen del pomo", MargenPomo, margen)
	casi(t, "el recorrido", RecorridoPomo, recorrido)
	casi(t, "el hueco `gap-2.5` hasta la etiqueta", HuecoConmutador, 10)
	casi(t, "margen + pomo + recorrido + margen tiene que dar el carril",
		margen+pomo+recorrido+margen, ancho)
	casi(t, "el alto de la fila", NuevoConmutador("Notificaciones", nil).MinSize().Height, 22)

	// Con `class="text-sm"` —la autorespuesta de Mail— la etiqueta y la fila encogen.
	sm := NuevoConmutador("Activada", nil)
	sm.Escalon = tema.TextoSM
	casi(t, "el ancho con la etiqueta en text-sm", sm.MinSize().Width,
		ancho+HuecoConmutador+Medir("Activada", tema.TextoSM, Normal).Width)
	casi(t, "el alto con la etiqueta en text-sm", sm.MinSize().Height, max(alto, tema.TextoSM.Linea))

	// Apagado: el contraste lo pone un contorno de un píxel POR FUERA (`ring-1`), que es
	// un `box-shadow` y no ocupa sitio. El carril sigue midiendo 36 × 20.
	c := NuevoConmutador("", nil)
	c.Resize(c.MinSize())
	r, _ := c.CreateRenderer().(*conmutadorRenderer)
	r.Layout(c.MinSize())
	casi(t, "el pomo apagado empieza en x", r.pomo.Position().X, margen)

	// En píxeles se mide con la tolerancia de `mismaCaja`: las esquinas de una píldora
	// SÍ llevan antialias, así que la columna del extremo no da el color exacto.
	img := colocado(t, c, c.MinSize(), 6)
	x0, x1, y0, y1 := caja(img, tema.Color("neutral-500", tema.Claro))
	mismaCaja(t, "el contorno del carril apagado",
		fyne.NewSize(ancho+2*tema.Borde, alto+2*tema.Borde), x0, x1, y0, y1)

	// Encendido: el carril se tiñe del acento y el pomo se va al otro extremo.
	c.Encendido = true
	c.Refresh()
	r.Refresh()
	casi(t, "el pomo encendido empieza en x", r.pomo.Position().X, margen+recorrido)
	img = colocado(t, c, c.MinSize(), 6)
	x0, x1, y0, y1 = caja(img, tema.Color("primary", tema.Claro))
	mismaCaja(t, "el carril encendido", fyne.NewSize(ancho, alto), x0, x1, y0, y1)
}

// --------------------------------------------------------------------------------
// 8. El campo de fecha
// --------------------------------------------------------------------------------

// TestElCampoDeFechaEsUnaEntradaConSuIcono: la caja es la misma, para que un formulario
// con fechas y textos no tenga dos alturas.
func TestElCampoDeFechaEsUnaEntradaConSuIcono(t *testing.T) {
	f := CampoFecha(CampoMD)
	casi(t, "el alto del campo de fecha", f.MinSize().Height, 36)
	if f.Icono != iconos.Calendario {
		t.Errorf("el campo de fecha lleva el icono %v", f.Icono)
	}
	casi(t, "el alto del campo de hora sm", CampoHora(CampoSM).MinSize().Height, 24)
}

// TestLeerFechaAceptaLoQueLaGenteTeclea recorre los separadores de los 26 idiomas de la
// suite y las dos formas de escribir una fecha.
func TestLeerFechaAceptaLoQueLaGenteTeclea(t *testing.T) {
	zona := time.UTC
	quiero := time.Date(2026, 10, 5, 0, 0, 0, 0, zona)
	for _, s := range []string{
		"05/10/2026", "5/10/2026", "05.10.2026", "05-10-2026", "05 10 2026", "05102026", "2026-10-05",
	} {
		got, vacia, ok := LeerFecha(s, zona)
		if !ok || vacia || !got.Equal(quiero) {
			t.Errorf("LeerFecha(%q) = %v, vacia=%v, ok=%v; quiero %v", s, got, vacia, ok, quiero)
		}
	}
	if _, vacia, ok := LeerFecha("   ", zona); !vacia || !ok {
		t.Error("un campo en blanco no es un error: es un campo en blanco")
	}
	// Y lo que NO vale. El 31 de febrero es el caso que se cuela: `time.Date` lo
	// normalizaría al 3 de marzo y el formulario guardaría una fecha que nadie escribió.
	for _, s := range []string{"31/02/2026", "05/13/2026", "ayer", "5/10/85", "32/10/2026"} {
		if _, _, ok := LeerFecha(s, zona); ok {
			t.Errorf("LeerFecha(%q) lo ha dado por bueno", s)
		}
	}
}

// TestLeerHoraAceptaElTecladoNumerico: «930» es lo que sale de teclear en un móvil.
func TestLeerHoraAceptaElTecladoNumerico(t *testing.T) {
	for _, c := range []struct {
		s    string
		h, m int
		vale bool
	}{
		{"09:30", 9, 30, true},
		{"9:30", 9, 30, true},
		{"09.30", 9, 30, true},
		{"930", 9, 30, true},
		{"0930", 9, 30, true},
		{"9", 9, 0, true},
		{"23:59", 23, 59, true},
		{"24:00", 0, 0, false},
		{"09:60", 0, 0, false},
		{"tarde", 0, 0, false},
	} {
		h, m, _, ok := LeerHora(c.s)
		if ok != c.vale || (ok && (h != c.h || m != c.m)) {
			t.Errorf("LeerHora(%q) = %d:%02d, ok=%v; quiero %d:%02d, ok=%v", c.s, h, m, ok, c.h, c.m, c.vale)
		}
	}
}

// --------------------------------------------------------------------------------
// 9. Lo transversal, y los guardianes vistos en rojo
// --------------------------------------------------------------------------------

// TestLoInerteSePintaAMitadDeOpacidadYNoResponde recorre los controles con estado
// propio. `disabled:opacity-50` está en `controlBase` y en el `<label>` de la casilla y
// el conmutador: es la misma regla para todos.
func TestLoInerteSePintaAMitadDeOpacidadYNoResponde(t *testing.T) {
	c := NuevaCasilla("Recordarme", nil)
	c.Disable()
	c.Tapped(nil)
	if c.Marcada {
		t.Error("una casilla inerte se ha marcado al pulsarla")
	}
	sw := NuevoConmutador("Avisos", nil)
	sw.Disable()
	sw.Tapped(nil)
	if sw.Encendido {
		t.Error("un conmutador inerte se ha encendido al pulsarlo")
	}
	s := NuevoSelector([]string{"A", "B"}, nil)
	s.Inerte(true)
	s.Tapped(nil)
	if s.abierto != nil {
		t.Error("un selector inerte ha abierto su lista")
	}

	// Y el color: una casilla marcada e inerte NO es el acento atenuado, es el gris de
	// Chromium con la etiqueta al 50 %. Medido en Chromium a ×2 con paleta final (2026-10-04): #e8e8e8.
	solo := NuevaCasilla("", nil)
	solo.SinEtiqueta = true
	solo.Marcada = true
	solo.Disable()
	img := colocado(t, solo, solo.MinSize(), 6)
	quiero := color.NRGBA{R: 0xe8, G: 0xe8, B: 0xe8, A: 255}
	centro := int((6 + LadoEleccion/2) * escala)
	if got := px(img, centro, centro-8); !cerca(got, quiero) {
		t.Errorf("la casilla inerte pinta %v y Chromium, con la etiqueta al 50 %%, %v", got, quiero)
	}
}

// TestElGuardianDeLaCajaSeVeEnRojo invierte a mano lo que vigila `mismaCaja`: se le da
// de comer una medida declarada que NO es la pintada y tiene que protestar.
//
// Un guardián que no se ha visto en rojo no protege nada, y este sostiene todas las
// comprobaciones de píxel de este fichero.
func TestElGuardianDeLaCajaSeVeEnRojo(t *testing.T) {
	falso := &testing.T{}
	c := NuevaCaja(tema.Color("surface", tema.Claro), tema.Color("border", tema.Claro), tema.RadioMD)
	c.Redimensionar(fyne.NewSize(40, 20))
	img := pinta(t, 60, 40, c.Objetos()...)
	x0, x1, y0, y1 := caja(img, tema.Color("border", tema.Claro))
	mismaCaja(falso, "una caja mentirosa", fyne.NewSize(60, 20), x0, x1, y0, y1)
	if !falso.Failed() {
		t.Fatal("mismaCaja ha dado por buena una caja 20 px más ancha de lo que pinta: el guardián no vale")
	}
}

// TestLasCotasNoDependenDelTexto es el guardián de la trampa que describe la cabecera.
//
// Si el conformador inflara el texto un 6 %, ninguna de las cotas atadas arriba se
// movería. Se demuestra midiendo el mismo control con dos rótulos de longitudes muy
// distintas: el alto tiene que salir idéntico, y el ancho de una caja fija también.
func TestLasCotasNoDependenDelTexto(t *testing.T) {
	corto := NuevaCasilla("Sí", nil)
	largo := NuevaCasilla("Recordar esta sesión en este dispositivo durante treinta días", nil)
	if corto.MinSize().Height != largo.MinSize().Height {
		t.Errorf("la fila mide %v con un rótulo corto y %v con uno largo: el alto depende del texto",
			corto.MinSize().Height, largo.MinSize().Height)
	}
	// Y el ancho SÍ cambia, que es lo correcto: si no, es que el rótulo no se mide y el
	// control saldría recortado. El guardián anterior, invertido.
	if corto.MinSize().Width >= largo.MinSize().Width {
		t.Errorf("los dos miden %v de ancho: el rótulo no se está midiendo", corto.MinSize().Width)
	}

	e1, e2 := NuevaEntrada("a"), NuevaEntrada(strings.Repeat("marca de agua muy larga ", 8))
	e1.Ancho, e2.Ancho = 280, 280
	if e1.MinSize() != e2.MinSize() {
		t.Errorf("un campo de ancho fijo mide %v con una marca corta y %v con una larga:"+
			" la marca de agua está empujando la caja", e1.MinSize(), e2.MinSize())
	}
}

// TestLosControlesSeAnuncian comprueba lo que la cáscara de widget compra frente a un
// árbol de primitivas, que se anunciaría como «Container».
func TestLosControlesSeAnuncian(t *testing.T) {
	for _, c := range []struct {
		que string
		a   fyne.Accessible
		rot string
	}{
		{"la casilla", NuevaCasilla("Recordarme", nil), "Recordarme"},
		{"el radio", NuevoRadio("Mensual", nil), "Mensual"},
		{"el conmutador", NuevoConmutador("Avisos", nil), "Avisos"},
		{"el botón", NuevoBoton("Guardar", nil), "Guardar"},
		// El `aria-label` manda sobre lo escrito, como en el navegador.
		{"el botón con aria-label", func() *Boton { b := NuevoBoton("Copiar", nil); b.Rotulo = "Copiar para Slack"; return b }(), "Copiar para Slack"},
		{"el selector", NuevoSelector([]string{"#general"}, nil), "#general"},
		{"el selector con aria-label", func() *Selector {
			s := NuevoSelector([]string{"#general"}, nil)
			s.Rotulo = "Canal para el bot"
			return s
		}(), "Canal para el bot"},
	} {
		if c.a.AccessibilityLabel() != c.rot {
			t.Errorf("%s se anuncia como %q y debería ser %q", c.que, c.a.AccessibilityLabel(), c.rot)
		}
		if c.a.AccessibilityRole() == fyne.AccessibleRoleContainer {
			t.Errorf("%s se anuncia con rol «container»", c.que)
		}
	}
}

// TestElBotonSeEnfocaYSePulsaConElTeclado: un `<button>` responde a Intro y a la barra
// espaciadora, y su aro es el de HUECO porque va sobre relleno: sobre el azul de un
// botón principal, un anillo pegado se funde con el propio relleno y deja de leerse.
func TestElBotonSeEnfocaYSePulsaConElTeclado(t *testing.T) {
	pulsado := 0
	b := NuevoBoton("Guardar", func() { pulsado++ })
	b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	b.TypedRune(' ')
	if pulsado != 2 {
		t.Errorf("el botón se ha pulsado %d veces con Intro y espacio", pulsado)
	}
	b.Inerte = true
	b.TypedRune(' ')
	if pulsado != 2 {
		t.Error("un botón inerte ha respondido al teclado")
	}

	// El aro con hueco se sale 3 px: 1 de superficie + 2 de anillo.
	b2 := NuevoBoton("Guardar", nil)
	b2.FocusGained()
	tam := b2.MinSize()
	img := colocado(t, b2, tam, 8)
	x0, _, _, _ := tintaEnBanda(img, 8+tam.Height/2, 8+tam.Height/2+1, blanco())
	casi(t, "el aro del botón empieza", x0, 8-(ConHueco+float32(grosorAro)))
}

// TestElWidgetRecibeElClic es lo que justifica la cáscara: sin `fyne.Widget`,
// `internal/driver/util.go` no desciende en el árbol y no hay clic que llegue.
func TestElWidgetRecibeElClic(t *testing.T) {
	marcada := false
	c := NuevaCasilla("Recordarme", func(b bool) { marcada = b })
	c.Resize(c.MinSize())
	c.Tapped(&fyne.PointEvent{})
	if !marcada {
		t.Error("la casilla no ha recibido la pulsación")
	}
}

// TestElCromoDeFyneEstaApagado es la regla 1 del paquete comprobada y no prometida: el
// único widget de serie que usa esta familia es `widget.Entry`, y solo como MOTOR de
// edición. Todo lo que Fyne pintaría por su cuenta está a cero.
func TestElCromoDeFyneEstaApagado(t *testing.T) {
	te := temaEditor{escalon: tema.TextoBase}
	for _, n := range []fyne.ThemeSizeName{"inputBorder", "innerPadding", "padding", "inputRadius"} {
		if got := te.Size(n); got != 0 {
			t.Errorf("el tema del editor deja %s en %v y tiene que ser 0, o Fyne pinta su propio cromo", n, got)
		}
	}
	for _, n := range []fyne.ThemeColorName{"inputBorder", "inputBackground", "background"} {
		if c, _ := te.Color(n, 0).(color.NRGBA); c.A != 0 {
			t.Errorf("el tema del editor deja %s opaco (%v): se pintará encima de nuestra caja", n, c)
		}
	}
	// Y el tamaño de letra es el de la talla, que es lo único que no se puede pedir por
	// campo en `widget.Entry`.
	casi(t, "el cuerpo de letra del editor", te.Size("text"), tema.TextoBase.Tam)
	casi(t, "y el de la talla sm", temaEditor{escalon: tema.TextoXS}.Size("text"), tema.TextoXS.Tam)
}

// TestElBotonTieneLasCuatroTallasDeLaWeb recorre la tabla `sizes` de Button.tsx. Los
// altos salen de `h-*` con el rem de 16 y los rellenos de `px-*`; `xs` lleva además el
// `min-h-[24px]` que la hoja le pone encima: `h-6` ya son los 24 del suelo de WCAG 2.5.8.
func TestElBotonTieneLasCuatroTallasDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		clase               string
		talla               Talla
		alto, margen, letra float32
	}{
		{"xs: h-6 min-h-[24px] px-2 text-xs", TallaXS, 24, 8, 12},
		{"sm: h-8 px-3 text-sm", TallaSM, 32, 12, 13},
		{"md: h-9 px-4 text-base", TallaMD, 36, 16, 14},
		{"lg: h-11 px-5 text-md", TallaLG, 44, 20, 16},
	} {
		m := medidas[c.talla]
		casi(t, c.clase+" — alto", m.alto, c.alto)
		casi(t, c.clase+" — relleno", m.margen, c.margen)
		casi(t, c.clase+" — cuerpo de letra", m.texto.Tam, c.letra)
		casi(t, c.clase+" — alto declarado", MedidaBoton("Guardar", c.talla, 0).Height, c.alto)
		// Y lo pintado, a ×2, donde los medios píxeles de esta escala son exactos.
		b := NuevoBoton("Guardar", nil).Con(Primario, c.talla)
		declarada := b.MinSize()
		img := colocado(t, b, declarada, 8)
		x0, x1, y0, y1 := caja(img, tema.Color("primary", tema.Claro))
		mismaCaja(t, "el botón "+c.clase, declarada, x0, x1, y0, y1)
	}
}

// TestElBotonTieneLasCuatroVariantes ata cada variante a los tokens de `variants` en
// Button.tsx. El principal usa el petróleo 600 del tema compartido, que permite texto
// blanco y contrasta por sí solo sobre las superficies claras.
func TestElBotonTieneLasCuatroVariantes(t *testing.T) {
	conPetroleoClaro(t)
	for _, c := range []struct {
		clase                 string
		v                     Variante
		relleno, texto, borde string
	}{
		{"primary: bg-primary text-on-primary", Primario, "primary", "on-primary", ""},
		{"secondary: bg-primary text-on-primary border-border-strong", Secundario, "primary", "on-primary", "border-strong"},
		{"ghost: bg-transparent text-primary-text", Fantasma, "", "primary-text", ""},
		// Blanco, no `on-primary`: es el rótulo del primario y en oscuro es casi negro.
		{"danger: bg-danger text-white", Peligro, "danger", "neutral-0", ""},
	} {
		b := NuevoBoton("Guardar", nil).Con(c.v, TallaMD)
		relleno, texto, borde := b.colores(tema.Claro)
		if c.relleno == "" {
			if relleno.A != 0 {
				t.Errorf("%s: el relleno es %v y tiene que ser transparente", c.clase, relleno)
			}
		} else if quiero := tema.Color(c.relleno, tema.Claro); relleno != quiero {
			t.Errorf("%s: el relleno es %v y el token %s es %v", c.clase, relleno, c.relleno, quiero)
		}
		if quiero := tema.Color(c.texto, tema.Claro); texto != quiero {
			t.Errorf("%s: la tinta es %v y el token %s es %v", c.clase, texto, c.texto, quiero)
		}
		if c.borde == "" {
			if borde.A != 0 {
				t.Errorf("%s: lleva borde (%v) y en la hoja no tiene", c.clase, borde)
			}
		} else {
			quiero := tema.Color(c.borde, tema.Claro)
			if c.v == Secundario {
				// border-strong rgba(15,29,32,.6) sobre primary-600 #0f7680:
				// CSS pinta el borde sobre el relleno del propio botón.
				quiero = color.NRGBA{R: 15, G: 65, B: 70, A: 255}
			}
			if borde != quiero {
				t.Errorf("%s: el borde es %v; CSS requiere %v", c.clase, borde, quiero)
			}
		}
	}
	// El valor absoluto viene de colors.css: `primary` resuelve a `primary-600`.
	if p, quiero := tema.Color("primary", tema.Claro), (color.NRGBA{R: 0x0f, G: 0x76, B: 0x80, A: 255}); p != quiero {
		t.Errorf("`primary` vale %v y el primario 600 de colors.css es %v", p, quiero)
	}
}

// TestLosTokensDeLaFamiliaExistenEnLosDosTemas evita el fallo más tonto y más caro:
// `tokens.Color` entra en pánico con un nombre que no está, así que un token que solo
// exista en claro tira la aplicación entera al ponerla en oscuro. Y en oscuro no la
// mira nadie hasta que alguien la mira.
func TestLosTokensDeLaFamiliaExistenEnLosDosTemas(t *testing.T) {
	usados := []string{
		"surface", "surface-raised", "bg-subtle", "border", "border-strong", "text",
		"text-subtle", "danger", "primary", "on-primary", "focus-ring", "selected", "neutral-500",
	}
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		for _, n := range usados {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("el token %q no existe en el tema %v: %v", n, v, r)
					}
				}()
				_ = tema.Color(n, v)
			}()
		}
	}
	// Y los controles se construyen y se pintan en oscuro sin caerse.
	for _, w := range []fyne.Widget{
		NuevaEntrada("Buscar"), NuevaAreaTexto("Notas"), NuevoSelector([]string{"A"}, nil),
		NuevaCasilla("Sí", nil), NuevoRadio("No", nil), NuevoConmutador("Avisos", nil),
		CampoFecha(CampoMD),
	} {
		w.Resize(w.MinSize())
		w.Refresh()
	}
}

// TestElCampoPropioLlevaSuRelleno: el `<input>` escrito a mano de la ficha del ERP
// (`border-border bg-bg px-3 py-2 text-sm`) mide 36 (18 + 2×8 + 2) y su fondo es
// `bg`; el `<select>` de la misma clase, también. Aunque bg y surface son ambos
// blancos en claro, siguen siendo roles distintos del tema compartido.
//
// En oscuro, bg y surface difieren: se comprueba también la elección del token
// antes de pintar para que la igualdad de los dos blancos no esconda una regresión.
func TestElCampoPropioLlevaSuRelleno(t *testing.T) {
	propio := CampoPropio{MargenX: tema.Espacio(3), MargenY: tema.Espacio(2), Texto: tema.TextoSM, Borde: "border", Relleno: "bg"}
	e := NuevaEntrada("").ConPropia(propio)
	e.Ancho = 200
	casi(t, "el alto de `py-2 text-sm` con borde", e.MinSize().Height, 36)
	s := NuevoSelector([]string{"General (con IVA)"}, nil)
	s.Propio = &SelectorPropio{Campo: propio, Alto: 32}
	s.Ancho = 200
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		rellenoCampo := e.rellenoPropio(tema.Color("surface", v), v)
		rellenoSelector, _, _, _ := s.colores(v)
		if quiero := tema.Color("bg", v); rellenoCampo != quiero || rellenoSelector != quiero {
			t.Errorf("en tema %v el campo/selector rellenan %v/%v y bg es %v", v, rellenoCampo, rellenoSelector, quiero)
		}
	}
	for nombre, w := range map[string]fyne.Widget{"campo": e, "selector": s} {
		img := colocado(t, w, w.MinSize(), 10)
		centro := px(img, int(150*escala), int((10+w.MinSize().Height/2)*escala)) // pasado el texto y antes del chevron
		if !cerca(centro, tema.Color("bg", tema.Claro)) {
			t.Errorf("%s: el relleno pinta %v y el token `bg` es %v", nombre, centro, tema.Color("bg", tema.Claro))
		}
	}
}

// TestLaEntradaSeAnunciaConSuRotulo: el `aria-label` del campo; sin él, la marca de
// agua. VISTO EN ROJO sin `AccessibilityLabel`: el campo no se anunciaba.
func TestLaEntradaSeAnunciaConSuRotulo(t *testing.T) {
	var e fyne.CanvasObject = NuevaEntrada("Buscar eventos")
	a, es := e.(fyne.Accessible)
	if !es || a.AccessibilityLabel() != "Buscar eventos" {
		t.Fatal("el campo no se anuncia con su marca de agua a falta de rótulo")
	}
	e.(*Entrada).Rotulo = "Buscar en la agenda"
	if a.AccessibilityLabel() != "Buscar en la agenda" {
		t.Error("con rótulo, el campo tiene que anunciarse con él")
	}
}

// TestLaEntradaRecortaLoQueNoCabe: un `<input>` recorta el texto largo y lo desplaza con
// el cursor; con `ScrollNone` Fyne lo pintaba fuera de la caja. VISTO EN ROJO con
// `ScrollNone`: tinta a la derecha del campo.
func TestLaEntradaRecortaLoQueNoCabe(t *testing.T) {
	e := NuevaEntrada(strings.Repeat("Marca de agua muy larga ", 8))
	e.Ancho = 160
	tam := e.MinSize()
	e.Resize(tam)
	img := pinta(t, tam.Width+80, tam.Height, e)
	fondo := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for y := range img.Bounds().Dy() {
		for x := int((tam.Width + 2) * escala); x < img.Bounds().Dx(); x++ {
			if !cerca(px(img, x, y), fondo) {
				t.Fatalf("hay tinta en x=%v, fuera del campo de %v: el texto se sale", float32(x)/escala, tam.Width)
			}
		}
	}
}

// La etiqueta de una casilla o un radio va en `text-base` salvo que se pida otro
// escalón: los radios de «Opciones de envío» de Rooms van en `text-sm`, y la fila mide
// esa caja de línea.
func TestLaEleccionLlevaSuEscalon(t *testing.T) {
	e := NuevoRadio("Normal", nil)
	base := e.MinSize()
	e.Escalon = tema.TextoSM
	if m := e.MinSize(); m.Width >= base.Width || m.Height != max(LadoEleccion, tema.TextoSM.Linea) {
		t.Errorf("en `text-sm` mide %v (en `text-base`, %v)", m, base)
	}
	for _, o := range test.WidgetRenderer(e).Objects() {
		if tx, ok := o.(*canvas.Text); ok && tx.Text == "Normal" && tx.TextSize != tema.TextoSM.Tam {
			t.Errorf("la etiqueta se pinta a %v", tx.TextSize)
		}
	}
}

// TestElSelectorRecortaLaOpcionLarga: la opción que no cabe acaba en «…» antes de la
// flecha. VISTO EN ROJO sin el recorte: el texto pisaba el chevrón (Drive, NAS a 360).
func TestElSelectorRecortaLaOpcionLarga(t *testing.T) {
	largo := strings.Repeat("Unidad de red del NAS de la oficina ", 3)
	s := NuevoSelector([]string{largo}, nil)
	s.Ancho = 160
	tam := s.MinSize()
	s.Resize(tam)
	r := test.WidgetRenderer(s).(*selectorRenderer)
	r.Layout(tam)
	c, m := s.medidas()
	cabe := tam.Width - tema.Borde - c.margen - m.chevronX - m.chevron
	if got := Medir(r.rotulo.Text, c.texto, Normal).Width; got > cabe+0.01 || !strings.HasSuffix(r.rotulo.Text, "…") {
		t.Errorf("el texto %q mide %v y caben %v antes de la flecha", r.rotulo.Text, got, cabe)
	}
}

// TestLaFlechaDelSelectNativoEsLaDeChromium: el `<select>` sin estilo no lleva el
// chevrón de trazo de Form.tsx, sino el macizo de Chromium, que mide 9,31 × 6,03, acaba a
// 4,375 del borde derecho y va centrado en alto, con cualquier letra. Medido a 8
// aumentos en Chromium (2026-09-25); aquí también a 8, con medio píxel de tolerancia.
//
// VISTO EN ROJO (2026-09-25) con el chevrón de trazo: 11,25 de ancho y a 3,625 del borde.
func TestLaFlechaDelSelectNativoEsLaDeChromium(t *testing.T) {
	const esc = 8
	for _, alto := range []float32{AltoSelectNativo2XS, AltoSelectNativoXS} {
		s := NuevoSelector([]string{"Arial"}, nil)
		s.Propio = &SelectorPropio{Campo: CampoPropio{MargenX: tema.Espacio(2), Texto: tema.TextoXS}, Alto: alto}
		s.Ancho = 60
		s.Resize(fyne.NewSize(60, alto))
		img := pintaA(t, esc, 60, alto, s)
		tinta := tema.Color("text", tema.Claro)
		x0, x1, y0, y1 := 1<<30, -1, 1<<30, -1
		for y := 0; y < int(alto*esc); y++ {
			for x := int(40 * esc); x < int((60-1.5)*esc); x++ { // la zona de la flecha, sin el borde
				if cerca(px(img, x, y), tinta) {
					x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
				}
			}
		}
		if x1 < 0 {
			t.Fatalf("alto %v: no hay flecha", alto)
		}
		ancho, altoF := float32(x1-x0+1)/esc, float32(y1-y0+1)/esc
		derecha, arriba := 60-float32(x1+1)/esc, float32(y0)/esc
		for _, c := range []struct {
			que         string
			got, quiero float32
		}{
			{"ancho", ancho, 9.31},
			{"alto", altoF, 6.03},
			{"hueco a la derecha", derecha, 4.375},
			{"hueco arriba", arriba, (alto - 6.03) / 2},
		} {
			if c.got < c.quiero-0.5 || c.got > c.quiero+0.5 {
				t.Errorf("alto %v: la flecha tiene %s %v y en Chromium %v", alto, c.que, c.got, c.quiero)
			}
		}
	}
}

// TestUnAtajoQueElCampoNoConoceSubeAlLienzo: con el cursor en un campo, Ctrl+F llega a
// los atajos de la ventana, como el `keydown` que sube al documento; Ctrl+← sigue siendo
// del campo (salta de palabra).
func TestUnAtajoQueElCampoNoConoceSubeAlLienzo(t *testing.T) {
	e := NuevaEntrada("")
	var lienzo fyne.ShortcutHandler
	buscar, izquierda := 0, 0
	lienzo.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) { buscar++ })
	lienzo.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeft, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) { izquierda++ })
	antes := atajosDelLienzo
	atajosDelLienzo = func(fyne.CanvasObject) fyne.Shortcutable { return &lienzo }
	defer func() { atajosDelLienzo = antes }()
	e.editor.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault})
	e.editor.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeft, Modifier: fyne.KeyModifierShortcutDefault})
	if buscar != 1 {
		t.Errorf("Ctrl+F dentro del campo llega %d veces a la ventana", buscar)
	}
	if izquierda != 0 {
		t.Error("Ctrl+← se lo ha quitado la ventana al campo")
	}
}
