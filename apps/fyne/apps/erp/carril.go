// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El carril de iconos (`.erp-rail` de ErpApp.tsx y de app.css)
//
// La navegación compacta del ERP en ancho: una columna con un icono por destino y, abajo,
// el buscador y Sytue AI. El menú de módulos entero va en el cajón que abre su primer
// botón. Por debajo de 768 no existe (`display: none`): ahí manda el botón de `main`.
//
// Medido en un navegador con las hojas de main, en las dos formas (2026-10-05):
//
//	nav      72 de ancho, `padding-block` 24, hueco 10
//	acción   44 × 44 en x 14; la primera en y 24 y las de abajo, pegadas al fondo
//	icono    20, centrado
//	cuenta   18 × 18, 4,8 por fuera de la esquina de arriba a la derecha; la cifra, de
//	         11/600, centrada a lo ancho y con su caja de línea (16,5) ARRIBA
//
// El radio y los estados son tokens de FORMA (`--radius-nav` y `--sytue-rail-*`, de
// appearance.css). En Marco el carril es parte del marco: icono sin fondo en reposo y, en
// el activo, un velo y una BARRA ámbar a plena opacidad en el borde del carril
// (`--sytue-rail-indicator`, 3 px), que es lo que dice «estás aquí» donde no hay rótulo
// que poner en negrita: el relleno del icono se pinta al 34 % y no llega a 3:1, y hay
// iconos sin formas cerradas. En Carril, cada icono sobre su velo y el activo en un
// cuadro macizo con el icono del color, sin barra.
const (
	// AnchoCarril es el `width: 4.5rem` del `<nav>` y CarrilPY su `padding-block: 1.5rem`.
	AnchoCarril float32 = 4.5 * tema.Rem
	CarrilPY    float32 = 1.5 * tema.Rem
	// HuecoCarril es el `gap: 0.625rem` entre acciones, arriba y abajo.
	HuecoCarril float32 = 0.625 * tema.Rem
	// LadoAccionCarril es el `width: 2.75rem; min-height: 2.75rem` de una acción.
	LadoAccionCarril float32 = 2.75 * tema.Rem
	// LadoIconoCarril es el `size={20}` de sus iconos.
	LadoIconoCarril float32 = 20
	// LadoCuentaCarril es el `min-width: 1.125rem; height: 1.125rem` de la cuenta,
	// FueraCuentaCarril su `inset: -0.3rem` y CuentaCarrilPX su `padding-inline: 0.2rem`.
	LadoCuentaCarril  float32 = 1.125 * tema.Rem
	FueraCuentaCarril float32 = 0.3 * tema.Rem
	CuentaCarrilPX    float32 = 0.2 * tema.Rem
	// IndicadorCarrilX es el `inset-inline-start: -0.875rem` de la barra de «estás
	// aquí» —el borde del carril, que deja 14 a cada lado de la acción— e
	// IndicadorCarrilY su `inset-block: 0.5rem`.
	IndicadorCarrilX float32 = -0.875 * tema.Rem
	IndicadorCarrilY float32 = 0.5 * tema.Rem
)

// anchoIndicadorCarril es el `--sytue-rail-indicator` de la forma en vigor: 3 en Marco y
// 0 en Carril, donde el activo ya es un cuadro macizo.
func anchoIndicadorCarril() float32 { return tema.Forma("rail-indicator") }

// EscalonCuentaCarril es el `font-size: 0.6875rem` de la cuenta con el `line-height: 1.5`
// que hereda del documento: 11 sobre 16,5.
var EscalonCuentaCarril = tema.Escalon{Tam: 0.6875 * tema.Rem, Linea: 1.5 * 0.6875 * tema.Rem}

// AccionCarril es un botón del carril de iconos (`.erp-rail-action`).
type AccionCarril struct {
	ui.Pulsable

	Icono iconos.Icono
	// Titulo es su `title`: lo único que dice qué es, porque no lleva rótulo.
	Titulo string
	// Activa es `aria-current="page"`.
	Activa bool
	// Contador es `.erp-rail-count`: las tareas de hoy. Cero, no hay.
	Contador int
}

var _ fyne.Widget = (*AccionCarril)(nil)

// NuevaAccionCarril crea una acción. `nombre` es su `title` y su `aria-label`.
func NuevaAccionCarril(i iconos.Icono, nombre string, activa bool, alPulsar func()) *AccionCarril {
	a := &AccionCarril{Icono: i, Titulo: nombre, Activa: activa}
	a.Nombre, a.AlPulsar = nombre, alPulsar
	a.ExtendBaseWidget(a)
	return a
}

