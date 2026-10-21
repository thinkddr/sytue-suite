// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// ESCRIBIR en el folio: el cursor, la selección, el teclado, el ratón, el portapapeles, el
// historial y los comandos de la cinta. Es lo que en la web hace el navegador con el
// contenteditable y `exec.ts` con `execCommand`.
//
// Todo pasa por el modelo (`rico`) y nada por el pintado: cada cambio edita el árbol del
// HTML, se vuelve a maquetar y se repinta. Así lo que se ve es siempre lo que se guarda.

// punto es una posición del cursor: el bloque y la runa dentro de él.
type punto struct{ b, p int }

func (a punto) antes(b punto) bool { return a.b < b.b || (a.b == b.b && a.p < b.p) }

// PausaParpadeo es lo que el cursor pasa encendido y apagado, como el de Chrome.
const PausaParpadeo = 500 * time.Millisecond

// PausaFormato es el `FMT_THROTTLE_MS` de `EditorApp.tsx`: la cinta refleja el formato
// bajo el cursor como mucho cada 100 ms, no en cada tecla.
const PausaFormato = 100 * time.Millisecond

// historial es el deshacer y rehacer: fotos del HTML con su cursor. Las teclas seguidas
// se agrupan en un paso, como en el navegador.
type historial struct {
	atras, adelante []paso
	escribiendo     bool // el último paso fue teclear: lo siguiente tecleado va con él
	ultima          time.Time
}

type paso struct {
	html     string
	cur, anc punto
}

// maxPasos acota el historial: cada paso es el documento entero.
// ponytail: fotos completas, no diferencias. Con documentos de decenas de KB son unos
// MB como mucho; si pesa, se guardan solo los bloques tocados.
const maxPasos = 200

// PausaAgrupar es cuánto puede tardar la siguiente tecla en ir al mismo paso.
const PausaAgrupar = time.Second

// ---- selección ----

func (f *Folio) haySeleccion() bool { return f.cur != f.anc }

// orden devuelve la selección con el principio delante.
func (f *Folio) orden() (punto, punto) {
	if f.anc.antes(f.cur) {
		return f.anc, f.cur
	}
	return f.cur, f.anc
}

// largo es el número de runas del bloque i.
func (f *Folio) largo(i int) int {
	if i < 0 || i >= len(f.cajas) {
		return 0
	}
	return f.cajas[i].largo
}

// deTexto dice si el bloque i lleva texto donde se puede poner el cursor.
func (f *Folio) deTexto(i int) bool {
	return i >= 0 && i < len(f.bloques) && f.bloques[i].Tipo.EsTexto() && !f.bloques[i].Oculta
}

// acotar deja el punto dentro del documento y en un bloque de texto.
func (f *Folio) acotar(q punto) punto {
	if len(f.bloques) == 0 {
		return punto{}
	}
	q.b = min(max(q.b, 0), len(f.bloques)-1)
	for i := q.b; i < len(f.bloques); i++ { // hacia abajo, y si no hay, hacia arriba
		if f.deTexto(i) {
			if i != q.b {
				q.p = 0
			}
			q.b = i
			break
		}
	}
	for !f.deTexto(q.b) && q.b > 0 {
		q.b--
		q.p = f.largo(q.b)
	}
	q.p = min(max(q.p, 0), f.largo(q.b))
	return q
}

// edicion abre el bloque del cursor para editarlo; la conserva mientras el cursor siga en
// él para no perder la marca pendiente.
func (f *Folio) edicion(i int) *rico.Edicion {
	if i == f.cur.b && f.ed != nil {
		return f.ed
	}
	if i < 0 || i >= len(f.bloques) {
		return nil
	}
	e := f.bloques[i].Editar()
	if i == f.cur.b {
		f.ed = e
	}
	return e
}

// mover pone el cursor en q; con `extender` la selección sigue desde donde empezó.
func (f *Folio) mover(q punto, extender bool) {
	q = f.acotar(q)
	if q.b != f.cur.b {
		f.ed = nil
	} else if f.ed != nil && q != f.cur {
		f.ed = nil // la marca pendiente es para escribir AQUÍ; al moverse se olvida
	}
	f.cur, f.conCursor = q, true
	if !extender {
		f.anc = q
	}
	f.hist.escribiendo = false
	f.caretVisible = true
	f.pag.Refresh()
	f.asegurarVisible()
	f.programarFormato()
}

// ---- cambios ----

// antesDeCambiar guarda la foto para deshacer. Las teclas seguidas van al mismo paso.
func (f *Folio) antesDeCambiar(escribir bool) { f.apuntar(paso{f.doc.HTML(), f.cur, f.anc}, escribir) }

// apuntar es antesDeCambiar con una foto ya tomada: la de antes de un cambio que solo se
// sabe si ha cambiado algo después de hacerlo.
func (f *Folio) apuntar(foto paso, escribir bool) {
	ahora := time.Now()
	if escribir && f.hist.escribiendo && ahora.Sub(f.hist.ultima) < PausaAgrupar {
		f.hist.ultima = ahora
		return
	}
	f.hist.atras = append(f.hist.atras, foto)
	if len(f.hist.atras) > maxPasos {
		f.hist.atras = f.hist.atras[1:]
	}
	f.hist.adelante = nil
	f.hist.escribiendo, f.hist.ultima = escribir, ahora
}

// cambiado se llama tras cada edición: remaqueta, lleva el cursor a la vista y avisa.
func (f *Folio) cambiado() {
	f.cambios++
	f.remaquetar()
	f.cur, f.anc = f.acotar(f.cur), f.acotar(f.anc)
	f.caretVisible = true
	f.asegurarVisible()
	if f.AlEditar != nil {
		f.AlEditar()
	}
	if f.revisor != nil {
		f.revisado = nil // lo resaltado ya puede no estar donde estaba
		f.revisor.programar()
	}
	f.programarFormato()
}

// reestructurado es cambiado tras una edición que cambia la lista de bloques: la edición
// abierta ya no vale.
func (f *Folio) reestructurado() {
	f.ed = nil
	f.cambiado()
}

// irA pone el cursor en el bloque que ocupa ese lugar del árbol, tras reestructurar.
func (f *Folio) irA(l rico.Lugar, p int) {
	f.bloques = f.doc.Bloques()
	if i := rico.Indice(f.bloques, l); i >= 0 {
		f.cur = punto{i, p}
		f.anc = f.cur
	}
}

