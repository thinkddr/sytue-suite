// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/BarraMovil.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// BarraMovil es la barra que AppShell.tsx pone bajo la topbar por debajo de 768: la
// ÚNICA vía de abrir el menú —y de volver del detalle a la lista— en un teléfono. Va en
// `Armazon.BarraMovil`, que ya la coloca y la esconde en ancho.
//
//	barra  `flex h-11 items-center gap-1.5 border-b bg-surface px-4`       hoja
//	botón  `ghost sm iconOnly` con el `menu` de 18, o el `chevronLeft` de  hoja
//	       18 si hay detalle abierto que cerrar
//	título `text-sm font-semibold text-text truncate`; con lista, SOLO en   hoja
//	       el detalle (con la lista a la vista, la lista trae el suyo y se leía
//	       dos veces). Sin lista, siempre.
//	       Sin menú y sin detalle que cerrar, no hay botón.
//
// Y lo que el mes del teléfono de Calendar le pide (`mobileBarCenter`,
// `mobileBarActions` y `mobileNavNotice` de AppShell):
//
//	TituloCentrado  el título en `text-md font-semibold`, centrado en el ancho ENTERO
//	                de la barra y no en el hueco que dejan los botones: no baila según
//	                cuántas acciones haya a cada lado
//	Acciones        a la derecha (`ml-auto flex items-center gap-1`), con el borde en
//	                el canalón `px-4` y la misma corrección táctil que la hamburguesa
//	Aviso           el nombre del ☰ cerrado y un punto de 8 en la esquina del ICONO
//	                (el color de señal: `sidebar-indicator` sobre el marco de color,
//	                `indicator` sobre la superficie de Carril)
//
// Sin ninguno de los tres, la barra es la de siempre.
// Los `px-4` son el mismo canalón que la topbar, para que la hamburguesa caiga en la
// vertical de la marca de arriba.
type BarraMovil struct {
	widget.BaseWidget

	// Titulo se enseña siempre sin lista (`ConLista`) y, con lista, solo con EnDetalle.
	Titulo string
	// ConLista es que el armazón tiene panel de lista: lo pone `Armazon`.
	ConLista bool
	// EnDetalle cambia el menú por «volver a la lista».
	EnDetalle bool
	// CajonAbierto cambia el nombre accesible del botón de menú.
	CajonAbierto bool
	// Los tres nombres accesibles, YA traducidos: `ui.openNav`, `ui.closeNav` y
	// `ui.backToList`.
	RotuloAbrir, RotuloCerrar, RotuloVolver string
	AlMenu, AlVolver                        func()
	// TituloCentrado pinta el título en `text-md font-semibold` centrado en el ancho de
	// la barra (`mobileBarCenter`) o, si así no cabe, lo más cerca del centro dentro del
	// hueco entre el ☰ y las acciones; si ni ahí cabe, en `text-sm` antes de truncar.
	TituloCentrado bool
	// Acciones van a la derecha, cada una con su MinSize (`mobileBarActions`). Las que
	// son botones de icono (`NuevoBotonBarraMovil`, o un widget que lo embebe) toman los
	// tonos de la barra, como la hamburguesa.
	Acciones []fyne.CanvasObject
	// Aviso es el nombre accesible, ya traducido, del botón de menú cerrado cuando hay
	// algo pendiente en el cajón, y pinta su punto (`mobileNavNotice`). Vacío: ni punto
	// ni cambio de nombre.
	Aviso string

	// enSuite: va DENTRO de la barra de la suite, la barra única del teléfono (lo pone el
	// armazón por orden de `BarraSuite.Montar`). Mide lo que la barra de la suite, pinta
	// el carril y deja a la derecha `reservaSuite` para la entrada a la suite (el avatar),
	// que pinta la barra de la suite por encima. Ver `TopBarMovil` en TopBar.tsx.
	enSuite      bool
	reservaSuite float32
	// tituloRespaldo es el nombre del producto: lo que se lee en la barra única cuando la
	// app no tiene título que enseñar (la lista de Mail, la de Contacts).
	tituloRespaldo string

	boton *BotonIcono // el de menú o volver: adonde vuelve el foco al cerrar el cajón
	// listaVisible conserva EnDetalle y su callback al girar, pero ofrece el menú
	// cuando lista y detalle ya se ven juntos.
	listaVisible bool
}

var _ fyne.Widget = (*BarraMovil)(nil)

