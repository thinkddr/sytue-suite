// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las medidas del marco de las páginas: `max-w-7xl mx-auto p-6`, y los huecos entre
// bloques (`space-y-8` entre secciones, `mb-3` bajo el título de una sección, `gap-4`
// en las rejillas de tarjetas).
const (
	AnchoMaximo  = 80 * tema.Rem
	MargenPagina = 6 * 0.25 * tema.Rem
	HuecoSeccion = 8 * 0.25 * tema.Rem
	HuecoTitulo  = 3 * 0.25 * tema.Rem
	HuecoRejilla = 4 * 0.25 * tema.Rem
	HuecoFila    = 2 * 0.25 * tema.Rem
)

// Los glifos propios de Cast: lo que el catálogo compartido no tiene porque ninguna
// otra app lo necesitaba (reproducir, pausa, volumen, pantalla completa, el directo).
// Son de la familia del catálogo —trazo de 2 sobre la rejilla de 24—, y el de
// reproducir y pausa van rellenos, como en cualquier reproductor.
const (
	glifoInicio   = "M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z"
	glifoVideo    = "M16 13l5.2 3.5a.5.5 0 0 0 .8-.4V7.9a.5.5 0 0 0-.8-.4L16 10.5M4 6h10a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2z"
	glifoDirecto  = "M4.9 19.1C1 15.2 1 8.8 4.9 4.9M7.8 16.2c-2.3-2.3-2.3-6.1 0-8.5M16.2 7.8c2.3 2.3 2.3 6.1 0 8.5M19.1 4.9C23 8.8 23 15.1 19.1 19M14 12a2 2 0 1 1-4 0 2 2 0 0 1 4 0z"
	glifoPlay     = "M7 4.5v15l12.5-7.5z"
	glifoPausa    = "M6 4h4v16H6zM14 4h4v16h-4z"
	glifoVolumen  = "M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5zM16 9a5 5 0 0 1 0 6M19.4 18.4a9 9 0 0 0 0-12.8"
	glifoSilencio = "M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5zM22 9l-6 6M16 9l6 6"
	glifoAmpliar  = "M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"
	glifoOjo      = "M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12zM15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z"
	glifoOjoNo    = "M10.7 5.1A10 10 0 0 1 12 5c6.5 0 10 7 10 7a13 13 0 0 1-1.7 2.7M6.6 6.6A13.5 13.5 0 0 0 2 12s3.5 7 10 7a9.7 9.7 0 0 0 5.4-1.6M9.9 9.9a3 3 0 0 0 4.2 4.2M2 2l20 20"
)

// ---- El marco de una página ----

// Pagina es el fondo de la ventana con la caja de contenido centrada y los bloques
// dentro, uno debajo de otro.
func Pagina(v Vista, bloques ...ui.Bloque) *fyne.Container {
	fondo := canvas.NewRectangle(tema.Color("bg", v.Variante))
	// El papel ya lo pinta Armazon. Un `bg` opaco aquí tapaba la hoja traslúcida
	// de Marco y dejaba Cast con una segunda superficie dentro de ella.
	if tema.EsMarco() {
		fondo.FillColor = transparente()
	}
	ventana := v.Ventana
	if ventana <= 0 {
		ventana = 1280
	}
	return container.New(&marco{ventana: ventana}, fondo, ui.PilaBloques(bloques...))
}

// marco centra la caja con su margen. Su alto depende del ancho, que MinSize no
// recibe: recuerda el de la última vez que se colocó (el de la ventana).
type marco struct{ ventana float32 }

var _ fyne.Layout = (*marco)(nil)

// CajaPagina es dónde empieza y cuánto mide la caja de contenido a ese ancho.
func CajaPagina(ventana float32) (x, ancho float32) {
	m := MargenLateral(ventana)
	util := min(ventana, AnchoMaximo+2*m)
	return (ventana-util)/2 + m, max(util-2*m, 0)
}

// MargenLateral es el `px-4 sm:px-6` de las páginas de la web: en un teléfono, 14 a
// cada lado en vez de 21.
func MargenLateral(ventana float32) float32 {
	if ventana < ui.CorteSM {
		return tema.Espacio(4)
	}
	return MargenPagina
}

