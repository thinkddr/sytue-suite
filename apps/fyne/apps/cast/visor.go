// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"image"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Mando es lo que el visor le pide al reproductor. Lo cumple `video.Reproductor`; los
// tests ponen uno de mentira. En directo no hay mando: solo se mira.
type Mando interface {
	Alternar()
	Buscar(time.Duration)
	Posicion() time.Duration
	Duracion() time.Duration
	Pausado() bool
	Silenciar(bool)
	Silenciado() bool
	Volumen(float64)
}

// Visor es el reproductor de la página de un vídeo (Player y LivePlayer de la web): el
// cuadro negro 16:9 con la imagen y, debajo, la barra de mandos.
//
// Vive en App y no se rehace al repintar la página: los fotogramas le llegan 30 veces
// por segundo y rehacer la página cada vez sería absurdo. Se rehace solo su barra.
type Visor struct {
	v Vista

	// Mando es el reproductor de un vídeo bajo demanda; nil, sin mandos de tiempo.
	Mando Mando
	// Directo pinta la marca «EN DIRECTO» y los espectadores en vez de la barra de tiempo.
	Directo    bool
	Viendo     int
	AlAmpliar  func()
	AlSilencio func()
	// AlCalidad abre el menú de calidad colgado de su botón; nil, sin menú (un
	// directo, o un vídeo con una sola calidad).
	AlCalidad func(disparador fyne.CanvasObject)
	// AlPiP saca el vídeo a la ventana de imagen dentro de imagen; nil, sin botón.
	AlPiP func()
	// AlMedir avisa del tamaño del cuadro (en unidades de Fyne) cuando cambia: al
	// redimensionar la ventana o pasar a pantalla completa. Lo usa la calidad
	// «Automática».
	AlMedir func(fyne.Size)

	// volumen es el del deslizador, de 0 a 1 (el reproductor no lo devuelve).
	volumen float64

	medido fyne.Size

	fondo    *canvas.Rectangle
	imagen   *canvas.Image
	aviso    fyne.CanvasObject
	barra    *fyne.Container
	toque    *toque
	raiz     *fyne.Container
	tiempo   *canvas.Text
	progreso *ui.Deslizador
	boton    *ui.BotonIcono
}

// NuevoVisor crea el visor vacío (negro, con el póster si se le pone).
func NuevoVisor(v Vista) *Visor {
	x := &Visor{v: v, volumen: 1}
	x.fondo = canvas.NewRectangle(color.NRGBA{A: 255})
	x.fondo.CornerRadius = tema.Forma("radius-card")
	x.imagen = canvas.NewImageFromImage(nil)
	x.imagen.CornerRadius = tema.Forma("radius-card")
	x.imagen.FillMode = canvas.ImageFillContain
	x.imagen.ScaleMode = canvas.ImageScaleSmooth
	x.barra = container.NewWithoutLayout()
	// Tocar el vídeo lo pausa o lo sigue, como en cualquier reproductor de teléfono (y
	// como el clic sobre el <video> de la web).
	x.toque = &toque{al: x.alternar}
	x.toque.ExtendBaseWidget(x.toque)
	x.raiz = container.New(&visorLayout{x: x}, x.fondo, x.imagen, x.toque, x.barra)
	return x
}

// Objeto es lo que se cuelga de la página.
func (x *Visor) Objeto() fyne.CanvasObject { return x.raiz }

// Imagen pone el fotograma (o el póster). En el hilo de la interfaz.
func (x *Visor) Imagen(img image.Image) {
	x.imagen.Image = img
	x.imagen.Refresh()
}

// Medido es el tamaño del cuadro en el último reparto (unidades de Fyne).
func (x *Visor) Medido() fyne.Size { return x.medido }

// TieneImagen dice si hay algo pintado.
func (x *Visor) TieneImagen() bool { return x.imagen.Image != nil }