// borrarSeleccion quita lo seleccionado, uniendo el primer bloque con el último. En
// control de cambios lo marca como borrado, y el cursor queda detrás.
func (f *Folio) borrarSeleccion() {
	if !f.haySeleccion() {
		return
	}
	if f.rastreando() {
		f.borrarSeleccionRastreada(false)
		return
	}
	a, b := f.orden()
	if a.b == b.b {
		f.edicion(a.b).Sustituir(a.p, b.p, "")
		f.cur, f.anc = a, a
		return
	}
	primero := f.bloques[a.b].Editar()
	ultimo := f.bloques[b.b].Editar()
	if f.deTexto(a.b) {
		primero.Sustituir(a.p, primero.Largo(), "")
	}
	if f.deTexto(b.b) {
		ultimo.Sustituir(0, b.p, "")
	}
	// Los de en medio, del último al primero: quitar uno no mueve los nodos de los demás.
	for i := b.b - 1; i > a.b; i-- {
		f.doc.Quitar(f.bloques, i)
	}
	f.bloques = f.doc.Bloques()
	i := rico.Indice(f.bloques, ultimo.Lugar)
	f.ed = nil
	if i > 0 {
		if l, en, ok := f.doc.Unir(f.bloques, i); ok {
			f.irA(l, en)
			return
		}
	}
	f.irA(primero.Lugar, a.p)
}

// escribir mete texto donde está el cursor, sustituyendo la selección. Un salto de línea
// parte el bloque, como Intro.
func (f *Folio) escribir(texto string) {
	if !f.editable() || texto == "" {
		return
	}
	if f.enRegion() {
		texto = strings.NewReplacer("\r\n", " ", "\n", " ").Replace(texto) // la región es UNA línea
	}
	f.antesDeCambiar(!f.haySeleccion() && !strings.Contains(texto, "\n") && utf8.RuneCountInString(texto) == 1)
	f.borrarSeleccion()
	for i, trozo := range strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n") {
		if i > 0 {
			f.partir()
		}
		if trozo == "" {
			continue
		}
		if e := f.edicion(f.cur.b); f.rastreando() {
			e.InsertarRastreado(f.cur.p, trozo, f.autor(), sello())
		} else {
			e.Sustituir(f.cur.p, f.cur.p, trozo)
		}
		f.cur.p += utf8.RuneCountInString(trozo)
		f.anc = f.cur
	}
	f.cambiado()
}

// partir es Intro: el bloque se parte en dos y el cursor va al principio del nuevo.
func (f *Folio) partir() {
	l, nuevo := f.edicion(f.cur.b).Partir(f.cur.p)
	if nuevo {
		f.irA(l, 0)
		f.ed = nil
		return
	}
	// Una línea suelta, sin elemento que repetir: Intro es un salto de línea.
	f.cur.p++
	f.anc = f.cur
}

// intro es la tecla: con Mayús, un salto de línea dentro del bloque (`<br>`).
func (f *Folio) intro() {
	if !f.editable() || f.enRegion() {
		return // en el encabezado y el pie, Intro no parte la región
	}
	f.antesDeCambiar(false)
	f.borrarSeleccion()
	if f.mayus {
		// Al final del bloque hacen falta DOS `<br>`: el navegador no pinta el último, y
		// sin el segundo la línea nueva no existiría.
		salto := "\n"
		if f.cur.p == f.largo(f.cur.b) {
			salto = "\n\n"
		}
		f.edicion(f.cur.b).Sustituir(f.cur.p, f.cur.p, salto)
		f.cur.p++
		f.anc = f.cur
		f.cambiado()
		return
	}
	f.partir()
	f.reestructurado()
}

// retroceso borra hacia atrás; al principio de un bloque lo funde con el de arriba (un
// título, una cita o un elemento de lista vuelven antes a ser texto corriente).
func (f *Folio) retroceso() {
	if !f.editable() {
		return
	}
	if f.haySeleccion() {
		f.antesDeCambiar(false)
		if f.rastreando() {
			f.borrarSeleccionRastreada(true)
		} else {
			f.borrarSeleccion()
		}
		f.reestructurado()
		return
	}
	if f.cur.p > 0 && f.rastreando() {
		f.borrarRastreado(f.cur.p-1, f.cur.p, true)
		return
	}
	if f.cur.p > 0 {
		f.antesDeCambiar(true)
		f.edicion(f.cur.b).Sustituir(f.cur.p-1, f.cur.p, "")
		f.cur.p--
		f.anc = f.cur
		f.cambiado()
		return
	}
	f.antesDeCambiar(false)
	if l, en, ok := f.doc.Fundir(f.bloques, f.cur.b); ok {
		f.irA(l, en)
		f.reestructurado()
		return
	}
	f.hist.atras = f.hist.atras[:len(f.hist.atras)-1] // no ha cambiado nada
}

// suprimir borra hacia delante; al final de un bloque le une el de abajo.
func (f *Folio) suprimir() {
	if !f.editable() {
		return
	}
	if f.haySeleccion() && f.rastreando() {
		f.antesDeCambiar(false)
		f.borrarSeleccionRastreada(false)
		f.reestructurado()
		return
	}
	if f.haySeleccion() {
		f.retroceso()
		return
	}
	if f.cur.p < f.largo(f.cur.b) && f.rastreando() {
		f.borrarRastreado(f.cur.p, f.cur.p+1, false)
		return
	}
	if f.cur.p < f.largo(f.cur.b) {
		f.antesDeCambiar(false)
		f.edicion(f.cur.b).Sustituir(f.cur.p, f.cur.p+1, "")
		f.cambiado()
		return
	}
	f.antesDeCambiar(false)
	aqui := f.bloques[f.cur.b].Lugar
	if _, _, ok := f.doc.Unir(f.bloques, f.cur.b+1); ok {
		f.irA(aqui, f.cur.p)
		f.reestructurado()
		return
	}
	f.hist.atras = f.hist.atras[:len(f.hist.atras)-1]
}

// deshacer y rehacer cambian el documento por la foto de antes (o de después).
func (f *Folio) deshacer() { f.viajar(&f.hist.atras, &f.hist.adelante) }
func (f *Folio) rehacer()  { f.viajar(&f.hist.adelante, &f.hist.atras) }

func (f *Folio) viajar(desde, hacia *[]paso) {
	if !f.editable() || len(*desde) == 0 {
		return
	}
	p := (*desde)[len(*desde)-1]
	*desde = (*desde)[:len(*desde)-1]
	*hacia = append(*hacia, paso{f.doc.HTML(), f.cur, f.anc})
	// El modo del control de cambios no se deshace: es de la vista, como el `ui.track` de
	// la web, aunque viaje en la sentinela.
	rastreo := f.doc.Rastreo()
	f.doc = rico.ParseCon(p.html, f.hoja())
	f.doc.PonerRastreo(rastreo)
	f.pagina = paginaDe(f.doc.PaginaDe(), f.pagina) // la página también se deshace: es del documento
	f.cur, f.anc = p.cur, p.anc
	f.hist.escribiendo = false
	f.reestructurado()
}

