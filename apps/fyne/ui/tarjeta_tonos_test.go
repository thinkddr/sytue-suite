// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"regexp"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestElOrdenDeTonosEsElQueDistribuyeElLanzadorWeb(t *testing.T) {
	web := webref.Read(t, "apps/frontend/packages/ui/src/components/Card.tsx")
	bloque := regexp.MustCompile(`(?s)export const CARD_TONES = \[(.*?)\]`).FindStringSubmatch(web)
	if len(bloque) != 2 {
		t.Fatal("Card.tsx ya no declara CARD_TONES; revisar el contrato de distribución")
	}
	valores := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(bloque[1], -1)
	tonos := TonosTarjeta()
	if len(valores) != len(tonos) {
		t.Fatalf("Card.tsx distribuye %d tonos, la interfaz nativa %d", len(valores), len(tonos))
	}
	for i, valor := range valores {
		if tonos[i] != TonoTarjeta(valor[1]) {
			t.Errorf("posición %d: nativa=%q web=%q", i, tonos[i], valor[1])
		}
	}
}

// Los colores literales proceden de branding/tokens/colors.css: los tres bloques
// :root, [data-theme='dark'] y [data-theme='translucent']. Los alfa de rgba se
// expresan en bytes; no se obtienen del resolver que debe proteger esta prueba.
func TestLosNueveTonosDeTarjetaSiguenLosTresTemas(t *testing.T) {
	hex := func(rgb uint32) color.NRGBA {
		return color.NRGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
	}
	rgba := func(r, g, b, a uint8) color.NRGBA {
		return color.NRGBA{R: r, G: g, B: b, A: a}
	}
	// `normal` y `solida` van por apariencia, en el orden claro, oscuro, traslúcido: el
	// oscuro aclara el azul y el violeta sólidos y baja la menta, que es el primario.
	// `normal` es el PASTEL del tono, que la tarjeta ya no pinta —sin `Solida` es la
	// superficie blanca, como Card.tsx—; el token `card-<tono>` sigue en la hoja y lo
	// usan el Hub y el ERP, así que su vector se sigue comprobando.
	superficie := [3]color.NRGBA{hex(0xffffff), hex(0x131c1e), rgba(255, 255, 255, 168)}
	todas := func(rgb uint32) [3]uint32 { return [3]uint32{rgb, rgb, rgb} }
	tonos := []struct {
		tono   TonoTarjeta
		normal [3]color.NRGBA
		solida [3]uint32
		tinta  uint32
	}{
		{TarjetaAzul, [3]color.NRGBA{hex(0xe8efff), rgba(36, 87, 232, 56), rgba(36, 87, 232, 41)}, [3]uint32{0x2457e8, 0x3d6cf0, 0x2457e8}, 0xffffff},
		{TarjetaVioleta, [3]color.NRGBA{hex(0xeeebf8), rgba(101, 70, 197, 64), rgba(101, 70, 197, 43)}, [3]uint32{0x6546c5, 0x7559d6, 0x6546c5}, 0xffffff},
		{TarjetaMenta, [3]color.NRGBA{hex(0xe5f0ea), rgba(37, 118, 91, 56), rgba(37, 118, 91, 43)}, [3]uint32{0x0f7680, 0x0f7680, 0x0f7680}, 0xffffff},
		{TarjetaRosa, [3]color.NRGBA{hex(0xf6e5ed), rgba(182, 64, 104, 56), rgba(182, 64, 104, 41)}, todas(0xb64068), 0xffffff},
		{TarjetaAmbar, [3]color.NRGBA{hex(0xf5efca), rgba(234, 214, 65, 31), rgba(234, 214, 65, 66)}, todas(0xf2c230), 0x10141a},
		{TarjetaCian, [3]color.NRGBA{hex(0xdff0f3), rgba(21, 122, 146, 56), rgba(21, 122, 146, 43)}, todas(0x157a92), 0xffffff},
		{TarjetaCoral, [3]color.NRGBA{hex(0xf5e1dc), rgba(206, 105, 105, 51), rgba(206, 105, 105, 51)}, todas(0xce6969), 0x10141a},
		{TarjetaLima, [3]color.NRGBA{hex(0xedf1d6), rgba(175, 201, 77, 36), rgba(175, 201, 77, 66)}, todas(0xafc94d), 0x10141a},
		{TarjetaPizarra, [3]color.NRGBA{hex(0xe5e9ef), rgba(131, 162, 219, 33), rgba(32, 40, 50, 31)}, todas(0x202832), 0xffffff},
	}
	apariencias := []struct {
		nombre   string
		variante tema.Variante
		tinta    uint32
	}{
		{"claro", tema.Claro, 0x0f1d20},
		{"oscuro", tema.Oscuro, 0xe7efef},
		{"traslucido", tema.Translucido, 0x0f1d20},
	}
	t.Cleanup(func() { tema.Elegir("") })
	for i, apariencia := range apariencias {
		t.Run(apariencia.nombre, func(t *testing.T) {
			tema.Elegir(apariencia.nombre)
			cuerpo := canvas.NewRectangle(hex(0xff00ff))
			cuerpo.SetMinSize(fyne.NewSize(120, 40))
			tarjeta := NuevaTarjeta("Resumen", cuerpo)
			tarjeta.Plana = true
			r := test.WidgetRenderer(tarjeta).(*tarjetaRenderer)
			tarjeta.Resize(fyne.NewSize(154, 125))
			for _, caso := range tonos {
				for _, solida := range []bool{false, true} {
					tarjeta.Tono, tarjeta.Solida = caso.tono, solida
					tarjeta.Refresh()
					fondo, tinta := superficie[i], hex(apariencia.tinta)
					if got := tema.Color("card-"+string(caso.tono), apariencia.variante); got != caso.normal[i] {
						t.Errorf("%s: el token card-%s = %v, CSS = %v", apariencia.nombre, caso.tono, got, caso.normal[i])
					}
					if solida {
						fondo, tinta = hex(caso.solida[i]), hex(caso.tinta)
					}
					if got := r.caja.dentro.FillColor; got != color.Color(fondo) {
						t.Errorf("%s solid=%v: fondo = %v, CSS = %v", caso.tono, solida, got, fondo)
					}
					if got := r.titulo.Color; got != color.Color(tinta) {
						t.Errorf("%s solid=%v: tinta del título = %v, CSS = %v", caso.tono, solida, got, tinta)
					}
					if got := tarjeta.ColorTexto(apariencia.variante); got != tinta {
						t.Errorf("%s solid=%v: tinta disponible para el cuerpo = %v, CSS = %v", caso.tono, solida, got, tinta)
					}
					if got := tarjeta.MinSize(); got != fyne.NewSize(154, 125) {
						t.Errorf("%s solid=%v: tamaño = %v, px-4, py-4 y cabecera web requieren 154×125", caso.tono, solida, got)
					}
					if got := cuerpo.Position(); got != fyne.NewPos(17, 68) {
						t.Errorf("%s solid=%v: cuerpo empieza en %v, debe conservar (17,68)", caso.tono, solida, got)
					}
					if got := cuerpo.FillColor; got != color.Color(hex(0xff00ff)) {
						t.Errorf("%s solid=%v: cambió el color explícito del cuerpo: %v", caso.tono, solida, got)
					}
				}
			}
		})
	}
}