// Aviso pone un mensaje encima del cuadro negro («procesando», «el directo no ha
// empezado»). Vacío lo quita. `rueda` añade el indicador de trabajo.
func (x *Visor) Aviso(texto string, rueda bool) {
	if len(x.raiz.Objects) > 4 {
		x.raiz.Objects = x.raiz.Objects[:4]
	}
	x.aviso = nil
	if texto != "" {
		blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 230}
		p := ui.NuevoParrafoConTinta(texto, tema.TextoSM, ui.Medio, blanco)
		hijos := []fyne.CanvasObject{}
		if rueda {
			hijos = append(hijos, ui.NuevaRuedaCon(ui.LadoRueda, blanco))
		}
		hijos = append(hijos, p)
		x.aviso = container.New(&avisoVisor{}, hijos...)
		x.raiz.Objects = append(x.raiz.Objects, x.aviso)
	}
	x.raiz.Refresh()
}

// Rehacer vuelve a armar la barra de mandos con el estado del mando: se llama al
// cambiar pausa, silencio o espectadores, y cada medio segundo para el tiempo.
func (x *Visor) Rehacer() {
	v := x.v
	var hijos []fyne.CanvasObject
	if x.Mando != nil && !x.Directo {
		glifo, rotulo := glifoPausa, "player.pause"
		if x.Mando.Pausado() {
			glifo, rotulo = glifoPlay, "player.play"
		}
		x.boton = botonVisor(v, glifo, v.Traduce(rotulo), x.Mando.Alternar, true)
		d := x.Mando.Duracion()
		x.progreso = ui.NuevoDeslizador(0, max(d.Seconds(), 0.001), x.Mando.Posicion().Seconds(), func(s float64) {
			x.Mando.Buscar(time.Duration(s * float64(time.Second)))
		}).Con(v.Traduce("player.seek"))
		x.progreso.Paso = 5
		x.tiempo = ui.Texto(x.textoTiempo(), tema.TextoXS, ui.Medio, tema.Color("text-muted", v.Variante))
		hijos = append(hijos, x.boton, x.progreso, x.tiempo)
	} else if x.Directo {
		hijos = append(hijos, InsigniaDirecto(v),
			ui.Texto(v.Traduce("live.viewers", i18n.Vars{"count": x.Viendo}), tema.TextoXS, ui.Medio, tema.Color("text-muted", v.Variante)))
	}
	silencio := x.Mando != nil && x.Mando.Silenciado()
	glifo, rotulo := glifoVolumen, "player.mute"
	if silencio {
		glifo, rotulo = glifoSilencio, "player.unmute"
	}
	derecha := []fyne.CanvasObject{botonVisor(v, glifo, v.Traduce(rotulo), x.AlSilencio, x.AlSilencio != nil)}
	var volumen fyne.CanvasObject
	if x.Mando != nil {
		vol := ui.NuevoDeslizador(0, 1, x.volumen, func(f float64) {
			x.volumen = f
			x.Mando.Volumen(f)
		}).Con(v.Traduce("player.volume"))
		vol.Paso = 0.05
		volumen = container.New(&anchoFijo{ancho: 5 * tema.Rem}, vol)
		derecha = append(derecha, volumen)
	}
	if x.AlCalidad != nil {
		var b *ui.BotonIcono
		b = botonIconoVisor(iconos.Ajustes, v.Traduce("player.quality"), func() { x.AlCalidad(b) })
		derecha = append(derecha, b)
	}
	if x.AlPiP != nil {
		derecha = append(derecha, botonIconoVisor(iconos.ImagenEnImagen, v.Traduce("player.pip"), x.AlPiP))
	}
	derecha = append(derecha, botonVisor(v, glifoAmpliar, v.Traduce("player.fullscreen"), x.AlAmpliar, x.AlAmpliar != nil))
	hijos = append(hijos, derecha...)
	x.barra.Objects = hijos
	x.barra.Layout = &barraVisor{directo: x.Directo || x.Mando == nil, derecha: len(derecha), volumen: volumen}
	x.barra.Refresh()
	x.raiz.Refresh()
}

// alternar es tocar el vídeo: pausa o sigue. En directo no hay nada que pausar.
func (x *Visor) alternar() {
	if x.Mando == nil || x.Directo {
		return
	}
	x.Mando.Alternar()
	x.Rehacer()
}

