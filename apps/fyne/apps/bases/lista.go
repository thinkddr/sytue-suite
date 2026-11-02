// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// paridad-web: apps/frontend/apps/bases/src/pages/index.astro

import (
	"context"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
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
	view := p.Widget.View()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		lista, err := p.svc.ListBases(ctx)
		if err == nil {
			publicarBasesWidget(ctx, view, lista)
		}
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

func publicarBasesWidget(ctx context.Context, view widgets.View, lista []api.BasesBase) {
	items := make([]widgets.Item, 0, min(len(lista), widgets.MaxItems))
	for _, b := range lista {
		nombre := strings.TrimSpace(b.Name)
		if nombre == "" {
			continue
		}
		var startAt int64
		if !b.UpdatedAt.IsZero() && b.UpdatedAt.Unix() > 0 {
			startAt = b.UpdatedAt.Unix()
		}
		items = append(items, widgets.Item{
			Title:    nombre,
			Subtitle: b.Description,
			Badge:    strings.ToUpper(b.Role),
			StartAt:  startAt,
		})
		if len(items) >= widgets.MaxItems {
			break
		}
	}
	_ = view.PublicarDetalle(ctx, widgets.Detail{
		Counters: []widgets.Counter{{Kind: "documents", Value: int64(len(lista))}},
		Items:    widgets.SanitizeItems(items),
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
	x := &tarjetaBase{Nombre: b.Name, Detalle: detalle, Rol: rol, Tono: tono, Color: tonoDeBase(id), AlPulsar: func() { p.Abrir(id) }}
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
	// Color es el color de grupo de la base, en su icono (`CardIcon`).
	Color    ui.TonoTarjeta
	AlPulsar func()

	encima, enfocada bool
}

var (
	_ fyne.Tappable     = (*tarjetaBase)(nil)
	_ fyne.Focusable    = (*tarjetaBase)(nil)
	_ desktop.Hoverable = (*tarjetaBase)(nil)
)

var margenTarjeta = tema.Espacio(3)

// ladoIconoBase es el `size-10` del icono de color de una base, y ladoGlifoBase su glifo.
var (
	ladoIconoBase = tema.Espacio(10)
	ladoGlifoBase = float32(20)
)

func (x *tarjetaBase) MinSize() fyne.Size {
	return fyne.NewSize(ladoIconoBase+tema.Espacio(3)+ui.MedidaInsignia(x.Rol).Width+2*margenTarjeta+tema.Espacio(2),
		2*margenTarjeta+max(tema.TextoBase.Linea+tema.TextoSM.Linea, ladoIconoBase))
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
		icono:    canvas.NewRectangle(color.Transparent),
	}
	r.icono.CornerRadius = tema.RadioLG
	relleno, tinta := ui.TonoSolido(x.Color, v)
	r.icono.FillColor = relleno
	r.glifo = iconos.TrazoConTinta(iconos.BaseDeDatos, ladoGlifoBase, tinta)
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
	icono           *canvas.Rectangle
	glifo           *iconos.Dibujo
}

func (r *tarjetaRenderer) Destroy()           {}
func (r *tarjetaRenderer) MinSize() fyne.Size { return r.x.MinSize() }
func (r *tarjetaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.caja, r.icono, r.glifo.Objeto(), r.nombre, r.detalle, r.insignia}
}

func (r *tarjetaRenderer) Layout(t fyne.Size) {
	r.caja.Resize(t)
	ins := ui.MedidaInsignia(r.x.Rol)
	r.insignia.Resize(ins)
	r.insignia.Move(fyne.NewPos(t.Width-margenTarjeta-ins.Width, (t.Height-ins.Height)/2))
	yIcono := (t.Height - ladoIconoBase) / 2
	r.icono.Move(fyne.NewPos(margenTarjeta, yIcono))
	r.icono.Resize(fyne.NewSquareSize(ladoIconoBase))
	r.glifo.Colocar(margenTarjeta+(ladoIconoBase-ladoGlifoBase)/2, yIcono+(ladoIconoBase-ladoGlifoBase)/2, ladoGlifoBase)
	xTexto := margenTarjeta + ladoIconoBase + tema.Espacio(3)
	cabe := max(t.Width-xTexto-margenTarjeta-ins.Width-tema.Espacio(2), 0)
	ui.Truncar(r.nombre, r.x.Nombre, tema.TextoBase, ui.SemiNegrita, cabe)
	ui.Truncar(r.detalle, r.x.Detalle, tema.TextoSM, ui.Normal, cabe)
	yTexto := (t.Height - tema.TextoBase.Linea - tema.TextoSM.Linea) / 2
	ui.ColocarTexto(r.nombre, tema.TextoBase, xTexto, yTexto)
	ui.ColocarTexto(r.detalle, tema.TextoSM, xTexto, yTexto+tema.TextoBase.Linea)
}

func (r *tarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, tinta := ui.TonoSolido(r.x.Color, v)
	r.icono.FillColor = relleno
	r.glifo.TenirCon(tinta)
	r.nombre.Color = tema.Color("text", v)
	r.detalle.Color = tema.Color("text-muted", v)
	r.insignia.Refresh()
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
