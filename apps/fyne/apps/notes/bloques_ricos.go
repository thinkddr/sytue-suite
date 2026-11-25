// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// LOS BLOQUES RICOS DE UNA NOTA: paneles, desplegables, etiquetas de estado, decisiones,
// fechas y el lenguaje de un bloque de código. El FORMATO es el que la web y el escritorio
// comparten, y es vinculante —así se guarda la nota y así vuelve—:
//
//	panel        <div data-panel="info|nota|exito|aviso|error">…</div>
//	desplegable  <details data-expandir [open]><summary>Título</summary>…</details>
//	estado       <span data-estado="gris|azul|verde|amarillo|rojo|morado">TEXTO</span>
//	decisiones   <ul data-tipo="decisiones"><li>…</li></ul>
//	fecha        <time datetime="AAAA-MM-DD">texto</time>
//	código       <pre [data-lineas] [data-ajuste]><code data-lenguaje="go">…</code></pre>
//	tabla        <table [data-numerada]>; <thead> la fila de cabecera; <th scope="row">
//	             la columna de cabecera; data-fondo="gris|azul|…" el fondo de una celda;
//	             colspan y rowspan, las celdas combinadas (combinar.go)
//	columnas     <div data-columnas="2|3"><div data-columna>…</div>…</div> (columnas.go)
//	subíndice    <sub>…</sub>, y superíndice <sup>…</sup>, como cualquier otra marca
//	imagen       <img src="adjunto:ID" alt="nombre"> y fichero <a href="adjunto:ID">,
//	             los adjuntos de la nota (adjuntos_cuerpo.go)
//
// Las tareas NO cambian: siguen siendo `<div class="task-row"><input type="checkbox">
// <span>…</span></div>`, que es lo que el modelo, la web y el servicio ya escriben.
//
// Todos los valores son listas cerradas: lo que no está en ellas no se lee (el atributo se
// cae, la nota deja de ser fiel y se abre en solo lectura, como cualquier HTML que esta
// vista no entiende).
//
// Un panel y un desplegable son una CAJA: como una tabla, no es un bloque sino algo que
// comparten los bloques de dentro (`Bloque.Caja`), así que dentro se escribe, se pega y se
// deshace igual que fuera. Intro en la línea vacía del final sale de la caja y Retroceso
// al principio de su primera línea, también.

// Las listas cerradas de valores.
var (
	PanelesPermitidos   = []string{"info", "nota", "exito", "aviso", "error"}
	EstadosPermitidos   = []string{"gris", "azul", "verde", "amarillo", "rojo", "morado"}
	FondosPermitidos    = EstadosPermitidos
	LenguajesPermitidos = []string{
		"texto", "bash", "c", "cpp", "csharp", "css", "go", "html", "java", "javascript",
		"json", "kotlin", "markdown", "php", "python", "ruby", "rust", "sql", "swift",
		"typescript", "xml", "yaml",
	}
)

// ListaDecisiones es el `data-tipo` de una lista de decisiones.
const ListaDecisiones = "decisiones"

// Los dos tipos de caja.
const (
	CajaPanel    = "panel"
	CajaExpandir = "expandir"
)

// Caja es un panel o un desplegable. No se cambia en su sitio: las fotos de deshacer la
// comparten, así que cambiar el título es hacer una caja nueva.
type Caja struct {
	Tipo string
	// Valor es el tipo de panel (`info`, `nota`…); en un desplegable, vacío.
	Valor string
	// Titulo es el `<summary>` de un desplegable, en texto plano.
	Titulo string
	// Abierta es el `open` de un desplegable: si al leer la nota se ve lo de dentro. La
	// web lo pone al insertarlo y lo cambia al plegarlo; escribiendo, se ve siempre.
	Abierta bool
}

// cajaDe dice si un elemento abre una caja: un `div[data-panel]` con un valor de la
// lista o un `details[data-expandir]`.
func cajaDe(n *html.Node) *Caja {
	switch {
	case n.DataAtom == atom.Div && tiene(n, "data-panel"):
		if v := atributo(n, "data-panel"); slices.Contains(PanelesPermitidos, v) {
			return &Caja{Tipo: CajaPanel, Valor: v}
		}
	case n.DataAtom == atom.Details && tiene(n, "data-expandir"):
		return &Caja{Tipo: CajaExpandir, Abierta: tiene(n, "open")}
	}
	return nil
}

