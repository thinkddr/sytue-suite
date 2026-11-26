// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La VISTA RÁPIDA de un evento: EventPopover.tsx. Se abre junto al evento pulsado en
// cualquier vista, en vez de saltar al formulario: título, cuándo, lugar, videollamada
// y descripción, y las acciones —editar, duplicar, eliminar; o copiarlo a tu calendario
// si es de solo lectura; y contestar si te han invitado—.
//
//	border-border bg-surface-raised w-72 rounded-lg border p-3.5 shadow-md
//	cabecera   `flex items-start gap-2`: punto `mt-1 h-2.5`, título `text-sm
//	           leading-snug font-semibold break-words`, aspa `-mt-0.5 -mr-1`
//	detalle    `mt-2 flex-col gap-1.5 pl-[18px]`, `text-xs`
//	acciones   `border-t mt-3 pt-2.5 flex flex-wrap gap-2`, botones `sm`
const (
	// MargenVistaRapida es el `MARGIN`: lo mínimo que se aparta del borde de la ventana.
	MargenVistaRapida float32 = 8
	// SeparacionVistaRapida es el `GAP` entre el evento y el panel.
	SeparacionVistaRapida float32 = 6
	// SangriaDetalle es el `pl-[18px]` del detalle: el punto más su hueco.
	SangriaDetalle float32 = 18
)

var (
	// AnchoVistaRapida es el `w-72`: 18rem, 288 con el rem de 16, lo mismo que la reserva
	// `WIDTH = 288` con la que la web mide antes de montar.
	AnchoVistaRapida = tema.Espacio(72)
	rellenoRapida    = tema.Espacio(3.5)
	ladoPuntoRapida  = tema.Espacio(2.5)
	// escalonSnug es `text-sm leading-snug` (1,375).
	escalonSnug = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: tema.TextoSM.Tam * 1.375}
)

// ColocarVistaRapida es el `useLayoutEffect` de EventPopover: debajo del evento y
// alineada a su izquierda; si no cabe debajo, ENCIMA; y siempre dentro de la ventana
// con 8 px de aire.
func ColocarVistaRapida(ancla fyne.Position, tamAncla, panel, ventana fyne.Size) fyne.Position {
	x := ancla.X
	if x+panel.Width > ventana.Width-MargenVistaRapida {
		x = ventana.Width - MargenVistaRapida - panel.Width
	}
	x = max(x, MargenVistaRapida)
	y := ancla.Y + tamAncla.Height + SeparacionVistaRapida
	if y+panel.Height > ventana.Height-MargenVistaRapida {
		if encima := ancla.Y - SeparacionVistaRapida - panel.Height; encima >= MargenVistaRapida {
			y = encima
		} else {
			y = max(MargenVistaRapida, ventana.Height-MargenVistaRapida-panel.Height)
		}
	}
	return fyne.NewPos(x, y)
}

// AccionesVistaRapida son lo que la vista rápida puede pedir. Las que vengan a nil no
// se pintan (`onDuplicate` y `onResponder` son opcionales en la web).
type AccionesVistaRapida struct {
	Cerrar    func()
	Editar    func()
	Duplicar  func()
	Eliminar  func()
	Responder func(estado string)
	// Unirse abre la videollamada. Lo pone la app porque abrir un enlace es del sistema.
	Unirse func()
	// DescargarICS guarda el evento como .ics en Descargas (`popover.downloadIcs`). Nil:
	// sin servicio que lo dé.
	DescargarICS func()
	// Notas pide a Notepads las notas que cuelgan del evento; con error, la sección no
	// sale. AbrirNotas abre una ruta de Notepads y Crecer avisa de que el panel ha
	// cambiado de alto. Nil: sin Notepads.
	Notas      func(listo func([]api.Note, error))
	AbrirNotas func(ruta string)
	Crecer     func()
}