// AltoBarraMovil es el `h-11` (44 px).
var AltoBarraMovil = tema.Espacio(11)

var (
	canalonBarraMovil  = tema.Espacio(4)   // px-4
	huecoBarraMovil    = tema.Espacio(1.5) // gap-1.5
	huecoAccionesMovil = tema.Espacio(1)   // gap-1 de las acciones
)

// LadoPuntoAvisoMovil es el `h-[8px] w-[8px]` del punto de pendientes del ☰, y
// DesvioPuntoAvisoMovil lo que su esquina de arriba a la derecha se separa del centro del
// botón (`top-[calc(50%-15px)] right-[calc(50%-15px)]`): la esquina del glifo de 18.
const (
	LadoPuntoAvisoMovil   float32 = 8
	DesvioPuntoAvisoMovil float32 = 15
)

// LadoBotonBarraMovil es la caja de un botón de la barra: `ghost sm iconOnly`, que con
// el dedo crece al objetivo táctil.
func LadoBotonBarraMovil() float32 {
	return MedidaBotonCon("", TallaSM, 0, 0, true, iconos.TallaMD).Width
}

// NuevoBotonBarraMovil es un botón de la barra móvil (`<Button variant="ghost" size="sm"
// iconOnly>`): el de la hamburguesa y las acciones. Los tonos se los pone la barra.
func NuevoBotonBarraMovil(icono iconos.Icono, nombre string, alPulsar func()) *BotonIcono {
	b := NuevoBotonIcono(icono, nombre, alPulsar)
	b.Lado = LadoBotonBarraMovil()
	b.TamIcono = iconos.TallaMD
	return b
}

// botonDeBarra lo cumple todo widget que embebe un BotonIcono: así una acción con dibujo
// propio (la hoja de «Hoy» de Calendar) toma los tonos de la barra igual que un botón.
type botonDeBarra interface{ botonIcono() *BotonIcono }

func (b *BotonIcono) botonIcono() *BotonIcono { return b }

// tonoBarraMovilEn pone los tokens de un botón de la barra: sobre la superficie en
// Carril, y sobre el carril de color en Marco (`.sytue-mobilebar`) y en la barra única.
func tonoBarraMovilEn(b *BotonIcono, carril bool) {
	b.Trazo, b.TrazoEncima = "primary-text", "primary-text"
	b.FondoEncima, b.FondoActivo, b.TokenAro = "selected", "selected", "focus-ring"
	if carril {
		b.Trazo, b.TrazoEncima = "sidebar-fg", "sidebar-fg"
		b.FondoEncima, b.FondoActivo, b.TokenAro = "sidebar-active-bg", "sidebar-active-bg", "sidebar-focus"
	}
}

// NuevaBarraMovil crea la barra.
func NuevaBarraMovil() *BarraMovil {
	b := &BarraMovil{}
	b.ExtendBaseWidget(b)
	return b
}

// MedidaBarraMovil es lo que ocupa: su alto, con el filete dentro.
func MedidaBarraMovil() fyne.Size { return fyne.NewSize(0, AltoBarraMovil) }

// CreateRenderer compone fondo, filete, botón y título. Termina en Refresh (regla 7).
func (b *BarraMovil) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	boton := NuevoBotonBarraMovil(iconos.Menu, "", nil)
	b.boton = boton
	r := &barraMovilRenderer{
		b:      b,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		boton:  boton,
		titulo: Texto("", tallaTituloMovil(b), SemiNegrita, tema.Color("text", v)),
		punto:  canvas.NewCircle(color.NRGBA{}),
	}
	r.Refresh()
	return r
}

// tallaTituloMovil es la talla del título: `text-sm` junto al botón y `text-md` centrado
// o dentro de la barra única.
func tallaTituloMovil(b *BarraMovil) tema.Escalon {
	if b.TituloCentrado || b.enSuite {
		return tema.TextoMD
	}
	return tema.TextoSM
}

type barraMovilRenderer struct {
	b             *BarraMovil
	fondo, filete *canvas.Rectangle
	boton         *BotonIcono
	titulo        *canvas.Text
	punto         *canvas.Circle
}

func (r *barraMovilRenderer) MinSize() fyne.Size {
	if r.b.enSuite {
		return fyne.NewSize(0, AltoBarraSuite())
	}
	return MedidaBarraMovil()
}

