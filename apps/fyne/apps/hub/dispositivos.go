// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// The native device screen connects the OS file provider and the suite's OAuth
// consent wizard. The session manages account changes and access revocation.
type Dispositivos struct {
	pagina
	T                 *i18n.Translator
	DriveEnSistema    bool
	DriveOcupado      bool
	PuedeConectarApps bool
	AlCambiarDrive    func(bool)
	AlAbrirDrive      func()
	AlConectarApps    func()
}

// HuecoAsistente is also the spacing used by onboarding.
const HuecoAsistente float32 = 6 * 0.25 * tema.Rem

// HuecoBotonesPie is shared with the onboarding footer.
const HuecoBotonesPie float32 = 2 * 0.25 * tema.Rem

func NuevosDispositivos(t *i18n.Translator) *Dispositivos {
	d := &Dispositivos{T: t, DriveEnSistema: true}
	d.ExtendBaseWidget(d)
	return d
}

func (d *Dispositivos) Resize(s fyne.Size) {
	if d.recordar(s) {
		d.BaseWidget.Resize(s)
		d.Refresh()
		return
	}
	d.BaseWidget.Resize(s)
}

func (d *Dispositivos) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (d *Dispositivos) CreateRenderer() fyne.WidgetRenderer {
	r := &dispositivosRenderer{d: d, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	r.activo = ui.NuevoConmutador("Sytue Drive", func(activo bool) {
		if d.AlCambiarDrive != nil {
			d.AlCambiarDrive(activo)
		}
	})
	r.drive = ui.NuevoBoton(d.T.T("devices.native.openDrive", nil), func() {
		if d.AlAbrirDrive != nil {
			d.AlAbrirDrive()
		}
	}).Con(ui.Secundario, ui.TallaMD)
	r.apps = ui.NuevoBoton(d.T.T("apps.connect", nil), func() {
		if d.PuedeConectarApps && d.AlConectarApps != nil {
			d.AlConectarApps()
		}
	}).Con(ui.Primario, ui.TallaMD)
	r.rehacer()
	return r
}

type dispositivosRenderer struct {
	d               *Dispositivos
	fondo           *ui.FondoSuite
	titulo, entrada *ui.Parrafo
	activo          *ui.Conmutador
	drive, apps     *ui.Boton
	cuerpo          bloque
	objetos         []fyne.CanvasObject
}

func (r *dispositivosRenderer) rehacer() {
	d, t := r.d, r.d.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.PonerVariante(v)
	r.titulo = ui.NuevoParrafo(t.T("devices.title", nil), ui.EscalonTitulo(2), ui.PesoTitulo(2), "text", v)
	r.entrada = ui.NuevoParrafo(t.T("devices.native.body", nil), baseRelajada, ui.Normal, "text-muted", v)
	r.activo.Encendido = d.DriveEnSistema
	if d.AlCambiarDrive == nil || d.DriveOcupado {
		r.activo.Disable()
	} else {
		r.activo.Enable()
	}
	r.activo.Refresh()
	r.drive.Inerte = d.AlAbrirDrive == nil || !d.DriveEnSistema || d.DriveOcupado
	r.drive.Refresh()
	r.apps.Inerte = !d.PuedeConectarApps || d.AlConectarApps == nil
	r.apps.Refresh()
	r.cuerpo = col(HuecoAsistente,
		nuevaBSeccion("Sytue Drive", t.T("devices.native.driveBody", nil),
			col(ui.HuecoPieDialogo, fijo{r.activo}, fijo{r.drive})),
		nuevaBSeccion(t.T("nav.connections", nil), "", fijo{r.apps}))
	r.objetos = append([]fyne.CanvasObject{r.fondo, r.titulo, r.entrada}, r.cuerpo.objetos()...)
}

func (r *dispositivosRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, r.alto(r.d.Ancho())) }
func (r *dispositivosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *dispositivosRenderer) Destroy()                     {}
func (r *dispositivosRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.d.Size())
	canvas.Refresh(r.d)
}
func (r *dispositivosRenderer) alto(ancho float32) float32 { return r.colocar(ancho, false) }
func (r *dispositivosRenderer) Layout(s fyne.Size) {
	if s.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(s.Width, max(s.Height, r.alto(s.Width))))
	r.colocar(s.Width, true)
}

func (r *dispositivosRenderer) colocar(ventana float32, mover bool) float32 {
	x, ancho := CajaAjustes(ventana)
	y := ponerParrafo(r.titulo, x, MargenAjustesY, ancho, mover) + MargenEntradaAjustes
	y = ponerParrafo(r.entrada, x, y, min(ancho, AnchoIntro), mover) + MargenAjustesY
	r.activo.AnchoEtiqueta = max(0, ancho-ui.AnchoCarril-ui.HuecoConmutador)
	if mover {
		r.cuerpo.poner(x, y, ancho)
	}
	return y + r.cuerpo.alto(ancho) + MargenAjustesY
}

// vacioB is shared by onboarding.
type vacioB struct{ v *ui.Vacio }

func (b vacioB) alto(ancho float32) float32 { return b.v.MedidaVacio(ancho) }
func (b vacioB) poner(x, y, ancho float32) {
	ponerObjeto(b.v, x, y, fyne.NewSize(ancho, b.alto(ancho)), true)
}
func (b vacioB) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.v} }