// nuevaVistaRapida arma el panel de una ocurrencia. `respuesta` es `miRespuesta`: con
// ella (no vacía) sale «¿Vas a ir?», con la respuesta dada marcada.
//
// Lo que devuelve es lo de DENTRO: la caja, el `p-3.5`, la sombra y la capa son de
// `ui.MostrarEmergenteEn`, en `surface-raised`.
func (p *Pantalla) nuevaVistaRapida(o Ocurrencia, respuesta string, a AccionesVistaRapida) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	util := AnchoVistaRapida - 2*(rellenoRapida+tema.Borde)
	titulo := resumenDe(o, p.t("event.untitled", nil))

	// La cabecera: punto, título que parte en líneas y el aspa.
	aspa := ui.NuevoBoton("", a.Cerrar).Con(ui.Fantasma, ui.TallaSM)
	aspa.SoloIcono, aspa.Icono, aspa.TamIcono = true, iconos.Cerrar, 16
	aspa.Rotulo = p.t("common.close", nil)
	anchoTitulo := util - ladoPuntoRapida - 2*espacio2 - (ui.MedidaBoton("", ui.TallaSM, 0).Height - tema.Espacio(1))
	cabecera := &cabeceraRapida{
		punto:  puntoDe(o.Color, ladoPuntoRapida),
		titulo: ui.NuevoParrafo(titulo, escalonSnug, ui.SemiNegrita, "text", v).A(anchoTitulo),
		aspa:   aspa,
	}
	hijos := []fyne.CanvasObject{container.New(cabecera, cabecera.punto, cabecera.titulo, aspa)}

	// El detalle, sangrado bajo el título.
	var detalle []fyne.CanvasObject
	if cuando := p.cuandoDe(o); cuando != "" {
		detalle = append(detalle, textoXS(cuando, "text-muted", util-SangriaDetalle, 1))
	}
	if Disponible(o) {
		// «Disponible» se dice en el detalle: en la rejilla se ve como cualquier otro.
		detalle = append(detalle, textoXS(p.t("popover.showsAsFree", nil), "text-muted", util-SangriaDetalle, 1))
	}
	if o.Event.Location != "" {
		detalle = append(detalle, textoXS(o.Event.Location, "text-muted", util-SangriaDetalle, 1))
	}
	if o.Event.ConferenceURL != "" {
		detalle = append(detalle, nuevoBotonUnirse(p.t("popover.join", nil), a.Unirse))
	}
	if o.Event.Description != "" {
		// `sytue-meta line-clamp-3 break-words`.
		detalle = append(detalle, textoXS(o.Event.Description, "text-subtle", util-SangriaDetalle, 3))
	}
	if len(detalle) > 0 {
		hijos = append(hijos, container.New(&sangria{Izq: SangriaDetalle, Arr: espacio2, Hueco: tema.Espacio(1.5)}, detalle...))
	}

	// Quién organiza y qué ha contestado cada uno, bajo el detalle (`mt-2.5 pl-[18px]`):
	// es lo que se mira para saber si la reunión sigue en pie.
	if inv := p.seccionInvitados(o.Event, util-SangriaDetalle, tema.TextoXS); inv != nil {
		hijos = append(hijos, container.New(&sangria{Izq: SangriaDetalle, Arr: tema.Espacio(2.5)}, inv))
	}

	// Las notas de Notepads llegan después: su hueco se rellena cuando contesta.
	if a.Notas != nil {
		hueco := container.New(ui.Columna{})
		hijos = append(hijos, hueco)
		a.Notas(func(l []api.Note, err error) {
			if err != nil {
				return
			}
			hueco.Objects = []fyne.CanvasObject{p.seccionNotas(o, l, a.AbrirNotas)}
			hueco.Refresh()
			if a.Crecer != nil {
				a.Crecer()
			}
		})
	}

	if respuesta != "" && a.Responder != nil {
		botones := make([]fyne.CanvasObject, 0, 4)
		botones = append(botones, textoXS(p.t("popover.respond", nil), "text-subtle", util, 1))
		botones = append(botones, p.botonesRespuesta(respuesta, ui.TallaSM, a.Responder)...)
		hijos = append(hijos, seccionRapida(util, botones, true))
	}
	if o.SoloLectura {
		botones := []fyne.CanvasObject{textoXS(p.t("popover.readOnly", nil), "text-subtle", util, 0)}
		if a.Duplicar != nil {
			botones = append(botones, ui.NuevoBoton(p.t("popover.copyToMine", nil), a.Duplicar).Con(ui.Secundario, ui.TallaSM))
		}
		if a.DescargarICS != nil {
			botones = append(botones, botonICS(p.t("popover.downloadIcs", nil), a.DescargarICS))
		}
		hijos = append(hijos, seccionRapida(util, botones, true))
	} else {
		botones := []fyne.CanvasObject{ui.NuevoBoton(p.t("common.edit", nil), a.Editar).Con(ui.Primario, ui.TallaSM)}
		if a.Duplicar != nil {
			botones = append(botones, ui.NuevoBoton(p.t("popover.duplicate", nil), a.Duplicar).Con(ui.Secundario, ui.TallaSM))
		}
		if a.DescargarICS != nil {
			botones = append(botones, botonICS(p.t("popover.downloadIcs", nil), a.DescargarICS))
		}
		botones = append(botones, ui.NuevoBoton(p.t("common.delete", nil), a.Eliminar).Con(ui.Peligro, ui.TallaSM))
		hijos = append(hijos, seccionRapida(util, botones, false))
	}

	// ponytail: el `aria-label` del diálogo («Detalles de …», `popover.details`) no se
	// anuncia: la capa del emergente no es un objeto accesible con nombre.
	return container.New(&anchoFijo{Ancho: util}, container.New(ui.Columna{}, hijos...))
}

