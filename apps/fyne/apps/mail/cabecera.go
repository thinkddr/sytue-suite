// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La barra de la cabecera del lector (`<header>` de MessageReader.tsx), de la hoja:
//
//	barra   `mt-4 flex flex-wrap gap-2`: Responder, Responder a todos, Reenviar,
//	        [Darse de baja] y, con `ms-auto`, el grupo de triaje
//	grupo   `flex flex-wrap items-center gap-1`: destacar, archivar, papelera,
//	        [No es spam], «Más acciones», «Etiquetar» y «Mover a»
//	chips   `mt-3 flex flex-wrap items-center gap-1.5`: las etiquetas del hilo
//
// Todos los botones son `size="sm"` (28 de alto, `px-3`) y los glifos de 16. Por
// debajo de 768 px de VENTANA (`max-md:`) los rótulos de todos menos «Responder» pasan
// a `sr-only`: siguen siendo el nombre accesible, pero el botón se queda en su glifo.
// Con los rótulos, la barra ocupaba cuatro filas en un teléfono antes de ver el correo.
const (
	HuecoBarraLector  float32 = 2 * 0.25 * tema.Rem
	HuecoTriajeLector float32 = 1 * 0.25 * tema.Rem
	AireChipsLector   float32 = 3 * 0.25 * tema.Rem
	HuecoChipsLector  float32 = 1.5 * 0.25 * tema.Rem
)

// Los chips de etiqueta de la cabecera: `rounded-pill border bg-bg-subtle text-xs
// py-0.5 ps-2 pe-1 gap-1.5`, el punto de `h-2 w-2`, el nombre con `max-w-[12rem]` y el
// aspa de 24 con `-my-1`, que se come 3,5 por arriba y por abajo para no ensanchar la
// fila de chips.
const (
	margenIzqChipLector float32 = 2 * 0.25 * tema.Rem
	margenDerChipLector float32 = 1 * 0.25 * tema.Rem
	rellenoChipLector   float32 = 0.5 * 0.25 * tema.Rem
	puntoChipLector     float32 = 2 * 0.25 * tema.Rem
	huecoChipLector     float32 = 1.5 * 0.25 * tema.Rem
	maxNombreChip       float32 = 12 * tema.Rem
	// aspaChipLector es `h-6 w-6`: 21 con el rem de 14. El comentario de la web dice
	// 24 y el DOM mide 21 × 21 (arnés sin sesión, 2026-09-22): se copia lo medido.
	aspaChipLector float32 = 1.5 * tema.Rem
	comeAspaChip   float32 = 1 * 0.25 * tema.Rem
	tallaAspaChip  float32 = 12
)

// estrecha es `max-md:`: la VENTANA por debajo de 768 px del rem de 16.
func estrecha(o fyne.CanvasObject, respaldo float32) bool {
	return ventana(o, respaldo) < ui.Estrecho
}

// botonCabecera es un botón `sm` de la cabecera con uno de los glifos de Mail delante
// y, si `detras`, el galón de un desplegable. `soloGlifo` es el rótulo en `sr-only`.
func botonCabecera(texto, d string, variante ui.Variante, token string, lleno, soloGlifo bool, detras iconos.Icono, alPulsar func()) *ui.Boton {
	b := ui.NuevoBoton(texto, alPulsar).Con(variante, ui.TallaSM)
	b.Rotulo = texto
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if token == "" {
		token = tintaBoton(variante)
	}
	if lleno {
		b.Glifo = GlifoRelleno(d, TallaGlifoLector, token, v)
	} else {
		b.Glifo = Glifo(d, TallaGlifoLector, token, v)
	}
	b.TamIcono = TallaGlifoLector
	b.IconoDetras = detras
	if soloGlifo {
		b.Texto = ""
	}
	return b
}

