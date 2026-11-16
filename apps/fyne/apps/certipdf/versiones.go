// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/packages/ui/src/components/VersionList.tsx

package certipdf

import (
	"context"
	"errors"
	"image/color"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
«Versiones» (`Versiones.tsx`): volver a como estaba el PDF antes de un cambio.

Cada anotación, relleno, censura o firma reescribe el fichero en Drive y el visor no tiene
«Deshacer». Drive guarda cada escritura como una versión y restaura sin perder nada —lo
que había pasa a ser una versión más—, así que aquí se listan, se puede ABRIR una
anterior para mirarla (en la web, en otra pestaña con el visor del navegador; aquí, con
el programa de PDF del sistema, desde descargas) y RESTAURARLA tras confirmarlo. Con
firmas, la confirmación avisa de que la firma puesta después no estará en lo restaurado.
*/

// abrirConElSistema abre un fichero con el programa del sistema; las pruebas lo cambian.
var abrirConElSistema = ficheros.Abrir

// dialogoVersiones es el estado del diálogo.
type dialogoVersiones struct {
	a *dialogoAbierto

	lista       []api.DriveVersion
	cursor      string
	cargadas    bool
	fallo       bool
	cargandoMas bool
	// restaurando es la versión que se está restaurando (0: ninguna).
	restaurando int
	error       string
}

func nuevoDialogoVersiones(a *dialogoAbierto) *dialogoVersiones {
	d := &dialogoVersiones{a: a}
	d.cargar("")
	return d
}

// cargar pide una página del historial. Sin la primera no hay nada que enseñar; sin las
// siguientes, lo cargado sigue valiendo.
func (d *dialogoVersiones) cargar(cursor string) {
	doc := d.a.p.doc
	if doc == nil || doc.pdf == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(d.a.p.ctx, plazo)
		defer parar()
		vs, sig, err := doc.Versiones(ctx, cursor)
		fyne.Do(func() {
			d.cargandoMas = false
			switch {
			case err != nil && cursor == "":
				d.fallo = true
			case err != nil:
				d.error = d.a.p.mensajeDeError(err, "pdf.versions.loadError")
			case cursor == "":
				d.lista, d.cursor, d.cargadas = vs, sig, true
			default:
				d.lista, d.cursor = append(d.lista, vs...), sig
			}
			d.a.rehacer()
		})
	}()
}

// abrir deja la versión en descargas («contrato (v2).pdf») y la abre con el programa de
// PDF del sistema: mirarla no toca el documento que se está editando.
func (d *dialogoVersiones) abrir(v api.DriveVersion) {
	doc := d.a.p.doc
	if doc == nil {
		return
	}
	nombre := strings.TrimSuffix(nombreDeRuta(doc.Ruta()), ".pdf")
	nombre = strings.TrimSuffix(nombre, ".PDF") + " (v" + strconv.Itoa(v.Version) + ").pdf"
	go func() {
		ctx, parar := context.WithTimeout(d.a.p.ctx, plazo)
		defer parar()
		datos, err := doc.Version(ctx, v.Version)
		ruta := ""
		if err == nil {
			ruta, err = guardarEnDescargas(nombre, datos)
		}
		if err == nil {
			err = abrirConElSistema(ruta)
		}
		fyne.Do(func() {
			if err != nil {
				d.error = d.a.p.mensajeDeError(err, "pdf.download.failed")
				d.a.rehacer()
			}
		})
	}()
}

// confirmarRestaurar es el `useConfirm` de la web: dice qué pasa siempre y, con firmas,
// lo que hace dudar. El tono es el primario: restaurar no destruye nada.
func (d *dialogoVersiones) confirmarRestaurar(v api.DriveVersion) {
	p := d.a.p
	if d.restaurando != 0 {
		return
	}
	mensaje := d.mensajeRestaurar(v)
	c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz)
	if c == nil {
		return
	}
	tv := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(mensaje, tema.Escalon{Tam: tema.TextoSM.Tam, Linea: tema.TextoSM.Tam * 1.625}, ui.Normal, "text", tv)
	var dlg *ui.Dialogo
	dlg = ui.Confirmacion(p.tv("pdf.versions.confirm.title", i18n.Vars{"version": v.Version}),
		p.t("ui.cancel"), p.t("pdf.versions.restore"), false, cuerpo,
		func() { dlg.Cerrar() },
		func() { dlg.Cerrar(); d.restaurar(v.Version) })
	dlg.Mostrar(c)
}