// MinSize es el cuadrado de 44: la cuenta sobresale, no ocupa.
func (a *AccionCarril) MinSize() fyne.Size {
	a.ExtendBaseWidget(a)
	return fyne.NewSize(LadoAccionCarril, LadoAccionCarril)
}

// colores son el fondo, la tinta del icono y su relleno en el estado de ahora. El activo
// gana al puntero: su regla va después en la hoja y pesa lo mismo.
func (a *AccionCarril) colores(v tema.Variante) (fondo, tinta, relleno color.NRGBA) {
	switch {
	case a.Activa:
		return tema.Color("rail-active-bg", v), tema.Color("rail-active-fg", v), tema.Color("sidebar-indicator", v)
	case a.Encima():
		fondo = tema.Color("rail-item-hover", v)
	default:
		fondo = tema.Color("rail-item-bg", v)
	}
	// `color: var(--sytue-primary-text)`, que dentro de `bg-rail` es el del carril.
	return fondo, tema.Color("sidebar-fg", v), tema.Color("rail-item-fill", v)
}

// CreateRenderer construye el aro, la caja, el icono y la cuenta.
func (a *AccionCarril) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	radio := tema.Forma("radius-nav")
	r := &accionCarrilRenderer{
		a: a,
		// El aro del carril es el suyo (`--sytue-focus-ring` dentro de `bg-rail`).
		aro:   ui.NuevoAroCon("sidebar-focus", radio, ui.SinHueco, v),
		caja:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, radio),
		barra: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, anchoIndicadorCarril()/2),
		icono: iconos.Trazo(a.Icono, LadoIconoCarril, "sidebar-fg", v),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.barra.Objetos()...)
	r.objetos = append(r.objetos, r.icono.Objeto())
	if a.Contador > 0 {
		r.cuenta = ui.NuevaCaja(tema.Color("danger", v), color.NRGBA{}, tema.RadioPill)
		// Blanco y no `on-primary`: sobre el rojo de peligro el rótulo del primario se
		// queda en 3,7:1 en oscuro (app.css).
		r.cifra = ui.Texto(contar(a.Contador), EscalonCuentaCarril, ui.SemiNegrita, tema.Color("neutral-0", v))
		r.objetos = append(r.objetos, r.cuenta.Objetos()...)
		r.objetos = append(r.objetos, r.cifra)
	}
	r.Refresh() // regla 7 de `ui.go`
	return r
}

type accionCarrilRenderer struct {
	a       *AccionCarril
	aro     *ui.Aro
	caja    *ui.Caja
	barra   *ui.Caja // «estás aquí»: el `::before` de la activa
	icono   *iconos.Dibujo
	cuenta  *ui.Caja
	cifra   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *accionCarrilRenderer) MinSize() fyne.Size           { return r.a.MinSize() }
func (r *accionCarrilRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *accionCarrilRenderer) Destroy()                     {}

func (r *accionCarrilRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.icono.Colocar((t.Width-LadoIconoCarril)/2, (t.Height-LadoIconoCarril)/2, LadoIconoCarril)
	// La barra va FUERA de la acción, en el borde del carril, como la cuenta sobresale.
	ui.MoverCaja(r.barra, fyne.NewPos(IndicadorCarrilX, IndicadorCarrilY),
		fyne.NewSize(anchoIndicadorCarril(), max(t.Height-2*IndicadorCarrilY, 0)))
	if r.cuenta == nil {
		return
	}
	// `min-width` con su `padding-inline`, anclada por la derecha: al crecer, crece
	// hacia la izquierda.
	cifra := ui.Medir(r.cifra.Text, EscalonCuentaCarril, ui.SemiNegrita).Width
	ancho := max(LadoCuentaCarril, cifra+2*CuentaCarrilPX)
	pos := fyne.NewPos(t.Width+FueraCuentaCarril-ancho, -FueraCuentaCarril)
	ui.MoverCaja(r.cuenta, pos, fyne.NewSize(ancho, LadoCuentaCarril))
	// La cifra va centrada a lo ancho (el `text-align` del botón) y con su caja de línea
	// arriba: la cuenta no la centra en vertical, y así se ve en la web.
	ui.ColocarTexto(r.cifra, EscalonCuentaCarril, pos.X+(ancho-cifra)/2, pos.Y)
}

