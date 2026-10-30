// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El menú «Archivo» de la cinta (`ArchivoMenu` de `Ribbon.tsx`, §6.2): el botón que abre
// la tira, delante de las pestañas, y su menú con las descargas, guardar versión,
// renombrar y volver.
//
// Ribbon.tsx: `h-6 rounded-sm bg-header text-header-fg ring-1 ring-inset`,
// centrado en el contenedor h-full de la tira; la primera pestaña va detrás con gap-1.

// rotuloFormato es la clave de cada descarga para este tipo de documento: se nombra el
// formato que le toca —«Word», «Excel», «PowerPoint»—, no el estándar.
func rotuloFormato(t Tipo, formato string) string {
	for _, command := range office.Exports() {
		if command.ID == formato {
			return command.Labels[t.officeKind()]
		}
	}
	return ""
}

func (t Tipo) officeKind() string {
	switch t {
	case Hoja:
		return "spreadsheet"
	case Presentacion:
		return "presentation"
	case Documento:
	}
	return "document"
}

// MedidaBotonArchivo es el botón: el rótulo en `text-sm font-medium` con `px-3`, y de
// alto `h-6` (24 px con raíz de 16).
func MedidaBotonArchivo(rotulo string) fyne.Size {
	return fyne.NewSize(ui.Medir(rotulo, tema.TextoSM, ui.Medio).Width+2*MargenPestana, ui.SueloTactil(tema.Espacio(6)))
}

// BotonArchivo es el disparador del menú: `bg-header text-header-fg h-6 rounded-sm
// px-3 text-sm font-medium ring-1 ring-inset`, con brightness-125 al apuntar o abrir.
type BotonArchivo struct {
	activable
	ctx   *Contexto
	menu  *ui.Menu
	ancla fyne.CanvasObject
	antes func() []fyne.CanvasObject
}

var _ fyne.Widget = (*BotonArchivo)(nil)

// NuevoBotonArchivo crea el botón con su menú.
func NuevoBotonArchivo(ctx *Contexto) *BotonArchivo {
	b := &BotonArchivo{ctx: ctx}
	b.Rotulo = ctx.Traducir("file.menu")
	b.AlPulsar = b.abrir
	b.ExtendBaseWidget(b)
	return b
}

// Menu construye el menú con el estado de ahora: sin red, las descargas y guardar
// versión se apagan, como en la web.
func (b *BotonArchivo) Menu() *ui.Menu {
	c := b.ctx
	item := func(clave string, inerte bool, f func(), vars ...i18n.Vars) *ui.ElementoMenu {
		e := ui.NuevoElementoMenu(c.Traducir(clave, vars...), iconos.Ninguno, f)
		e.Denso = true // `size="sm"`
		e.Inerte = inerte || f == nil
		return e
	}
	var elems []fyne.CanvasObject
	if b.antes != nil {
		elems = append(elems, b.antes()...)
	}
	// «Nuevo» y «Abrir…», solo en nativo (abrir.go); sin gancho no salen, para que un
	// editor que aún no los tiene no enseñe entradas muertas.
	if c.Nuevo != nil {
		elems = append(elems, item(claveNuevo(c.Tipo), c.SinRed, c.Nuevo))
	}
	if c.Abrir != nil {
		elems = append(elems, item("list.new.open", c.SinRed, c.Abrir))
	}
	if len(elems) > 0 {
		elems = append(elems, ui.NuevoSeparadorMenu())
	}
	if c.GuardarEnDrive != nil {
		elems = append(elems, item("file.saveToDrive", c.SinRed, c.GuardarEnDrive))
	}
	for _, command := range office.Exports() {
		f := command.ID
		var hacer func()
		if c.Exportar != nil {
			hacer = func() { c.Exportar(f) }
		}
		elems = append(elems, item(rotuloFormato(c.Tipo, f), !command.Available(c.SinRed, c.Exportando != ""), hacer))
		if command.Option != nil { // «PDF accesible»: el interruptor con tic de Ribbon.tsx
			op := item(command.Option.Label, false, func() {
				PDFSimple = !PDFSimple
				if c.Refrescar != nil {
					c.Refrescar()
				}
			})
			op.Marcada, op.MantieneAbierto = !PDFSimple, true
			elems = append(elems, op)
		}
	}
	// Ribbon.tsx añade estas dos descargas después de OFFICE_EXPORTS.
	for _, formato := range []string{"ooxml", "odf"} {
		clave := "file.downloadTemplateOoxml"
		if formato == "odf" {
			clave = "file.downloadTemplateOdf"
		}
		var hacer func()
		if c.ExportarPlantilla != nil {
			hacer = func() { c.ExportarPlantilla(formato) }
		}
		elems = append(elems, item(clave, c.SinRed || c.Exportando != "", hacer,
			i18n.Vars{"ext": api.ExportTemplateExt(c.Tipo.Kind(), formato)}))
	}
	elems = append(elems, ui.NuevoSeparadorMenu(),
		item("file.saveVersion", c.SinRed, c.preguntarVersion(b)),
		item("file.rename", b.antes != nil && c.AlRenombrar == nil, c.EnfocarTitulo),
		ui.NuevoSeparadorMenu(),
		// En nativo «volver» es a la portada de la app, como la flecha de la cabecera:
		// al Hub se va cambiando de aplicación (`docs/CONVENTIONS.md` §13.6).
		item("ui.backToList", false, c.Volver))
	m := ui.NuevoMenu(elems...)
	m.AlInicio = true // `absolute top-full left-0`
	m.Rotulo = b.Rotulo
	m.Ancla = b.ancla
	m.Separacion = tema.Espacio(1)
	// `w-max max-w-[min(360px,90vw)] min-w-[220px]`, usando el panel común.
	m.Ancho = min(ui.MedidaMenu(m, fyne.Size{}).Width, 360)
	if lienzo := fyne.CurrentApp().Driver().CanvasForObject(b); lienzo != nil {
		m.Ancho = max(ui.MinAnchoMenu, min(m.Ancho, lienzo.Size().Width*0.9))
	}
	return m
}

