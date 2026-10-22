// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El grupo «Correspondencia» de Referencias (`CorrespondenciaGroup` de
// Correspondencia.tsx): pegar el origen de datos, insertar un campo, la vista previa
// registro a registro y «Finalizar y combinar» (exportar el HTML o reemplazar el
// documento). El HTML es de `rico` (combinacion.go).

// Las órdenes de la combinación; en la web no pasan por `exec`.
const (
	cmdOrigen     editor.Comando = "mergeSource" // valor: el origen en JSON, o "" para quitarlo
	cmdCampo      editor.Comando = "mergeField"  // valor: el nombre del campo
	cmdReemplazar editor.Comando = "mergeReplace"
)

// Anchos de los desplegables: `w-72`, `w-52` y `w-60`.
var (
	AnchoOrigen             = tema.Espacio(72)
	AnchoCampos             = tema.Espacio(52)
	AnchoFinalizar          = tema.Espacio(60)
	AnchoContadorPr float32 = 3.5 * tema.Rem // `min-w-[3.5rem]`
)

// ordenCombinacion ejecuta las órdenes de la combinación. Devuelve false si no es una.
func (f *Folio) ordenCombinacion(c editor.Comando, valor string) bool {
	switch c {
	case cmdOrigen:
		var o *rico.Origen
		if valor != "" {
			o = &rico.Origen{}
			if json.Unmarshal([]byte(valor), o) != nil {
				return true
			}
		}
		f.antesDeCambiar(false)
		f.doc.PonerOrigen(o)
		f.reestructurado()
	case cmdCampo:
		f.antesDeCambiar(false)
		f.borrarSeleccion()
		e := f.edicion(f.cur.b)
		e.InsertarCampoCombinacion(f.cur.p, valor)
		f.irA(e.Lugar, f.cur.p+1)
		f.reestructurado()
	case cmdReemplazar:
		o := f.doc.OrigenCombinacion()
		if o == nil || len(o.Registros) == 0 {
			return true
		}
		f.antesDeCambiar(false)
		f.doc = rico.Parse(f.doc.Combinado(*o))
		f.cur, f.anc = punto{}, punto{}
		f.reestructurado()
	default:
		return false
	}
	return true
}

// VistaPrevia pinta en cada campo el valor del registro i (`applyPreview`), y el folio
// queda en solo lectura mientras dura, como en la web: nada de eso se guarda. -1 sale.
func (f *Folio) VistaPrevia(i int) {
	o := f.doc.OrigenCombinacion()
	if i < 0 || o == nil || len(o.Registros) == 0 {
		f.previa = nil
	} else {
		i = min(i, len(o.Registros)-1)
		f.previa = &i
	}
	f.remaquetar()
	f.pag.Refresh()
}

// registroPrevia es el registro que se está viendo, o nil.
func (f *Folio) registroPrevia() map[string]string {
	if f.previa == nil {
		return nil
	}
	if o := f.doc.OrigenCombinacion(); o != nil && *f.previa < len(o.Registros) {
		return o.Registros[*f.previa]
	}
	return nil
}

var reNombreFichero = regexp.MustCompile(`[\\/:*?"<>|]+`)

// nombreCombinado es `safeFilename`: el título saneado, a 116 caracteres, con «-combinado».
func nombreCombinado(titulo string) string {
	t := strings.TrimSpace(titulo)
	if t == "" {
		t = "documento"
	}
	t = reNombreFichero.ReplaceAllString(t, "_")
	if r := []rune(t); len(r) > 116 {
		t = string(r[:116])
	}
	return t + "-combinado"
}

// exportarCombinado es «Exportar combinado (HTML)»: el documento combinado en una página
// con el idioma de la interfaz, en la carpeta de descargas; el aviso dice el fichero.
func exportarCombinado(c *editor.Contexto, f *Folio) {
	o := f.doc.OrigenCombinacion()
	if o == nil || len(o.Registros) == 0 {
		return
	}
	titulo := c.Titulo
	if titulo == "" {
		titulo = c.Traducir("merge.exportTitle")
	}
	pagina := `<!doctype html><html lang="` + escaparHTML(string(c.Loc)) + `"><head><meta charset="utf-8"><title>` +
		escaparHTML(titulo) + `</title></head><body>` + f.doc.Combinado(*o) + `</body></html>`
	dir, err := ficheros.Carpeta()
	if err == nil {
		var ruta string
		if ruta, err = ficheros.Guardar(dir, nombreCombinado(c.Titulo)+".html", strings.NewReader(pagina)); err == nil && c.Notificar != nil {
			c.Notificar(ui.AvisoExito, filepath.Base(ruta))
		}
	}
	if err != nil && c.Avisar != nil {
		c.Avisar(c.Traducir("doc.msg.exportFailed"), "")
	}
}

