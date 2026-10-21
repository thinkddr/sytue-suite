// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// ---- La navegación de las seis pestañas ----

// Seccion es una pestaña de la nav, en el orden de Nav.tsx.
type Seccion uint8

// Las seis.
const (
	SecFotos Seccion = iota
	SecFavoritas
	SecAlbumes
	SecCompartidos
	SecRepetidas
	SecPapelera
)

// glifoRepetidas es `GlifoRepetidas` de Nav.tsx: dos marcos superpuestos, porque
// «repetidas» no puede llevar el mismo icono que «álbumes».
//
// Es un glifo PROPIO de esta app —no está en `Icon.tsx` ni en el catálogo compartido,
// igual que en la web—, así que vive aquí con su guardián
// (`TestElGlifoDeRepetidasEsElDeNavTsx`), que lo compara con el `.tsx` de verdad.
const glifoRepetidas = "M8 8h12v12H8zM4 16V4h12"

// Las medidas de la nav, de Nav.tsx: `h-8` de alto, `gap-1` entre pestañas, `px-2` de
// relleno y `gap-1.5` entre el icono y el rótulo. El alto es `h-8` y no `py-1` a
// propósito: con el relleno la pestaña medía 23 px, por debajo de los 24×24 que pide
// WCAG 2.5.8 para lo que se toca con el dedo, y son las cinco rutas de la app.
//
// Medido en producción a 1280: la nav entera son 407,25×28 y cada pestaña, 28 de alto.
const (
	AltoPestanaNav = 8 * 0.25 * tema.Rem
	HuecoNav       = 1 * 0.25 * tema.Rem
	RellenoPestana = 2 * 0.25 * tema.Rem
	HuecoIconoNav  = 1.5 * 0.25 * tema.Rem
	IconoNav       = float32(16)
)

var clavesNav = [...]string{
	SecFotos: "nav.photos", SecFavoritas: "nav.favorites", SecAlbumes: "nav.albums",
	// Del catálogo COMÚN: «Compartido conmigo» es el mismo rótulo en Notepads.
	SecCompartidos: "sharedWithMe.title",
	SecRepetidas:   "nav.duplicates", SecPapelera: "nav.trash",
}

var iconosNav = [...]iconos.Icono{
	SecFotos: iconos.Imagen, SecFavoritas: iconos.Corazon, SecAlbumes: iconos.Album,
	SecCompartidos: iconos.Usuarios,
	SecRepetidas:   iconos.Ninguno, SecPapelera: iconos.Papelera,
}

// PestanaNav es una de las cinco. Pinta lo suyo y no `ui.Boton` ni `ui.Pestana`: el
// botón no tiene `leadingIcon` todavía (está en los huecos medidos) y la pestaña de
// `ui` es la de una barra con subrayado, que es otra pieza. Esta es un ancla con fondo
// al estar activa, que es lo que pinta Nav.tsx. Puntero, pulsación y teclado son los
// de `ui.Pulsable`.
type PestanaNav struct {
	ui.Pulsable

	Rotulo string
	Icono  iconos.Icono
	// Propio es la `d` de un glifo de la app, cuando el icono no está en el catálogo
	// compartido. Manda sobre Icono.
	Propio string
	Activa bool
}

var _ fyne.Widget = (*PestanaNav)(nil)

// NuevaPestanaNav crea la pestaña.
func NuevaPestanaNav(rotulo string, ic iconos.Icono, activa bool, alPulsar func()) *PestanaNav {
	p := &PestanaNav{Rotulo: rotulo, Icono: ic, Activa: activa}
	p.AlPulsar = alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityRole: en la web es un `<a>`, y Fyne tiene el rol.
func (p *PestanaNav) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleLink }

// AccessibilityLabel es el rótulo.
func (p *PestanaNav) AccessibilityLabel() string { return p.Rotulo }

// pesoPestana es `font-medium` en la activa y normal en las demás.
func (p *PestanaNav) pesoPestana() ui.Peso {
	if p.Activa {
		return ui.Medio
	}
	return ui.Normal
}

// MedidaPestanaNav es lo que ocupa: relleno, icono, hueco y rótulo.
func MedidaPestanaNav(p *PestanaNav) fyne.Size {
	ancho := 2*RellenoPestana + IconoNav + HuecoIconoNav +
		ui.Medir(p.Rotulo, tema.TextoSM, p.pesoPestana()).Width
	return fyne.NewSize(ancho, AltoPestanaNav)
}

