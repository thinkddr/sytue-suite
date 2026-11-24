// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"slices"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Buscar y reemplazar en la nota» (Ctrl/⌘+F, o +H con el reemplazo abierto), el de
// RichText.tsx: una barra bajo la de formato con la caja, la cuenta «2 de 5», «Aa»
// (distinguir mayúsculas), anterior, siguiente, abrir el reemplazo y cerrar; y, debajo,
// la caja de reemplazo con «Reemplazar» y «Reemplazar todo».
//
// Intro va a la siguiente, Mayús+Intro a la anterior y Escape cierra dejando
// seleccionada la coincidencia en la que estaba. En la caja de reemplazo, Intro
// reemplaza la actual y salta a la siguiente. Sin «Aa», con la regla de búsqueda de la
// suite: sin acentos ni mayúsculas; con «Aa», la letra exacta.
//
// Todas las coincidencias se pintan con un velo detrás del texto, como los `::highlight`
// de la web (styles/app.css): el amarillo del `<mark>` para todas y uno más subido para
// la actual. La actual es además la SELECCIÓN del cuerpo (lo que queda al cerrar), pero
// mientras la barra está abierta no se tiñe encima: el velo ya la destaca, como en la web.
//
// Reemplazar es un paso de deshacer; «Reemplazar todo», uno solo para todas. Lo que se
// pone conserva el formato de la primera letra de lo que sustituye.

type buscadorNota struct {
	campo     *ui.Entrada
	reemplazo *ui.Entrada
	cuenta    *canvas.Text
	fondo     *canvas.Rectangle
	filete    *canvas.Rectangle
	barra     *fyne.Container
	cuerpo    *fyne.Container
	fila      fyne.CanvasObject
	filaRepl  fyne.CanvasObject
	// mayus es «Aa»: distinguir mayúsculas (y acentos: la letra exacta).
	mayus   bool
	btMayus *botonBarra
	btRepl  *botonBarra
	conRepl bool
	// uno y todos son «Reemplazar» y «Reemplazar todo».
	uno, todos *ui.Boton
	hallados   [][2]Pos
	i          int
}

// El tono de la coincidencia actual, que la hoja escribe a mano como el del `<mark>`
// (prosa.go): `::highlight(sytue-buscar-actual)` en los dos temas.
var (
	buscarActualClaro  = color.NRGBA{R: 0xf5, G: 0xc4, B: 0x6b, A: 0xff}
	buscarActualOscuro = color.NRGBA{R: 0xa3, G: 0x6a, B: 0x00, A: 0xff}
)

// coloresBuscar son el velo de todas las coincidencias y el de la actual.
func coloresBuscar(v tema.Variante) (todas, actual color.NRGBA) {
	if v == tema.Oscuro {
		return resaltadoOscuro, buscarActualOscuro
	}
	return resaltadoClaro, buscarActualClaro
}

// rangosExactos son los de q en las runas, letra a letra y sin solaparse.
func rangosExactos(pajar []rune, q string) [][2]int {
	aguja := []rune(q)
	if len(aguja) == 0 {
		return nil
	}
	var out [][2]int
	for j := 0; j+len(aguja) <= len(pajar); {
		if slices.Equal(pajar[j:j+len(aguja)], aguja) {
			out = append(out, [2]int{j, j + len(aguja)})
			j += len(aguja)
			continue
		}
		j++
	}
	return out
}

// coincidenciasDoc son las de q en el documento, en orden, bloque a bloque. Con `mayus`,
// la letra exacta; sin él, la regla de la suite.
func coincidenciasDoc(d *Documento, q string, mayus bool) [][2]Pos {
	var out [][2]Pos
	for bi, b := range d.Bloques {
		if !editable(b) {
			continue
		}
		texto := []rune(b.Texto())
		rangos := buscar.Rangos(texto, q)
		if mayus {
			rangos = rangosExactos(texto, q)
		}
		for _, r := range rangos {
			if r[0] < inicioEditable(b) {
				continue // el hueco de la casilla de una tarea no es texto
			}
			out = append(out, [2]Pos{{bi, r[0]}, {bi, r[1]}})
		}
	}
	return out
}

