// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Cast en el teléfono
//
// Las pantallas son las mismas que en escritorio (ya se colocan por el ancho de la
// ventana, como la web). Lo que cambia en un teléfono es lo que no existe en un
// escritorio, y está aquí:
//
//   - «Atrás» de Android vuelve a la página anterior; en la primera, la app al fondo.
//   - Pantalla completa es el reproductor a toda la pantalla, por encima de la barra de
//     la suite, y se entra y se sale GIRANDO el teléfono.
//   - La pantalla no se apaga mientras se ve un vídeo o un directo, o se emite.
//   - Al irse al fondo el vídeo se pausa y el directo que se miraba se cuelga (vuelve a
//     entrar al volver): el sistema puede matar la app ahí y nadie la está mirando.
//   - Emitir cambia entre la cámara delantera y la trasera, y espera al permiso del
//     sistema en vez de darlo por negado.

// estadoMovil es lo que App guarda solo para el teléfono.
type estadoMovil struct {
	// historial son las páginas por las que se ha pasado, para «atrás».
	historial []lugar
	volviendo bool
	// medido y tumbado: la postura del teléfono la última vez que se colocó la ventana.
	medido, tumbado bool
	// enPiP: la app está (o se ha pedido que esté) en la esquina del sistema; y lo último
	// que se le dijo al sistema de ella (prepararPiPMovil).
	enPiP        bool
	pipPreparada bool
	pipW, pipH   int
	// hueco es el sitio del teclado bajo la página (ui.MostrarFoco).
	hueco ui.HuecoTeclado
	// ampliado es la capa del reproductor a pantalla completa; nil, no lo está.
	ampliado *capaAmpliada
	// encendida es lo último que se pidió al sistema sobre apagar la pantalla.
	encendida bool
	// directoEnPausa es el directo que se colgó al irse al fondo, para volver a entrar.
	directoEnPausa string
	// camaraID es la cámara elegida para emitir; "" es la del sistema (la delantera).
	camaraID                     string
	esperandoCamara, microPedido bool
}

// maxHistorial son las páginas que «atrás» recuerda. Más atrás no vuelve nadie.
const maxHistorial = 30

// lugar es una página a la que volver: la ruta y lo que la abre (el vídeo, el canal).
type lugar struct {
	ruta  Ruta
	param string
}

func (a *App) lugarActual() lugar {
	switch a.ruta {
	case RutaVer:
		return lugar{a.ruta, a.ver.id}
	case RutaGestionar:
		return lugar{a.ruta, a.ges.id}
	case RutaCanal:
		return lugar{a.ruta, a.can.org}
	case RutaDesdeDrive:
		return lugar{a.ruta, a.dri.id}
	default:
		return lugar{ruta: a.ruta}
	}
}

// apuntar guarda la página actual antes de ir a `destino`, si es otra. La primera
// pantalla (gen 0) no se guarda: antes de ella no hay nada.
func (a *App) apuntar(destino lugar) {
	if a.movil.volviendo || a.gen == 0 {
		return
	}
	actual := a.lugarActual()
	if actual == destino {
		return
	}
	a.movil.historial = append(a.movil.historial, actual)
	if n := len(a.movil.historial); n > maxHistorial {
		a.movil.historial = a.movil.historial[n-maxHistorial:]
	}
}

// atras es el botón «atrás» sin nada encima que cerrar: la página anterior. Devuelve
// false en la primera, y entonces la app va al fondo (ui.AlAtras).
func (a *App) atras() bool {
	if a.movil.ampliado != nil {
		a.salirDeAmpliado(true)
		return true
	}
	n := len(a.movil.historial)
	if n == 0 {
		return false
	}
	l := a.movil.historial[n-1]
	a.movil.historial = a.movil.historial[:n-1]
	a.movil.volviendo = true
	defer func() { a.movil.volviendo = false }()
	a.irA(l)
	return true
}