// ---- la cinta ----

// Mandar ejecuta un comando de la cinta sobre la selección, y devuelve el foco al folio,
// que el botón pulsado se llevó (es el `restoreSelection` de `exec.ts`).
func (f *Folio) Mandar(c editor.Comando, valor string) {
	defer f.enfocar()
	switch c {
	case editor.Deshacer:
		f.deshacer()
		return
	case editor.Rehacer:
		f.rehacer()
		return
	default:
	}
	if !f.editable() {
		return
	}
	if m, ok := marcas[c]; ok {
		f.marcar(m)
		return
	}
	a, b := f.orden()
	switch c {
	case editor.Fuente:
		f.porTramos(func(e *rico.Edicion, d, h int) { e.PonerPropiedad(d, h, "font-family", familiaCSS(valor)) })
	case editor.Tamano:
		f.porTramos(func(e *rico.Edicion, d, h int) { e.PonerPropiedad(d, h, "font-size", tamanoCSS(valor)) })
	case editor.ColorTexto:
		f.porTramos(func(e *rico.Edicion, d, h int) { e.PonerPropiedad(d, h, "color", colorCSS(valor)) })
	case editor.Resaltado:
		f.porTramos(func(e *rico.Edicion, d, h int) { e.PonerPropiedad(d, h, "background-color", colorCSS(valor)) })
	case editor.SinFormato:
		f.porTramos(func(e *rico.Edicion, d, h int) { e.QuitarFormato(d, h) })
	case editor.AlinearIzq, editor.AlinearCen, editor.AlinearDer, editor.Justificar:
		f.porBloques(a, b, func(_ rico.Bloque, e *rico.Edicion) bool { return e.PonerBloque("text-align", alineaciones[c]) })
	case editor.Interlineado:
		f.porBloques(a, b, func(_ rico.Bloque, e *rico.Edicion) bool { return e.PonerBloque("line-height", valor) })
	case editor.MasSangria, editor.MenosSangria:
		f.sangrar(a, b, c == editor.MasSangria)
	case editor.Titulo:
		if valor == "" {
			valor = "p"
		}
		f.porBloques(a, b, func(_ rico.Bloque, e *rico.Edicion) bool { return e.CambiarTipo(valor) })
	case editor.ListaPuntos, editor.ListaNumero:
		f.listas(a, b, c == editor.ListaNumero)
	default:
		f.otraOrden(c, valor)
	}
}

// otraOrden son las órdenes de la imagen seleccionada, de «Encabezado y pie» y de las
// pestañas Insertar y Referencias.
func (f *Folio) otraOrden(c editor.Comando, valor string) {
	if f.enRegion() && !soloEnLinea[c] {
		return // lo que mete bloques partiría la región
	}
	for _, hacer := range []func(editor.Comando, string) bool{f.ordenImagen, f.regiones, f.ordenDecoracion, f.ordenFigura, f.ordenBloque, f.ordenBibliografia, f.ordenCombinacion, f.referencias, f.indicesYLeyendas, f.insertar} {
		if hacer(c, valor) {
			return
		}
	}
}

var marcas = map[editor.Comando]rico.Marca{
	editor.Negrita: rico.Negrita, editor.Cursiva: rico.Cursiva, editor.Subrayado: rico.Subrayado,
	editor.Tachado: rico.Tachado, editor.Superindice: rico.Superindice, editor.Subindice: rico.Subindice,
}

var alineaciones = map[editor.Comando]string{
	editor.AlinearIzq: "left", editor.AlinearCen: "center", editor.AlinearDer: "right", editor.Justificar: "justify",
}

// tramos recorre la selección bloque a bloque con su trozo de runas.
func (f *Folio) tramos(hacer func(i, desde, hasta int)) {
	a, b := f.orden()
	for i := a.b; i <= b.b; i++ {
		if !f.deTexto(i) {
			continue
		}
		desde, hasta := 0, f.largo(i)
		if i == a.b {
			desde = a.p
		}
		if i == b.b {
			hasta = b.p
		}
		hacer(i, desde, hasta)
	}
}

// porTramos aplica un cambio de carácter a cada trozo de la selección. Sin selección
// queda pendiente en el cursor para lo próximo que se escriba.
func (f *Folio) porTramos(hacer func(e *rico.Edicion, desde, hasta int)) {
	f.antesDeCambiar(false)
	if !f.haySeleccion() {
		hacer(f.edicion(f.cur.b), f.cur.p, f.cur.p)
		f.programarFormato()
		return
	}
	f.tramos(func(i, d, h int) { hacer(f.edicion(i), d, h) })
	f.ed = nil
	f.cambiado()
}

// marcar pone o quita una marca en toda la selección a la vez: si la lleva TODO, se
// quita; si no, se pone. Es lo que hace un procesador de textos con una selección mixta.
func (f *Folio) marcar(m rico.Marca) {
	poner := false
	f.tramos(func(i, d, h int) {
		if !f.edicion(i).Tiene(d, h, m) {
			poner = true
		}
	})
	f.porTramos(func(e *rico.Edicion, d, h int) { e.Marcar(d, h, m, poner) })
}

// porBloques aplica un cambio de párrafo a cada bloque tocado por la selección.
func (f *Folio) porBloques(a, b punto, hacer func(bl rico.Bloque, e *rico.Edicion) bool) {
	f.antesDeCambiar(false)
	lugares := f.lugaresSeleccion(a, b)
	cambio := false
	for _, i := range f.indices(lugares) {
		if hacer(f.bloques[i], f.bloques[i].Editar()) {
			cambio = true
		}
	}
	if !cambio {
		f.hist.atras = f.hist.atras[:len(f.hist.atras)-1]
		return
	}
	f.volverA(lugares, a, b)
}

// lugaresSeleccion son los lugares del árbol de los bloques de texto seleccionados: sobreviven
// a las ediciones que cambian la lista de bloques.
func (f *Folio) lugaresSeleccion(a, b punto) []rico.Lugar {
	var out []rico.Lugar
	for i := a.b; i <= b.b; i++ {
		if f.deTexto(i) {
			out = append(out, f.bloques[i].Lugar)
		}
	}
	return out
}

func (f *Folio) indices(ls []rico.Lugar) []int {
	var out []int
	for _, l := range ls {
		if i := rico.Indice(f.bloques, l); i >= 0 {
			out = append(out, i)
		}
	}
	return out
}