func (e *EditorNota) abrirBuscador() {
	if e.buscador == nil {
		e.buscador = e.armarBuscador()
	}
	if e.haySeleccion() && !e.actualSeleccionada() {
		e.buscador.campo.PonerTexto(e.Seleccionado())
	}
	e.buscarEnNota(e.buscador.campo.Texto(), 0)
	e.Refresh()
	llamar0(e.AlCrecer)
	if c := fyne.CurrentApp().Driver().CanvasForObject(e.zona); c != nil {
		c.Focus(e.buscador.campo)
	}
}

// abrirReemplazar es Ctrl/⌘+H: la barra con el reemplazo abierto y el foco en lo que
// falte por escribir (lo que se busca, si está vacío; si no, el reemplazo).
func (e *EditorNota) abrirReemplazar() {
	e.abrirBuscador()
	b := e.buscador
	if !b.conRepl {
		e.alternarReemplazo()
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(e.zona); c != nil && b.campo.Texto() != "" {
		c.Focus(b.reemplazo)
	}
}

//nolint:funlen // La barra declara sus dos filas en un único lugar.
func (e *EditorNota) armarBuscador() *buscadorNota {
	t := e.tr.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := &buscadorNota{
		campo:     ui.NuevaEntrada(t("find.placeholder", nil)).Con(ui.CampoSM),
		reemplazo: ui.NuevaEntrada(t("editor.replace", nil)).Con(ui.CampoSM),
		cuenta:    ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		fondo:     canvas.NewRectangle(tema.Color("surface", v)),
		filete:    canvas.NewRectangle(tema.Color("border", v)),
	}
	b.campo.Rotulo = t("editor.find", nil)
	b.reemplazo.Rotulo = t("editor.replace", nil)
	b.campo.AlCambiar(func(s string) { e.buscarEnNota(s, 0) })
	b.campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			paso := 1
			if e.shift {
				paso = -1
			}
			e.buscarEnNota(b.campo.Texto(), b.i+paso)
			return true
		case fyne.KeyEscape:
			e.cerrarBuscador()
			return true
		default:
		}
		return false
	}
	b.reemplazo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			e.reemplazarActual()
			return true
		case fyne.KeyEscape:
			e.cerrarBuscador()
			return true
		default:
		}
		return false
	}
	atajo := func(rotulo, teclas string) string {
		return t("editor.shortcut", i18n.Vars{"label": rotulo, "keys": teclas})
	}
	boton := func(glifo string, icono iconos.Icono, rotulo string, f func()) fyne.CanvasObject {
		return nuevoBotonBarra(glifo, icono, rotulo, f).conPista(rotulo)
	}
	b.btMayus = nuevoBotonBarra(MayusD, iconos.Ninguno, t("editor.matchCase", nil), func() {
		b.mayus = !b.mayus
		b.btMayus.Activo = b.mayus
		b.btMayus.Refresh()
		e.buscarEnNota(b.campo.Texto(), b.i)
	})
	b.btMayus.ConEstado = true
	anterior := boton("", iconos.ChevronArriba, atajo(t("editor.findPrev", nil), "Shift+Enter"),
		func() { e.buscarEnNota(b.campo.Texto(), b.i-1) })
	siguiente := boton("", iconos.ChevronAbajo, atajo(t("editor.findNext", nil), "Enter"),
		func() { e.buscarEnNota(b.campo.Texto(), b.i+1) })
	b.btRepl = nuevoBotonBarra("", iconos.Intercambiar, atajo(t("editor.replace", nil), ui.AtajoDelSistema("Ctrl+H")), e.alternarReemplazo)
	b.btRepl.ConEstado = true
	cerrar := boton("", iconos.Cerrar, t("find.close", nil), e.cerrarBuscador)
	cuenta := &cuentaBuscar{Container: container.New(&textoVivo{t: b.cuenta, e: tema.TextoXS}, b.cuenta), e: e}
	b.fila = container.New(&filaElastica{hueco: tema.Espacio(1.5)}, b.campo, cuenta,
		b.btMayus.conPista(b.btMayus.Rotulo), anterior, siguiente, b.btRepl.conPista(b.btRepl.Rotulo), cerrar)

	b.uno = ui.NuevoBoton(t("editor.replace", nil), e.reemplazarActual)
	b.todos = ui.NuevoBoton(t("editor.replaceAll", nil), e.reemplazarTodo)
	for _, bt := range []*ui.Boton{b.uno, b.todos} {
		bt.Variante, bt.Talla = ui.Secundario, ui.TallaSM
	}
	b.filaRepl = container.New(&filaElastica{hueco: tema.Espacio(1.5)}, b.reemplazo, b.uno, b.todos)
	b.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(1)}, b.fila)
	b.barra = container.NewStack(b.fondo,
		container.NewBorder(nil, container.New(&altoFijo{alto: tema.Borde}, b.filete), nil, nil,
			container.New(ui.Margen{Izq: PXBarraEd, Der: PXBarraEd, Arr: PYBarraEd, Abj: PYBarraEd}, b.cuerpo)))
	return b
}