// mensajeRestaurar es lo que pregunta la confirmación: una frase del catálogo por caso,
// y la de las firmas DETRÁS de la que pasa siempre, porque es la que hace dudar.
func (d *dialogoVersiones) mensajeRestaurar(v api.DriveVersion) string {
	p := d.a.p
	mensaje := p.tv("pdf.versions.confirm", i18n.Vars{"version": v.Version})
	if p.Contenido != nil && len(p.Contenido.Firmas) > 0 {
		mensaje += " " + p.t("pdf.versions.confirm.signed")
	}
	return mensaje
}

// restaurar hace actual la versión `n`; al acabar, el visor vuelve a leer el documento.
func (d *dialogoVersiones) restaurar(n int) {
	doc := d.a.p.doc
	if doc == nil || d.restaurando != 0 {
		return
	}
	d.restaurando, d.error = n, ""
	d.a.rehacer()
	go func() {
		ctx, parar := context.WithTimeout(d.a.p.ctx, plazo)
		defer parar()
		err := doc.Restaurar(ctx, n)
		fyne.Do(func() {
			d.restaurando = 0
			if err != nil {
				var e *api.Error
				if errors.As(err, &e) && e.Status == http.StatusForbidden {
					d.error = d.a.p.t("pdf.versions.denied")
				} else {
					d.error = d.a.p.mensajeDeError(err, "pdf.versions.failed")
				}
				d.a.rehacer()
				return
			}
			d.a.cerrar()
			d.a.p.versionRestaurada(doc, n)
		})
	}()
}

