// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"image"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// EL FORMATO NATIVO DE NOTEPADS ES HTML. Este fichero es quien lo lee, quien lo vuelve
// a escribir y quien decide si esta vista puede GUARDAR la nota o solo enseñarla.
//
// # Las dos preguntas, que no son la misma
//
// Es el contrato de Diapos, y aquí discrepan en los dos sentidos:
//
//   - `Fiel()`: ¿lo que volvería a escribirse es lo mismo que había?
//   - `NoPintado()`: ¿qué trae que esta vista no sabe dibujar?
//
// Un `<p class="nota">` se pinta perfectamente y NO es fiel, porque esa clase no está en
// el modelo y se caería al reescribir; esa nota se abre en solo lectura aunque a la
// vista no se le note nada. Y al revés: lo que se conserva y no se sabe dibujar va al
// cartel (hoy no queda nada así: la tabla, que lo fue, se pinta entera).
//
// Y de Calcus se toma la consecuencia: **lo que no sea fiel no se puede escribir**, ni
// tecleando. Lo mira `Nota.Editable`.
//
// # Por qué reescribir y comparar, y por qué DESDE EL MODELO
//
// Porque es lo único comprobable: un lector que «entiende» un documento siempre cree
// que lo entiende, y lo que lo demuestra es volver a escribirlo y que salga igual.
//
// Y tiene que salir del MODELO y no del árbol original. Reescribir el árbol devuelve
// por definición lo que entró, así que `Fiel` habría salido en verde sobre cualquier
// nota —incluida una que esta vista no entiende— y no habría guardado nada. Al emitir
// desde el modelo, lo que no está modelado no sale, y la comparación lo caza.
//
// # El vocabulario, que no me lo invento
//
// Es EXACTAMENTE la allowlist de `services/notes/internal/domain/html.go`, que es la
// que el servicio aplica al guardar: lo que llegue aquí ya pasó por ella. De ahí salen
// las tres cosas que no se adivinan leyendo el editor de la web:
//
//  1. Todo `<a href>` sale del servicio con `target="_blank" rel="noopener noreferrer"`
//     puestos por él. Sin reproducirlos, NINGUNA nota con un enlace sería fiel.
//  2. Una tarea no es un `<li>`: es
//     `<div class="task-row"><input type="checkbox" checked> <span>…</span></div>`.
//  3. `class` solo puede valer `task-row` o `task-checkbox`, y `style` solo puede traer
//     `color`, `background-color`, `text-align` y `cursor:pointer`.
//
// Por eso `NoPintado` no habla de `<script>`: no existe en una nota guardada.
//
// # Las tablas, aplanadas
//
// Una tabla NO es un bloque: cada celda son sus bloques de siempre —un párrafo, una
// lista, una tarea— y cada uno lleva en `Celda` de qué tabla, fila y columna es. Así el
// cursor, la selección, escribir, las marcas y el deshacer funcionan dentro de una celda
// sin una línea más, y el maquetador pinta cada celda con la misma prosa que la nota. Lo
// que no es texto —las secciones, las filas, qué celda es de cabecera y los atributos de
// cada elemento— va en `Tabla`, que se comparte entre los bloques de una misma tabla y
// NO se cambia nunca en su sitio: las fotos de deshacer la comparten, así que añadir una
// fila es hacer una `Tabla` nueva. Los bloques de una tabla van SEGUIDOS y por filas, que
// es el orden en que se escriben.

// TipoBloque es qué clase de bloque es, que es lo que decide cómo se pinta.
type TipoBloque uint8

// Los bloques que esta vista dibuja, más `BloqueOpaco` para lo que no.
const (
	Parrafo  TipoBloque = iota
	Titulo              // h1…h6, con Nivel
	Cita                // blockquote
	Codigo              // pre
	Vineta              // li de un ul
	Numerada            // li de un ol
	Tarea               // div.task-row con su casilla
	Regla               // hr
	// BloqueOpaco se CONSERVA VERBATIM al reescribir y no se edita: hoy, una tabla
	// DENTRO de una celda. Se pinta con la misma rejilla que las demás (tabla.go).
	BloqueOpaco
)