// CreateRenderer arma la pestaña.
func (p *PestanaNav) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanaNavRenderer{p: p}
	r.caja = ui.NuevaCaja(p.fondo(v), color.NRGBA{}, tema.RadioSM)
	if p.Propio != "" {
		r.icono = iconos.TrazoDeRuta(p.Propio, IconoNav, p.tinta(v))
	} else {
		r.icono = iconos.TrazoConTinta(p.Icono, IconoNav, p.tinta(v))
	}
	r.texto = ui.Texto(p.Rotulo, tema.TextoSM, p.pesoPestana(), p.tinta(v))
	r.aro = ui.NuevoAro(tema.RadioSM, ui.SinHueco, v).Abierto()
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.icono.Objeto(), r.texto)
	return r
}

// fondo es `bg-bg-subtle` en la activa y nada en las demás.
func (p *PestanaNav) fondo(v tema.Variante) color.NRGBA {
	if p.Activa {
		return tema.Color("bg-subtle", v)
	}
	return color.NRGBA{}
}

// tinta es el color del texto: `text-text-muted`, y `text-text` en la activa o al pasar
// por encima.
func (p *PestanaNav) tinta(v tema.Variante) color.NRGBA {
	if p.Activa || p.Encima() { // `hover:text-text` de la inactiva
		return tema.Color("text", v)
	}
	return tema.Color("text-muted", v)
}

type pestanaNavRenderer struct {
	p       *PestanaNav
	caja    *ui.Caja
	icono   *iconos.Dibujo
	texto   *canvas.Text
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *pestanaNavRenderer) MinSize() fyne.Size           { return MedidaPestanaNav(r.p) }
func (r *pestanaNavRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pestanaNavRenderer) Destroy()                     {}

func (r *pestanaNavRenderer) Layout(t fyne.Size) {
	ui.MoverCaja(r.caja, fyne.NewPos(0, 0), t)
	r.aro.Colocar(t)
	r.aro.Ver(r.p.Enfocado())
	r.icono.Colocar(RellenoPestana, (t.Height-IconoNav)/2, IconoNav)
	ui.ColocarTexto(r.texto, tema.TextoSM,
		RellenoPestana+IconoNav+HuecoIconoNav, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *pestanaNavRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(r.p.fondo(v), color.NRGBA{})
	r.icono.TenirCon(r.p.tinta(v))
	r.texto.Color = r.p.tinta(v)
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.Layout(r.p.Size())
}

// Nav son las seis pestañas de Sytue Fotos.
func Nav(v Vista, activa Seccion, alIr func(Seccion)) *fyne.Container {
	hijos := make([]fyne.CanvasObject, 0, len(clavesNav))
	for s := range clavesNav {
		sec := Seccion(s)
		p := NuevaPestanaNav(v.Traduce(clavesNav[s]), iconosNav[s], sec == activa, func() {
			if alIr != nil {
				alIr(sec)
			}
		})
		if sec == SecRepetidas {
			p.Propio = glifoRepetidas
		}
		hijos = append(hijos, p)
	}
	fila := container.New(ui.Fila{Hueco: HuecoNav}, hijos...)
	if v.Ventana >= ui.CorteSM {
		return fila
	}
	// En móvil, UNA fila que se desplaza de lado (`flex-nowrap overflow-x-auto`): con
	// `flex-wrap` las cinco pestañas partían en dos filas y el cromo se comía un tercio
	// de la pantalla antes de la primera foto.
	return deslizable(fila)
}

// deslizable es `overflow-x-auto [scrollbar-width:none]`: la fila se desplaza de lado
// si no cabe, sin barra.
func deslizable(o fyne.CanvasObject) *fyne.Container {
	s := container.NewHScroll(o)
	return container.New(desliza{s}, container.NewThemeOverride(s, tema.SinBarras{}))
}

// desliza da a un desplazamiento lateral el mínimo de su CONTENIDO, para que quien lo
// reparte (una fila que se parte) sepa lo que mide de verdad, y lo deja encoger a lo que
// le den: el `min-w-0` de la nav.
type desliza struct{ s *container.Scroll }

func (d desliza) MinSize([]fyne.CanvasObject) fyne.Size { return d.s.Content.MinSize() }

func (desliza) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// ---- El recuadro ----

// recuadro es un `<div class="border rounded-* p-*">` con tokens: la caja de borde de
// `ui` (regla 4: el borde no es un trazo) y el hijo con su relleno. Un borde vacío es
// «sin borde»; un fondo vacío es el de la página (`bg`), porque la caja de `ui` pinta el
// borde DEBAJO del relleno y con un relleno transparente se vería entero.
func recuadro(v Vista, fondo, borde string, radio float32, m ui.Margen, hijo fyne.CanvasObject) *fyne.Container {
	if fondo == "" {
		fondo = "bg"
	}
	var b color.NRGBA
	if borde != "" {
		b = tema.Color(borde, v.Variante)
	}
	return recuadroCon(tema.Color(fondo, v.Variante), b, radio, m, hijo)
}

// sobre compone `arriba` encima de `abajo` (opaco), que es lo que hace el navegador con
// un `bg-bg/70` sobre la sección de debajo. Sirve para dar a la caja de `ui` un relleno
// OPACO donde la web pone uno translúcido sobre un fondo conocido.
func sobre(arriba, abajo color.NRGBA) color.NRGBA { return tokens.SobreSRGB(arriba, abajo) }

// recuadroCon es recuadro con los colores ya calculados: los literales de la web
// (`bg-black/85`, `border-white/15`) y los tokens con otra opacidad.
func recuadroCon(fondo, borde color.NRGBA, radio float32, m ui.Margen, hijo fyne.CanvasObject) *fyne.Container {
	caja := ui.NuevaCaja(fondo, borde, radio)
	if borde.A != 0 {
		// `border p-3` es el borde MÁS el relleno (`box-sizing: border-box`): el hijo
		// empieza un píxel más adentro por cada lado que lleve borde.
		m.Izq, m.Der, m.Arr, m.Abj = m.Izq+tema.Borde, m.Der+tema.Borde, m.Arr+tema.Borde, m.Abj+tema.Borde
	}
	return container.New(&recuadroLayout{caja: caja, m: m}, append(caja.Objetos(), hijo)...)
}

type recuadroLayout struct {
	caja *ui.Caja
	m    ui.Margen
}

func (r *recuadroLayout) hijo(objs []fyne.CanvasObject) fyne.CanvasObject { return objs[len(objs)-1] }

func (r *recuadroLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := r.hijo(objs).MinSize()
	return fyne.NewSize(h.Width+r.m.Izq+r.m.Der, h.Height+r.m.Arr+r.m.Abj)
}

// AltoCon: el del hijo al ancho que le deja el relleno.
func (r *recuadroLayout) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(r.hijo(objs), ancho-r.m.Izq-r.m.Der) + r.m.Arr + r.m.Abj
}

