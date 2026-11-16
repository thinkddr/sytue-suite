// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Asistente es `Wizard` de `@sytue/ui` (Wizard.tsx): la configuración GUIADA paso a
// paso, una idea por paso. Arriba el progreso (`Pasos`), luego el encabezado del paso
// en lenguaje llano (`sytue-h3`) con su frase (`text-base`, `max-w-2xl`), lo de dentro y
// la barra de acciones: `border-t pt-4` con «Cancelar» a la izquierda y «Atrás» y
// «Continuar» a la derecha. Todo en una columna con `gap-6`.
//
// El asistente NO decide el contenido: cada paso trae su `Cuerpo` y dice si se puede
// avanzar (`PuedeSeguir`). Un paso con `AlSiguiente` ejecuta su acción en vez de avanzar
// («Activar», que llama a la API y luego mueve el paso con `IrA`). Un paso `Terminal`
// no lleva barra de acciones ni vuelta atrás.
//
// Accesibilidad, como la web: al cambiar de paso el foco va al ENCABEZADO del paso —que
// no es parada del tabulador, es el `tabIndex={-1}`—, y no al montar, que robaría el
// foco a quien ya navegaba la página. Los pasos hechos son botones del progreso.
//
// Lo usaba solo el alta del Hub, escrito a mano en `apps/hub/bienvenida.go`: §2.8 dice
// que el asistente no se reimplementa por app.
type Asistente struct {
	widget.BaseWidget

	Pasos []PasoAsistente
	// Actual es el paso en curso. Se cambia con IrA, que avisa y mueve el foco; si se
	// cambia a mano, el siguiente Refresh también lleva el foco al encabezado.
	Actual int
	// AlCambiar avisa de que el paso en curso ha cambiado (`onCurrentChange`).
	AlCambiar func(int)
	// AlCancelar, si está, pone «Cancelar» a la izquierda.
	AlCancelar func()
	// Ocupado es el `busy`: las acciones se apagan, «Continuar» sale con su rueda y el
	// progreso deja de ser pulsable.
	Ocupado bool
	// Ventana es el ancho de la VENTANA, para el `sm:` del progreso. Cero es ancha.
	Ventana float32
	// Rotulos son los textos del armazón; NuevoAsistente los toma del catálogo común.
	Rotulos RotulosAsistente

	progreso                   *Pasos
	encabezado                 *encabezadoPaso
	descripcion                *Parrafo
	cancelar, atras, siguiente *Boton
	filete                     *canvas.Rectangle
	// previo es el paso que se pintó la última vez: el foco solo se mueve al cambiar.
	previo int
}

// PasoAsistente es un paso del asistente (`WizardStep`).
type PasoAsistente struct {
	// Titulo es la etiqueta corta del progreso («Configurar»).
	Titulo string
	// Encabezado es el del paso en lenguaje llano; vacío, el Titulo.
	Encabezado string
	// Descripcion es una frase que explica qué se hace aquí.
	Descripcion string
	Cuerpo      fyne.CanvasObject
	// PuedeSeguir es la verja del paso: si dice que no, «Continuar» se apaga. Nil, se
	// puede. Se pregunta en cada Refresh: quien cambia lo que mira, refresca.
	PuedeSeguir func() bool
	// RotuloSiguiente es el del botón de avance; vacío, «Continuar».
	RotuloSiguiente string
	// AlSiguiente es la acción propia del paso en vez de avanzar.
	AlSiguiente func()
	// Terminal es el `done`: sin barra de acciones ni vuelta atrás.
	Terminal bool
	// SinVuelta es el `canGoBack={false}`: sin «Atrás» y con el progreso sin enlaces,
	// pero con su avance. Para el paso que va DESPUÉS de algo que ya no se deshace.
	SinVuelta bool
}

// RotulosAsistente son los textos del armazón (`WizardLabels`).
type RotulosAsistente struct {
	Atras, Siguiente, Cancelar string
	// IrAlPaso es el nombre accesible de un paso hecho: «Volver al paso 1: ».
	IrAlPaso func(n int) string
}

var (
	_ fyne.Widget     = (*Asistente)(nil)
	_ fyne.Accessible = (*Asistente)(nil)
)

// Medidas de Wizard.tsx.
const (
	// HuecoAsistente es el `gap-6` de la columna.
	HuecoAsistente float32 = 6 * 0.25 * tema.Rem
	// MargenDescripcionPaso es el `mt-1.5` de la frase del paso.
	MargenDescripcionPaso float32 = 1.5 * 0.25 * tema.Rem
	// MargenPieAsistente es el `pt-4` de la barra de acciones, bajo su filete.
	MargenPieAsistente float32 = 4 * 0.25 * tema.Rem
	// HuecoBotonesAsistente es el `gap-2` entre los botones de la barra.
	HuecoBotonesAsistente float32 = 2 * 0.25 * tema.Rem
	// TamIconoAtras es el `<Icon name="chevronLeft" size={16}>` de «Atrás».
	TamIconoAtras float32 = 16
)