// toque es la superficie del vídeo que se pulsa.
type toque struct {
	widget.BaseWidget
	al func()
}

func (t *toque) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

// Tapped pausa o sigue.
func (t *toque) Tapped(*fyne.PointEvent) {
	if t.al != nil {
		t.al()
	}
}

// Avanzar actualiza el tiempo y la barra de progreso sin rehacer nada.
func (x *Visor) Avanzar() {
	if x.Mando == nil || x.tiempo == nil {
		return
	}
	x.tiempo.Text = x.textoTiempo()
	x.tiempo.Refresh()
	x.progreso.Valor = x.Mando.Posicion().Seconds()
	x.progreso.Refresh()
	x.barra.Refresh()
}

func (x *Visor) textoTiempo() string {
	return Duracion(x.Mando.Posicion()) + " / " + Duracion(x.Mando.Duracion())
}

// botonVisor es un mando cuadrado de la barra (el IconButton de la web, talla LG).
func botonVisor(v Vista, glifo, rotulo string, al func(), activo bool) *ui.BotonIcono {
	b := ui.NuevoBotonIcono(iconos.Ninguno, rotulo, al)
	lleno := glifo == glifoPlay || glifo == glifoPausa
	tinta := tema.Color("text", v.Variante)
	if lleno {
		b.Glifo = iconos.RellenoDeRuta(glifo, iconos.TallaMD, tinta)
	} else {
		b.Glifo = iconos.TrazoDeRuta(glifo, iconos.TallaMD, tinta)
	}
	b.Lado, b.TamIcono = ui.LadoIconoLG, iconos.TallaMD
	b.Inerte = !activo
	return b
}

// botonIconoVisor es un mando de la barra con un icono del catálogo compartido.
func botonIconoVisor(icono iconos.Icono, rotulo string, al func()) *ui.BotonIcono {
	b := ui.NuevoBotonIcono(icono, rotulo, al)
	b.Lado, b.TamIcono = ui.LadoIconoLG, iconos.TallaMD
	return b
}

// Medidas del visor: la barra es `h-11 px-2 gap-2`.
var (
	altoBarraVisor           = tema.Espacio(11)
	margenBarra              = tema.Espacio(2)
	anchoProgresoMin float32 = 6 * tema.Rem
)

// AltoVisor es lo que mide el visor a un ancho: el cuadro 16:9 más la barra.
func AltoVisor(ancho float32) float32 { return ancho*9/16 + altoBarraVisor }

type visorLayout struct{ x *Visor }

func (l *visorLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(16*tema.Rem, AltoVisor(16*tema.Rem))
}

// AltoCon: el del cuadro a ese ancho más la barra.
func (l *visorLayout) AltoCon(_ []fyne.CanvasObject, ancho float32) float32 { return AltoVisor(ancho) }

func (l *visorLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	alto := t.Height - altoBarraVisor
	cuadro := fyne.NewSize(t.Width, alto)
	if x := l.x; x != nil {
		radio := tema.Forma("radius-card")
		// En el teléfono el vídeo va a sangre; el resto conserva las esquinas
		// de tarjeta, también al cambiar la forma con el reproductor abierto.
		if x.v.Movil && x.v.Ventana < ui.CorteSM {
			radio = 0
		}
		x.fondo.CornerRadius, x.imagen.CornerRadius = radio, radio
	}
	if x := l.x; x != nil && cuadro != x.medido && cuadro.Width > 0 {
		x.medido = cuadro
		if x.AlMedir != nil {
			x.AlMedir(cuadro)
		}
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(cuadro)
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(cuadro)
	objs[2].Move(fyne.NewPos(0, 0))
	objs[2].Resize(cuadro)
	objs[3].Move(fyne.NewPos(0, alto))
	objs[3].Resize(fyne.NewSize(t.Width, altoBarraVisor))
	if len(objs) > 4 {
		objs[4].Move(fyne.NewPos(0, 0))
		objs[4].Resize(cuadro)
	}
}

// barraVisor: el primer mando a la izquierda, la barra de progreso estirada en medio,
// el tiempo y los mandos de la derecha a su tamaño. En directo no hay nada que estirar:
// la marca y los espectadores a la izquierda y los mandos a la derecha.
type barraVisor struct {
	directo bool
	// derecha es cuántos de los últimos hijos van a la derecha.
	derecha int
	// volumen es el deslizador del volumen: en un reproductor estrecho (un teléfono) no
	// va, como en la web (`hidden @lg:block`); el volumen es el de los botones del lado.
	volumen fyne.CanvasObject
}

// anchoConVolumen es desde dónde cabe el deslizador del volumen: el `@lg` del
// contenedor de la web (32rem).
const anchoConVolumen = 32 * tema.Rem

func (b *barraVisor) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(anchoProgresoMin, altoBarraVisor)
}