// Tramo es un trozo de texto con sus marcas, que es lo que `canvas.Text` puede pintar
// de una vez: Fyne no tiene texto enriquecido, así que una línea son varios tramos.
type Tramo struct {
	Texto string
	// Marcas son las etiquetas de formato que envuelven el texto, en el orden en que
	// se abren y CON EL NOMBRE QUE TENÍAN.
	//
	// Es una lista y no seis booleanos por una razón medida contra el saneador del
	// servicio: `<b>` y `<strong>` pintan igual y NO son el mismo documento, y con
	// booleanos los dos volvían como `<strong>`. El caso `<p><b>a</b> <i>b</i></p>` de
	// `html_test.go` dejaba de ser fiel, o sea que toda nota escrita con `<b>` se
	// habría abierto en solo lectura. El orden importa por lo mismo: `<b><i>` y
	// `<i><b>` tampoco son el mismo documento.
	Marcas []string
	// EnSpan dice que el tramo venía envuelto en un `<span>`. Un span sin atributos no
	// cambia nada de cómo se PINTA y sí de cómo se ESCRIBE: el saneador lo deja pasar
	// y lo pone él mismo alrededor del texto de una tarea.
	EnSpan bool
	// Color y Fondo son el `color` y el `background-color` que el saneador permite,
	// en el hexadecimal en que venían.
	Color string
	Fondo string
	// Fuente y Tamano son el `font-family` y el `font-size` del span, tal como venían. Los
	// pone la barra de Sytue Mail; el servicio de notas no los deja pasar, así que en una
	// nota no aparecen.
	Fuente string
	Tamano string
	// Imagen son los atributos de un `<img>` —`src` incluido— tal como venían: el tramo
	// ES la imagen, sin texto, y cuenta como una letra, igual que un salto. Nil: es texto.
	// Solo las escribe Sytue Mail (`cid:` de una parte del mensaje, o `https:`); el
	// servicio de notas tampoco las deja pasar.
	Imagen []html.Attribute
	// Enlace es el `href`, y Externo dice que traía el `target`/`rel` que el servicio
	// añade a todos los enlaces guardados.
	Enlace  string
	Externo bool
	// Salto es un `<br/>` DENTRO del bloque. Es un tramo y no un bloque nuevo porque
	// `<p>a<br/>b</p>` y `<p>a</p><p>b</p>` no son el mismo documento, y partirlo en
	// dos dejaba sin poder editar cualquier nota con un salto suelto —que es lo que
	// escribe Mayús+Intro en el editor de la web—.
	Salto bool
}

// Bloque es una línea del documento.
type Bloque struct {
	Tipo TipoBloque
	// Nivel es 1..6 en un título y la profundidad de anidamiento en una lista.
	Nivel int
	// Marcada y Bloqueada son el `checked` y el `disabled` de la casilla de una tarea.
	Marcada   bool
	Bloqueada bool
	Tramos    []Tramo
	// Etiqueta es el elemento tal como venía: `p` o `div` en un párrafo —el editor de
	// la web escribe los dos y no son intercambiables al reescribir—, y el nombre del
	// elemento opaco para el cartel.
	Etiqueta string
	// Alineacion es el `text-align` del bloque, vacío si no traía.
	Alineacion string
	// Crudo es el HTML original de un bloque opaco, tal cual, para devolverlo igual.
	Crudo string
	// Celda es la celda de tabla en la que está el bloque; sin tabla, fuera de ellas.
	Celda Celda
	// Plegada es el `class="plegada"` de un `<li>` (la lista plegada de Twos): al leer la
	// nota no se ve lo que cuelga de él (plegar.go).
	Plegada bool
}

// Celda dice en qué celda de qué tabla está un bloque. Con `Tabla` nil, en ninguna. Es
// un valor comparable: dos bloques están en la misma celda si sus `Celda` son iguales.
type Celda struct {
	Tabla     *Tabla
	Fila, Col int
}

// EnTabla dice si el bloque está dentro de una tabla.
func (c Celda) EnTabla() bool { return c.Tabla != nil }

// Tabla es lo que una tabla tiene además del texto de sus celdas. No se cambia en su
// sitio (ver la cabecera).
type Tabla struct {
	Atributos []html.Attribute
	Filas     []FilaTabla
}

