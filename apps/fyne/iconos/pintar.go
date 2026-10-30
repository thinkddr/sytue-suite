// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"weak"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # Por qué esto rasteriza y no usa canvas.Image con un recurso SVG
//
// Fyne trae rasterizador de SVG y la tentación es dejarle el trabajo. Medido el
// 2026-09-21 contra Chromium, con el mismo SVG y el mismo número de píxeles, no vale
// por dos motivos, y ninguno se arregla desde el SVG:
//
//  1. **Rasteriza al tamaño que le diga `CanvasForObject`, no al de dispositivo.**
//     `canvas.Image.renderSVG` pregunta por el lienzo del objeto; si todavía no está
//     colgado de uno —o si el que contesta es el del driver de pruebas— se queda con
//     los píxeles de CSS y después ESTIRA el mapa de bits con `ApproxBiLinear`. Un
//     icono de 24 a ×2 salía rasterizado a 24×24 y ampliado a 48×48: el trazo, que en
//     el navegador son cuatro filas limpias, se convertía en dos negras y cuatro
//     medias. `canvas.Raster` no tiene ese problema: Fyne llama al generador con el
//     número de píxeles de DISPOSITIVO y pinta el resultado tal cual.
//
//  2. **`oksvg` no escala el grosor del trazo con el viewBox.** `DrawTransformed`
//     transforma los puntos del camino y pasa `LineWidth` sin tocar
//     (`r.SetStroke(fixed.Int26_6(svgp.LineWidth*64), …)`). Un `stroke-width="2"` en
//     una rejilla de 24 pintada a 48 px salía de 2 px cuando el navegador da 4. Se
//     corrige aquí multiplicando por la escala del viewBox, que es lo que dice la
//     especificación.
//
// Lo que sí hace bien `oksvg`, comprobado icono a icono: `stroke-linecap`,
// `stroke-linejoin`, `fill-opacity`, los grupos, los caminos con arcos y el viewBox a
// cualquier talla. Por eso NO se reescribe ni un trazo a mano: se pinta el mismo SVG
// que sirve el navegador.

// rejilla es la caja de los iconos de trazo de la web (`viewBox="0 0 24 24"`).
const rejilla = 24

// paridad-web: apps/frontend/packages/ui/src/components/Glifo.tsx

// # El estilo del trazo: «duotono con acento»
//
// Es el de `Glifo.tsx`, el envoltorio de TODOS los iconos de interfaz de la web, y se
// pinta aquí con sus mismos números (`TestElEstiloEsElDeGlifo` los lee de ese .tsx):
//
//  1. El trazo mide 1,5 px de CSS a cualquier tamaño. Antes era `stroke-width="2"` en
//     la rejilla de 24, que escala con el icono: 1,3 px a 16 y 2,7 px a 32.
//  2. Debajo del trazo va una capa de relleno, solo en los subtrazos CERRADOS del
//     dibujo y en par-impar: una forma dentro de otra queda como hueco.
//  3. El color de esa capa es el `--sytue-glyph-fill` de la web: el primario del tema
//     bajo una tinta neutra, y la propia tinta cuando es de color o va sobre un relleno
//     de color (ver `acentoDeAhora`), salvo que quien pone el icono diga otra cosa.
//  4. Los puntos del dibujo —los de «…», las viñetas— van macizos y con tamaño propio.
const (
	// grosorPx es `TRAZO_PX` de Glifo.tsx: el trazo en píxeles de CSS.
	grosorPx = 1.5
	// opacidadAcento es el `fill-opacity` de la capa de relleno.
	opacidadAcento = 0.34
)

// grosorTrazo es `grosorDeGlifo` de Glifo.tsx: el `stroke-width`, en unidades de la
// rejilla, con el que el trazo mide 1,5 px cuando el icono va a `lado` px. Con su
// mismo redondeo a centésimas, para que los dos lados pinten el mismo número.
func grosorTrazo(lado float32) float64 {
	if lado <= 0 {
		return grosorPx
	}
	return math.Round(grosorPx*rejilla/float64(lado)*100) / 100
}

// pintura es con qué se pinta un dibujo en ESTE repintado. Se junta en un tipo porque
// es también la clave de la caché: si cambia cualquiera de los tres, el mapa guardado
// ya no vale.
type pintura struct {
	tinta  color.NRGBA // el `currentColor` de la web
	acento color.NRGBA // el relleno de las formas cerradas de un glifo
	lado   float32     // el lado en px de CSS: de él sale el grosor del trazo
}

// svgDeTrazo arma el SVG de un icono con su color ya dentro. El color va en la cadena
// —y no se retoca después— porque `svg.Colorize` de Fyne solo sustituye RELLENOS, y
// estos iconos son sobre todo trazo: pasarlos por ahí los dejaría del color original.
func svgDeTrazo(d string, p pintura) []byte { return svgDe(d, p, false) }

// svgDe arma el SVG con la forma LLENA o sin llenar. `lleno` es el `filled` de `Glyph`
// en Sytue Mail (`fill="currentColor"`, con el trazo intacto): la estrella destacada.
// Lleno no lleva capa de acento, igual que en la web: quedaría debajo y no se vería.
func svgDe(d string, p pintura, lleno bool) []byte {
	c := p.tinta
	relleno, capa := "none", ""
	if lleno {
		relleno = fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	} else {
		capa = capaDeAcento(subtrazosCerrados(d), p)
	}
	return fmt.Appendf(nil,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">%s`+
			`<path d="%s" fill="%s" fill-opacity="%.4f" stroke="#%02x%02x%02x" stroke-opacity="%.4f"`+
			` stroke-width="%g" stroke-linecap="round" stroke-linejoin="round"/>%s</svg>`,
		rejilla, rejilla, rejilla, rejilla, capa, d, relleno, float32(c.A)/255,
		c.R, c.G, c.B, float32(c.A)/255, grosorTrazo(p.lado), capaDePuntos(puntos(d), c))
}

// capaDePuntos es el `<path>` macizo que `Glifo.tsx` pone ENCIMA del dibujo con los
// puntos del glifo (ver `puntos`). Sin puntos no hay capa.
func capaDePuntos(motas string, c color.NRGBA) string {
	if motas == "" {
		return ""
	}
	return `<path d="` + motas + fmt.Sprintf(`" fill="#%02x%02x%02x" fill-opacity="%.4f"/>`, c.R, c.G, c.B, float32(c.A)/255)
}