// NuevoAsistente crea el asistente con los textos del armazón del catálogo común
// (`ui.wizard.*`): en una consola en inglés, los botones del pie salían en español.
func NuevoAsistente(t *i18n.Translator, pasos ...PasoAsistente) *Asistente {
	a := &Asistente{Pasos: pasos}
	if t != nil {
		a.Rotulos = RotulosAsistente{
			Atras:     t.T("ui.wizard.back", nil),
			Siguiente: t.T("ui.wizard.next", nil),
			Cancelar:  t.T("ui.wizard.cancel", nil),
			IrAlPaso:  func(n int) string { return t.T("ui.wizard.goToStep", i18n.Vars{"n": n}) },
		}
	}
	a.ExtendBaseWidget(a)
	return a
}

// AccessibilityRole: un contenedor; se leen sus piezas.
func (a *Asistente) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el encabezado del paso en curso.
func (a *Asistente) AccessibilityLabel() string {
	if p, ok := a.paso(); ok {
		return p.encabezado()
	}
	return ""
}

func (p PasoAsistente) encabezado() string {
	if p.Encabezado != "" {
		return p.Encabezado
	}
	return p.Titulo
}

// paso es el paso en curso, acotado.
func (a *Asistente) paso() (PasoAsistente, bool) {
	if len(a.Pasos) == 0 {
		return PasoAsistente{}, false
	}
	return a.Pasos[min(max(a.Actual, 0), len(a.Pasos)-1)], true
}

// sinVuelta: ni «Atrás» ni progreso pulsable.
func (a *Asistente) sinVuelta() bool {
	p, _ := a.paso()
	return p.Terminal || p.SinVuelta
}

// PuedeSeguir dice si «Continuar» está encendido: la verja del paso y que no esté
// ocupado.
func (a *Asistente) PuedeSeguir() bool {
	p, ok := a.paso()
	if !ok || a.Ocupado || p.Terminal {
		return false
	}
	return p.PuedeSeguir == nil || p.PuedeSeguir()
}

// IrA lleva el asistente al paso i, avisa y deja el foco en su encabezado.
func (a *Asistente) IrA(i int) {
	if i < 0 || i >= len(a.Pasos) || i == a.Actual {
		return
	}
	a.Actual = i
	a.Refresh()
	if a.AlCambiar != nil {
		a.AlCambiar(i)
	}
}

// Avanzar es «Continuar»: la acción del paso si la tiene y, si no, el siguiente. No
// hace nada si la verja está cerrada, que es lo mismo que pulsar un botón apagado.
func (a *Asistente) Avanzar() {
	if !a.PuedeSeguir() {
		return
	}
	p, _ := a.paso()
	if p.AlSiguiente != nil {
		p.AlSiguiente()
		return
	}
	a.IrA(a.Actual + 1)
}

// Retroceder es «Atrás».
func (a *Asistente) Retroceder() {
	if a.Ocupado || a.sinVuelta() {
		return
	}
	a.IrA(a.Actual - 1)
}

// piezas crea una vez los controles del armazón: sobreviven a los cambios de paso para
// no quitarle el foco a quien acaba de pulsar «Continuar».
func (a *Asistente) piezas() {
	if a.progreso != nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.progreso = NuevosPasos()
	a.encabezado = &encabezadoPaso{p: NuevoParrafo("", EscalonTitulo(3), PesoTitulo(3), "text", v)}
	a.encabezado.ExtendBaseWidget(a.encabezado)
	a.descripcion = NuevoParrafo("", tema.TextoBase, Normal, "text-muted", v)
	a.cancelar = NuevoBoton("", func() {
		if a.AlCancelar != nil {
			a.AlCancelar()
		}
	}).Con(Fantasma, TallaMD)
	a.atras = NuevoBoton("", a.Retroceder).Con(Secundario, TallaMD).ConIcono(iconos.ChevronIzquierda, TamIconoAtras)
	a.siguiente = NuevoBoton("", a.Avanzar)
	a.filete = canvas.NewRectangle(tema.Color("border-subtle", v))
	a.previo = a.Actual
}

