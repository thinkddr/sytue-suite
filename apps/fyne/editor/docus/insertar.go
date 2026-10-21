// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La pestaña «Insertar» de Docus (`ribbon/tabs/Insertar.tsx` con `kind === 'document'`):
// Tabla · (herramientas de tabla, con el cursor en una) · Imagen · Enlace · Elementos ·
// Comentario.
//
// Formas, Gráfico y SmartArt van en `figuras.go`.
//
// ponytail: faltan Power BI y Formulario. Escriben en el folio HTML que `rico` aún no
// entiende (el `<iframe>` del informe, `<span data-formfield>`…): ofrecerlos aquí dejaría
// el documento en solo lectura al insertarlos.

// MaxImagen es el tope de la web para una imagen del documento: la data: URL entera,
// porque el documento viaja entero en cada guardado.
const MaxImagen = 1_500_000

// ElegirImagen abre el selector de ficheros del sistema (`accept="image/*"`) y da lo
// elegido como data: URL. Es una variable para que las pruebas no abran nada.
var ElegirImagen = func(listo func(src string)) {
	editor.ElegirDelSistema(func(rutas []string) {
		ext := strings.ToLower(filepath.Ext(rutas[0]))
		if !slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".webp"}, ext) {
			return
		}
		datos, err := os.ReadFile(rutas[0]) //nolint:gosec // G304: la ruta la ha elegido la persona en el diálogo del sistema
		if err != nil {
			return
		}
		listo("data:" + http.DetectContentType(datos) + ";base64," + base64.StdEncoding.EncodeToString(datos))
	})
}