// capaDeAcento es el `<path>` que `Glifo.tsx` pone DEBAJO del trazo: los subtrazos
// cerrados, rellenos en par-impar y sin trazo. Sin nada que rellenar no hay capa.
//
// La opacidad se multiplica por el alfa de la tinta: un icono inerte se pinta con la
// tinta al 50 % (`Dibujo.TenirCon`), y en la web ese 50 % es la opacidad del control
// entero, relleno incluido.
//
// ponytail: eso atenúa también el relleno cuando la tinta es un token que ya trae
// alfa (alguno del tema translúcido), cosa que la web no hace. Son unas décimas sobre
// un 34 %; si se llega a notar, la opacidad del control viaja aparte de la tinta.
func capaDeAcento(cerrados string, p pintura) string {
	if cerrados == "" {
		return ""
	}
	a := p.acento
	return `<path d="` + cerrados + fmt.Sprintf(`" fill="#%02x%02x%02x" fill-opacity="%.4f" fill-rule="evenodd"/>`,
		a.R, a.G, a.B, opacidadAcento*float64(a.A)/255*float64(p.tinta.A)/255)
}

// rasteriza pinta un SVG en un mapa de bits de w×h píxeles de DISPOSITIVO, con fondo
// transparente.
//
// Un SVG que no se entienda es un error de quien lo metió en el paquete, no un estado
// de la aplicación, así que entra en pánico como `regexp.MustCompile`. Y lo hace al
// construir el icono, no la primera vez que se pinta esa pantalla.
func rasteriza(svg []byte, w, h int) *image.NRGBA {
	svg = normaliza(svg)
	return rasterizaCon(svg, w, h, parImpar(svg))
}

// RasterizarSVG pinta un SVG entero a w×h píxeles de DISPOSITIVO con las mismas
// correcciones que los iconos (el grosor del trazo con la escala del viewBox, los arcos
// y los `rx` sin `ry`). Es para quien dibuja SVG propio que no es un icono —los
// gráficos y los minigráficos de Calcus— y se pinta con un `canvas.Raster` por el
// punto 1 de la nota de arriba. El SVG tiene que ser legible: si no, entra en pánico.
func RasterizarSVG(svg []byte, w, h int) *image.NRGBA { return rasteriza(svg, w, h) }

// rasterizaCrudo es rasteriza SIN las correcciones de `normaliza` ni la de `parImpar`.
// Existe para que el guardián pueda enseñar lo que pasa sin ellas; nadie más debería
// llamarla.
func rasterizaCrudo(svg []byte, w, h int) *image.NRGBA { return rasterizaCon(svg, w, h, nil) }

func rasterizaCon(svg []byte, w, h int, impares []int) *image.NRGBA {
	if w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	icono, err := oksvg.ReadIconStream(bytes.NewReader(svg))
	if err != nil {
		panic("iconos: SVG ilegible: " + err.Error())
	}
	// El grosor del trazo, a mano: ver el punto 2 de la nota de arriba.
	esc := float64(w) / icono.ViewBox.W
	for i := range icono.SVGPaths {
		icono.SVGPaths[i].LineWidth *= esc
	}
	icono.SetTarget(0, 0, float64(w), float64(h))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	sc := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	trazador := rasterx.NewDasher(w, h, sc)
	// En orden, como `icono.Draw`: lo de después tapa a lo de antes.
	for i := range icono.SVGPaths {
		if slices.Contains(impares, i) {
			pintarParImpar(rgba, &icono.SVGPaths[i], icono.Transform, trazador)
			continue
		}
		icono.SVGPaths[i].DrawTransformed(trazador, 1, icono.Transform)
	}

	out := image.NewNRGBA(rgba.Bounds())
	draw.Draw(out, rgba.Bounds(), rgba, rgba.Bounds().Min, draw.Src)
	return out
}

// Dos cosas que `oksvg` NO hace y que la especificación de SVG sí dice. No se arreglan
// en `branding/`, que es la fuente de verdad y la comparte con la web: se corrigen al
// leer, y hay un guardián por cada una que enseña el estropicio si se quitan.
var (
	// `ry` toma el valor de `rx` cuando no se declara. `oksvg` lo deja en 0 y entonces
	// `rasterx.AddRoundRect` pinta la tesela CUADRADA: las diecinueve marcas de
	// producto perdían su esquina redonda.
	rectSinRy = regexp.MustCompile(`(<rect\b[^>]*?\brx="([0-9.]+)")([^>]*?)(/?>)`)
	yaTieneRy = regexp.MustCompile(`\bry="`)
	// `scale(k)` es `scale(k, k)`. `oksvg` pide dos números y, si solo hay uno, aborta
	// el fichero ENTERO con «param mismatch»: la marca se quedaba sin su glifo.
	escalaDeUno = regexp.MustCompile(`scale\(\s*(-?[0-9.]+)\s*\)`)
	// Los semicírculos relativos necesitan una holgura en sus radios para evitar
	// puntos de control en NaN por el redondeo de sus extremos; ver `holgura`.
	atributoD = regexp.MustCompile(`(\s)d="([^"]*)"`)
	numeroSVG = regexp.MustCompile(`^[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?`)
	// Un `stroke-width` DENTRO de un `<g transform="… scale(…)">` se pinta en el sistema
	// ya escalado del grupo, así que el grosor también encoge; `oksvg` transforma los
	// puntos pero no el grosor. Se multiplica por la escala del grupo (la media
	// geométrica de sus ejes). Solo el EXPLÍCITO: el heredado se queda como está, y de
	// eso vive el glifo de PDF (`GlifoPDF` en apps/certipdf), cuyo sello a 0,63 tiene
	// que salir con el mismo grosor que la hoja.
	grupoEscalado  = regexp.MustCompile(`(?s)<g\b[^>]*\btransform="([^"]*)"[^>]*>.*?</g>`)
	escalaDeGrupo  = regexp.MustCompile(`scale\(\s*(-?[0-9.]+)[\s,]+(-?[0-9.]+)\s*\)`)
	grosorExplicit = regexp.MustCompile(`\bstroke-width="([0-9.]+)"`)
)

// arcosConHolgura ajusta los radios sin separar los arcos agrupados: `oksvg` ya
// interpreta cada juego de siete parámetros tras una sola A/a. El resto de órdenes
// se deja tal cual. Las banderas pueden ir pegadas («a2 2 0 012 2»), así que se leen
// como un solo carácter, que es lo que dice la gramática de SVG.
func arcosConHolgura(d string) string {
	var out strings.Builder
	for i := 0; i < len(d); {
		c := d[i]
		if c != 'a' && c != 'A' {
			out.WriteByte(c)
			i++
			continue
		}
		i++
		out.WriteByte(c)
		leidos := 0
		for {
			args, fin, ok := juegoDeArco(d[i:])
			if !ok {
				break
			}
			if leidos > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(strings.Join(args, " "))
			i += fin
			leidos++
		}
	}
	return out.String()
}