// volverA deja la selección sobre los mismos bloques tras una edición de párrafo.
func (f *Folio) volverA(lugares []rico.Lugar, a, b punto) {
	f.bloques = f.doc.Bloques()
	if len(lugares) > 0 {
		if i := rico.Indice(f.bloques, lugares[0]); i >= 0 {
			a.b = i
		}
		if i := rico.Indice(f.bloques, lugares[len(lugares)-1]); i >= 0 {
			b.b = i
		}
	}
	if f.cur.antes(f.anc) {
		f.cur, f.anc = a, b
	} else {
		f.cur, f.anc = b, a
	}
	f.reestructurado()
}

// listas mete los bloques en una lista, o los saca. Cada edición deja un lugar nuevo
// para su bloque, que es por donde se encuentra después.
func (f *Folio) listas(a, b punto, ordenada bool) {
	f.antesDeCambiar(false)
	lugares := f.lugaresSeleccion(a, b)
	var nuevos []rico.Lugar
	for _, l := range lugares {
		f.bloques = f.doc.Bloques()
		i := rico.Indice(f.bloques, l)
		if i < 0 {
			continue
		}
		if n, ok := f.bloques[i].Editar().AlternarLista(ordenada); ok {
			nuevos = append(nuevos, n)
		} else {
			nuevos = append(nuevos, l)
		}
	}
	f.volverA(nuevos, a, b)
}

// sangrar es «Aumentar/Disminuir sangría»: 40 px de `margin-left` por paso, el que usa
// el navegador.
//
// ponytail: dentro de una lista la web ANIDA el elemento en otra lista; aquí cambia su
// margen. Se ve igual de sangrado, pero la numeración no reinicia. Relevar anidando.
func (f *Folio) sangrar(a, b punto, mas bool) {
	f.porBloques(a, b, func(bl rico.Bloque, e *rico.Edicion) bool {
		actual := bl.Base.MargenIzq
		nuevo := actual - PasoSangria
		if mas {
			nuevo = actual + PasoSangria
		}
		if nuevo <= 0 {
			return actual > 0 && e.PonerBloque("margin-left", "")
		}
		return e.PonerBloque("margin-left", pxTexto(nuevo))
	})
}

// PasoSangria es lo que mueve cada pulsación de sangría.
const PasoSangria float32 = 40

// ---- el formato bajo el cursor ----

// Formato es el `FormatState` de la web: lo que la cinta pinta pulsado. Sin cursor en el
// folio es el vacío, como `getFormatState` cuando la selección no está en el lienzo.
func (f *Folio) Formato() editor.Formato {
	if !f.conCursor || !f.deTexto(f.cur.b) {
		return editor.Formato{PuedeDeshacer: len(f.hist.atras) > 0, PuedeRehacer: len(f.hist.adelante) > 0}
	}
	a, b := f.orden()
	out := editor.Formato{PuedeDeshacer: len(f.hist.atras) > 0, PuedeRehacer: len(f.hist.adelante) > 0}
	tiene := func(m rico.Marca) bool {
		if !f.haySeleccion() {
			return f.edicion(f.cur.b).Tiene(f.cur.p, f.cur.p, m)
		}
		todo := true
		f.tramos(func(i, d, h int) {
			if !f.edicion(i).Tiene(d, h, m) {
				todo = false
			}
		})
		return todo
	}
	out.Negrita, out.Cursiva = tiene(rico.Negrita), tiene(rico.Cursiva)
	out.Subrayado, out.Tachado = tiene(rico.Subrayado), tiene(rico.Tachado)
	e := f.edicion(a.b)
	est := e.Formato(a.p, max(b.p, a.p))
	if a.b != b.b {
		est = e.Formato(a.p, f.largo(a.b))
	}
	out.Fuente, out.Tamano = est.Familia, pasoDeTamano(est.Tamano)
	switch f.bloques[f.cur.b].Base.Alineacion {
	case rico.Centro:
		out.Alineacion = editor.Centro
	case rico.Derecha:
		out.Alineacion = editor.Derecha
	case rico.Justificada:
		out.Alineacion = editor.Justificado
	case rico.Izquierda:
	}
	cur := f.edicion(f.cur.b)
	out.Puntos, out.Numeros = cur.Lista() == "ul", cur.Lista() == "ol"
	out.EnTabla = f.bloques[f.cur.b].Celda != nil
	out.Imagen = f.ImagenSeleccionada()
	switch t := cur.Bloque(); t {
	case "h1", "h2", "h3", "blockquote":
		out.Titulo = t
	default:
		out.Titulo = "p"
	}
	return out
}

// programarFormato repinta la cinta si el formato bajo el cursor ha cambiado, como mucho
// cada [PausaFormato]; la barra de estado se repinta siempre (las palabras).
func (f *Folio) programarFormato() {
	if f.formatoPendiente || f.ctx == nil {
		return
	}
	f.formatoPendiente = true
	f.Programar(PausaFormato, func() {
		f.formatoPendiente = false
		if nuevo := f.Formato(); nuevo != f.ultimoFormato {
			f.ultimoFormato = nuevo
			if f.ctx.Refrescar != nil {
				f.ctx.Refrescar()
				return
			}
		}
		if f.ctx.RefrescarEstado != nil {
			f.ctx.RefrescarEstado()
		}
	})
}

// ---- dónde está cada cosa ----

// caretEnMaqueta es la raya del cursor en px de maqueta, desde la caja de contenido.
func (f *Folio) caretEnMaqueta(q punto) (x, y, alto float32, ok bool) {
	if q.b < 0 || q.b >= len(f.cajas) || len(f.cajas[q.b].lineas) == 0 {
		return 0, 0, 0, false
	}
	c := f.cajas[q.b]
	l := lineaDe(c, q.p)
	est := c.b.Base
	for _, p := range l.piezas {
		if p.desde <= q.p && q.p <= p.hasta {
			est = p.est
			if q.p < p.hasta {
				break
			}
		}
	}
	top := c.y + c.arribaTexto + l.y
	base := top + l.base + desplazamientoVertical(est)
	return c.izq + l.x0 + xEnLinea(l, q.p), base - ascFuente*est.Tamano, cajaFuente * est.Tamano, true
}

// caretEnPagina es la raya del cursor en px de la página (con zoom y márgenes).
func (f *Folio) caretEnPagina() (x, y, alto float32, ok bool) {
	mx, my, ma, ok := f.caretEnMaqueta(f.cur)
	if !ok {
		return 0, 0, 0, false
	}
	z := f.escala()
	ox, oy := f.pag.origen()
	return ox + mx*z, oy + my*z, ma * z, true
}

