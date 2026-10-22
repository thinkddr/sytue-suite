// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Skeleton.tsx

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Esqueleto es `Skeleton.tsx`: el bloque que reserva el sitio de lo que está cargando
// para que al llegar los datos no salte el layout.
//
// # Por qué está aquí y no en cada app
//
// Porque ya estaba en SEIS sitios el 2026-09-22 —`apps/hub` (`Esqueleto` y
// `EsqueletoRedondo`), `apps/drive`, `apps/mail`, `agenda.Bloque` y el de la rejilla de
// Calendar— y ya habían divergido: cinco lo redondeaban con `RadioMD` (`rounded-md`,
// que es lo que pide la hoja) y `agenda.Bloque` con `RadioSM`. Es el caso literal de
// `docs/CONVENTIONS.md` §13.2.
//
// # Por qué es un widget y no un `canvas.Rectangle` con su color
//
// Por el PULSO, que es la mitad de lo que un esqueleto comunica: `.sytue-skeleton`
// anima la opacidad entre 1 y 0,55 cada 1600 ms, y sin él la pantalla parece congelada
// y no cargando. Una animación infinita necesita dónde pararse cuando el bloque
// desaparece, y eso es `WidgetRenderer.Destroy`: un `canvas.Rectangle` suelto se lo
// lleva el recolector con la animación todavía repintándolo.
//
// ponytail: una animación POR BLOQUE. Fyne las tica todas desde una sola goroutina, así
// que nueve filas de Drive son nueve cierres en la lista del animador, no nueve hilos;
// lo que cuesta de verdad es un `Refresh` por bloque y fotograma mientras se carga. Si
// alguna vez hay una pantalla con cientos, lo que toca es un latido de paquete —una
// animación y un registro de bloques vivos—, no quitar el pulso.
type Esqueleto struct {
	widget.BaseWidget

	// Ancho es el `width`. Cero es el `width: 100%` por defecto de Skeleton.tsx: el
	// bloque no pide nada y se queda con lo que le dé su contenedor.
	Ancho float32
	// Alto es el `height`, que por defecto es `1rem`.
	Alto float32
	// Redondo es el `circle`: `rounded-full` en vez de `rounded-md`.
	Redondo bool
	// Radio es la clase `rounded-*` que quien lo usa pone ENCIMA de la de serie: las
	// teselas de carga de la portada de Docus son `rounded-lg`. Cero es `rounded-md`.
	Radio float32
	// Tinte es el token del relleno. Vacío es `bg-subtle`, que es el de la hoja; sobre
	// el cromo oscuro del menú la web pide `sidebar-hover-bg`, porque ahí un gris claro
	// sería un destello.
	Tinte string

	mu   sync.Mutex
	fase float32
	anim *fyne.Animation
}

var (
	_ fyne.Widget     = (*Esqueleto)(nil)
	_ fyne.Accessible = (*Esqueleto)(nil)
)

// AltoEsqueleto es el `height = '1rem'` por defecto de Skeleton.tsx. Con el rem de 14
// son 14, no 16.
const AltoEsqueleto = tema.Rem

// DuracionPulso es el `animation: sytue-skeleton-pulse 1600ms var(--ease-standard)
// infinite` de la hoja.
const DuracionPulso = 1600 * time.Millisecond

// OpacidadPulso es el `opacity: 0.55` del fotograma del 50 %. Los del 0 % y el 100 %
// son opacos.
const OpacidadPulso float32 = 0.55

// NuevoEsqueleto crea el bloque. Con `alto` a cero se usa el `1rem` de la web.
func NuevoEsqueleto(ancho, alto float32) *Esqueleto {
	e := &Esqueleto{Ancho: ancho, Alto: alto}
	e.ExtendBaseWidget(e)
	return e
}

// NuevoEsqueletoRedondo es el `<Skeleton circle>`: el punto que precede a una fila.
func NuevoEsqueletoRedondo(lado float32) *Esqueleto {
	e := NuevoEsqueleto(lado, lado)
	e.Redondo = true
	return e
}

// SobreCromo tiñe el bloque para el carril oscuro y lo devuelve, para encadenarlo.
func (e *Esqueleto) SobreCromo() *Esqueleto {
	e.Tinte = "sidebar-hover-bg"
	return e
}

// AccessibilityLabel es vacío: en la web el bloque es `aria-hidden="true"` y quien
// anuncia la carga es el contenedor con su `aria-busy`.
func (e *Esqueleto) AccessibilityLabel() string { return "" }

