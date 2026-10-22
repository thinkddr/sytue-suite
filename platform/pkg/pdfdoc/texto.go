// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"git.sytue.eu/sytue/one/platform/pkg/pdffuente"
)

// Alineacion de un párrafo. «Justificado» NO existe: con Identity-H el operador Tw no
// actúa (solo vale para el código de un byte 32), y repartir el espacio a mano pide
// partir cada Tj. ponytail: se alinea a la izquierda; justificar cuando un cliente lo pida.
type Alineacion int

const (
	Izquierda Alineacion = iota
	Centro
	Derecha
)

// Fragmento es un tramo de texto con su formato de carácter.
type Fragmento struct {
	Texto                                string
	Negrita, Cursiva, Subrayado, Tachado bool
	Color                                string // «RRGGBB»; vacío = negro (azul y subrayado si es un enlace)
	Enlace                               string // destino saliente; solo http, https, mailto y tel
	Idioma                               string // BCP 47 del tramo si no es el del documento
}

// Parrafo es un bloque de texto con ajuste de línea por palabras.
type Parrafo struct {
	Fragmentos []Fragmento
	Rol        string  // "" = P; «H1»…«H6» para encabezados (la etiqueta se normaliza, el tamaño no)
	Tam        float64 // cuerpo en puntos; 0 = 11
	Alinear    Alineacion
	Sangria    float64 // margen izquierdo extra en puntos
	Despues    float64 // espacio tras el párrafo
	Cita       bool    // BlockQuote con barra lateral
}

type glifo struct {
	g uint16
	r rune
}

// estilo es lo que comparten las piezas de un fragmento: el elemento de estructura que
// posee su contenido marcado y cómo se pintan.
type estilo struct {
	owner      *Elem
	color      [3]float64
	conColor   bool
	subr, tach bool
	enlace     *enlace
}

// pieza es un tramo sin espacios de una sola cara; una palabra puede ser varias piezas si
// parte de sus letras salen de la fuente de reserva.
type pieza struct {
	cara    *caraUsada
	est     *estilo
	glifos  []glifo
	ancho   float64
	espacio bool
	salto   bool
	final   bool // espacio añadido al final de una línea que continúa: no se subraya ni cuenta
}

type linea struct {
	piezas []pieza
	ancho  float64
}

const interlineado = 1.4

// Parrafo maqueta y pinta un párrafo, paginando línea a línea.
func (d *Doc) Parrafo(p Parrafo) {
	d.listas = nil
	padre := d.contenedor()
	if p.Cita {
		padre = d.nuevo("BlockQuote", padre, true)
		p.Sangria += 18
	}
	tipo := "P"
	if strings.HasPrefix(p.Rol, "H") {
		tipo = d.tipoCabecera(p.Rol)
	}
	d.escribir(padre, tipo, p, nil)
}

// marcador es la etiqueta (Lbl) de un elemento de lista, colgada a la izquierda.
type marcador struct {
	lbl *Elem
	txt string
	x   float64
}

type nivelLista struct {
	l, lbody *Elem
	ordenada bool
	n        int
}

const sangriaLista = 18.0