func (a *App) irA(l lugar) {
	switch l.ruta {
	case RutaVer:
		a.AbrirVideo(l.param)
	case RutaGestionar:
		a.AbrirGestionar(l.param)
	case RutaCanal:
		a.AbrirCanal(l.param)
	case RutaDesdeDrive:
		a.AbrirDesdeDrive(l.param)
	default:
		a.Ir(l.ruta)
	}
}

// ---- Pantalla completa ----

// capaAmpliada es el reproductor a toda la pantalla: una capa del lienzo, para quedar
// también por encima de la barra de la suite. «Atrás» y Escape la cierran (Descartar).
//
// Es un widget y no un contenedor envuelto: el recorrido de Fyne solo entra en
// `*fyne.Container` y en `fyne.Widget`, y un tipo propio con un contenedor dentro no se
// pintaba (visto en el emulador: la página sin vídeo y nada encima).
type capaAmpliada struct {
	widget.BaseWidget
	a     *App
	fondo *canvas.Rectangle
	// barra es el fondo de la barra de mandos: sus glifos y el deslizador van en los
	// tokens de la página, y sobre el negro no se veían (visto en el emulador).
	barra *canvas.Rectangle
	visor fyne.CanvasObject
}

func nuevaCapaAmpliada(a *App) *capaAmpliada {
	c := &capaAmpliada{
		a: a, fondo: canvas.NewRectangle(color.NRGBA{A: 255}),
		barra: canvas.NewRectangle(tema.Color("bg", varianteActual())),
	}
	c.ExtendBaseWidget(c)
	return c
}

// Descartar es lo que hace «atrás»: volver a la página.
func (c *capaAmpliada) Descartar() { c.a.salirDeAmpliado(true) }

func (c *capaAmpliada) CreateRenderer() fyne.WidgetRenderer { return &capaAmpliadaRenderer{c: c} }

type capaAmpliadaRenderer struct{ c *capaAmpliada }

func (r *capaAmpliadaRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *capaAmpliadaRenderer) Destroy()           {}
func (r *capaAmpliadaRenderer) Refresh() {
	r.c.barra.FillColor = tema.Color("bg", varianteActual())
	r.c.barra.Refresh()
	r.Layout(r.c.Size())
}

func (r *capaAmpliadaRenderer) Objects() []fyne.CanvasObject {
	if r.c.visor == nil {
		return []fyne.CanvasObject{r.c.fondo}
	}
	return []fyne.CanvasObject{r.c.fondo, r.c.barra, r.c.visor}
}

// Layout pone el negro en toda la capa y el vídeo dentro de la zona segura: con el
// teléfono tumbado, la barra de gestos de Android tapaba los mandos.
func (r *capaAmpliadaRenderer) Layout(t fyne.Size) {
	r.c.fondo.Move(fyne.NewPos(0, 0))
	r.c.fondo.Resize(t)
	if r.c.visor == nil {
		return
	}
	if r.c.a.enEsquina() {
		// En la esquina del sistema no se toca nada de la app (los mandos son los del
		// sistema): solo el vídeo, y la barra de mandos fuera, por debajo del borde.
		r.c.barra.Hide()
		r.c.visor.Move(fyne.NewPos(0, 0))
		r.c.visor.Resize(fyne.NewSize(t.Width, t.Height+altoBarraVisor))
		return
	}
	r.c.barra.Show()
	pos, tam := fyne.NewPos(0, 0), t
	if c := fyne.CurrentApp().Driver().CanvasForObject(r.c); c != nil {
		if p, z := c.InteractiveArea(); z.Width > 0 && z.Height > 0 {
			pos, tam = p, fyne.NewSize(min(z.Width, t.Width-p.X), min(z.Height, t.Height-p.Y))
		}
	}
	r.c.visor.Move(pos)
	r.c.visor.Resize(tam)
	r.c.barra.Move(fyne.NewPos(0, pos.Y+tam.Height-altoBarraVisor))
	r.c.barra.Resize(fyne.NewSize(t.Width, t.Height-(pos.Y+tam.Height-altoBarraVisor)))
}

