// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Contrapartida de apps/frontend/apps/rooms/src/components/MessageBody.tsx dentro de
// la pantalla de canal declarada en pantalla.go.
//
// El cuerpo de un mensaje con su Markdown ligero: **negrita**, _cursiva_, ~~tachado~~,
// `código`, bloques ``` con su lenguaje, enlaces y `[texto](url)`, títulos #, ##, ###,
// listas - y 1., citas >, los :shortcode: de emoji y los chips de @mención. Es el
// mismo parser de la web —plano, sin anidar marcas— portado regla a regla.

// TipoNodo es el `kind` de un `MdNode`.
type TipoNodo uint8

// Los once.
const (
	NodoTexto TipoNodo = iota
	NodoNegrita
	NodoCursiva
	NodoTachado
	NodoCodigo
	NodoBloque
	NodoEnlace
	NodoTitulo
	NodoCita
	NodoLista
	NodoNumerada
)

// Nodo es un `MdNode`.
type Nodo struct {
	Tipo TipoNodo
	// Texto es el de las marcas inline y el del bloque de código; Enlace, el `href`.
	Texto, Enlace, Lenguaje string
	// Nivel es el del título (1-3) e Inicio el primer número de una lista numerada.
	Nivel, Inicio int
	Hijos         []Nodo
	Items         [][]Nodo
}

var (
	reValla     = regexp.MustCompile("```([\\w+-]*)\\n?([\\s\\S]*?)```")
	reEnLinea   = regexp.MustCompile("`([^`\\n]+)`|\\*\\*([^\\n]+?)\\*\\*|~~([^~\\n]+?)~~|_([^_\\n]+)_|\\[([^\\]\\n]+)\\]\\((https?://[^\\s)]+)\\)|https?://[^\\s]+")
	reColgante  = regexp.MustCompile(`[.,;:!?…»)\]]+$`)
	reTitulo    = regexp.MustCompile(`^(#{1,3})\s+(.+?)\s*$`)
	reVineta    = regexp.MustCompile(`^\s*[-*]\s+(.+)$`)
	reNumero    = regexp.MustCompile(`^\s*(\d{1,9})\.\s+(.+)$`)
	reCita      = regexp.MustCompile(`^\s*>\s?(.*)$`)
	reShortcode = regexp.MustCompile(`:([a-z0-9_+-]+):`)
	reDeCanal   = regexp.MustCompile(`@(?:channel|here|all)\b`)
)