// ItemLista escribe un elemento de lista de un nivel (0 = el primero). La estructura
// L › LI › (Lbl, LBody) se arma sola a partir de la lista plana: un nivel más hondo anida
// su L dentro del LBody del elemento anterior.
func (d *Doc) ItemLista(nivel int, ordenada bool, p Parrafo) {
	nivel = max(0, min(nivel, 8))
	if len(d.listas) > nivel+1 {
		d.listas = d.listas[:nivel+1]
	}
	if len(d.listas) == nivel+1 && d.listas[nivel].ordenada != ordenada {
		d.listas = d.listas[:nivel]
	}
	for len(d.listas) <= nivel {
		padre := d.contenedor()
		if n := len(d.listas); n > 0 {
			if d.listas[n-1].lbody == nil { // nivel intermedio sin elemento propio: uno vacío
				li := d.nuevo("LI", d.listas[n-1].l, true)
				d.listas[n-1].lbody = d.nuevo("LBody", li, true)
			}
			padre = d.listas[n-1].lbody
		}
		d.listas = append(d.listas, &nivelLista{l: d.nuevo("L", padre, true), ordenada: ordenada})
	}
	top := d.listas[nivel]
	top.n++
	li := d.nuevo("LI", top.l, true)
	txt := "•"
	if ordenada {
		txt = strconv.Itoa(top.n) + "."
	}
	mk := &marcador{lbl: d.nuevo("Lbl", li, true), txt: txt, x: Margin + p.Sangria + float64(nivel)*sangriaLista}
	top.lbody = d.nuevo("LBody", li, true)
	p.Sangria += float64(nivel+1) * sangriaLista
	d.escribir(top.lbody, "P", p, mk)
}

// Regla dibuja una línea horizontal (un artefacto: no es contenido).
func (d *Doc) Regla() {
	d.listas = nil
	if d.y-12 < Margin {
		d.NuevaPagina()
	}
	d.y -= 6
	y := d.y
	d.artefacto(func(b *strings.Builder) {
		fmt.Fprintf(b, "q 0.6 G 0.75 w %s %s m %s %s l S Q\n", ftoa(Margin), ftoa(y), ftoa(PageW-Margin), ftoa(y))
	})
	d.y -= 6
}

func tieneTexto(frs []Fragmento) bool {
	for _, fr := range frs {
		for _, r := range fr.Texto {
			if r != ' ' && r != '\t' && r != '\n' && !sinContenido(r) {
				return true
			}
		}
	}
	return false
}

func (d *Doc) escribir(padre *Elem, tipo string, p Parrafo, mk *marcador) {
	tam := p.Tam
	if tam == 0 {
		tam = 11
	}
	alto := tam * interlineado
	if !tieneTexto(p.Fragmentos) && mk == nil {
		d.y -= alto + p.Despues
		return
	}
	var pe *Elem
	var lineas []linea
	ancho := PageW - 2*Margin - p.Sangria
	if tieneTexto(p.Fragmentos) {
		pe = d.nuevo(tipo, padre, true)
		lineas = d.romper(d.piezas(p.Fragmentos, tam, pe), tam, ancho)
	} else {
		lineas = []linea{{}}
	}
	for i, l := range lineas {
		need := alto
		if i == 0 && tipo != "P" {
			need = alto * 3 // un encabezado no se queda solo al pie de la página
		}
		if d.y-need < Margin {
			d.NuevaPagina()
		}
		x := Margin + p.Sangria
		switch p.Alinear {
		case Izquierda:
			// x ya es el margen
		case Centro:
			x += (ancho - l.ancho) / 2
		case Derecha:
			x += ancho - l.ancho
		}
		base := d.y - tam*1.05
		if i == 0 && mk != nil {
			d.pintar(linea{piezas: d.piezas([]Fragmento{{Texto: mk.txt}}, tam, mk.lbl), ancho: 0}, mk.x, base, tam)
		}
		if i < len(lineas)-1 {
			l = d.conEspacioFinal(l, tam)
		}
		d.pintar(l, x, base, tam)
		if p.Cita {
			top := d.y
			d.artefacto(func(b *strings.Builder) {
				fmt.Fprintf(b, "q 0.7 G 2 w %s %s m %s %s l S Q\n", ftoa(x-8), ftoa(top), ftoa(x-8), ftoa(top-alto))
			})
		}
		d.y -= alto
	}
	d.y -= p.Despues
}