// alternarReemplazo abre o cierra la fila de reemplazo.
func (e *EditorNota) alternarReemplazo() {
	b := e.buscador
	if b == nil {
		return
	}
	b.conRepl = !b.conRepl
	b.btRepl.Activo = b.conRepl
	b.btRepl.Refresh()
	b.cuerpo.Objects = []fyne.CanvasObject{b.fila}
	if b.conRepl {
		b.cuerpo.Objects = append(b.cuerpo.Objects, b.filaRepl)
	}
	b.cuerpo.Refresh()
	b.barra.Refresh()
	e.Refresh()
	llamar0(e.AlCrecer)
	if c := fyne.CurrentApp().Driver().CanvasForObject(e.zona); c != nil && b.conRepl {
		c.Focus(b.reemplazo)
	}
}

// buscarEnNota busca q y lleva la selección a la coincidencia i (dando la vuelta).
func (e *EditorNota) buscarEnNota(q string, i int) {
	b := e.buscador
	if b == nil {
		return
	}
	b.hallados = coincidenciasDoc(e.doc, q, b.mayus)
	n := len(b.hallados)
	b.i = 0
	if n > 0 {
		b.i = ((i % n) + n) % n
		e.ancla, e.cur = b.hallados[b.i][0], b.hallados[b.i][1]
	}
	e.ponerCuenta(q)
	b.barra.Refresh()
	e.repintar()
	if n > 0 && e.AlMoverCursor != nil {
		_, arriba, alto := e.zona.cajaCursor()
		e.AlMoverCursor(PYZonaEd+arriba, alto)
	}
}

// rehacerHallados vuelve a buscar tras un cambio en el cuerpo con la barra abierta, para
// que los velos sigan al texto; no mueve la selección (se está escribiendo).
func (e *EditorNota) rehacerHallados() {
	b := e.buscador
	if b == nil {
		return
	}
	q := b.campo.Texto()
	b.hallados = coincidenciasDoc(e.doc, q, b.mayus)
	b.i = min(b.i, max(len(b.hallados)-1, 0))
	e.ponerCuenta(q)
}

// ponerCuenta escribe la cuenta de la barra.
func (e *EditorNota) ponerCuenta(q string) {
	b := e.buscador
	b.cuenta.Text = e.textoCuenta(q)
	b.cuenta.Refresh()
}

// textoCuenta es «2 de 5», «Sin coincidencias» o nada si la caja está vacía.
func (e *EditorNota) textoCuenta(q string) string {
	b := e.buscador
	switch n := len(b.hallados); {
	case n > 0:
		return e.tr.T("find.count", i18n.Vars{"current": strconv.Itoa(b.i + 1), "total": strconv.Itoa(n)})
	case q != "":
		return e.tr.T("find.none", nil)
	default:
		return ""
	}
}

// coincidencias es lo que dice la cuenta para quien la lee en voz alta: «5 coincidencias».
func (e *EditorNota) coincidencias() string {
	if e.buscador == nil {
		return ""
	}
	return e.tr.T("editor.matches", i18n.Vars{"count": len(e.buscador.hallados)})
}

// cuentaBuscar es la cuenta de la barra: se VE «2 de 5» y se LEE «5 coincidencias».
type cuentaBuscar struct {
	*fyne.Container
	e *EditorNota
}

var _ fyne.Accessible = (*cuentaBuscar)(nil)

func (c *cuentaBuscar) AccessibilityLabel() string { return c.e.coincidencias() }

func (c *cuentaBuscar) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// reemplazarActual cambia la coincidencia actual por lo que diga la caja de reemplazo y
// salta a la siguiente. Si la selección no es la actual (se ha movido el cursor), solo
// lleva a ella: reemplazar algo que no se ve sería a ciegas.
func (e *EditorNota) reemplazarActual() {
	b := e.buscador
	if b == nil || len(b.hallados) == 0 {
		return
	}
	if !e.actualSeleccionada() {
		e.buscarEnNota(b.campo.Texto(), b.i)
		return
	}
	h, nuevo, i := b.hallados[b.i], b.reemplazo.Texto(), b.i
	e.hacer(func() {
		e.cur = e.doc.reemplazar(h[0], h[1], nuevo)
		e.ancla = e.cur
	})
	e.buscarEnNota(b.campo.Texto(), i)
}

