// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Popover.tsx

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Emergente es el panel flotante que cuelga de su disparador: `POPOVER_SHELL` de
// `editor/ribbon/primitives.tsx`, la carcasa que la web escribía a mano en 29 sitios
// con las mismas clases —
//
//	border-border bg-menu-surface absolute top-full mt-1 rounded-lg border
//	z-[var(--z-popover)] shadow-[var(--shadow-md)]
//
// No es `Menu`, y por dos medidas: `Menu` es Dropdown.tsx, con `min-w-[220px]`, `p-1`
// y el panel en `menu-surface`; la carcasa del editor no tiene ancho mínimo —cada
// sitio pone el suyo, `w-max` en la paleta de color y `w-56` en «Organizar»—, su
// relleno lo decide quien la usa (`p-1` en los menús de filas, `p-2` en los paneles) y
// va también en `menu-surface`. Lo que sí comparten es la capa: el mismo portal, el mismo velo
// transparente que recoge el clic de fuera y Escape.
//
// Y con `MostrarPopover` es el OTRO panel flotante de la web, `Popover.tsx`: en
// `menu-surface`, con `p-4`, a 6 del disparador, que se VOLTEA hacia arriba si no cabe
// debajo y cambia de borde de anclaje si se sale por la derecha, y que se lleva el foco
// (el primer control, o el propio panel) y lo devuelve al cerrar con Escape.
type Emergente struct {
	widget.BaseWidget
	sinParada

	contenido fyne.CanvasObject
	relleno   float32
	caja      *Caja
	capa      *capa
	previo    fyne.Focusable
	// AlRecogerse AVISA de que se ha recogido, lo cierre quien lo cierre. No se llama
	// `AlCerrar` a propósito: en `Dialogo`, `AlCerrar` es lo contrario —la PETICIÓN de
	// cierre, que decide si cierra—, y con el mismo nombre Rooms escribió dos diálogos
	// con un aviso donde tocaba cerrar, y ni el aspa, ni Escape, ni el velo los cerraban.
	AlRecogerse func()
	// Rotulo es el nombre accesible del panel, YA traducido: el `aria-label` del
	// `role="dialog"` de Popover.tsx.
	Rotulo string
	// Solo Popover.tsx: cuando su contenido supera el área segura se desplaza
	// dentro del panel. Los emergentes con colocación propia conservan su contrato.
	desplazable bool
	// aSangre es el panel de borde a borde de MostrarEmergenteASangre: sin esquinas.
	aSangre     bool
	recortado   bool
	desplaza    *container.Scroll
	fluido      *fyne.Container
	anchoScroll float32
}

// AccessibilityLabel es el `aria-label` del panel.
func (e *Emergente) AccessibilityLabel() string { return e.Rotulo }

// AccessibilityRole: es un panel que agrupa (Fyne no tiene el rol `dialog`).
func (e *Emergente) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

var (
	_ fyne.Widget      = (*Emergente)(nil)
	_ fyne.Focusable   = (*Emergente)(nil)
	_ fyne.Disableable = (*Emergente)(nil)
	_ fyne.Accessible  = (*Emergente)(nil)
)

// Las medidas de Popover.tsx.
var (
	// SeparacionPopover es el `GAP = 6` entre el disparador y el panel.
	SeparacionPopover float32 = 6
	// RellenoPopover es su `p-4`.
	RellenoPopover = tema.Espacio(4)
)

