// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
)

/*
SELECCIONAR TEXTO sobre una página que aquí es un dibujo, para resaltar, subrayar y
tachar (`resaltarSeleccion` de `PdfApp.tsx`).

En la web el texto de la página es TEXTO del DOM: el navegador selecciona y da los
rectángulos (`getClientRects`). Aquí la página llega rasterizada y no hay nada que
seleccionar, así que se hace lo que hace el navegador con la MISMA fuente de verdad: se
pide el SVG de esa página —el mismo que pinta la web— y se leen sus `<text>`. Cada uno
trae su matriz, su tamaño de letra y su `textLength`, que el motor fija al avance que el
PDF declara: exactamente con lo que el navegador coloca y selecciona ese texto.

El orden de la selección es el de los `<text>` en el SVG, que es el orden del flujo de
contenido: el mismo que sigue la selección del navegador sobre el DOM.

Lo que NO es igual, y por qué:

  - ponytail: dentro de un `<text>` el avance se reparte a partes iguales por carácter.
    El SVG da el largo del tramo y no el de cada glifo; una selección que empieza a media
    palabra puede caer medio carácter antes o después. Se releva si el motor manda el
    avance por glifo.
  - ponytail: el alto de la caja es el de Arial (sube 0,905 y baja 0,212 em), no el de la
    fuente incrustada, que el navegador sí lee. Lo que cambia es el alto del resaltado,
    no qué texto queda marcado.
  - ponytail: sin selección con el teclado (Mayúsculas + flechas). La web la tiene porque
    el navegador la da; aquí haría falta un cursor sobre el dibujo. Se releva con él.
*/

// Lo que sube y baja la caja de un carácter sobre la línea base, en em (Arial).
const (
	subeLetra = 0.905
	bajaLetra = 0.212
)

// TramoTexto es un `<text>` del SVG de una página.
type TramoTexto struct {
	// M es la matriz del tramo (`transform`) YA compuesta con su `scale(1,-1)`: lleva
	// del espacio del texto (x a lo largo de la línea base, y hacia abajo) al SVG.
	M [6]float64
	// Tam es el `font-size` y Largo el `textLength` (lo que mide el tramo, en el espacio
	// del texto).
	Tam, Largo float64
	Runas      []rune
}

var (
	reTexto  = regexp.MustCompile(`(?s)<text\s([^>]*)>(.*?)</text>`)
	reMatriz = regexp.MustCompile(`matrix\(([^)]*)\)`)
	reAttr   = func(nombre string) *regexp.Regexp { return regexp.MustCompile(`\s` + nombre + `="([^"]*)"`) }
	reTam    = reAttr("font-size")
	reLargo  = reAttr("textLength")
	reTransf = reAttr("transform")
)

// TramosDelSVG saca los `<text>` de la página. Los de `<defs>` no cuentan: no se pintan
// donde están.
func TramosDelSVG(svg string) []TramoTexto {
	if i := strings.LastIndex(svg, "</defs>"); i >= 0 {
		svg = svg[i:]
	}
	var out []TramoTexto
	for _, m := range reTexto.FindAllStringSubmatch(svg, -1) {
		attrs, contenido := " "+m[1], html.UnescapeString(m[2])
		runas := []rune(contenido)
		if strings.TrimSpace(contenido) == "" {
			continue
		}
		t := TramoTexto{M: [6]float64{1, 0, 0, 1, 0, 0}, Runas: runas}
		if a := reTransf.FindStringSubmatch(attrs); a != nil {
			if mm := reMatriz.FindStringSubmatch(a[1]); mm != nil {
				campos := strings.Fields(strings.ReplaceAll(mm[1], ",", " "))
				for i := 0; i < 6 && i < len(campos); i++ {
					t.M[i], _ = strconv.ParseFloat(campos[i], 64)
				}
			}
			if strings.Contains(a[1], "scale(1,-1)") {
				// M · diag(1, -1): se niega la columna de la y.
				t.M[2], t.M[3] = -t.M[2], -t.M[3]
			}
		}
		if a := reTam.FindStringSubmatch(attrs); a != nil {
			t.Tam, _ = strconv.ParseFloat(a[1], 64)
		}
		if a := reLargo.FindStringSubmatch(attrs); a != nil {
			t.Largo, _ = strconv.ParseFloat(a[1], 64)
		}
		if t.Largo <= 0 {
			// ponytail: un tramo de un solo glifo no lleva `textLength`; medio em por
			// carácter es el ancho medio de una letra latina.
			t.Largo = 0.5 * t.Tam * float64(len(runas))
		}
		out = append(out, t)
	}
	return out
}