// palabra es `\w` de JavaScript: ASCII, cifra o guion bajo.
func palabra(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// enLinea es `parseInline`. La cursiva lleva en la web un lookbehind y un lookahead
// (`(?<![\w_])_…_(?!\w)`) para no romper snake_case; Go no los tiene y se comprueban a
// mano: una cursiva que no los cumple deja el guion bajo como texto y sigue buscando
// desde la letra siguiente.
func enLinea(texto string) []Nodo {
	var out []Nodo
	ultimo, desde := 0, 0
	for desde <= len(texto) {
		m := reEnLinea.FindStringSubmatchIndex(texto[desde:])
		if m == nil {
			break
		}
		for i := range m {
			if m[i] >= 0 {
				m[i] += desde
			}
		}
		ini, fin := m[0], m[1]
		if m[8] >= 0 { // _cursiva_
			antes := ini == 0 || !palabra(texto[ini-1])
			despues := fin == len(texto) || !palabra(texto[fin])
			if !antes || !despues {
				desde = ini + 1
				continue
			}
		}
		if ini > ultimo {
			out = append(out, Nodo{Tipo: NodoTexto, Texto: texto[ultimo:ini]})
		}
		ultimo, desde = fin, fin
		switch {
		case m[2] >= 0:
			out = append(out, Nodo{Tipo: NodoCodigo, Texto: texto[m[2]:m[3]]})
		case m[4] >= 0:
			out = append(out, Nodo{Tipo: NodoNegrita, Texto: texto[m[4]:m[5]]})
		case m[6] >= 0:
			out = append(out, Nodo{Tipo: NodoTachado, Texto: texto[m[6]:m[7]]})
		case m[8] >= 0:
			out = append(out, Nodo{Tipo: NodoCursiva, Texto: texto[m[8]:m[9]]})
		case m[10] >= 0:
			// `[texto](url)`: solo http y https, así que un `javascript:` se queda en texto.
			out = append(out, Nodo{Tipo: NodoEnlace, Enlace: texto[m[12]:m[13]], Texto: texto[m[10]:m[11]]})
		default:
			href := reColgante.ReplaceAllString(texto[ini:fin], "")
			if href == "http://" || href == "https://" {
				out = append(out, Nodo{Tipo: NodoTexto, Texto: texto[ini:fin]})
				break
			}
			out = append(out, Nodo{Tipo: NodoEnlace, Enlace: href})
			ultimo, desde = ini+len(href), ini+len(href) // la puntuación vuelve al texto
		}
		if fin == ini {
			desde++
		}
	}
	if ultimo < len(texto) {
		out = append(out, Nodo{Tipo: NodoTexto, Texto: texto[ultimo:]})
	}
	return out
}

// bloques es `parseBlocks`: títulos, listas y citas línea a línea; lo demás se junta en
// párrafos, que pasan por `enLinea`.
func bloques(texto string) []Nodo {
	var parrafo []string
	var nodos []Nodo
	cerrar := func() {
		if len(parrafo) > 0 {
			nodos = append(nodos, enLinea(strings.Join(parrafo, "\n"))...)
			parrafo = nil
		}
	}
	lineas := strings.Split(texto, "\n")
	for i := 0; i < len(lineas); {
		l := lineas[i]
		switch {
		case reTitulo.MatchString(l):
			cerrar()
			m := reTitulo.FindStringSubmatch(l)
			nodos = append(nodos, Nodo{Tipo: NodoTitulo, Nivel: len(m[1]), Hijos: enLinea(m[2])})
			i++
		case reVineta.MatchString(l):
			cerrar()
			n := Nodo{Tipo: NodoLista}
			for ; i < len(lineas) && reVineta.MatchString(lineas[i]); i++ {
				n.Items = append(n.Items, enLinea(reVineta.FindStringSubmatch(lineas[i])[1]))
			}
			nodos = append(nodos, n)
		case reNumero.MatchString(l):
			cerrar()
			n := Nodo{Tipo: NodoNumerada}
			for ; i < len(lineas) && reNumero.MatchString(lineas[i]); i++ {
				m := reNumero.FindStringSubmatch(lineas[i])
				if len(n.Items) == 0 {
					n.Inicio, _ = strconv.Atoi(m[1])
				}
				n.Items = append(n.Items, enLinea(m[2]))
			}
			nodos = append(nodos, n)
		case reCita.MatchString(l):
			cerrar()
			var citado []string
			for ; i < len(lineas) && reCita.MatchString(lineas[i]); i++ {
				citado = append(citado, reCita.FindStringSubmatch(lineas[i])[1])
			}
			nodos = append(nodos, Nodo{Tipo: NodoCita, Hijos: enLinea(strings.Join(citado, "\n"))})
		default:
			parrafo = append(parrafo, l)
			i++
		}
	}
	cerrar()
	return nodos
}

// ParsearCuerpo es `parseMarkdown`: primero los bloques de código, que no se tocan por
// dentro, y lo demás por bloques.
func ParsearCuerpo(cuerpo string) []Nodo {
	var out []Nodo
	ultimo := 0
	for _, m := range reValla.FindAllStringSubmatchIndex(cuerpo, -1) {
		if m[0] > ultimo {
			out = append(out, bloques(strings.TrimSuffix(cuerpo[ultimo:m[0]], "\n"))...)
		}
		out = append(out, Nodo{
			Tipo: NodoBloque, Lenguaje: cuerpo[m[2]:m[3]],
			Texto: strings.TrimSuffix(cuerpo[m[4]:m[5]], "\n"),
		})
		ultimo = m[1]
		if ultimo < len(cuerpo) && cuerpo[ultimo] == '\n' {
			ultimo++
		}
	}
	if ultimo < len(cuerpo) {
		out = append(out, bloques(cuerpo[ultimo:])...)
	}
	return out
}

// Emojificar es `emojify`: los :shortcode: conocidos por su emoji; los demás, literales.
func Emojificar(s string) string {
	if !strings.Contains(s, ":") {
		return s
	}
	return reShortcode.ReplaceAllStringFunc(s, func(t string) string {
		if e, hay := emojiPorNombre[t[1:len(t)-1]]; hay {
			return e
		}
		return t
	})
}

// ---- La pintura ----

// Estilo es cómo va un trozo de texto.
type estilo uint8

const (
	estNormal estilo = iota
	estNegrita
	estCursiva
	estTachado
	estCodigo
	estEnlace
	estMencion
	estDeCanal
	// estAutorEmote es el nombre de «* Autor acción»: en cursiva, `text-text font-medium`.
	estAutorEmote
	// estEmojiPropio es un `:nombre:` del catálogo de la organización: su imagen.
	estEmojiPropio
)

// trozo es un tramo de texto con su estilo, antes de repartir en líneas.
type trozo struct {
	texto  string
	estilo estilo
}

// pieza es lo que se coloca: un trozo ya en su línea.
type pieza struct {
	x, y, ancho float32
	texto       string
	estilo      estilo
	escalon     tema.Escalon
	peso        ui.Peso
	tinta       string
}

// Las medidas de la HOJA.
var (
	// CanalonChip es el `px-1` de un chip de mención y del código en línea.
	CanalonChipCuerpo = tema.Espacio(1)
	// SangriaCita es el `border-l-2 pl-3` de una cita: 2 + 10,5.
	SangriaCita = 2 + tema.Espacio(3)
	// SangriaLista es el `pl-1` de un elemento de lista.
	SangriaLista = tema.Espacio(1)
	// AireBloque es el `my-1` de citas, listas y bloques de código.
	AireBloque = tema.Espacio(1)
)

// maquetador reparte los nodos en piezas y cajas a un ancho dado.
type maquetador struct {
	ancho     float32
	menciones *regexp.Regexp
	piezas    []pieza
	cajas     []fondoCuerpo
	codigos   []bloqueCodigo
	y         float32
	finX      float32
	// solo es el fichero de un cuerpo que es SOLO un emoji de la organización, y tamSolo
	// lo que mide su imagen.
	solo    string
	tamSolo fyne.Size
}

// bloqueCodigo es un bloque ``` ya colocado: su caja, su lenguaje y su texto. Lo pinta
// el renderer, porque lleva un botón y un desplazamiento horizontal.
type bloqueCodigo struct {
	x, y, ancho, alto float32
	lenguaje, texto   string
}

// fondoCuerpo es un fondo: un chip, el código, la barra de una cita o un bloque de código.
type fondoCuerpo struct {
	x, y, ancho, alto float32
	relleno, borde    string
	radio             float32
}

// trozosDe son los tramos de un nodo inline, con el emoji y las menciones ya partidos.
func (m *maquetador) trozosDe(nodos []Nodo) []trozo {
	var out []trozo
	for _, n := range nodos {
		switch n.Tipo {
		case NodoNegrita:
			out = append(out, trozo{n.Texto, estNegrita})
		case NodoCursiva:
			out = append(out, trozo{n.Texto, estCursiva})
		case NodoTachado:
			out = append(out, trozo{n.Texto, estTachado})
		case NodoCodigo:
			out = append(out, trozo{n.Texto, estCodigo})
		case NodoEnlace:
			t := n.Texto
			if t == "" {
				t = n.Enlace
			}
			out = append(out, trozo{t, estEnlace})
		default:
			out = append(out, m.conPropios(n.Texto)...)
		}
	}
	return out
}

// conPropios es `withInline`: primero los `:nombre:` de la organización, que ganan al
// emoji Unicode del mismo nombre; lo que queda pasa por el emoji y las menciones.
func (m *maquetador) conPropios(s string) []trozo {
	if len(catalogoOrg.fichero) == 0 || !strings.Contains(s, ":") {
		return m.conMenciones(Emojificar(s))
	}
	var out []trozo
	ultimo := 0
	for _, i := range reEmojiPropio.FindAllStringSubmatchIndex(s, -1) {
		if _, ok := ficheroPropio(s[i[2]:i[3]]); !ok {
			continue
		}
		out = append(out, m.conMenciones(Emojificar(s[ultimo:i[0]]))...)
		out = append(out, trozo{s[i[0]:i[1]], estEmojiPropio})
		ultimo = i[1]
	}
	return append(out, m.conMenciones(Emojificar(s[ultimo:]))...)
}

// conMenciones parte el texto en lo corriente, las @menciones de personas (chip azul)
// y las de canal (@channel, @here, @all: chip de aviso).
func (m *maquetador) conMenciones(s string) []trozo {
	var out []trozo
	partir := func(s string, re *regexp.Regexp, e estilo, resto func(string)) {
		ultimo := 0
		if re != nil {
			for _, i := range re.FindAllStringIndex(s, -1) {
				if e == estDeCanal && i[0] > 0 && (palabra(s[i[0]-1]) || s[i[0]-1] == '@') {
					continue // `(?<![\w@])`
				}
				resto(s[ultimo:i[0]])
				out = append(out, trozo{s[i[0]:i[1]], e})
				ultimo = i[1]
			}
		}
		resto(s[ultimo:])
	}
	partir(s, reDeCanal, estDeCanal, func(t string) {
		partir(t, m.menciones, estMencion, func(u string) {
			if u != "" {
				out = append(out, trozo{u, estNormal})
			}
		})
	})
	return out
}

// estiloDe da la tipografía y la tinta de un trozo en un bloque de tinta base.
func estiloDe(e estilo, base ui.Peso, tinta string) (tema.Escalon, ui.Peso, string) {
	switch e {
	case estNegrita:
		return tema.TextoBase, ui.SemiNegrita, tinta
	case estCodigo:
		return tema.TextoSM, ui.Normal, tinta
	case estEnlace:
		return tema.TextoBase, base, "primary-text"
	case estMencion:
		return tema.TextoBase, ui.Medio, "primary-text"
	case estDeCanal:
		return tema.TextoBase, ui.Medio, "warning-text"
	case estAutorEmote:
		return tema.TextoBase, ui.Medio, "text"
	case estNormal, estCursiva, estTachado, estEmojiPropio:
	}
	return tema.TextoBase, base, tinta
}

// fluir coloca los trozos desde `x0`, partiendo por palabras y en cada `\n`, y deja la
// última línea abierta en m.finX. `linea` es el alto de cada renglón.
//
//nolint:gocognit // Las ramas reflejan los estilos de texto que debe conservar el maquetador.
func (m *maquetador) fluir(trozos []trozo, x0 float32, esc tema.Escalon, base ui.Peso, tinta string) {
	x := x0
	for _, tz := range trozos {
		e, p, t := estiloDe(tz.estilo, base, tinta)
		if esc.Tam != tema.TextoBase.Tam && tz.estilo != estCodigo {
			e = esc
		}
		atomico := tz.estilo == estCodigo || tz.estilo == estMencion || tz.estilo == estDeCanal
		extra := float32(0)
		if atomico {
			extra = 2 * CanalonChipCuerpo
		}
		palabras := []string{tz.texto}
		if !atomico {
			palabras = partirPalabras(tz.texto)
		}
		if tz.estilo == estEmojiPropio {
			x = m.fluirPropio(tz.texto, x, x0, esc, e, p)
			continue
		}
		for _, w := range palabras {
			if w == "\n" {
				x, m.y = x0, m.y+esc.Linea
				continue
			}
			ancho := ui.Medir(w, e, p).Width + extra
			if tz.estilo == estCodigo {
				ancho = medirMono(w, e) + extra
			}
			if tz.estilo == estCursiva || tz.estilo == estAutorEmote {
				ancho = medirCursiva(w, e)
			}
			if x > x0 && x+ancho > m.ancho && strings.TrimSpace(w) != "" {
				x, m.y = x0, m.y+esc.Linea
			}
			if x == x0 && strings.TrimSpace(w) == "" {
				continue // un espacio al principio de línea no se ve
			}
			if atomico {
				m.cajas = append(m.cajas, fondoChip(tz.estilo, x, m.y+(esc.Linea-e.Linea)/2, ancho, e.Linea))
			}
			m.piezas = append(m.piezas, pieza{
				x: x + extra/2, y: m.y + (esc.Linea-e.Linea)/2, ancho: ancho - extra, texto: w,
				estilo: tz.estilo, escalon: e, peso: p, tinta: t,
			})
			x += ancho
		}
	}
	m.finX = x
}

// fluirPropio coloca un emoji de la organización, que no se parte, y dice dónde sigue
// la línea.
func (m *maquetador) fluirPropio(texto string, x, x0 float32, esc, e tema.Escalon, p ui.Peso) float32 {
	ancho := anchoEmojiPropio(e)
	if x > x0 && x+ancho > m.ancho {
		x, m.y = x0, m.y+esc.Linea
	}
	m.piezas = append(m.piezas, pieza{
		x: x + 1, y: m.y + (esc.Linea-e.Linea)/2, ancho: ancho - 2, texto: texto,
		estilo: estEmojiPropio, escalon: e, peso: p,
	})
	return x + ancho
}

// fondoChip es la caja de un trozo que no se parte: el código en línea (con su borde),
// una @mención o un @here/@channel.
func fondoChip(e estilo, x, y, ancho, alto float32) fondoCuerpo {
	c := fondoCuerpo{x: x, y: y, ancho: ancho, alto: alto, relleno: "selected", radio: tema.RadioSM}
	switch e {
	case estCodigo:
		c.relleno, c.borde = "surface-sunken", "border"
	case estDeCanal:
		c.relleno = "warning-bg"
	default: // una @mención
	}
	return c
}

// partirPalabras corta en palabras y espacios, y deja cada `\n` suelto.
func partirPalabras(s string) []string {
	var out []string
	ini := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			if i > ini {
				out = append(out, s[ini:i])
			}
			out = append(out, "\n")
			ini = i + 1
		case ' ':
			if i > ini {
				out = append(out, s[ini:i])
			}
			out = append(out, " ")
			ini = i + 1
		}
	}
	if ini < len(s) {
		out = append(out, s[ini:])
	}
	return out
}