// botonIconoCabecera es un `iconOnly` `sm`: cuadrado de 28 con el glifo centrado.
func botonIconoCabecera(rotulo, d, token string, lleno bool, alPulsar func()) *ui.Boton {
	b := botonCabecera(rotulo, d, ui.Fantasma, token, lleno, false, iconos.Ninguno, alPulsar)
	b.SoloIcono = true
	return b
}

// claveBarra resume todo lo que cambia la barra: si no cambia, no se rehace, y un
// botón que se rehace pierde el hover y el foco. Lo que la cambia es el estado del
// hilo y de la vista, no cada repintado.
func (x *Lector) claveBarra(estrecha bool) string {
	a, c := x.Acciones, x.Conversacion
	var b strings.Builder
	fmt.Fprintf(&b, "%v|%v|%v|%v|%v|%v|%v|%s", x.SoloLectura, estrecha, x.Baja, x.DandoDeBaja,
		x.CreandoEvento, c.Destacada, c.SinLeer, c.HiloID)
	if a != nil {
		fmt.Fprintf(&b, "|%v%v%v%v%v%v%v|%d|%d", a.Archivar != nil, a.Papelera != nil, a.Spam != nil,
			a.NoSpam != nil, a.Purgar != nil, a.CrearRegla != nil, a.Etiquetar != nil,
			len(a.Destinos), len(a.Etiquetas))
		for _, e := range EtiquetasDe(c, a.Etiquetas) {
			b.WriteString("|" + e.ID + e.Nombre + e.Color)
		}
	}
	return b.String()
}

// barra monta la fila de botones de la cabecera.
func (x *Lector) barra(estrecha bool) *fyne.Container {
	t := x.T
	objs := []fyne.CanvasObject{
		// «Responder» conserva el rótulo también en el móvil: es la acción del lector.
		botonCabecera(t.T("reader.reply", nil), ResponderD, ui.Primario, "", false, false, iconos.Ninguno,
			func() { x.responder(Responder) }),
		botonCabecera(t.T("reader.replyAll", nil), ResponderTodosD, ui.Secundario, "", false, estrecha, iconos.Ninguno,
			func() { x.responder(ResponderATodos) }),
		botonCabecera(t.T("reader.forward", nil), ReenviarD, ui.Secundario, "", false, estrecha, iconos.Ninguno,
			func() { x.responder(Reenviar) }),
	}
	if !x.SoloLectura && x.AlCrearEvento != nil {
		// Con rótulo también en el móvil: un «+» a secas junto a responder y reenviar
		// no dice qué añade. El «+» es el `Icon` de 20 por defecto, no un glifo de 16.
		evento := ui.NuevoBoton(t.T("reader.createEvent", nil), func() {
			if len(x.Conversacion.Mensajes) > 0 {
				x.AlCrearEvento(x.Conversacion.Mensajes[0])
			}
		}).Con(ui.Fantasma, ui.TallaSM)
		evento.Icono, evento.Cargando = iconos.Mas, x.CreandoEvento
		objs = append(objs, evento)
	}
	if !x.SoloLectura && x.Baja {
		baja := botonCabecera(t.T("reader.unsubscribe", nil), BajaD, ui.Fantasma, "", false, estrecha, iconos.Ninguno,
			func() {
				if x.AlDarseDeBaja != nil && len(x.Conversacion.Mensajes) > 0 {
					x.AlDarseDeBaja(x.Conversacion.Mensajes[0])
				}
			})
		baja.Cargando = x.DandoDeBaja
		objs = append(objs, baja)
	}
	if !x.SoloLectura && x.Acciones != nil {
		objs = append(objs, x.grupoTriaje(estrecha))
	}
	return container.New(flujo{hueco: HuecoBarraLector, ultimoADerecha: true}, objs...)
}