func (in *inicio) cuerpoInsertar(c *editor.Contexto) fyne.CanvasObject {
	f, _ := c.Lienzo.(*Folio)
	formato := editor.Formato{}
	if f != nil {
		formato = f.Formato()
	}
	mandar := func(cmd editor.Comando, valor string) func() {
		return func() { c.Mandar(cmd, valor) }
	}
	inerte := func(o *editor.BotonCinta) *editor.BotonCinta {
		o.Inerte = o.Inerte || c.SoloLectura
		return o
	}
	emergente := func(i iconos.Ed, clave string, cuerpo func(cerrar func()) fyne.CanvasObject) *editor.BotonCinta {
		return inerte(editor.NuevoBotonEmergente(i, c.Traducir(clave), cuerpo))
	}
	grupos := []fyne.CanvasObject{
		editor.NuevoGrupo(c.Traducir("settings.table"), emergente(iconos.EdTabla, "insert.table", func(cerrar func()) fyne.CanvasObject {
			return editor.NuevoPanelTabla(c, func(filas, cols int, cabecera bool) {
				cerrar()
				valor := strconv.Itoa(filas) + "x" + strconv.Itoa(cols)
				if cabecera {
					valor += "h"
				}
				c.Mandar(editor.InsertarTabla, valor)
			})
		})),
	}
	if formato.EnTabla {
		// Herramientas de celda, solo con el cursor dentro de una tabla (`fmt.inTable`).
		verticales := []editor.Opcion{
			{Valor: "top", Rotulo: c.Traducir("sheet.valign.top")},
			{Valor: "middle", Rotulo: c.Traducir("sheet.valign.middle")},
			{Valor: "bottom", Rotulo: c.Traducir("sheet.valign.bottom")},
		}
		vertical := editor.NuevoSelectorCinta(c.Traducir("table.cellVAlign"), AnchoSelectorVertical, verticales, "top",
			func(v string) { c.Mandar(editor.TablaVertical, v) })
		vertical.Inerte = c.SoloLectura
		fondo := editor.NuevaMuestraCinta(c, iconos.EdResaltado, c.Traducir("table.cellColor"), editor.PaletaResaltado[0],
			editor.PaletaResaltado, func(v string) { c.Mandar(editor.TablaFondo, v) })
		fondo.Inerte = c.SoloLectura
		accion := func(i iconos.Ed, clave string, cmd editor.Comando) *editor.BotonCinta {
			return inerte(editor.NuevoBotonCinta(i, c.Traducir(clave), mandar(cmd, "")))
		}
		grupos = append(grupos, editor.SeparadorGrupos(), editor.NuevoGrupo(c.Traducir("group.tableTools"),
			accion(iconos.EdFilaArriba, "table.rowAbove", editor.TablaFilaEncima),
			accion(iconos.EdFilaAbajo, "table.rowBelow", editor.TablaFilaDebajo),
			accion(iconos.EdColIzq, "table.colLeft", editor.TablaColIzq),
			accion(iconos.EdColDer, "table.colRight", editor.TablaColDer),
			accion(iconos.EdCombinar, "sheet.merge", editor.TablaCombinar),
			accion(iconos.EdDividirCeldas, "table.splitCell", editor.TablaDividir),
			fondo, vertical,
			accion(iconos.EdPapelera, "table.delete", editor.TablaBorrar),
		))
	}
	imagen := inerte(editor.NuevoBotonCinta(iconos.EdImagen, c.Traducir("insert.image"), func() {
		ElegirImagen(func(src string) {
			if len(src) > MaxImagen {
				if c.Avisar != nil {
					c.Avisar(c.Traducir("insert.msg.imageMax"), "")
				}
				return
			}
			c.Mandar(editor.InsertarImagen, src)
		})
	}))
	comentario := editor.NuevoBotonCinta(iconos.EdComentarioNuevo, c.Traducir("insert.comment"), c.NuevoComentario)
	comentario.Inerte = c.NuevoComentario == nil || c.SinRed
	seleccion := ""
	if f != nil {
		seleccion = f.textoSeleccionado()
	}
	grupos = append(grupos,
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("slides.box.image"), imagen),
		editor.SeparadorGrupos(),
	)
	if f != nil {
		grupos = append(grupos, grupoFormas(c, f, inerte), editor.SeparadorGrupos(), grupoGrafico(c, f, inerte), editor.SeparadorGrupos(),
			grupoSmart(c, f, inerte), editor.SeparadorGrupos())
	}
	// Las herramientas de la imagen seleccionada, contextuales como las de tabla.
	if f != nil && formato.Imagen {
		if g := f.grupoImagen(c); g != nil {
			grupos = append(grupos, g, editor.SeparadorGrupos())
		}
	}
	grupos = append(grupos,
		editor.NuevoGrupo(c.Traducir("group.link"), emergente(iconos.EdEnlace, "insert.link", func(cerrar func()) fyne.CanvasObject {
			return editor.NuevoPanelEnlace(c, seleccion, func(href, texto string) {
				cerrar()
				c.Mandar(editor.InsertarEnlace, href+"\n"+texto)
			})
		})),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.elements"),
			inerte(editor.NuevoBotonCinta(iconos.EdSeparador, c.Traducir("insert.horizontalRule"), mandar(editor.InsertarRegla, ""))),
			inerte(editor.NuevoBotonCinta(iconos.EdSaltoPagina, c.Traducir("insert.pageBreak"), mandar(editor.SaltoPagina, ""))),
			emergente(iconos.EdSimbolo, "insert.symbol", func(cerrar func()) fyne.CanvasObject {
				return editor.NuevoPanelSimbolos(c, func(s string) {
					cerrar()
					c.Mandar(editor.InsertarTexto, s)
				})
			}),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.headerFooter"),
			inerte(editor.NuevoBotonCinta(iconos.EdEncabezado, c.Traducir("insert.header"), mandar(editor.InsertarEncabezado, ""))),
			inerte(editor.NuevoBotonCinta(iconos.EdPie, c.Traducir("insert.footer"), mandar(editor.InsertarPie, ""))),
			// El texto del campo va resuelto al insertarlo, como en la web; luego lo pone la vista.
			inerte(editor.NuevoBotonCinta(iconos.EdNumeroPagina, c.Traducir("insert.pageNumber"),
				mandar(editor.InsertarNumeroPagina, c.Traducir("status.pageOf")))),
			inerte(editor.NuevoBotonCinta(iconos.EdVertical, c.Traducir("insert.differentFirstPage"), mandar(editor.PrimeraPaginaDistinta, ""))),
		),
		editor.SeparadorGrupos(),
		// «Comentario» es el mismo «Nuevo comentario» de Colaboración: pregunta el texto y
		// lo ancla a la selección. El de la web es un popover que pone la cita delante
		// del cuerpo, de cuando aún no había anclas.
		editor.NuevoGrupo(c.Traducir("group.comment"), comentario),
	)
	// Los bloques reutilizables de Sytue y de la organización, al final como en la web.
	if c.Bloques != nil {
		grupos = append(grupos, editor.SeparadorGrupos(), c.Bloques.Grupo(c))
	}
	return editor.Grupos(grupos...)
}