// El valor cero de Go es tone='blue' y fill='tinted', como Card.tsx, y `tinted` es la
// superficie blanca: el tono solo pinta con `Solida`. Neutral sigue la superficie de la
// apariencia; solid neutral usa slate, como solidClass.neutral.
func TestElTonoCeroYNeutralDeLaTarjeta(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, caso := range []struct {
		nombre     string
		variante   tema.Variante
		superficie color.NRGBA
		tinta      color.NRGBA
	}{
		{"claro", tema.Claro, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{R: 15, G: 29, B: 32, A: 255}},
		{"oscuro", tema.Oscuro, color.NRGBA{R: 19, G: 28, B: 30, A: 255}, color.NRGBA{R: 231, G: 239, B: 239, A: 255}},
		{"traslucido", tema.Translucido, color.NRGBA{R: 255, G: 255, B: 255, A: 168}, color.NRGBA{R: 15, G: 29, B: 32, A: 255}},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			tema.Elegir(caso.nombre)
			tarjeta := NuevaTarjeta("Resumen", nil)
			r := test.WidgetRenderer(tarjeta).(*tarjetaRenderer)
			if tarjeta.Solida || tarjeta.rolFondo() != "surface" || tarjeta.tono() != TarjetaAzul {
				t.Fatalf("la tarjeta cero resuelve solid=%v, fondo=%q, tono=%q; el defecto web es la superficie blanca con tone blue",
					tarjeta.Solida, tarjeta.rolFondo(), tarjeta.tono())
			}
			for _, solida := range []bool{false, true} {
				tarjeta.Tono, tarjeta.Solida = TarjetaNeutra, solida
				tarjeta.Refresh()
				fondo, tinta := caso.superficie, caso.tinta
				if solida {
					fondo = color.NRGBA{R: 32, G: 40, B: 50, A: 255}
					tinta = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				}
				if got := r.caja.dentro.FillColor; got != color.Color(fondo) {
					t.Errorf("neutral solid=%v: fondo = %v, CSS = %v", solida, got, fondo)
				}
				if got := r.titulo.Color; got != color.Color(tinta) {
					t.Errorf("neutral solid=%v: título = %v, CSS = %v", solida, got, tinta)
				}
				if got := tarjeta.ColorTexto(caso.variante); got != tinta {
					t.Errorf("neutral solid=%v: ColorTexto = %v, CSS = %v", solida, got, tinta)
				}
			}
		})
	}
}