func medirCursiva(s string, e tema.Escalon) float32 {
	tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, e.Tam, fyne.TextStyle{}, tema.Cursiva)
	return tam.Width
}

// maquetar reparte todos los nodos.
func (m *maquetador) maquetar(nodos []Nodo) {
	for i, n := range nodos {
		// Lo inline seguido va en el mismo renglón: se junta hasta el siguiente bloque.
		if esInline(n.Tipo) {
			if i > 0 && esInline(nodos[i-1].Tipo) {
				continue
			}
			j := i
			for j < len(nodos) && esInline(nodos[j].Tipo) {
				j++
			}
			if i > 0 {
				m.y += tema.TextoBase.Linea // tras un bloque, el renglón siguiente
			}
			m.fluir(m.trozosDe(nodos[i:j]), 0, tema.TextoBase, ui.Normal, "text")
			continue
		}
		if i > 0 {
			m.y += tema.TextoBase.Linea
		}
		m.bloque(n)
	}
}

func esInline(t TipoNodo) bool {
	return t <= NodoCodigo || t == NodoEnlace
}

// bloque pinta un título, una cita, una lista o un bloque de código.
func (m *maquetador) bloque(n Nodo) {
	switch n.Tipo {
	case NodoTitulo:
		esc := map[int]tema.Escalon{1: tema.TextoLG, 2: tema.TextoMD}[n.Nivel]
		if esc.Tam == 0 {
			esc = tema.TextoBase
		}
		m.y += tema.Espacio(1.5) // `mt-1.5`
		m.fluir(m.trozosDe(n.Hijos), 0, esc, ui.SemiNegrita, "text")
		m.y += esc.Linea - tema.TextoBase.Linea + tema.Espacio(0.5) // `mb-0.5`
	case NodoCita:
		m.y += AireBloque
		y0 := m.y
		m.fluir(m.trozosDe(n.Hijos), SangriaCita, tema.TextoBase, ui.Normal, "text-muted")
		m.cajas = append(m.cajas, fondoCuerpo{x: 0, y: y0, ancho: 2, alto: m.y + tema.TextoBase.Linea - y0, relleno: "border"})
		m.y += AireBloque
	case NodoLista, NodoNumerada:
		m.y += AireBloque
		for i, it := range n.Items {
			if i > 0 {
				m.y += tema.TextoBase.Linea
			}
			marca := "• "
			if n.Tipo == NodoNumerada {
				marca = strconv.Itoa(n.Inicio+i) + ". "
			}
			m.fluir(append([]trozo{{marca, estNormal}}, m.trozosDe(it)...), SangriaLista, tema.TextoBase, ui.Normal, "text")
		}
		m.y += AireBloque
	case NodoBloque:
		// La caja con su cabecera —el lenguaje y «Copiar»— y el código en la letra
		// monoespaciada, sin partir líneas y con desplazamiento horizontal.
		m.y += AireBloque
		alto := AltoBloqueCodigo(n.Texto)
		m.codigos = append(m.codigos, bloqueCodigo{x: 0, y: m.y, ancho: m.ancho, alto: alto, lenguaje: n.Lenguaje, texto: n.Texto})
		m.y += alto - tema.TextoBase.Linea + AireBloque
		m.finX = m.ancho
	default: // los de en línea no llegan aquí: los reparte fluir
	}
}