// grupoTriaje es el `<span role="group">` del triaje del hilo abierto. Destacar,
// archivar y papelera siguen a un clic —son el triaje de todos los días y tienen
// atajo—; lo menos frecuente va al menú «Más acciones», que es lo que evita que la
// cabecera se parta en tres filas en un portátil.
func (x *Lector) grupoTriaje(estrecha bool) *fyne.Container {
	t, a, c := x.T, x.Acciones, x.Conversacion
	var objs []fyne.CanvasObject
	if a.Destacar != nil {
		clave, token := "action.star", "text"
		if c.Destacada {
			clave, token = "action.unstar", "warning-text"
		}
		objs = append(objs, botonIconoCabecera(t.T(clave, nil), EstrellaD, token, c.Destacada,
			func() { a.Destacar(x.Conversacion) }))
	}
	if a.Archivar != nil {
		objs = append(objs, botonIconoCabecera(t.T("action.archive", nil), ArchivarD, "", false,
			func() { a.Archivar(x.Conversacion) }))
	}
	if a.Papelera != nil {
		objs = append(objs, botonIconoCabecera(t.T("action.trash", nil), PapeleraD, "", false,
			func() { a.Papelera(x.Conversacion) }))
	}
	if a.NoSpam != nil {
		objs = append(objs, botonCabecera(t.T("action.notSpam", nil), SpamD, ui.Fantasma, "", false, false,
			iconos.Ninguno, func() { a.NoSpam(x.Conversacion) }))
	}
	mas := ui.NuevoBoton(t.T("reader.moreActions", nil), nil).Con(ui.Fantasma, ui.TallaSM)
	mas.Rotulo, mas.SoloIcono, mas.Icono, mas.TamIcono = mas.Texto, true, iconos.Puntos, 16
	mas.AlPulsar = func() { x.menuMas().Mostrar(mas) }
	objs = append(objs, mas)
	if a.Etiquetar != nil {
		var etiquetar *ui.Boton
		etiquetar = botonCabecera(t.T("action.label", nil), EtiquetaD, ui.Fantasma, "", false, estrecha,
			iconos.ChevronAbajo, func() { x.menuEtiquetas().Mostrar(etiquetar) })
		objs = append(objs, etiquetar)
	}
	if a.Mover != nil && len(a.Destinos) > 0 {
		var mover *ui.Boton
		mover = botonCabecera(t.T("action.moveTo", nil), CarpetaD, ui.Fantasma, "", false, estrecha,
			iconos.ChevronAbajo, func() { x.menuMover().Mostrar(mover) })
		objs = append(objs, mover)
	}
	return container.New(flujo{hueco: HuecoTriajeLector}, objs...)
}