func (a *App) alternarAmpliado() {
	if a.movil.ampliado != nil {
		a.salirDeAmpliado(true)
		return
	}
	a.entrarAmpliado()
}

func (a *App) entrarAmpliado() {
	if a.movil.ampliado != nil || a.visor == nil || a.w == nil || a.ruta != RutaVer {
		return
	}
	capa := nuevaCapaAmpliada(a)
	a.movil.ampliado = capa
	a.pintar() // saca el visor de la página: un objeto no puede estar en dos sitios
	capa.visor = a.visor.Objeto()
	a.w.Canvas().Overlays().Add(capa)
	// El lienzo del móvil no le da tamaño a una capa que no lo pide: sin esto quedaba de
	// 0×0, invisible, y la página sin su vídeo (visto en el emulador al tumbarlo).
	capa.Resize(a.w.Canvas().Size())
}

// salirDeAmpliado quita la capa. `repintar` vuelve a poner el visor en la página; al
// salir de la página no hace falta, porque se pinta otra.
func (a *App) salirDeAmpliado(repintar bool) {
	capa := a.movil.ampliado
	if capa == nil {
		return
	}
	a.movil.ampliado = nil
	if a.w != nil {
		a.w.Canvas().Overlays().Remove(capa)
	}
	capa.visor = nil
	if repintar && a.ruta == RutaVer {
		a.pintar()
	}
}

// orientacion mira si el teléfono se ha girado: tumbado en la página de un vídeo, el
// reproductor a pantalla completa; de pie otra vez, vuelve a la página. Se llama al
// colocar la ventana, así que lo de cambiar de capa va después.
func (a *App) orientacion(t fyne.Size) {
	if !a.Movil || t.Width <= 0 || t.Height <= 0 {
		return
	}
	if c := a.movil.ampliado; c != nil && c.Size() != t {
		c.Resize(t) // al girar, la capa sigue a la pantalla
	}
	tumbado := t.Width > t.Height
	if a.movil.medido && tumbado == a.movil.tumbado {
		return
	}
	primera := !a.movil.medido
	a.movil.medido, a.movil.tumbado = true, tumbado
	if primera || a.ruta != RutaVer || a.visor == nil {
		return
	}
	go a.enLaInterfaz(func() {
		if tumbado {
			a.entrarAmpliado()
		} else {
			a.salirDeAmpliado(true)
		}
	})
}

// ---- La pantalla encendida y el fondo ----

// actualizarEncendido pide que la pantalla no se apague mientras algo se mueve solo:
// un vídeo que se reproduce, un directo que se mira o uno que se emite.
func (a *App) actualizarEncendido() {
	a.prepararPiPMovil()
	if a.PantallaEncendida == nil {
		return
	}
	quiere := (a.reproductor != nil && !a.reproductor.Pausado()) || a.sala != nil || a.emiSala != nil
	if quiere != a.movil.encendida {
		a.movil.encendida = quiere
		a.PantallaEncendida(quiere)
	}
}

// AlFondo es lo que pasa al dejar la app en el teléfono: el vídeo se pausa y el
// directo que se miraba se cuelga (AlVolver vuelve a entrar). Lo que se EMITE sigue:
// cortarlo por mirar una notificación sería peor.
func (a *App) AlFondo() {
	if !a.Movil {
		return
	}
	if a.enEsquina() {
		// Se ha ido a la esquina (el botón, o sola al salir con el vídeo en marcha): el
		// vídeo sigue, a pantalla completa en la ventana pequeña.
		a.movil.enPiP = true
		a.entrarAmpliado()
		if c := a.movil.ampliado; c != nil {
			c.Refresh()
		}
		return
	}
	if r := a.reproductor; r != nil && !r.Pausado() {
		r.Alternar()
		if a.visor != nil {
			a.visor.Rehacer()
		}
	}
	if a.sala != nil && a.ruta == RutaVer {
		s := a.sala
		a.sala = nil
		a.movil.directoEnPausa = a.ver.id
		go s.Colgar()
		if a.visor != nil {
			a.visor.Aviso(a.v.Traduce("live.connecting"), true)
		}
	}
	a.actualizarEncendido()
}