// reMenciones es la expresión de las @menciones de esos nombres, los más largos
// primero para que «@Ana» no tape a «@Ana María».
func reMenciones(nombres []string) *regexp.Regexp {
	vistos := map[string]bool{}
	var ns []string
	for _, n := range nombres {
		if strings.TrimSpace(n) != "" && !vistos[n] {
			vistos[n] = true
			ns = append(ns, regexp.QuoteMeta(n))
		}
	}
	if len(ns) == 0 {
		return nil
	}
	sort.SliceStable(ns, func(i, j int) bool { return len(ns[i]) > len(ns[j]) })
	return regexp.MustCompile("@(?:" + strings.Join(ns, "|") + ")")
}

// MaquetarCuerpo reparte un cuerpo a un ancho y dice cuánto mide de alto y dónde acaba
// su último renglón (para poner «(editado)» detrás).
func MaquetarCuerpo(cuerpo string, menciones []string, ancho float32) (alto, finX float32) {
	return MaquetarCuerpoDe(cuerpo, "", menciones, ancho)
}

// MaquetarCuerpoDe es MaquetarCuerpo sabiendo el autor, que es quien sale en un «/me».
func MaquetarCuerpoDe(cuerpo, autor string, menciones []string, ancho float32) (alto, finX float32) {
	m := maquetadorDe(cuerpo, autor, menciones, ancho)
	return m.y + tema.TextoBase.Linea, m.finX
}

