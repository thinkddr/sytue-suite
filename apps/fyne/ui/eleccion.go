// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// LadoEleccion es el `h-4 w-4` de la casilla y del radio: 1rem con el rem de 14.
var LadoEleccion = tema.Espacio(4)

// HuecoEleccion es el `gap-2` entre el control y su etiqueta.
var HuecoEleccion = tema.Espacio(2)

// MedidaEleccion es lo que ocupa una casilla o un radio con su etiqueta: el cuadradito,
// el hueco y el rótulo, por la caja de línea de `text-base`.
//
// El alto NO es el del cuadradito: el `<label>` es un `inline-flex items-center
// text-base`, así que la fila mide lo que la caja de línea de su texto —19,25— y el
// cuadradito de 14 va centrado dentro. Medir 14 aquí deja las filas de un grupo de
// casillas más juntas que en la web.
func MedidaEleccion(etiqueta string) fyne.Size { return MedidaEleccionEn(etiqueta, tema.TextoBase) }

// MedidaEleccionEn es MedidaEleccion con la etiqueta en otro escalón.
func MedidaEleccionEn(etiqueta string, esc tema.Escalon) fyne.Size {
	ancho := LadoEleccion
	if etiqueta != "" {
		ancho += HuecoEleccion + Medir(etiqueta, esc, Normal).Width
	}
	return fyne.NewSize(ancho, max(LadoEleccion, esc.Linea))
}

// Eleccion es `Checkbox` y `Radio` de Form.tsx: el cuadradito de `h-4 w-4` con su
// etiqueta al lado, y se pulsa en cualquiera de los dos.
//
// El estado es de quien lo pinta, como en la web: al recibir AlCambiar, quien lo usa
// mueve su propio booleano —o elige esa fila, si es un radio—.
type Eleccion struct {
	widget.BaseWidget
	Etiqueta string
	Marcada  bool
	// Redondo es el radio de «una de varias». Sin él es una casilla.
	Redondo bool
	// Parcial es el tercer estado de una casilla: el `indeterminate` del DOM, que la
	// web pone por `ref` porque no es un atributo. Lo usa todo «seleccionar todo» con
	// una selección a medias —el explorador de Sytue Drive, la bandeja de Mail, la
	// lista de Contactos— y se pinta como el navegador: la caja sólida del acento con
	// una RAYA en vez del tic.
	//
	// Gana a Marcada, igual que en el DOM: un `<input>` con `indeterminate = true`
	// enseña la raya esté marcado o no. Al pulsar NO se consulta: quien lo use decide
	// si el clic marca todo o desmarca todo, que es una regla de la lista y no del
	// control.
	Parcial bool
	// SinEtiqueta pinta el control SOLO: la etiqueta se anuncia pero no se escribe. Es
	// el de marcar una fila de una lista, donde el texto ya está en la fila.
	SinEtiqueta bool
	// AnchoEtiqueta limita la etiqueta y permite repartirla en líneas. Cero conserva
	// la medida natural de las casillas existentes. Lo usa un formulario estrecho.
	AnchoEtiqueta float32
	// Escalon es el de la etiqueta. Cero es text-base; los radios de Rooms usan text-sm.
	Escalon   tema.Escalon
	AlCambiar func(bool)

	inerte, enfocado, encima bool
}

var (
	_ fyne.Widget       = (*Eleccion)(nil)
	_ fyne.Tappable     = (*Eleccion)(nil)
	_ fyne.Focusable    = (*Eleccion)(nil)
	_ fyne.Accessible   = (*Eleccion)(nil)
	_ fyne.Disableable  = (*Eleccion)(nil)
	_ desktop.Hoverable = (*Eleccion)(nil)
)

// NuevaCasilla crea una casilla de verificación.
func NuevaCasilla(etiqueta string, alCambiar func(bool)) *Eleccion {
	e := &Eleccion{Etiqueta: etiqueta, AlCambiar: alCambiar}
	e.ExtendBaseWidget(e)
	return e
}

// NuevoRadio crea un radio: mismo control, redondo y para «una de varias».
func NuevoRadio(etiqueta string, alCambiar func(bool)) *Eleccion {
	e := NuevaCasilla(etiqueta, alCambiar)
	e.Redondo = true
	return e
}

// MinSize es la medida declarada.
func (e *Eleccion) MinSize() fyne.Size {
	e.ExtendBaseWidget(e)
	if e.SinEtiqueta {
		return MedidaEleccion("")
	}
	if e.AnchoEtiqueta > 0 {
		w := min(Medir(e.Etiqueta, e.escalon(), Normal).Width, e.AnchoEtiqueta)
		ancho := LadoEleccion + HuecoEleccion + w
		return fyne.NewSize(ancho, e.AltoPara(ancho))
	}
	m := MedidaEleccionEn(e.Etiqueta, e.escalon())
	// Con el dedo, la etiqueta es el blanco: 44 de alto, como en Form.tsx.
	m.Height = sueloTactil(m.Height)
	return m
}