// Los filetes sólidos se componen sobre el fondo de recurso. Estos vectores son la
// mezcla CSS, hecha a mano, de `border` (tinta 15, 29, 32 al 12 %; en oscuro 230, 240,
// 235 al 11 %) y `border-subtle` (7 % en claro, 6 % en oscuro y traslúcido) sobre cada
// relleno; no llaman BordeSobre. La tarjeta blanca lleva `border-subtle` también en el
// contorno (la sombra ya dibuja el canto); la sólida, `border`.
func TestLosFiletesDeTarjetaSeComponenSobreSuTono(t *testing.T) {
	hex := func(rgb uint32) color.NRGBA {
		return color.NRGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
	}
	t.Cleanup(func() { tema.Elegir("") })
	for _, caso := range []struct {
		nombre    string
		tono      TonoTarjeta
		solida    bool
		borde     uint32
		linea     uint32
		alfaBorde uint8
		alfaLinea uint8
	}{
		// Sobre la superficie #ffffff: 255 − .07·(255 − tinta), con la tinta 15, 29, 32.
		{"claro", TarjetaAzul, false, 0xeeefef, 0xeeefef, 255, 255},
		// Sobre la superficie #131c1e, con el 6 % ya en bytes (15/255) de 231, 239, 239.
		{"oscuro", TarjetaAzul, false, 0x1f282a, 0x1f282a, 255, 255},
		// Sobre rgba(255, 255, 255, .66): el alfa combinado es .059 + .659·.941 = .679
		// (173/255) y el color, (tinta·.059 + 255·.620) / .679. La línea de la cabecera
		// va cruda, encima del relleno ya pintado.
		{"traslucido", TarjetaAzul, false, 0xeaebec, 0x0f1d20, 173, 15},
		// Sobre el sólido #2457e8 (claro y traslúcido) y #3d6cf0 (oscuro).
		{"claro", TarjetaAzul, true, 0x2150d1, 0x2253da, 255, 255},
		{"oscuro", TarjetaAzul, true, 0x507bf0, 0x4875f0, 255, 255},
		{"traslucido", TarjetaAzul, true, 0x2150d1, 0x2353dc, 255, 255},
		// Sobre el ámbar sólido #f2c230.
		{"claro", TarjetaAmbar, true, 0xd7ad2e, 0xe2b730, 255, 255},
		{"oscuro", TarjetaAmbar, true, 0xf1c745, 0xf2c53c, 255, 255},
		{"traslucido", TarjetaAmbar, true, 0xd7ad2e, 0xe5b830, 255, 255},
	} {
		t.Run(caso.nombre+"/"+string(caso.tono), func(t *testing.T) {
			tema.Elegir(caso.nombre)
			tarjeta := NuevaTarjeta("Resumen", nil)
			tarjeta.Tono, tarjeta.Solida = caso.tono, caso.solida
			r := test.WidgetRenderer(tarjeta).(*tarjetaRenderer)
			borde, linea := hex(caso.borde), hex(caso.linea)
			borde.A, linea.A = caso.alfaBorde, caso.alfaLinea
			colorBorde := r.caja.fuera.FillColor
			if r.caja.fuera.StrokeWidth > 0 {
				colorBorde = r.caja.fuera.StrokeColor
			}
			if got := color.NRGBAModel.Convert(colorBorde).(color.NRGBA); !cercaDe(got, borde, 1) || got.A != borde.A {
				t.Errorf("borde = %v; source-over CSS sobre su recurso = %v", got, borde)
			}
			if got := color.NRGBAModel.Convert(r.linea.FillColor).(color.NRGBA); !cercaDe(got, linea, 1) || got.A != linea.A {
				t.Errorf("filete = %v; tinta CSS sobre el relleno ya pintado = %v", got, linea)
			}
		})
	}
}