// conEspacioFinal añade un espacio real al final de una línea que continúa en la
// siguiente: sin él un lector de pantalla junta la última palabra de una línea con la
// primera de la otra. No cuenta para el ancho (no mueve la alineación).
func (d *Doc) conEspacioFinal(l linea, tam float64) linea {
	n := len(l.piezas)
	if n == 0 {
		return l
	}
	u := l.piezas[n-1]
	f := u.cara.f
	g := f.Glifo(' ')
	if g == 0 {
		return l
	}
	w := float64(f.Avance(g)) * tam / 1000
	out := l
	out.piezas = append(slices.Clone(l.piezas), pieza{cara: u.cara, est: u.est, glifos: []glifo{{g, ' '}}, ancho: w, espacio: true, final: true})
	return out
}

// piezas convierte los fragmentos en piezas, cada carácter con la primera fuente de la
// pila que lo trae. Un carácter que ninguna trae se sustituye por «?» y se cuenta. padre es
// el elemento (P, H1, Lbl…) que posee el contenido; un enlace o un cambio de idioma crean
// su Link o Span como hijo suyo.
func (d *Doc) piezas(frs []Fragmento, tam float64, padre *Elem) []pieza {
	var out []pieza
	var prev *estilo
	var prevLang string
	for _, fr := range frs {
		est := d.estiloDe(fr, padre, prev, prevLang)
		prev, prevLang = est, fr.Idioma
		pila := pdffuente.Pila(fr.Negrita, fr.Cursiva)
		for _, r := range fr.Texto {
			if r == '\n' {
				out = append(out, pieza{salto: true})
				continue
			}
			if sinContenido(r) {
				continue
			}
			esp := r == ' ' || r == '\t'
			if esp {
				r = ' '
			}
			f, g, r := d.glifoDe(pila, r, esp, out, est)
			if g == 0 {
				continue
			}
			c := d.cara(f)
			w := float64(f.Avance(g)) * tam / 1000
			if n := len(out); n > 0 && !esp && !out[n-1].espacio && !out[n-1].salto && out[n-1].cara == c && out[n-1].est == est {
				out[n-1].glifos = append(out[n-1].glifos, glifo{g, r})
				out[n-1].ancho += w
				continue
			}
			out = append(out, pieza{cara: c, est: est, glifos: []glifo{{g, r}}, ancho: w, espacio: esp})
		}
	}
	return out
}

// estiloDe decide el elemento propietario y el aspecto de un fragmento. Fragmentos
// consecutivos del mismo enlace (o del mismo idioma) comparten elemento.
func (d *Doc) estiloDe(fr Fragmento, padre *Elem, prev *estilo, prevLang string) *estilo {
	est := &estilo{owner: padre, subr: fr.Subrayado, tach: fr.Tachado}
	if len(fr.Color) == 6 {
		if v, err := strconv.ParseUint(fr.Color, 16, 32); err == nil {
			est.color = [3]float64{float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255}
			est.conColor = true
		}
	}
	lang := d.idiomaDe(fr)
	if uri, ok := urlSegura(fr.Enlace); ok && fr.Enlace != "" {
		if prev != nil && prev.enlace != nil && prev.enlace.uri == uri {
			est.owner, est.enlace = prev.owner, prev.enlace
			est.enlace.texto += fr.Texto
		} else {
			est.owner, est.enlace = d.nuevo("Link", padre, false), &enlace{uri: uri, texto: fr.Texto}
			est.owner.lang = lang
		}
		if !est.conColor {
			est.color, est.conColor = [3]float64{0.04, 0.27, 0.62}, true
		}
		est.subr = true
	} else if lang != "" {
		if prev != nil && prev.owner != padre && prev.enlace == nil && prevLang == fr.Idioma {
			est.owner = prev.owner
		} else {
			est.owner = d.nuevo("Span", padre, false)
			est.owner.lang = lang
		}
	}
	return est
}

