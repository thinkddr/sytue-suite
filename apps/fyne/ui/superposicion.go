// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"reflect"
	"sync"
	"time"
	"weak"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # Lo que se pinta ENCIMA: diálogos, menús, avisos y pistas
//
// Todos van a `fyne.Canvas.Overlays()`, que es la pila de superposiciones de Fyne, y no
// a un hueco reservado dentro de la pantalla. Tres cosas salen gratis por ir ahí y las
// tres había que escribirlas a mano en el port anterior:
//
//   - **El atrapa-foco.** `OverlayStack.Add` crea un `app.FocusManager` POR capa y el
//     Tab del driver usa el de la capa de ARRIBA (`TopFocusManager`). El foco no puede
//     salirse de un diálogo abierto porque el gestor que lo mueve no ve nada más.
//   - **La inercia de lo de debajo.** La capa ocupa el lienzo entero, así que el
//     puntero no llega a lo que tapa.
//   - **El orden.** Un menú abierto dentro de un diálogo se apila ENCIMA, y Escape se
//     lleva primero lo de encima, que es lo que espera cualquiera.
//
// Y la trampa que evita: **el velo no está puesto siempre**. Una capa de velo
// permanente —aunque sea transparente— deja la aplicación entera sin poder pulsar. La
// capa se crea al abrir y se retira al cerrar; `TestSinDialogoNoHayVeloQueEstorbe` lo
// vigila invirtiéndolo.

// Descartable es una capa que sabe cerrarse: el diálogo, el menú emergente, la pista.
type Descartable interface {
	fyne.CanvasObject
	// Descartar es lo que hacen Escape, el clic fuera y el aspa: PEDIR el cierre.
	// Quien lo implementa decide si cierra o pregunta antes, como `onClose` en la web.
	Descartar()
}

// Descartar cierra la capa de más arriba del lienzo y dice si había alguna.
//
// # Por qué esto es una función y no un manejador de Escape
//
// La web atrapa Escape en `document` y en fase de CAPTURA, así que el diálogo lo recibe
// aunque el foco esté dentro de un campo. En Fyne **no hay fase de captura**:
// `processKeyPressed` entrega la tecla al objeto enfocado y, solo si no hay ninguno, al
// lienzo; y un Escape sin modificador nunca llega a la tabla de atajos
// (`triggersShortcut` solo sintetiza atajo cuando hay modificador). Comprobado en
// `internal/driver/glfw/window.go` de Fyne v2.8.1.
//
// O sea: el Escape de un campo enfocado es del campo. Por eso el contrato se invierte
// respecto a la web —lo que no consuma la tecla la sube—, y esta es la subida. Un
// control que atiende teclas y no necesita Escape llama aquí:
//
//	case fyne.KeyEscape:
//	    ui.Descartar(fyne.CurrentApp().Driver().CanvasForObject(w))
//
// Está en UNA función y no repetido en cada control a propósito: la regla «primero lo
// de encima» se decide aquí, mirando la cima de la pila, y no en doce sitios.
func Descartar(c fyne.Canvas) bool {
	if c == nil {
		return false
	}
	d, es := c.Overlays().Top().(Descartable)
	if !es {
		// Sin capa encima, un cajón abierto: Escape lo cierra (`useNavDrawer`).
		if a := cajonDe(c); a != nil && c.Overlays().Top() == nil {
			a.CerrarCajon()
			return true
		}
		return false
	}
	d.Descartar()
	return true
}

// SubirEscape es esa subida para quien no necesita Escape: si la tecla lo es (o es el
// «atrás» del teléfono) y hay una capa encima, la cierra y dice que se la ha quedado.
// `yo` es el control enfocado. TODO control enfocable de la suite que no use Escape lo
// llama desde su `TypedKey`: si no, con el foco en él el diálogo de encima no se cierra
// con el teclado. Lo vigila `TestTodoTypedKeySubeEscape`.
func SubirEscape(yo fyne.CanvasObject, k *fyne.KeyEvent) bool {
	c := lienzoDe(yo)
	if EsCierre(k) && Descartar(c) {
		return true
	}
	if k != nil && k.Name == mobile.KeyBack {
		// «Atrás» con el foco en un campo y nada encima: lo mismo que sin foco. Sin
		// esto, con el cursor en un buscador el botón no hacía nada.
		atras(c)
		return true
	}
	return false
}