// juegoDeArco lee siete parámetros de arco del principio de `s` —cinco números y dos
// banderas— y dice cuánto ha consumido. Sin un juego completo devuelve `ok` a falso y
// no consume nada.
func juegoDeArco(s string) (args []string, fin int, ok bool) {
	saltar := func() {
		for fin < len(s) && (s[fin] == ' ' || s[fin] == ',' || s[fin] == '\t' || s[fin] == '\n' || s[fin] == '\r') {
			fin++
		}
	}
	for k := range 7 {
		saltar()
		if fin >= len(s) {
			return nil, 0, false
		}
		if k == 3 || k == 4 {
			if s[fin] != '0' && s[fin] != '1' {
				return nil, 0, false
			}
			args = append(args, s[fin:fin+1])
			fin++
			continue
		}
		n := numeroSVG.FindString(s[fin:])
		if n == "" {
			return nil, 0, false
		}
		fin += len(n)
		if k < 2 {
			n = holgura(n)
		}
		args = append(args, n)
	}
	return args, fin, true
}

// holgura agranda un radio de arco en una diezmillonésima.
//
// Un semicírculo EXACTO —el radio es la mitad de la cuerda, como se dibujan los
// círculos de dos medios— sale
// de un solo cubo con los puntos de control en NaN cuando el final es relativo, porque
// `placeY + 12.8` da 31,400000000000002 y la cuerda queda un pelo más larga que el
// diámetro. «a6.4 6.4 0 1 1 0 12.8» no pinta nada; «a6.41…» y el mismo arco en absoluto
// sí. Con la holgura el centro se mueve 0,003 unidades del viewBox, que no es un píxel
// en ninguna talla de la suite.
func holgura(n string) string {
	v, err := strconv.ParseFloat(n, 64)
	if err != nil || v == 0 {
		return n
	}
	return strconv.FormatFloat(v*(1+1e-7), 'g', -1, 64)
}

// # `fill-rule="evenodd"`, que `oksvg` lee y `rasterx` no pinta
//
// Once de las diecisiete marcas de producto dibujan su glifo con AGUJEROS por la regla
// par-impar: las ventanas del edificio del ERP, las líneas de la hoja de Docus. `oksvg`
// guarda la regla, pero el único rasterizador de `rasterx`, `ScannerGV`, la ignora
// («no-op as scanner gv does not support even-odd winding») y rellena con la de no
// cero: el ERP era un cuadrado blanco. No se arregla en `branding/`, que es la fuente
// de la web: se pinta aquí cada subtrayecto por separado y se combinan en par-impar,
// que para agujeros sin bordes compartidos da lo mismo que el navegador.

// pintarParImpar pinta el relleno de `p` con la regla par-impar sobre `dst` y, si lo
// lleva, su trazo encima con el trazador de siempre.
func pintarParImpar(dst *image.RGBA, p *oksvg.SvgPath, t rasterx.Matrix2D, trazador *rasterx.Dasher) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cobertura := make([]float64, w*h)
	var trozos subtrayectos
	p.Path.AddTo(&trozos)
	for _, sub := range trozos {
		uno := *p
		uno.Path, uno.FillOpacity = sub, 1
		uno.SetFillColor(color.NRGBA{A: 0xff})
		uno.SetLineColor(nil)
		lienzo := image.NewRGBA(dst.Bounds())
		uno.DrawTransformed(rasterx.NewDasher(w, h, rasterx.NewScannerGV(w, h, lienzo, lienzo.Bounds())), 1, t)
		// La probabilidad de cruzar un número impar de bordes: `a ⊕ b` con cobertura
		// parcial en los píxeles del borde.
		for i := range cobertura {
			a := float64(lienzo.Pix[4*i+3]) / 0xff
			cobertura[i] += a - 2*cobertura[i]*a
		}
	}
	r, g, b, a := p.GetFillColor().RGBA() // premultiplicado, 16 bits
	for i, c := range cobertura {
		f := c * p.FillOpacity
		if f <= 0 {
			continue
		}
		fondo := 1 - float64(a)/0xffff*f
		px := dst.Pix[4*i : 4*i+4 : 4*i+4]
		for k, v := range [4]uint32{r, g, b, a} {
			px[k] = uint8(min(float64(v)/0x101*f+float64(px[k])*fondo+0.5, 0xff))
		}
	}
	trazo := *p
	trazo.SetFillColor(nil)
	trazo.DrawTransformed(trazador, 1, t)
}

// subtrayectos es un `rasterx.Adder` que separa un trayecto en sus subtrayectos: uno
// por cada `Start`.
type subtrayectos []rasterx.Path

func (s *subtrayectos) actual() *rasterx.Path { return &(*s)[len(*s)-1] }

func (s *subtrayectos) Start(a fixed.Point26_6) {
	*s = append(*s, nil)
	s.actual().Start(a)
}
func (s *subtrayectos) Line(b fixed.Point26_6)             { s.actual().Line(b) }
func (s *subtrayectos) QuadBezier(b, c fixed.Point26_6)    { s.actual().QuadBezier(b, c) }
func (s *subtrayectos) CubeBezier(b, c, d fixed.Point26_6) { s.actual().CubeBezier(b, c, d) }

// Stop cierra el subtrayecto en curso. `AddTo` lo llama también ANTES del primer
// `Start` («implicit close if currently in path»), cuando todavía no hay ninguno.
func (s *subtrayectos) Stop(cerrar bool) {
	if len(*s) > 0 {
		s.actual().Stop(cerrar)
	}
}

// estiloRelleno es lo que un elemento hereda de su `<g>` para decidir el par-impar: la
// regla y si rellena de verdad.
type estiloRelleno struct{ impar, relleno bool }

// con aplica los atributos propios de un elemento sobre lo heredado.
func (st estiloRelleno) con(atributos []xml.Attr) estiloRelleno {
	for _, a := range atributos {
		v := strings.TrimSpace(a.Value)
		switch a.Name.Local {
		case "fill-rule":
			st.impar = v == "evenodd"
		case "fill":
			st.relleno = v != "none" && !strings.HasPrefix(v, "url(")
		case "style":
			if strings.Contains(strings.ReplaceAll(v, " ", ""), "fill-rule:evenodd") {
				st.impar = true
			}
		}
	}
	return st
}