// botonICS es «Descargar .ics» de EventPopover: fantasma, con su flecha, para que no
// compita con «Editar».
func botonICS(rotulo string, alPulsar func()) *ui.Boton {
	return ui.NuevoBoton(rotulo, alPulsar).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Descargar, iconos.TallaSM)
}

// botonesRespuesta son los tres botones de «¿Vas a ir?» (Sí, Quizá, No), como
// `aspectoRespuesta` de EventPopover.tsx: primario y con la marca ✓ solo el de la
// respuesta dada, para que se vea qué contestaste; sin respuesta («pending»), los tres
// secundarios, que un «Sí» primario se leería como «ya has dicho que sí».
func (p *Pantalla) botonesRespuesta(respuesta string, talla ui.Talla, responder func(estado string)) []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, 3)
	for _, r := range []struct{ clave, estado string }{
		{"popover.yes", "accepted"},
		{"popover.maybe", "tentative"},
		{"popover.no", "declined"},
	} {
		estado := r.estado
		b := ui.NuevoBoton(p.t(r.clave, nil), func() { responder(estado) }).Con(ui.Secundario, talla)
		if estado == respuesta {
			b.Variante, b.Icono, b.TamIcono = ui.Primario, iconos.Tic, 16
		}
		out = append(out, b)
	}
	return out
}

// anchoFijo declara un ancho fijo y el alto de su hijo: el `w-72` menos el relleno.
type anchoFijo struct{ Ancho float32 }

func (a *anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(a.Ancho, objs[0].MinSize().Height)
}

func (a *anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// cabeceraRapida es `flex items-start gap-2` con el aspa metida `-mt-0.5 -mr-1`: el
// margen negativo la saca hacia arriba y a la derecha SIN quitarle sitio al título más
// que lo que ocupa dentro.
type cabeceraRapida struct {
	punto  fyne.CanvasObject
	titulo *ui.Parrafo
	aspa   *ui.Boton
}

func (c *cabeceraRapida) MinSize([]fyne.CanvasObject) fyne.Size {
	lado := c.aspa.MinSize().Height
	return fyne.NewSize(0, max(c.titulo.MinSize().Height, lado-espacio0p5, tema.Espacio(1)+ladoPuntoRapida))
}

func (c *cabeceraRapida) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.punto.Move(fyne.NewPos(0, tema.Espacio(1)))
	c.punto.Resize(fyne.NewSize(ladoPuntoRapida, ladoPuntoRapida))
	lado := c.aspa.MinSize().Height
	c.aspa.Move(fyne.NewPos(t.Width-lado+tema.Espacio(1), -espacio0p5))
	c.aspa.Resize(fyne.NewSize(lado, lado))
	x := ladoPuntoRapida + espacio2
	c.titulo.Move(fyne.NewPos(x, 0))
	c.titulo.Resize(fyne.NewSize(max(t.Width-x-espacio2-lado+tema.Espacio(1), 0), c.titulo.MinSize().Height))
}

