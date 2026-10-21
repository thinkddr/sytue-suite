// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// paridad-web: apps/frontend/apps/bases/src/pages/index.astro

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Medidas de «Mis bases» (BasesApp.tsx).
var (
	// anchoLista es el `max-w-4xl` de la página.
	anchoLista = 56 * float32(tema.Rem)
	// altoEsqueleto es el `height="3.5rem"` de las dos filas de carga.
	altoEsqueleto = 3.5 * float32(tema.Rem)
)

// cargarLista pide las bases a las que se tiene acceso.
func (p *Pantalla) cargarLista() {
	p.Cargando, p.Fallo = true, ""
	p.Repintar()
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		lista, err := p.svc.ListBases(ctx)
		enHilo(func() {
			p.Cargando = false
			if err != nil {
				p.Fallo = p.mensajeDeError(err)
			} else {
				p.Lista = lista
			}
			p.Repintar()
		})
	})
}

// ajustarCrear apaga «Crear base» sin nombre, como el `disabled={!nombre.trim()}`.
func (p *Pantalla) ajustarCrear() {
	if p.crear == nil {
		return
	}
	p.crear.Inerte = strings.TrimSpace(p.nombre.Texto()) == ""
	p.crear.Cargando = p.creando
	p.crear.Refresh()
}

// crearBase crea la base con el nombre escrito y la abre.
func (p *Pantalla) crearBase() {
	nombre := strings.TrimSpace(p.nombre.Texto())
	if nombre == "" || p.creando || p.svc == nil {
		return
	}
	p.creando, p.Fallo = true, ""
	p.ajustarCrear()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		b, err := p.svc.CreateBase(ctx, nombre)
		enHilo(func() {
			p.creando = false
			if err != nil {
				p.Fallo = p.mensajeDeError(err)
				p.ajustarCrear()
				p.Repintar()
				return
			}
			p.nombre.PonerTexto("")
			p.ajustarCrear()
			p.Abrir(b.ID)
		})
	})
}

// objetoLista es BasesApp.tsx: el título, el alta de una base y la lista.
func (p *Pantalla) objetoLista(v tema.Variante) fyne.CanvasObject {
	// Un botón NUEVO en cada repintado: el que venía de «cargando» se quedaba con el sitio
	// de la rueda y el rótulo cortado (visto en una ventana real, 2026-10-05).
	p.crear = ui.NuevoBoton(p.t("bases.create"), p.crearBase)
	p.ajustarCrear()
	hijos := []fyne.CanvasObject{
		ui.NuevoRotulo(p.t("bases.title"), tema.Texto2XL, ui.SemiNegrita, "text"),
		container.New(extremos{Hueco: tema.Espacio(3)},
			ui.GrupoCampo(p.t("bases.newName"), false, p.nombre, "", ""), p.crear),
	}
	if p.Fallo != "" {
		hijos = append(hijos, alerta(p.Fallo, v))
	}
	switch {
	case p.Cargando:
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(2)},
			ui.NuevoEsqueleto(0, altoEsqueleto), ui.NuevoEsqueleto(0, altoEsqueleto)))
	case len(p.Lista) == 0 && p.Fallo == "":
		hijos = append(hijos, ui.NuevoVacio(p.t("bases.empty.title"), p.t("bases.empty.body")))
	default:
		filas := make([]fyne.CanvasObject, 0, len(p.Lista))
		for _, b := range p.Lista {
			filas = append(filas, p.tarjeta(b))
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
	}
	return container.New(acotado{Max: anchoLista, Margen: tema.Espacio(6)},
		container.New(ui.Columna{Hueco: tema.Espacio(6)}, hijos...))
}

// tarjeta es el `<a>` de una base: su nombre, cuándo se actualizó (y de qué organización
// viene, si es de fuera) y el rol de quien mira.
func (p *Pantalla) tarjeta(b api.BasesBase) *tarjetaBase {
	detalle := p.t("bases.updated", i18n.Vars{"date": i18n.FormatDate(p.tr.Locale(), b.UpdatedAt)})
	if b.HostOrg != "" {
		detalle += " · " + p.t("bases.external", i18n.Vars{"org": b.HostOrg})
	}
	rol, tono := p.t("bases.partial"), ui.InsigniaNeutra
	if b.Role != "" {
		rol = p.t("role." + b.Role)
	}
	if b.Role == api.BasesOwner {
		tono = ui.InsigniaPrimaria
	}
	id := b.ID
	x := &tarjetaBase{Nombre: b.Name, Detalle: detalle, Rol: rol, Tono: tono, AlPulsar: func() { p.Abrir(id) }}
	x.ExtendBaseWidget(x)
	return x
}