// Medidas de la barra única: en el teléfono el canalón lo pone el AIRE de los botones
// (`--sytue-topbar-pad: 0.25rem` y `gap-0.5` de TopBar.tsx), no la barra.
var (
	canalonBarraUnica = tema.Espacio(1)
	huecoBarraUnica   = tema.Espacio(0.5)
)

func (r *barraMovilRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.fondo, r.filete, r.boton, r.titulo, r.punto}, r.b.Acciones...)
}
func (r *barraMovilRenderer) Destroy() {}

func (r *barraMovilRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	linea := t.Height
	if !r.filete.Hidden {
		linea = t.Height - tema.Borde
	}
	canalon, hueco, huecoAcciones := canalonBarraMovil, huecoBarraMovil, huecoAccionesMovil
	if r.b.enSuite {
		canalon, hueco, huecoAcciones = canalonBarraUnica, huecoBarraUnica, huecoBarraUnica
	}
	x := canalon
	if r.b.enSuite && !r.boton.Visible() {
		// Sin ☰, la barra de la suite pone la marca del producto en su sitio (ver
		// `layoutTelefono`): el título empieza donde con ☰.
		x += LadoBotonBarraMovil() + huecoBarraUnica
	}
	if r.boton.Visible() {
		lado := r.boton.MinSize()
		base := ladoSoloIcono(TallaSM)
		// En táctil el área pulsable crece a 44 px sin desplazar la vertical del glifo
		// respecto a la marca de la barra superior. En la barra única no hay marca
		// encima: el botón va al canalón.
		izquierda := max(canalon-max(lado.Width-base, 0)/2, 0)
		if r.b.enSuite {
			izquierda = canalon
		}
		r.boton.Move(fyne.NewPos(izquierda, max((linea-lado.Height)/2, 0)))
		r.boton.Resize(lado)
		x = izquierda + lado.Width + hueco
		// El punto de pendientes, en la esquina del glifo (no de la caja, que con el
		// dedo crece a 44).
		cx, cy := izquierda+lado.Width/2, max((linea-lado.Height)/2, 0)+lado.Height/2
		r.punto.Move(fyne.NewPos(cx+DesvioPuntoAvisoMovil-LadoPuntoAvisoMovil, cy-DesvioPuntoAvisoMovil))
		r.punto.Resize(fyne.NewSize(LadoPuntoAvisoMovil, LadoPuntoAvisoMovil))
	}
	// Las acciones, de derecha a izquierda desde el canalón (en la barra única, desde
	// la entrada a la suite).
	der := t.Width
	if r.b.enSuite {
		der -= r.b.reservaSuite
	}
	for i := len(r.b.Acciones) - 1; i >= 0; i-- {
		a := r.b.Acciones[i]
		m := a.MinSize()
		switch {
		case i < len(r.b.Acciones)-1:
			der -= huecoAcciones
		case r.b.enSuite:
			der -= huecoAcciones
		default:
			der -= max(canalon-max(m.Width-ladoSoloIcono(TallaSM), 0)/2, 0)
		}
		der -= m.Width
		a.Move(fyne.NewPos(der, max((linea-m.Height)/2, 0)))
		a.Resize(m)
	}
	talla := tallaTituloMovil(r.b)
	if !r.b.TituloCentrado {
		Truncar(r.titulo, r.tituloVisible(), talla, SemiNegrita, max(der-canalon-x, 0))
		ColocarTexto(r.titulo, talla, x, (linea-talla.Linea)/2)
		return
	}
	// Centrado en el ancho entero si cabe. Si no, lo más cerca del centro que deja el
	// lado más ocupado, dentro del hueco REAL entre el ☰ y las acciones, y truncado solo
	// si ni el hueco le basta (ni en `text-sm`). Es la rejilla `minmax(max-content,1fr) minmax(0,max-content)
	// minmax(max-content,1fr)` de AppShell. Hasta el 2026-11-07 truncaba a `ancho −
	// 2·ocupado` aunque hubiera sitio a un lado: a 320, «Σεπτέμβριος 20…» perdía el año.
	izq, dcha := x, der-hueco
	if len(r.b.Acciones) == 0 && !r.b.enSuite {
		dcha = t.Width - canalon - hueco // la columna vacía y su `gap`
	}
	// Y si ni en el hueco cabe en `text-md`, baja a `text-sm` antes de truncar: a 320 el
	// año («2027») es lo que distingue otro año, y es lo primero que se comería el «…».
	libre := max(dcha-izq, 0)
	if Medir(r.tituloVisible(), talla, SemiNegrita).Width > libre {
		talla = tema.TextoSM
	}
	r.titulo.TextSize = talla.Tam
	ancho := Truncar(r.titulo, r.tituloVisible(), talla, SemiNegrita, libre)
	pos := min(max((t.Width-ancho)/2, izq), max(dcha-ancho, izq))
	ColocarTexto(r.titulo, talla, pos, (linea-talla.Linea)/2)
}