// AccessibilityRole: no es un control; agrupa tan poco como un `<div>` decorativo.
func (e *Esqueleto) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaEsqueleto es lo que ocupa: lo que se le pidió, con el `1rem` de alto por
// defecto. Es la única cuenta.
func MedidaEsqueleto(e *Esqueleto) fyne.Size {
	alto := e.Alto
	if alto <= 0 {
		alto = AltoEsqueleto
	}
	return fyne.NewSize(e.Ancho, alto)
}

// opacidadPulso es el fotograma de `sytue-skeleton-pulse` en el instante `p` del ciclo.
//
// La curva se aplica a CADA TRAMO entre fotogramas, que es lo que hace el navegador con
// un `animation-timing-function`: no es una curva sobre el ciclo entero. Por eso la
// animación de Fyne va con `AnimationLinear` y el suavizado se hace aquí.
func opacidadPulso(p float32) float32 {
	if p > 0.5 {
		return OpacidadPulso + (1-OpacidadPulso)*CurvaEstandar((p-0.5)*2)
	}
	return 1 - (1-OpacidadPulso)*CurvaEstandar(p*2)
}

// tinta es el relleno con la opacidad del instante.
//
// El alfa se aplica TAL CUAL y no se aplana contra `surface` como hace `atenuarCon`:
// aquí no hay opacidad de grupo que emular —es un rectángulo suelto— y el esqueleto se
// pinta lo mismo sobre `bg` que sobre una tarjeta, así que quien tiene que componer es
// el pintor, con lo que de verdad haya debajo.
func (e *Esqueleto) tinta() color.NRGBA {
	tinte := e.Tinte
	if tinte == "" {
		tinte = "bg-subtle"
	}
	c := tema.Color(tinte, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	e.mu.Lock()
	o := opacidadPulso(e.fase)
	e.mu.Unlock()
	if o >= 1 {
		return c
	}
	c.A = uint8(float32(c.A) * o)
	return c
}

// CreateRenderer construye el bloque y arranca el pulso.
func (e *Esqueleto) CreateRenderer() fyne.WidgetRenderer {
	r := &esqueletoRenderer{e: e, bloque: canvas.NewRectangle(color.NRGBA{})}
	e.arrancar(r.bloque)
	// Regla 7 del paquete: el color lo pone `Refresh` y Fyne no lo llama antes del
	// primer pintado. Sin esta línea el bloque nace transparente.
	r.Refresh()
	return r
}

// arrancar lanza el pulso. Con `SinMovimiento` el bloque se queda opaco: sigue diciendo
// «esto está cargando» con la forma, que es lo que hace la hoja con
// `prefers-reduced-motion`.
func (e *Esqueleto) arrancar(bloque *canvas.Rectangle) {
	if SinMovimiento {
		return
	}
	a := fyne.NewAnimation(DuracionPulso, func(p float32) {
		e.mu.Lock()
		e.fase = p
		e.mu.Unlock()
		bloque.FillColor = e.tinta()
		bloque.Refresh()
	})
	a.Curve = fyne.AnimationLinear
	a.RepeatCount = fyne.AnimationRepeatForever
	e.mu.Lock()
	e.anim = a
	e.mu.Unlock()
	a.Start()
}

// Parar detiene el pulso. Lo llama el renderer al destruirse.
func (e *Esqueleto) Parar() {
	e.mu.Lock()
	a := e.anim
	e.anim = nil
	e.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

type esqueletoRenderer struct {
	e      *Esqueleto
	bloque *canvas.Rectangle
}

func (r *esqueletoRenderer) MinSize() fyne.Size           { return MedidaEsqueleto(r.e) }
func (r *esqueletoRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bloque} }
func (r *esqueletoRenderer) Destroy()                     { r.e.Parar() }
func (r *esqueletoRenderer) Layout(t fyne.Size)           { r.bloque.Resize(t) }

func (r *esqueletoRenderer) Refresh() {
	radio := tema.RadioMD
	if r.e.Radio > 0 {
		radio = r.e.Radio
	}
	if r.e.Redondo {
		radio = min(r.e.Ancho, MedidaEsqueleto(r.e).Height) / 2
	}
	r.bloque.CornerRadius = radio
	r.bloque.FillColor = r.e.tinta()
	r.bloque.Refresh()
}