// puntoDe es un punto redondo de color.
func puntoDe(c color.NRGBA, lado float32) *canvas.Rectangle {
	r := canvas.NewRectangle(c)
	r.CornerRadius = lado / 2
	r.SetMinSize(fyne.NewSize(lado, lado))
	return r
}

// textoXS es un párrafo de `text-xs` con su tinta; `lineas` es el `line-clamp` (0: sin
// tope; 1: una línea recortada, el `truncate`).
func textoXS(s, token string, ancho float32, lineas int) *ui.Parrafo {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return ui.NuevoParrafo(s, tema.TextoXS, ui.Normal, token, v).A(ancho).Recortado(lineas)
}

// sangria es `pl-*` más `mt-*` sobre una columna con `gap-*`.
type sangria struct{ Izq, Arr, Hueco float32 }

func (s *sangria) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := ui.Columna{Hueco: s.Hueco}.MinSize(objs)
	return fyne.NewSize(m.Width+s.Izq, m.Height+s.Arr)
}

func (s *sangria) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := s.Arr
	for _, o := range objs {
		h := o.MinSize().Height
		w := t.Width - s.Izq
		if b, ok := o.(*botonUnirse); ok {
			w = b.MinSize().Width
		}
		o.Move(fyne.NewPos(s.Izq, y))
		o.Resize(fyne.NewSize(max(w, 0), h))
		y += h + s.Hueco
	}
}

// seccionRapida es `border-border mt-3 flex flex-wrap items-center gap-2 border-t
// pt-2.5`: un filete y, debajo, lo que haya en líneas que se parten si no caben. Con
// `rotulada`, el primer hijo es el texto `w-full` que va en su propia línea.
func seccionRapida(util float32, hijos []fyne.CanvasObject, rotulada bool) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filete := canvas.NewRectangle(tema.Color("border", v))
	return container.New(&conFileteArriba{Arr: tema.Espacio(3), Pad: tema.Espacio(2.5)},
		filete, container.New(ui.Flujo{Hueco: espacio2, Ancho: util, PrimeroEntero: rotulada}, hijos...))
}

// conFileteArriba es `mt-* border-t pt-*`: el primer hijo es el filete.
type conFileteArriba struct{ Arr, Pad float32 }

func (c *conFileteArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width, c.Arr+tema.Borde+c.Pad+m.Height)
}

func (c *conFileteArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, c.Arr))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	y := c.Arr + tema.Borde + c.Pad
	objs[1].Move(fyne.NewPos(0, y))
	objs[1].Resize(fyne.NewSize(t.Width, max(t.Height-y, 0)))
}

// botonUnirse es el enlace «Unirse» de la videollamada: no es un `Button` sino un `<a>`
// con sus clases propias, más bajo que un botón `sm`.
//
//	bg-primary inline-flex rounded-md px-2.5 py-1 text-xs font-semibold
//	text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]
type botonUnirse struct {
	ui.Boton
}

func nuevoBotonUnirse(texto string, alPulsar func()) *botonUnirse {
	b := &botonUnirse{}
	b.Texto, b.AlPulsar, b.Variante, b.Talla = texto, alPulsar, ui.Primario, ui.TallaXS
	b.ExtendBaseWidget(b)
	return b
}

// MinSize es `px-2.5 py-1` sobre la línea de `text-xs`: 21 de alto.
func (b *botonUnirse) MinSize() fyne.Size {
	return fyne.NewSize(ui.Medir(b.Texto, tema.TextoXS, ui.SemiNegrita).Width+2*espacio2p5,
		tema.TextoXS.Linea+2*tema.Espacio(1))
}
