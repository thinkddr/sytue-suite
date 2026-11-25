// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/BotonFlotante.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # El «+» flotante de crear
//
// BotonFlotante es BotonFlotante.tsx: la acción de CREAR de cada app —«Crear evento»,
// «Redactar», «Nuevo contacto», el «Nuevo» de Drive, el «Subir» de Cast, «Añadir fotos»—,
// un «+» redondo que flota abajo a la derecha EN TODOS LOS ANCHOS.
//
// Fue el espejo de `MobileFab`: cuadrado con la esquina de los botones y solo por debajo
// de 768, porque en ancho la acción la llevaba el botón primario del menú lateral. Por
// decisión de producto (2026-11-25) el «+» es EL control de crear en toda la suite, así
// que el del menú se quita y este se ve también en escritorio. Es la excepción a los
// botones rectangulares de CONVENTIONS §5: un círculo con un «+» se reconoce sin leer.
//
// El glifo es siempre el «+»: Drive y Cast llevaban la flecha de subir, y cada app acababa
// diciendo «crear» a su manera. Con más de una cosa que crear (el «Nuevo» de Drive), la
// app abre su `Menu` colgado del botón al pulsarlo (`Menu.Arriba`, que abajo no cabe).
//
//	`h-14 w-14 rounded-full` en `primary`, con el `plus` de 24 en `on-primary`, la sombra
//	`lg` y `disabled:opacity-50`; a `right-4` y `bottom-4` (o el borde seguro).
type BotonFlotante struct {
	Pulsable
}

// LadoBotonFlotante es el `h-14 w-14`: 56 con el rem de 16.
var LadoBotonFlotante = tema.Espacio(14)

// MargenBotonFlotante es el `right-4` y el `bottom-[max(1rem,…)]`.
var MargenBotonFlotante = tema.Espacio(4)

// MargenFlotanteEn es lo que el botón se separa de la esquina de una ventana de `ancho`:
// el margen, DENTRO de la hoja. En «Marco» y desde `Estrecho` la hoja flota a `sheet-gap`
// del borde (`+var(--sytue-sheet-gap)` en BotonFlotante.tsx), y sin sumarlo el botón
// quedaba a caballo entre la hoja y el marco, medio fundido con un marco que es del mismo
// color que él. En «Carril» y en el teléfono la hoja va a sangre y no suma nada.
func MargenFlotanteEn(ancho float32) float32 {
	if tema.EsMarco() && ancho >= Estrecho {
		return MargenBotonFlotante + tema.Forma("sheet-gap")
	}
	return MargenBotonFlotante
}

// NuevoBotonFlotante crea el «+» con su nombre accesible: lo que crea.
func NuevoBotonFlotante(rotulo string, alPulsar func()) *BotonFlotante {
	b := &BotonFlotante{}
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

// radioFlotante es el `rounded-full`: medio lado, así que el botón es un círculo.
func radioFlotante() float32 { return LadoBotonFlotante / 2 }

// CreateRenderer compone sombra, círculo, «+», anillo y velo de cajón. Termina en Refresh.
func (b *BotonFlotante) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	velo := canvas.NewRectangle(color.NRGBA{})
	velo.Hidden = true
	r := &flotanteRenderer{
		b:      b,
		sombra: NuevaSombra(tema.SombraLG, radioFlotante()),
		fondo:  canvas.NewRectangle(tema.Color("primary", v)),
		glifo:  iconos.Trazo(iconos.Mas, iconos.TallaXL, "on-primary", v),
		aro:    NuevoAro(radioFlotante(), SinHueco, v),
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
	radio := radioFlotante()
	r.fondo.CornerRadius = radio
	r.velo.CornerRadius = radio
	r.sombra.CambiarRadio(radio)
	r.aro.CambiarRadio(radio)
	relleno := tema.Color("primary", v)
	if !r.b.Inerte && (r.b.pulsado || r.b.Encima()) {
		relleno = tema.Color("primary-hover", v)
	}
	// `disabled:opacity-50`: el círculo y el «+» se apagan juntos (Contacts sin libreta).
	r.fondo.FillColor = atenuar(relleno, r.b.Inerte)
	r.glifo.TenirCon(atenuar(tema.Color("on-primary", v), r.b.Inerte))
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

// detalleALaVista dice si el armazón de la ventana de `o` enseña su detalle en vez de la
// lista: la ficha, el lector. Ahí no va el botón flotante.
func detalleALaVista(o fyne.CanvasObject) bool {
	c := lienzoDe(o)
	if c == nil {
		return false
	}
	a := armazonEn(c.Content())
	return a != nil && a.enDetalle()
}

// CapaFlotante es el `fixed right-4 bottom-4` del botón flotante: lo pega a la esquina de
// abajo a la derecha de lo que cubre, en TODOS los anchos (era `md:hidden`, y en ancho lo
// escondía).
//
// Salvo en una pantalla de DETALLE en el teléfono: con la ficha de un contacto o el lector
// de Mail a la vista, el «+» tapaba el final de lo que se lee y no es la acción de esa
// pantalla (auditoría del 2026-11-24). Lo dice el armazón de la ventana
// (`Armazon.DetalleAbierto` con panel, en estrecho), que vuelve a colocar la capa cuando
// eso cambia; en la web, `data-sytue-detalle` de AppShell.
type CapaFlotante struct{}

var _ fyne.Layout = CapaFlotante{}

// MinSize no pide nada: la capa va encima de la pantalla y no la empuja.
func (CapaFlotante) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

// Layout coloca el botón en la esquina, o lo esconde en el detalle.
func (CapaFlotante) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		esconder(o, detalleALaVista(o))
		m, margen := o.MinSize(), MargenFlotanteEn(t.Width)
		o.Move(fyne.NewPos(t.Width-margen-m.Width, t.Height-margen-m.Height))
		o.Resize(m)
	}
}