var escaparHTML = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace

// grupoCorrespondencia es el grupo «Correspondencia»: el origen, insertar campo, la vista
// previa y finalizar; en la vista previa, anterior, «n / total», siguiente y salir.
func (in *inicio) grupoCorrespondencia(c *editor.Contexto) fyne.CanvasObject {
	f, _ := c.Lienzo.(*Folio)
	if f == nil {
		return editor.NuevoGrupo(c.Traducir("group.mailMerge"))
	}
	o := f.doc.OrigenCombinacion()
	total := 0
	if o != nil {
		total = len(o.Registros)
	}
	if f.previa != nil {
		i := *f.previa
		ir := func(d int) func() { return func() { f.VistaPrevia(i + d); c.Refrescar() } }
		ant := editor.NuevoBotonCinta(iconos.EdChevronIzq, c.Traducir("merge.prevRecord"), ir(-1))
		ant.Inerte = i <= 0
		sig := editor.NuevoBotonCinta(iconos.EdChevronDer, c.Traducir("merge.nextRecord"), ir(1))
		sig.Inerte = i >= total-1
		cuenta := ui.NuevoRotulo(strconv.Itoa(i+1)+" / "+strconv.Itoa(total), tema.TextoXS, ui.Normal, "text-muted")
		salir := editor.NuevoBotonCinta(iconos.EdPrevia, c.Traducir("merge.exitPreview"), func() { f.VistaPrevia(-1); c.Refrescar() })
		salir.Activo = true
		return editor.NuevoGrupo(c.Traducir("group.mailMerge"), ant,
			container.New(anchoMinimo{AnchoContadorPr}, container.NewCenter(cuenta)), sig, salir)
	}
	origen := editor.NuevoBotonEmergente(iconos.EdTabla, c.Traducir("extdata.source"), func(cerrar func()) fyne.CanvasObject {
		return panelOrigen(c, f, cerrar)
	})
	origen.Activo, origen.Inerte = o != nil, c.SoloLectura
	campo := editor.NuevoBotonEmergente(iconos.EdCampoCorresp, c.Traducir("mail.mergeField.insert"), func(cerrar func()) fyne.CanvasObject {
		return panelCampos(c, f)
	})
	campo.Inerte = c.SoloLectura || o == nil || len(o.Campos) == 0
	previa := editor.NuevoBotonCinta(iconos.EdPrevia, c.Traducir("merge.preview"), func() { f.VistaPrevia(0); c.Refrescar() })
	previa.Inerte = total == 0
	fin := editor.NuevoBotonEmergente(iconos.EdCorrespondencia, c.Traducir("mail.finishMerge"), func(cerrar func()) fyne.CanvasObject {
		return panelFinalizar(c, f, total, cerrar)
	})
	fin.Inerte = c.SoloLectura || total == 0
	return editor.NuevoGrupo(c.Traducir("group.mailMerge"), origen, campo, previa, fin)
}

// panelOrigen es `OrigenPopover`: el resumen del origen guardado (con la papelera para
// quitarlo), el área donde se pega la tabla, lo que se detecta y «Guardar origen».
func panelOrigen(c *editor.Contexto, f *Folio, cerrar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := AnchoOrigen - 2*tema.Espacio(2)
	objs := []fyne.CanvasObject{
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1), 0, 0), ui.NuevoRotulo(c.Traducir("extdata.source"), tema.TextoXS, ui.Medio, "text")),
	}
	if o := f.doc.OrigenCombinacion(); o != nil {
		resumen := ui.NuevoRotulo(c.Traducir("merge.source.summary", i18n.Vars{"fields": len(o.Campos), "records": len(o.Registros)}),
			tema.Texto2XS, ui.Normal, "text-muted")
		quitar := ui.NuevoBotonIcono(iconos.Ninguno, c.Traducir("extdata.source.remove"), func() {
			cerrar()
			c.Mandar(cmdOrigen, "")
		})
		quitar.Glifo, quitar.Lado, quitar.TamIcono, quitar.TrazoEncima = iconos.TrazoEd(iconos.EdPapelera, 14, "text-muted", v), ui.LadoIconoSM, 14, "danger-text"
		// `border-b pb-2 mb-2`.
		raya := canvas.NewRectangle(tema.Color("border", v))
		raya.SetMinSize(fyne.NewSize(ancho, tema.Borde))
		objs = append(objs, container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(2), 0, 0), container.New(ui.Columna{},
			container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(2), 0, 0), container.New(filaAcciones{}, resumen, quitar)), raya)))
	}
	// `FIELD_CLASS h-24 resize-none font-mono`: el campo de la cinta (`px-1.5 text-xs`,
	// borde `border`) en JetBrains Mono, seis líneas de `text-xs` que dan los 84 de `h-24`.
	area := ui.NuevaAreaTexto(c.Traducir("merge.source.example"))
	area.ConPropia(ui.CampoPropio{MargenX: tema.Espacio(1.5), Texto: tema.TextoXS, Borde: "border", Radio: tema.RadioSM, Mono: true})
	area.Filas = 6
	area.Ancho = ancho
	detectado := ui.NuevoRotulo("", tema.Texto2XS, ui.Normal, "text-subtle")
	guardar := nuevoBotonAccion(c.Traducir("merge.source.save"), func() {
		o := rico.ParsearOrigen(area.Texto())
		if len(o.Campos) == 0 {
			return
		}
		datos, _ := json.Marshal(o)
		cerrar()
		c.Mandar(cmdOrigen, string(datos))
	})
	guardar.Inerte = true
	area.AlCambiar(func(s string) {
		detectado.Texto = ""
		o := rico.ParsearOrigen(s)
		if strings.TrimSpace(s) != "" {
			detectado.Texto = c.Traducir("merge.source.noColumns")
			if len(o.Campos) > 0 {
				detectado.Texto = c.Traducir("merge.source.detected", i18n.Vars{"fields": len(o.Campos), "records": len(o.Registros)})
			}
		}
		detectado.Refresh()
		guardar.Inerte = len(o.Campos) == 0
		guardar.Refresh()
	})
	objs = append(objs,
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(0.5), 0, 0), ui.NuevoRotulo(c.Traducir("merge.paste.hint"), tema.Texto2XS, ui.Normal, "text-subtle")),
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1.5), 0, 0), area),
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1.5), 0, 0), detectado),
		guardar)
	return container.New(anchoFijo{ancho}, container.New(ui.Columna{}, objs...))
}