// ColocarPopover es la colocación de Popover.tsx: debajo y alineado a la izquierda del
// disparador; ARRIBA si debajo no cabe y arriba sí; alineado a la DERECHA si a la
// izquierda se sale por el borde derecho y a la derecha no se sale por el izquierdo. Y
// luego el empujón de `useFitInViewport`, que lo deja a 8 del borde, y el mismo tope
// por arriba y por abajo, porque aquí el documento no se desplaza.
func ColocarPopover(disparador fyne.Position, tamDisparador, panel, ventana fyne.Size) fyne.Position {
	abajo := disparador.Y + tamDisparador.Height + SeparacionPopover
	y := abajo
	cabeAbajo := abajo+panel.Height <= ventana.Height
	cabeArriba := disparador.Y-SeparacionPopover-panel.Height >= 0
	if !cabeAbajo && cabeArriba {
		y = disparador.Y - SeparacionPopover - panel.Height
	}
	x := disparador.X
	seSaleDerecha := disparador.X+panel.Width > ventana.Width
	seSaleIzquierda := disparador.X+tamDisparador.Width-panel.Width < 0
	if seSaleDerecha && !seSaleIzquierda {
		x = disparador.X + tamDisparador.Width - panel.Width // `right-0`
	}
	tope := func(v, lado, total float32) float32 {
		return max(MargenVentana, min(v, total-lado-MargenVentana))
	}
	return fyne.NewPos(tope(x, panel.Width, ventana.Width), tope(y, panel.Height, ventana.Height))
}

// MostrarPopover despliega `contenido` como Popover.tsx. Ver el tipo.
func MostrarPopover(disparador, contenido fyne.CanvasObject) *Emergente {
	e := &Emergente{contenido: contenido, relleno: RellenoPopover, desplazable: true}
	e.ExtendBaseWidget(e)
	c := lienzoDe(disparador)
	if c == nil {
		return e
	}
	// El clic de fuera cierra SIN devolver el foco al disparador: se lo robaría a
	// donde la persona acaba de pulsar (`restoreRef = false` de la web).
	e.capa = nuevaCapa(nuevoVeloCruzado(func() { e.cerrar(false) }), e, func(ventana fyne.Size) (fyne.Position, fyne.Size) {
		tam := MedidaEmergente(contenido, e.relleno)
		tam.Width = max(min(tam.Width, ventana.Width-2*MargenVentana), 0)
		interior := max(tam.Width-2*(tema.Borde+e.relleno), 0)
		tam.Height = min(AltoA(contenido, interior)+2*(tema.Borde+e.relleno), max(ventana.Height-2*MargenVentana, 0))
		abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(disparador)
		return ColocarPopover(abs, disparador.Size(), tam, ventana), tam
	})
	e.capa.sombra = NuevaSombra(tema.SombraMD, tema.RadioLG)
	e.capa.alDescartar = e.Cerrar
	e.previo = mostrar(c, e.capa)
	if f := primerEnfocable(contenido); f != nil {
		enfocar(c, f)
	} else {
		e.dar(c, e)
	}
	// `sytue-pop-in` de Popover.tsx, con `origin-top` si cuelga hacia abajo y
	// `origin-bottom` si se ha volcado encima del disparador.
	ancla := float32(0)
	if e.Position().Y < fyne.CurrentApp().Driver().AbsolutePositionForObject(disparador).Y {
		ancla = 1
	}
	e.capa.entrar(c, DuracionPop, -DesplazamientoPop, EscalaEntrada, ancla)
	return e
}

// MostrarEmergenteEn es la misma capa para un panel que se coloca a su manera: `coloca`
// recibe el tamaño de la ventana y el que pide el panel, y dice dónde va y cuánto mide.
// Va en `menu-surface`, como Popover.tsx, porque sus dos usos son de esa familia: la
// vista rápida de un evento de Sytue Calendar (EventPopover.tsx), que se pega al evento
// y se voltea encima si no cabe debajo, y su centro de recordatorios, pegado al borde
// DERECHO de la campana.
func MostrarEmergenteEn(c fyne.Canvas, contenido fyne.CanvasObject, relleno float32,
	coloca func(ventana, tam fyne.Size) (fyne.Position, fyne.Size),
) *Emergente {
	return mostrarEmergenteEn(c, &Emergente{contenido: contenido, relleno: relleno}, coloca)
}