// AnchoSelectorVertical es el `width={92}` de la alineación vertical de la celda.
const AnchoSelectorVertical float32 = 92

// insertar ejecuta las órdenes de la pestaña «Insertar». Devuelve false si no es una.
//
//nolint:gocyclo // una rama por orden de la cinta
func (f *Folio) insertar(c editor.Comando, valor string) bool {
	estructura := func(hacer func() (rico.Lugar, int, bool)) {
		f.antesDeCambiar(false)
		if l, p, ok := hacer(); ok {
			f.irA(l, p)
		}
		f.reestructurado()
	}
	enBloque := func(frag string) {
		estructura(func() (rico.Lugar, int, bool) {
			f.borrarSeleccion()
			l, ok := f.doc.InsertarHTML(f.doc.Bloques(), f.cur.b, f.cur.p, frag)
			return l, 0, ok
		})
	}
	celda := func() *rico.Celda {
		if f.cur.b < len(f.bloques) {
			return f.bloques[f.cur.b].Celda
		}
		return nil
	}
	orden := map[editor.Comando]rico.OrdenTabla{
		editor.TablaFilaEncima: rico.FilaEncima, editor.TablaFilaDebajo: rico.FilaDebajo,
		editor.TablaColIzq: rico.ColIzquierda, editor.TablaColDer: rico.ColDerecha,
		editor.TablaBorrarFila: rico.BorrarFila, editor.TablaBorrarCol: rico.BorrarColumna, editor.TablaBorrar: rico.BorrarTabla,
	}
	switch c {
	case editor.InsertarTexto:
		f.escribir(valor)
	case editor.InsertarTabla:
		spec, cabecera := strings.CutSuffix(valor, "h")
		fs, cs, _ := strings.Cut(spec, "x")
		filas, _ := strconv.Atoi(fs)
		cols, _ := strconv.Atoi(cs)
		if filas < 1 || cols < 1 || filas > 20 || cols > 20 {
			return true
		}
		enBloque(rico.HTMLTabla(filas, cols, cabecera))
	case editor.InsertarRegla:
		enBloque(rico.HTMLRegla)
	case editor.SaltoPagina:
		enBloque(rico.HTMLSaltoPagina)
	case editor.InsertarImagen:
		estructura(func() (rico.Lugar, int, bool) {
			f.borrarSeleccion()
			e := f.edicion(f.cur.b)
			e.InsertarImagen(f.cur.p, valor)
			return e.Lugar, f.cur.p + 1, true
		})
	case editor.InsertarEnlace:
		href, texto, _ := strings.Cut(valor, "\n")
		estructura(func() (rico.Lugar, int, bool) {
			a, b := f.orden()
			if a.b != b.b {
				f.borrarSeleccion()
				a, b = f.cur, f.cur
			}
			f.cur, f.anc = a, a
			e := f.edicion(a.b)
			e.InsertarEnlace(a.p, b.p, href, texto)
			return e.Lugar, a.p + len([]rune(texto)), true
		})
	case editor.TablaCombinar:
		estructura(func() (rico.Lugar, int, bool) {
			l, ok := f.doc.Combinar(f.celdasSeleccion())
			return l, 0, ok
		})
	case editor.TablaDividir:
		estructura(func() (rico.Lugar, int, bool) {
			l, ok := f.doc.Dividir(celda())
			return l, 0, ok
		})
	case editor.TablaFondo, editor.TablaVertical:
		prop := map[editor.Comando]string{editor.TablaFondo: "background-color", editor.TablaVertical: "vertical-align"}[c]
		f.antesDeCambiar(false)
		if f.doc.PonerCeldas(f.celdasSeleccion(), prop, valor) {
			f.reestructurado()
		}
	default:
		o, ok := orden[c]
		if !ok {
			return false
		}
		estructura(func() (rico.Lugar, int, bool) {
			l, ok := f.doc.Tabla(celda(), o)
			return l, 0, ok
		})
	}
	return true
}

