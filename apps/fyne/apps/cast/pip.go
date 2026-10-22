// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/video"
)

// # Imagen dentro de imagen
//
// El botón de imagen dentro de imagen del reproductor (y la tecla P) saca el vídeo a
// una ventana pequeña propia, como el `requestPictureInPicture` del navegador en la
// web. Solo bajo demanda, como allí: el reproductor del directo no lo ofrece.
//
// La ventana se queda con el MISMO reproductor —no se abre otro—, así que el sonido y
// la posición siguen sin un corte; lo único que cambia es dónde se pintan los
// fotogramas. Mientras está abierta, navegar por la app no la para (salir de Ver ya no
// suelta ese reproductor, que es de la ventana). Al cerrarla el vídeo vuelve a su sitio
// en Ver, en pausa, como hace el navegador con la X; «Volver a la ventana principal»
// lo devuelve sonando y, si se está en otra pantalla, abre la del vídeo.
//
// SIEMPRE ENCIMA: Fyne 2.8 (y el fork de la suite) tiene `desktop.Window
// .RequestAlwaysOnTop`, el `GLFW_FLOATING` de GLFW, y se pide antes de enseñarla; el
// gestor de ventanas puede no hacerle caso (lo dice la propia API). SIN BORDES, no: la
// ventana sin decoración de Fyne (CreateSplashWindow) no se puede mover ni cambiar de
// tamaño con el ratón, y moverla a mano con RequestPosition no funciona en Wayland. Es
// una ventana normal, pequeña y redimensionable, que mueve el gestor de ventanas.

// altoBarraPiP es la barra de mandos de la ventana (`h-11`, la del reproductor).
var altoBarraPiP = tema.Espacio(11)

// Tamaño con el que nace la ventana: 16:9 de 480 más la barra.
var tamPiP = fyne.NewSize(480, 270+altoBarraPiP)

// ventanaPiP es la ventana de imagen dentro de imagen y lo que suena en ella.
type ventanaPiP struct {
	a      *App
	w      fyne.Window
	id     string
	r      *video.Reproductor
	f      Fuente
	imagen *canvas.Image
	barra  *fyne.Container
	// Lo de la calidad que tenía la página, para devolvérselo al volver.
	calidad, enUso int
	medida         *fuenteMedida
}

// alternarPiP abre o cierra la ventana (el botón y la tecla P).
func (a *App) alternarPiP() {
	if a.Movil {
		return // la tecla P con un teclado en el teléfono: no hay ventana que abrir
	}
	if a.pip != nil && a.pip.id == a.ver.id {
		a.cerrarPiP(true)
		return
	}
	a.abrirPiP()
}

// abrirPiP saca a su ventana el vídeo que suena en Ver.
func (a *App) abrirPiP() {
	if a.ruta != RutaVer || a.reproductor == nil || a.ver.video.EnDirecto() || fyne.CurrentApp() == nil {
		return
	}
	if a.pip != nil {
		a.cerrarPiP(false) // la de otro vídeo: una sola a la vez
	}
	w := fyne.CurrentApp().NewWindow(a.ver.video.Title)
	if d, ok := w.(desktop.Window); ok {
		d.RequestAlwaysOnTop()
	}
	w.SetPadded(false)
	p := &ventanaPiP{
		a: a, w: w, id: a.ver.id, r: a.reproductor, f: a.fuente,
		calidad: a.ver.calidad, enUso: a.ver.enUso, medida: a.ver.medida,
	}
	// La ventana es ahora la dueña: salir de Ver ya no lo suelta.
	a.reproductor, a.fuente = nil, nil
	p.imagen = canvas.NewImageFromImage(nil)
	if a.visor != nil {
		p.imagen.Image = a.visor.imagen.Image
	}
	p.imagen.FillMode, p.imagen.ScaleMode = canvas.ImageFillContain, canvas.ImageScaleSmooth
	p.barra = container.NewWithoutLayout()
	fondo := canvas.NewRectangle(color.NRGBA{A: 255})
	w.SetContent(container.New(&pipLayout{p: p}, fondo, p.imagen, p.barra))
	w.Resize(tamPiP)
	w.SetCloseIntercept(func() { a.cerrarPiP(false) })
	w.Canvas().SetOnTypedKey(p.tecla)
	a.pip = p
	p.rehacer()
	a.conectar(p.r)
	if a.visor != nil {
		// El cuadro se queda en negro con el aviso: el vídeo está en la otra ventana.
		a.visor.Imagen(nil)
		a.visor.Aviso(a.v.Traduce("player.pipActive"), false)
		a.visor.Rehacer()
	}
	w.Show()
}