// MostrarEmergenteASangre es MostrarEmergenteEn para un panel que va de borde a borde de la
// ventana: la banda de la cinta desplegada con «Más» (`fixed inset-x-0 border-b` en
// Ribbon.tsx). Sin esquinas redondeadas, que contra el borde de la pantalla no redondean
// nada, y con el contenido desplazable si no cabe en el alto que `coloca` le da.
func MostrarEmergenteASangre(c fyne.Canvas, contenido fyne.CanvasObject,
	coloca func(ventana, tam fyne.Size) (fyne.Position, fyne.Size),
) *Emergente {
	return mostrarEmergenteEn(c, &Emergente{contenido: contenido, aSangre: true, desplazable: true}, coloca)
}

func mostrarEmergenteEn(c fyne.Canvas, e *Emergente, coloca func(ventana, tam fyne.Size) (fyne.Position, fyne.Size)) *Emergente {
	e.ExtendBaseWidget(e)
	if c == nil {
		return e
	}
	// El clic de fuera cierra sin comerse el clic y sin devolver el foco, como el
	// Popover.
	e.capa = nuevaCapa(nuevoVeloCruzado(func() { e.cerrar(false) }), e, func(ventana fyne.Size) (fyne.Position, fyne.Size) {
		return coloca(ventana, MedidaEmergente(e.contenido, e.relleno))
	})
	e.capa.sombra = NuevaSombra(tema.SombraMD, e.radio())
	e.capa.alDescartar = e.Cerrar
	e.previo = mostrar(c, e.capa)
	return e
}

// radio es el `rounded-lg` de la carcasa, salvo a sangre.
func (e *Emergente) radio() float32 {
	if e.aSangre {
		return 0
	}
	return tema.RadioLG
}

// primerEnfocable es el primer control del contenido, bajando por los contenedores y
// deteniéndose en cada widget, como `querySelector(FOCUSABLE)`.
func primerEnfocable(o fyne.CanvasObject) fyne.Focusable {
	switch v := o.(type) {
	case nil:
		return nil
	case fyne.Focusable:
		if d, es := o.(fyne.Disableable); es && d.Disabled() {
			return nil
		}
		return v
	case *fyne.Container:
		for _, h := range v.Objects {
			if f := primerEnfocable(h); f != nil {
				return f
			}
		}
	}
	return nil
}

// SeparacionEmergente es el `mt-1` entre el disparador y el panel.
var SeparacionEmergente = tema.Espacio(1)

// MedidaEmergente es lo que ocupa: el contenido, su relleno y el borde.
func MedidaEmergente(contenido fyne.CanvasObject, relleno float32) fyne.Size {
	m := contenido.MinSize()
	lado := 2 * (tema.Borde + relleno)
	return fyne.NewSize(m.Width+lado, m.Height+lado)
}

// ColocarEmergente es `absolute top-full left-0 mt-1`: debajo del disparador y alineado
// con su borde izquierdo. La web no lo vuelca hacia arriba nunca; lo único que se le
// añade es el tope de 8 px contra el borde de la ventana que ya tiene el menú, porque
// en la web el documento se desplaza y aquí un panel que se sale no se alcanza.
func ColocarEmergente(disparador fyne.Position, tamDisparador, panel, ventana fyne.Size) fyne.Position {
	tope := func(v, lado, total float32) float32 {
		return max(MargenVentana, min(v, total-lado-MargenVentana))
	}
	return fyne.NewPos(
		tope(disparador.X, panel.Width, ventana.Width),
		tope(disparador.Y+tamDisparador.Height+SeparacionEmergente, panel.Height, ventana.Height),
	)
}