// FilaTabla es un `<tr>` con la sección en la que va.
type FilaTabla struct {
	// Seccion es `thead`, `tbody` o `tfoot`, y Nueva dice que esta fila ABRE su
	// elemento de sección: dos `<tbody>` seguidos no son uno.
	Seccion          string
	Nueva            bool
	AtributosSeccion []html.Attribute
	Atributos        []html.Attribute
	Celdas           []CeldaTabla
}

// CeldaTabla es un `<td>` o, con Cabecera, un `<th>`.
type CeldaTabla struct {
	Cabecera  bool
	Atributos []html.Attribute
}

// Texto es el bloque entero en plano, con los saltos donde estaban.
func (b Bloque) Texto() string {
	var s strings.Builder
	for _, t := range b.Tramos {
		if t.Salto {
			s.WriteByte('\n')
			continue
		}
		if t.Imagen != nil {
			s.WriteRune(Objeto) // una letra, para que las posiciones cuadren
			continue
		}
		s.WriteString(t.Texto)
	}
	return s.String()
}

// Objeto es la letra que ocupa una imagen en `Bloque.Texto()`: U+FFFC, el carácter de
// objeto incrustado. `TextoEntre` la quita, así que no llega al portapapeles.
const Objeto = '\ufffc'

// Documento es una nota ya interpretada.
type Documento struct {
	Bloques []Bloque
	// Imagenes da la imagen de un `src` para pintarla, o nil si no la hay (todavía). Es de
	// quien aloja el editor: Sytue Mail la saca de las partes `cid:` del mensaje. Sin él,
	// las imágenes se pintan como un hueco.
	Imagenes func(src string) image.Image

	original  string
	fiel      bool
	noPintado []string
	// celda es la celda que se está leyendo, mientras se lee.
	celda Celda
	// desplegado enseña también lo plegado: es la copia que se edita (`ParaEditar`),
	// como en la web, donde el editor ve la lista entera.
	desplegado bool
}

// Leer interpreta el HTML de una nota.
//
// Nunca devuelve nil ni error: una nota que no se entiende se abre en solo lectura, no
// tira la pantalla. Un documento vacío es fiel —no hay nada que perder—, y así una nota
// recién creada se puede escribir desde el primer momento.
func Leer(fuente string) *Documento {
	d := &Documento{original: fuente}
	if strings.TrimSpace(fuente) == "" {
		d.fiel = true
		return d
	}
	nodos, err := fragmento(fuente)
	if err != nil {
		return d // no fiel: no se ha podido leer entero
	}
	for _, n := range nodos {
		d.recorrer(n, Tramo{}, 0)
	}
	d.cerrarSuelto(Celda{})
	d.fiel = normalizar(fuente) == normalizar(d.HTML())
	return d
}

// fragmento analiza el HTML como INTERIOR de un `<div>`, que es lo que es el cuerpo de
// una nota. Con `html.Parse` la vuelta traería `<html>` y `<body>` y ninguna nota sería
// fiel.
func fragmento(s string) ([]*html.Node, error) {
	return html.ParseFragment(strings.NewReader(s), &html.Node{
		Type: html.ElementNode, Data: "div", DataAtom: atom.Div,
	})
}

