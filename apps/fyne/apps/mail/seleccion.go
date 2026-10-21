// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La multiselección y la barra de lote (MessageList.tsx + BatchActionBar.tsx)
//
// NO es `ui.BarraDeLote`: esa es `BulkActionBar.tsx` de la suite, con rótulos y
// `flex-wrap`. La de Mail es propia en la web y se pinta distinta —«seleccionar
// todo» a la izquierda, iconos densos sin rótulo, un filete vertical y el aspa al
// final—, así que aquí también es propia. La paridad manda sobre la reutilización.
//
// De la hoja: la franja es `flex items-center gap-2 border-b bg-bg-subtle px-4 py-1.5`
// con la casilla de `h-4 w-4` y la barra `flex flex-1 items-center gap-0.5`: la cuenta
// en `text-sm font-medium` con su `mr-1`, el filete `mx-1 h-5 w-px`, los botones `sm`
// fantasma de solo icono (28) con el glifo de 16, un hueco que empuja y el aspa.
const (
	MargenBarraSeleccion  float32 = 4 * 0.25 * tema.Rem
	RellenoBarraSeleccion float32 = 1.5 * 0.25 * tema.Rem
	HuecoBarraSeleccion   float32 = 0.5 * 0.25 * tema.Rem
	margenCuentaLote      float32 = 1 * 0.25 * tema.Rem
	altoFileteLote        float32 = 5 * 0.25 * tema.Rem
)

// AltoBarraSeleccion es lo que ocupa la franja: su `py-1.5`, el botón `sm` (que manda
// sobre la casilla) y el filete de abajo.
var AltoBarraSeleccion = 2*RellenoBarraSeleccion + ui.MedidaBoton("", ui.TallaSM, 1).Height + tema.Borde

// Lote son las acciones sobre la selección, con el mismo contrato que Acciones: nil
// esconde el botón.
type Lote struct {
	Leer, NoLeer                     func()
	Archivar, Papelera, Spam, Purgar func()
	Etiquetar                        func(etiqueta string)
	Etiquetas                        []Etiqueta
	// Todas es la casilla de «seleccionar todo» y Quitar, el aspa.
	Todas, Quitar func()
}

// BarraSeleccion es la franja que aparece al marcar alguna conversación.
type BarraSeleccion struct {
	widget.BaseWidget
	T *i18n.Translator
	// Cuantas son las marcadas de entre las visibles, y Todas si lo están todas.
	Cuantas int
	Todas   bool
	Lote    *Lote

	clave string
}

var _ fyne.Widget = (*BarraSeleccion)(nil)

func nuevaBarraSeleccion(t *i18n.Translator, l *Lote) *BarraSeleccion {
	b := &BarraSeleccion{T: t, Lote: l}
	b.ExtendBaseWidget(b)
	return b
}