func (m *marco) MinSize(objs []fyne.CanvasObject) fyne.Size {
	_, ancho := CajaPagina(m.ventana)
	return fyne.NewSize(0, ui.AltoA(objs[1], ancho)+2*MargenPagina)
}

func (m *marco) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m.ventana = t.Width
	objs[0].Resize(t)
	x, ancho := CajaPagina(t.Width)
	objs[1].Move(fyne.NewPos(x, MargenPagina))
	objs[1].Resize(fyne.NewSize(ancho, ui.AltoA(objs[1], ancho)))
}

// Cabecera es el `<SectionHeader level={1}>` de una página (HomeApp, ExploreApp,
// UploadApp, StudioApp y ManageApp): el título en `sytue-h4`, una línea de ayuda debajo
// y sus acciones a la derecha, sobre la superficie y con su filete. MEDIDO en la web el
// 2026-10-05, raíz de 16: 18 / 26 en 600. Fue un `<header>` propio con el título en
// `Texto2XL`, diez píxeles más grande que el de la web y sin su barra.
func Cabecera(titulo, ayuda string, acciones ...fyne.CanvasObject) fyne.CanvasObject {
	c := ui.NuevaCabeceraApp(titulo, acciones...)
	c.Subtitulo = ayuda
	return c
}

// TituloSeccion es el `<h2 class="sytue-h4">` de una sección, con algo a
// la derecha si hace falta (un recuento, «Ver todo»).
func TituloSeccion(v Vista, texto string, derecha ...fyne.CanvasObject) fyne.CanvasObject {
	escalon := ui.EscalonTitulo(4)
	t := ui.NuevoParrafo(texto, escalon, ui.PesoTitulo(4), "text", v.Variante)
	hijos := append([]fyne.CanvasObject{t}, derecha...)
	return container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 1, Elastico: 1, Partir: true, SaltoFila: HuecoFila}, hijos...)
}

// Parrafo es un texto que se reparte en líneas, con su token.
func Parrafo(v Vista, texto string, e tema.Escalon, p ui.Peso, token string) *ui.Parrafo {
	return ui.NuevoParrafo(texto, e, p, token, v.Variante)
}

// Fila es una fila de botones con `gap-2` que se parte si no cabe.
func Fila(hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: len(hijos), Partir: true, SaltoFila: HuecoFila}, hijos...)
}

// Recuadro es una Card plana (`rounded-lg border bg-surface p-5`): el contenedor de
// los formularios y paneles de Subir, Gestionar y Emitir.
func Recuadro(v Vista, titulo string, cuerpo fyne.CanvasObject) fyne.CanvasObject {
	bloques := []ui.Bloque{}
	if titulo != "" {
		bloques = append(bloques, ui.Bloque{
			Obj:    ui.NuevoParrafo(titulo, tema.TextoMD, ui.SemiNegrita, "text", v.Variante),
			Margen: HuecoTitulo,
		})
	}
	bloques = append(bloques, ui.Bloque{Obj: cuerpo})
	caja := ui.NuevaCaja(tema.Color("surface", v.Variante), tema.Color("border", v.Variante), tema.RadioLG)
	return container.New(&recuadroLayout{caja: caja}, append(caja.Objetos(), ui.PilaBloques(bloques...))...)
}

// rellenoRecuadro es el `p-5` del recuadro.
const rellenoRecuadro = 5 * 0.25 * tema.Rem

// recuadroLayout: la caja a todo el tamaño y el cuerpo (el último hijo) dentro, con
// su relleno.
type recuadroLayout struct{ caja *ui.Caja }

func (r *recuadroLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*rellenoRecuadro, m.Height+2*rellenoRecuadro)
}

// AltoCon es el del cuerpo al ancho que le deja el relleno.
func (r *recuadroLayout) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[len(objs)-1], ancho-2*rellenoRecuadro) + 2*rellenoRecuadro
}

func (r *recuadroLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	r.caja.Redimensionar(t)
	c := objs[len(objs)-1]
	c.Move(fyne.NewPos(rellenoRecuadro, rellenoRecuadro))
	c.Resize(fyne.NewSize(max(t.Width-2*rellenoRecuadro, 0), max(t.Height-2*rellenoRecuadro, 0)))
}

// ---- Los estados ----

