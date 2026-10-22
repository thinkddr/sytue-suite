// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestAtrasVuelveALaPaginaAnterior: el «atrás» de Android recorre al revés las páginas
// por las que se pasó, con su vídeo o su canal, y en la primera deja ir la app al fondo.
// Volver a pintar la misma página (al entrar o salir de la sesión) no cuenta como paso.
//
// VISTO EN ROJO sin `apuntar` en AbrirVideo: atrás desde el canal se salta el vídeo.
func TestAtrasVuelveALaPaginaAnterior(t *testing.T) {
	a, _ := appDePrueba(t, servicioCompleto(), true)
	a.Arrancar()
	a.Ir(RutaExplorar)
	a.AbrirVideo("v01")
	a.Recargar()
	a.AbrirCanal("acme")
	for _, quiero := range []lugar{{RutaVer, "v01"}, {RutaExplorar, ""}, {RutaInicio, ""}} {
		if !a.atras() {
			t.Fatalf("atrás no vuelve a %+v", quiero)
		}
		if got := a.lugarActual(); got != quiero {
			t.Fatalf("atrás lleva a %+v y tocaba %+v", got, quiero)
		}
	}
	if a.atras() {
		t.Error("en la primera página atrás tiene que dejar ir la app al fondo")
	}
}

// TestElTelefonoTumbadoAmpliaElVideo: en la página de un vídeo, tumbar el teléfono pone
// el reproductor a toda la pantalla, por encima de la barra de la suite; ponerlo de pie
// o pulsar «atrás» lo devuelve a la página. El botón de pantalla completa hace lo mismo
// en el teléfono, en vez de pedírselo a la ventana.
//
// VISTO EN ROJO sin la llamada a `orientacion` en medirVentana: tumbado sigue la página.
func TestElTelefonoTumbadoAmpliaElVideo(t *testing.T) {
	a, w := appDePrueba(t, servicioCompleto(), true)
	vaciar := colaDeInterfaz(t, a)
	a.Movil = true
	w.Resize(fyne.NewSize(360, 760))
	_ = w.Canvas().Capture()
	a.AbrirVideo("v01")
	_ = w.Canvas().Capture()
	encima := func() bool {
		c, ok := w.Canvas().Overlays().Top().(*capaAmpliada)
		return ok && c.visor == a.visor.Objeto() && c.Size() == w.Canvas().Size()
	}
	if encima() {
		t.Fatal("de pie el vídeo ya está a pantalla completa")
	}
	w.Resize(fyne.NewSize(760, 360))
	_ = w.Canvas().Capture()
	vaciar("la pantalla completa al tumbarlo", encima)
	img := w.Canvas().Capture()
	guardar(t, "movil-ampliado", img)
	// Y se PINTA: en el centro de la pantalla, el negro del vídeo y no la página. Una capa
	// que Fyne no sabe recorrer pasa el `encima` y no se ve (visto en el emulador).
	if px := colorEn(img, img.Bounds().Dx()/2, img.Bounds().Dy()/3); px.R > 60 || px.G > 60 || px.B > 60 {
		t.Errorf("a pantalla completa el centro pinta %v y tenía que ser el vídeo", px)
	}

	w.Resize(fyne.NewSize(360, 760))
	_ = w.Canvas().Capture()
	vaciar("la página al ponerlo de pie", func() bool { return w.Canvas().Overlays().Top() == nil })

	a.pantallaCompleta()
	if !encima() {
		t.Fatal("el botón de pantalla completa no amplía el vídeo en el teléfono")
	}
	if w.FullScreen() {
		t.Error("en el teléfono no se pide la pantalla completa a la ventana")
	}
	if !a.atras() || w.Canvas().Overlays().Top() != nil || a.ruta != RutaVer {
		t.Error("atrás tiene que salir de la pantalla completa y quedarse en el vídeo")
	}
}