// elementoGlifo es un `MenuItem` con uno de los glifos de Mail en `icon`: el hueco del
// icono lo tiñe la web de `text-text-muted` (o `text-danger-text` en el peligroso).
func elementoGlifo(texto, d string, peligro bool, alPulsar func()) *ui.ElementoMenu {
	token := "text-muted"
	if peligro {
		token = "danger-text"
	}
	e := ui.NuevoElementoMenu(texto, iconos.Ninguno, alPulsar)
	e.Glifo = Glifo(d, TallaGlifoLector, token, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	e.TamIcono = TallaGlifoLector
	e.Peligro = peligro
	return e
}

// menuMas es «Más acciones»: marcar como no leído, crear una regla, spam y el borrado
// definitivo, en ese orden.
func (x *Lector) menuMas() *ui.Menu {
	t, a := x.T, x.Acciones
	els := []fyne.CanvasObject{
		// Es el mismo conmutador que el de la fila (`onMarkUnread={handleToggleSeen}`):
		// un hilo abierto ya está leído, así que lo deja sin leer y lo cierra.
		elementoGlifo(t.T("action.markUnread", nil), SinLeerD, false, func() {
			if a.Leido != nil {
				a.Leido(x.Conversacion)
			}
		}),
	}
	if a.CrearRegla != nil {
		els = append(els, elementoGlifo(t.T("reader.createRule", nil), CarpetaD, false,
			func() { a.CrearRegla(x.Conversacion) }))
	}
	if a.Spam != nil {
		els = append(els, elementoGlifo(t.T("action.spam", nil), SpamD, false,
			func() { a.Spam(x.Conversacion) }))
	}
	if a.Purgar != nil {
		els = append(els, elementoGlifo(t.T("action.purge", nil), PapeleraD, true,
			func() { a.Purgar(x.Conversacion) }))
	}
	m := ui.NuevoMenu(els...)
	m.Rotulo = t.T("reader.moreActions", nil)
	return m
}

// circulo es la ruta de un círculo que llena su caja con el trazo de 2 del glifo: el
// punto de color de una etiqueta dentro de un menú.
const circulo = "M12 1a11 11 0 1 0 0 22a11 11 0 1 0 0-22Z"

// menuEtiquetas es «Etiquetar la conversación»: una entrada por etiqueta con su punto,
// y el tic en las que el hilo ya lleva. Pulsar conmuta.
func (x *Lector) menuEtiquetas() *ui.Menu {
	t, a, c := x.T, x.Acciones, x.Conversacion
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	puestas := map[string]bool{}
	for _, e := range EtiquetasDe(c, a.Etiquetas) {
		puestas[e.ID] = true
	}
	var els []fyne.CanvasObject
	for _, e := range a.Etiquetas {
		on := puestas[e.ID]
		it := ui.NuevoElementoMenu(e.Nombre, iconos.Ninguno, func() { a.Etiquetar(x.Conversacion, e.ID, !on) })
		it.Glifo = iconos.TrazoDeRutaRelleno(circulo, puntoMenu, ColorEtiqueta(e.Color, v))
		it.TamIcono = puntoMenu
		it.Marcada, it.TintaTic = on, "primary-text"
		els = append(els, it)
	}
	if len(els) == 0 {
		// «No tienes etiquetas todavía.»: un párrafo, no una acción.
		els = append(els, container.New(ui.Margen{
			Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(2), Abj: tema.Espacio(2),
		}, ui.NuevoRotulo(t.T("reader.noLabelsYet", nil), tema.TextoSM, ui.Normal, "text-subtle")))
	}
	m := ui.NuevoMenu(els...)
	m.Rotulo = t.T("reader.labelConversation", nil)
	return m
}

// puntoMenu es el `h-2.5 w-2.5` del punto de una etiqueta en un menú.
const puntoMenu float32 = 2.5 * 0.25 * tema.Rem

// menuMover es «Mover a otra carpeta»: las carpetas destino con el glifo de carpeta.
func (x *Lector) menuMover() *ui.Menu {
	t, a := x.T, x.Acciones
	els := make([]fyne.CanvasObject, 0, len(a.Destinos))
	for _, c := range a.Destinos {
		els = append(els, elementoGlifo(RotuloCarpeta(t, c.Nombre, c.Rol), CarpetaD, false,
			func() { a.Mover(x.Conversacion, c) }))
	}
	m := ui.NuevoMenu(els...)
	m.Rotulo = t.T("reader.moveToFolder", nil)
	return m
}

// chips monta la fila de etiquetas del hilo. Nil si no lleva ninguna.
func (x *Lector) chips() *fyne.Container {
	if x.Acciones == nil {
		return nil
	}
	var objs []fyne.CanvasObject
	for _, e := range EtiquetasDe(x.Conversacion, x.Acciones.Etiquetas) {
		ch := nuevoChipLector(x.T, e, !x.SoloLectura && x.Acciones.Etiquetar != nil)
		ch.AlQuitar = func() { x.Acciones.Etiquetar(x.Conversacion, e.ID, false) }
		objs = append(objs, ch)
	}
	if len(objs) == 0 {
		return nil
	}
	return container.New(flujo{hueco: HuecoChipsLector}, objs...)
}

// chipLector es una etiqueta aplicada, en la cabecera del lector.
type chipLector struct {
	widget.BaseWidget
	Etiqueta Etiqueta
	AlQuitar func()

	aspa *ui.BotonIcono // nil en solo lectura: no hay nada que quitar
}

func nuevoChipLector(t *i18n.Translator, e Etiqueta, quitable bool) *chipLector {
	c := &chipLector{Etiqueta: e}
	if quitable {
		rot := t.T("reader.removeLabel", i18n.Vars{"label": e.Nombre})
		c.aspa = ui.NuevoBotonIcono(iconos.Cerrar, rot, func() {
			if c.AlQuitar != nil {
				c.AlQuitar()
			}
		})
		// `hover:text-danger-text` sobre el `text-text-muted` del chip, sin velo, y la
		// esquina redonda entera (`rounded-full`).
		c.aspa.Lado, c.aspa.TamIcono, c.aspa.Radio = aspaChipLector, tallaAspaChip, tema.Forma("radius-control")
		c.aspa.Trazo, c.aspa.TrazoEncima, c.aspa.FondoEncima = "text-muted", "danger-text", ""
	}
	c.ExtendBaseWidget(c)
	return c
}

// MedidaChipLector es lo que ocupa un chip: el nombre con su tope, el punto, los
// huecos y, si se puede quitar, el aspa, que manda en el alto aunque se coma su
// `-my-1`.
func MedidaChipLector(nombre string, quitable bool) fyne.Size {
	ancho := 2*tema.Borde + margenIzqChipLector + margenDerChipLector + puntoChipLector + huecoChipLector +
		min(ui.Medir(nombre, tema.TextoXS, ui.Normal).Width, maxNombreChip)
	fila := tema.TextoXS.Linea
	if quitable {
		ancho += huecoChipLector + aspaChipLector
		fila = max(fila, aspaChipLector-2*comeAspaChip)
	}
	return fyne.NewSize(ancho, fila+2*rellenoChipLector+2*tema.Borde)
}

func (c *chipLector) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &chipLectorRenderer{
		c:      c,
		caja:   ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioPill),
		punto:  canvas.NewCircle(color.NRGBA{}),
		nombre: ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
	}
	r.objetos = append(r.caja.Objetos(), r.punto, r.nombre)
	if c.aspa != nil {
		r.objetos = append(r.objetos, c.aspa)
	}
	r.Refresh()
	return r
}