// recorrer baja por el árbol acumulando bloques. `e` son las marcas heredadas.
//
//nolint:gocyclo,cyclop // es un despacho por etiqueta y partirlo esconde el vocabulario.
func (d *Documento) recorrer(n *html.Node, e Tramo, nivel int) {
	switch n.Type {
	case html.TextNode:
		// El blanco ENTRE elementos (`<ul>\n  <li>`) no es texto de la nota: meterlo en
		// el bloque anterior lo devolvería en otro sitio, y ninguna nota con el HTML
		// sangrado sería fiel.
		if strings.TrimSpace(n.Data) == "" && !llevaTexto(n.Parent) {
			return
		}
		d.añadirTexto(n.Data, e)
		return
	case html.ElementNode:
	case html.ErrorNode, html.DocumentNode, html.CommentNode, html.DoctypeNode, html.RawNode:
		// Un comentario ni se pinta ni se reescribe, así que su nota deja de ser fiel y
		// se abre en solo lectura. Es lo correcto: guardar encima lo borraría.
		return
	}
	switch n.Data {
	case "p", "div":
		if atributo(n, "class") == "task-row" {
			d.abrirTarea(n)
			break
		}
		d.abrir(Bloque{Tipo: Parrafo, Etiqueta: n.Data, Alineacion: alineacion(n)})
	case "h1", "h2", "h3", "h4", "h5", "h6":
		d.abrir(Bloque{
			Tipo: Titulo, Nivel: int(n.Data[1] - '0'),
			Etiqueta: n.Data, Alineacion: alineacion(n),
		})
	case "blockquote":
		d.abrir(Bloque{Tipo: Cita, Etiqueta: n.Data, Alineacion: alineacion(n)})
	case "pre":
		d.abrir(Bloque{Tipo: Codigo, Etiqueta: n.Data})
	case "hr":
		d.abrir(Bloque{Tipo: Regla})
		return
	case "br":
		d.añadirSalto(e)
		return
	case "li":
		b := Bloque{Tipo: Vineta, Nivel: max(nivel, 1), Etiqueta: n.Data, Plegada: atributo(n, "class") == claseplegada}
		if enOrdenada(n) {
			b.Tipo = Numerada
		}
		d.abrir(b)
	case "ul", "ol":
		nivel++
	case "table":
		if d.celda.EnTabla() {
			// Una tabla DENTRO de una celda (la barra de la web la mete si se pulsa
			// «Tabla» con el cursor en una) no se aplana: va opaca y verbatim, con su
			// texto para buscar y copiar, y tabla.go la pinta con la misma rejilla.
			d.abrir(Bloque{Tipo: BloqueOpaco, Etiqueta: "table", Crudo: serializar(n)})
			d.añadirTexto(htmltext.Text(interior(n)), Tramo{})
			return
		}
		d.leerTabla(n)
		return
	case "input":
		// Solo la casilla de una tarea, que se lee al abrir su `div.task-row`. Una
		// suelta no está modelada, y por eso su nota deja de ser fiel.
		return
	case "b", "strong", "i", "em", "u", "s", "strike", "del", "mark", "code":
		// `slices.Concat` y no `append`: `e` se pasa por valor a los hermanos y un
		// `append` sobre el mismo respaldo le metería a uno la marca del otro.
		e.Marcas = slices.Concat(e.Marcas, []string{n.Data})
	case "span":
		e.EnSpan = true
		st := atributo(n, "style")
		e.Color, e.Fondo = propiedad(st, "color"), propiedad(st, "background-color")
		e.Fuente, e.Tamano = propiedad(st, "font-family"), propiedad(st, "font-size")
	case "img":
		d.asegurarBloque()
		e.Texto, e.Imagen = "", slices.Clone(n.Attr)
		b := &d.Bloques[len(d.Bloques)-1]
		b.Tramos = append(b.Tramos, e)
		return
	case "a":
		e.Enlace = atributo(n, "href")
		e.Externo = atributo(n, "target") == "_blank" &&
			atributo(n, "rel") == "noopener noreferrer"
	default:
		// Una etiqueta que el saneador del servicio permite y este lector no modela.
		//
		// NO se añade al cartel de «esto no se dibuja», y es a propósito: no hay clave
		// de catálogo con la que nombrarla —la web no tiene ninguna, porque allí se
		// pinta todo— e inventarla aquí sacaría el literal `note.block.unknown` en
		// pantalla. Lo que sí pasa, y es lo accionable, es que la vuelta no la lleva:
		// `Fiel` sale en falso solo y la nota se abre en SOLO LECTURA con su aviso.
		// Anotado en el informe: la clave hay que escribirla en la web.
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		d.recorrer(h, e, nivel)
	}
}

// abrirTarea lee un `div.task-row` con su casilla y deja el bloque abierto para el
// texto que venga detrás.
func (d *Documento) abrirTarea(n *html.Node) {
	b := Bloque{Tipo: Tarea, Etiqueta: n.Data}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.Type == html.ElementNode && h.Data == "input" {
			b.Marcada, b.Bloqueada = tiene(h, "checked"), tiene(h, "disabled")
		}
	}
	d.abrir(b)
}