// parImpar son los índices, entre las figuras del SVG y en el orden en que `oksvg` las
// convierte en trazados, de las que se rellenan con `fill-rule="evenodd"` —propio o
// heredado de su `<g>`—. Las de `fill="none"` no cuentan, y tampoco las de un
// degradado (`url(…)`), que ninguna marca usa con agujeros.
//
// ponytail: cuenta las figuras que `oksvg` convierte (`line`, `rect`, `circle`,
// `ellipse`, `polyline`, `polygon`, `path`) y se salta `<defs>`; un `<use>` que
// clonara una figura desplazaría la cuenta. Ninguna marca ni icono lo usa; si alguno
// lo hace, se sigue la referencia.
func parImpar(svg []byte) []int {
	var (
		out    []int
		figura int
		pila   []estiloRelleno // lo que hereda cada nivel
		enDefs int
	)
	d := xml.NewDecoder(bytes.NewReader(svg))
	for {
		tok, err := d.Token()
		if err != nil {
			return out
		}
		switch e := tok.(type) {
		case xml.StartElement:
			st := estiloRelleno{relleno: true}
			if len(pila) > 0 {
				st = pila[len(pila)-1]
			}
			st = st.con(e.Attr)
			pila = append(pila, st)
			switch e.Name.Local {
			case "defs":
				enDefs++
			case "line", "rect", "circle", "ellipse", "polyline", "polygon", "path":
				if enDefs == 0 {
					if st.impar && st.relleno {
						out = append(out, figura)
					}
					figura++
				}
			}
		case xml.EndElement:
			if e.Name.Local == "defs" {
				enDefs--
			}
			if len(pila) > 0 {
				pila = pila[:len(pila)-1]
			}
		}
	}
}

func normaliza(svg []byte) []byte {
	svg = escalaDeUno.ReplaceAll(svg, []byte("scale($1 $1)"))
	svg = grupoEscalado.ReplaceAllFunc(svg, grosorEnGrupo)
	svg = atributoD.ReplaceAllFunc(svg, func(m []byte) []byte {
		p := atributoD.FindSubmatch(m)
		return fmt.Appendf(nil, `%sd="%s"`, p[1], arcosConHolgura(string(p[2])))
	})
	return rectSinRy.ReplaceAllFunc(svg, func(m []byte) []byte {
		if yaTieneRy.Match(m) {
			return m
		}
		p := rectSinRy.FindSubmatch(m)
		return fmt.Appendf(nil, `%s ry="%s"%s%s`, p[1], p[2], p[3], p[4])
	})
}

// grosorEnGrupo multiplica los `stroke-width` explícitos de un grupo con `scale` por la
// escala del grupo. Solo los de DENTRO: el del propio `<g>` está en el sistema de fuera.
func grosorEnGrupo(g []byte) []byte {
	m := grupoEscalado.FindSubmatch(g)
	f := 1.0
	for _, e := range escalaDeGrupo.FindAllSubmatch(m[1], -1) {
		sx, _ := strconv.ParseFloat(string(e[1]), 64)
		sy, _ := strconv.ParseFloat(string(e[2]), 64)
		f *= math.Sqrt(math.Abs(sx * sy))
	}
	if f == 1 {
		return g
	}
	abre := bytes.IndexByte(g, '>') + 1
	cuerpo := grosorExplicit.ReplaceAllFunc(g[abre:], func(w []byte) []byte {
		n, _ := strconv.ParseFloat(string(grosorExplicit.FindSubmatch(w)[1]), 64)
		return fmt.Appendf(nil, `stroke-width="%.4g"`, n*f)
	})
	return append(append([]byte{}, g[:abre]...), cuerpo...)
}

// # Los `var(--sytue-*)` de branding/, resueltos AL LEER
//
// El line-art de `branding/illustrations` se dibuja entero con roles de la paleta
// —`stroke="var(--sytue-neutral-400)"`— para que conmute solo entre claro y oscuro. En
// el navegador eso funciona porque el SVG va EN LÍNEA en el documento y las variables
// resuelven contra el tema activo (es lo que hace `Illustration.tsx`, y lo que no hacía
// un `<img src>`, que dejaba el dibujo invisible).
//
// `oksvg` no sabe nada de variables CSS: un `stroke="var(...)"` no es que lo pinte del
// color equivocado, es que **aborta el fichero entero**, y las trece ilustraciones no se
// podían usar. Las que llevan un valor de reserva —`var(--sytue-product-mail, #dc3450)`—
// tampoco se salvan, porque el problema es la sintaxis.
//
// Se corrige AQUÍ y no en `branding/`, que es la fuente compartida con la web y no se
// toca: la misma decisión que con `ry` y `scale(k)`, con el mismo guardián que enseña el
// estropicio si se quita.
var varCSS = regexp.MustCompile(`var\(\s*--sytue-([a-z0-9-]+)\s*(?:,\s*([^)]*?)\s*)?\)`)

// conTokens sustituye cada `var(--sytue-X)` por el hex del token X en esa variante.
//
// Si el nombre no está en la hoja se usa el valor de reserva que traiga el propio
// `var()`, que es lo que haría el navegador; y si tampoco lo hay, se deja tal cual para
// que `rasteriza` entre en pánico nombrando el fichero. Un trazo que desaparece en
// silencio es peor que un error.
func conTokens(svg []byte, v tema.Variante) []byte {
	return varCSS.ReplaceAllFunc(svg, func(m []byte) []byte {
		p := varCSS.FindSubmatch(m)
		nombre := string(p[1])
		if conocidos[nombre] {
			c := tema.Color(nombre, v)
			return fmt.Appendf(nil, "#%02x%02x%02x", c.R, c.G, c.B)
		}
		if len(p[2]) > 0 {
			return p[2]
		}
		return m
	})
}

// conocidos es el conjunto de tokens de la hoja, para poder preguntar sin que
// `tema.Color` entre en pánico.
var conocidos = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range tokens.Names() {
		m[n] = true
	}
	return m
}()

// El `width` y el `height` del `<svg>`, para saber a qué tamaño natural va una
// ilustración sin escribirlo a mano en dos sitios.
var anchoAlto = regexp.MustCompile(`<svg\b[^>]*?\bwidth="([0-9.]+)"[^>]*?\bheight="([0-9.]+)"`)

func medidaSVG(svg []byte) (float32, float32) {
	p := anchoAlto.FindSubmatch(svg)
	if p == nil {
		return 0, 0
	}
	w, _ := strconv.ParseFloat(string(p[1]), 32)
	h, _ := strconv.ParseFloat(string(p[2]), 32)
	return float32(w), float32(h)
}