// celdasSeleccion son las celdas de la selección, sin repetir, en orden de
// documento: las que toca el rango, como `selectedCells` de la web.
func (f *Folio) celdasSeleccion() []*rico.Celda {
	a, b := f.orden()
	var out []*rico.Celda
	for i := a.b; i <= b.b && i < len(f.bloques); i++ {
		// Solo las de la tabla donde empieza la selección, como la web.
		if c := f.bloques[i].Celda; c != nil && !slices.Contains(out, c) && (len(out) == 0 || c.Tabla == out[0].Tabla) {
			out = append(out, c)
		}
	}
	return out
}

// ---- Referencias ----

// cuerpoReferencias es la pestaña «Referencias» (`Referencias.tsx`): la tabla de
// contenidos y las notas al pie, con los vínculos y el comentario de la web.
func (in *inicio) cuerpoReferencias(c *editor.Contexto) fyne.CanvasObject {
	accion := func(i iconos.Ed, clave string, cmd editor.Comando, valor string) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, c.Traducir(clave), func() { c.Mandar(cmd, valor) })
		b.Inerte = c.SoloLectura
		return b
	}
	titulo := c.Traducir("ref.toc")
	comentario := editor.NuevoBotonCinta(iconos.EdComentarioNuevo, c.Traducir("comments.add"), c.NuevoComentario)
	comentario.Inerte = c.NuevoComentario == nil || c.SinRed
	enlace := editor.NuevoBotonEmergente(iconos.EdEnlace, c.Traducir("insert.link"), func(cerrar func()) fyne.CanvasObject {
		seleccion := ""
		if f, ok := c.Lienzo.(*Folio); ok {
			seleccion = f.textoSeleccionado()
		}
		return editor.NuevoPanelEnlace(c, seleccion, func(href, texto string) {
			cerrar()
			c.Mandar(editor.InsertarEnlace, href+"\n"+texto)
		})
	})
	enlace.Inerte = c.SoloLectura
	return editor.Grupos(
		editor.NuevoGrupo(titulo,
			accion(iconos.EdIndice, "ref.tocInsert", editor.InsertarIndice, titulo),
			accion(iconos.EdHistorial, "ref.tocUpdate", editor.ActualizarIndice, titulo),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.index"), in.botonMarcarIndice(c),
			accion(iconos.EdIndiceAlfa, "ref.indexBuild", editor.GenerarIndice, ""),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.captions"),
			in.botonLeyenda(c, iconos.EdLeyenda, "ref.captionTable", rico.LeyendaTabla, "dialog.caption.table"),
			in.botonLeyenda(c, iconos.EdImagen, "ref.captionFigure", rico.LeyendaFigura, "dialog.caption.figure"),
			in.botonReferencia(c),
			accion(iconos.EdHistorial, "ref.captionsUpdate", editor.ActualizarLeyendas, ""),
		),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.notes"),
			accion(iconos.EdNotaPie, "ref.footnote", editor.NotaPie, c.Traducir("ref.note.placeholder")),
			accion(iconos.EdNotaFinal, "ref.endnote", editor.NotaFinal, ""),
			accion(iconos.EdHistorial, "ref.endnotesUpdate", editor.ActualizarFinal, ""),
		),
		editor.SeparadorGrupos(),
		in.grupoCitas(c),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.links"), enlace),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.comment"), comentario),
		editor.SeparadorGrupos(),
		in.grupoCorrespondencia(c),
	)
}