type chipLectorRenderer struct {
	c       *chipLector
	caja    *ui.Caja
	punto   *canvas.Circle
	nombre  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *chipLectorRenderer) MinSize() fyne.Size {
	return MedidaChipLector(r.c.Etiqueta.Nombre, r.c.aspa != nil)
}
func (r *chipLectorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *chipLectorRenderer) Destroy()                     {}

func (r *chipLectorRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	x := tema.Borde + margenIzqChipLector
	r.punto.Move(fyne.NewPos(x, (t.Height-puntoChipLector)/2))
	r.punto.Resize(fyne.NewSize(puntoChipLector, puntoChipLector))
	x += puntoChipLector + huecoChipLector
	w := ui.Truncar(r.nombre, r.c.Etiqueta.Nombre, tema.TextoXS, ui.Normal, maxNombreChip)
	ui.ColocarTexto(r.nombre, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
	if a := r.c.aspa; a != nil {
		x += w + huecoChipLector
		a.Move(fyne.NewPos(x, (t.Height-aspaChipLector)/2))
		a.Resize(fyne.NewSize(aspaChipLector, aspaChipLector))
	}
}

func (r *chipLectorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("bg-subtle", v), tema.Color("border", v))
	r.punto.FillColor = ColorEtiqueta(r.c.Etiqueta.Color, v)
	r.nombre.Color = tema.Color("text-muted", v)
	for _, o := range r.objetos {
		o.Refresh()
	}
	if t := r.c.Size(); t.Width > 0 {
		r.Layout(t)
	}
}