// leerCaja lee lo de dentro de una caja con ella puesta. El `<summary>` de un desplegable
// es su título, no una línea.
func (d *Documento) leerCaja(n *html.Node, c *Caja) {
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.DataAtom == atom.Summary && c.Tipo == CajaExpandir {
			c.Titulo = textoPlano(h)
		}
	}
	antes := d.caja
	d.cerrarSuelto(d.celda)
	d.caja = c
	n0 := len(d.Bloques)
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.DataAtom == atom.Summary {
			continue
		}
		d.recorrer(h, Tramo{}, 0)
	}
	d.cerrarSuelto(d.celda)
	if len(d.Bloques) == n0 {
		// Una caja vacía conserva su línea: sin ella no habría dónde escribir.
		d.Bloques = append(d.Bloques, Bloque{Tipo: Parrafo, Etiqueta: "p", Caja: c})
	}
	d.caja = antes
}

// textoPlano es el texto de un nodo y sus hijos.
func textoPlano(n *html.Node) string {
	var b strings.Builder
	var ir func(*html.Node)
	ir = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for h := x.FirstChild; h != nil; h = h.NextSibling {
			ir(h)
		}
	}
	ir(n)
	return b.String()
}

// esCodigoConLenguaje dice si un `<code>` es el que lleva el lenguaje de su `<pre>`.
func esCodigoConLenguaje(n *html.Node) bool {
	p := n.Parent
	return p != nil && p.DataAtom == atom.Pre && slices.Contains(LenguajesPermitidos, atributo(n, "data-lenguaje")) &&
		unicoHijo(p) == n
}

// lenguajeDe es el lenguaje de un `<pre>`: el de su único `<code data-lenguaje>`.
func lenguajeDe(pre *html.Node) string {
	if c := unicoHijo(pre); c != nil && c.DataAtom == atom.Code {
		if v := atributo(c, "data-lenguaje"); slices.Contains(LenguajesPermitidos, v) {
			return v
		}
	}
	return ""
}

// unicoHijo es el único hijo elemento de un nodo, sin contar blancos; nil si hay otra cosa.
func unicoHijo(n *html.Node) *html.Node {
	var uno *html.Node
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		switch {
		case h.Type == html.TextNode && strings.TrimSpace(h.Data) == "":
		case h.Type == html.ElementNode && uno == nil:
			uno = h
		default:
			return nil
		}
	}
	return uno
}

// EsFecha dice si un `datetime` es una fecha AAAA-MM-DD de verdad.
func EsFecha(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil && len(s) == len(time.DateOnly)
}

// escribirCaja escribe los bloques de UNA caja dentro de su elemento.
func escribirCaja(b *strings.Builder, run []Bloque) {
	c := run[0].Caja
	dentro := make([]Bloque, len(run))
	for i, bl := range run {
		bl.Caja = nil
		dentro[i] = bl
	}
	switch c.Tipo {
	case CajaExpandir:
		abierta := ""
		if c.Abierta {
			abierta = ` open=""` // detrás de `data-expandir`, como lo escribe la web
		}
		b.WriteString(`<details data-expandir=""` + abierta + `><summary>` + html.EscapeString(c.Titulo) + `</summary>`)
		escribirBloques(b, dentro)
		b.WriteString("</details>")
	default:
		b.WriteString(`<div data-panel="` + html.EscapeString(c.Valor) + `">`)
		escribirBloques(b, dentro)
		b.WriteString("</div>")
	}
}

// ultimoDeCaja dice si el bloque `i` es el último de su caja.
func (d *Documento) ultimoDeCaja(i int) bool {
	return i+1 >= len(d.Bloques) || d.Bloques[i+1].Caja != d.Bloques[i].Caja
}

// rangoDeCaja son los bloques [desde, hasta) de la caja del bloque `i`.
func (d *Documento) rangoDeCaja(i int) (int, int) {
	c := d.Bloques[i].Caja
	desde, hasta := i, i+1
	for desde > 0 && d.Bloques[desde-1].Caja == c {
		desde--
	}
	for hasta < len(d.Bloques) && d.Bloques[hasta].Caja == c {
		hasta++
	}
	return desde, hasta
}