// aSVG lleva un punto del espacio del texto al SVG.
func (t TramoTexto) aSVG(u, v float64) (float64, float64) {
	return t.M[0]*u + t.M[2]*v + t.M[4], t.M[1]*u + t.M[3]*v + t.M[5]
}

// aTexto es la inversa: del SVG al espacio del texto. Una matriz degenerada no tiene
// vuelta y deja el punto en el origen del tramo.
func (t TramoTexto) aTexto(x, y float64) (float64, float64) {
	det := t.M[0]*t.M[3] - t.M[2]*t.M[1]
	if det == 0 {
		return 0, 0
	}
	dx, dy := x-t.M[4], y-t.M[5]
	return (t.M[3]*dx - t.M[2]*dy) / det, (-t.M[1]*dx + t.M[0]*dy) / det
}

// caja es lo que ocupan en el SVG los caracteres [desde, hasta) del tramo.
func (t TramoTexto) caja(desde, hasta int) CajaSVG {
	n := float64(len(t.Runas))
	u0, u1 := t.Largo*float64(desde)/n, t.Largo*float64(hasta)/n
	v0, v1 := -subeLetra*t.Tam, bajaLetra*t.Tam
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, q := range [4][2]float64{{u0, v0}, {u1, v0}, {u1, v1}, {u0, v1}} {
		x, y := t.aSVG(q[0], q[1])
		minX, minY, maxX, maxY = math.Min(minX, x), math.Min(minY, y), math.Max(maxX, x), math.Max(maxY, y)
	}
	return CajaSVG{X: minX, Y: minY, Ancho: maxX - minX, Alto: maxY - minY}
}

// cursor es una posición en el texto de la página: el tramo y el hueco entre caracteres.
type cursor struct{ tramo, car int }

func (a cursor) antes(b cursor) bool {
	return a.tramo < b.tramo || (a.tramo == b.tramo && a.car < b.car)
}

// cursorEn es el hueco de texto más cercano a un punto del SVG: el tramo cuya caja cae
// más cerca y, dentro, el hueco entre caracteres más cercano.
func cursorEn(tramos []TramoTexto, x, y float64) (cursor, bool) {
	i, pos, ok := enTramo(tramos, x, y)
	if !ok {
		return cursor{}, false
	}
	return cursor{tramo: i, car: min(max(int(math.Round(pos)), 0), len(tramos[i].Runas))}, true
}

// enTramo es el tramo más cercano a un punto del SVG y dónde cae dentro, en caracteres
// (2,4 es a media distancia entre el tercero y el cuarto).
func enTramo(tramos []TramoTexto, x, y float64) (int, float64, bool) {
	mejor, pos, dist := 0, 0.0, math.Inf(1)
	for i, t := range tramos {
		u, v := t.aTexto(x, y)
		cu := math.Max(0, math.Min(u, t.Largo))
		cv := math.Max(-subeLetra*t.Tam, math.Min(v, bajaLetra*t.Tam))
		px, py := t.aSVG(cu, cv)
		if d := math.Hypot(px-x, py-y); d < dist {
			mejor, pos, dist = i, cu/t.Largo*float64(len(t.Runas)), d
		}
	}
	return mejor, pos, !math.IsInf(dist, 1)
}

