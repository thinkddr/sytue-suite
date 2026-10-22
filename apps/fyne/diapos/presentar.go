// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Salir es el destino que devuelve TeclaPresentando cuando la tecla cierra la
// presentación. No es un índice, así que no puede ser un número: -1 se confundiría con
// «no hay destino» en la primera resta que alguien escriba.
const Salir = -2

// SinDestino es «esta tecla no hace nada aquí».
const SinDestino = -1

// TeclaPresentando es `presentKeyTarget`: a qué diapositiva ENTERA lleva una tecla.
// `ultima` es el índice de la última (0-based).
//
// Las flechas arriba y abajo NO están, y es a propósito: la web no las atiende, y una
// presentación que avanza con ↓ aquí y no allí es otra presentación.
func TeclaPresentando(tecla fyne.KeyName, i, ultima int) int {
	switch tecla {
	case fyne.KeyRight, fyne.KeySpace, fyne.KeyPageDown:
		return min(ultima, i+1)
	case fyne.KeyLeft, fyne.KeyPageUp:
		return max(0, i-1)
	case fyne.KeyHome:
		return 0
	case fyne.KeyEnd:
		return ultima
	case fyne.KeyEscape:
		return Salir
	default:
		return SinDestino
	}
}

// DestinoAccion es `actionTarget`: a qué diapositiva lleva el enlace de una caja.
// Devuelve SinDestino si el enlace no apunta a ninguna válida.
func DestinoAccion(accion json.RawMessage, i, ultima int) int {
	if len(accion) == 0 {
		return SinDestino
	}
	var s string
	if json.Unmarshal(accion, &s) == nil {
		switch s {
		case "first":
			return 0
		case "last":
			return ultima
		case "next":
			return min(ultima, i+1)
		case "prev":
			return max(0, i-1)
		}
		return SinDestino
	}
	var n int
	if json.Unmarshal(accion, &n) == nil && n >= 1 && n <= ultima+1 {
		return n - 1 // el modelo lo guarda 1-based
	}
	return SinDestino
}