// conBoton dice si la barra lleva ☰ o «volver» (el mismo reparto que Refresh).
func (b *BarraMovil) conBoton() bool {
	return (b.EnDetalle && !b.listaVisible) || b.AlMenu != nil
}

func (r *barraMovilRenderer) tituloVisible() string {
	t := ""
	if r.b.EnDetalle || !r.b.ConLista {
		t = r.b.Titulo
	}
	// En la barra única nunca se queda vacía: sin título, el nombre del producto.
	if t == "" && r.b.enSuite {
		return r.b.tituloRespaldo
	}
	return t
}

func (r *barraMovilRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.boton.Lado = MedidaBotonCon("", TallaSM, 0, 0, true, iconos.TallaMD).Width
	r.fondo.FillColor = tema.Color("surface", v)
	r.filete.FillColor = tema.Color("border", v)
	r.titulo.Color = tema.Color("text", v)
	r.titulo.TextSize = tallaTituloMovil(b).Tam
	carril := tema.EsMarco() || b.enSuite
	tonoBarraMovilEn(r.boton, carril)
	r.fondo.Show()
	r.titulo.Show()
	r.punto.FillColor = tema.Color("indicator", v)
	if carril {
		// En Marco esta fila prolonga la barra de la suite (`.sytue-mobilebar`): el fondo
		// lo pinta el armazón con el mismo degradado horizontal y la hoja empieza debajo.
		// En la barra única ES la barra de la suite, del carril (`.sytue-topbar-carril`
		// por debajo de 768).
		r.fondo.FillColor = tema.Color(tokenFondoBarra(true), v)
		r.fondo.Hidden = tema.EsMarco()
		r.titulo.Color = tema.Color("sidebar-fg", v)
		r.punto.FillColor = tema.Color("sidebar-indicator", v)
	}
	r.filete.Hidden = carril
	for _, a := range b.Acciones {
		if bi, es := a.(botonDeBarra); es {
			tonoBarraMovilEn(bi.botonIcono(), carril)
			a.Refresh()
		}
	}
	r.boton.Show()
	switch {
	case b.EnDetalle && !b.listaVisible:
		r.boton.Icono, r.boton.Rotulo, r.boton.AlPulsar = iconos.ChevronIzquierda, b.RotuloVolver, b.AlVolver
	case b.AlMenu == nil:
		r.boton.Hide() // ni menú ni detalle: AppShell no pinta botón
	case b.CajonAbierto:
		r.boton.Icono, r.boton.Rotulo, r.boton.AlPulsar = iconos.Menu, b.RotuloCerrar, b.AlMenu
	case b.Aviso != "":
		r.boton.Icono, r.boton.Rotulo, r.boton.AlPulsar = iconos.Menu, b.Aviso, b.AlMenu
	default:
		r.boton.Icono, r.boton.Rotulo, r.boton.AlPulsar = iconos.Menu, b.RotuloAbrir, b.AlMenu
	}
	// El punto acompaña al nombre: solo con el menú cerrado y algo pendiente.
	esconder(r.punto, b.Aviso == "" || !r.boton.Visible() || r.boton.Icono != iconos.Menu || b.CajonAbierto)
	r.punto.Refresh()
	r.fondo.Refresh()
	r.filete.Refresh()
	r.boton.Refresh()
	r.Layout(b.Size())
	r.titulo.Refresh()
}

// ---- El botón flotante ----

// BotonFlotante es MobileFab.tsx: la acción principal al alcance del pulgar en el
// teléfono, donde el menú lateral es un cajón cerrado. Cuadrado de `h-14` en `primary`,
// con la esquina `rounded-control`, el icono de 24 y la sombra `lg`, a `right-4` y
// `bottom-4` (o el borde seguro, si es mayor). Solo se ve por debajo de 768
// (`md:hidden`): lo coloca `CapaFlotante`.
type BotonFlotante struct {
	Pulsable
	Icono iconos.Icono
}

// LadoBotonFlotante es el `h-14 w-14`: 49 con el rem de 14.
var LadoBotonFlotante = tema.Espacio(14)