// sincronizar pone en las piezas el estado del paso en curso.
func (a *Asistente) sincronizar() {
	a.piezas()
	p, _ := a.paso()
	titulos := make([]string, len(a.Pasos))
	for i, x := range a.Pasos {
		titulos[i] = x.Titulo
	}
	a.progreso.Titulos, a.progreso.Ventana = titulos, a.Ventana
	a.progreso.Actual = min(max(a.Actual, 0), max(len(a.Pasos)-1, 0))
	a.progreso.RotuloIr = a.Rotulos.IrAlPaso
	a.progreso.AlElegir = nil
	if !a.sinVuelta() && !a.Ocupado {
		a.progreso.AlElegir = a.IrA
	}
	a.encabezado.p.Texto = p.encabezado()
	a.descripcion.Texto = p.Descripcion
	a.cancelar.Texto, a.cancelar.Inerte = a.Rotulos.Cancelar, a.Ocupado
	a.atras.Texto, a.atras.Inerte = a.Rotulos.Atras, a.Ocupado
	a.siguiente.Texto = p.RotuloSiguiente
	if a.siguiente.Texto == "" {
		a.siguiente.Texto = a.Rotulos.Siguiente
	}
	a.siguiente.Cargando = a.Ocupado
	a.siguiente.Inerte = !a.Ocupado && !a.PuedeSeguir()
}

// botones son los de la barra: «Cancelar» (o nil) y los de la derecha.
func (a *Asistente) botones() (cancelar *Boton, derecha []*Boton) {
	if a.AlCancelar != nil {
		cancelar = a.cancelar
	}
	if a.Actual > 0 && !a.sinVuelta() {
		derecha = append(derecha, a.atras)
	}
	return cancelar, append(derecha, a.siguiente)
}

func anchoFila(bs []*Boton) (w, h float32) {
	for i, b := range bs {
		m := b.MinSize()
		if i > 0 {
			w += HuecoBotonesAsistente
		}
		w += m.Width
		h = max(h, m.Height)
	}
	return w, h
}

// MedidaAsistente es lo que ocupa el asistente a ese ancho. El ancho no baja del de la
// barra de acciones. Es la única cuenta (regla 2 de ui.go).
func MedidaAsistente(a *Asistente, ancho float32) fyne.Size {
	a.sincronizar()
	ancho = max(ancho, a.anchoMinimo())
	return fyne.NewSize(ancho, a.colocar(ancho, false))
}

// AltoPara es el alto a ese ancho, para `AltoA` y las secciones.
func (a *Asistente) AltoPara(ancho float32) float32 { return MedidaAsistente(a, ancho).Height }

func (a *Asistente) anchoMinimo() float32 {
	cancelar, derecha := a.botones()
	w, _ := anchoFila(derecha)
	if cancelar != nil {
		w = max(w, cancelar.MinSize().Width)
	}
	return w
}

// colocar mide (y, con `mover`, coloca) el asistente a ese ancho y devuelve su alto.
func (a *Asistente) colocar(ancho float32, mover bool) float32 {
	p, ok := a.paso()
	if !ok {
		return 0
	}
	// Todo va a la izquierda de la columna: solo la barra de botones se reparte.
	poner := func(o fyne.CanvasObject, y, w, h float32) {
		if mover {
			o.Move(fyne.NewPos(0, y))
			o.Resize(fyne.NewSize(w, h))
		}
	}
	y := float32(0)
	poner(a.progreso, y, ancho, AltoPasos)
	y += AltoPasos + HuecoAsistente

	h := AltoParrafo(a.encabezado.p, ancho)
	poner(a.encabezado, y, ancho, h)
	y += h
	if p.Descripcion != "" {
		w := min(ancho, AnchoLecturaAjustes)
		h = AltoParrafo(a.descripcion, w)
		poner(a.descripcion, y+MargenDescripcionPaso, w, h)
		y += MargenDescripcionPaso + h
	}
	if p.Cuerpo != nil {
		y += HuecoAsistente
		h = AltoA(p.Cuerpo, ancho)
		poner(p.Cuerpo, y, ancho, h)
		y += h
	}
	if p.Terminal {
		return y
	}
	y += HuecoAsistente
	poner(a.filete, y, ancho, tema.Borde)
	y += tema.Borde + MargenPieAsistente
	return y + a.colocarBotones(y, ancho, mover)
}

// colocarBotones pone la barra (`flex flex-wrap`, con la derecha en `ml-auto`) y
// devuelve su alto. Si «Cancelar» y la derecha no caben juntos, la derecha baja a otra
// línea, como hace el `flex-wrap`.
func (a *Asistente) colocarBotones(y, ancho float32, mover bool) float32 {
	cancelar, derecha := a.botones()
	wd, hd := anchoFila(derecha)
	alto := hd
	yd := y
	if cancelar != nil {
		mc := cancelar.MinSize()
		if mc.Width+HuecoBotonesAsistente+wd > ancho {
			yd = y + mc.Height + HuecoBotonesAsistente
			alto = mc.Height + HuecoBotonesAsistente + hd
		} else {
			alto = max(mc.Height, hd)
		}
		if mover {
			// `items-center`: en la misma línea, centrado con los de la derecha.
			cy := y
			if yd == y {
				cy += (alto - mc.Height) / 2
			}
			cancelar.Move(fyne.NewPos(0, cy))
			cancelar.Resize(mc)
		}
	}
	if mover {
		x := ancho - wd
		for _, b := range derecha {
			m := b.MinSize()
			b.Move(fyne.NewPos(x, yd+(hd-m.Height)/2))
			b.Resize(m)
			x += m.Width + HuecoBotonesAsistente
		}
	}
	return alto
}