// botonMarcarIndice es «Marcar entrada de índice»: pregunta el término, con lo seleccionado
// de partida, y marca con él la selección (o deja el marcador en el cursor).
func (in *inicio) botonMarcarIndice(c *editor.Contexto) *editor.BotonCinta {
	b := editor.NuevoBotonCinta(iconos.EdEntradaIndice, c.Traducir("ref.indexMark"), nil)
	b.AlPulsar = func() {
		f, ok := c.Lienzo.(*Folio)
		lienzo := fyne.CurrentApp().Driver().CanvasForObject(b)
		if !ok || lienzo == nil {
			return
		}
		ui.Pregunta(c.Traducir("dialog.index.entry"), strings.TrimSpace(f.textoSeleccionado()), c.Traducir("ui.cancel"),
			c.Traducir("ui.save"), c.Traducir("ui.required"), func(s string) { c.Mandar(editor.MarcarIndice, s) }).Mostrar(lienzo)
	}
	b.Inerte = c.SoloLectura
	return b
}

// botonLeyenda es «Leyenda de tabla» o «de figura»: pregunta el texto y la inserta. El
// texto puede ir vacío —la leyenda es entonces solo «Tabla 3»—, como `permitirVacio` en
// Referencias.tsx; cancelar no inserta nada.
func (in *inicio) botonLeyenda(c *editor.Contexto, i iconos.Ed, rotulo, clase, pregunta string) *editor.BotonCinta {
	b := editor.NuevoBotonCinta(i, c.Traducir(rotulo), nil)
	b.AlPulsar = func() {
		lienzo := fyne.CurrentApp().Driver().CanvasForObject(b)
		if lienzo == nil {
			return
		}
		ui.PreguntaOpcional(c.Traducir(pregunta), "", c.Traducir("ui.cancel"), c.Traducir("ui.save"),
			func(s string) { c.Mandar(editor.InsertarLeyenda, clase+"\n"+s) }).Mostrar(lienzo)
	}
	b.Inerte = c.SoloLectura
	return b
}

// botonReferencia es «Insertar referencia cruzada»: el desplegable con las leyendas del
// documento —su etiqueta en `text-xs font-medium` y su texto en `sytue-meta-dense`— que
// inserta la elegida.
func (in *inicio) botonReferencia(c *editor.Contexto) *editor.BotonCinta {
	b := editor.NuevoBotonEmergente(iconos.EdRefCruzada, c.Traducir("ref.crossReference.insert"), func(cerrar func()) fyne.CanvasObject {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		f, ok := c.Lienzo.(*Folio)
		if !ok {
			return nil
		}
		ls := f.doc.Leyendas()
		if len(ls) == 0 {
			return ui.NuevoParrafo(c.Traducir("ref.captions.empty"), tema.TextoXS, ui.Normal, "text-muted", v).A(AnchoReferencias - 2*tema.Espacio(2))
		}
		filas := make([]fyne.CanvasObject, 0, len(ls))
		for _, l := range ls {
			filas = append(filas, nuevaFilaLeyenda(l, func() {
				cerrar()
				c.Mandar(editor.ReferenciaCruzada, l.ID)
			}))
		}
		return container.NewVScroll(container.New(ui.Columna{}, filas...))
	})
	b.Inerte = c.SoloLectura
	return b
}

// AnchoReferencias es el `w-60` del desplegable de referencias cruzadas.
var AnchoReferencias = tema.Espacio(60)

// referencias ejecuta las órdenes de «Referencias». Devuelve false si no es una.
func (f *Folio) referencias(c editor.Comando, valor string) bool {
	hacer := f.hacerReferencia
	switch c {
	case editor.InsertarIndice:
		hacer(func() (rico.Lugar, bool) {
			f.borrarSeleccion()
			return f.doc.InsertarIndice(f.doc.Bloques(), f.cur.b, f.cur.p, valor)
		}, false)
	case editor.ActualizarIndice:
		hacer(func() (rico.Lugar, bool) {
			if !f.doc.ActualizarIndice(valor) {
				return f.doc.InsertarIndice(f.doc.Bloques(), f.cur.b, f.cur.p, valor)
			}
			return f.bloques[min(f.cur.b, len(f.bloques)-1)].Lugar, true
		}, false)
	case editor.NotaPie:
		hacer(func() (rico.Lugar, bool) {
			f.borrarSeleccion()
			return f.doc.InsertarNotaPie(f.doc.Bloques(), f.cur.b, f.cur.p, valor)
		}, true)
	case editor.NotaFinal:
		hacer(func() (rico.Lugar, bool) {
			f.borrarSeleccion()
			return f.doc.InsertarNotaFinal(f.doc.Bloques(), f.cur.b, f.cur.p, f.traducir("ref.note.placeholder"), f.traducir("ref.endnote.back"))
		}, true)
		if f.haySeleccion() {
			f.anc = punto{f.cur.b, min(2, f.cur.p)} // sin el enlace de vuelta ni su espacio
		}
	case editor.ActualizarFinal:
		hacer(func() (rico.Lugar, bool) {
			aqui := f.bloques[min(f.cur.b, len(f.bloques)-1)].Lugar
			if !f.doc.RenumerarNotasFinal(f.traducir("ref.note.placeholder"), f.traducir("ref.endnote.back")) {
				return rico.Lugar{}, false
			}
			return aqui, true
		}, false)
	default:
		return false
	}
	return true
}