// leerTabla aplana una tabla: su forma a `Tabla` y el contenido de cada celda a bloques
// con su `Celda`. Cada celda empieza con un párrafo suelto, que es donde va el texto que
// no viene envuelto en nada (`<td>12</td>`) y lo que queda de una celda vacía.
func (d *Documento) leerTabla(n *html.Node) {
	t := &Tabla{Atributos: n.Attr}
	for sec := n.FirstChild; sec != nil; sec = sec.NextSibling {
		switch sec.DataAtom {
		case atom.Thead, atom.Tbody, atom.Tfoot:
		default:
			// Blanco entre secciones, un `<caption>`… no está modelado, así que no
			// vuelve y la nota se abre en solo lectura.
			continue
		}
		nueva := true
		for tr := sec.FirstChild; tr != nil; tr = tr.NextSibling {
			if tr.DataAtom != atom.Tr {
				continue
			}
			fila := FilaTabla{Seccion: sec.Data, Nueva: nueva, AtributosSeccion: sec.Attr, Atributos: tr.Attr}
			nueva = false
			f := len(t.Filas)
			for c := tr.FirstChild; c != nil; c = c.NextSibling {
				if c.DataAtom != atom.Td && c.DataAtom != atom.Th {
					continue
				}
				d.celda = Celda{Tabla: t, Fila: f, Col: len(fila.Celdas)}
				fila.Celdas = append(fila.Celdas, CeldaTabla{Cabecera: c.DataAtom == atom.Th, Atributos: c.Attr})
				d.abrir(Bloque{Tipo: Parrafo})
				for h := c.FirstChild; h != nil; h = h.NextSibling {
					d.recorrer(h, Tramo{}, 0)
				}
			}
			t.Filas = append(t.Filas, fila)
		}
	}
	d.celda = Celda{}
}

// abrir empieza un bloque nuevo en la celda que se está leyendo, cerrando el anterior
// si nació vacío.
func (d *Documento) abrir(b Bloque) {
	b.Celda = d.celda
	d.cerrarSuelto(b.Celda)
	d.Bloques = append(d.Bloques, b)
}

// cerrarSuelto tira el último bloque si no llegó a tener contenido: un `<div>`
// contenedor abre bloque y no aporta línea. La regla y la tarea nunca son vacías —la
// una es la línea y la otra es su casilla—, y un opaco tampoco. Tampoco el único
// bloque de una celda vacía, si lo que viene ya es de otra: sin él la celda no tendría
// dónde poner el cursor. `sig` es la celda de lo que viene.
func (d *Documento) cerrarSuelto(sig Celda) {
	n := len(d.Bloques)
	if n == 0 {
		return
	}
	u := d.Bloques[n-1]
	if u.Celda.EnTabla() && u.Celda != sig && (n == 1 || d.Bloques[n-2].Celda != u.Celda) {
		return
	}
	switch u.Tipo {
	case Regla, Tarea, BloqueOpaco:
	case Parrafo, Titulo, Cita, Codigo, Vineta, Numerada:
		if len(u.Tramos) == 0 {
			d.Bloques = d.Bloques[:n-1]
		}
	}
}

// añadirTexto mete un tramo en el bloque abierto, creando un párrafo si no hay ninguno
// (texto suelto en la raíz, que el editor de la web produce con la primera línea).
func (d *Documento) añadirTexto(s string, e Tramo) {
	if s == "" {
		return
	}
	d.asegurarBloque()
	e.Texto = s
	b := &d.Bloques[len(d.Bloques)-1]
	b.Tramos = append(b.Tramos, e)
}

// añadirSalto mete un `<br/>` conservando las marcas de donde aparece: un salto dentro
// de una negrita sigue estando dentro de la negrita.
func (d *Documento) añadirSalto(e Tramo) {
	d.asegurarBloque()
	e.Salto = true
	b := &d.Bloques[len(d.Bloques)-1]
	b.Tramos = append(b.Tramos, e)
}