// TestElVideoSePausaAlIrseAlFondo: mientras un vídeo se reproduce la pantalla no se
// apaga; al irse la app al fondo se pausa y la pantalla vuelve a poder apagarse.
//
// VISTO EN ROJO sin el `r.Alternar()` de AlFondo: el vídeo sigue sonando en el bolsillo.
func TestElVideoSePausaAlIrseAlFondo(t *testing.T) {
	cola := make(chan func(), 4096)
	vaciar := func(que string, hasta func() bool) {
		t.Helper()
		limite := time.Now().Add(15 * time.Second)
		for !hasta() {
			if time.Now().After(limite) {
				t.Fatalf("no llega %s", que)
			}
			select {
			case f := <-cola:
				f()
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	a, _ := appDePrueba(t, servicioCompleto(), true)
	a.enLaInterfaz = func(f func()) { cola <- f }
	a.AbrirMedia = nil
	a.Movil = true
	var pedidos []bool
	a.PantallaEncendida = func(si bool) { pedidos = append(pedidos, si) }
	a.AbrirVideo("v01")
	vaciar("el reproductor", func() bool { return a.reproductor != nil })
	a.visor.alternar() // tocar el vídeo: reproducir
	vaciar("la pantalla encendida", func() bool { return len(pedidos) > 0 && pedidos[len(pedidos)-1] })
	if a.reproductor.Pausado() {
		t.Fatal("tocar el vídeo no lo reproduce")
	}
	a.AlFondo()
	if !a.reproductor.Pausado() {
		t.Error("al irse al fondo el vídeo sigue")
	}
	if pedidos[len(pedidos)-1] {
		t.Error("en pausa la pantalla sigue sin poder apagarse")
	}
}

// camarasDeTelefono son la delantera y la trasera de un Android (ids de Camera2).
type camarasDeTelefono struct {
	camarasFalsas
	mu      sync.Mutex
	pedidas []string
	permiso bool
}

func (c *camarasDeTelefono) lista() []camara.Dispositivo {
	return []camara.Dispositivo{{ID: "1", Nombre: "front"}, {ID: "0", Nombre: "back"}}
}

// TestEmitirConLaCamaraTrasera: en el teléfono hay botón para cambiar de cámara; pasa a
// la trasera, que se ve sin espejo, y vuelve a la delantera.
//
// VISTO EN ROJO con `cambioDeCamara` devolviendo siempre nil: no hay botón.
func TestEmitirConLaCamaraTrasera(t *testing.T) {
	a, w := appDePrueba(t, servicioCompleto(), true)
	vaciar := colaDeInterfaz(t, a)
	c := &camarasDeTelefono{}
	a.Movil = true
	a.Camaras = c.lista
	a.Camara = func() (camara.Fuente, error) {
		c.mu.Lock()
		c.pedidas = append(c.pedidas, a.CamaraElegida())
		c.mu.Unlock()
		return c.abrir()
	}
	a.Ir(RutaEmitir)
	vaciar("la página", func() bool { return a.emi.directos != nil })
	if hayIcono(w, a.v.Traduce("studio.app.flipCamera")) == nil {
		t.Fatal("en el teléfono no hay botón para cambiar de cámara")
	}
	a.fuenteCamara()()
	vaciar("la delantera", func() bool { return a.emi.ahora.camara })
	a.cambiarCamara()
	vaciar("la trasera", func() bool { return a.emi.ahora.camara && a.previaCam != nil })
	if a.CamaraElegida() != "0" || a.camaraDelantera() {
		t.Errorf("tras cambiar, la elegida es %q (delantera %v)", a.CamaraElegida(), a.camaraDelantera())
	}
	if c.abiertas() != 1 {
		t.Errorf("hay %d cámaras abiertas a la vez", c.abiertas())
	}
	a.cambiarCamara()
	vaciar("otra vez la delantera", func() bool { return a.CamaraElegida() == "1" && a.emi.ahora.camara })

	a.Movil = false
	a.pintar()
	if hayIcono(w, a.v.Traduce("studio.app.flipCamera")) != nil {
		t.Error("en escritorio no se cambia de cámara con este botón")
	}
}

// TestLaCamaraEsperaAlPermiso: en Android el primer intento abre el diálogo del sistema
// y vuelve «sin permiso» en el acto. Se avisa, y en cuanto se concede la cámara se
// enciende sola; y el micrófono se pide DESPUÉS, no a la vez.
//
// VISTO EN ROJO sin `esperarPermisoCamara`: se queda en «denegado» aunque se conceda.
func TestLaCamaraEsperaAlPermiso(t *testing.T) {
	a, _ := appDePrueba(t, servicioCompleto(), true)
	vaciar := colaDeInterfaz(t, a)
	c := &camarasDeTelefono{}
	a.Movil = true
	a.CamaraConcedida = func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.permiso
	}
	a.Camara = func() (camara.Fuente, error) {
		c.mu.Lock()
		si := c.permiso
		c.mu.Unlock()
		if !si {
			return nil, camara.ErrSinPermiso
		}
		return c.abrir()
	}
	micros := 0
	a.PedirMicro = func() error { micros++; return nil }
	a.Ir(RutaEmitir)
	vaciar("la página", func() bool { return a.emi.directos != nil })
	a.fuenteCamara()()
	vaciar("el aviso", func() bool { return a.emi.ahora.denegado })
	if micros != 0 {
		t.Error("el micrófono se pide a la vez que la cámara: los dos diálogos se pisan")
	}
	c.mu.Lock()
	c.permiso = true
	c.mu.Unlock()
	vaciar("la cámara al conceder", func() bool { return a.emi.ahora.camara && !a.emi.ahora.denegado })
	if micros != 1 {
		t.Errorf("con la cámara concedida el micrófono se pidió %d veces", micros)
	}
}

// TestLaCopiaDelTelefonoNoSeQueda: lo que el selector del teléfono copió y no se va a
// subir se suelta en el acto. VISTO EN ROJO sin `soltarCopia` en anadir.
func TestLaCopiaDelTelefonoNoSeQueda(t *testing.T) {
	a, _ := appDePrueba(t, servicioCompleto(), true)
	var sueltas []string
	a.SoltarCopia = func(r string) { sueltas = append(sueltas, r) }
	a.Ir(RutaSubir)
	a.anadir([]string{"/tmp/sytue-elegido-1/notas.txt"})
	if len(sueltas) != 1 || sueltas[0] != "/tmp/sytue-elegido-1/notas.txt" {
		t.Errorf("se soltaron %v", sueltas)
	}
}

// hayIcono es el botón de icono con ese nombre accesible, o nil.
func hayIcono(w fyne.Window, rotulo string) *ui.BotonIcono {
	var hallado *ui.BotonIcono
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if hallado != nil || o == nil || !o.Visible() {
			return
		}
		if b, ok := o.(*ui.BotonIcono); ok && b.Rotulo == rotulo {
			hallado = b
			return
		}
		switch x := o.(type) {
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				mirar(h)
			}
		}
	}
	mirar(w.Content())
	return hallado
}