// hacerReferencia corre una orden de Referencias que devuelve dónde dejar el cursor, en UN
// paso de deshacer; `seleccionar` deja seleccionado el texto provisional (una nota nueva):
// teclear lo sustituye.
func (f *Folio) hacerReferencia(accion func() (rico.Lugar, bool), seleccionar bool) {
	f.antesDeCambiar(false)
	l, ok := accion()
	if !ok {
		f.hist.atras = f.hist.atras[:len(f.hist.atras)-1]
		return
	}
	f.irA(l, 0)
	f.reestructurado()
	if seleccionar {
		f.anc, f.cur = punto{f.cur.b, 0}, punto{f.cur.b, f.largo(f.cur.b)}
	}
}

// indicesYLeyendas ejecuta las órdenes del índice alfabético y de las leyendas. Devuelve
// false si no es una.
func (f *Folio) indicesYLeyendas(c editor.Comando, valor string) bool {
	hacer := f.hacerReferencia
	switch c {
	case editor.MarcarIndice:
		termino, sub, ok := rico.TerminoIndice(valor)
		if !ok {
			return true
		}
		f.antesDeCambiar(false)
		a, b := f.orden()
		if a.b != b.b {
			b = punto{a.b, f.largo(a.b)} // la marca va en el bloque donde empieza la selección
		}
		if !f.edicion(a.b).MarcarIndice(a.p, b.p, termino, sub) {
			f.hist.atras = f.hist.atras[:len(f.hist.atras)-1]
			return true
		}
		f.ed = nil
		f.cambiado()
	case editor.GenerarIndice:
		hacer(func() (rico.Lugar, bool) {
			return f.doc.GenerarIndiceAlfa(f.doc.Bloques(), f.cur.b, f.traducir("ref.index"), f.traducir("ref.index.empty"))
		}, false)
	case editor.InsertarLeyenda:
		clase, texto, _ := strings.Cut(valor, "\n")
		etiqueta := f.traducir(map[bool]string{true: "caption.figure", false: "caption.table"}[clase == rico.LeyendaFigura])
		hacer(func() (rico.Lugar, bool) {
			return f.doc.InsertarLeyenda(f.doc.Bloques(), f.cur.b, clase, etiqueta, texto)
		}, false)
		f.cur.p = f.largo(f.cur.b)
		f.anc = f.cur
	case editor.ActualizarLeyendas:
		foto := paso{f.doc.HTML(), f.cur, f.anc}
		if f.doc.ActualizarLeyendas(f.traducir("caption.table"), f.traducir("caption.figure")) {
			f.apuntar(foto, false)
			f.reestructurado()
		}
	case editor.ReferenciaCruzada:
		for _, l := range f.doc.Leyendas() {
			if l.ID != valor {
				continue
			}
			f.antesDeCambiar(false)
			a, b := f.orden()
			if a.b != b.b {
				f.borrarSeleccion()
				a, b = f.cur, f.cur
			}
			e := f.edicion(a.b)
			e.InsertarReferencia(a.p, b.p, l)
			f.irA(e.Lugar, a.p+len([]rune(l.Etiqueta)))
			f.reestructurado()
		}
	default:
		return false
	}
	return true
}
