// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// ColoresCasilla son los de un `<input type="checkbox">` SIN estilo de Chromium: el
// del navegador, que no son tokens porque la suite no le pone `accent-color`. Cambian
// con el tema porque la hoja declara `color-scheme: dark` en oscuro (styles.css).
//
// Medidos en Chromium (CertiPDF, 2026-09-25): en claro, borde #767676, fondo blanco y,
// marcada, #0075ff con el tic blanco; en oscuro, borde #858585, fondo #3b3b3b y,
// marcada, #99c8ff con el tic #3b3b3b. Con el ratón encima (ERP, 2026-09-24): el borde
// #4f4f4f y la marcada #005cc8 en claro; #acacac y #d1e6ff en oscuro.
type ColoresCasilla struct {
	Borde, BordeEncima, Fondo, Marcada, MarcadaEncima, Tic color.NRGBA
	// Los del control apagado, ver conInertes.
	BordeInerte, FondoInerte, MarcadaInerte, TicInerte color.NRGBA
}

// CasillaNativa son los colores de la casilla de Chromium en esa variante.
func CasillaNativa(v tema.Variante) ColoresCasilla {
	gris := func(n uint8) color.NRGBA { return color.NRGBA{R: n, G: n, B: n, A: 255} }
	if v == tema.Oscuro {
		c := ColoresCasilla{
			Borde: gris(0x85), BordeEncima: gris(0xac), Fondo: gris(0x3b),
			Marcada: color.NRGBA{R: 0x99, G: 0xc8, B: 0xff, A: 255}, MarcadaEncima: color.NRGBA{R: 0xd1, G: 0xe6, B: 0xff, A: 255}, Tic: gris(0x3b),
		}
		c.conInertes(v)
		return c
	}
	c := ColoresCasilla{
		Borde: gris(0x76), BordeEncima: gris(0x4f), Fondo: gris(0xff),
		Marcada: color.NRGBA{R: 0x00, G: 0x75, B: 0xff, A: 255}, MarcadaEncima: color.NRGBA{R: 0x00, G: 0x5c, B: 0xc8, A: 255}, Tic: gris(0xff),
	}
	c.conInertes(v)
	return c
}

// Los apagados, en CRUDO: lo que pinta Chromium antes de que la etiqueta le ponga el
// `has-[:disabled]:opacity-50` encima. Medidos a ×2 en producción (2026-09-25, pulido de
// Fyne) ya con ese 50 % sobre `surface` —en claro, borde #e7e7e7, fondo #fbfbfb, la marcada
// #e7e7e7 y su tic #f5f5f5; en oscuro, #3b4246, #272f32, #444c4f y #272f32— y deshecha la
// mezcla: son los que, pasados por `atenuar`, dan esos. El apagado NO lleva el acento: la
// casilla marcada e inerte es gris, en claro y en oscuro.
func (c *ColoresCasilla) conInertes(v tema.Variante) {
	gris := func(n uint8) color.NRGBA { return color.NRGBA{R: n, G: n, B: n, A: 255} }
	if v == tema.Oscuro {
		c.BordeInerte, c.FondoInerte, c.MarcadaInerte, c.TicInerte = gris(0x62), gris(0x3b), gris(0x74), gris(0x3b)
		return
	}
	c.BordeInerte, c.FondoInerte, c.MarcadaInerte, c.TicInerte = gris(0xcf), gris(0xf7), gris(0xcf), gris(0xeb)
}

// acentoClaro dice si Chromium trata ese `accent-color` como CLARO: el blanco del tic no
// llega a 3:1 contra él (luminancia relativa por encima de 0,30), así que pinta la casilla
// con el aspecto del tema oscuro —el tic del color de la casilla sin estilo, #3b3b3b— y con
// el ratón encima lo ACLARA en vez de oscurecerlo.
//
// El corte estaba en 0,5 y con los cuatro acentos medidos hasta entonces daba igual; el
// primario claro de un tema oscuro (el esmeralda #2fc98a, 0,44) cae en medio y lo delató: Chromium le pone
// tic #3b3b3b y lo aclara a #58d8a4 (medido el 2026-10-04, ×2).
func acentoClaro(c color.NRGBA) bool {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	return 0.2126*lineal(r)+0.7152*lineal(g)+0.0722*lineal(b) > 0.3
}