// MostrarEmergente despliega `contenido` bajo `disparador`, que tiene que estar ya
// montado. `relleno` es el `p-*` del panel.
func MostrarEmergente(disparador, contenido fyne.CanvasObject, relleno float32) *Emergente {
	e := &Emergente{contenido: contenido, relleno: relleno}
	e.ExtendBaseWidget(e)
	c := lienzoDe(disparador)
	if c == nil {
		return e
	}
	e.capa = nuevaCapa(nuevoVeloCruzado(e.Cerrar), e, func(ventana fyne.Size) (fyne.Position, fyne.Size) {
		tam := MedidaEmergente(contenido, relleno)
		abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(disparador)
		return ColocarEmergente(abs, disparador.Size(), tam, ventana), tam
	})
	e.capa.sombra = NuevaSombra(tema.SombraMD, tema.RadioLG)
	e.capa.alDescartar = e.Cerrar
	e.previo = mostrar(c, e.capa)
	return e
}

// Recolocar vuelve a medir y a colocar el panel abierto cuando su contenido cambia de
// alto —las opciones que llegan al escribir—, como `Dialogo.Recolocar`.
func (e *Emergente) Recolocar() {
	if e.capa != nil {
		e.capa.Refresh()
	}
}

// Abierto dice si el panel está puesto.
func (e *Emergente) Abierto() bool { return e.capa != nil }

// Cerrar lo recoge y devuelve el foco a donde estaba.
func (e *Emergente) Cerrar() { e.cerrar(true) }

func (e *Emergente) cerrar(devolverFoco bool) {
	if e.capa == nil {
		return
	}
	previo := e.previo
	if !devolverFoco {
		previo = nil
	}
	ocultar(lienzoDe(e.capa), e.capa, previo)
	e.capa, e.previo = nil, nil
	if e.AlRecogerse != nil {
		e.AlRecogerse()
	}
}

// Tapped se traga el clic que cae en el panel y no en un control, para que el velo de
// debajo no lo recoja y lo cierre.
func (e *Emergente) Tapped(*fyne.PointEvent) {}

// FocusGained y FocusLost no pintan nada: el panel se enfoca para oír Escape.
func (e *Emergente) FocusGained() {}

// FocusLost no hace nada.
func (e *Emergente) FocusLost() {}

// TypedRune no escribe.
func (e *Emergente) TypedRune(rune) {}

// TypedKey cierra con Escape y devuelve el foco al disparador.
func (e *Emergente) TypedKey(k *fyne.KeyEvent) {
	if EsCierre(k) {
		e.Cerrar()
	}
}

// MinSize es MedidaEmergente.
func (e *Emergente) MinSize() fyne.Size {
	e.ExtendBaseWidget(e)
	return MedidaEmergente(e.contenido, e.relleno)
}

// CreateRenderer compone la caja con borde y el contenido dentro.
func (e *Emergente) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e.caja = NuevaCaja(tema.Color(e.fondo(), v), tema.Color("border", v), e.radio())
	r := &emergenteRenderer{e: e}
	r.Refresh()
	return r
}

type emergenteRenderer struct{ e *Emergente }

func (r *emergenteRenderer) IsClip() {}

func (r *emergenteRenderer) MinSize() fyne.Size { return r.e.MinSize() }

func (r *emergenteRenderer) Objects() []fyne.CanvasObject {
	contenido := r.e.contenido
	if r.e.recortado {
		contenido = r.e.desplaza
	}
	return append(append([]fyne.CanvasObject{}, r.e.caja.Objetos()...), contenido)
}

func (r *emergenteRenderer) Destroy() {}

func (r *emergenteRenderer) Layout(t fyne.Size) {
	r.e.caja.Redimensionar(t)
	d := tema.Borde + r.e.relleno
	hueco := fyne.NewSize(max(t.Width-2*d, 0), max(t.Height-2*d, 0))
	e := r.e
	_, yaDesplaza := e.contenido.(*container.Scroll)
	e.recortado = e.desplazable && !yaDesplaza && AltoA(e.contenido, hueco.Width) > hueco.Height
	contenido := e.contenido
	if e.recortado {
		if e.fluido == nil || e.anchoScroll != hueco.Width {
			e.anchoScroll = hueco.Width
			e.fluido = AlAnchoFluido(hueco.Width, e.contenido)
			if e.desplaza == nil {
				e.desplaza = container.NewVScroll(e.fluido)
			} else {
				e.desplaza.Content = e.fluido
			}
		}
		contenido = e.desplaza
	}
	contenido.Move(fyne.NewPos(d, d))
	contenido.Resize(hueco)
	if e.recortado {
		e.desplaza.Refresh()
	}
}