// idiomaDe devuelve el idioma del fragmento si difiere del idioma del documento: el que declara,
// si es un BCP 47 válido, o griego si el texto lo es en su mayoría (la única lengua de la
// UE que usa esa escritura). El cirílico no se adivina: puede ser búlgaro o ruso.
func (d *Doc) idiomaDe(fr Fragmento) string {
	l := strings.TrimSpace(fr.Idioma)
	if l != "" && reIdioma.MatchString(l) && !strings.EqualFold(l, d.op.Idioma) {
		return l
	}
	if l == "" && !strings.HasPrefix(d.op.Idioma, "el") {
		letras, griego := 0, 0
		for _, r := range fr.Texto {
			if unicode.IsLetter(r) {
				letras++
				if unicode.Is(unicode.Greek, r) {
					griego++
				}
			}
		}
		if letras > 0 && griego*2 > letras {
			return "el"
		}
	}
	return ""
}

// sinContenido: controles, guion blando y caracteres de ancho cero. No se pintan ni cuentan.
func sinContenido(r rune) bool {
	return r == 0xad || r == '\r' || (r >= 0x200b && r <= 0x200d) || r == 0x2060 || r == 0xfeff ||
		(r < 0x20 && r != '\t') || (r >= 0x7f && r < 0xa0)
}

// glifoDe elige cara y glifo de r. Un espacio va en la cara de la palabra vecina (menos
// cortes de cadena); un carácter sin glifo en ninguna fuente se cuenta y se sustituye por «?».
func (d *Doc) glifoDe(pila []*pdffuente.Fuente, r rune, esp bool, out []pieza, est *estilo) (*pdffuente.Fuente, uint16, rune) {
	if esp && len(out) > 0 && out[len(out)-1].est == est && out[len(out)-1].cara != nil {
		if g := out[len(out)-1].cara.f.Glifo(' '); g != 0 {
			return out[len(out)-1].cara.f, g, r
		}
	}
	f, g := buscar(pila, r)
	if g == 0 {
		d.avisos[r]++
		f, g = buscar(pila, '?')
		r = '?'
	}
	return f, g, r
}

func buscar(pila []*pdffuente.Fuente, r rune) (*pdffuente.Fuente, uint16) {
	for _, f := range pila {
		if g := f.Glifo(r); g != 0 {
			return f, g
		}
	}
	return nil, 0
}

// romper reparte las piezas en líneas que caben en ancho. Los espacios entre palabras que
// caen en el corte se descartan; una palabra más ancha que la línea se parte por glifos.
func (d *Doc) romper(ps []pieza, tam, ancho float64) []linea {
	var out []linea
	var cur linea
	var pend []pieza
	pendW := 0.0
	flush := func() {
		out = append(out, cur)
		cur, pend, pendW = linea{}, nil, 0
	}
	for i := 0; i < len(ps); {
		switch p := ps[i]; {
		case p.salto:
			flush()
			i++
		case p.espacio:
			if len(cur.piezas) > 0 {
				pend, pendW = append(pend, p), pendW+p.ancho
			}
			i++
		default:
			j, w := i, 0.0
			for j < len(ps) && !ps[j].espacio && !ps[j].salto {
				w += ps[j].ancho
				j++
			}
			palabra := ps[i:j]
			if len(cur.piezas) > 0 && cur.ancho+pendW+w > ancho {
				out = append(out, cur)
				cur, pend, pendW = linea{}, nil, 0
			}
			if w > ancho {
				out, cur = partirPalabra(out, cur, palabra, tam, ancho)
			} else {
				cur.piezas, cur.ancho = append(append(cur.piezas, pend...), palabra...), cur.ancho+pendW+w
			}
			pend, pendW = nil, 0
			i = j
		}
	}
	return append(out, cur)
}

// partirPalabra reparte por glifos una palabra más ancha que la línea.
func partirPalabra(out []linea, cur linea, palabra []pieza, tam, ancho float64) ([]linea, linea) {
	for _, q := range palabra {
		for _, g := range q.glifos {
			gw := float64(q.cara.f.Avance(g.g)) * tam / 1000
			if cur.ancho+gw > ancho && len(cur.piezas) > 0 {
				out, cur = append(out, cur), linea{}
			}
			cur = cur.con(q.cara, q.est, g, gw)
		}
	}
	return out, cur
}

