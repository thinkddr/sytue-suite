// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
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

// «Buscar en la nota» (Ctrl+F dentro del editor), el de RichText.tsx: una barra bajo la
// de formato con la caja, la cuenta «2 de 5» y anterior, siguiente y cerrar. Intro va a
// la siguiente, Mayús+Intro a la anterior y Escape cierra dejando seleccionada la
// coincidencia en la que estaba. Con la regla de búsqueda de la suite: sin acentos ni
// mayúsculas.
//
// Todas las coincidencias se pintan con un velo detrás del texto, como los `::highlight`
// de la web (styles/app.css): el amarillo del `<mark>` para todas y uno más subido para
// la actual. La actual es además la SELECCIÓN del cuerpo (lo que queda al cerrar), pero
// mientras la barra está abierta no se tiñe encima: el velo ya la destaca, como en la web.

type buscadorNota struct {
	campo    *ui.Entrada
	cuenta   *canvas.Text
	fondo    *canvas.Rectangle
	filete   *canvas.Rectangle
	barra    *fyne.Container
	hallados [][2]Pos
	i        int
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

// coincidenciasDoc son las de q en el documento, en orden, bloque a bloque.
func coincidenciasDoc(d *Documento, q string) [][2]Pos {
	var out [][2]Pos
	for bi, b := range d.Bloques {
		if !editable(b) {
			continue
		}
		for _, r := range buscar.Rangos([]rune(b.Texto()), q) {
			out = append(out, [2]Pos{{bi, r[0]}, {bi, r[1]}})
		}
	}
	return out
}

func (e *EditorNota) abrirBuscador() {
	if e.buscador == nil {
		e.buscador = e.armarBuscador()
	}
	if e.haySeleccion() {
		e.buscador.campo.PonerTexto(e.Seleccionado())
	}
	e.buscarEnNota(e.buscador.campo.Texto(), 0)
	e.Refresh()
	llamar0(e.AlCrecer)
	if c := fyne.CurrentApp().Driver().CanvasForObject(e.zona); c != nil {
		c.Focus(e.buscador.campo)
	}
}

func (e *EditorNota) armarBuscador() *buscadorNota {
	t := e.tr.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := &buscadorNota{
		campo:  ui.NuevaEntrada(t("find.placeholder", nil)).Con(ui.CampoSM),
		cuenta: ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
	}
	b.campo.Rotulo = t("find.placeholder", nil)
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
	atajo := func(rotulo, teclas string) string {
		return t("editor.shortcut", i18n.Vars{"label": rotulo, "keys": teclas})
	}
	anterior := nuevoBotonBarra("", iconos.ChevronArriba, atajo(t("find.previous", nil), "Shift+Enter"),
		func() { e.buscarEnNota(b.campo.Texto(), b.i-1) })
	siguiente := nuevoBotonBarra("", iconos.ChevronAbajo, atajo(t("find.next", nil), "Enter"),
		func() { e.buscarEnNota(b.campo.Texto(), b.i+1) })
	cerrar := nuevoBotonBarra("", iconos.Cerrar, t("find.close", nil), e.cerrarBuscador)
	cuenta := container.New(&textoVivo{t: b.cuenta, e: tema.TextoXS}, b.cuenta)
	fila := container.New(&filaElastica{hueco: tema.Espacio(1.5)}, b.campo, cuenta, anterior, siguiente, cerrar)
	b.barra = container.NewStack(b.fondo,
		container.NewBorder(nil, container.New(&altoFijo{alto: tema.Borde}, b.filete), nil, nil,
			container.New(ui.Margen{Izq: PXBarraEd, Der: PXBarraEd, Arr: PYBarraEd, Abj: PYBarraEd}, fila)))
	return b
}

// buscarEnNota busca q y lleva la selección a la coincidencia i (dando la vuelta).
func (e *EditorNota) buscarEnNota(q string, i int) {
	b := e.buscador
	if b == nil {
		return
	}
	b.hallados = coincidenciasDoc(e.doc, q)
	n := len(b.hallados)
	b.i = 0
	if n > 0 {
		b.i = ((i % n) + n) % n
		e.ancla, e.cur = b.hallados[b.i][0], b.hallados[b.i][1]
	}
	b.cuenta.Text = e.textoCuenta(q)
	b.cuenta.Refresh()
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
	b.hallados = coincidenciasDoc(e.doc, q)
	b.i = min(b.i, max(len(b.hallados)-1, 0))
	b.cuenta.Text = e.textoCuenta(q)
	b.cuenta.Refresh()
}

// textoCuenta es «2 de 5», «Sin resultados» o nada si la caja está vacía.
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