// PonerCaja mete en una caja nueva los bloques de la selección (los que no son de una
// tabla). Si ya están todos en una caja del mismo tipo y valor, la quita: es un
// conmutador, como las listas. Sin nada que meter (una nota vacía), crea la línea.
func (d *Documento) PonerCaja(a, b Pos, tipo, valor string) {
	if len(d.Bloques) == 0 {
		d.Bloques = []Bloque{{Tipo: Parrafo, Etiqueta: "p"}}
	}
	a, b = ordenar(a, b)
	quitar := true
	d.bloques(a, b, func(bl *Bloque) {
		quitar = quitar && bl.Caja != nil && bl.Caja.Tipo == tipo && bl.Caja.Valor == valor
	})
	// Un desplegable nace abierto, como el de la web (`plantillaExpandir`).
	c := &Caja{Tipo: tipo, Valor: valor, Abierta: tipo == CajaExpandir}
	d.bloques(a, b, func(bl *Bloque) {
		if bl.Celda.EnTabla() && !bl.Celda.Tabla.EsColumnas() {
			return
		}
		if quitar {
			bl.Caja = nil
			return
		}
		bl.Caja = c
		if bl.Tipo == Parrafo && bl.Etiqueta == "" {
			bl.Etiqueta = "p" // dentro de una caja no hay texto suelto
		}
	})
}

// TitularCaja cambia el título del desplegable del bloque `i`: una caja nueva para todos
// sus bloques, que la vieja la comparten las fotos de deshacer.
func (d *Documento) TitularCaja(i int, titulo string) {
	d.cambiarCaja(i, func(c *Caja) { c.Titulo = titulo })
}

// AbrirCaja pone o quita el `open` del desplegable del bloque `i`: si al leer la nota
// se ve abierto.
func (d *Documento) AbrirCaja(i int, abierta bool) {
	d.cambiarCaja(i, func(c *Caja) { c.Abierta = abierta })
}

// cambiarCaja hace una caja nueva para todos los bloques de la del bloque `i`, con el
// cambio puesto.
func (d *Documento) cambiarCaja(i int, cambio func(*Caja)) {
	if i < 0 || i >= len(d.Bloques) || d.Bloques[i].Caja == nil {
		return
	}
	nueva := *d.Bloques[i].Caja
	cambio(&nueva)
	desde, hasta := d.rangoDeCaja(i)
	for k := desde; k < hasta; k++ {
		d.Bloques[k].Caja = &nueva
	}
}

// AlternarDecisiones convierte la selección en una lista de decisiones, o la saca de ella
// si ya lo es entera.
func (d *Documento) AlternarDecisiones(a, b Pos) {
	todas := true
	d.bloques(a, b, func(bl *Bloque) { todas = todas && bl.Tipo == Vineta && bl.Lista == ListaDecisiones })
	if todas {
		d.AlternarLista(a, b, Vineta) // ya son viñetas: salen de la lista
		d.bloques(a, b, func(bl *Bloque) { bl.Lista = "" })
		return
	}
	d.bloques(a, b, func(bl *Bloque) {
		if bl.Tipo == Regla || bl.Tipo == BloqueOpaco {
			return
		}
		if bl.Tipo == Tarea {
			quitarLetras(bl, 0, inicioEditable(*bl))
			for i := range bl.Tramos {
				bl.Tramos[i].EnSpan = false
			}
		}
		if bl.Tipo != Vineta {
			bl.Nivel = max(bl.Nivel, 1)
		}
		bl.Tipo, bl.Etiqueta, bl.Lista, bl.Lenguaje = Vineta, "li", ListaDecisiones, ""
		bl.Nivel = max(bl.Nivel, 1)
		limpiar(bl)
	})
}

// PonerLenguaje cambia el lenguaje del bloque de código `i` (vacío: ninguno).
func (d *Documento) PonerLenguaje(i int, lenguaje string) {
	if i < 0 || i >= len(d.Bloques) || d.Bloques[i].Tipo != Codigo {
		return
	}
	if lenguaje != "" && !slices.Contains(LenguajesPermitidos, lenguaje) {
		return
	}
	d.Bloques[i].Lenguaje = lenguaje
}

// InsertarEtiqueta escribe en `p` un tramo especial —una etiqueta de estado o una
// fecha— y, detrás, un espacio duro corriente para seguir escribiendo fuera de él (lo
// mismo que un enlace). Devuelve dónde queda el cursor.
func (d *Documento) InsertarEtiqueta(p Pos, texto string, especial Tramo) Pos {
	base := Tramo{}
	if len(d.Bloques) > 0 && p.B < len(d.Bloques) {
		base = estiloEn(d.Bloques[p.B], p.O)
		base.Estado, base.Fecha = "", ""
	}
	especial.Marcas, especial.EnSpan, especial.Enlace, especial.Externo = base.Marcas, base.EnSpan, base.Enlace, base.Externo
	fin := d.Insertar(p, texto, especial)
	return d.Insertar(fin, " ", base)
}