func TestLosTonosDeTarjetaConservanElRellenoYLaSombra(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	cuerpo := canvas.NewRectangle(color.NRGBA{A: 255})
	cuerpo.SetMinSize(fyne.NewSize(120, 40))
	tarjeta := NuevaTarjeta("Resumen", cuerpo)
	tarjeta.Tono, tarjeta.Solida = TarjetaPizarra, true
	tarjeta.SinRelleno = true
	r := test.WidgetRenderer(tarjeta).(*tarjetaRenderer)
	// `rounded-lg` (14) por fuera y, descontado el borde de 1, 13 por dentro.
	if r.sombra == nil || r.caja.fuera.CornerRadius != 14 || r.caja.dentro.CornerRadius != 13 {
		t.Errorf("el tono cambió la sombra o los radios: sombra=%v radios=%v/%v", r.sombra, r.caja.fuera.CornerRadius, r.caja.dentro.CornerRadius)
	}
	if got := tarjeta.MinSize(); got != fyne.NewSize(122, 93) {
		t.Errorf("sin relleno mide %v; cuerpo 120×40, cabecera 51 y dos bordes requieren 122×93", got)
	}
	tarjeta.Resize(fyne.NewSize(122, 93))
	if got := cuerpo.Position(); got != fyne.NewPos(1, 52) {
		t.Errorf("sin relleno el cuerpo empieza en %v, debe conservar (1,52)", got)
	}
	if got := cuerpo.Size(); got != fyne.NewSize(120, 40) {
		t.Errorf("sin relleno el cuerpo mide %v, debe conservar 120×40", got)
	}
	plana := NuevaTarjeta("Resumen", nil)
	plana.Tono, plana.Solida, plana.Plana = TarjetaPizarra, true, true
	if test.WidgetRenderer(plana).(*tarjetaRenderer).sombra != nil {
		t.Error("el acabado sólido perdió Plana y añadió sombra")
	}
}