// Dibujo es un icono o una marca ya colocable en un renderer. Por dentro es un
// `canvas.Raster`, que es la única primitiva de Fyne a la que se le pide el mapa de
// bits en píxeles de DISPOSITIVO.
//
// No ES un `fyne.CanvasObject`, lo TIENE, igual que `ui.Caja`: el pintor de Fyne
// reconoce sus primitivas con un `switch` de tipo (`case *canvas.Raster:`), así que un
// tipo propio que EMBEBA el raster no casa con ninguna rama y no se pinta nada. Por
// eso el objeto se entrega con `Objeto()` y va a los `Objects()` del renderer.
//
// Se rasteriza una vez por (tamaño en píxeles, color) y se guarda: Fyne llama al
// generador en cada repintado, y volver a interpretar el SVG cada vez cuesta entre
// 0,15 ms (16 px) y 0,7 ms (40 px) medidos en el piloto — poco para uno, mucho para
// una bandeja con doscientas filas.
type Dibujo struct {
	raster *canvas.Raster

	svg   func(pintura) []byte // cómo se arma el SVG con la pintura de turno
	tinta color.NRGBA
	// acento es el relleno de las formas cerradas si lo ha fijado quien usa el icono
	// (`Acento`); acentoTinta, que el relleno es la propia tinta (`AcentoTinta`). Sin
	// ninguno de los dos vale el primario del tema, que se mira al pintar.
	acento      *color.NRGBA
	acentoTinta bool
	// carril: el icono va DENTRO del menú lateral de color y su relleno es el ámbar de
	// allí (`sidebar-indicator`), mirado al pintar para que siga al tema (`EnCarril`).
	carril bool

	mu     sync.Mutex
	w, h   int
	deQue  pintura
	ultimo *image.NRGBA
	// apagado es el `opacity-40 grayscale` de una tesela apagada; apagadoDe, con qué
	// valor se hizo `ultimo`.
	apagado, apagadoDe bool
	// giro son los grados del `rotate()` que se le aplica al mapa YA rasterizado (ver
	// Girar). Cero: derecho.
	giro float32
}

func nuevoDibujo(lado float32, tinta color.NRGBA, svg func(pintura) []byte) *Dibujo {
	d := &Dibujo{svg: svg, tinta: tinta}
	d.raster = nuevoRaster(d)
	d.raster.Resize(fyne.NewSize(lado, lado))
	// Y ADEMÁS su MÍNIMO, que no es lo mismo. Un `canvas.Raster` nace con `MinSize` a
	// cero, así que un layout que coloque a sus hijos por el mínimo —`ui.Vacio` con su
	// ilustración, `ui.Fila`, `ui.Columna`— le daba 0×0 y el icono salía como un punto.
	// Dentro del renderer de un widget no se notaba porque allí se le llama a `Colocar`
	// con el lado a mano.
	//
	// Estaba ya visto y parcheado A MANO en `apps/hub` (`piezas.Glifo` hacía este
	// mismo `SetMinSize`), y en `apps/drive` no, así que sus estados vacíos pintaban la
	// ilustración invisible. Arreglarlo aquí lo cierra para los tres sitios, que es lo
	// que dice `docs/CONVENTIONS.md` §13.2: si no encaja, se amplía.
	d.raster.SetMinSize(fyne.NewSize(lado, lado))
	return d
}

// nuevoRaster es la primitiva del dibujo, aparte porque una ilustración no es cuadrada
// y se construye con su propio tamaño.
func nuevoRaster(d *Dibujo) *canvas.Raster {
	r := canvas.NewRaster(d.generar)
	r.ScaleMode = canvas.ImageScalePixels
	// Se apunta de quién es, para `De`. Con punteros DÉBILES en los dos sentidos: el
	// mapa no retiene ni el dibujo ni su primitiva, y la entrada se va cuando el dibujo
	// se recoge.
	clave := weak.Make(r)
	deRaster.Store(clave, weak.Make(d))
	runtime.AddCleanup(d, func(k weak.Pointer[canvas.Raster]) { deRaster.Delete(k) }, clave)
	return r
}

// deRaster dice de qué Dibujo es cada primitiva: weak.Pointer[canvas.Raster] →
// weak.Pointer[Dibujo].
var deRaster sync.Map

// De devuelve el Dibujo al que pertenece una primitiva (la que entrega `Objeto`), o nil
// si no es de ninguno. Es para quien recibe el icono ya hecho —una fila del menú, a la
// que su app le pasa `Objeto()`— y tiene que teñirlo con su estado, como hace
// `currentColor` en la web.
func De(o fyne.CanvasObject) *Dibujo {
	r, ok := o.(*canvas.Raster)
	if !ok {
		return nil
	}
	d, ok := deRaster.Load(weak.Make(r))
	if !ok {
		return nil
	}
	return d.(weak.Pointer[Dibujo]).Value() //nolint:errcheck,forcetypeassert // el mapa solo guarda eso
}

// Tinta es el color con el que está pintado ahora.
func (d *Dibujo) Tinta() color.NRGBA {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.tinta
}

// DelCarril dice si el dibujo es un icono del menú lateral: se construyó con una tinta
// del carril (ver DeToken y EnCarril).
func (d *Dibujo) DelCarril() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.carril
}

// Objeto es la primitiva que hay que meter en los `Objects()` del renderer.
func (d *Dibujo) Objeto() fyne.CanvasObject { return d.raster }

// Ver enseña o esconde el dibujo sin quitarlo del renderer, que es lo que hace un tic
// de casilla al marcarse y desmarcarse.
func (d *Dibujo) Ver(si bool) {
	if si {
		d.raster.Show()
		return
	}
	d.raster.Hide()
}

func (d *Dibujo) generar(w, h int) image.Image {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := pintura{tinta: d.tinta, acento: d.acentoDeAhora(), lado: d.raster.Size().Width}
	if d.ultimo == nil || d.w != w || d.h != h || d.deQue != p || d.apagadoDe != d.apagado {
		d.ultimo, d.w, d.h, d.deQue, d.apagadoDe = rasteriza(d.svg(p), w, h), w, h, p, d.apagado
		if d.apagado {
			apagar(d.ultimo)
		}
	}
	if d.giro != 0 {
		return rotar(d.ultimo, d.giro)
	}
	return d.ultimo
}