// AcentoEncima es el acento de una casilla o un radio MARCADOS con el ratón encima, como lo
// calcula Chromium para cualquier `accent-color`: la misma tinta con la luminosidad HSL
// 28/255 más baja si el acento es oscuro, y más alta si es claro (ver acentoClaro).
//
// Medido, y la cuenta da los seis con un nivel de margen: #1a3541 pasa a #0a1419, #1a6fbb
// a #13528a, el petróleo #0f7680 a #065132 y el del tema oscuro #2fc98a a #58d8a4
// (`accent-primary` de Form.tsx); el azul de serie #0075ff a #005cc8 y el de oscuro
// #99c8ff a #d1e6ff (`CasillaNativa`).
func AcentoEncima(c color.NRGBA) color.NRGBA {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	l := (hi + lo) / 2
	d := 28.0 / 255
	if !acentoClaro(c) {
		d = -d
	}
	nl := min(max(l+d, 0), 1)
	if hi == lo {
		n := uint8(math.Round(nl * 255))
		return color.NRGBA{R: n, G: n, B: n, A: c.A}
	}
	// Mismo tono y misma saturación HSL: cada canal conserva su sitio entre el mínimo y el
	// máximo, y los dos extremos se mueven con la luminosidad.
	s := (hi - lo) / (1 - math.Abs(2*l-1))
	croma := (1 - math.Abs(2*nl-1)) * s
	nlo := nl - croma/2
	canal := func(x float64) uint8 { return uint8(math.Round((nlo + (x-lo)/(hi-lo)*croma) * 255)) }
	return color.NRGBA{R: canal(r), G: canal(g), B: canal(b), A: c.A}
}