func (d *dialogoVersiones) vista(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	switch {
	case d.fallo:
		reintentar := ui.NuevoBoton(a.t("pdf.retry"), func() {
			d.fallo = false
			a.rehacer()
			d.cargar("")
		}).Con(ui.Secundario, ui.TallaSM)
		return container.New(ui.Columna{Hueco: tema.Espacio(2)},
			ui.NuevoParrafo(a.t("pdf.versions.loadError"), tema.TextoBase, ui.Normal, "text-muted", v).A(ancho),
			container.NewHBox(reintentar))
	case !d.cargadas:
		r := ui.NuevaRueda(22, "text", v)
		r.Rotulo = a.t("pdf.versions.loading")
		return container.NewCenter(r)
	}

	ayuda := ui.NuevaAyudaEnLinea(a.t("pdf.versions.help"))
	ayuda.Resize(ui.MedidaAyudaEnLinea(ayuda, ancho))
	partes := []fyne.CanvasObject{ayuda}
	if d.error != "" {
		pista := ui.NuevaPistaCampo(d.error).ConTono(ui.AyudaPeligro)
		pista.Ancho = ancho
		partes = append(partes, pista)
	}
	if len(d.lista) <= 1 {
		partes = append(partes, ui.NuevoVacio(a.t("pdf.versions.empty"), ""))
	} else {
		partes = append(partes, d.listaDeVersiones(ancho, v))
	}
	if d.cursor != "" {
		mas := ui.NuevoBoton(a.t("pdf.versions.loadMore"), func() {
			if d.cargandoMas {
				return
			}
			d.cargandoMas = true
			a.rehacer()
			d.cargar(d.cursor)
		}).Con(ui.Fantasma, ui.TallaSM)
		mas.Cargando = d.cargandoMas
		partes = append(partes, container.NewCenter(mas))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// lista es la `VersionList` de la web: una caja con borde y `rounded-md`, una fila por
// versión y un filete entre una y otra.
func (d *dialogoVersiones) listaDeVersiones(ancho float32, v tema.Variante) fyne.CanvasObject {
	// Lo que le queda a cada fila dentro del marco y de su `px-3`.
	dentro := ancho - 2*tema.Borde - 2*tema.Espacio(3)
	filas := make([]fyne.CanvasObject, 0, 2*len(d.lista))
	for i, x := range d.lista {
		if i > 0 {
			filete := canvas.NewRectangle(tema.Color("border", v))
			filete.SetMinSize(fyne.NewSize(0, tema.Borde))
			filas = append(filas, filete)
		}
		filas = append(filas, container.New(ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(2.5), Abj: tema.Espacio(2.5)}, d.fila(x, dentro, v)))
	}
	marco := canvas.NewRectangle(color.Transparent)
	marco.StrokeColor, marco.StrokeWidth, marco.CornerRadius = tema.Color("border", v), tema.Borde, tema.RadioMD
	return container.NewStack(marco, container.New(ui.Margen{Izq: tema.Borde, Der: tema.Borde, Arr: tema.Borde, Abj: tema.Borde},
		container.New(ui.Columna{}, filas...)))
}

// fila es una versión: la tesela del reloj, «Versión n» con la insignia de la actual,
// debajo la fecha, el autor y el tamaño, y a la derecha «Abrir» y «Restaurar» si no es
// la actual.
func (d *dialogoVersiones) fila(x api.DriveVersion, ancho float32, v tema.Variante) fyne.CanvasObject {
	a := d.a
	loc := i18n.Locale(a.p.Idioma)
	if loc == "" {
		loc = i18n.Default
	}
	var derecha fyne.CanvasObject
	if !x.IsCurrent {
		abrir := ui.NuevoBoton(a.t("pdf.versions.open"), func() { d.abrir(x) }).Con(ui.Fantasma, ui.TallaSM)
		abrir.Rotulo = a.p.tv("pdf.versions.openAria", i18n.Vars{"version": x.Version})
		restaurar := ui.NuevoBoton(a.t("pdf.versions.restore"), func() { d.confirmarRestaurar(x) }).Con(ui.Secundario, ui.TallaSM)
		restaurar.Rotulo = a.p.tv("pdf.versions.restoreAria", i18n.Vars{"version": x.Version})
		restaurar.Cargando = d.restaurando == x.Version
		restaurar.Inerte = d.restaurando != 0
		derecha = container.New(filaCentrada{hueco: tema.Espacio(1)}, abrir, restaurar)
	}
	// Lo que queda para los textos: la fila menos la tesela, sus huecos y los botones.
	cabe := ancho - tema.Espacio(9) - tema.Espacio(3)
	if derecha != nil {
		cabe -= derecha.MinSize().Width + tema.Espacio(3)
	}
	cabe = max(cabe, 0)

	nombre := []fyne.CanvasObject{ui.NuevoRotulo(a.p.tv("pdf.versions.version", i18n.Vars{"version": x.Version}), tema.TextoSM, ui.Medio, "text")}
	if x.IsCurrent {
		nombre = append(nombre, ui.NuevaInsignia(a.t("pdf.versions.current"), ui.InsigniaPrimaria))
	}
	fecha := "—"
	if !x.CreatedAt.IsZero() {
		fecha = i18n.FormatDateTime(loc, x.CreatedAt.In(time.Local))
	}
	resto := i18n.FormatBytes(loc, float64(x.SizeBytes))
	if x.AuthorSub != "" {
		resto = x.AuthorSub + " · " + resto
	}
	lineas := []fyne.CanvasObject{container.New(filaCentrada{hueco: tema.Espacio(2)}, nombre...)}
	// En el teléfono, la fecha y debajo autor y tamaño, como la web: en una línea
	// recortada se perdían los dos.
	if a.estrecha() {
		lineas = append(lineas,
			rotuloRecortado(fecha, tema.TextoXS, ui.Normal, "text-subtle", cabe),
			rotuloRecortado(resto, tema.TextoXS, ui.Normal, "text-subtle", cabe))
	} else {
		lineas = append(lineas, rotuloRecortado(fecha+" · "+resto, tema.TextoXS, ui.Normal, "text-subtle", cabe))
	}
	textos := container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, lineas...)
	// La tesela `h-9 w-9 rounded-md bg-bg-subtle` con el reloj de 16 en `text-muted`.
	fondo := canvas.NewRectangle(tema.Color("bg-subtle", v))
	fondo.CornerRadius = tema.RadioMD
	fondo.SetMinSize(fyne.NewSize(tema.Espacio(9), tema.Espacio(9)))
	tesela := container.NewStack(fondo, container.NewCenter(iconos.Trazo(iconos.Reloj, 16, "text-muted", v).Objeto()))
	izquierda := container.New(filaCentrada{hueco: tema.Espacio(3)}, tesela, textos)
	if derecha == nil {
		return izquierda
	}
	return container.NewBorder(nil, nil, nil, derecha, izquierda)
}

// versionRestaurada es lo que hace el visor tras volver a una versión: el fichero es
// otro, así que se lee todo de nuevo, como tras aplicar un cambio.
func (p *Pantalla) versionRestaurada(doc *Documento, n int) {
	if p.doc != doc {
		return
	}
	p.Copia, p.Error = "", ""
	p.Aviso = p.tv("pdf.versions.restored", i18n.Vars{"version": n})
	p.textos = nil
	p.cargar(p.claveUsada)
}