// Vacio es el `EmptyState` de la web con un glifo de Cast en el sitio de la
// ilustración (la suite no tiene ilustración de vídeo, y dibujar una nueva no es
// cosa de esta app).
func Vacio(v Vista, glifo, titulo, descripcion string, accion fyne.CanvasObject) *ui.Vacio {
	e := ui.NuevoVacio(titulo, descripcion)
	e.Glifo = iconos.TrazoDeRuta(glifo, 40, tema.Color("text-subtle", v.Variante))
	e.Accion = accion
	return e
}

// CuerpoDeEstado es lo que se pinta en cada estado que NO es la lista. nil en Lista:
// «pinta tus datos». `vacio` es el de la pantalla, con su texto y su acción.
func CuerpoDeEstado(v Vista, vacio *ui.Vacio) fyne.CanvasObject {
	reintentar := func() fyne.CanvasObject {
		if v.AlReintentar == nil {
			return nil
		}
		return ui.NuevoBoton(v.Traduce("errors.retry"), v.AlReintentar).Con(ui.Secundario, ui.TallaSM)
	}
	switch v.Estado {
	case Lista:
		return nil
	case Cargando:
		return Esqueletos(v, 4)
	case Vacia:
		if vacio == nil {
			return nil
		}
		return vacio
	case Error:
		detalle := v.Detalle
		if detalle == "" {
			detalle = v.Traduce("errors.description")
		}
		return Vacio(v, glifoVideo, v.Traduce("errors.title"), detalle, reintentar())
	case SinConexion:
		return Vacio(v, glifoVideo, v.Traduce("errors.offline.title"), v.Traduce("errors.offline.description"), reintentar())
	case SinPermiso:
		return Vacio(v, glifoVideo, v.Traduce("errors.forbidden.title"), v.Traduce("errors.forbidden.description"), nil)
	case SinSesion:
		entrar := ui.NuevoBoton(v.Traduce("account.signIn"), v.AlEntrar).Con(ui.Primario, ui.TallaSM)
		return Vacio(v, glifoVideo, v.Traduce("account.signedOutTitle"), v.Traduce("account.signedOutDescription"), entrar)
	}
	return nil
}

// Esqueletos es la rejilla mientras carga: tarjetas grises del tamaño de las de
// verdad, para que la página no dé un salto cuando llegan.
func Esqueletos(v Vista, n int) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, n)
	for i := range hijos {
		poster := ui.NuevoEsqueleto(0, 0)
		poster.Radio = radioTarjeta()
		hijos[i] = container.New(&esqueletoLayout{},
			poster, ui.NuevoEsqueleto(0, 0), ui.NuevoEsqueleto(0, 0), ui.NuevoEsqueleto(0, 0))
	}
	return Rejilla(v, hijos...)
}

// esqueletoLayout reparte el póster, la línea del título y la de los datos como en una
// tarjeta.
type esqueletoLayout struct{}

func (esqueletoLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (esqueletoLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	alto := t.Width * 9 / 16
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, alto))
	escalon := escalonTituloTarjeta()
	y := alto + HuecoPieTarjeta + (escalon.Linea-escalon.Tam)/2
	objs[1].Move(fyne.NewPos(0, y))
	objs[1].Resize(fyne.NewSize(t.Width*0.8, escalon.Tam))
	y = alto + HuecoPieTarjeta + 2*escalon.Linea + tema.Espacio(1)
	objs[2].Move(fyne.NewPos(0, y))
	objs[2].Resize(fyne.NewSize(t.Width*0.5, tema.TextoXS.Tam))
	y += tema.TextoXS.Linea + tema.Espacio(1)
	objs[3].Move(fyne.NewPos(0, y))
	objs[3].Resize(fyne.NewSize(t.Width*0.6, tema.TextoXS.Tam))
}

// transparente es el borde de una caja que no lo lleva.
func transparente() color.NRGBA { return color.NRGBA{} }

// ---- Insignias ----

// InsigniaDirecto es la marca roja «EN DIRECTO» con el punto: en la tarjeta, en la
// ficha y en el reproductor.
func InsigniaDirecto(v Vista) *ui.Insignia {
	return ui.NuevaInsignia(v.Traduce("live.badge"), ui.InsigniaPeligro)
}