// TestElBotonFlotanteNoTapaElCajon: en Inicio, en el teléfono, «Subir» flota abajo a la
// derecha; con el cajón del menú abierto se esconde, que el velo tapa la página y el
// botón quedaba encima de los dos. VISTO EN ROJO sin `arm.AlCajon`.
func TestElBotonFlotanteNoTapaElCajon(t *testing.T) {
	a, w := appDePrueba(t, servicioCompleto(), true)
	w.Resize(fyne.NewSize(360, 760))
	a.Arrancar()
	_ = w.Canvas().Capture()
	var fab *ui.BotonFlotante
	var barra *ui.BarraMovil
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.BotonFlotante:
			fab = x
		case *ui.BarraMovil:
			barra = x
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				mirar(h)
			}
		}
	}
	mirar(w.Content())
	if fab == nil || barra == nil || barra.AlMenu == nil {
		t.Fatalf("en Inicio a 360 falta el botón flotante (%v) o la barra con su menú (%v)", fab, barra)
	}
	visible := func() bool { return fab.Visible() && a.raiz.Objects[1].Visible() }
	if !visible() {
		t.Fatal("con el cajón cerrado el botón flotante no se ve")
	}
	barra.AlMenu()
	if visible() {
		t.Error("con el cajón abierto el botón flotante sigue encima")
	}
	barra.AlMenu()
	if !visible() {
		t.Error("al cerrar el cajón el botón flotante no vuelve")
	}
}