// tarjetaBase es la fila de una base en «Mis bases»: `border-border bg-surface
// hover:bg-bg-subtle rounded-lg border p-3`, con el nombre y el detalle a la izquierda
// y la insignia del rol a la derecha. No es una pieza de @sytue/ui: la web la escribe a
// mano en BasesApp.tsx.
type tarjetaBase struct {
	widget.BaseWidget
	Nombre, Detalle, Rol string
	Tono                 ui.Tono
	AlPulsar             func()

	encima, enfocada bool
}

var (
	_ fyne.Tappable     = (*tarjetaBase)(nil)
	_ fyne.Focusable    = (*tarjetaBase)(nil)
	_ desktop.Hoverable = (*tarjetaBase)(nil)
)

var margenTarjeta = tema.Espacio(3)

func (x *tarjetaBase) MinSize() fyne.Size {
	return fyne.NewSize(ui.MedidaInsignia(x.Rol).Width+2*margenTarjeta+tema.Espacio(2),
		2*margenTarjeta+tema.TextoBase.Linea+tema.TextoSM.Linea)
}

func (x *tarjetaBase) Tapped(*fyne.PointEvent) {
	if x.AlPulsar != nil {
		x.AlPulsar()
	}
}

func (x *tarjetaBase) MouseIn(*desktop.MouseEvent)    { x.encima = true; x.Refresh() }
func (x *tarjetaBase) MouseMoved(*desktop.MouseEvent) {}
func (x *tarjetaBase) MouseOut()                      { x.encima = false; x.Refresh() }
func (x *tarjetaBase) FocusGained()                   { x.enfocada = true; x.Refresh() }
func (x *tarjetaBase) FocusLost()                     { x.enfocada = false; x.Refresh() }
func (x *tarjetaBase) TypedRune(r rune) {
	if r == ' ' {
		x.Tapped(nil)
	}
}

func (x *tarjetaBase) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
		x.Tapped(nil)
		return
	}
	ui.SubirEscape(x, k)
}

// AccessibilityLabel dice el nombre, el detalle y el rol, que es lo que lee el enlace.
func (x *tarjetaBase) AccessibilityLabel() string { return x.Nombre + ", " + x.Detalle + ", " + x.Rol }

// AccessibilityRole: es el enlace a la base.
func (x *tarjetaBase) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleLink }

func (x *tarjetaBase) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tarjetaRenderer{
		x:        x,
		caja:     canvas.NewRectangle(tema.Color("surface", v)),
		nombre:   ui.Texto(x.Nombre, tema.TextoBase, ui.SemiNegrita, tema.Color("text", v)),
		detalle:  ui.Texto(x.Detalle, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		insignia: ui.NuevaInsignia(x.Rol, x.Tono),
	}
	r.caja.CornerRadius = tema.RadioLG
	r.caja.StrokeWidth = tema.Borde
	r.Refresh()
	return r
}

type tarjetaRenderer struct {
	x               *tarjetaBase
	caja            *canvas.Rectangle
	nombre, detalle *canvas.Text
	insignia        *ui.Insignia
}

func (r *tarjetaRenderer) Destroy()           {}
func (r *tarjetaRenderer) MinSize() fyne.Size { return r.x.MinSize() }
func (r *tarjetaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.caja, r.nombre, r.detalle, r.insignia}
}

func (r *tarjetaRenderer) Layout(t fyne.Size) {
	r.caja.Resize(t)
	ins := ui.MedidaInsignia(r.x.Rol)
	r.insignia.Resize(ins)
	r.insignia.Move(fyne.NewPos(t.Width-margenTarjeta-ins.Width, (t.Height-ins.Height)/2))
	cabe := max(t.Width-2*margenTarjeta-ins.Width-tema.Espacio(2), 0)
	ui.Truncar(r.nombre, r.x.Nombre, tema.TextoBase, ui.SemiNegrita, cabe)
	ui.Truncar(r.detalle, r.x.Detalle, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoBase, margenTarjeta, margenTarjeta)
	ui.ColocarTexto(r.detalle, tema.TextoSM, margenTarjeta, margenTarjeta+tema.TextoBase.Linea)
}

func (r *tarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := "surface"
	if r.x.encima {
		fondo = "bg-subtle"
	}
	r.caja.FillColor = tema.Color(fondo, v)
	r.caja.StrokeColor, r.caja.StrokeWidth = tema.Color("border", v), tema.Borde
	if r.x.enfocada {
		r.caja.StrokeColor, r.caja.StrokeWidth = tema.Color("focus-ring", v), tema.AnilloFoco
	}
	r.Layout(r.x.Size())
	canvas.Refresh(r.x)
}