// Transcurrido es `fmtElapsed`: el cronómetro del moderador, «M:SS» o «H:MM:SS».
func Transcurrido(segundos int) string {
	if segundos < 0 {
		segundos = 0
	}
	h, m, s := segundos/3600, segundos/60%60, segundos%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// i18nVars son las variables de una cadena del catálogo.
type i18nVars = i18n.Vars

// Medidas del visor (`Presentation` y `PresentButton` de SlidesCanvas.tsx).
var (
	// LadoBotonVisor es `h-11 w-11`: la zona de toque mínima de un dedo, 38,5 con el
	// rem de 14 (el comentario de la web dice 44 porque se escribió con rem 16).
	LadoBotonVisor = tema.Espacio(11)
	// HuecoBotonesVisor es el `gap-1.5` entre ellos; y van en `right-3 bottom-3`.
	HuecoBotonesVisor  = tema.Espacio(1.5)
	MargenBotonesVisor = tema.Espacio(3)
	// IconoBotonVisor es el `size="md"` del glifo.
	IconoBotonVisor float32 = 16
	// La pista de teclas va en `bottom-4 left-3`.
	AbajoPistaVisor = tema.Espacio(4)
	IzqPistaVisor   = tema.Espacio(3)
	// OpacidadMandos es el `opacity-35` de los mandos en reposo.
	OpacidadMandos float32 = 0.35
	// DesplazaTransicion es el `translateX(4%)` de sytue-present-slide-fwd (y −4 % de
	// -back): una fracción del ancho de la diapositiva.
	DesplazaTransicion float32 = 0.04
)

// DuracionTransicion es el `300ms ease-out` de sytue-present-fade y -slide.
const DuracionTransicion = 300 * time.Millisecond

// Presentacion es el modo presentación: UNA diapositiva escalada a la ventana entera,
// sobre la tinta de la presentación (`background: INK`), con sus pasos de animación, la
// capa de notas y los mandos a la vista.
//
// Los mandos no son un adorno —lo dice la web—: en una pantalla táctil no hay Escape ni
// flechas, y sin ellos de aquí no se sale.
//
// Las transiciones de fundido y deslizar son las de la web (ver `transicionar`), y las
// entradas y trayectorias de las cajas al revelar un paso también (ver `animarPaso`).
//
// El morph interpola las cajas emparejadas con la diapositiva anterior (`morph.go`).
//
// ponytail: el giro sale como con `prefers-reduced-motion`: el cambio es instantáneo y lo
// que se ve al acabar el paso es exactamente lo de la web. Fyne no gira un objeto; se
// relevaría pintando la caja en un raster girado durante los 700 ms.
type Presentacion struct {
	widget.BaseWidget

	M *Modelo
	// Diapo y Paso son la posición: el paso 0 es la diapositiva sin animaciones
	// disparadas.
	Diapo, Paso int
	// Notas es la capa de notas del ponente (tecla N).
	Notas bool
	// Moderador dice si la vista del moderador está abierta (tecla P).
	Moderador bool
	// AlSalir cierra la presentación.
	AlSalir func()
	// AlCambiarModerador abre o cierra la vista del moderador.
	AlCambiarModerador func(abierta bool)
	// AlMover avisa de cada cambio de posición (la vista del moderador lo sigue).
	AlMover func()
	// AlCambiarDiapo avisa de la diapositiva a la que se llega (no de los pasos): es lo que
	// «Presentar con Cast» manda a los asistentes.
	AlCambiarDiapo func(diapo int)
	// AbrirMedio reproduce el audio o el vídeo de una caja pulsada; nil, no se pulsan.
	//
	// ponytail: Fyne no trae reproductor, y uno propio pide un decodificador de vídeo
	// (FFmpeg o similar) que no entra sin dependencias de fuera: el medio se reproduce con
	// el reproductor del SISTEMA (ver `AbrirMedioDelSistema`), no dentro de la
	// diapositiva, y el `autoplay` no lo abre solo.
	AbrirMedio func(Caja)

	rotulo  func(clave string, v i18nVars) string
	lienzo  *Lienzo
	mandos  []*botonVisor
	vivos   bool // los mandos a plena opacidad: el puntero o el foco está en ellos
	enlaces []*enlaceCaja

	// velo y corre son el fotograma de la transición en curso: la opacidad de la tinta
	// que tapa la diapositiva y su desplazamiento, en fracción de su ancho.
	velo, corre float32
	anim        *fyne.Animation
	r           *presentacionRenderer
	// efectos son las animaciones de las cajas del paso revelado.
	efectos []*fyne.Animation
}

var (
	_ fyne.Widget   = (*Presentacion)(nil)
	_ fyne.Tappable = (*Presentacion)(nil)
)

// NuevaPresentacion monta el visor desde la diapositiva `desde`.
func NuevaPresentacion(m *Modelo, desde int, rotulo func(string, i18nVars) string) *Presentacion {
	p := &Presentacion{M: m, Diapo: min(max(desde, 0), len(m.Diapos)-1), rotulo: rotulo}
	p.lienzo = NuevoLienzo(m, p.Diapo)
	p.lienzo.Variante = tema.Oscuro
	p.lienzo.ConPasos = true
	p.lienzo.Rotulo = func(c string) string { return p.t(c, nil) }
	p.ExtendBaseWidget(p)
	return p
}

func (p *Presentacion) t(clave string, v i18nVars) string {
	if p.rotulo == nil {
		return clave
	}
	return p.rotulo(clave, v)
}

func (p *Presentacion) ultima() int { return len(p.M.Diapos) - 1 }

// Ir fija diapositiva y paso, con topes: el paso se recorta a los de su destino.
func (p *Presentacion) Ir(diapo, paso int) {
	diapo = min(max(diapo, 0), p.ultima())
	paso = min(max(paso, 0), Pasos(p.M.Diapos[diapo]))
	if diapo == p.Diapo && paso == p.Paso {
		return
	}
	dir := 1
	if diapo < p.Diapo {
		dir = -1
	}
	cambia := diapo != p.Diapo
	adelante := diapo > p.Diapo || (!cambia && paso > p.Paso)
	anterior := p.Diapo
	p.Diapo, p.Paso = diapo, paso
	p.lienzo.Saliendo = nil
	if adelante {
		p.lienzo.Saliendo = salidasEn(p.M.Diapos[diapo], paso)
	}
	p.Refresh()
	p.animarPaso(adelante) // antes que la transición: para los efectos que queden, y el morph es uno
	if cambia {
		p.transicionar(dir, anterior)
	}
	if cambia && p.AlCambiarDiapo != nil {
		p.AlCambiarDiapo(diapo)
	}
	if p.AlMover != nil {
		p.AlMover()
	}
}

// transicionar arranca la transición de la diapositiva a la que se llega con `dir` (1
// adelante, −1 atrás). La web remonta el envoltorio con `key={slide.id}` y su animación
// CSS vuelve a correr: al cambiar de diapositiva —no de paso— y también al abrir el
// visor. El morph no mueve la diapositiva entera, solo sus cajas.
func (p *Presentacion) transicionar(dir, desde int) {
	if p.anim != nil {
		p.anim.Stop()
	}
	var corre float32
	switch p.M.Diapos[p.Diapo].Transicion {
	case "morph":
		p.anim = nil
		p.fotograma(1, 0)
		if dir > 0 {
			p.morfear(desde) // solo al AVANZAR, como la web
		}
		return
	case "fade":
	case "slide":
		corre = DesplazaTransicion * float32(dir)
	default:
		p.anim = nil
		p.fotograma(1, 0)
		return
	}
	p.anim = fyne.NewAnimation(DuracionTransicion, func(a float32) { p.fotograma(a, corre) })
	p.anim.Curve = salidaCSS
	p.anim.Start()
}

// curvaCSS es `cubic-bezier(x1, y1, x2, y2)`: la y de cada x, buscando el parámetro por
// bisección (la x de la curva crece con él).
func curvaCSS(x1, y1, x2, y2 float64) fyne.AnimationCurve {
	b := func(t, p1, p2 float64) float64 {
		u := 1 - t
		return 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t
	}
	return func(x float32) float32 {
		lo, hi := 0.0, 1.0
		for range 30 {
			if m := (lo + hi) / 2; b(m, x1, x2) < float64(x) {
				lo = m
			} else {
				hi = m
			}
		}
		return float32(b((lo+hi)/2, y1, y2))
	}
}

// Las curvas de los efectos (`ease-out`, `ease-in`, `ease-in-out`, las de volar y la de
// las trayectorias) y sus medidas, las de las `@keyframes` de SlidesCanvas.tsx.
var (
	salidaCSS       = curvaCSS(0, 0, 0.58, 1)
	entradaCSS      = curvaCSS(0.42, 0, 1, 1)
	vaivenCSS       = curvaCSS(0.42, 0, 0.58, 1)
	curvaVolar      = curvaCSS(0.22, 1, 0.36, 1)
	curvaVolarFuera = curvaCSS(0.4, 0, 1, 0.6)
	curvaTrayecto   = curvaCSS(0.4, 0, 0.2, 1)
	subidaVolar     = float32(26)   // el `translateY(26px)` de volar
	escalaZoom      = float32(0.82) // el `scale(0.82)` del zoom
	duracionFundir  = 320 * time.Millisecond
	duracionVolar   = 380 * time.Millisecond
	duracionZoom    = 300 * time.Millisecond
	duracionSalir   = 300 * time.Millisecond
	duracionLatido  = 620 * time.Millisecond
	duracionCrecer  = 560 * time.Millisecond
	duracionMover   = 640 * time.Millisecond
)

// vaiven es la escala de un énfasis de ida y vuelta (`0%, 100% { scale(1) } 50% {
// scale(tope) }`): la curva `ease-in-out` va en cada mitad, como la aplica CSS a cada
// tramo entre fotogramas clave.
func vaiven(e, tope float32) float32 {
	if e < 0.5 {
		return 1 + (tope-1)*vaivenCSS(2*e)
	}
	return tope - (tope-1)*vaivenCSS(2*e-1)
}

// salidasEn son las cajas con una salida ANIMADA en el paso `paso`: se siguen pintando
// ese paso para que su efecto las lleve hasta invisibles.
func salidasEn(d Diapo, paso int) map[string]bool {
	var fuera map[string]bool
	for _, pl := range Plan(d) {
		if pl.Paso == paso && pl.Categoria == Salida && pl.Anim.Tipo != "disappear" {
			if fuera == nil {
				fuera = map[string]bool{}
			}
			fuera[pl.Caja] = true
		}
	}
	return fuera
}

// animarPaso lanza las animaciones de las cajas que dispara el paso al que se acaba de
// llegar AVANZANDO (al retroceder no se dispara nada: `boxFiringAtStep`). Cada una
// espera su escalón (`delayMs`) mostrando su primer fotograma, como el `fill both`.
func (p *Presentacion) animarPaso(adelante bool) {
	for _, a := range p.efectos {
		a.Stop()
	}
	p.efectos = p.efectos[:0]
	if !adelante || p.Paso == 0 {
		return
	}
	d := p.M.Diapos[p.Diapo]
	k := p.lienzo.escala(p.lienzo.Size())
	for _, pl := range Plan(d) {
		if pl.Paso != p.Paso {
			continue
		}
		dur, curva, f := p.efecto(pl, k)
		if f == nil {
			continue
		}
		retardo := time.Duration(pl.RetardoMS) * time.Millisecond
		total := retardo + dur
		a := fyne.NewAnimation(total, func(t float32) {
			e := float32(0)
			switch pasado := time.Duration(float64(total) * float64(t)); {
			case t >= 1:
				e = 1 // exacto: el reposo, sin el residuo de la bisección
			case pasado > retardo:
				e = curva(float32(pasado-retardo) / float32(dur))
			}
			f(e)
		})
		a.Curve = fyne.AnimationLinear
		p.efectos = append(p.efectos, a)
		a.Start()
	}
}

// efecto es lo que hace un efecto de la caja en cada fotograma (`e` de 0 a 1, ya con su
// curva): entrar fundiendo, volando 26 px desde abajo o con el zoom, o recorrer su
// trayectoria desde donde estaba. nil si no se anima.
func (p *Presentacion) efecto(pl Planeada, k float32) (time.Duration, fyne.AnimationCurve, func(float32)) {
	id, l := pl.Caja, p.lienzo
	quieta := fyne.Delta{}
	switch pl.Anim.Tipo {
	case "fade":
		return duracionFundir, salidaCSS, func(e float32) { l.Efecto(id, e, quieta, 1) }
	case "zoom":
		return duracionZoom, salidaCSS, func(e float32) { l.Efecto(id, e, quieta, escalaZoom+(1-escalaZoom)*e) }
	case "fly":
		return duracionVolar, curvaVolar, func(e float32) {
			l.Efecto(id, e, fyne.Delta{DY: subidaVolar * k * (1 - e)}, 1)
		}
	case "pulse":
		return duracionLatido, fyne.AnimationLinear, func(e float32) { l.Efecto(id, 1, quieta, vaiven(e, 1.06)) }
	case "grow-shrink":
		return duracionCrecer, fyne.AnimationLinear, func(e float32) { l.Efecto(id, 1, quieta, vaiven(e, 1.22)) }
	case "fade-out":
		return duracionSalir, entradaCSS, func(e float32) { l.Efecto(id, 1-e, quieta, 1) }
	case "fly-out":
		return duracionVolar, curvaVolarFuera, func(e float32) {
			l.Efecto(id, 1-e, fyne.Delta{DY: subidaVolar * k * e}, 1)
		}
	case "zoom-out":
		return duracionSalir, entradaCSS, func(e float32) { l.Efecto(id, 1-e, quieta, 1-(1-escalaZoom)*e) }
	}
	if pl.Categoria != Movimiento {
		return 0, nil, nil
	}
	pts := PuntosMovimiento(pl.Anim.Tipo, pl.Anim.Puntos)
	fin := pts[len(pts)-1]
	mulX, mulY := float64(AnchoEscenario*k)/100, float64(p.M.AltoEscenario()*k)/100
	return duracionMover, curvaTrayecto, func(e float32) {
		// La caja está ya en su reposo tras la trayectoria: se la lleva al punto del
		// recorrido que toca, que al final es ese mismo sitio.
		pt := puntoEnRecorrido(pts, mulX, mulY, float64(e))
		l.Efecto(id, 1, fyne.Delta{DX: float32((pt.DX - fin.DX) * mulX), DY: float32((pt.DY - fin.DY) * mulY)}, 1)
	}
}

// puntoEnRecorrido es el `offset-distance`: el punto de la polilínea a la fracción `t`
// de su LONGITUD (en px, que las dos escalas no son iguales).
func puntoEnRecorrido(pts []Punto, mulX, mulY, t float64) Punto {
	largo := func(a, b Punto) float64 { return math.Hypot((b.DX-a.DX)*mulX, (b.DY-a.DY)*mulY) }
	var total float64
	for i := 1; i < len(pts); i++ {
		total += largo(pts[i-1], pts[i])
	}
	falta := t * total
	for i := 1; i < len(pts); i++ {
		l := largo(pts[i-1], pts[i])
		if falta <= l && l > 0 {
			f := falta / l
			return Punto{DX: pts[i-1].DX + f*(pts[i].DX-pts[i-1].DX), DY: pts[i-1].DY + f*(pts[i].DY-pts[i-1].DY)}
		}
		falta -= l
	}
	return pts[len(pts)-1]
}

// fotograma pone la transición en su avance `a` (de 0 a 1): la diapositiva entra desde
// `corre` × su ancho y desde opacidad 0. Sobre la tinta lisa del visor, opacidad `a` es
// lo mismo, píxel a píxel, que un velo de tinta de 1 − a encima, y eso sí lo pinta Fyne.
func (p *Presentacion) fotograma(a, corre float32) {
	p.velo, p.corre = 1-a, corre*(1-a)
	if p.r == nil {
		return
	}
	p.r.Layout(p.Size())
	p.r.velo.FillColor = conAlfa(tinta, p.velo)
	canvas.Refresh(p.r.velo)
	canvas.Refresh(p.lienzo)
}

func conAlfa(c color.NRGBA, a float32) color.NRGBA { return tokens.ConAlfa(c, float64(a)) }

// Avanzar es `advance`: revela el paso siguiente o salta de diapositiva.
func (p *Presentacion) Avanzar(dir int) { p.Ir(Avanzar(p.M.Diapos, p.Diapo, p.Paso, dir)) }

// Tecla es el `onKeyDown` del visor.
func (p *Presentacion) Tecla(k fyne.KeyName) {
	switch k {
	case fyne.KeyRight:
		p.Avanzar(1)
		return
	case fyne.KeyLeft:
		p.Avanzar(-1)
		return
	case fyne.KeySpace:
		return // llega por Runa: atenderla aquí también avanzaría dos pasos
	default:
	}
	switch d := TeclaPresentando(k, p.Diapo, p.ultima()); d {
	case SinDestino:
	case Salir:
		p.salir()
	default:
		p.Ir(d, 0) // un salto aterriza al principio de la diapositiva
	}
}

// Runa atiende lo que Fyne entrega como carácter: el espacio avanza, N alterna las
// notas y P la vista del moderador.
func (p *Presentacion) Runa(r rune) {
	switch r {
	case ' ':
		p.Avanzar(1)
	case 'n', 'N':
		p.Notas = !p.Notas
		p.Refresh()
	case 'p', 'P':
		// Sin quien abra la vista, P no hace nada, en vez de decir «abierta» sin abrir nada.
		if p.AlCambiarModerador == nil {
			return
		}
		p.Moderador = !p.Moderador
		p.AlCambiarModerador(p.Moderador)
		p.Refresh()
	}
}

func (p *Presentacion) salir() {
	if p.AlSalir != nil {
		p.AlSalir()
	}
}

// Tapped es el clic en cualquier sitio del visor: avanza.
func (p *Presentacion) Tapped(*fyne.PointEvent) { p.Avanzar(1) }

// AccessibilityLabel es el `aria-live` del visor: «Diapositiva n de m, paso k de j».
func (p *Presentacion) AccessibilityLabel() string {
	s := p.t("slides.aria.slideOf", i18nVars{"n": p.Diapo + 1, "total": p.ultima() + 1})
	if pasos := Pasos(p.M.Diapos[p.Diapo]); pasos > 0 {
		s += ", " + p.t("slides.present.step", i18nVars{"n": p.Paso, "total": pasos})
	}
	return s
}

// AccessibilityRole es contenedor: el visor agrupa la diapositiva y sus mandos.
func (p *Presentacion) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// tieneNotas es `slide.notes.trim() !== ”`.
func (p *Presentacion) tieneNotas() bool {
	for _, r := range p.M.Diapos[p.Diapo].Notas {
		if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
			return true
		}
	}
	return false
}