// AccessibilityLabel es el `aria-label` de la `Toolbar`.
func (b *BarraSeleccion) AccessibilityLabel() string { return b.T.T("batch.toolbar", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (b *BarraSeleccion) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

func (b *BarraSeleccion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &barraSeleccionRenderer{
		b:       b,
		fondo:   canvas.NewRectangle(tema.Color("bg-subtle", v)),
		filete:  canvas.NewRectangle(tema.Color("border", v)),
		divisor: canvas.NewRectangle(tema.Color("border", v)),
		cuenta:  ui.Texto("", tema.TextoSM, ui.Medio, tema.Color("text", v)),
	}
	r.casilla = ui.NuevaCasilla(b.T.T("list.selectAll", nil), func(bool) {
		if b.Lote.Todas != nil {
			b.Lote.Todas()
		}
	})
	r.casilla.SinEtiqueta = true
	r.Refresh()
	return r
}

type barraSeleccionRenderer struct {
	b                      *BarraSeleccion
	fondo, filete, divisor *canvas.Rectangle
	cuenta                 *canvas.Text
	casilla                *ui.Eleccion
	acciones               []*ui.Boton
	tira                   *ui.TiraDeslizable
	cerrar                 *ui.Boton
	objetos                []fyne.CanvasObject
}

func (r *barraSeleccionRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoBarraSeleccion)
}
func (r *barraSeleccionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraSeleccionRenderer) Destroy()                     {}

// Layout reparte de izquierda a derecha, y el aspa a la derecha del todo: el
// `<span class="flex-1" />` se lleva lo que sobre.
//
// Si no cabe todo, el aspa se queda en su sitio y los botones van en una tira que se
// desplaza de lado sin barra (con seis acciones en 280 no cabe «Etiquetar»): se llega
// a ellos con la rueda, el gesto o el tabulador, y no se esconde ninguno.
func (r *barraSeleccionRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	contenido := t.Height - tema.Borde
	centro := func(alto float32) float32 { return (contenido - alto) / 2 }
	lado := ui.MedidaBoton("", ui.TallaSM, 1).Height

	x := MargenBarraSeleccion
	r.casilla.Move(fyne.NewPos(x, centro(ui.LadoEleccion)))
	r.casilla.Resize(fyne.NewSize(ui.LadoEleccion, ui.LadoEleccion))
	x += ui.LadoEleccion + ui.HuecoEleccion

	ui.ColocarTexto(r.cuenta, tema.TextoSM, x, centro(tema.TextoSM.Linea))
	x += ui.Medir(r.cuenta.Text, tema.TextoSM, ui.Medio).Width + margenCuentaLote + HuecoBarraSeleccion
	x += margenCuentaLote
	r.divisor.Move(fyne.NewPos(x, centro(altoFileteLote)))
	r.divisor.Resize(fyne.NewSize(tema.Borde, altoFileteLote))
	x += tema.Borde + margenCuentaLote + HuecoBarraSeleccion

	fin := t.Width - MargenBarraSeleccion
	r.cerrar.Move(fyne.NewPos(fin-lado, centro(lado)))
	r.cerrar.Resize(fyne.NewSize(lado, lado))
	tope := fin - lado - HuecoBarraSeleccion
	r.tira.Move(fyne.NewPos(x, centro(lado)))
	r.tira.Resize(fyne.NewSize(max(min(tope-x, r.tira.AnchoContenido()), 0), lado))
}

// claveLote dice qué botones hay: si no cambia, no se rehacen.
func claveLote(l *Lote) string {
	return fmt.Sprintf("%v%v%v%v%v%v%d", l.Leer != nil, l.NoLeer != nil, l.Archivar != nil, l.Papelera != nil,
		l.Spam != nil, l.Purgar != nil, len(l.Etiquetas))
}

// botonLote es un `BarAction`: fantasma, `sm`, solo icono y el glifo de 16.
func botonLote(rotulo, d string, alPulsar func()) *ui.Boton {
	b := ui.NuevoBoton(rotulo, alPulsar).Con(ui.Fantasma, ui.TallaSM)
	b.Rotulo, b.SoloIcono, b.TamIcono = rotulo, true, TallaGlifoLector
	b.Glifo = Glifo(d, TallaGlifoLector, "text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	return b
}

// montar rehace los botones en el orden de BatchActionBar.tsx.
func (r *barraSeleccionRenderer) montar() {
	t, l := r.b.T, r.b.Lote
	r.acciones = r.acciones[:0]
	// El manejador se lee AL PULSAR y no al montar: el Buzón rehace el Lote en cada
	// vista y los botones solo se rehacen cuando cambia cuáles hay.
	poner := func(h func(*Lote) func(), clave, d string) {
		if h(l) != nil {
			r.acciones = append(r.acciones, botonLote(t.T(clave, nil), d, func() {
				if f := h(r.b.Lote); f != nil {
					f()
				}
			}))
		}
	}
	poner(func(l *Lote) func() { return l.Leer }, "action.markRead", LeidoD)
	poner(func(l *Lote) func() { return l.NoLeer }, "action.markUnread", SinLeerD)
	poner(func(l *Lote) func() { return l.Archivar }, "action.archive", ArchivarD)
	poner(func(l *Lote) func() { return l.Papelera }, "action.trash", PapeleraD)
	poner(func(l *Lote) func() { return l.Spam }, "action.spam", SpamD)
	// En la Papelera «mover a la papelera» no dice nada: lo que hace falta es borrar.
	poner(func(l *Lote) func() { return l.Purgar }, "action.purge", PapeleraD)
	if len(l.Etiquetas) > 0 && l.Etiquetar != nil {
		var et *ui.Boton
		et = botonLote(t.T("batch.labelSelection", nil), EtiquetaD, func() { r.menuEtiquetas().Mostrar(et) })
		r.acciones = append(r.acciones, et)
	}
	r.cerrar = ui.NuevoBoton(t.T("batch.clear", nil), func() {
		if q := r.b.Lote.Quitar; q != nil {
			q()
		}
	}).Con(ui.Fantasma, ui.TallaSM)
	r.cerrar.Rotulo, r.cerrar.SoloIcono, r.cerrar.Icono, r.cerrar.TamIcono = r.cerrar.Texto, true, iconos.Cerrar, 16

	botones := make([]fyne.CanvasObject, 0, len(r.acciones))
	for _, b := range r.acciones {
		botones = append(botones, b)
	}
	r.tira = ui.NuevaTiraDeslizable(ui.MedidaBoton("", ui.TallaSM, 1).Height, HuecoBarraSeleccion, botones...)
	r.objetos = []fyne.CanvasObject{r.fondo, r.filete, r.casilla, r.cuenta, r.divisor, r.tira, r.cerrar}
}

// menuEtiquetas es «Etiquetar la selección»: AÑADE la etiqueta a todo, que es el caso
// común de la barra (quitar en masa es raro, y la web no lo ofrece aquí).
func (r *barraSeleccionRenderer) menuEtiquetas() *ui.Menu {
	l := r.b.Lote
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	els := make([]fyne.CanvasObject, 0, len(l.Etiquetas))
	for _, e := range l.Etiquetas {
		it := ui.NuevoElementoMenu(e.Nombre, iconos.Ninguno, func() { l.Etiquetar(e.ID) })
		it.Glifo = iconos.TrazoDeRutaRelleno(circulo, puntoMenu, ColorEtiqueta(e.Color, v))
		it.TamIcono = puntoMenu
		els = append(els, it)
	}
	m := ui.NuevoMenu(els...)
	m.Rotulo = r.b.T.T("batch.labelSelection", nil)
	return m
}

func (r *barraSeleccionRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if k := claveLote(b.Lote); k != b.clave || r.objetos == nil {
		b.clave = k
		r.montar()
	}
	r.fondo.FillColor = tema.Color("bg-subtle", v)
	r.filete.FillColor = tema.Color("border", v)
	r.divisor.FillColor = tema.Color("border", v)
	r.cuenta.Text = b.T.T("batch.selected", i18n.Vars{"count": b.Cuantas})
	r.cuenta.Color = tema.Color("text", v)
	r.casilla.Marcada, r.casilla.Parcial = b.Todas, b.Cuantas > 0 && !b.Todas
	for _, o := range r.objetos {
		o.Refresh()
	}
	if t := b.Size(); t.Width > 0 {
		r.Layout(t)
	}
}

// ---- El estado de la selección, en el Buzón ----

// teclaMayus dice si Mayúsculas está pulsada: el rango de Mayús + clic. Fyne no la
// pasa en el `PointEvent` del clic; la sabe el driver de escritorio.
func teclaMayus() bool {
	d, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	return ok && d.CurrentKeyModifiers()&fyne.KeyModifierShift != 0
}

// marcarFila es `toggleSelect`: conmuta una conversación y fija el ancla; con
// Mayúsculas, AÑADE el rango visible desde el ancla hasta esta.
func (b *Buzon) marcarFila(c Conversacion, i int) {
	m := b.P.Bandeja.Marcadas
	convs := b.P.Bandeja.Conversaciones
	switch {
	case teclaMayus() && b.ancla >= 0:
		lo, hi := min(b.ancla, i), max(b.ancla, i)
		for j := lo; j <= hi && j < len(convs); j++ {
			m[convs[j].HiloID] = true
		}
	case m[c.HiloID]:
		delete(m, c.HiloID)
	default:
		m[c.HiloID] = true
	}
	if !teclaMayus() {
		b.ancla = i
	}
	b.pintarSeleccion()
}

// marcarTodas es `toggleSelectAll`: si están todas, ninguna; si no, todas las
// VISIBLES.
func (b *Buzon) marcarTodas() {
	convs := b.P.Bandeja.Conversaciones
	todas := len(convs) > 0 && b.P.Bandeja.CuantasMarcadas() == len(convs)
	b.P.Bandeja.Marcadas = map[string]bool{}
	if !todas {
		for _, c := range convs {
			b.P.Bandeja.Marcadas[c.HiloID] = true
		}
	}
	b.ancla = -1
	b.pintarSeleccion()
}

// quitarSeleccion es `clearSelection`.
func (b *Buzon) quitarSeleccion() {
	b.P.Bandeja.Marcadas = map[string]bool{}
	b.ancla = -1
	b.pintarSeleccion()
}

func (b *Buzon) pintarSeleccion() {
	b.P.Bandeja.Refresh()
}

// seleccionadas son las conversaciones marcadas de entre las visibles.
func (b *Buzon) seleccionadas() []Conversacion {
	var out []Conversacion
	for _, c := range b.P.Bandeja.Conversaciones {
		if b.P.Bandeja.Marcadas[c.HiloID] {
			out = append(out, c)
		}
	}
	return out
}

// ponerLote monta las acciones de la barra con las mismas reglas que el triaje de
// una conversación: cada una reusa el camino en lote del triaje, aplicado a todos los
// mensajes de todas las elegidas.
func (b *Buzon) ponerLote() {
	l := b.P.Bandeja.Lote
	a := b.P.Acciones
	*l = Lote{Todas: b.marcarTodas, Quitar: b.quitarSeleccion}
	if b.P.Bandeja.SoloLectura {
		return
	}
	l.Etiquetas = a.Etiquetas
	l.Leer = func() { b.marcarConvs(b.seleccionadas(), api.MailFlagSeen, true, "error.seen"); b.quitarSeleccion() }
	l.NoLeer = func() { b.marcarConvs(b.seleccionadas(), api.MailFlagSeen, false, "error.seen"); b.quitarSeleccion() }
	destino := func(h func(Conversacion), rol string) func() {
		if h == nil {
			return nil
		}
		c, _ := b.carpetaDeRol(rol)
		return func() { b.moverConvs(b.seleccionadas(), c) }
	}
	l.Archivar = destino(a.Archivar, "archive")
	l.Papelera = destino(a.Papelera, "trash")
	l.Spam = destino(a.Spam, "junk")
	if a.Purgar != nil {
		l.Purgar = func() { b.pedirPurga(b.seleccionadas()) }
	}
	l.Etiquetar = b.etiquetarSeleccion
}

// etiquetarSeleccion es `onBatchLabel`: añade la etiqueta a toda la selección, suelta
// la selección y refresca los contadores. Si el servidor dice que no, se avisa.
func (b *Buzon) etiquetarSeleccion(etiqueta string) {
	cs := b.seleccionadas()
	ids := idsDe(cs)
	if len(ids) == 0 {
		return
	}
	b.marcar(ids, etiqueta, true)
	b.quitarSeleccion()
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, b.S.ApplyLabel(ctx, etiqueta, ids, true)
	}, func(_ struct{}, err error) {
		if err != nil {
			b.avisar(ui.AvisoPeligro, "error.label", nil)
			return
		}
		b.cargarEtiquetas()
	})
}