// lineal es la componente sRGB en luz lineal, para la luminancia relativa.
func lineal(x float64) float64 {
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

// CuerpoTicChromium es el tic que Chromium pinta en una casilla marcada, en la rejilla de 24
// (`NativeThemeBase::PaintCheckbox`): de (0,2·l, 0,5·l) baja 0,2·l en diagonal y sube hasta
// (0,8·l, 0,2·l), con un grosor de 0,16·l, puntas planas y la esquina en inglete. Casa con la
// captura a ×2 de la casilla de 14 en producción; el `check` del catálogo de la suite, que es
// lo que se pintaba antes, sale con la mitad de grosor.
const CuerpoTicChromium = `<path d="M4.8 12 9.6 16.8 19.2 4.8" stroke-width="3.84" stroke-linecap="butt" stroke-linejoin="miter"/>`

// PiezaCasilla es el cuadradito (o el círculo) de un `<input type="checkbox|radio">` tal
// como lo pinta Chromium: la ÚNICA copia de ese dibujo en la suite. La usan `Eleccion` —el
// `Checkbox`/`Radio` de Form.tsx— y las casillas SIN estilo que la web deja al navegador
// (la tabla del ERP, los comentarios de CertiPDF, las tareas de Notepads, los medios de
// Diapos), que antes se escribían la suya, cada una con su tic.
//
// Geometría de Chromium, medida a ×2: esquina de 2 y borde de 1; marcada, el acento entero
// sin borde con el tic de CuerpoTicChromium; a medias, una raya de 0,6·l × 0,16·l centrada
// y con las puntas apenas redondeadas; el radio marcado, un anillo del acento de 1, el hueco
// del color del tic y un punto del acento de 0,6·l.
//
// La raya NO va del color del tic: va del de la casilla SIN estilo en ese tema, también con
// `accent-primary`. Medido en producción: blanca en claro y #3b3b3b en oscuro, donde el tic
// de la misma casilla es blanco.
type PiezaCasilla struct {
	lado            float32
	redondo, nativa bool
	caja            *Caja
	tic             *iconos.Dibujo
	raya            *canvas.Rectangle
	punto           *canvas.Circle
}

// EstadoCasilla es lo que cambia de un pintado a otro.
type EstadoCasilla struct{ Marcada, Parcial, Encima, Inerte bool }

// NuevaPiezaCasilla crea la pieza. `nativa` es la casilla SIN `accent-color` (el azul de
// Chromium); sin ella lleva `accent-primary`, que es lo que pone Form.tsx.
func NuevaPiezaCasilla(lado float32, redondo, nativa bool, v tema.Variante) *PiezaCasilla {
	n := CasillaNativa(v)
	radio := radioPieza(lado, redondo)
	p := &PiezaCasilla{
		lado: lado, redondo: redondo, nativa: nativa,
		caja:  NuevaCaja(n.Fondo, n.Borde, radio),
		tic:   iconos.TrazoDeCuerpo(CuerpoTicChromium, lado, n.Tic),
		raya:  canvas.NewRectangle(n.Tic),
		punto: canvas.NewCircle(n.Marcada),
	}
	p.raya.CornerRadius = 0.04 * lado
	return p
}

// RadioCasillaNativa es la esquina de la casilla de Chromium: 2, a cualquier tamaño.
const RadioCasillaNativa float32 = 2

// radioPieza es la esquina de una pieza de ese lado: la de Chromium en la casilla y medio
// lado —un círculo— en el radio. De ella sale también la del aro de Eleccion.
func radioPieza(lado float32, redondo bool) float32 {
	if redondo {
		return lado / 2
	}
	return RadioCasillaNativa
}

// Objetos van en ese orden en los `Objects()` del renderer.
func (p *PiezaCasilla) Objetos() []fyne.CanvasObject {
	return append(p.caja.Objetos(), p.tic.Objeto(), p.raya, p.punto)
}

// Colocar pone la pieza con su esquina de arriba a la izquierda en `pos`.
func (p *PiezaCasilla) Colocar(pos fyne.Position) {
	l := p.lado
	MoverCaja(p.caja, pos, fyne.NewSize(l, l))
	p.tic.Colocar(pos.X, pos.Y, l)
	p.raya.Move(fyne.NewPos(pos.X+0.2*l, pos.Y+0.42*l))
	p.raya.Resize(fyne.NewSize(0.6*l, 0.16*l))
	p.punto.Move(fyne.NewPos(pos.X+0.2*l, pos.Y+0.2*l))
	p.punto.Resize(fyne.NewSize(0.6*l, 0.6*l))
}

// Pintar pone los colores del estado.
func (p *PiezaCasilla) Pintar(e EstadoCasilla, v tema.Variante) {
	n := CasillaNativa(v)
	// El tic lo decide Chromium por el acento, no la hoja: blanco sobre un acento oscuro y
	// #3b3b3b sobre uno claro. No es `on-primary`.
	acento, tic := tema.Color("primary", v), color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	if acentoClaro(acento) {
		tic = CasillaNativa(tema.Oscuro).Tic
	}
	if p.nativa {
		acento, tic = n.Marcada, n.Tic
	}
	fondo, borde, raya := n.Fondo, n.Borde, n.Tic
	// El hueco del radio marcado va del color del tic; apagado, del fondo (medido: #fbfbfb y
	// no #f5f5f5 en claro).
	hueco := tic
	switch {
	case e.Inerte:
		acento, tic, fondo, borde, raya = n.MarcadaInerte, n.TicInerte, n.FondoInerte, n.BordeInerte, n.TicInerte
		hueco = fondo
	case e.Encima:
		acento, borde = AcentoEncima(acento), n.BordeEncima
	}
	acento, tic, raya, hueco = atenuar(acento, e.Inerte), atenuar(tic, e.Inerte), atenuar(raya, e.Inerte), atenuar(hueco, e.Inerte)
	fondo, borde = atenuar(fondo, e.Inerte), atenuar(borde, e.Inerte)

	marcada := e.Marcada || (e.Parcial && !p.redondo)
	switch {
	case marcada && !p.redondo:
		// Un cuadrado SÓLIDO del acento: todo el color en el rectángulo de fuera, y el de
		// dentro transparente para no apilar dos capas atenuadas.
		p.caja.Pintar(color.NRGBA{}, acento)
	case marcada:
		p.caja.Pintar(hueco, acento)
	default:
		p.caja.Pintar(fondo, borde)
	}
	p.tic.TenirCon(tic)
	p.tic.Ver(marcada && !p.redondo && !e.Parcial)
	p.raya.FillColor = raya
	ver(p.raya, e.Parcial && !p.redondo)
	p.raya.Refresh()
	p.punto.FillColor = acento
	ver(p.punto, marcada && p.redondo)
	p.punto.Refresh()
}