// CreateRenderer compone el fondo, la diapositiva, los enlaces, las notas, la pista y
// los mandos.
func (p *Presentacion) CreateRenderer() fyne.WidgetRenderer {
	r := &presentacionRenderer{
		p:          p,
		fondo:      canvas.NewRectangle(tinta),
		velo:       canvas.NewRectangle(color.Transparent),
		papelNotas: canvas.NewRectangle(papel),
		pista:      ui.Texto("", tema.TextoXS, ui.Normal, color.NRGBA{R: 255, G: 255, B: 255, A: 115}),
	}
	p.mandos = []*botonVisor{
		nuevoBotonVisor(p, iconos.EdComentario, p.t("slides.notes", nil), func() { p.Runa('n') }),
		nuevoBotonVisor(p, iconos.EdChevronIzq, p.t("nav.previous", nil), func() { p.Avanzar(-1) }),
		nuevoBotonVisor(p, iconos.EdChevronDer, p.t("nav.nextItem", nil), func() { p.Avanzar(1) }),
		nuevoBotonVisor(p, iconos.EdCerrar, p.t("slides.present.exit", nil), p.salir),
	}
	p.r = r
	r.Refresh()
	p.transicionar(1, p.Diapo)
	return r
}

type presentacionRenderer struct {
	p          *Presentacion
	fondo      *canvas.Rectangle
	velo       *canvas.Rectangle
	papelNotas *canvas.Rectangle
	notas      []*canvas.Text
	pista      *canvas.Text
}