// lineaDe es la línea donde cae la runa p. En un corte de línea sin salto, la posición
// del corte es el principio de la línea de abajo, como en el navegador.
func lineaDe(c caja, p int) linea {
	for i, l := range c.lineas {
		if p < l.hasta || (p == l.hasta && (i == len(c.lineas)-1 || c.lineas[i+1].desde != p)) {
			if p >= l.desde {
				return l
			}
		}
	}
	return c.lineas[len(c.lineas)-1]
}

// xEnLinea es la x de la runa p dentro de la línea, sin la alineación. Una posición
// entre dos piezas es el principio de la de la derecha: en una línea justificada, el
// cursor que sigue a un espacio va pegado a la palabra, no al final del espacio sin
// estirar.
func xEnLinea(l linea, p int) float32 {
	if len(l.piezas) == 0 {
		return 0
	}
	if p <= l.piezas[0].desde {
		return l.piezas[0].x
	}
	for _, pz := range l.piezas {
		if p >= pz.desde && p < pz.hasta {
			runas := []rune(pz.texto)
			return pz.x + medidor.ancho(string(runas[:p-pz.desde]), pz.est)
		}
	}
	ult := l.piezas[len(l.piezas)-1]
	return ult.x + ult.ancho
}

// enPunto convierte un punto de la página (px de pantalla) en una posición del cursor.
func (f *Folio) enPunto(pos fyne.Position) punto {
	z := f.escala()
	ox, oy := f.pag.origen()
	mx, my := (pos.X-ox)/z, (pos.Y-oy)/z
	return f.enMaqueta(mx, my)
}

// enMaqueta es enPunto en px de maqueta: el bloque de la altura, la línea de la altura y
// la runa más cercana en esa línea.
func (f *Folio) enMaqueta(mx, my float32) punto {
	if len(f.cajas) == 0 {
		return punto{}
	}
	if r := f.regionEn(my); r >= 0 {
		return f.enLineas(r, mx, my)
	}
	b := len(f.cajas) - 1
	for b > 0 && f.cajas[b].b.Region != "" {
		b--
	}
	for i, c := range f.cajas {
		if c.b.Region != "" || c.colDer != 0 && (mx < c.colIzq || mx >= c.colDer) {
			continue // fuera de la corriente, o en otra columna
		}
		bajo := c.y + c.alto
		if c.tabla != nil {
			bajo = c.tabla.y + c.tabla.alto // una tabla se toma entera, y dentro, la celda
		}
		if my < bajo {
			b = i
			break
		}
	}
	if t := f.cajas[b].tabla; t != nil {
		b = f.enCelda(t.celdaEn(mx, my), my, b)
	}
	for (!f.deTexto(b) || f.cajas[b].b.Region != "") && b < len(f.cajas)-1 {
		b++
	}
	for (!f.deTexto(b) || f.cajas[b].b.Region != "") && b > 0 {
		b--
	}
	return f.enLineas(b, mx, my)
}

// enLineas es el punto del bloque b más cercano a (mx, my): la línea de la altura y la
// runa más cercana en ella.
func (f *Folio) enLineas(b int, mx, my float32) punto {
	c := f.cajas[b]
	if len(c.lineas) == 0 {
		return punto{b, 0}
	}
	ly := my - c.y - c.arribaTexto
	l := c.lineas[len(c.lineas)-1]
	for _, cand := range c.lineas {
		if ly < cand.y+cand.alto {
			l = cand
			break
		}
	}
	return punto{b, runaEnX(l, mx-c.izq-l.x0)}
}

// enCelda es el bloque de la celda a la altura y: el primero que acaba por debajo, o el
// último de la celda. `si` es el que se devuelve si la celda no tiene ninguno.
func (f *Folio) enCelda(cel *rico.Celda, y float32, si int) int {
	ultimo := -1
	for i, c := range f.cajas {
		if c.b.Celda != cel {
			continue
		}
		if y < c.y+c.alto {
			return i
		}
		ultimo = i
	}
	if ultimo < 0 {
		return si
	}
	return ultimo
}