// Esta captura comprueba el source-over real de Fyne contra la opacidad escrita
// en la hoja, sobre el FondoSuite que se pintó sin tarjeta. No obtiene la espera
// de Color, BordeSobre, SobreSRGB ni del generador del degradado.
// Chromium real confirma23/34/66 para blue oscuro sobre#131313. El pintor
// software de Fyne cuantiza dos veces a8bits: rasterx en image.RGBA y draw.Over.
// Sólo estos asserts de composición admiten ese error acotado de2niveles;
// tintas, alfa, radio, piezas y geometría se comprueban exactamente aparte.
func TestLaTarjetaTraslucidaDejaVerFondoSuiteSinBordeNiSombraInterior(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	// La superficie de cada apariencia, de la hoja: opaca en claro y en oscuro y blanca
	// al 66 % en el traslúcido, que es donde la tarjeta deja ver el lienzo.
	for _, caso := range []struct {
		nombre   string
		variante tema.Variante
		tinta    color.NRGBA
		opacidad float64
	}{
		{"claro", tema.Claro, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, 1},
		{"oscuro", tema.Oscuro, color.NRGBA{R: 19, G: 28, B: 30, A: 255}, 1},
		{"traslucido", tema.Translucido, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, .66},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			tema.Elegir(caso.nombre)
			fondo := NuevoFondoSuite(caso.variante)
			fondo.Resize(fyne.NewSize(300, 180))
			base := pinta(t, 300, 180, fondo)
			for _, plana := range []bool{false, true} {
				tarjeta := NuevaTarjeta("", nil)
				tarjeta.Plana = plana
				tarjeta.Move(fyne.NewPos(40, 40))
				tarjeta.Resize(fyne.NewSize(220, 100))
				img := pinta(t, 300, 180, fondo, tarjeta)
				for _, punto := range []fyne.Position{{X: 60, Y: 65}, {X: 150, Y: 90}, {X: 230, Y: 115}} {
					x, y := int(punto.X*escala), int(punto.Y*escala)
					debajo := px(base, x, y)
					mezcla := func(tinta, papel uint8) uint8 {
						return uint8(float64(tinta)*caso.opacidad + float64(papel)*(1-caso.opacidad) + .5)
					}
					css := color.NRGBA{R: mezcla(caso.tinta.R, debajo.R), G: mezcla(caso.tinta.G, debajo.G), B: mezcla(caso.tinta.B, debajo.B), A: 255}
					if got := px(img, x, y); !cercaDe(got, css, 2) {
						t.Errorf("plana=%v, %v: interior = %v; rgba CSS sobre FondoSuite = %v (base %v)", plana, punto, got, css, debajo)
					}
				}
			}
		})
	}
}