func (r *presentacionRenderer) MinSize() fyne.Size { return fyne.NewSize(1, 1) }
func (r *presentacionRenderer) Destroy()           {}

func (r *presentacionRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.fondo, r.p.lienzo}
	for _, e := range r.p.enlaces {
		objs = append(objs, e)
	}
	objs = append(objs, r.velo)
	if r.verNotas() {
		objs = append(objs, r.papelNotas)
		for _, t := range r.notas {
			objs = append(objs, t)
		}
	}
	objs = append(objs, r.pista)
	for _, b := range r.botones() {
		objs = append(objs, b)
	}
	return objs
}

func (r *presentacionRenderer) verNotas() bool { return r.p.Notas && r.p.tieneNotas() }

// botones son los mandos que se ven: el de notas solo si la diapositiva tiene.
func (r *presentacionRenderer) botones() []*botonVisor {
	if r.p.tieneNotas() {
		return r.p.mandos
	}
	return r.p.mandos[1:]
}

func (r *presentacionRenderer) Layout(t fyne.Size) {
	p := r.p
	ui.FondoHastaElBorde(p, r.fondo, t)
	r.velo.Resize(t)
	esc := p.lienzo.MedidaEscenario(t)
	dx := p.corre * esc.Width
	p.lienzo.Resize(t)
	p.lienzo.Move(fyne.NewPos(dx, 0))

	// Los enlaces de acción son una capa pulsable encima de su caja.
	o := fyne.NewPos((t.Width-esc.Width)/2+dx, (t.Height-esc.Height)/2)
	for _, e := range p.enlaces {
		c := e.caja
		e.Move(o.AddXY(float32(c.X)/100*esc.Width, float32(c.Y)/100*esc.Height))
		e.Resize(fyne.NewSize(float32(c.W)/100*esc.Width, float32(c.H)/100*esc.Height))
	}

	if r.verNotas() {
		r.colocarNotas(t)
	}

	ui.ColocarTexto(r.pista, tema.TextoXS, IzqPistaVisor, t.Height-AbajoPistaVisor-tema.TextoXS.Linea)

	bs := r.botones()
	x := t.Width - MargenBotonesVisor - float32(len(bs))*LadoBotonVisor - float32(len(bs)-1)*HuecoBotonesVisor
	y := t.Height - MargenBotonesVisor - LadoBotonVisor
	for _, b := range bs {
		b.Move(fyne.NewPos(x, y))
		b.Resize(fyne.NewSize(LadoBotonVisor, LadoBotonVisor))
		x += LadoBotonVisor + HuecoBotonesVisor
	}
}