func (b *BotonArchivo) abrir() {
	if b.menu != nil && b.menu.Abierto() {
		b.menu.Cerrar()
		return
	}
	b.menu = b.Menu()
	b.menu.Mostrar(b)
	b.Refresh()
}

// MinSize es la medida declarada.
func (b *BotonArchivo) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return MedidaBotonArchivo(b.Rotulo)
}

// CreateRenderer compone el fondo del puntero, el aro y el rótulo.
func (b *BotonArchivo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := b.colores(v)
	r := &archivoRenderer{
		b:      b,
		caja:   ui.NuevaCaja(fondo, borde, tema.RadioSM),
		aro:    ui.NuevoAroCon("chrome-focus", tema.RadioSM, ui.SinHueco, v).Abierto(),
		rotulo: ui.Texto(b.Rotulo, tema.TextoSM, ui.Medio, tinta),
	}
	r.objetos = append(r.aro.Antes(r.caja.Objetos()...), r.rotulo)
	return r
}

// colores compone el fondo y el ring-inset sobre la superficie de la tira. El
// filtro CSS brightness-125 multiplica los canales visibles, sin tocar el alfa.
func (b *BotonArchivo) colores(v tema.Variante) (fondo, borde, tinta color.NRGBA) {
	fondo = tokens.SobreSRGB(tema.Color("header-bg", v), tema.Color("surface", v))
	borde = tokens.SobreSRGB(tokens.Crudo("chrome-border", v), fondo)
	tinta = tema.Color("header-fg", v)
	if b.Encima() || (b.menu != nil && b.menu.Abierto()) {
		for _, c := range []*color.NRGBA{&fondo, &borde, &tinta} {
			c.R = uint8(min(255, (int(c.R)*5+2)/4))
			c.G = uint8(min(255, (int(c.G)*5+2)/4))
			c.B = uint8(min(255, (int(c.B)*5+2)/4))
		}
	}
	return fondo, borde, tinta
}

type archivoRenderer struct {
	b       *BotonArchivo
	caja    *ui.Caja
	aro     *ui.Aro
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *archivoRenderer) MinSize() fyne.Size           { return MedidaBotonArchivo(r.b.Rotulo) }
func (r *archivoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *archivoRenderer) Destroy()                     {}

func (r *archivoRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	caja := ui.Medir(r.b.Rotulo, tema.TextoSM, ui.Medio)
	ui.ColocarTexto(r.rotulo, tema.TextoSM, (t.Width-caja.Width)/2, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *archivoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := r.b.colores(v)
	r.caja.Pintar(fondo, borde)
	r.rotulo.Text = r.b.Rotulo
	r.rotulo.Color = tinta
	r.rotulo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
}