// panelCampos es `InsertarCampoPopover`: los campos del origen; el pulsado va al cursor.
func panelCampos(c *editor.Contexto, f *Folio) fyne.CanvasObject {
	o := f.doc.OrigenCombinacion()
	if o == nil {
		return nil
	}
	filas := make([]fyne.CanvasObject, 0, 1+len(o.Campos))
	filas = append(filas, container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1), tema.Espacio(1), 0),
		ui.NuevoRotulo(c.Traducir("insert.atCursor"), tema.Texto2XS, ui.Normal, "text-subtle")))
	for _, campo := range o.Campos {
		filas = append(filas, nuevaFilaDoble(campo, "", AnchoCampos-2*tema.Espacio(2), c.Traducir("merge.insertField", i18n.Vars{"field": campo}),
			func() { c.Mandar(cmdCampo, campo) }))
	}
	return container.NewVScroll(container.New(ui.Columna{}, filas...))
}

// panelFinalizar es `FinalizarPopover`: exportar el combinado en HTML, o reemplazar el
// documento por él, que antes se confirma (la plantilla y el origen se pierden).
func panelFinalizar(c *editor.Contexto, f *Folio, total int, cerrar func()) fyne.CanvasObject {
	ancho := AnchoFinalizar - 2*tema.Espacio(2)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	exportar := nuevoBotonAccion(c.Traducir("merge.export.html"), func() { cerrar(); exportarCombinado(c, f) })
	exportar.Glifo = iconos.TrazoEd(iconos.EdDescargar, 14, "text", v)
	reemplazar := nuevoBotonAccion(c.Traducir("merge.replaceDocument"), func() {
		cerrar()
		lienzo := fyne.CurrentApp().Driver().CanvasForObject(f)
		if lienzo == nil {
			return
		}
		cuerpo := ui.NuevoParrafo(c.Traducir("dialog.merge.reset", i18n.Vars{"total": total}), tema.TextoSM, ui.Normal, "text", v).A(320 - 2*tema.Espacio(6))
		var d *ui.Dialogo
		d = ui.Confirmacion(c.Traducir("merge.finish"), c.Traducir("ui.cancel"), c.Traducir("merge.finish"), true, cuerpo,
			func() { d.Cerrar() }, func() { d.Cerrar(); c.Mandar(cmdReemplazar, "") })
		d.Mostrar(lienzo)
	})
	return container.New(anchoFijo{ancho}, container.New(ui.Columna{},
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1), 0, 0), ui.NuevoRotulo(c.Traducir("mail.finishMerge"), tema.TextoXS, ui.Medio, "text")),
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(2), 0, 0),
			ui.NuevoParrafo(c.Traducir("merge.finish.description", i18n.Vars{"total": total}), tema.Texto2XS, ui.Normal, "text-subtle", v).A(ancho)),
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1.5), 0, 0), exportar),
		reemplazar))
}

// anchoMinimo da a su hijo al menos ese ancho (`min-w-[…]`).
type anchoMinimo struct{ ancho float32 }

func (a anchoMinimo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(max(a.ancho, m.Width), m.Height)
}

func (a anchoMinimo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}