// acentoDeAhora es el color de la capa de relleno en este repintado. Va con el cerrojo
// ya cogido.
//
// El de la casa se mira AL PINTAR y no al construir: es el primario del tema, y así un
// cambio de tema lo arrastra sin que cada uno de los ~280 sitios que construyen un
// icono tenga que acordarse de él. La tinta sí se resuelve al construir porque la
// elige quien pone el icono; el acento por defecto no lo elige nadie.
//
// Y hay una excepción que tampoco tiene que recordar nadie, la misma de `styles.css` y
// reconocida igual, por la TINTA: quien pinta un icono en blanco o en `on-primary` lo
// está poniendo sobre un relleno de color —el tic de un paso, el botón flotante, la
// tesela de un producto—, y ahí el relleno es la propia tinta. El blanco puro no es la
// tinta de texto de ningún tema (en oscuro es #f0f0f0), así que no hay confusión.
func (d *Dibujo) acentoDeAhora() color.NRGBA {
	switch {
	case d.acentoTinta:
		return d.tinta
	case d.acento != nil:
		return *d.acento
	}
	v := tema.Claro
	if app := fyne.CurrentApp(); app != nil {
		v = tema.De(app.Settings().ThemeVariant())
	}
	// DENTRO DEL CARRIL el relleno es su ámbar: es el `--sytue-glyph-fill` que
	// appearance.css reasigna para todo lo que cuelga de `bg-sidebar`. Va ANTES que la
	// regla del blanco, porque en claro la tinta del carril ES el blanco. Lo dice quien
	// construye el icono (`EnCarril`, o el token `sidebar-*` de `Trazo`); la tinta
	// atenuada del carril, que no coincide con ninguna otra de la hoja, se reconoce sola.
	if d.carril || d.tinta == tema.Color("sidebar-fg-muted", v) {
		return tema.Color("sidebar-indicator", v)
	}
	if d.tinta == (color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}) || d.tinta == tema.Color("on-primary", v) {
		return d.tinta
	}
	// Y la otra mitad de la regla de `styles.css`: con la tinta YA de color —rojo de
	// peligro, verde de éxito, el color de un producto— el relleno sigue a la tinta.
	if tintaDeColor(d.tinta) {
		return d.tinta
	}
	return tema.Color("primary", v)
}

// croma por encima de la cual una tinta se da por «de color».
const cromaDeColor = 40

// tintaDeColor dice si una tinta es de color o neutra, por su croma: la distancia entre
// su canal más alto y el más bajo. La web lo sabe por la utilidad con que se escribe
// (`text-danger-text`…); aquí la tinta llega ya resuelta a un color, y lo que distingue
// a un rojo de peligro de la tinta de texto es justo eso.
//
// Medido sobre la hoja: las tintas neutras de los tres temas (`text`, `text-muted`,
// `text-subtle`, `sidebar-fg*`, `header-fg`) quedan entre 0 y 17; las de estado y las de
// producto, de 80 para arriba. `primary-text` también es «de color», y da lo mismo: su
// relleno es el de marca en los dos casos.
//
// ponytail: un umbral y no la lista de tokens, porque la tinta puede venir mezclada o
// atenuada y ya no ser igual a ningún token. Si un tema nuevo trae una tinta neutra muy
// teñida, o un color de estado muy apagado, se mide y se mueve el 40.
func tintaDeColor(c color.NRGBA) bool {
	return int(max(c.R, c.G, c.B))-int(min(c.R, c.G, c.B)) > cromaDeColor
}

// Acento fija el color del relleno de las formas cerradas: es poner
// `--sytue-glyph-fill` en la web. Lo usa quien coloca el icono sobre un fondo donde
// el primario no es el acento —el carril de color, con su ámbar—. Devuelve el propio
// dibujo para encadenarlo al construir.
func (d *Dibujo) Acento(c color.NRGBA) *Dibujo {
	d.mu.Lock()
	d.acento, d.acentoTinta = &c, false
	d.mu.Unlock()
	d.raster.Refresh()
	return d
}

// EnCarril dice que el icono va dentro del menú lateral de color: el relleno de sus
// formas cerradas pasa al ámbar del carril y sigue al tema. Por la tinta no siempre se
// puede saber —la del carril es blanco puro en claro, que es también la de un icono
// sobre un botón de color, y en oscuro es la misma que la del texto—, así que lo dice
// quien lo pone. `Trazo` y `TrazoLleno` lo hacen solos con un token `sidebar-*`.
func (d *Dibujo) EnCarril() *Dibujo {
	d.mu.Lock()
	d.acento, d.acentoTinta, d.carril = nil, false, true
	d.mu.Unlock()
	d.raster.Refresh()
	return d
}

// DeToken dice de qué token es la tinta del dibujo: si es uno del menú lateral
// (`sidebar-fg`, `sidebar-fg-muted`, `sidebar-active-fg`) va dentro del carril, y si no,
// fuera. Es para los constructores que reciben el token y pintan con su color —los de
// este paquete y los `Glifo` propios de cada app— y para el control que cambia de token
// según dónde esté (`ui.BotonIcono`).
func (d *Dibujo) DeToken(token string) *Dibujo {
	carril := strings.HasPrefix(token, "sidebar-")
	d.mu.Lock()
	cambia := d.carril != carril
	d.carril = carril
	d.mu.Unlock()
	if cambia {
		d.raster.Refresh()
	}
	return d
}

// AcentoTinta hace que el relleno sea la propia tinta, el `currentColor` de la web: es
// lo que lleva un icono SOBRE un relleno de color, donde el primario encima del
// primario no se vería. Con tinta blanca o `on-primary` ya sale solo (ver
// `acentoDeAhora`); esto es para quien la atenúa o la mezcla —un botón relleno
// inerte—, que deja de ser reconocible por el color. Sigue a la tinta si se tiñe.
func (d *Dibujo) AcentoTinta() *Dibujo {
	d.mu.Lock()
	d.acento, d.acentoTinta = nil, true
	d.mu.Unlock()
	d.raster.Refresh()
	return d
}

// Girar es el `transform: rotate(grados)` del icono: el `animate-spin` del «Actualizar»
// de Sytue Mail mientras carga. Se gira el mapa YA rasterizado, sin volver a pasar por
// el SVG, que es también lo que hace el navegador: pinta la capa una vez y la rota al
// componer. Así un icono que gira cuesta una rotación de 32×32 por fotograma y no un
// `oksvg` entero.
func (d *Dibujo) Girar(grados float32) {
	d.mu.Lock()
	cambia := d.giro != grados
	d.giro = grados
	d.mu.Unlock()
	if cambia {
		d.raster.Refresh()
	}
}