func (r *recuadroLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	r.caja.Redimensionar(t)
	h := r.hijo(objs)
	h.Move(fyne.NewPos(r.m.Izq, r.m.Arr))
	h.Resize(fyne.NewSize(max(t.Width-r.m.Izq-r.m.Der, 0), max(t.Height-r.m.Arr-r.m.Abj, 0)))
}

// Pagina es el marco de las nueve páginas con sesión: fondo, la caja centrada con su
// `px`/`py` y dentro la nav y los bloques.
//
// `galeria` elige la caja: la línea de tiempo usa `max-w-screen-2xl lg:px-8` y las
// demás `max-w-7xl px-4`.
func Pagina(v Vista, activa Seccion, alIr func(Seccion), galeria bool, bloques ...ui.Bloque) *fyne.Container {
	todos := append([]ui.Bloque{{Obj: Nav(v, activa, alIr), Margen: SeparacionBloque}}, bloques...)
	return Envolver(v, galeria, ui.PilaBloques(todos...))
}

// Envolver pone el fondo de la ventana y centra una caja de contenido. Lo usa Pagina y
// también `/p/{token}`, que no lleva nav.
func Envolver(v Vista, galeria bool, dentro *fyne.Container) *fyne.Container {
	fondo := canvas.NewRectangle(tema.Color("bg", v.Variante))
	ventana := v.Ventana
	if ventana <= 0 {
		ventana = AnchoMaximoGaleria
	}
	return container.New(&marco{galeria: galeria, dentro: dentro, ventana: ventana}, fondo, dentro)
}

// marco centra la caja de contenido con el `py-6` arriba y abajo.
//
// Su alto depende del ancho —cuántas columnas caben—, y `MinSize` no lo recibe: por eso
// recuerda el de la última vez que se colocó, que es el de la ventana. Quien lo mete en
// un desplazamiento vuelve a refrescarlo cuando la ventana cambia (ver `raizLayout`).
type marco struct {
	galeria bool
	dentro  *fyne.Container
	ventana float32
}

var _ fyne.Layout = (*marco)(nil)

func (m *marco) caja(ventana float32) (x, ancho float32) {
	if m.galeria {
		return CajaGaleria(ventana)
	}
	return CajaPagina(ventana)
}

func (m *marco) altoDentro(ancho float32) float32 { return ui.AltoA(m.dentro, ancho) }

func (m *marco) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 2 {
		return fyne.NewSize(0, 0)
	}
	_, ancho := m.caja(m.ventana)
	return fyne.NewSize(0, m.altoDentro(ancho)+2*MargenPaginaY)
}

func (m *marco) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 2 {
		return
	}
	m.ventana = t.Width
	objs[0].Resize(t)
	x, ancho := m.caja(t.Width)
	objs[1].Move(fyne.NewPos(x, MargenPaginaY))
	objs[1].Resize(fyne.NewSize(ancho, m.altoDentro(ancho)))
}