// colocarNotas es la capa de notas: `fixed inset-x-0 bottom-0 max-h-[32%] px-8 pt-4
// pb-16` sobre el papel, con el texto `mx-auto max-w-4xl text-base leading-relaxed
// whitespace-pre-wrap`.
//
// ponytail: `overflow-auto` no está; unas notas que pasen del 32 % de la pantalla se
// cortan. Se releva metiendo las líneas en un desplazador.
func (r *presentacionRenderer) colocarNotas(t fyne.Size) {
	e := tema.Escalon{Tam: tema.TextoBase.Tam, Linea: tema.TextoBase.Tam * 1.625}
	ancho := min(t.Width-2*tema.Espacio(8), 56*tema.Rem) // max-w-4xl = 56rem
	lineas := ui.Lineas(r.p.M.Diapos[r.p.Diapo].Notas, e, ui.Normal, ancho)
	alto := min(tema.Espacio(4)+float32(len(lineas))*e.Linea+tema.Espacio(16), t.Height*0.32)
	r.papelNotas.Resize(fyne.NewSize(t.Width, alto))
	r.papelNotas.Move(fyne.NewPos(0, t.Height-alto))
	x, y := (t.Width-ancho)/2, t.Height-alto+tema.Espacio(4)
	r.notas = r.notas[:0]
	for _, l := range lineas {
		txt := ui.Texto(l, e, ui.Normal, tinta)
		ui.ColocarTexto(txt, e, x, y)
		r.notas = append(r.notas, txt)
		y += e.Linea
	}
}