// CambiarEtiqueta cambia el color (estado) o la fecha y el texto del tramo especial que
// cubre la letra `o` del bloque `b`.
func (d *Documento) CambiarEtiqueta(p Pos, texto string, cambiar func(*Tramo)) {
	a, z, ok := d.tramoEspecial(p)
	if !ok {
		return
	}
	estilo := estiloEn(d.Bloques[p.B], a.O+1)
	cambiar(&estilo)
	d.Borrar(a, z)
	d.Insertar(a, texto, estilo)
}

// tramoEspecial son los extremos de la etiqueta de estado o la fecha que cubre la letra de
// delante de `p` (o la de detrás).
func (d *Documento) tramoEspecial(p Pos) (Pos, Pos, bool) {
	if p.B < 0 || p.B >= len(d.Bloques) {
		return p, p, false
	}
	pos := 0
	for _, t := range d.Bloques[p.B].Tramos {
		n := largoTramo(t)
		if (t.Estado != "" || t.Fecha != "") && p.O >= pos && p.O <= pos+n {
			return Pos{p.B, pos}, Pos{p.B, pos + n}, true
		}
		pos += n
	}
	return p, p, false
}

// especialEn es el tramo especial (estado o fecha) que cubre `p`, si lo hay.
func (d *Documento) especialEn(p Pos) (Tramo, bool) {
	a, _, ok := d.tramoEspecial(p)
	if !ok {
		return Tramo{}, false
	}
	return estiloEn(d.Bloques[p.B], a.O+1), true
}

// atributosCelda son los de una celda sin lo que la lista cerrada no permite: un
// `data-fondo` de otro color o un `scope` que no sea `row` se caen.
func atributosCelda(as []html.Attribute) []html.Attribute {
	return slices.DeleteFunc(slices.Clone(as), func(a html.Attribute) bool {
		return a.Key == "data-fondo" && !slices.Contains(FondosPermitidos, a.Val) ||
			a.Key == "scope" && a.Val != "row"
	})
}

// fondoDe es el `data-fondo` de una celda, si lo trae.
func fondoDe(c CeldaTabla) string { return attr(c.Atributos, "data-fondo") }

// conAtributo devuelve los atributos con `k` puesto a `v` (o quitado, si `v` está vacío),
// en una lista nueva.
func conAtributo(as []html.Attribute, k, v string) []html.Attribute {
	out := slices.DeleteFunc(slices.Clone(as), func(a html.Attribute) bool { return a.Key == k })
	if v != "" {
		out = append(out, html.Attribute{Key: k, Val: v})
	}
	return out
}

// corrida dice si el bloque `i` abre una caja o una tabla: hasta dónde llegan sus bloques
// y con qué se escriben. Sin nada de eso, nil.
func corrida(bloques []Bloque, i int) (int, func(*strings.Builder, []Bloque)) {
	bl := bloques[i]
	mismo := func(o Bloque) bool { return o.Caja == bl.Caja }
	escribir := escribirCaja
	switch {
	case bl.Caja != nil && !bl.Celda.EnTabla():
	case bl.Celda.EnTabla():
		mismo = func(o Bloque) bool { return o.Celda.Tabla == bl.Celda.Tabla }
		escribir = escribirTabla
	default:
		return i, nil
	}
	j := i + 1
	for j < len(bloques) && mismo(bloques[j]) {
		j++
	}
	return j, escribir
}

// conMarcaLeida añade la marca de un elemento de formato a lo heredado. El `<code
// data-lenguaje>` de un bloque de código no es una marca: es el lenguaje del `<pre>`,
// que ya lo ha leído.
func conMarcaLeida(n *html.Node, e Tramo) Tramo {
	if n.Data == "code" && esCodigoConLenguaje(n) {
		return e
	}
	// `slices.Concat` y no `append`: `e` se pasa por valor a los hermanos y un `append`
	// sobre el mismo respaldo le metería a uno la marca del otro.
	e.Marcas = slices.Concat(e.Marcas, []string{n.Data})
	return e
}

// tramoDeLinea es lo que un `<time>` o un `<span>` ponen a lo heredado: la fecha, el
// estado o, en un `span` corriente, su estilo.
func tramoDeLinea(n *html.Node, e Tramo) Tramo {
	if n.DataAtom == atom.Time {
		if f := atributo(n, "datetime"); EsFecha(f) {
			e.Fecha = f
		}
		return e
	}
	if v := atributo(n, "data-estado"); v != "" {
		if slices.Contains(EstadosPermitidos, v) {
			e.Estado = v
		}
		return e
	}
	e.EnSpan = true
	st := atributo(n, "style")
	e.Color, e.Fondo = propiedad(st, "color"), propiedad(st, "background-color")
	e.Fuente, e.Tamano = propiedad(st, "font-family"), propiedad(st, "font-size")
	return e
}