// # «Atrás» del teléfono
//
// Lo de encima se lo lleva primero (Descartar). Si no hay nada, la app decide: una página
// de detalle vuelve a la anterior. Y si la app no la quiere —ya está en su portada—, la
// app va al fondo, que es lo que hace el driver con la ranura de teclas vacía.

// atrases son los receptores de «atrás» de cada lienzo, puestos con AlAtras.
var atrases sync.Map // weak.Pointer[byte] del lienzo → func() bool

// AlAtras pone lo que hace el botón «atrás» de Android en ese lienzo cuando no hay nada
// encima que cerrar. `f` dice si se lo ha quedado; si devuelve false, la app va al fondo.
// Asegura además la ranura de teclas, para que llegue aunque nada esté enfocado.
func AlAtras(c fyne.Canvas, f func() bool) {
	k, ok := claveLienzo(c)
	if !ok {
		return
	}
	atrases.Store(k, f)
	asegurarTeclas(c)
}

// atras es «atrás» sin nada que cerrar: el receptor de la app o, si no lo quiere, al fondo.
func atras(c fyne.Canvas) {
	if c != nil {
		// Igual que al pulsar el botón visible: FocusLost confirma el campo antes
		// de guardar o cambiar de vista. Atrás también llega con el cursor dentro.
		c.Unfocus()
	}
	if k, ok := claveLienzo(c); ok {
		if v, ok := atrases.Load(k); ok {
			if f, _ := v.(func() bool); f != nil && f() {
				return
			}
		}
	}
	if c != nil {
		if a := armazonEn(c.Content()); a != nil && a.volverAlPanel() {
			return
		}
	}
	// Lo que hace el driver móvil cuando la ranura está vacía: la app al fondo.
	if d, es := fyne.CurrentApp().Driver().(interface{ GoBack() }); es {
		d.GoBack()
	}
}

// EsCierre dice si la tecla pide cerrar lo de encima: Escape o el «atrás» de Android,
// que el driver móvil entrega como `mobile.KeyBack` al control enfocado y NUNCA como
// Escape. Sin esto, «atrás» con un diálogo abierto mandaba la app al fondo y dejaba el
// diálogo puesto.
func EsCierre(k *fyne.KeyEvent) bool {
	return k != nil && (k.Name == fyne.KeyEscape || k.Name == mobile.KeyBack)
}

// # Escape sin nada enfocado
//
// Un clic en el cuerpo de un diálogo —en un texto, fuera de cualquier control— QUITA el
// foco (`processMouseClicked` desenfoca lo que no es enfocable), y sin foco el driver
// entrega la tecla a `Canvas.OnTypedKey`: la ranura global, que casi ninguna app rellena
// y las que sí (Notepads, CertiPDF, el ERP) la usan para otra cosa. El diálogo se quedaba
// sin poder cerrarse con Escape tras el primer clic. Y en Android, «atrás» sin foco y sin
// ranura manda la app al fondo con el diálogo puesto.
//
// asegurarTeclas pone delante de esa ranura la subida de `Descartar`, envolviendo lo que
// la app tuviera. Se llama al abrir una capa o el cajón: una app que vuelva a llamar a
// `SetOnTypedKey` después queda envuelta en la siguiente apertura.
func asegurarTeclas(c fyne.Canvas) {
	previa := c.OnTypedKey()
	if previa != nil && reflect.ValueOf(previa).Pointer() == punteroTeclas {
		return
	}
	c.SetOnTypedKey(teclasSueltas(c, previa))
}

func teclasSueltas(c fyne.Canvas, previa func(*fyne.KeyEvent)) func(*fyne.KeyEvent) {
	return func(k *fyne.KeyEvent) {
		if EsCierre(k) && Descartar(c) {
			return
		}
		if k != nil && k.Name == mobile.KeyBack {
			atras(c)
			return
		}
		if previa != nil {
			previa(k)
		}
	}
}

// punteroTeclas es el código de la envoltura: todas las que salen de `teclasSueltas`
// comparten el mismo, y así se sabe si la ranura ya está envuelta.
//
// En un `init` y no en la declaración: `teclasSueltas` llega, por `Descartar` y el cajón,
// otra vez a `asegurarTeclas`, y eso es un ciclo de inicialización.
var punteroTeclas uintptr