func TestLaCajaConTrazoConservaElBordeInteriorYAdmiteCambiarRelleno(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	relleno := color.NRGBA{R: 36, G: 87, B: 232, A: 41}
	borde := color.NRGBA{R: 20, G: 30, B: 40, A: 255}
	caja := NuevaCaja(relleno, borde, 8).ConTrazo()
	caja.Redimensionar(fyne.NewSize(100, 60))
	if caja.fuera.FillColor != color.Transparent || caja.fuera.StrokeColor != color.Color(borde) || caja.fuera.StrokeWidth != 1 {
		t.Errorf("contorno translúcido: fill=%v, stroke=%v, ancho=%v", caja.fuera.FillColor, caja.fuera.StrokeColor, caja.fuera.StrokeWidth)
	}
	if caja.fuera.Position() != fyne.NewPos(.5, .5) || caja.fuera.Size() != fyne.NewSize(99, 59) || caja.fuera.CornerRadius != 7.5 {
		t.Errorf("el eje del contorno no conserva el borde dentro de 100×60 y radio exterior 8: pos=%v tamaño=%v radio=%v", caja.fuera.Position(), caja.fuera.Size(), caja.fuera.CornerRadius)
	}
	if caja.dentro.Position() != fyne.NewPos(1, 1) || caja.dentro.Size() != fyne.NewSize(98, 58) || caja.dentro.CornerRadius != 7 {
		t.Errorf("el relleno no conserva su geometría: pos=%v tamaño=%v radio=%v", caja.dentro.Position(), caja.dentro.Size(), caja.dentro.CornerRadius)
	}
	caja.Pintar(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, borde)
	if caja.fuera.FillColor != color.Color(borde) || caja.fuera.StrokeWidth != 0 || caja.fuera.CornerRadius != 8 || caja.fuera.Position() != fyne.NewPos(0, 0) || caja.fuera.Size() != fyne.NewSize(100, 60) {
		t.Error("el relleno opaco no volvió a la caja de dos rectángulos con la misma medida")
	}
	caja.Pintar(relleno, borde)
	if caja.fuera.FillColor != color.Transparent || caja.fuera.StrokeWidth != 1 || caja.fuera.Position() != fyne.NewPos(.5, .5) {
		t.Error("al volver a rgba el borde dejó un fondo opaco debajo")
	}
	interior, exterior := caja.dentro, caja.fuera
	caja.CambiarRadio(16)
	if caja.dentro != interior || caja.fuera != exterior || caja.dentro.CornerRadius != 15 || caja.fuera.CornerRadius != 15.5 || caja.fuera.Size() != fyne.NewSize(99, 59) {
		t.Error("cambiar a radio 16 reconstruyó la caja, cambió sus medidas o perdió los radios interiores")
	}
	caja.CambiarRadio(0)
	if caja.dentro.CornerRadius != 0 || caja.fuera.CornerRadius != 0 {
		t.Error("un panel pegado al borde no perdió las esquinas al pedir radio 0")
	}
	caja.Redimensionar(fyne.NewSize(0, 0))
	if caja.fuera.Size() != fyne.NewSize(0, 0) || caja.dentro.Size() != fyne.NewSize(0, 0) {
		t.Error("un trazo todavía sin layout dejó lados negativos")
	}
	pinta(t, 20, 20, caja.Objetos()...)
	defecto := NuevaCaja(relleno, borde, 8)
	defecto.Redimensionar(fyne.NewSize(100, 60))
	if defecto.fuera.FillColor != color.Color(borde) || defecto.fuera.StrokeWidth != 0 || defecto.fuera.Size() != fyne.NewSize(100, 60) {
		t.Error("ConTrazo cambió el comportamiento de las cajas que no lo piden")
	}
}

func TestLaSombraSinInteriorConservaSuAlcanceYCache(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	sinRecorte := NuevaSombra(tema.SombraSM, 8)
	recortada := NuevaSombra(tema.SombraSM, 8).SinInterior()
	for _, sombra := range []*Sombra{sinRecorte, recortada} {
		sombra.Ajustar(fyne.NewPos(0, 0), fyne.NewSize(100, 60), tema.Claro)
	}
	if sinRecorte.Objeto().Position() != recortada.Objeto().Position() || sinRecorte.Objeto().Size() != recortada.Objeto().Size() {
		t.Error("recortar el interior cambió el alcance exterior de la sombra")
	}
	x, y := int(recortada.margen+50), int(recortada.margen+30)
	if p := px(sinRecorte.img.Image, x, y); p.A == 0 {
		t.Fatal("la sombra por defecto ya no conserva su interior")
	}
	if p := px(recortada.img.Image, x, y); p.A != 0 {
		t.Errorf("la sombra recortada conserva alfa %v dentro del panel", p.A)
	}
	x, y = int(recortada.margen+50), int(recortada.margen+62)
	if got, antes := px(recortada.img.Image, x, y), px(sinRecorte.img.Image, x, y); got != antes || got.A == 0 {
		t.Errorf("la sombra exterior cambió o desapareció: recortada=%v, original=%v", got, antes)
	}
	imagen := recortada.img.Image
	recortada.Ajustar(fyne.NewPos(20, 20), fyne.NewSize(100, 60), tema.Claro)
	if recortada.img.Image != imagen {
		t.Error("mover la sombra regeneró la imagen, aunque tamaño y apariencia no cambiaron")
	}
	recortada.CambiarRadio(0)
	recortada.Ajustar(fyne.NewPos(20, 20), fyne.NewSize(100, 60), tema.Claro)
	if recortada.img.Image == imagen {
		t.Error("cambiar el radio no invalidó la sombra recortada")
	}
	imagen = recortada.img.Image
	recortada.CambiarRadio(0)
	recortada.Ajustar(fyne.NewPos(20, 20), fyne.NewSize(100, 60), tema.Claro)
	if recortada.img.Image != imagen {
		t.Error("pedir otra vez el mismo radio invalidó innecesariamente la caché")
	}
	recortada.Ajustar(fyne.NewPos(20, 20), fyne.NewSize(120, 80), tema.Oscuro)
	if recortada.img.Image == imagen {
		t.Error("cambiar tamaño y apariencia no invalidó la sombra recortada")
	}
}