// rotar gira `img` alrededor de su centro, en el sentido del reloj como `rotate()` de
// CSS, con interpolación bilineal sobre el color PREMULTIPLICADO (como compone el
// navegador): sin premultiplicar, el borde suavizado se oscurece al mezclarse con el
// negro de los píxeles transparentes.
func rotar(img *image.NRGBA, grados float32) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(b)
	rad := float64(grados) * math.Pi / 180
	sen, cos := math.Sin(rad), math.Cos(rad)
	cx, cy := float64(w)/2, float64(h)/2
	muestra := func(x, y int) (r, g, bl, a float64) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return 0, 0, 0, 0
		}
		p := img.Pix[img.PixOffset(b.Min.X+x, b.Min.Y+y):]
		a = float64(p[3]) / 255
		return float64(p[0]) * a, float64(p[1]) * a, float64(p[2]) * a, a
	}
	for y := range h {
		for x := range w {
			// El píxel de destino, llevado hacia atrás por la rotación inversa.
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			sx, sy := dx*cos+dy*sen+cx-0.5, -dx*sen+dy*cos+cy-0.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(x0), sy-float64(y0)
			var r, g, bl, a float64
			for _, v := range [4]struct {
				x, y int
				peso float64
			}{{x0, y0, (1 - fx) * (1 - fy)}, {x0 + 1, y0, fx * (1 - fy)}, {x0, y0 + 1, (1 - fx) * fy}, {x0 + 1, y0 + 1, fx * fy}} {
				mr, mg, mb, ma := muestra(v.x, v.y)
				r, g, bl, a = r+mr*v.peso, g+mg*v.peso, bl+mb*v.peso, a+ma*v.peso
			}
			if a <= 0 {
				continue
			}
			o := out.Pix[out.PixOffset(b.Min.X+x, b.Min.Y+y):]
			o[0], o[1], o[2], o[3] = uint8(min(r/a, 255)+0.5), uint8(min(g/a, 255)+0.5), uint8(min(bl/a, 255)+0.5), uint8(min(a*255, 255)+0.5)
		}
	}
	return out
}

// Apagar es el `opacity-40 grayscale` con el que la web pinta la tesela de un producto
// que esta cuenta no puede usar (AppGrid.tsx y el lanzador del Hub). Fyne no tiene
// filtros de imagen, así que se aplica al mapa de bits del dibujo.
func (d *Dibujo) Apagar(si bool) {
	d.mu.Lock()
	cambia := d.apagado != si
	d.apagado = si
	d.mu.Unlock()
	if cambia {
		d.raster.Refresh()
	}
}

// apagar deja la imagen en gris y al 40 %. El gris es la matriz de `grayscale(1)` de
// Filter Effects, aplicada a los canales sRGB tal cual, que es como lo hacen Chromium y
// WebKit con el atajo de CSS; la opacidad multiplica el alfa, que sobre un dibujo de una
// sola capa es lo mismo que la opacidad de grupo de la web.
func apagar(img *image.NRGBA) {
	for i := 0; i+3 < len(img.Pix); i += 4 {
		p := img.Pix[i : i+4 : i+4]
		g := uint8(min(0.2126*float64(p[0])+0.7152*float64(p[1])+0.0722*float64(p[2])+0.5, 0xff))
		p[0], p[1], p[2], p[3] = g, g, g, uint8(float64(p[3])*0.4+0.5)
	}
}

// Tenir cambia el color sin reconstruir el objeto: es lo que llama un renderer en su
// Refresh cuando el puntero entra o sale, igual que `ui.Caja.Pintar`.
func (d *Dibujo) Tenir(token string, v tema.Variante) { d.TenirCon(tema.Color(token, v)) }

// TenirCon es Tenir con un color ya calculado. Es la salida de emergencia para los
// colores DERIVADOS de un token —el 50 % de opacidad de un control inerte, por
// ejemplo—, que no tienen token propio en la hoja. Para todo lo demás va Tenir: un
// color suelto que no salga de `tema.Color` es un color fuera de la paleta.
func (d *Dibujo) TenirCon(c color.NRGBA) {
	d.mu.Lock()
	cambia := c != d.tinta
	d.tinta = c
	d.mu.Unlock()
	if cambia {
		d.raster.Refresh()
	}
}

// Cambiar sustituye el DIBUJO y su color sin rehacer el objeto ni tocar el árbol del
// renderer. Es el galón de una sección plegable, que pasa de apuntar a la derecha a
// apuntar hacia abajo al abrirse: en la web es el mismo `<Icon>` con otro `name`, no
// una rotación.
func (d *Dibujo) Cambiar(i Icono, c color.NRGBA) {
	ruta := catalogo[i].d
	if ruta == "" {
		panic(fmt.Sprintf("iconos: %s no tiene ruta en el catálogo (¿Ninguno?)", i))
	}
	d.mu.Lock()
	d.svg = func(p pintura) []byte { return svgDeTrazo(ruta, p) }
	d.tinta, d.ultimo = c, nil // invalidar: el mapa guardado es del icono anterior
	d.mu.Unlock()
	d.raster.Refresh()
}

// Colocar pone el dibujo en (x, y) con el lado pedido. Los iconos son cuadrados en la
// web —`<svg width={size} height={size}>`— y aquí también.
func (d *Dibujo) Colocar(x, y, lado float32) {
	d.ColocarCaja(x, y, fyne.NewSize(lado, lado))
}

// ColocarCaja es Colocar para lo que NO es cuadrado: una ilustración es 200×160.
func (d *Dibujo) ColocarCaja(x, y float32, tam fyne.Size) {
	d.raster.Move(fyne.NewPos(x, y))
	d.raster.Resize(tam)
}

// Medida es lo que ocupa el dibujo ahora mismo. Lo necesita quien coloca una
// ilustración, cuyo tamaño natural lo trae el propio SVG.
func (d *Dibujo) Medida() fyne.Size { return d.raster.Size() }

// DeSVG es un dibujo de un SVG ENTERO que arma quien llama, del tamaño `tam`.
//
// Es lo que la web pinta como `<svg>` en línea con el viewBox en píxeles de SU caja: las
// formas, los gráficos, los diagramas y los conectores de una diapositiva. Pasa por la
// misma maquinaria que los iconos —píxeles de DISPOSITIVO, las correcciones de `oksvg`,
// la caché por tamaño— y el color va DENTRO del SVG, así que `Tenir` no lo cambia.
//
// `oksvg` no pinta `<text>` y aborta el fichero si lo encuentra: el texto de un gráfico
// o de un diagrama lo pone quien llama encima, con `canvas.Text`.
func DeSVG(svg []byte, tam fyne.Size) *Dibujo {
	d := &Dibujo{svg: func(pintura) []byte { return svg }}
	d.raster = nuevoRaster(d)
	d.raster.Resize(tam)
	d.raster.SetMinSize(tam)
	return d
}

// Trazo es un icono del catálogo compartido, del tamaño y el color de token pedidos.
//
// El token se resuelve AQUÍ y no al pintar: uno que no exista revienta al armar el
// widget, no la primera vez que se abre esa pantalla. Es la diferencia que costó el
// `sidebar-hover` de Gio.
func Trazo(i Icono, lado float32, token string, v tema.Variante) *Dibujo {
	return TrazoConTinta(i, lado, tema.Color(token, v)).DeToken(token)
}

// TrazoConTinta es Trazo con un color ya calculado; ver `Dibujo.TenirCon` para cuándo
// vale usarlo.
func TrazoConTinta(i Icono, lado float32, c color.NRGBA) *Dibujo {
	d := catalogo[i].d
	if d == "" {
		panic(fmt.Sprintf("iconos: %s no tiene ruta en el catálogo (¿Ninguno?)", i))
	}
	return nuevoDibujo(lado, c, func(p pintura) []byte { return svgDeTrazo(d, p) })
}