func init() { punteroTeclas = reflect.ValueOf(teclasSueltas(nil, nil)).Pointer() }

// Velo es el fondo de una capa: `bg-[var(--sytue-overlay)]` en un diálogo, y nada
// —transparente pero pulsable— en un menú emergente.
//
// Es un widget y no un `canvas.Rectangle` porque tiene que recibir el clic de fuera, y
// el recorrido de Fyne solo desciende en `*fyne.Container` y en `fyne.Widget`: un
// rectángulo suelto no se pulsa.
type Velo struct {
	widget.BaseWidget
	// Opaco: con el color `overlay` de la hoja. Sin él, transparente.
	Opaco bool
	// AlPulsar es el clic fuera. nil: el clic no cierra (`dismissable={false}`).
	AlPulsar func()
	// cruzado es el velo de lo que NO es modal (menú, popover): cierra al APRETAR, como
	// el `mousedown` de la web, y deja que el clic siga hasta lo de debajo (TapThrough).
	cruzado bool
	// nacido es cuándo se puso: el segundo clic de un doble clic que ABRIÓ el diálogo
	// cae sobre el velo, y no lo tiene que cerrar (ver Tapped).
	nacido time.Time

	fondo *canvas.Rectangle
}

var (
	_ fyne.Widget       = (*Velo)(nil)
	_ fyne.Tappable     = (*Velo)(nil)
	_ desktop.Mouseable = (*Velo)(nil)
)

// nuevoVeloCruzado es el velo transparente de un menú o un popover.
func nuevoVeloCruzado(alPulsar func()) *Velo {
	v := NuevoVelo(false, alPulsar)
	v.cruzado = true
	return v
}

// # El clic que cierra un menú no se pierde
//
// En la web, el clic fuera de un menú lo cierra en el `mousedown` y el `click` sigue su
// camino: se pulsa lo que había debajo. En Fyne, con una capa puesta el puntero SOLO
// visita la capa, así que el velo se tragaba el clic y hacían falta dos.
//
// Desde `v2.8.1-sytue.5` el driver del fork lo resuelve donde toca: si al APRETAR se cierra
// una capa que dice `TapThrough`, vuelve a despachar ese mismo clic desde el principio a
// lo que haya debajo —un control de la suite o un `widget.Entry`, da igual—, con su
// `MouseDown`, su foco y su `Tapped`. Y en un teléfono, el toque. El velo solo tiene que
// cerrar al apretar. Hasta el `.4` lo remataban a mano los controles de la suite
// (`clicCruzado`), y un campo de Fyne debajo no se enteraba.

// MouseDown cierra el menú o el popover al apretar, como la web.
func (v *Velo) MouseDown(ev *desktop.MouseEvent) {
	if !v.cruzado || v.AlPulsar == nil || ev == nil {
		return
	}
	v.AlPulsar()
}

// MouseUp no hace nada: el clic es ya del control de debajo.
func (v *Velo) MouseUp(*desktop.MouseEvent) {}

// NuevoVelo crea el velo.
func NuevoVelo(opaco bool, alPulsar func()) *Velo {
	v := &Velo{Opaco: opaco, AlPulsar: alPulsar, nacido: time.Now()}
	v.ExtendBaseWidget(v)
	return v
}

// ColorVelo es el token `overlay` con la opacidad de la animación de entrada aplicada:
// `p` va de 0 a 1.
func ColorVelo(v tema.Variante, p float32) color.NRGBA {
	c := tema.Color("overlay", v)
	c.A = uint8(float32(c.A)*p + 0.5)
	return c
}

// CreateRenderer construye el velo: un rectángulo y nada más.
func (v *Velo) CreateRenderer() fyne.WidgetRenderer {
	// Nace con el color FINAL, no a cero: si el driver no anima —o alguien apaga las
	// animaciones— un velo que arranca transparente se queda transparente, y un
	// diálogo sin velo es un diálogo que no se ve que es modal. `Entrar` lo pone a
	// cero y lo trae de vuelta; si no llega a correr, no se pierde nada.
	c := color.NRGBA{}
	if v.Opaco {
		c = ColorVelo(tema.De(fyne.CurrentApp().Settings().ThemeVariant()), 1)
	}
	v.fondo = canvas.NewRectangle(c)
	return &veloRenderer{v: v}
}