// reemplazarTodo cambia TODAS las coincidencias en un solo paso de deshacer. De la
// última a la primera, para que las posiciones de las de antes sigan valiendo.
func (e *EditorNota) reemplazarTodo() {
	b := e.buscador
	if b == nil || len(b.hallados) == 0 {
		return
	}
	hallados, nuevo := slices.Clone(b.hallados), b.reemplazo.Texto()
	e.hacer(func() {
		for k := len(hallados) - 1; k >= 0; k-- {
			e.cur = e.doc.reemplazar(hallados[k][0], hallados[k][1], nuevo)
		}
		e.ancla = e.cur
	})
	e.buscarEnNota(b.campo.Texto(), 0)
}

// reemplazar cambia [a, z) de un bloque por `nuevo`, con el formato de la primera letra
// de lo que se va, y devuelve dónde acaba lo puesto.
func (d *Documento) reemplazar(a, z Pos, nuevo string) Pos {
	estilo := estiloEn(d.Bloques[a.B], a.O+1)
	p := d.Borrar(a, z)
	if nuevo == "" {
		return p
	}
	return d.Insertar(p, nuevo, estilo)
}

// actualSeleccionada dice si lo seleccionado es justo la coincidencia actual: entonces
// no se tiñe de selección y se ve el velo de la actual.
func (e *EditorNota) actualSeleccionada() bool {
	b := e.buscador
	if b == nil || len(b.hallados) == 0 {
		return false
	}
	a, z := e.seleccion()
	return a == b.hallados[b.i][0] && z == b.hallados[b.i][1]
}

// conservaFoco dice si el foco está en la barra (sus cajas o sus botones): el cuerpo no
// se cierra por eso.
func (b *buscadorNota) conservaFoco(f fyne.Focusable) bool {
	return b.campo.Enfocado() || b.reemplazo.Enfocado() ||
		f != nil && (f == fyne.Focusable(b.uno) || f == fyne.Focusable(b.todos))
}

// velosBuscar son los velos de las coincidencias, detrás del texto y de la selección.
// Se reparten por bloque para recorrer las líneas una sola vez aunque la nota sea larga
// y la búsqueda corta (una letra puede dar cientos).
func (r *zonaRenderer) velosBuscar(v tema.Variante) []fyne.CanvasObject {
	b := r.z.e.buscador
	if b == nil || len(b.hallados) == 0 {
		return nil
	}
	todas, actual := coloresBuscar(v)
	porBloque := map[int][]int{}
	for k, h := range b.hallados {
		porBloque[h[0].B] = append(porBloque[h[0].B], k)
	}
	var out, delante []fyne.CanvasObject
	for _, l := range r.z.maq.Lineas {
		for _, k := range porBloque[l.Bloque] {
			h := b.hallados[k]
			desde, hasta := max(l.Desde, h[0].O), min(l.Hasta, h[1].O)
			if hasta <= desde {
				continue
			}
			x0, x1 := l.XDe(desde), l.XDe(hasta)
			if k == b.i {
				// La actual va la última para que nada la tape.
				delante = append(delante, rect(actual, PXZonaEd+x0, PYZonaEd+l.Y, x1-x0, l.Alto, 0))
				continue
			}
			out = append(out, rect(todas, PXZonaEd+x0, PYZonaEd+l.Y, x1-x0, l.Alto, 0))
		}
	}
	return append(out, delante...)
}

// cerrarBuscador quita la barra y devuelve el foco al cuerpo con la coincidencia
// seleccionada.
func (e *EditorNota) cerrarBuscador() {
	if e.buscador == nil {
		return
	}
	e.buscador = nil
	e.Refresh()
	llamar0(e.AlCrecer)
	e.reenfocar()
}

// textoVivo coloca un `canvas.Text` cuyo contenido cambia: mide lo que lleve AHORA.
type textoVivo struct {
	t *canvas.Text
	e tema.Escalon
}

func (l *textoVivo) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(ui.Medir(l.t.Text, l.e, ui.Normal).Width, l.e.Linea)
}

func (l *textoVivo) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.ColocarTexto(l.t, l.e, 0, (t.Height-l.e.Linea)/2)
}