// asegurarBloque abre un párrafo SIN ENVOLTORIO para el texto suelto de la raíz, o
// para el que sigue a una tabla (el último bloque es de una celda, y no es su sitio).
//
// `Etiqueta` vacía es «esto no venía dentro de nada», y al reescribir no se le pone
// nada: la nota `a<br>b` del saneador no tiene `<p>`, y envolverla la cambiaba. Se
// pinta igual que un párrafo, que es lo que hace el navegador con el texto anónimo.
func (d *Documento) asegurarBloque() {
	if n := len(d.Bloques); n == 0 || d.Bloques[n-1].Celda != d.celda {
		d.Bloques = append(d.Bloques, Bloque{Tipo: Parrafo, Celda: d.celda})
	}
}

// Fiel dice si el documento se puede volver a escribir sin perder nada, y por lo tanto
// si se puede GUARDAR. Un `false` no es un error de la nota: es esta vista reconociendo
// que no la entiende entera.
func (d *Documento) Fiel() bool { return d != nil && d.fiel }

// NoPintado son las claves de catálogo de lo que la nota trae y esta vista no dibuja con
// su forma, sin repetir y en orden estable. Vacío: lo que se ve es lo que hay.
func (d *Documento) NoPintado() []string {
	if d == nil {
		return nil
	}
	return slices.Clone(d.noPintado)
}

// Vacio dice si no hay nada que pintar.
func (d *Documento) Vacio() bool { return d == nil || len(d.Bloques) == 0 }

// Original es el HTML tal como llegó. Es lo que se manda al servicio cuando la nota no
// se ha tocado: ni siquiera el paso por el serializador, que normaliza comillas.
func (d *Documento) Original() string {
	if d == nil {
		return ""
	}
	return d.original
}

// HTML vuelve a escribir el documento DESDE LOS BLOQUES. Es la vuelta del viaje, y es
// lo que se manda al servicio cuando la nota se ha editado.
func (d *Documento) HTML() string {
	if d == nil || len(d.Bloques) == 0 {
		return ""
	}
	var b strings.Builder
	escribirBloques(&b, d.Bloques)
	return b.String()
}

// escribirBloques escribe una lista de bloques: la del documento o la de una celda.
func escribirBloques(b *strings.Builder, bloques []Bloque) {
	// Las listas ANIDADAS: una pila con la lista abierta de cada nivel y si su último
	// `<li>` sigue abierto, porque una lista más honda va DENTRO de ese `<li>`
	// (`<ul><li>a<ul><li>b</li></ul></li></ul>`). Sangrar en el editor es subir el nivel,
	// y sin esto la nota dejaba de ser fiel.
	type abierta struct {
		etiqueta string
		li       bool
	}
	var pila []abierta
	cerrarUna := func() {
		a := pila[len(pila)-1]
		if a.li {
			b.WriteString("</li>")
		}
		b.WriteString("</" + a.etiqueta + ">")
		pila = pila[:len(pila)-1]
	}
	for i := 0; i < len(bloques); i++ {
		bl := bloques[i]
		if bl.Celda.EnTabla() {
			for len(pila) > 0 {
				cerrarUna()
			}
			j := i + 1
			for j < len(bloques) && bloques[j].Celda.Tabla == bl.Celda.Tabla {
				j++
			}
			escribirTabla(b, bloques[i:j])
			i = j - 1
			continue
		}
		quiere := ""
		switch bl.Tipo {
		case Vineta:
			quiere = "ul"
		case Numerada:
			quiere = "ol"
		case Parrafo, Titulo, Cita, Codigo, Tarea, Regla, BloqueOpaco:
		}
		if quiere == "" {
			for len(pila) > 0 {
				cerrarUna()
			}
			escribirBloque(b, bl)
			continue
		}
		nivel := max(bl.Nivel, 1)
		for len(pila) > nivel {
			cerrarUna()
		}
		if len(pila) == nivel {
			if pila[nivel-1].etiqueta != quiere {
				cerrarUna()
			} else if pila[nivel-1].li {
				b.WriteString("</li>")
				pila[nivel-1].li = false
			}
		}
		for len(pila) < nivel {
			b.WriteString("<" + quiere + ">")
			pila = append(pila, abierta{etiqueta: quiere})
		}
		escribirElemento(b, bl)
		pila[len(pila)-1].li = true
	}
	for len(pila) > 0 {
		cerrarUna()
	}
}