func (r *presentacionRenderer) Refresh() {
	p := r.p
	p.lienzo.Indice, p.lienzo.Paso = p.Diapo, p.Paso
	p.lienzo.Refresh()

	// Una capa pulsable por caja con enlace de acción PRESENTE en este paso, y por cada
	// audio o vídeo, que se reproduce al pulsarlo (ver `AbrirMedio`).
	p.enlaces = p.enlaces[:0]
	d := p.M.Diapos[p.Diapo]
	por := PorCaja(Plan(d))
	for _, c := range d.Cajas {
		pulsable := len(c.Accion) > 0 || (c.Tipo == "media" && p.AbrirMedio != nil)
		if pulsable && PresenteEn(por[c.ID], p.Paso) {
			p.enlaces = append(p.enlaces, nuevoEnlaceCaja(p, c))
		}
	}

	clave := "slides.present.keys"
	if p.Moderador {
		clave = "slides.present.presenterOpen"
	}
	r.pista.Text = p.t(clave, nil)
	r.pista.Refresh()

	ultimo := p.Diapo == p.ultima() && p.Paso == Pasos(d)
	p.mandos[0].Activo = p.Notas
	p.mandos[1].Inerte = p.Diapo == 0 && p.Paso == 0
	p.mandos[2].Inerte = ultimo
	for _, b := range p.mandos {
		b.Refresh()
	}
	r.Layout(p.Size())
	canvas.Refresh(p)
}

// ---- mandos ----

// botonVisor es PresentButton: 38,5 × 38,5, `rounded-md`, borde blanco al 22 %, fondo
// negro al 45 % (blanco al 24 % pulsado) y el glifo blanco de 16. Frena el clic —si no,
// el visor avanzaría además de lo que haga el botón—.
type botonVisor struct {
	widget.BaseWidget
	p        *Presentacion
	icono    iconos.Ed
	rotulo   string
	alPulsar func()
	Activo   bool
	Inerte   bool
	encima   bool
	enfocado bool
}

var (
	_ fyne.Tappable     = (*botonVisor)(nil)
	_ fyne.Focusable    = (*botonVisor)(nil)
	_ desktop.Hoverable = (*botonVisor)(nil)
)

func nuevoBotonVisor(p *Presentacion, i iconos.Ed, rotulo string, f func()) *botonVisor {
	b := &botonVisor{p: p, icono: i, rotulo: rotulo, alPulsar: f}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonVisor) Tapped(*fyne.PointEvent) {
	if !b.Inerte && b.alPulsar != nil {
		b.alPulsar()
	}
}