// esperaVelo es esa ventana; las pruebas de este paquete, que pulsan el velo en el
// mismo instante en que lo ponen, la dejan a cero (TestMain).
var esperaVelo = esperaDobleClic

// Tapped es el clic fuera. El que llega dentro de la ventana de un doble clic desde que
// se puso el velo es el segundo clic de ese doble clic —en Drive, doble clic en un
// fichero abría la vista previa y el segundo clic la cerraba en el acto (visto en
// Windows)— y no cierra nada.
func (v *Velo) Tapped(*fyne.PointEvent) {
	if time.Since(v.nacido) < esperaVelo {
		return
	}
	if v.AlPulsar != nil {
		v.AlPulsar()
	}
}

// Entrar arranca `sytue-fade-in`: 150 ms de 0 a la opacidad del token.
//
// El driver de pruebas de Fyne llama a `Tick(1)` y punto, así que en un guardián el
// velo sale directamente a su color final y la animación no falsea ninguna medida.
func (v *Velo) Entrar() {
	if !v.Opaco || v.fondo == nil {
		return
	}
	variante := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a := fyne.NewAnimation(DuracionFundido, func(p float32) {
		v.fondo.FillColor = ColorVelo(variante, p)
		canvas.Refresh(v.fondo)
	})
	a.Curve = CurvaEstandar
	a.Start()
}

type veloRenderer struct{ v *Velo }

func (r *veloRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *veloRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.v.fondo} }
func (r *veloRenderer) Destroy()                     {}
func (r *veloRenderer) Layout(t fyne.Size)           { r.v.fondo.Resize(t) }

// Refresh sigue al tema: el `overlay` del oscuro es otro.
func (r *veloRenderer) Refresh() {
	if r.v.Opaco {
		r.v.fondo.FillColor = ColorVelo(tema.De(fyne.CurrentApp().Settings().ThemeVariant()), 1)
	}
	canvas.Refresh(r.v.fondo)
}

// capa es una superposición: ocupa el lienzo entero, pinta un fondo pulsable y coloca
// un panel encima. La usan el diálogo, el menú emergente, la pila de avisos y la pista.
//
// `coloca` recibe el tamaño del lienzo y devuelve dónde y cuánto mide el panel. Es el
// único sitio donde se reparte la geometría de una capa, y por eso los topes de la web
// —`max-w`, `max-h-[90%]`, el clamp del menú— se leen en una sola función por pieza.
type capa struct {
	widget.BaseWidget
	fondo *Velo
	panel fyne.CanvasObject
	// sombra es el `box-shadow` del panel, si lleva. Va de HERMANO y detrás, no
	// dentro: el panel recorta (`IsClip`) y un recorte se come lo que sobresale, que
	// es toda la sombra. nil: sin sombra.
	sombra *Sombra
	// faldon es el relleno que prolonga la hoja del teléfono desde el borde de abajo de
	// la capa hasta el de la pantalla: la capa vive en `InteractiveArea`, que deja fuera
	// la barra de inicio, y sin él ahí asomaba la pantalla de debajo sin velo. nil: nada.
	faldon      *canvas.Rectangle
	coloca      func(lienzo fyne.Size) (fyne.Position, fyne.Size)
	alDescartar func()
	// fundido es la entrada con opacidad de grupo, mientras dura (ver fundido.go).
	fundido *fundido
}

// sombraDe y faldonDe son la sombra y el faldón de una capa que puede no existir todavía
// (el diálogo se pinta antes de abrirse en algunos guardianes).
func (c *capa) sombraDe() *Sombra {
	if c == nil {
		return nil
	}
	return c.sombra
}

func (c *capa) faldonDe() *canvas.Rectangle {
	if c == nil {
		return nil
	}
	return c.faldon
}

// entrar es la entrada de la capa: el panel se funde desde transparente, a `dy` px y a
// `escala`, en `dur` (`sytue-modal-in`, `sytue-pop-in`). Va DESPUÉS de dar el foco: con
// el panel escondido, Fyne no enfoca lo que va dentro.
func (c *capa) entrar(lienzo fyne.Canvas, dur time.Duration, dy, escala, anclaY float32) {
	fundir(lienzo, c.panel, c.sombra, dur, dy, escala, anclaY, func(f *fundido) {
		c.fundido = f
		c.Refresh()
	})
}