// MargenBotonFlotante es el `right-4` y el `bottom-[max(1rem,…)]`.
var MargenBotonFlotante = tema.Espacio(4)

// NuevoBotonFlotante crea el botón con su nombre accesible y su icono.
func NuevoBotonFlotante(rotulo string, icono iconos.Icono, alPulsar func()) *BotonFlotante {
	b := &BotonFlotante{Icono: icono}
	b.Nombre, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// Tapped delega en el cierre del cajón si el menú móvil está abierto por debajo en la
// pila: el botón flotante vive sobre el armazón, pero en la web queda bajo el velo
// (`z-20` frente a `z-sidebar`).
func (b *BotonFlotante) Tapped(e *fyne.PointEvent) {
	if b.Inerte {
		return
	}
	if a := cajonDe(lienzoDe(b)); a != nil && a.CajonAbierto {
		a.cerrarDesdeMando()
		return
	}
	b.Pulsable.Tapped(e)
}

// CreateRenderer compone sombra, caja, icono, anillo y velo de cajón. Termina en Refresh.
func (b *BotonFlotante) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	velo := canvas.NewRectangle(color.NRGBA{})
	velo.Hidden = true
	r := &flotanteRenderer{
		b:      b,
		sombra: NuevaSombra(tema.SombraLG, tema.Forma("radius-control")),
		fondo:  canvas.NewRectangle(tema.Color("primary", v)),
		glifo:  iconos.Trazo(b.Icono, iconos.TallaXL, "on-primary", v),
		aro:    NuevoAro(tema.Forma("radius-control"), SinHueco, v),
		velo:   velo,
	}
	r.Refresh()
	return r
}

type flotanteRenderer struct {
	b      *BotonFlotante
	sombra *Sombra
	fondo  *canvas.Rectangle
	glifo  *iconos.Dibujo
	aro    *Aro
	velo   *canvas.Rectangle
}

func (r *flotanteRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoBotonFlotante, LadoBotonFlotante)
}

func (r *flotanteRenderer) Objects() []fyne.CanvasObject {
	out := append([]fyne.CanvasObject{r.sombra.Objeto()}, r.aro.Antes(r.fondo, r.glifo.Objeto())...)
	return append(out, r.velo)
}
func (r *flotanteRenderer) Destroy() {}

func (r *flotanteRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	r.fondo.Resize(t)
	r.aro.Colocar(t)
	r.glifo.Colocar((t.Width-iconos.TallaXL)/2, (t.Height-iconos.TallaXL)/2, iconos.TallaXL)
	r.velo.Resize(t)
}

func (r *flotanteRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	radio := tema.Forma("radius-control")
	r.fondo.CornerRadius = radio
	r.velo.CornerRadius = radio
	r.sombra.CambiarRadio(radio)
	r.aro.CambiarRadio(radio)
	switch {
	case r.b.pulsado, r.b.Encima():
		r.fondo.FillColor = tema.Color("primary-hover", v)
	default:
		r.fondo.FillColor = tema.Color("primary", v)
	}
	r.glifo.Tenir("on-primary", v)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	var a *Armazon
	if !r.b.Inerte {
		if c := lienzoDe(r.b); c != nil {
			a = cajonDe(c)
			if a == nil {
				a = armazonEn(c.Content())
			}
		}
	}
	bajoVelo := a != nil && (a.CajonAbierto || a.arrastrandoCajon || a.cerrandoCajon)
	esconder(r.velo, !bajoVelo)
	if bajoVelo {
		r.velo.FillColor = ColorVelo(v, a.progresoCajon())
		r.velo.Refresh()
	}
	r.fondo.Refresh()
	r.Layout(r.b.Size())
}

// CapaFlotante es el `fixed right-4 bottom-4 md:hidden` del botón flotante: lo pega a
// la esquina de abajo a la derecha de lo que cubre, y lo esconde en ancho (≥ 768).
type CapaFlotante struct{}

var _ fyne.Layout = CapaFlotante{}

// MinSize no pide nada: la capa va encima de la pantalla y no la empuja.
func (CapaFlotante) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

// Layout coloca el botón en la esquina, o lo esconde.
func (CapaFlotante) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		esconder(o, t.Width >= Estrecho)
		m := o.MinSize()
		o.Move(fyne.NewPos(t.Width-MargenBotonFlotante-m.Width, t.Height-MargenBotonFlotante-m.Height))
		o.Resize(m)
	}
}