// runaEnX es la frontera entre runas más cercana a x dentro de la línea.
func runaEnX(l linea, x float32) int {
	if len(l.piezas) == 0 || x <= l.piezas[0].x {
		return l.desde
	}
	for _, pz := range l.piezas {
		if x > pz.x+pz.ancho {
			continue
		}
		if pz.img != nil || pz.atomo { // una imagen es una runa: antes o después, lo más cerca
			if x > pz.x+pz.ancho/2 {
				return pz.hasta
			}
			return pz.desde
		}
		runas := []rune(pz.texto)
		mejor, dist := pz.desde, abs(x-pz.x)
		for k := 1; k <= len(runas); k++ {
			xk := pz.x + medidor.ancho(string(runas[:k]), pz.est)
			if d := abs(x - xk); d < dist {
				mejor, dist = pz.desde+k, d
			}
		}
		return mejor
	}
	// Más allá del final: el final de la línea, sin el espacio que cuelga de un corte.
	fin := l.hasta
	if n := len(l.piezas); n > 0 && strings.HasSuffix(l.piezas[n-1].texto, " ") && fin > l.desde {
		fin--
	}
	return fin
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// rectangulosSeleccion son las cajas de la selección, línea a línea, en px de maqueta.
func (f *Folio) rectangulosSeleccion() []rect {
	a, b := f.orden()
	return f.rectangulosEntre(a, b)
}

// rectangulosEntre son las cajas del tramo que empieza en `a` y acaba en `b`, línea a línea.
func (f *Folio) rectangulosEntre(a, b punto) []rect {
	var out []rect
	for i := a.b; i <= b.b && i < len(f.cajas); i++ {
		c := f.cajas[i]
		for _, l := range c.lineas {
			desde, hasta := max(l.desde, 0), l.hasta
			if i == a.b {
				desde = max(desde, a.p)
			}
			if i == b.b {
				hasta = min(hasta, b.p)
			}
			if desde > hasta || (desde == hasta && (i == b.b && l.hasta >= b.p)) {
				continue
			}
			x0, x1 := xEnLinea(l, desde), xEnLinea(l, hasta)
			if i != b.b || hasta < b.p || l.hasta < b.p {
				x1 += medidor.ancho(" ", c.b.Base) // el salto seleccionado, como el navegador
			}
			out = append(out, rect{c.izq + l.x0 + x0, c.y + c.arribaTexto + l.y, x1 - x0, l.alto})
		}
	}
	return out
}

// rect es una caja en px de maqueta.
type rect struct{ x, y, w, h float32 }

// textoDocumento es el documento como texto plano, el `innerText` del folio: los bloques
// de texto separados por saltos, sin las regiones ni lo que no es texto.
func (f *Folio) textoDocumento() string {
	var s strings.Builder
	for i, b := range f.bloques {
		if !f.deTexto(i) || b.Region != "" {
			continue
		}
		if s.Len() > 0 {
			s.WriteByte('\n')
		}
		s.WriteString(strings.NewReplacer("\u00a0", " ", string(rico.ObjetoRuna), "").Replace(b.Texto()))
	}
	return s.String()
}

// textoSeleccionado es la selección como texto plano: los bloques separados por saltos.
func (f *Folio) textoSeleccionado() string {
	var s strings.Builder
	a, b := f.orden()
	for i := a.b; i <= b.b; i++ {
		if i > a.b {
			s.WriteByte('\n')
		}
		if !f.deTexto(i) {
			continue
		}
		runas := []rune(f.bloques[i].Texto())
		desde, hasta := 0, len(runas)
		if i == a.b {
			desde = min(a.p, len(runas))
		}
		if i == b.b {
			hasta = min(b.p, len(runas))
		}
		// Como texto, una imagen no se lleva: se queda fuera, igual que en la web.
		s.WriteString(strings.NewReplacer(" ", " ", string(rico.ObjetoRuna), "").Replace(string(runas[desde:max(desde, hasta)])))
	}
	return s.String()
}

// asegurarVisible desplaza lo justo para que el cursor quede a la vista.
func (f *Folio) asegurarVisible() {
	if f.caja != nil {
		return // una caja no se desplaza: lo que no cabe se recorta, como con `overflow: hidden`
	}
	x, y, alto, ok := f.caretEnPagina()
	vista := f.scroll.Size()
	if !ok || vista.Height <= 0 {
		return
	}
	off := f.scroll.Offset
	switch {
	case y < off.Y:
		off.Y = y - alto
	case y+alto > off.Y+vista.Height:
		off.Y = y + 2*alto - vista.Height
	}
	switch {
	case x < off.X:
		off.X = x - 20
	case x > off.X+vista.Width:
		off.X = x - vista.Width + 20
	}
	// Sin pasarse del final: la página no se desplaza más allá de lo que mide.
	tope := f.pag.MinSize()
	off.X = min(off.X, max(tope.Width-vista.Width, 0))
	off.Y = min(off.Y, max(tope.Height-vista.Height, 0))
	f.scroll.ScrollToOffset(fyne.NewPos(max(off.X, 0), max(off.Y, 0)))
}

// ---- teclado ----

var (
	_ fyne.Focusable      = (*pagina)(nil)
	_ fyne.Shortcutable   = (*pagina)(nil)
	_ fyne.Tappable       = (*pagina)(nil)
	_ fyne.DoubleTappable = (*pagina)(nil)
	_ fyne.Draggable      = (*pagina)(nil)
	_ desktop.Keyable     = (*pagina)(nil)
	_ desktop.Mouseable   = (*pagina)(nil)
	_ desktop.Cursorable  = (*pagina)(nil)
	_ mobile.Keyboardable = (*pagina)(nil)
	_ fyne.Accessible     = (*pagina)(nil)
)

// AccessibilityRole es el `role="textbox"` del folio: Fyne 2.8 no tiene otro que
// anunciar un área de texto, y «texto» es el más cercano.
func (p *pagina) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// AccessibilityLabel es el `aria-label` de la web: el título del documento.
func (p *pagina) AccessibilityLabel() string {
	if p.f.caja != nil {
		return p.f.caja.Rotulo
	}
	if c := p.f.ctx; c != nil {
		if c.Titulo != "" {
			return c.Titulo
		}
		return c.Traducir("kind.document")
	}
	return ""
}

// Keyboard pide el teclado de texto en el móvil.
func (p *pagina) Keyboard() mobile.KeyboardType { return mobile.DefaultKeyboard }

// Cursor es la «I» del texto.
func (p *pagina) Cursor() desktop.Cursor { return desktop.TextCursor }

func (f *Folio) enfocar() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(f.pag); c != nil && c.Focused() != f.pag {
		c.Focus(f.pag)
	}
}

// FocusGained enciende el cursor y su parpadeo.
//
// El parpadeo es una animación de Fyne y no un temporizador: corre en el hilo de la
// interfaz, y en las pruebas —donde `fyne.Do` no cambia de hilo— no deja una rutina que
// repinte a la vez que la prueba (la carrera que el detector cazaba en quien aloja el
// folio). Medio segundo encendido y medio apagado, como el de Chrome; el driver de pruebas
// da un solo paso al final, y ahí el cursor se ve.
func (p *pagina) FocusGained() {
	f := p.f
	f.enfocado, f.caretVisible, f.conCursor = true, true, true
	if f.parpadeo == nil {
		f.parpadeo = fyne.NewAnimation(2*PausaParpadeo, func(v float32) {
			if ver := v < 0.5 || v == 1; ver != f.caretVisible && f.enfocado {
				f.caretVisible = ver
				p.Refresh()
			}
		})
		f.parpadeo.Curve = fyne.AnimationLinear
		f.parpadeo.RepeatCount = fyne.AnimationRepeatForever
		f.parpadeo.Start()
	}
	p.Refresh()
}

// FocusLost apaga el cursor; la selección se queda, como en el navegador.
func (p *pagina) FocusLost() {
	f := p.f
	f.enfocado, f.mayus = false, false
	if f.parpadeo != nil {
		f.parpadeo.Stop()
		f.parpadeo = nil
	}
	p.Refresh()
	if f.caja != nil && f.caja.AlPerderFoco != nil {
		f.caja.AlPerderFoco()
	}
}

// TypedRune escribe.
func (p *pagina) TypedRune(r rune) { p.f.escribir(string(r)) }

// KeyDown y KeyUp siguen la Mayús, que Fyne no manda con cada tecla.
func (p *pagina) KeyDown(e *fyne.KeyEvent) {
	if e.Name == desktop.KeyShiftLeft || e.Name == desktop.KeyShiftRight {
		p.f.mayus = true
	}
}

func (p *pagina) KeyUp(e *fyne.KeyEvent) {
	if e.Name == desktop.KeyShiftLeft || e.Name == desktop.KeyShiftRight {
		p.f.mayus = false
	}
}