func maquetadorDe(cuerpo, autor string, menciones []string, ancho float32) *maquetador {
	m := &maquetador{ancho: ancho, menciones: reMenciones(menciones)}
	if ancho <= 0 {
		m.ancho = 1 << 20 // sin ancho aún: no se parte, como `Repartir`
	}
	// El mensaje que es SOLO un emoji de la organización va en grande: así se manda un
	// GIF. Con texto alrededor, a tamaño de emoji.
	if f, ok := soloPropio(cuerpo); ok {
		m.solo, m.tamSolo = f, tamSoloPropio(imagenPropia(f), ancho)
		m.y, m.finX = m.tamSolo.Height-tema.TextoBase.Linea, m.tamSolo.Width
		return m
	}
	if accion := AccionDeEmote(cuerpo); accion != "" {
		m.emote(accion, autor)
		return m
	}
	m.maquetar(ParsearCuerpo(cuerpo))
	return m
}

// emote es el «/me acción» de `MessageBody`: `<em class="text-text-muted">` con «* », el
// autor en `text-text font-medium` y la acción con sus marcas en línea.
//
// ponytail: el autor va en la cursiva normal y no en una media cursiva, que la suite no
// embebe; la diferencia es de un peso (500 contra 400).
func (m *maquetador) emote(accion, autor string) {
	trozos := []trozo{{"* ", estCursiva}}
	if autor != "" {
		trozos = append(trozos, trozo{autor + " ", estAutorEmote})
	}
	for _, t := range m.trozosDe(enLinea(accion)) {
		if t.estilo == estNormal {
			t.estilo = estCursiva
		}
		trozos = append(trozos, t)
	}
	m.fluir(trozos, 0, tema.TextoBase, ui.Normal, "text-muted")
}