// pipFalsa es la imagen dentro de imagen del sistema, de mentira.
type pipFalsa struct {
	activa   bool
	entradas int
	suena    []bool
}

func (p *pipFalsa) Disponible() bool              { return true }
func (p *pipFalsa) Activa() bool                  { return p.activa }
func (p *pipFalsa) Preparar(_, _ int, suena bool) { p.suena = append(p.suena, suena) }
func (p *pipFalsa) Entrar(bool) bool              { p.entradas++; p.activa = true; return true }

// El reloj maestro es el audio: sin pedir muestras el clip no llega al final.
// Esta prueba ejercita controles PiP, no la reproducción cronometrada (video_test.go).
type tarjetaSinAvance struct{}

func (tarjetaSinAvance) Abrir(_, _ func([]int16)) (func(), error) {
	return func() {}, nil
}

// TestLaImagenEnImagenDelTelefono: en el teléfono el botón no abre otra ventana (Fyne
// tiene una) sino la del sistema, con el vídeo a pantalla completa; al irse la app a la
// esquina el vídeo NO se pausa, cerrar la esquina sí lo pausa, y el sistema sabe en todo
// momento si el vídeo suena (para su botón y para entrar solo al salir). Sin la del
// sistema, no hay botón.
//
// VISTO EN ROJO sin el `if a.enEsquina()` de AlFondo: el vídeo se pausa al encoger.
func TestLaImagenEnImagenDelTelefono(t *testing.T) {
	cola := make(chan func(), 4096)
	vaciar := func(que string, hasta func() bool) {
		t.Helper()
		limite := time.Now().Add(15 * time.Second)
		for !hasta() {
			if time.Now().After(limite) {
				t.Fatalf("no llega %s", que)
			}
			select {
			case f := <-cola:
				f()
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	a, w := appDePrueba(t, servicioCompleto(), true)
	a.enLaInterfaz = func(f func()) { cola <- f }
	a.AbrirMedia = nil
	a.Audio = tarjetaSinAvance{}
	a.Movil = true
	pip := &pipFalsa{}
	a.PiPMovil = pip
	a.AbrirVideo("v01")
	vaciar("el reproductor", func() bool { return a.reproductor != nil && a.visor.AlPiP != nil })
	a.visor.alternar()
	vaciar("que el sistema sepa que suena", func() bool { return len(pip.suena) > 0 && pip.suena[len(pip.suena)-1] })

	a.visor.AlPiP()
	if pip.entradas != 1 || a.movil.ampliado == nil {
		t.Fatalf("el botón no manda la app a la esquina con el vídeo a pantalla completa (%d, %v)", pip.entradas, a.movil.ampliado)
	}
	a.AlFondo()
	if a.reproductor.Pausado() {
		t.Error("al irse a la esquina el vídeo se pausa")
	}
	_ = w.Canvas().Capture()
	if a.reproductor.Terminado() || a.reproductor.Posicion() != 0 {
		t.Fatal("la prueba PiP necesita el reloj de audio detenido, no el clip terminado")
	}

	a.SucesoPiP(false) // el botón de la ventana
	if !a.reproductor.Pausado() {
		t.Error("el botón de la ventana no pausa")
	}
	a.SucesoPiP(false)
	pip.activa = false
	a.SucesoPiP(true) // la cierran
	if !a.reproductor.Pausado() || a.movil.ampliado != nil {
		t.Error("cerrar la esquina deja el vídeo sonando o a pantalla completa")
	}

	a.PiPMovil = nil
	a.AbrirVideo("v01")
	vaciar("el reproductor otra vez", func() bool { return a.reproductor != nil })
	if a.visor.AlPiP != nil {
		t.Error("sin la del sistema, el teléfono ofrece imagen dentro de imagen")
	}
}