// cerrarPiP cierra la ventana y devuelve el vídeo. `volver` es «Volver a la ventana
// principal»: sigue sonando y, si hace falta, se abre su página. Sin él (la X, cerrar
// la ventana) vuelve en pausa a su sitio si su página está abierta; si no, se para.
func (a *App) cerrarPiP(volver bool) {
	p := a.pip
	if p == nil {
		return
	}
	a.pip = nil
	enSuPagina := a.ruta == RutaVer && a.ver.id == p.id && a.visor != nil
	switch {
	case enSuPagina:
		a.devolver(p, volver)
	case volver:
		// Se abre su página y, cuando esté, recoge este mismo reproductor
		// (abrirReproductor) en vez de abrir otro.
		a.pendiente = p
		a.AbrirVideo(p.id)
	default:
		p.r.Cerrar()
		_ = p.f.Close()
	}
	if volver && a.w != nil {
		a.w.RequestFocus()
	}
	p.w.Close()
}

// devolver pone el reproductor de la ventana en el visor de la página.
func (a *App) devolver(p *ventanaPiP, sonando bool) {
	if !sonando {
		p.r.Pausar()
	}
	a.reproductor, a.fuente = p.r, p.f
	a.ver.calidad, a.ver.enUso, a.ver.medida = p.calidad, p.enUso, p.medida
	a.visor.Aviso("", false)
	if p.imagen.Image != nil {
		a.visor.Imagen(p.imagen.Image)
	}
	a.conectar(p.r)
}

// rehacer arma la barra: reproducir o pausa a la izquierda; volver y cerrar a la derecha.
func (p *ventanaPiP) rehacer() {
	v := p.a.v
	glifo, rotulo := glifoPausa, "player.pause"
	if p.r.Pausado() {
		glifo, rotulo = glifoPlay, "player.play"
	}
	p.barra.Objects = []fyne.CanvasObject{
		botonVisor(v, glifo, v.Traduce(rotulo), p.alternar, true),
		botonIconoVisor(iconos.ImagenEnImagen, v.Traduce("player.pipBack"), func() { p.a.cerrarPiP(true) }),
		botonIconoVisor(iconos.Cerrar, v.Traduce("player.pipClose"), func() { p.a.cerrarPiP(false) }),
	}
	p.barra.Layout = &barraVisor{directo: true, derecha: 2}
	p.barra.Refresh()
}

func (p *ventanaPiP) alternar() {
	p.r.Alternar()
	p.rehacer()
}

// tecla: espacio reproduce y pausa, P devuelve el vídeo y Escape cierra.
func (p *ventanaPiP) tecla(e *fyne.KeyEvent) {
	switch e.Name {
	case fyne.KeySpace:
		p.alternar()
	case fyne.KeyP:
		p.a.cerrarPiP(true)
	case fyne.KeyEscape:
		p.a.cerrarPiP(false)
	default:
	}
}

// pipLayout: el vídeo a todo lo que queda y la barra abajo. Al cambiar de tamaño,
// «Automática» vuelve a decidir con el alto de esta ventana.
type pipLayout struct {
	p      *ventanaPiP
	medido fyne.Size
}

func (l *pipLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(10*tema.Rem, 5*tema.Rem+altoBarraPiP)
}

func (l *pipLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	cuadro := fyne.NewSize(t.Width, max(t.Height-altoBarraPiP, 0))
	for _, o := range objs[:2] {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(cuadro)
	}
	objs[2].Move(fyne.NewPos(0, cuadro.Height))
	objs[2].Resize(fyne.NewSize(t.Width, altoBarraPiP))
	if cuadro != l.medido && l.p.a.pip == l.p {
		l.medido = cuadro
		l.p.a.medidoVisor(cuadro)
	}
}