// TypedKey es la edición y el movimiento.
//
//nolint:gocyclo // una rama por tecla: la ramificación ES la tabla
func (p *pagina) TypedKey(e *fyne.KeyEvent) {
	f := p.f
	ext := f.mayus
	switch e.Name {
	case fyne.KeyEscape:
		switch {
		case f.caja != nil && f.caja.AlSalir != nil:
			f.caja.AlSalir()
		case f.sel != nil: // Escape suelta la imagen, como en la web
			f.sel = nil
			f.pag.Refresh()
			f.programarFormato()
		}
		return
	case fyne.KeyBackspace:
		f.retroceso()
	case fyne.KeyDelete:
		f.suprimir()
	case fyne.KeyReturn, fyne.KeyEnter:
		f.intro()
	case fyne.KeyLeft:
		if f.haySeleccion() && !ext {
			a, _ := f.orden()
			f.mover(a, false)
			return
		}
		f.mover(f.paso(f.cur, -1), ext)
	case fyne.KeyRight:
		if f.haySeleccion() && !ext {
			_, b := f.orden()
			f.mover(b, false)
			return
		}
		f.mover(f.paso(f.cur, 1), ext)
	case fyne.KeyUp, fyne.KeyDown:
		f.vertical(e.Name == fyne.KeyDown, ext, 0)
		return // conserva la columna
	case fyne.KeyPageUp, fyne.KeyPageDown:
		f.vertical(e.Name == fyne.KeyPageDown, ext, f.scroll.Size().Height/f.escala())
		return
	case fyne.KeyHome:
		c := f.cajas[f.cur.b]
		f.mover(punto{f.cur.b, lineaDe(c, f.cur.p).desde}, ext)
	case fyne.KeyEnd:
		c := f.cajas[f.cur.b]
		l := lineaDe(c, f.cur.p)
		fin := l.hasta
		if fin > l.desde && fin < c.largo && fin == siguienteDesde(c, l) {
			fin-- // el espacio del corte cuelga: el final es antes de él
		}
		f.mover(punto{f.cur.b, fin}, ext)
	default:
		return
	}
	f.hayObjetivo = false
}

func siguienteDesde(c caja, l linea) int {
	for i, x := range c.lineas {
		if x.desde == l.desde && i+1 < len(c.lineas) {
			return c.lineas[i+1].desde
		}
	}
	return -1
}

// paso mueve una runa, saltando de bloque en los extremos.
func (f *Folio) paso(q punto, d int) punto {
	q.p += d
	switch {
	case q.p < 0 && q.b > 0:
		q.b--
		for q.b > 0 && !f.deTexto(q.b) {
			q.b--
		}
		q.p = f.largo(q.b)
	case q.p > f.largo(q.b) && q.b < len(f.bloques)-1:
		q.b++
		for q.b < len(f.bloques)-1 && !f.deTexto(q.b) {
			q.b++
		}
		q.p = 0
	}
	// En los extremos del documento no hay bloque al que saltar: se queda en el borde,
	// no una runa fuera.
	q.p = min(max(q.p, 0), f.largo(q.b))
	return q
}

// palabra mueve hasta el principio de la palabra anterior o el final de la siguiente.
func (f *Folio) palabra(q punto, d int) punto {
	runas := []rune(f.bloques[q.b].Texto())
	// El doble clic pregunta desde `q.p+1`, que al final de un bloque cae FUERA: tumbaba
	// Sytue Docus al hacer doble clic detrás de la última letra de una línea.
	q.p = min(max(q.p, 0), len(runas))
	if (d < 0 && q.p == 0) || (d > 0 && q.p >= len(runas)) {
		return f.paso(q, d)
	}
	i := q.p
	if d < 0 {
		for i > 0 && esEspacio(runas[i-1]) {
			i--
		}
		for i > 0 && !esEspacio(runas[i-1]) {
			i--
		}
	} else {
		for i < len(runas) && esEspacio(runas[i]) {
			i++
		}
		for i < len(runas) && !esEspacio(runas[i]) {
			i++
		}
	}
	return punto{q.b, i}
}

// vertical sube o baja una línea —o `salto` px, con Re Pág y Av Pág— conservando la
// columna.
func (f *Folio) vertical(abajo, ext bool, salto float32) {
	x, y, alto, ok := f.caretEnMaqueta(f.cur)
	if !ok {
		return
	}
	if !f.hayObjetivo {
		f.xObjetivo, f.hayObjetivo = x, true
	}
	// Una línea: medio renglón por fuera de la del cursor cae en la de al lado.
	dest := y - 4 - salto
	if abajo {
		dest = y + alto + 4 + salto
	}
	q := f.enMaqueta(f.xObjetivo, dest)
	// En la última línea de una celda, la de al lado está pasado su relleno y su borde: se
	// busca allí, fuera de la celda.
	if c := f.cajas[f.cur.b]; c.tabla != nil && q.b == f.cur.b && lineaDe(c, q.p).desde == lineaDe(c, f.cur.p).desde {
		for _, cm := range c.tabla.celdas {
			if cm.c == c.b.Celda {
				borde := cm.y - 1
				if abajo {
					borde = cm.y + cm.h + 1
				}
				q = f.enMaqueta(f.xObjetivo, borde)
			}
		}
	}
	switch {
	case q == f.cur && !abajo:
		q = punto{f.cur.b, 0} // ya en la primera línea: al principio
	case q == f.cur:
		q = punto{f.cur.b, f.largo(f.cur.b)} // ya en la última: al final
	}
	objetivo := f.xObjetivo
	f.mover(q, ext)
	f.xObjetivo, f.hayObjetivo = objetivo, true
}

// TypedShortcut son el portapapeles, el historial y los atajos de la web (§6.8).
func (p *pagina) TypedShortcut(s fyne.Shortcut) {
	f := p.f
	switch s := s.(type) {
	case *fyne.ShortcutCopy:
		if f.haySeleccion() {
			fyne.CurrentApp().Clipboard().SetContent(f.textoSeleccionado())
		}
	case *fyne.ShortcutCut:
		if f.haySeleccion() && f.editable() {
			fyne.CurrentApp().Clipboard().SetContent(f.textoSeleccionado())
			f.retroceso()
		}
	case *fyne.ShortcutPaste:
		f.escribir(fyne.CurrentApp().Clipboard().Content())
	case *fyne.ShortcutSelectAll:
		n := len(f.bloques) - 1
		f.anc = f.acotar(punto{0, 0})
		f.cur = f.acotar(punto{n, f.largo(n)})
		f.pag.Refresh()
		f.programarFormato()
	case *fyne.ShortcutUndo:
		f.deshacer()
	case *fyne.ShortcutRedo:
		f.rehacer()
	case *desktop.CustomShortcut:
		f.atajo(s)
	}
}