func (b *barraVisor) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	colocar := func(o fyne.CanvasObject, x float32) float32 {
		m := o.MinSize()
		if tx, ok := o.(*canvas.Text); ok {
			ui.ColocarTexto(tx, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
			return x + ui.Medir(tx.Text, tema.TextoXS, ui.Medio).Width
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		return x + m.Width
	}
	if len(objs) == 0 {
		return
	}
	n := min(max(b.derecha, 1), len(objs))
	derecha := objs[len(objs)-n:]
	izquierda := objs[:len(objs)-n]
	if b.volumen != nil {
		if t.Width < anchoConVolumen {
			b.volumen.Hide()
			derecha = sinObjeto(derecha, b.volumen)
		} else {
			b.volumen.Show()
		}
	}
	anchoDer := float32(0)
	for i, o := range derecha {
		if i > 0 {
			anchoDer += margenBarra
		}
		anchoDer += o.MinSize().Width
	}
	xd := t.Width - margenBarra - anchoDer
	xr := xd
	for _, o := range derecha {
		xr = colocar(o, xr) + margenBarra
	}
	x := margenBarra
	if b.directo || len(izquierda) < 3 {
		for _, o := range izquierda {
			x = colocar(o, x) + margenBarra
		}
		return
	}
	// Reproducir, progreso (estirado) y tiempo.
	x = colocar(izquierda[0], x) + margenBarra
	tiempo, ok := izquierda[2].(*canvas.Text)
	if !ok {
		return
	}
	anchoTiempo := ui.Medir(tiempo.Text, tema.TextoXS, ui.Medio).Width
	xt := xd - margenBarra - anchoTiempo
	colocar(tiempo, xt)
	p := izquierda[1]
	m := p.MinSize()
	p.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
	p.Resize(fyne.NewSize(max(xt-2*margenBarra-x, 0), m.Height))
}

// avisoVisor centra la rueda y el mensaje sobre el cuadro.
type avisoVisor struct{}

func (avisoVisor) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (avisoVisor) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	ancho := min(t.Width-2*tema.Espacio(6), 28*tema.Rem)
	texto, ok := objs[len(objs)-1].(*ui.Parrafo)
	if !ok {
		return
	}
	alto := ui.AltoParrafo(texto, ancho)
	y := (t.Height - alto) / 2
	if len(objs) > 1 {
		y -= (ui.LadoRueda + tema.Espacio(3)) / 2
		objs[0].Move(fyne.NewPos((t.Width-ui.LadoRueda)/2, y))
		objs[0].Resize(fyne.NewSize(ui.LadoRueda, ui.LadoRueda))
		y += ui.LadoRueda + tema.Espacio(3)
	}
	w := min(ui.Medir(texto.Texto, tema.TextoSM, ui.Medio).Width, ancho)
	texto.Move(fyne.NewPos((t.Width-w)/2, y))
	texto.Resize(fyne.NewSize(w, alto))
}

// sinObjeto es la lista sin ese objeto, en una copia.
func sinObjeto(objs []fyne.CanvasObject, fuera fyne.CanvasObject) []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, len(objs))
	for _, o := range objs {
		if o != fuera {
			out = append(out, o)
		}
	}
	return out
}