// escribirTabla escribe los bloques de UNA tabla con su forma. Una celda que se hubiera
// quedado sin bloques sale vacía, que es lo que era.
func escribirTabla(b *strings.Builder, run []Bloque) {
	t := run[0].Celda.Tabla
	porCelda := map[[2]int][]Bloque{}
	for _, bl := range run {
		k := [2]int{bl.Celda.Fila, bl.Celda.Col}
		bl.Celda = Celda{}
		porCelda[k] = append(porCelda[k], bl)
	}
	b.WriteString("<table" + atributos(t.Atributos) + ">")
	seccion := ""
	for f, fila := range t.Filas {
		if fila.Nueva || seccion == "" {
			if seccion != "" {
				b.WriteString("</" + seccion + ">")
			}
			seccion = cmp.Or(fila.Seccion, "tbody")
			b.WriteString("<" + seccion + atributos(fila.AtributosSeccion) + ">")
		}
		b.WriteString("<tr" + atributos(fila.Atributos) + ">")
		for c, celda := range fila.Celdas {
			etiqueta := "td"
			if celda.Cabecera {
				etiqueta = "th"
			}
			b.WriteString("<" + etiqueta + atributos(celda.Atributos) + ">")
			escribirBloques(b, porCelda[[2]int{f, c}])
			b.WriteString("</" + etiqueta + ">")
		}
		b.WriteString("</tr>")
	}
	if seccion != "" {
		b.WriteString("</" + seccion + ">")
	}
	b.WriteString("</table>")
}

// atributos escribe los atributos de un elemento tal como venían.
func atributos(as []html.Attribute) string {
	var b strings.Builder
	for _, a := range as {
		b.WriteString(" " + a.Key + `="` + html.EscapeString(a.Val) + `"`)
	}
	return b.String()
}

// escribirElemento abre un `<li>` con su texto y lo deja ABIERTO: lo cierra quien sabe si
// detrás viene una lista más honda que va dentro.
func escribirElemento(b *strings.Builder, bl Bloque) {
	b.WriteString("<" + bl.Etiqueta)
	if bl.Alineacion != "" {
		b.WriteString(` style="text-align:` + html.EscapeString(bl.Alineacion) + `"`)
	}
	if bl.Plegada {
		b.WriteString(` class="` + claseplegada + `"`)
	}
	b.WriteString(">")
	for _, t := range bl.Tramos {
		escribirTramo(b, t)
	}
}

// escribirBloque escribe UN bloque con la etiqueta y los atributos que tenía.
func escribirBloque(b *strings.Builder, bl Bloque) {
	switch bl.Tipo {
	case BloqueOpaco:
		b.WriteString(bl.Crudo)
		return
	case Regla:
		b.WriteString("<hr/>")
		return
	case Tarea:
		b.WriteString(`<div class="task-row"><input type="checkbox"`)
		if bl.Marcada {
			b.WriteString(" checked")
		}
		if bl.Bloqueada {
			b.WriteString(" disabled")
		}
		b.WriteString("/>")
		for _, t := range bl.Tramos {
			escribirTramo(b, t)
		}
		b.WriteString("</div>")
		return
	case Parrafo, Titulo, Cita, Codigo, Vineta, Numerada:
	}
	if bl.Etiqueta == "" { // texto suelto de la raíz: sin envoltorio
		for _, t := range bl.Tramos {
			escribirTramo(b, t)
		}
		return
	}
	b.WriteString("<" + bl.Etiqueta)
	if bl.Alineacion != "" {
		b.WriteString(` style="text-align:` + html.EscapeString(bl.Alineacion) + `"`)
	}
	b.WriteString(">")
	for _, t := range bl.Tramos {
		escribirTramo(b, t)
	}
	b.WriteString("</" + bl.Etiqueta + ">")
}