// PalabraEn es lo que selecciona un doble clic en ese punto: la palabra (o el hueco, o
// el signo) bajo el puntero; con `linea`, el triple clic: la línea entera, los tramos
// seguidos sin salto (ver [separacion]). Lo devuelve como los dos puntos de un arrastre
// que la selecciona.
func PalabraEn(tramos []TramoTexto, x, y float64, linea bool) (desde, hasta [2]float64, ok bool) {
	i, pos, ok := enTramo(tramos, x, y)
	if !ok || len(tramos[i].Runas) == 0 {
		return desde, hasta, false
	}
	r := tramos[i].Runas
	if linea {
		ini, fin := i, i
		for ini > 0 && separacion(tramos[ini-1], tramos[ini]) != "\n" {
			ini--
		}
		for fin < len(tramos)-1 && separacion(tramos[fin], tramos[fin+1]) != "\n" {
			fin++
		}
		return tramos[ini].puntoDelHueco(0), tramos[fin].puntoDelHueco(len(tramos[fin].Runas)), true
	}
	k := min(max(int(pos), 0), len(r)-1)
	ini, fin := k, k+1
	for ini > 0 && claseDeLetra(r[ini-1]) == claseDeLetra(r[k]) {
		ini--
	}
	for fin < len(r) && claseDeLetra(r[fin]) == claseDeLetra(r[k]) {
		fin++
	}
	return tramos[i].puntoDelHueco(ini), tramos[i].puntoDelHueco(fin), true
}

// claseDeLetra junta lo que un doble clic toma de una vez: letras y cifras, espacios, y
// cada signo suelto.
func claseDeLetra(r rune) int {
	switch {
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	case unicode.IsSpace(r):
		return 2
	}
	return 3 + int(r)
}

// puntoDelHueco es el punto del SVG del hueco `car` del tramo, a media altura de la
// letra: [cursorEn] lo vuelve a dar como ese mismo hueco.
func (t TramoTexto) puntoDelHueco(car int) [2]float64 {
	x, y := t.aSVG(t.Largo*float64(car)/float64(len(t.Runas)), -0.3*t.Tam)
	return [2]float64{x, y}
}

// trozosSeleccion llama a `f` con cada tramo tocado entre dos puntos del SVG y los
// caracteres [ini, fin) que caen dentro, en el orden del documento.
func trozosSeleccion(tramos []TramoTexto, desde, hasta [2]float64, f func(i, ini, fin int)) {
	a, ok1 := cursorEn(tramos, desde[0], desde[1])
	b, ok2 := cursorEn(tramos, hasta[0], hasta[1])
	if !ok1 || !ok2 || a == b {
		return
	}
	if b.antes(a) {
		a, b = b, a
	}
	for i := a.tramo; i <= b.tramo; i++ {
		ini, fin := 0, len(tramos[i].Runas)
		if i == a.tramo {
			ini = a.car
		}
		if i == b.tramo {
			fin = b.car
		}
		if fin > ini {
			f(i, ini, fin)
		}
	}
}

// Seleccion son los trozos de texto entre dos puntos del SVG, en cajas del SVG: uno por
// tramo tocado, como los `getClientRects` de una selección del navegador.
func Seleccion(tramos []TramoTexto, desde, hasta [2]float64) []CajaSVG {
	var out []CajaSVG
	trozosSeleccion(tramos, desde, hasta, func(i, ini, fin int) { out = append(out, tramos[i].caja(ini, fin)) })
	return out
}

// TextoSeleccionado es lo que copia Ctrl+C de esa misma selección: los caracteres de cada
// tramo, con un salto de línea cuando el siguiente no sigue en la misma línea base y un
// espacio cuando sigue pero separado.
//
// ponytail: el navegador junta los `<text>` del SVG con sus propias reglas; estas son las
// de un visor de PDF, que es lo que se espera al pegar.
func TextoSeleccionado(tramos []TramoTexto, desde, hasta [2]float64) string {
	var b strings.Builder
	ant := -1
	trozosSeleccion(tramos, desde, hasta, func(i, ini, fin int) {
		if ant >= 0 {
			b.WriteString(separacion(tramos[ant], tramos[i]))
		}
		b.WriteString(string(tramos[i].Runas[ini:fin]))
		ant = i
	})
	return b.String()
}

// separacion es lo que va entre dos tramos seguidos: nada si el segundo empieza donde
// acaba el primero, un espacio si hay hueco en la misma línea, y un salto si cambia.
func separacion(a, b TramoTexto) string {
	u, v := a.aTexto(b.M[4], b.M[5])
	switch {
	case math.Abs(v) > 0.5*a.Tam:
		return "\n"
	case u-a.Largo > 0.15*a.Tam:
		return " "
	}
	return ""
}