// atajo son los atajos con modificador que no son del sistema.
//
//nolint:gocyclo // una rama por atajo
func (f *Folio) atajo(s *desktop.CustomShortcut) {
	if f.ctx.AtajoImprimir(s) {
		return
	}
	mod := s.Modifier &^ fyne.KeyModifierShift
	ext := s.Modifier&fyne.KeyModifierShift != 0
	principal := mod == fyne.KeyModifierShortcutDefault
	switch {
	case principal && s.KeyName == fyne.KeyB:
		f.Mandar(editor.Negrita, "")
	case principal && s.KeyName == fyne.KeyI:
		f.Mandar(editor.Cursiva, "")
	case principal && s.KeyName == fyne.KeyU:
		f.Mandar(editor.Subrayado, "")
	case principal && ext && s.KeyName == fyne.KeyX:
		f.Mandar(editor.Tachado, "")
	case principal && ext && s.KeyName == fyne.KeyZ:
		f.rehacer()
	case principal && s.KeyName == fyne.KeyS:
		if f.AlGuardarYa != nil {
			f.AlGuardarYa()
		}
	case principal && s.KeyName == fyne.KeyF && !ext:
		if f.ctx != nil {
			f.ctx.Panel = editor.PanelBuscar
			if f.ctx.Refrescar != nil {
				f.ctx.Refrescar()
			}
		}
	case principal && s.KeyName == fyne.KeyHome:
		f.mover(f.acotar(punto{0, 0}), ext)
	case principal && s.KeyName == fyne.KeyEnd:
		n := len(f.bloques) - 1
		f.mover(f.acotar(punto{n, f.largo(n)}), ext)
	case esSaltoPalabra(mod) && s.KeyName == fyne.KeyLeft:
		f.mover(f.palabra(f.cur, -1), ext)
	case esSaltoPalabra(mod) && s.KeyName == fyne.KeyRight:
		f.mover(f.palabra(f.cur, 1), ext)
	case mod == fyne.KeyModifierSuper && s.KeyName == fyne.KeyLeft:
		c := f.cajas[f.cur.b]
		f.mover(punto{f.cur.b, lineaDe(c, f.cur.p).desde}, ext) // Cmd+← en macOS
	case mod == fyne.KeyModifierSuper && s.KeyName == fyne.KeyRight:
		c := f.cajas[f.cur.b]
		f.mover(punto{f.cur.b, lineaDe(c, f.cur.p).hasta}, ext)
	}
}

// esSaltoPalabra: Alt en macOS y Ctrl en los demás, como en cualquier campo del sistema.
func esSaltoPalabra(mod fyne.KeyModifier) bool {
	if fyne.KeyModifierShortcutDefault == fyne.KeyModifierSuper {
		return mod == fyne.KeyModifierAlt
	}
	return mod == fyne.KeyModifierControl
}

// ---- ratón ----

// MouseDown coloca el cursor al pulsar, no al soltar, que es cuando lo hace el navegador;
// con Mayús extiende la selección.
func (p *pagina) MouseDown(e *desktop.MouseEvent) {
	if e.Button != desktop.MouseButtonPrimary {
		return
	}
	f := p.f
	f.enfocar()
	f.porRaton = true
	if f.pulsarImagen(e.Position) {
		return
	}
	f.mover(f.enPunto(e.Position), e.Modifier&fyne.KeyModifierShift != 0)
	f.hayObjetivo = false
}

func (p *pagina) MouseUp(*desktop.MouseEvent) {}

// Tapped es el clic en el móvil, donde no llega MouseDown.
func (p *pagina) Tapped(e *fyne.PointEvent) {
	f := p.f
	if f.porRaton {
		f.porRaton = false
		return
	}
	f.enfocar()
	if f.pulsarImagen(e.Position) {
		return
	}
	f.mover(f.enPunto(e.Position), false)
	f.hayObjetivo = false
}

// DoubleTapped selecciona la palabra.
func (p *pagina) DoubleTapped(e *fyne.PointEvent) {
	f := p.f
	q := f.enPunto(e.Position)
	a := f.palabra(punto{q.b, q.p + 1}, -1)
	b := f.palabra(punto{q.b, max(q.p-1, 0)}, 1)
	if a.b != q.b {
		a = punto{q.b, 0}
	}
	if b.b != q.b {
		b = punto{q.b, f.largo(q.b)}
	}
	f.anc, f.cur = a, b
	f.pag.Refresh()
	f.programarFormato()
}

// Dragged extiende la selección en el escritorio; en el móvil mueve un tirador de la
// selección o desplaza la página, que es lo que espera un dedo.
func (p *pagina) Dragged(e *fyne.DragEvent) {
	f := p.f
	if f.arrastre != nil {
		f.arrastrar(e.Position)
		return
	}
	if esMovil() {
		f.arrastrarTactil(e) // un tirador de la selección, o desplazar (`tactil.go`)
		return
	}
	f.mover(f.enPunto(e.Position), true)
	f.hayObjetivo = false
}

// DragEnd termina el arrastre: el de un tirador o el de mover una imagen se guarda.
func (p *pagina) DragEnd() {
	p.f.tiradorTexto = sinDecidir
	p.f.soltarArrastre()
}

// ---- valores de la cinta ----

// familiaCSS escribe la familia como la escribe el navegador: entre comillas si lleva
// espacios.
func familiaCSS(v string) string {
	if strings.ContainsAny(v, " ") {
		return `"` + v + `"`
	}
	return v
}

// tamanosCSS son los siete peldaños de `execCommand('fontSize')` como los escribe Chrome
// con `styleWithCSS`.
var tamanosCSS = []string{"x-small", "small", "medium", "large", "x-large", "xx-large", "xxx-large"}

// pxTamanos son esos mismos peldaños en px, para reconocerlos al leer.
var pxTamanos = []float32{10, 13, 16, 18, 24, 32, 48}

func tamanoCSS(paso string) string {
	if len(paso) == 1 && paso[0] >= '1' && paso[0] <= '7' {
		return tamanosCSS[paso[0]-'1']
	}
	return ""
}

// pasoDeTamano es lo contrario: el peldaño de un tamaño en px, o "" si no es ninguno.
func pasoDeTamano(px float32) string {
	for i, t := range pxTamanos {
		if abs(px-t) < 0.01 {
			return string(rune('1' + i))
		}
	}
	return ""
}

// colorCSS escribe un `#rrggbb` de la paleta como lo escribe Chrome: `rgb(r, g, b)`.
// «transparent», vacío o algo que no es un color quitan el color.
func colorCSS(hex string) string {
	h, ok := strings.CutPrefix(hex, "#")
	n, err := strconv.ParseUint(h, 16, 32)
	if !ok || len(h) != 6 || err != nil {
		return ""
	}
	return "rgb(" + strconv.Itoa(int(n>>16)) + ", " + strconv.Itoa(int(n>>8&0xff)) + ", " + strconv.Itoa(int(n&0xff)) + ")"
}

// pxTexto escribe unos px sin decimales de sobra: «40px».
func pxTexto(n float32) string { return strconv.FormatFloat(float64(n), 'f', -1, 32) + "px" }