// Refresh pinta el estado: reposo, puntero encima o activo.
func (r *accionCarrilRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta, relleno := r.a.colores(v)
	radio := tema.Forma("radius-nav") // la forma se cambia en caliente
	r.aro.CambiarRadio(radio)
	r.aro.Ver(r.a.Enfocado())
	r.aro.Pintar(v)
	r.caja.CambiarRadio(radio)
	r.caja.Pintar(fondo, color.NRGBA{})
	// Sin ancho tampoco tiene color: a un rectángulo de ancho 0 el pintor de GL le deja
	// una raya (ver `barraActiva` en menu.go).
	indicador := color.NRGBA{}
	if r.a.Activa && anchoIndicadorCarril() > 0 {
		indicador = tema.Color("sidebar-indicator", v)
	}
	r.barra.CambiarRadio(anchoIndicadorCarril() / 2) // `rounded-pill`
	r.barra.Pintar(indicador, color.NRGBA{})
	r.icono.TenirCon(tinta)
	r.icono.Acento(relleno)
	if r.cuenta != nil {
		r.cuenta.Pintar(tema.Color("danger", v), color.NRGBA{})
		r.cifra.Color = tema.Color("neutral-0", v)
		r.cifra.Refresh()
	}
	r.Layout(r.a.Size())
}

// CarrilIconos es el estado del carril: qué acciones lleva arriba y cuáles abajo
// (`.erp-rail-bottom`). Como el Menu, no habla con nadie: lo rellena quien lo monta.
type CarrilIconos struct {
	Arriba, Abajo []*AccionCarril
}

// Objeto arma el carril. En Carril pinta su fondo; en Marco deja ver el marco, que ya es
// de su color (`.sytue-frame .bg-rail` en appearance.css).
func (c *CarrilIconos) Objeto(v tema.Variante) fyne.CanvasObject {
	fondo := canvas.NewRectangle(tema.Color("rail", v))
	if tema.EsMarco() {
		fondo.FillColor = color.NRGBA{}
	}
	hijos := make([]fyne.CanvasObject, 0, 1+len(c.Arriba)+len(c.Abajo))
	hijos = append(hijos, fondo)
	for _, a := range append(append([]*AccionCarril{}, c.Arriba...), c.Abajo...) {
		// El `title` de la web, con la pista de la suite: a la derecha, que encima se
		// saldría de la ventana. Sale con el puntero y con el foco.
		titulo := ui.NuevaPista(a.Titulo, a)
		titulo.Derecha = true
		hijos = append(hijos, titulo)
	}
	contenido := container.New(&columnaCarril{arriba: len(c.Arriba)}, hijos...)
	desplaza := container.NewVScroll(contenido)
	ui.RevelarFocoEnScroll(desplaza, contenido)
	return container.New(viewportCarril{}, desplaza)
}

// El carril conserva su ancho y permite recorrer todas las acciones con poco alto.
type viewportCarril struct{}

func (viewportCarril) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(AnchoCarril, 0)
}

func (viewportCarril) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.Position{})
	objs[0].Resize(t)
	if s, ok := objs[0].(*container.Scroll); ok && ui.RevelarFocoEnScroll(s, s.Content) {
		ui.MostrarFoco(s, fyne.CurrentApp().Driver().CanvasForObject(s), 0)
	}
}

// columnaCarril coloca [fondo, las de arriba…, las de abajo…]: las primeras desde el
// `padding-block` y las últimas pegadas al fondo, que es el `margin-block-start: auto` de
// `.erp-rail-bottom`.
type columnaCarril struct{ arriba int }

var _ fyne.Layout = (*columnaCarril)(nil)

func (l *columnaCarril) MinSize(objs []fyne.CanvasObject) fyne.Size {
	n := float32(len(objs) - 1)
	return fyne.NewSize(AnchoCarril, 2*CarrilPY+n*LadoAccionCarril+max(n-1, 0)*HuecoCarril)
}

func (l *columnaCarril) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	lado := fyne.NewSize(LadoAccionCarril, LadoAccionCarril)
	x, paso := (t.Width-LadoAccionCarril)/2, LadoAccionCarril+HuecoCarril
	acciones := objs[1:]
	for i, o := range acciones {
		y := CarrilPY + float32(i)*paso
		if i >= l.arriba {
			y = max(y, t.Height-CarrilPY-LadoAccionCarril-float32(len(acciones)-1-i)*paso)
		}
		o.Move(fyne.NewPos(x, y))
		o.Resize(lado)
	}
}