// AltoPara conserva el reparto optativo de AnchoEtiqueta al ancho asignado,
// también cuando un formulario abierto cambia de tamaño.
func (e *Eleccion) AltoPara(ancho float32) float32 {
	if e.SinEtiqueta {
		return MedidaEleccion("").Height
	}
	return sueloTactil(e.altoEtiqueta(ancho))
}

// altoEtiqueta es el alto de la etiqueta repartida a ese ancho.
func (e *Eleccion) altoEtiqueta(ancho float32) float32 {
	if e.SinEtiqueta {
		return MedidaEleccion("").Height
	}
	if e.AnchoEtiqueta <= 0 {
		return MedidaEleccionEn(e.Etiqueta, e.escalon()).Height
	}
	w := max(min(e.AnchoEtiqueta, ancho-LadoEleccion-HuecoEleccion), 1)
	return max(LadoEleccion, float32(len(Repartir(e.Etiqueta, e.escalon(), Normal, w)))*e.escalon().Linea)
}

// escalon es el de la etiqueta: el pedido, o `text-base`.
func (e *Eleccion) escalon() tema.Escalon {
	if e.Escalon.Tam == 0 {
		return tema.TextoBase
	}
	return e.Escalon
}

// Tapped marca o desmarca. Un radio ya marcado NO se desmarca al pulsarlo, igual que
// `<input type="radio">`: para eso está el grupo.
func (e *Eleccion) Tapped(*fyne.PointEvent) {
	if e.inerte {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(e); c != nil {
		enfocar(c, e)
	}
	if e.Redondo && e.Marcada {
		return
	}
	e.Marcada = !e.Marcada
	e.Refresh()
	if e.AlCambiar != nil {
		e.AlCambiar(e.Marcada)
	}
}

// Disable y Enable son `disabled`: al 50 % de opacidad y sin responder.
func (e *Eleccion) Disable() { e.inerte = true; e.Refresh() }

// Enable lo devuelve a la vida.
func (e *Eleccion) Enable() { e.inerte = false; e.Refresh() }

// Disabled dice si está inerte.
func (e *Eleccion) Disabled() bool { return e.inerte }

// FocusGained enciende el aro.
func (e *Eleccion) FocusGained() { e.enfocado = true; e.Refresh() }

// FocusLost lo apaga.
func (e *Eleccion) FocusLost() { e.enfocado = false; e.Refresh() }

// TypedRune marca con la barra espaciadora, como el control nativo.
func (e *Eleccion) TypedRune(r rune) {
	if r == ' ' {
		e.Tapped(nil)
	}
}

// TypedKey no atiende ninguna otra tecla: el recorrido de un grupo de radios con las
// flechas lo lleva quien monta el grupo, que es el único que sabe cuáles son hermanos.
func (e *Eleccion) TypedKey(k *fyne.KeyEvent) { SubirEscape(e, k) }

// AccessibilityLabel es el rótulo que anuncia el lector de pantalla. Con SinEtiqueta el
// texto no se escribe pero SÍ se anuncia: por eso el campo sigue estando.
func (e *Eleccion) AccessibilityLabel() string { return e.Etiqueta }

// AccessibilityRole es «button».
//
// Fyne 2.8 solo conoce cuatro roles —button, container, link y text—, así que no hay
// forma de distinguir una casilla de un radio para el lector de pantalla, ni de
// anunciar si está marcada. Es un techo del toolkit: se cierra el día que Fyne amplíe
// `AccessibleRole`, y hasta entonces el rótulo es lo único que llega.
func (e *Eleccion) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// CreateRenderer compone el cuadradito, su marca y la etiqueta.
func (e *Eleccion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &eleccionRenderer{
		e: e,
		// El aro sigue la FORMA de la pieza: una esquina de 2 + 2 en la casilla y un círculo
		// de 18 en el radio, como el `<span class="sytue-focus-ring-choice">` que lo lleva en
		// la web (Form.tsx). Va relleno y detrás: la pieza es opaca y le tapa el centro, así
		// que por dentro no asoma nada, sea cual sea el fondo de debajo.
		aro:   NuevoAro(radioPieza(LadoEleccion, e.Redondo), SinHueco, v),
		pieza: NuevaPiezaCasilla(LadoEleccion, e.Redondo, false, v),
	}
	r.objetos = r.aro.Antes(r.pieza.Objetos()...)
	if !e.SinEtiqueta {
		r.texto = Texto(e.Etiqueta, e.escalon(), Normal, atenuar(tema.Color("text", v), e.inerte))
		r.parrafo = NuevoParrafo(e.Etiqueta, e.escalon(), Normal, "text", v)
		r.objetos = append(r.objetos, r.texto, r.parrafo)
	}
	r.Refresh()
	return r
}

// MouseIn y MouseOut son el `:hover` del `<input>`, que en la web también se enciende al pasar
// por la ETIQUETA: un `<label>` con `:hover` se lo pasa a su control.
func (e *Eleccion) MouseIn(*desktop.MouseEvent) { e.encima = true; e.Refresh() }

// MouseMoved no cambia nada.
func (e *Eleccion) MouseMoved(*desktop.MouseEvent) {}

// MouseOut apaga el `:hover`.
func (e *Eleccion) MouseOut() { e.encima = false; e.Refresh() }

// MoverCaja pone una Caja ya dimensionada en otra posición.
//
// `Caja.Redimensionar` la deja siempre en el origen, que es lo que necesita un control
// cuya caja ocupa el widget entero. Una casilla no: su cuadradito va CENTRADO en la
// caja de línea de la etiqueta, así que hay que moverla. Se recolocan los dos
// rectángulos desde cero en cada llamada —y no con un desplazamiento relativo—, porque
// Layout se ejecuta en cada redimensión y un incremento se acumularía.
func MoverCaja(c *Caja, p fyne.Position, tam fyne.Size) {
	c.Redimensionar(tam)
	objs := c.Objetos()
	c.fuera.Move(p)
	if len(objs) == 2 {
		c.dentro.Move(fyne.NewPos(p.X+tema.Borde, p.Y+tema.Borde))
		return
	}
	c.dentro.Move(p)
}

// eleccionRenderer lleva la pieza de Chromium (PiezaCasilla), que tiene SIEMPRE sus tres
// marcas —el tic, la raya y el punto— y enseña la que toca: meterlas y sacarlas de
// `Objects()` según el estado deja un objeto sin colocar el primer fotograma.
type eleccionRenderer struct {
	e       *Eleccion
	aro     *Aro
	pieza   *PiezaCasilla
	texto   *canvas.Text
	parrafo *Parrafo
	objetos []fyne.CanvasObject
}

func (r *eleccionRenderer) MinSize() fyne.Size           { return r.e.MinSize() }
func (r *eleccionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *eleccionRenderer) Destroy()                     {}

func (r *eleccionRenderer) Layout(t fyne.Size) {
	// `items-center`: el cuadradito va centrado en la caja de línea de la etiqueta.
	y := (t.Height - LadoEleccion) / 2
	pos := fyne.NewPos(0, y)
	r.pieza.Colocar(pos)
	r.aro.ColocarEn(pos, fyne.NewSize(LadoEleccion, LadoEleccion))
	if r.parrafo != nil && r.e.AnchoEtiqueta > 0 {
		w := max(min(t.Width-LadoEleccion-HuecoEleccion, r.e.AnchoEtiqueta), 1)
		h := AltoParrafo(r.parrafo, w)
		r.parrafo.Move(fyne.NewPos(LadoEleccion+HuecoEleccion, (t.Height-h)/2))
		r.parrafo.Resize(fyne.NewSize(w, h))
	}
	if r.texto != nil {
		ColocarTexto(r.texto, r.e.escalon(), LadoEleccion+HuecoEleccion, (t.Height-r.e.escalon().Linea)/2)
	}
}

func (r *eleccionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.pieza.Pintar(EstadoCasilla{Marcada: r.e.Marcada, Parcial: r.e.Parcial, Encima: r.e.encima && !r.e.inerte, Inerte: r.e.inerte}, v)
	r.aro.Pintar(v)
	r.aro.Ver(r.e.enfocado)
	if r.texto != nil {
		r.texto.Text = r.e.Etiqueta
		r.texto.Color = atenuar(tema.Color("text", v), r.e.inerte)
		r.texto.Refresh()
		ver(r.texto, r.e.AnchoEtiqueta <= 0)
	}
	if r.parrafo != nil {
		r.parrafo.Texto, r.parrafo.Escalon = r.e.Etiqueta, r.e.escalon()
		r.parrafo.Tinta = atenuar(tema.Color("text", v), r.e.inerte)
		r.parrafo.Ancho = r.e.AnchoEtiqueta
		r.parrafo.Refresh()
		ver(r.parrafo, r.e.AnchoEtiqueta > 0)
	}
	r.Layout(r.e.Size())
}

// ver enseña o esconde un objeto. Fyne no trae un `SetVisible`.
func ver(o fyne.CanvasObject, visible bool) {
	if visible {
		o.Show()
		return
	}
	o.Hide()
}