// Los mandos se atenúan juntos y vuelven enteros al apuntar a CUALQUIERA de ellos o al
// llegarles el foco (`focus-within:opacity-100 hover:opacity-100` del grupo).
func (b *botonVisor) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.p.avivar() }
func (b *botonVisor) MouseMoved(*desktop.MouseEvent) {}
func (b *botonVisor) MouseOut()                      { b.encima = false; b.p.avivar() }
func (b *botonVisor) FocusGained()                   { b.enfocado = true; b.p.avivar() }
func (b *botonVisor) FocusLost()                     { b.enfocado = false; b.p.avivar() }
func (b *botonVisor) TypedRune(r rune) {
	if r == ' ' {
		b.Tapped(nil)
	}
}

func (b *botonVisor) TypedKey(e *fyne.KeyEvent) {
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		b.Tapped(nil)
		return
	}
	b.p.Tecla(e.Name) // el resto de teclas siguen siendo del visor: Escape sale
}

func (b *botonVisor) AccessibilityLabel() string             { return b.rotulo }
func (b *botonVisor) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (p *Presentacion) avivar() {
	vivos := false
	for _, b := range p.mandos {
		vivos = vivos || b.encima || b.enfocado
	}
	if vivos != p.vivos {
		p.vivos = vivos
		for _, b := range p.mandos {
			b.Refresh()
		}
	}
}

func (b *botonVisor) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(LadoBotonVisor, LadoBotonVisor)
}

func (b *botonVisor) CreateRenderer() fyne.WidgetRenderer {
	r := &botonVisorRenderer{
		b:     b,
		aro:   ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, tema.Oscuro),
		caja:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control")),
		glifo: iconos.TrazoEdConTinta(b.icono, IconoBotonVisor, color.NRGBA{R: 255, G: 255, B: 255, A: 255}),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.glifo.Objeto())
	r.Refresh()
	return r
}

type botonVisorRenderer struct {
	b       *botonVisor
	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *botonVisorRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonVisorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonVisorRenderer) Destroy()                     {}

func (r *botonVisorRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.glifo.Colocar((t.Width-IconoBotonVisor)/2, (t.Height-IconoBotonVisor)/2, IconoBotonVisor)
}

// Refresh pinta los tres colores con la opacidad del grupo encima.
//
// ponytail: la web atenúa el GRUPO ya compuesto (`opacity-35`); aquí se multiplica el
// alfa de cada capa, y donde el borde, el fondo y el glifo se solapan el resultado
// difiere en unos pocos niveles. Fyne no tiene opacidad de subárbol.
func (r *botonVisorRenderer) Refresh() {
	op := float32(1)
	if !r.b.p.vivos {
		op = OpacidadMandos
	}
	if r.b.Inerte {
		op *= 0.3 // `disabled:opacity-30`
	}
	alfa := func(c color.NRGBA) color.NRGBA { c.A = uint8(float32(c.A) * op); return c }
	fondo := color.NRGBA{A: 115} // negro al 45 %
	if r.b.Activo {
		fondo = color.NRGBA{R: 255, G: 255, B: 255, A: 61} // blanco al 24 %
	}
	r.caja.Pintar(alfa(fondo), alfa(color.NRGBA{R: 255, G: 255, B: 255, A: 56})) // blanco al 22 %
	r.glifo.TenirCon(alfa(color.NRGBA{R: 255, G: 255, B: 255, A: 255}))
	r.aro.Pintar(tema.Oscuro)
	r.aro.Ver(r.b.enfocado)
	r.Layout(r.b.Size())
}

// ---- enlaces de acción ----

// enlaceCaja es la capa pulsable de una caja con enlace de acción: el clic navega y NO
// avanza el visor. Es `role="link"` con el destino por nombre.
type enlaceCaja struct {
	widget.BaseWidget
	p    *Presentacion
	caja Caja
}

var (
	_ fyne.Tappable      = (*enlaceCaja)(nil)
	_ desktop.Cursorable = (*enlaceCaja)(nil)
)

func nuevoEnlaceCaja(p *Presentacion, c Caja) *enlaceCaja {
	e := &enlaceCaja{p: p, caja: c}
	e.ExtendBaseWidget(e)
	return e
}

func (e *enlaceCaja) Tapped(*fyne.PointEvent) {
	if len(e.caja.Accion) == 0 && e.caja.Tipo == "media" {
		e.p.AbrirMedio(e.caja)
		return
	}
	if d := DestinoAccion(e.caja.Accion, e.p.Diapo, e.p.ultima()); d != SinDestino {
		e.p.Ir(d, 0)
	}
}

// Cursor es el `cursor: pointer` de la caja con enlace.
func (e *enlaceCaja) Cursor() desktop.Cursor { return desktop.PointerCursor }