// El contorno combina borde+relleno una vez y deja ver el mesh. El filete de la
// cabecera se aplica sobre el relleno ya pintado. Las opacidades son las de CSS.
func TestLosFiletesTraslucidosDeTarjetaDejanVerFondoSuite(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, caso := range []struct {
		nombre    string
		variante  tema.Variante
		relleno   color.NRGBA
		alfaFondo float64
		ink       color.NRGBA
		alfaBorde float64
		alfaLinea float64
	}{
		// La superficie de la tarjeta blanca y `border-subtle` (6 %) en contorno y línea.
		{"oscuro", tema.Oscuro, color.NRGBA{R: 19, G: 28, B: 30, A: 255}, 1, color.NRGBA{R: 231, G: 239, B: 239, A: 255}, .06, .06},
		{"traslucido", tema.Translucido, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, .66, color.NRGBA{R: 15, G: 29, B: 32, A: 255}, .06, .06},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			tema.Elegir(caso.nombre)
			fondo := NuevoFondoSuite(caso.variante)
			fondo.Resize(fyne.NewSize(300, 180))
			base := pinta(t, 300, 180, fondo)
			tarjeta := NuevaTarjeta("Resumen", nil)
			tarjeta.Plana = true
			tarjeta.Move(fyne.NewPos(40, 40))
			tarjeta.Resize(fyne.NewSize(220, 100))
			img := pinta(t, 300, 180, fondo, tarjeta)
			for _, punto := range []struct {
				x, y int
				alfa float64
			}{{80, 160, caso.alfaBorde}, {519, 160, caso.alfaBorde}, {300, 182, caso.alfaLinea}} {
				papel := px(base, punto.x, punto.y)
				canal := func(tinte, ink, bg uint8) uint8 {
					relleno := float64(tinte)*caso.alfaFondo + float64(bg)*(1-caso.alfaFondo)
					return uint8(float64(ink)*punto.alfa + relleno*(1-punto.alfa) + .5)
				}
				css := color.NRGBA{R: canal(caso.relleno.R, caso.ink.R, papel.R), G: canal(caso.relleno.G, caso.ink.G, papel.G), B: canal(caso.relleno.B, caso.ink.B, papel.B), A: 255}
				// A 3 niveles y no a 2: el pintor de pruebas compone en bytes premultiplicados
				// y TRUNCA en cada capa, y aquí hay dos (el relleno al 22 % y el filete).
				// Con el filete al 6 % (15/255) el rojo pierde casi un nivel por capa: da 26
				// donde la cuenta exacta da 28,6. Un alfa equivocado se va mucho más lejos
				// (al 12 % serían 41).
				if got := px(img, punto.x, punto.y); !cercaDe(got, css, 3) {
					t.Errorf("filete (%v,%v) = %v; source-over CSS sobre el canvas = %v", punto.x, punto.y, got, css)
				}
			}
		})
	}
}