// CreateRenderer compone el progreso, el encabezado, el paso y la barra. Termina en
// Refresh (regla 7).
func (a *Asistente) CreateRenderer() fyne.WidgetRenderer {
	a.piezas()
	r := &asistenteRenderer{a: a}
	r.Refresh()
	return r
}

type asistenteRenderer struct {
	a       *Asistente
	objetos []fyne.CanvasObject
}

func (r *asistenteRenderer) MinSize() fyne.Size {
	return MedidaAsistente(r.a, r.a.Size().Width)
}
func (r *asistenteRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *asistenteRenderer) Destroy()                     {}

func (r *asistenteRenderer) Layout(t fyne.Size) { r.a.colocar(t.Width, true) }

// Refresh pone el paso en curso, repinta con el tema y, si el paso ha cambiado desde el
// último pintado, lleva el foco a su encabezado.
func (r *asistenteRenderer) Refresh() {
	a := r.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.sincronizar()
	p, _ := a.paso()
	a.encabezado.p.Tinta = tema.Color("text", v)
	a.descripcion.Tinta = tema.Color("text-muted", v)
	a.filete.FillColor = tema.Color("border-subtle", v)
	r.objetos = []fyne.CanvasObject{a.progreso, a.encabezado}
	if p.Descripcion != "" {
		r.objetos = append(r.objetos, a.descripcion)
	}
	if p.Cuerpo != nil {
		r.objetos = append(r.objetos, p.Cuerpo)
	}
	if !p.Terminal {
		r.objetos = append(r.objetos, a.filete)
		cancelar, derecha := a.botones()
		if cancelar != nil {
			r.objetos = append(r.objetos, cancelar)
		}
		for _, b := range derecha {
			r.objetos = append(r.objetos, b)
		}
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
	r.Layout(a.Size())
	if a.previo != a.Actual {
		a.previo = a.Actual
		if c := fyne.CurrentApp().Driver().CanvasForObject(a); c != nil {
			a.encabezado.dar(c, a.encabezado)
		}
	}
}

// encabezadoPaso es el `<h2 tabIndex={-1}>` del paso: se le da el foco por código al
// cambiar de paso —el lector anuncia el paso nuevo y quien usa el teclado sigue desde
// ahí—, pero no es una parada del tabulador (`sinParada`). Sin anillo: la web lo quita
// con `focus-visible:outline-none`.
type encabezadoPaso struct {
	widget.BaseWidget
	sinParada
	p        *Parrafo
	enfocado bool
}

var (
	_ fyne.Focusable  = (*encabezadoPaso)(nil)
	_ fyne.Accessible = (*encabezadoPaso)(nil)
)

func (e *encabezadoPaso) FocusGained()   { e.enfocado = true }
func (e *encabezadoPaso) FocusLost()     { e.enfocado = false }
func (e *encabezadoPaso) TypedRune(rune) {}

// TypedKey no usa ninguna tecla, pero Escape y el «atrás» del teléfono tienen que
// llegar a la capa de encima: con el foco aquí, un diálogo no se cerraría.
func (e *encabezadoPaso) TypedKey(k *fyne.KeyEvent) { SubirEscape(e, k) }

func (e *encabezadoPaso) AccessibilityLabel() string { return e.p.Texto }
func (e *encabezadoPaso) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

func (e *encabezadoPaso) CreateRenderer() fyne.WidgetRenderer {
	r := &encabezadoRenderer{e: e}
	r.Refresh()
	return r
}

type encabezadoRenderer struct{ e *encabezadoPaso }

func (r *encabezadoRenderer) MinSize() fyne.Size           { return r.e.p.MinSize() }
func (r *encabezadoRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.e.p} }
func (r *encabezadoRenderer) Destroy()                     {}
func (r *encabezadoRenderer) Layout(t fyne.Size) {
	r.e.p.Move(fyne.NewPos(0, 0))
	r.e.p.Resize(t)
}

func (r *encabezadoRenderer) Refresh() {
	r.e.p.Refresh()
	r.Layout(r.e.Size())
}