// entrarLateral reutiliza la opacidad de grupo con translateX(16px), sin escala.
func (c *capa) entrarLateral(lienzo fyne.Canvas, dur time.Duration, dx float32) {
	fundirDesplazado(lienzo, c.panel, c.sombra, dur, dx, 0, 1, 0.5, func(f *fundido) {
		c.fundido = f
		c.Refresh()
	})
}

var (
	_ fyne.Widget = (*capa)(nil)
	_ Descartable = (*capa)(nil)
)

func nuevaCapa(fondo *Velo, panel fyne.CanvasObject, coloca func(fyne.Size) (fyne.Position, fyne.Size)) *capa {
	c := &capa{fondo: fondo, panel: panel, coloca: coloca}
	c.ExtendBaseWidget(c)
	return c
}

// TapThrough y ScrollThrough los pregunta el driver del fork de Fyne desde
// `v2.8.1-sytue.5`: una capa que NO es modal —la de un menú o un popover, con el velo
// transparente que cierra al apretar— deja que el clic que la cierra y la rueda del
// ratón lleguen a lo que tapa, como en la web, donde no hay velo y el `mousedown` en
// `document` solo cierra. La de un diálogo, no: su velo es el `onClick` del fondo.
func (c *capa) TapThrough() bool { return c.fondo.cruzado }

// ScrollThrough: ver TapThrough. Tras la rueda el driver refresca la capa, que vuelve a
// colocar el panel junto a su disparador (el `scroll` en captura de Dropdown.tsx).
func (c *capa) ScrollThrough() bool { return c.fondo.cruzado }

// Descartar sube el cierre a quien montó la capa.
func (c *capa) Descartar() {
	if c.alDescartar != nil {
		c.alDescartar()
	}
}

// CreateRenderer construye la capa: el fondo detrás y el panel delante.
func (c *capa) CreateRenderer() fyne.WidgetRenderer { return &capaRenderer{c: c} }

type capaRenderer struct{ c *capa }

func (r *capaRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }

func (r *capaRenderer) Objects() []fyne.CanvasObject {
	out := []fyne.CanvasObject{r.c.fondo, r.c.panel}
	if r.c.sombra != nil {
		out = []fyne.CanvasObject{r.c.fondo, r.c.sombra.Objeto(), r.c.panel}
	}
	if r.c.faldon != nil {
		out = append(out, r.c.faldon)
	}
	if o := r.c.fundido.objeto(); o != nil {
		out = append(out, o) // la imagen que se funde en el lugar del panel
	}
	// Los avisos de la ventana, ENCIMA del panel: ver `PilaAvisos`.
	if p := r.c.avisos(); p != nil {
		out = append(out, p.objetosAvisos()...)
	}
	return out
}

// avisos es la pila de avisos que esta capa se lleva encima: la de su ventana, si esta
// es la capa de arriba.
func (c *capa) avisos() *PilaAvisos {
	lienzo := lienzoDe(c)
	if lienzo == nil || lienzo.Overlays().Top() != fyne.CanvasObject(c) {
		return nil
	}
	p := pilaDe(lienzo)
	if p != nil && p.dialogoEncima() != nil {
		return nil
	}
	return p
}

func (r *capaRenderer) Destroy() {}