// TrazoLleno es Trazo con la forma RELLENA además de trazada: la estrella de un
// correo destacado. En la web es el mismo glifo con `filled`, no otro dibujo.
func TrazoLleno(i Icono, lado float32, token string, v tema.Variante) *Dibujo {
	return TrazoLlenoConTinta(i, lado, tema.Color(token, v)).DeToken(token)
}

// TrazoLlenoConTinta es TrazoLleno con un color ya calculado; ver `Dibujo.TenirCon`
// para cuándo vale usarlo.
func TrazoLlenoConTinta(i Icono, lado float32, c color.NRGBA) *Dibujo {
	d := catalogo[i].d
	if d == "" {
		panic(fmt.Sprintf("iconos: %s no tiene ruta en el catálogo (¿Ninguno?)", i))
	}
	return nuevoDibujo(lado, c, func(p pintura) []byte { return svgDe(d, p, true) })
}

// String hace legible el icono en los mensajes de error y en los fallos de los tests:
// sin él, un `%v` de un Icono imprime un número que no dice nada.
func (i Icono) String() string {
	if int(i) >= len(catalogo) {
		return fmt.Sprintf("Icono(%d)", uint8(i))
	}
	if i == Ninguno {
		return "Ninguno"
	}
	return catalogo[i].clave
}

// ---- Los glifos PROPIOS de una app ----

// TrazoDeRuta es Trazo con una ruta SVG en vez de una constante del catálogo.
//
// # Por qué existe, y por qué no es una puerta trasera
//
// Cada app de la suite tiene glifos que `packages/ui/src/components/Icon.tsx` NO
// declara y que viven dentro de la propia app en la web: los ocho del triaje de Mail,
// la estrella y la tarta de Contacts, los ocho de Notepads. Son ~22 en total y están
// contados en `docs/roadmap.md`.
//
// La alternativa era meterlos en `catalogo` y ampliar `TestElCatalogoEsElDeLaWeb` para
// que mirase además los `.tsx` de cada app. Se descartó por dos motivos MEDIDOS en
// este repositorio: el guardián compara en los dos sentidos —una ruta que sobre aquí
// lo pone en rojo—, así que cada app que añadiera el suyo tendría que tocar la lista
// compartida, y hay varias portándose a la vez; y el catálogo dejaría de ser el espejo
// EXACTO de un solo fichero, que es justo lo que lo hace comprobable.
//
// Así cada app guarda sus rutas en su paquete —donde la web también las tiene— y lo
// compartido es la maquinaria: el rasterizado a píxeles de DISPOSITIVO (no un mapa de
// bits estirado), las tres correcciones de `oksvg` y el teñido sin rehacer la imagen.
// Eso es ampliar el catálogo en vez de copiarlo.
func TrazoDeRuta(d string, lado float32, c color.NRGBA) *Dibujo {
	if d == "" {
		panic("iconos: TrazoDeRuta sin ruta")
	}
	return nuevoDibujo(lado, c, func(p pintura) []byte { return svgDeTrazo(d, p) })
}

// TrazoDeCuerpo es TrazoDeRuta para un glifo que NO es un solo `<path>`.
//
// Los de Sytue Rooms llevan `<rect>` y `<circle>` —la cámara de «Llamar», el reloj de
// los programados, la cara del selector de emoji, el «!» de prioridad—, y una `d` no
// los expresa. Convertirlos a mano a un camino con arcos sería exactamente el
// «redibujado a ojo» que este paquete existe para impedir: la esquina de un `rect rx`
// y el arco de un `circle` los pinta `oksvg` igual que el navegador, y a ojo no.
//
// `cuerpo` se copia TAL CUAL del `.tsx` —los hijos del `<svg>`, sin el envoltorio— y el
// trazo, el color y las uniones van en el `<g>` que los envuelve, que es donde la web
// también los pone (el componente `Glyph` de cada app). Quien lo use deja un guardián
// que compare su cadena contra ese `.tsx`, como hace `apps/rooms/glifos_test.go`.
func TrazoDeCuerpo(cuerpo string, lado float32, c color.NRGBA) *Dibujo {
	if cuerpo == "" {
		panic("iconos: TrazoDeCuerpo sin cuerpo")
	}
	return nuevoDibujo(lado, c, func(p pintura) []byte { return svgDeCuerpo(cuerpo, p) })
}

// svgDeCuerpo envuelve los hijos con los mismos atributos que `svgDeTrazo` le pone al
// `<path>`, solo que en un `<g>` para que los hereden todos, y con la misma capa de
// relleno debajo.
func svgDeCuerpo(cuerpo string, p pintura) []byte {
	c := p.tinta
	return fmt.Appendf(nil,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">%s`+
			`<g fill="none" stroke="#%02x%02x%02x" stroke-opacity="%.4f" stroke-width="%g"`+
			` stroke-linecap="round" stroke-linejoin="round">%s</g>%s</svg>`,
		rejilla, rejilla, rejilla, rejilla, capaDeAcento(rellenoDeCuerpo(cuerpo), p),
		c.R, c.G, c.B, float32(c.A)/255, grosorTrazo(p.lado), cuerpo,
		capaDePuntos(puntosDeCuerpo(cuerpo), c))
}

// TrazoDeRutaRelleno es TrazoDeRuta con el interior RELLENO además de trazado, que es
// el `fill="currentColor"` de la estrella de un contacto destacado. Con el mismo
// color en los dos, que es lo que hace `currentColor`.
func TrazoDeRutaRelleno(d string, lado float32, c color.NRGBA) *Dibujo {
	if d == "" {
		panic("iconos: TrazoDeRutaRelleno sin ruta")
	}
	return nuevoDibujo(lado, c, func(p pintura) []byte { return svgRelleno(d, p) })
}

// svgRelleno es svgDeTrazo con `fill` en vez de `fill="none"`, y por eso sin capa de
// acento.
func svgRelleno(d string, p pintura) []byte {
	c := p.tinta
	return fmt.Appendf(nil,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`+
			`<path d="%s" fill="#%02x%02x%02x" fill-opacity="%.4f" stroke="#%02x%02x%02x"`+
			` stroke-opacity="%.4f" stroke-width="%g" stroke-linecap="round"`+
			` stroke-linejoin="round"/></svg>`,
		rejilla, rejilla, rejilla, rejilla, d,
		c.R, c.G, c.B, float32(c.A)/255, c.R, c.G, c.B, float32(c.A)/255, grosorTrazo(p.lado))
}