// AlVolver vuelve a entrar en el directo que se colgó al irse al fondo, y devuelve a la
// página el vídeo que volvía de la esquina.
func (a *App) AlVolver() {
	if a.movil.enPiP && !a.enEsquina() {
		a.movil.enPiP = false
		if !a.movil.tumbado {
			a.salirDeAmpliado(true)
		} else if c := a.movil.ampliado; c != nil {
			c.Refresh()
		}
	}
	id := a.movil.directoEnPausa
	a.movil.directoEnPausa = ""
	if id == "" || a.ruta != RutaVer || a.ver.id != id {
		return
	}
	a.movil.volviendo = true
	defer func() { a.movil.volviendo = false }()
	a.AbrirVideo(id)
}

// ---- Emitir desde el teléfono ----

// CamaraElegida es la cámara con la que se emite; "" es la del sistema.
func (a *App) CamaraElegida() string { return a.movil.camaraID }

// cambioDeCamara es el botón de cambiar de cámara, o nil si no hay a cuál cambiar.
func (a *App) cambioDeCamara() func() {
	if !a.Movil || a.Camaras == nil || len(a.Camaras()) < 2 {
		return nil
	}
	return a.cambiarCamara
}

// cambiarCamara pasa a la siguiente cámara (en un teléfono, de la delantera a la trasera
// y vuelta) y la enciende en su lugar: en la vista previa o en la sala.
func (a *App) cambiarCamara() {
	cs := a.Camaras()
	if len(cs) < 2 {
		return
	}
	actual := -1
	for i, c := range cs {
		if c.ID == a.movil.camaraID || (a.movil.camaraID == "" && c.Nombre == "front") {
			actual = i
			break
		}
	}
	a.movil.camaraID = cs[(actual+1)%len(cs)].ID
	ah := &a.emi.ahora
	if !ah.camara {
		a.pintar()
		return
	}
	switch ah.fase {
	case EmisionEnDirecto:
		if a.emiSala != nil {
			_ = a.emiSala.Camara(false)
			if err := a.emiSala.Camara(true); err != nil {
				ah.camara, ah.denegado = false, true
			}
		}
		a.pintar()
	case EmisionPreparando, EmisionFallida:
		ah.camara = false
		a.soltarDePrevia(false)
		a.alternarFuente(false)
	case EmisionConectando:
	}
}

// camaraDelantera dice si la cámara elegida mira hacia quien la sostiene, que es la que
// se ve en espejo. En escritorio (sin nombres de lado) lo es: es la del portátil.
func (a *App) camaraDelantera() bool {
	if a.Camaras == nil {
		return true
	}
	cs := a.Camaras()
	for _, c := range cs {
		if c.ID == a.movil.camaraID {
			return !esTrasera(c.Nombre)
		}
	}
	return true // la del sistema: la delantera en el teléfono
}

// esTrasera reconoce la trasera por su nombre: «back» en Android (Camara.java) y «Back
// Camera»/«Cámara trasera» en iOS.
func esTrasera(nombre string) bool {
	n := strings.ToLower(nombre)
	return strings.Contains(n, "back") || strings.Contains(n, "trasera") || strings.Contains(n, "rear")
}

// esperaPermiso es cuánto se espera a que alguien conteste al diálogo del sistema.
const esperaPermiso = time.Minute