// botonVaciar es «Vaciar carpeta» de la cabecera de la lista: un `Button` fantasma
// `sm` cuyo único hijo es un `<span class="text-danger-text inline-flex gap-1.5">` con
// la papelera de 14 y el rótulo. El botón de `ui` pone el rótulo en `text` con `gap-2`,
// así que aquí se pinta el botón SIN rótulo —su caja, su hover y su foco— y el
// contenido va encima, en primitivas que no se llevan el clic.
type botonVaciar struct {
	widget.BaseWidget
	boton  *ui.Boton
	texto  string
	glifo  *iconos.Dibujo
	rotulo *canvas.Text
}

func nuevoBotonVaciar(texto string, alPulsar func()) *botonVaciar {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := &botonVaciar{
		texto:  texto,
		glifo:  Glifo(PapeleraD, tallaGlifoVaciar, "danger-text", v),
		rotulo: ui.Texto(texto, tema.TextoSM, ui.PesoRotulo, tema.Color("danger-text", v)),
	}
	b.boton = ui.NuevoBoton("", alPulsar).Con(ui.Fantasma, ui.TallaSM)
	b.boton.Rotulo = texto
	b.boton.Ancho = MedidaBotonVaciar(texto).Width
	b.ExtendBaseWidget(b)
	return b
}