// pedirTramos trae, una vez por página y documento, el SVG de la página `n` y se queda
// con sus tramos de texto.
func (p *Pantalla) pedirTramos(n int) {
	doc := p.doc
	if doc == nil || p.tramos[n] != nil || p.pidiendoTramos[n] {
		return
	}
	if p.tramos == nil {
		p.tramos, p.pidiendoTramos = map[int][]TramoTexto{}, map[int]bool{}
	}
	p.pidiendoTramos[n] = true
	// Lo que llegue después de cambiar el documento es texto de la versión anterior.
	version := p.versionTramos
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		svg, err := doc.SVG(ctx, n)
		fyne.Do(func() {
			defer p.avisarRed()
			if p.doc != doc || p.versionTramos != version {
				return
			}
			delete(p.pidiendoTramos, n)
			if err == nil {
				// Una página sin texto (un escaneo) se apunta vacía y no nil: no se vuelve
				// a pedir.
				p.tramos[n] = append([]TramoTexto{}, TramosDelSVG(svg)...)
			}
		})
	}()
}

// seleccionar es la selección mientras se arrastra con resaltar, subrayar o tachar, y
// su anotación al soltar (`resaltarSeleccion`). Con la mano se queda puesta para
// copiarla ([Pantalla.copiarSeleccion]); el SVG de la página se pide al primer arrastre,
// no al pasar por ella, porque casi nadie selecciona.
func (p *Pantalla) seleccionar(h *Hoja, g Gesto, fin bool) {
	if g.Clics > 1 {
		// ponytail: si el texto de la página no ha llegado, el doble clic se pierde (lo
		// pide para el siguiente).
		tramos, hay := p.tramos[h.Numero()]
		if !hay {
			p.pedirTramos(h.Numero())
			return
		}
		if g.Desde, g.Hasta, hay = PalabraEn(tramos, g.Desde[0], g.Desde[1], g.Clics > 2); !hay {
			return
		}
	}
	mano := h.Herramienta() == Mano
	if mano {
		// Una sola selección en todo el documento, como la del navegador: la nueva quita
		// la anterior, y un clic sin arrastrar solo quita, sin ir a por el texto.
		// ponytail: no pasa de una hoja a otra; la del navegador sí.
		p.copiable = ""
		for _, o := range p.hojas {
			if (o != h || g.Desde == g.Hasta) && len(o.seleccion) > 0 {
				o.PonerSeleccion(nil)
			}
		}
		if g.Desde == g.Hasta {
			return
		}
	}
	n := h.Numero()
	tramos, hay := p.tramos[n]
	if !hay {
		p.pedirTramos(n)
		return
	}
	cajas := Seleccion(tramos, g.Desde, g.Hasta)
	if mano {
		h.PonerSeleccion(cajas)
		if fin {
			p.copiable = TextoSeleccionado(tramos, g.Desde, g.Hasta)
		}
		return
	}
	if !fin {
		h.PonerSeleccion(cajas)
		return
	}
	h.PonerSeleccion(nil)
	if len(cajas) == 0 {
		return
	}
	pag := h.pagina
	cuadros := make([]Rect, len(cajas))
	for i, c := range cajas {
		ax, ay := APDF(pag, c.X, c.Y)
		bx, by := APDF(pag, c.X+c.Ancho, c.Y+c.Alto)
		cuadros[i] = Rect{math.Min(ax, bx), math.Min(ay, by), math.Max(ax, bx), math.Max(ay, by)}
	}
	hm := h.Herramienta()
	tipo, opacidad := "resaltado", 0.4
	switch hm {
	case Tachar:
		tipo, opacidad = "tachado", 1
	case Subrayar:
		tipo, opacidad = "subrayado", 1
	default:
	}
	hex, _ := p.tintaElegida()
	p.anotar([]AnotacionNueva{{
		Pagina: pag.Numero - 1, Tipo: tipo, Cuadros: cuadros,
		Color: ColorARGB(hex), Opacidad: opacidad, Grosor: float64(p.barra.Grosor),
	}})
}