func escribirTramo(b *strings.Builder, t Tramo) {
	if t.Salto {
		b.WriteString("<br/>")
		return
	}
	// `a` por fuera y `span` por dentro, que es el orden en que sale del saneador.
	if t.Enlace != "" {
		b.WriteString(`<a href="` + html.EscapeString(t.Enlace) + `"`)
		if t.Externo {
			b.WriteString(` target="_blank" rel="noopener noreferrer"`)
		}
		b.WriteString(">")
	}
	for _, m := range t.Marcas {
		b.WriteString("<" + m + ">")
	}
	if t.EnSpan {
		b.WriteString("<span" + estiloDe(t) + ">")
	}
	if t.Imagen != nil {
		b.WriteString("<img" + atributos(t.Imagen) + "/>")
	} else {
		b.WriteString(html.EscapeString(t.Texto))
	}
	if t.EnSpan {
		b.WriteString("</span>")
	}
	for i := len(t.Marcas) - 1; i >= 0; i-- {
		b.WriteString("</" + t.Marcas[i] + ">")
	}
	if t.Enlace != "" {
		b.WriteString("</a>")
	}
}

// estiloDe reescribe el `style` de un span con las propiedades del modelo, EN SU ORDEN:
// `color` y `background-color`, que es el de `estiloPermitido` del servicio de notas, y
// detrás `font-family` y `font-size`, que solo escribe Mail.
func estiloDe(t Tramo) string {
	var partes []string
	for _, p := range [][2]string{{"color", t.Color}, {"background-color", t.Fondo}, {"font-family", t.Fuente}, {"font-size", t.Tamano}} {
		if p[1] != "" {
			partes = append(partes, p[0]+":"+p[1])
		}
	}
	if len(partes) == 0 {
		return ""
	}
	return ` style="` + html.EscapeString(strings.Join(partes, ";")) + `"`
}

// normalizar pasa un HTML por el analizador y lo vuelve a escribir, para comparar dos
// documentos como HTML y no como cadenas: así las comillas de atributo, `<br>` contra
// `<br/>` y las entidades dejan de contar, que es lo que hace el navegador.
func normalizar(s string) string {
	nodos, err := fragmento(s)
	if err != nil {
		return s
	}
	var b strings.Builder
	for _, n := range nodos {
		if html.Render(&b, n) != nil {
			return s
		}
	}
	return b.String()
}

// enOrdenada dice si el `li` cuelga de un `<ol>`.
func enOrdenada(li *html.Node) bool {
	for p := li.Parent; p != nil; p = p.Parent {
		switch p.Data {
		case "ol":
			return true
		case "ul":
			return false
		}
	}
	return false
}

// llevaTexto dice si dentro de ese elemento el blanco cuenta como texto. Es la lista de
// los que en la web tienen contenido de flujo; `ul`, `ol` y los de tabla, no.
func llevaTexto(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "ul", "ol", "table", "thead", "tbody", "tr":
		return false
	}
	return true
}

// alineacion saca el `text-align` del `style`, que es lo único que el saneador deja en
// un bloque.
func alineacion(n *html.Node) string { return propiedad(atributo(n, "style"), "text-align") }

// propiedad busca una declaración en un `style` ya saneado. Se compara el nombre
// COMPLETO: con un `strings.Contains`, buscar `color` devolvería el fondo.
func propiedad(estilo, nombre string) string {
	for decl := range strings.SplitSeq(estilo, ";") {
		prop, valor, ok := strings.Cut(decl, ":")
		if ok && strings.TrimSpace(prop) == nombre {
			return strings.TrimSpace(valor)
		}
	}
	return ""
}

func atributo(n *html.Node, nombre string) string {
	for _, a := range n.Attr {
		if a.Key == nombre {
			return a.Val
		}
	}
	return ""
}

func tiene(n *html.Node, nombre string) bool {
	return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool { return a.Key == nombre })
}

// interior serializa los HIJOS de un nodo, para sacarles el texto con `htmltext`.
func interior(n *html.Node) string {
	var b strings.Builder
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if html.Render(&b, h) != nil {
			return ""
		}
	}
	return b.String()
}

// serializar escribe un nodo ENTERO, con su etiqueta: es lo que se guarda en `Crudo`.
func serializar(n *html.Node) string {
	var b strings.Builder
	if html.Render(&b, n) != nil {
		return ""
	}
	return b.String()
}