// con añade un glifo a la línea, fundiéndolo con la última pieza si es de la misma cara.
func (l linea) con(c *caraUsada, est *estilo, g glifo, w float64) linea {
	if n := len(l.piezas); n > 0 && l.piezas[n-1].cara == c && l.piezas[n-1].est == est && !l.piezas[n-1].espacio {
		last := &l.piezas[n-1]
		last.glifos = append(last.glifos[:len(last.glifos):len(last.glifos)], g)
		last.ancho += w
	} else {
		l.piezas = append(l.piezas, pieza{cara: c, est: est, glifos: []glifo{g}, ancho: w})
	}
	l.ancho += w
	return l
}

// decoracion es un subrayado o tachado pendiente de dibujar como artefacto.
type decoracion struct {
	x1, x2, y float64
	color     [3]float64
	conColor  bool
}

// pintar emite una línea en (x, base): una cadena Tj por racha de piezas de la misma cara y
// el mismo estilo, con los espacios dentro (son glifos reales, no huecos: lo que lee un
// lector de pantalla). Cada racha va dentro de la secuencia de contenido marcado de su
// elemento; subrayados y tachados, como artefactos, después.
func (d *Doc) pintar(l linea, x, base, tam float64) {
	var decos []decoracion
	for i := 0; i < len(l.piezas); {
		c, est := l.piezas[i].cara, l.piezas[i].est
		var hex strings.Builder
		w, visible := 0.0, 0.0
		j := i
		for ; j < len(l.piezas) && l.piezas[j].cara == c && l.piezas[j].est == est; j++ {
			for _, g := range l.piezas[j].glifos {
				fmt.Fprintf(&hex, "%04X", g.g)
				c.sub.Usar(g.g, g.r)
			}
			w += l.piezas[j].ancho
			if !l.piezas[j].final {
				visible += l.piezas[j].ancho
			}
		}
		d.abrirMC(est.owner)
		color := "0 g"
		if est.conColor {
			color = fmt.Sprintf("%.3f %.3f %.3f rg", est.color[0], est.color[1], est.color[2])
		}
		fmt.Fprintf(&d.cur.contenido, "%s BT /%s %s Tf %s %s Td <%s> Tj ET\n", color, c.nombre, ftoa(tam), ftoa(x), ftoa(base), hex.String())
		if est.enlace != nil {
			d.anotar(est, [4]float64{x, base - tam*0.25, x + visible, base + tam*0.85})
		}
		if est.subr {
			decos = append(decos, decoracion{x, x + visible, base - tam*0.12, est.color, est.conColor})
		}
		if est.tach {
			decos = append(decos, decoracion{x, x + visible, base + tam*0.3, est.color, est.conColor})
		}
		x += w
		i = j
	}
	if len(decos) > 0 {
		d.artefacto(func(b *strings.Builder) {
			for _, dc := range decos {
				col := "0 0 0"
				if dc.conColor {
					col = fmt.Sprintf("%.3f %.3f %.3f", dc.color[0], dc.color[1], dc.color[2])
				}
				fmt.Fprintf(b, "q %s RG %s w %s %s m %s %s l S Q\n", col, ftoa(tam*0.06), ftoa(dc.x1), ftoa(dc.y), ftoa(dc.x2), ftoa(dc.y))
			}
		})
	}
}

// anotar registra la anotación de enlace de una racha y la cuelga, como OBJR, del Link.
func (d *Doc) anotar(est *estilo, rect [4]float64) {
	a := &anotacion{pg: d.cur, rect: rect, uri: est.enlace.uri, contents: strings.TrimSpace(est.enlace.texto)}
	if a.contents == "" {
		a.contents = est.enlace.uri
	}
	d.cur.anots = append(d.cur.anots, a)
	est.owner.hijos = append(est.owner.hijos, hijo{an: a})
}