// esperarPermisoCamara espera a que se conceda la cámara: en Android el primer intento
// abre el diálogo del sistema y vuelve «sin permiso» en el acto, antes de que nadie
// conteste. Mientras tanto se enseña el aviso; si se concede, la cámara se enciende sola
// y el aviso se va.
func (a *App) esperarPermisoCamara(gen int) {
	if !a.Movil || a.CamaraConcedida == nil || a.movil.esperandoCamara {
		return
	}
	a.movil.esperandoCamara = true
	concedida := a.CamaraConcedida
	go func() {
		ok := false
		for limite := time.Now().Add(esperaPermiso); time.Now().Before(limite); {
			if concedida() {
				ok = true
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		a.enLaInterfaz(func() {
			a.movil.esperandoCamara = false
			ah := &a.emi.ahora
			if ok && gen == a.gen && !ah.camara && (ah.fase == EmisionPreparando || ah.fase == EmisionFallida) {
				a.alternarFuente(false)
			}
		})
	}()
}

// pedirMicro pide el permiso del micrófono una vez: sin él Android graba silencio sin
// dar ningún error.
func (a *App) pedirMicro() {
	if a.PedirMicro == nil || a.movil.microPedido {
		return
	}
	a.movil.microPedido = true
	pedir := a.PedirMicro
	a.lanzar(func() { _ = pedir() })
}

// soltarCopia borra la copia que el selector del teléfono dejó de un vídeo elegido
// cuando ya no hace falta: un vídeo de 2 GB copiado en la carpeta temporal y olvidado
// es espacio que el teléfono no recupera.
func (a *App) soltarCopia(ruta string) {
	if a.SoltarCopia != nil {
		a.SoltarCopia(ruta)
	}
}

// MostrarCampo deja el campo en el que se escribe por encima del teclado, que tapa
// `tapado` unidades desde abajo (ui.AlEscribir lo llama al enfocarse un campo).
func (a *App) MostrarCampo(tapado float32) {
	if a.w != nil {
		ui.MostrarFoco(a.scroll, a.w.Canvas(), tapado)
	}
}

// ---- Imagen dentro de imagen del sistema ----

// PiPSistema es la imagen dentro de imagen del teléfono: la cumple sesion.ImagenEnImagen.
type PiPSistema interface {
	Disponible() bool
	Activa() bool
	Preparar(ancho, alto int, reproduciendo bool)
	Entrar(reproduciendo bool) bool
}

// enEsquina dice si la app está ahora en la ventana pequeña del sistema.
func (a *App) enEsquina() bool { return a.PiPMovil != nil && a.PiPMovil.Activa() }

// pipMovil es el botón de imagen dentro de imagen en el teléfono: la app entera a la
// esquina, con el vídeo a pantalla completa.
func (a *App) pipMovil() {
	if a.PiPMovil == nil || a.reproductor == nil || a.ruta != RutaVer {
		return
	}
	a.entrarAmpliado()
	if a.PiPMovil.Entrar(!a.reproductor.Pausado()) {
		a.movil.enPiP = true
	}
}

// prepararPiPMovil le dice al sistema la proporción del vídeo y si se reproduce: el botón
// de la ventana pequeña y, en Android 12+, entrar sola al salir de la app. Fuera de la
// página de un vídeo, o en pausa, no entra sola.
func (a *App) prepararPiPMovil() {
	if a.PiPMovil == nil {
		return
	}
	suena := a.ruta == RutaVer && a.reproductor != nil && !a.reproductor.Pausado()
	w, h := 16, 9
	if a.visor != nil && a.visor.imagen.Image != nil {
		if b := a.visor.imagen.Image.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
			w, h = b.Dx(), b.Dy()
		}
	}
	if suena == a.movil.pipPreparada && w == a.movil.pipW && h == a.movil.pipH {
		return
	}
	a.movil.pipPreparada, a.movil.pipW, a.movil.pipH = suena, w, h
	a.PiPMovil.Preparar(w, h, suena)
}

// SucesoPiP es lo que llega de la ventana pequeña: su botón de reproducir o pausar, o que
// la han cerrado (el vídeo se pausa: ya no lo ve nadie).
func (a *App) SucesoPiP(cerrada bool) {
	if cerrada {
		a.movil.enPiP = false
		if r := a.reproductor; r != nil && !r.Pausado() {
			r.Alternar()
		}
		a.salirDeAmpliado(true)
	} else if a.visor != nil {
		a.visor.alternar()
	}
	if a.visor != nil {
		a.visor.Rehacer()
	}
	a.actualizarEncendido()
}