const (
	tallaGlifoVaciar float32 = 14
	huecoVaciar      float32 = 1.5 * 0.25 * tema.Rem
)

// MedidaBotonVaciar es el `px-3` y el contenido: glifo, `gap-1.5` y rótulo.
func MedidaBotonVaciar(texto string) fyne.Size {
	ancho := 2*tema.Espacio(3) + tallaGlifoVaciar + huecoVaciar + ui.Medir(texto, tema.TextoSM, ui.PesoRotulo).Width
	return fyne.NewSize(ancho, ui.MedidaBoton("", ui.TallaSM, 1).Height)
}

func (b *botonVaciar) CreateRenderer() fyne.WidgetRenderer {
	return &botonVaciarRenderer{b: b, objetos: []fyne.CanvasObject{b.boton, b.glifo.Objeto(), b.rotulo}}
}

type botonVaciarRenderer struct {
	b       *botonVaciar
	objetos []fyne.CanvasObject
}

func (r *botonVaciarRenderer) MinSize() fyne.Size           { return MedidaBotonVaciar(r.b.texto) }
func (r *botonVaciarRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonVaciarRenderer) Destroy()                     {}
func (r *botonVaciarRenderer) Layout(t fyne.Size) {
	r.b.boton.Resize(t)
	x := tema.Espacio(3)
	r.b.glifo.Colocar(x, (t.Height-tallaGlifoVaciar)/2, tallaGlifoVaciar)
	ui.ColocarTexto(r.b.rotulo, tema.TextoSM, x+tallaGlifoVaciar+huecoVaciar, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *botonVaciarRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.b.glifo.Tenir("danger-text", v)
	r.b.rotulo.Color = tema.Color("danger-text", v)
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- Vaciar carpeta ----

// pedirVaciado abre la confirmación de «Vaciar carpeta», que dice cuántos van a caer y
// de dónde.
func (b *Buzon) pedirVaciado() {
	lienzo := b.P.Lienzo()
	c, hay := b.carpetaActual()
	if lienzo == nil || !hay {
		return
	}
	t := b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(t.T("purge.emptyFolderBody", i18n.Vars{"count": c.Total, "name": b.P.Bandeja.Titulo}),
		tema.TextoSM, ui.Normal, "text", v).A(AnchoCuerpoDialogo(ui.DialogoSM, lienzo.Size().Width))
	var d *ui.Dialogo
	var boton *ui.Boton
	d = ui.Confirmacion(t.T("purge.emptyFolder", nil), t.T("common.cancel", nil), t.T("purge.emptyFolder", nil), true,
		cuerpo, func() {
			if boton == nil || !boton.Cargando {
				d.Cerrar()
			}
		}, func() { b.vaciar(c, d, boton) })
	d.RotuloCerrar = t.T("ui.close", nil)
	boton = botonConfirmar(d)
	d.Mostrar(lienzo)
}

// carpetaActual es la carpeta de la vista, con sus números.
func (b *Buzon) carpetaActual() (Carpeta, bool) {
	id := b.carpetaVista()
	for _, m := range b.carpetas {
		if m.ID == id && id != "" {
			return carpetaDe(m), true
		}
	}
	return Carpeta{}, false
}

// vaciar es `vaciarCarpeta`: página a página, sin cursor —cada pasada pide la primera
// página, que ya no es la misma—, hasta que no quede nada o no se avance.
func (b *Buzon) vaciar(c Carpeta, d *ui.Dialogo, boton *ui.Boton) {
	if boton != nil {
		boton.Cargando = true
		boton.Refresh()
	}
	pedir(b, func(ctx context.Context) (int, error) {
		borrados := 0
		for range 200 {
			pagina, _, err := b.S.Messages(ctx, c.ID, "", 100)
			if err != nil {
				return borrados, err
			}
			if len(pagina) == 0 {
				return borrados, nil
			}
			antes := borrados
			for _, m := range pagina {
				if err := b.S.Delete(ctx, m.ID); err != nil && !noEsta(err) {
					return borrados, err
				}
				borrados++
			}
			if borrados == antes {
				return borrados, errSinAvance
			}
		}
		return borrados, nil
	}, func(borrados int, err error) {
		if d != nil {
			d.Cerrar()
		}
		b.cargarLista()
		b.refrescarContadores()
		b.cargarCuota()
		if err != nil {
			b.avisar(ui.AvisoPeligro, "purge.emptyFailed", nil)
			return
		}
		b.avisar(ui.AvisoExito, "purge.emptied", i18n.Vars{"count": borrados})
	})
}

var errSinAvance = errors.New("vaciar: sin avance")

// noEsta es un 404: el mensaje ya no existe, que es lo que se pedía.
func noEsta(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// botonVaciarVisible dice si la cabecera ofrece «Vaciar carpeta»: en Papelera y Spam,
// fuera de la búsqueda y con algo dentro.
func (b *Buzon) botonVaciarVisible() bool {
	c, hay := b.carpetaActual()
	rol := strings.ToLower(c.Rol)
	return hay && !b.P.Bandeja.SoloLectura && (rol == "trash" || rol == "junk") && c.Total > 0
}