// CuerpoRico pinta el cuerpo de un mensaje.
type CuerpoRico struct {
	widget.BaseWidget
	Cuerpo    string
	Menciones []string
	// Autor es quien sale en un «/me acción».
	Autor string
	// T pone el rótulo de un bloque de código sin lenguaje y el de «Copiar»; sin él, los
	// respaldos de la web («código» y ninguno).
	T *i18n.Translator
	// AlCopiar recibe el texto de un bloque de código al pulsar su «Copiar».
	AlCopiar func(string)
	// Tinta es la del texto corriente: `text`, o `text-muted` mientras se envía.
	Tinta string

	objetos []fyne.CanvasObject
	ancho   float32
}

var _ fyne.Widget = (*CuerpoRico)(nil)

// NuevoCuerpoRico crea el cuerpo.
func NuevoCuerpoRico(cuerpo string, menciones []string) *CuerpoRico {
	c := &CuerpoRico{Cuerpo: cuerpo, Menciones: menciones, Tinta: "text"}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el texto tal cual.
func (c *CuerpoRico) AccessibilityLabel() string { return c.Cuerpo }

// AccessibilityRole implementa fyne.Accessible.
func (c *CuerpoRico) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer compone el cuerpo.
func (c *CuerpoRico) CreateRenderer() fyne.WidgetRenderer { return &cuerpoRenderer{c: c} }

type cuerpoRenderer struct{ c *CuerpoRico }

func (r *cuerpoRenderer) MinSize() fyne.Size {
	alto, _ := MaquetarCuerpoDe(r.c.Cuerpo, r.c.Autor, r.c.Menciones, r.c.ancho)
	return fyne.NewSize(0, alto)
}
func (r *cuerpoRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *cuerpoRenderer) Destroy()                     {}

func (r *cuerpoRenderer) Layout(t fyne.Size) {
	c := r.c
	c.ancho = t.Width
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := maquetadorDe(c.Cuerpo, c.Autor, c.Menciones, t.Width)
	c.objetos = c.objetos[:0]
	if m.solo != "" {
		if img := imagenPropia(m.solo); img != nil {
			ci := canvas.NewImageFromImage(img) // `max-h-48 max-w-full rounded-md`
			ci.FillMode, ci.CornerRadius = canvas.ImageFillContain, tema.RadioMD
			ci.Resize(m.tamSolo)
			c.objetos = append(c.objetos, ci)
		}
		return
	}
	for _, k := range m.cajas {
		relleno := tokens.SobreSRGB(tema.Color(k.relleno, v), tema.Color("surface", v))
		borde := color.NRGBA{}
		if k.borde != "" {
			borde = tema.Color(k.borde, v)
		}
		cj := ui.NuevaCaja(relleno, borde, k.radio)
		ui.MoverCaja(cj, fyne.NewPos(k.x, k.y), fyne.NewSize(k.ancho, k.alto))
		c.objetos = append(c.objetos, cj.Objetos()...)
	}
	for _, p := range m.piezas {
		if p.estilo == estEmojiPropio {
			f, _ := ficheroPropio(strings.Trim(p.texto, ":"))
			if img := imagenPropia(f); img != nil {
				ci := canvas.NewImageFromImage(img) // EMOJI_IMG_CLASS: `object-contain`
				ci.FillMode = canvas.ImageFillContain
				ci.Move(fyne.NewPos(p.x, yEmojiPropio(p.escalon, p.peso, p.y)))
				ci.Resize(fyne.NewSize(p.ancho, p.ancho))
				c.objetos = append(c.objetos, ci)
			}
			continue
		}
		tinta := p.tinta
		if tinta == "text" {
			tinta = c.Tinta
		}
		tx := ui.Texto(p.texto, p.escalon, p.peso, tema.Color(tinta, v))
		switch p.estilo {
		case estCursiva, estAutorEmote:
			tx.FontSource = tema.Cursiva
		case estEnlace:
			tx.TextStyle.Underline = true
		case estCodigo:
			if p.ancho < t.Width {
				break
			}
			ui.Truncar(tx, p.texto, p.escalon, p.peso, max(t.Width-p.x-tema.Espacio(2.5), 0))
		case estNormal, estNegrita, estTachado, estMencion, estDeCanal, estEmojiPropio:
		}
		if p.estilo == estCodigo {
			tx.FontSource, tx.TextStyle = caraMono()
		}
		ui.ColocarTexto(tx, p.escalon, p.x, p.y)
		c.objetos = append(c.objetos, tx)
		if p.estilo == estTachado {
			raya := canvas.NewRectangle(tema.Color(tinta, v))
			raya.Move(fyne.NewPos(p.x, p.y+p.escalon.Linea/2))
			raya.Resize(fyne.NewSize(p.ancho, 1))
			c.objetos = append(c.objetos, raya)
		}
	}
	for _, b := range m.codigos {
		c.objetos = append(c.objetos, c.pintarCodigo(b, v)...)
	}
}

func (r *cuerpoRenderer) Refresh() { r.Layout(r.c.Size()) }