// AccessibilityLabel es `actionLabel`: el destino dicho con palabras.
func (e *enlaceCaja) AccessibilityLabel() string {
	var s string
	if json.Unmarshal(e.caja.Accion, &s) == nil {
		switch s {
		case "first":
			return e.p.t("slides.link.goFirst", nil)
		case "last":
			return e.p.t("slides.link.goLast", nil)
		case "next":
			return e.p.t("slides.link.goNext", nil)
		case "prev":
			return e.p.t("slides.link.goPrev", nil)
		}
	}
	var n int
	_ = json.Unmarshal(e.caja.Accion, &n)
	return e.p.t("slides.link.goTo", i18nVars{"n": n})
}

func (e *enlaceCaja) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleLink }

func (e *enlaceCaja) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.NRGBA{}))
}

// ---- abrirla en una ventana ----

// Presentar abre la presentación en una ventana a pantalla completa, desde la
// diapositiva `desde`. `moderador` abre además la vista del moderador.
//
// El pantalla completa es del TOOLKIT y no de la plataforma: `SetFullScreen` lo
// resuelve en los cinco destinos. Va en una ventana PROPIA y no encima del editor: la
// web la pone encima porque en un navegador no hay otra; aquí, cerrarla devuelve el
// foco a la ventana del editor, que sigue donde estaba.
func Presentar(m *Modelo, desde int, moderador bool, idioma i18n.Locale,
	rotulo func(string, i18nVars) string,
) *Presentacion {
	a := fyne.CurrentApp()
	w := a.NewWindow(rotulo("slides.present", nil))
	w.SetPadded(false)
	p := NuevaPresentacion(m, desde, rotulo)
	p.AlSalir = func() { w.Close() }
	p.AbrirMedio = AbrirMedioDelSistema
	teclado(w, p)
	w.SetContent(p)
	w.SetFullScreen(true)
	if HayModerador() {
		p.AlCambiarModerador = ventanaModerador(p, idioma, rotulo)
		w.SetOnClosed(func() {
			if p.Moderador {
				p.AlCambiarModerador(false)
			}
		})
	} else {
		p.AlCambiarModerador = ModeradorEnLaMisma(w, p, idioma)
	}
	w.Show()
	if moderador {
		p.Runa('p')
	}
	return p
}

// HayModerador dice si la vista del moderador va en una segunda ventana. En un teléfono
// o una tableta no la hay: va en la misma, como la otra pestaña de la web (ver
// ModeradorEnLaMisma).
func HayModerador() bool { return !ui.EsTactil() }

// ModeradorEnLaMisma es la vista del moderador donde no hay segunda ventana: la misma
// ventana pasa de la presentación a la vista, como se pasa de pestaña en el navegador
// del teléfono, y vuelve tocando la diapositiva actual o con la P.
func ModeradorEnLaMisma(w fyne.Window, p *Presentacion, idioma i18n.Locale) func(bool) {
	var (
		parar func()
		mover func()
	)
	return func(abierta bool) {
		if !abierta {
			if parar != nil {
				parar()
				parar, p.AlMover = nil, mover
			}
			w.SetContent(p)
			return
		}
		if parar != nil {
			return
		}
		mod := NuevoModerador(p, idioma)
		mod.AlVolver = func() { p.Runa('p') }
		mover = p.AlMover
		p.AlMover = func() {
			mod.Refresh()
			if mover != nil {
				mover()
			}
		}
		w.SetContent(mod)
		parar = cadaSegundo(mod.Refresh)
	}
}

func teclado(w fyne.Window, p *Presentacion) {
	w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) { p.Tecla(e.Name) })
	w.Canvas().SetOnTypedRune(p.Runa)
	ui.AlAtras(w.Canvas(), func() bool {
		if p.Moderador {
			p.Runa('p')
		} else {
			p.Tecla(fyne.KeyEscape)
		}
		return true
	})
}

// ventanaModerador devuelve lo que abre y cierra la segunda ventana: la del moderador,
// de 1040 × 720, con el MISMO teclado que el visor y un reloj que la repinta cada
// segundo mientras está abierta. Cerrarla a mano apaga el moderador, como el
// `beforeunload` de la web.
func ventanaModerador(p *Presentacion, idioma i18n.Locale, rotulo func(string, i18nVars) string) func(bool) {
	var (
		wm    fyne.Window
		parar func()
	)
	return func(abierta bool) {
		if !abierta {
			if wm != nil {
				wm.Close()
			}
			return
		}
		if wm != nil {
			return
		}
		mod := NuevoModerador(p, idioma)
		wm = fyne.CurrentApp().NewWindow(rotulo("slides.presenter.title", nil))
		wm.SetPadded(false)
		wm.SetContent(mod)
		wm.Resize(fyne.NewSize(AnchoModerador, AltoModerador))
		teclado(wm, p)
		mover := p.AlMover
		p.AlMover = func() {
			mod.Refresh()
			if mover != nil {
				mover()
			}
		}
		parar = cadaSegundo(mod.Refresh)
		wm.SetOnClosed(func() {
			parar()
			wm, p.AlMover = nil, mover
			if p.Moderador {
				p.Moderador = false
				p.Refresh()
			}
		})
		wm.Show()
	}
}