func (r *capaRenderer) Layout(t fyne.Size) {
	r.c.fondo.Resize(t)
	pos, tam := r.c.coloca(t)
	r.c.panel.Resize(tam)
	r.c.panel.Move(pos)
	if r.c.sombra != nil && r.c.fundido.objeto() == nil { // mientras se funde, la mueve él
		r.c.sombra.Ajustar(pos, tam, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	if f := r.c.faldon; f != nil {
		// Del borde de abajo del panel al de la pantalla, mientras no esté entrando.
		hasta := t.Height
		if l := lienzoDe(r.c); l != nil {
			hasta = max(hasta, l.Size().Height-r.c.Position().Y)
		}
		f.Move(fyne.NewPos(pos.X, pos.Y+tam.Height))
		f.Resize(fyne.NewSize(tam.Width, max(hasta-pos.Y-tam.Height, 0)))
		if r.c.fundido.objeto() != nil {
			f.Hide()
		} else {
			f.Show()
		}
	}
	if p := r.c.avisos(); p != nil {
		p.colocar(t)
	}
}

func (r *capaRenderer) Refresh() {
	r.Layout(r.c.Size())
	r.c.panel.Refresh()
}

// mostrar apila la capa y devuelve el foco que había, para poder devolverlo al cerrar.
func mostrar(c fyne.Canvas, cp *capa) fyne.Focusable {
	previo := c.Focused()
	asegurarTeclas(c)
	c.Overlays().Add(cp)
	sincronizarExclusionMenuMovil(c)
	// El driver móvil descuenta el notch, el indicador de inicio y, en iOS, el
	// teclado de InteractiveArea. Al abrir no hay necesariamente otro resize:
	// usar Size aquí volvía a poner la capa bajo esas zonas hasta girar la pantalla.
	pos, tam := c.InteractiveArea()
	cp.Resize(tam)
	cp.Move(pos)
	cp.fondo.Entrar()
	if p := pilaDe(c); p != nil {
		p.refrescar() // la pila se esconde y la capa se lleva los avisos
	}
	return previo
}

// ocultar retira la capa y devuelve el foco a donde estaba.
//
// Devolver el foco es la mitad que casi nunca se escribe y la que se nota: sin ella,
// cerrar un diálogo deja el teclado en ninguna parte y el siguiente Tab empieza por el
// principio de la pantalla.
func ocultar(c fyne.Canvas, cp *capa, previo fyne.Focusable) {
	if c == nil {
		return
	}
	c.Overlays().Remove(cp)
	enfocar(c, previo)
	sincronizarExclusionMenuMovil(c)
	if p := pilaDe(c); p != nil {
		p.refrescar() // los avisos vuelven a la pila, o a la capa de debajo
	}
}

// porLienzo apunta, para cada ventana, una pieza suya —su pila de avisos, su cajón
// abierto— SIN alargarle la vida a ninguna de las dos: clave y valor son punteros
// débiles. Con un `sync.Map` de lienzo a pieza, cada ventana que alguna vez tuvo avisos
// quedaba viva para siempre, con su contenido, sus capas y las imágenes de sus sombras:
// medido, +280 MB por pasada en los tests de Mail, y lo mismo por cada ventana cerrada
// en una app abierta mucho rato.
//
// ponytail: la entrada de una ventana ya recogida se queda en el mapa (dos punteros
// débiles, apuntando a nada); si alguna vez pesa, `runtime.AddCleanup` sobre el lienzo.
type porLienzo[T any] struct{ m sync.Map } // weak.Pointer[byte] → weak.Pointer[T]

// claveLienzo es la identidad débil del lienzo: la de su objeto, que es un puntero en
// todos los drivers.
func claveLienzo(c fyne.Canvas) (weak.Pointer[byte], bool) {
	v := reflect.ValueOf(c)
	if c == nil || v.Kind() != reflect.Pointer || v.IsNil() {
		return weak.Pointer[byte]{}, false
	}
	return weak.Make((*byte)(v.UnsafePointer())), true
}

func (r *porLienzo[T]) poner(c fyne.Canvas, v *T) {
	if k, ok := claveLienzo(c); ok {
		r.m.Store(k, weak.Make(v))
	}
}

func (r *porLienzo[T]) quitar(c fyne.Canvas) {
	if k, ok := claveLienzo(c); ok {
		r.m.Delete(k)
	}
}

func (r *porLienzo[T]) de(c fyne.Canvas) *T {
	k, ok := claveLienzo(c)
	if !ok {
		return nil
	}
	v, _ := r.m.Load(k)
	w, _ := v.(weak.Pointer[T])
	return w.Value()
}

// lienzoDe busca el lienzo de un objeto ya montado. Devuelve nil si no lo está.
func lienzoDe(o fyne.CanvasObject) fyne.Canvas {
	a := fyne.CurrentApp()
	if a == nil || len(a.Driver().AllWindows()) == 0 {
		return nil
	}
	return a.Driver().CanvasForObject(o)
}