func (r *emergenteRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.e.caja.Pintar(tema.Color(r.e.fondo(), v), tema.Color("border", v))
	r.Layout(r.e.Size())
	if r.e.recortado {
		r.e.desplaza.Refresh()
	} else {
		r.e.contenido.Refresh()
	}
}

// fondo conserva una superficie opaca para leer las opciones sobre cualquier contenido.
func (e *Emergente) fondo() string {
	if e.aSangre {
		return "surface" // la banda desplegada es la cinta misma, no un menú (`bg-surface`)
	}
	return "menu-surface"
}

// Flotante es la capa NO modal de un panel que YA trae su caja: la lista abierta de un
// `Selector` o el panel del selector de relación del ERP. Sustituye a `widget.NewPopUp`
// con la misma forma de usarse (`NuevoFlotante`, `Resize`, `ShowAtPosition`, `Hide`), y
// por dos motivos medidos:
//
//   - La sombra. La del `PopUp` de Fyne es `canvas.Rectangle.Shadow`, que cambia de
//     tamaño con el DPI y que cada pintor entiende a su manera (`docs/CONVENTIONS.md`
//     §13.6), y sale del `ColorNameShadow` del tema, que la suite apaga. Aquí es la
//     `ui.Sombra` de `shadow-md`, la del panel de Dropdown.tsx que estas listas copian.
//   - El clic de fuera, que cierra sin comerse el clic: el mismo velo que un menú.
type Flotante struct {
	c        fyne.Canvas
	panel    fyne.CanvasObject
	capa     *capa
	pos      fyne.Position
	tam      fyne.Size
	previo   fyne.Focusable
	alCerrar func()
}

// NuevoFlotante prepara el panel sobre el lienzo, sin enseñarlo todavía.
func NuevoFlotante(panel fyne.CanvasObject, c fyne.Canvas) *Flotante {
	return &Flotante{c: c, panel: panel}
}

// AlCerrar avisa cuando el panel se recoge por su cuenta: el clic de fuera o Escape.
func (f *Flotante) AlCerrar(g func()) { f.alCerrar = g }

// Resize fija el tamaño del panel; si ya está a la vista, lo recoloca.
func (f *Flotante) Resize(t fyne.Size) {
	f.tam = t
	if f.capa != nil {
		f.capa.Refresh()
	}
}

// ShowAtPosition lo enseña con su esquina en `p`, en coordenadas de la ventana.
func (f *Flotante) ShowAtPosition(p fyne.Position) {
	f.pos = p
	if f.capa != nil || f.c == nil {
		return
	}
	cerrar := func() {
		f.Hide()
		if f.alCerrar != nil {
			f.alCerrar()
		}
	}
	f.capa = nuevaCapa(nuevoVeloCruzado(cerrar), f.panel, func(fyne.Size) (fyne.Position, fyne.Size) {
		return f.pos, f.tam
	})
	f.capa.sombra = NuevaSombra(tema.SombraMD, tema.RadioLG)
	f.capa.alDescartar = cerrar
	f.previo = mostrar(f.c, f.capa)
}

// Hide lo recoge y devuelve el foco a donde estaba.
func (f *Flotante) Hide() {
	if f.capa == nil {
		return
	}
	ocultar(f.c, f.capa, f.previo)
	f.capa, f.previo = nil, nil
}

// Visible dice si está a la vista.
func (f *Flotante) Visible() bool { return f.capa != nil }
