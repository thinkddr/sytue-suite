// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"fmt"
	"image"
	"image/color"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// contiene dice si alguna de las peticiones empieza por `prefijo`.
func contiene(vistas []string, prefijo string) bool {
	return slices.ContainsFunc(vistas, func(v string) bool { return strings.HasPrefix(v, prefijo) })
}

// TestArrancaDondeToca: con sesión, Inicio pide lo mío, lo de la organización, los
// directos y lo compartido; sin sesión, Explorar pide la portada pública.
func TestArrancaDondeToca(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.Arrancar()
	if a.Ruta() != RutaInicio {
		t.Fatalf("con sesión arranca en %v", a.Ruta())
	}
	v := s.vistas()
	for _, quiero := range []string{
		"GET /api/v1/videos?limit=24&scope=mine", "GET /api/v1/videos?limit=12&scope=org",
		"GET /api/v1/videos?limit=8&scope=org&status=live", "GET /api/v1/videos/shared",
	} {
		if !contiene(v, quiero) {
			t.Errorf("Inicio no pide %q; pidió %q", quiero, v)
		}
	}
	if n := len(a.ini.d.Organizacion); n != len(s.org) {
		t.Errorf("«Lo último de la organización» trae %d y debería traer %d (sin repetir el directo)", n, len(s.org))
	}

	s2 := servicioCompleto()
	b, _ := appDePrueba(t, s2, false)
	b.Arrancar()
	if b.Ruta() != RutaExplorar || !contiene(s2.vistas(), "GET /api/v1/public/explore?limit=24") {
		t.Errorf("sin sesión arranca en %v y pide %q", b.Ruta(), s2.vistas())
	}
	if len(b.exp.videos) == 0 || len(b.exp.directos) != 1 || !b.exp.mas {
		t.Errorf("la portada no se cargó: %+v", b.exp)
	}
	// «Cargar más» sigue por el cursor.
	b.pedirExplorar(true)
	if !contiene(s2.vistas(), "GET /api/v1/public/explore?cursor=e2&limit=24") {
		t.Errorf("«Cargar más» no manda el cursor: %q", s2.vistas())
	}
}

// TestLosFallosDanSuEstado: un 403 es «sin permiso», un 500 es «error» con reintentar,
// y sin servidor es «sin conexión».
func TestLosFallosDanSuEstado(t *testing.T) {
	for _, c := range []struct {
		status int
		quiero Estado
	}{{403, SinPermiso}, {404, SinPermiso}, {500, Error}, {401, SinSesion}} {
		s := servicioCompleto()
		s.falla = c.status
		a, _ := appDePrueba(t, s, true)
		a.Arrancar()
		v := a.vistaDe(a.ini.estadoCarga, !a.ini.pedidos, false, nil)
		if v.Estado != c.quiero {
			t.Errorf("un %d deja la pantalla en %v y debería en %v", c.status, v.Estado, c.quiero)
		}
	}
	a := Nueva(api.NewCast("http://127.0.0.1:1", nil, nil), vistaDePrueba(t, 0), nil)
	a.lanzar = func(f func()) { f() }
	a.ConSesion = func() bool { return true }
	a.Arrancar()
	if v := a.vistaDe(a.ini.estadoCarga, true, false, nil); v.Estado != SinConexion {
		t.Errorf("sin servidor la pantalla está en %v", v.Estado)
	}
}

// TestSubirUnVideo: elegir ficheros los pone en la cola con su nombre de título; lo
// que no es vídeo entra ya rechazado; el vídeo se da de alta con la visibilidad de
// lo nuevo, el original sube entero con su tipo y la fila queda «procesando». El
// título cambiado mientras subía se guarda al acabar.
func TestSubirUnVideo(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.Elegir = func() ([]string, error) { return []string{fixtureWebM, "notas.txt"}, nil }
	a.Ir(RutaSubir)
	a.sub.visibilidad = api.CastUnlisted
	// La subida va en línea (lanzar): el título se cambia antes, al elegir.
	a.lanzar = func(f func()) { f() }
	a.Elegir = func() ([]string, error) { return []string{fixtureWebM, "notas.txt"}, nil }
	a.elegirFicheros()
	// Lo que no es vídeo entra ya rechazado; el vídeo espera en «Antes de subir» con
	// su estimación y la recomendada marcada, y no sube nada hasta confirmar.
	if len(a.sub.cola) != 1 || len(a.sub.preparados) != 1 || a.sub.preparados[0].Estimacion == nil {
		t.Fatalf("la cola tiene %d y hay %d preparados", len(a.sub.cola), len(a.sub.preparados))
	}
	if !contiene(s.vistas(), "GET /api/v1/videos:estimate?size_bytes=17428") || contiene(s.vistas(), "POST /api/v1/videos?") {
		t.Fatalf("antes de confirmar solo se estima: %q", s.vistas())
	}
	if !slices.Equal(a.sub.calidades, []int{1080, 720, 360}) {
		t.Fatalf("la recomendada no viene marcada: %v", a.sub.calidades)
	}
	a.empezarSubida()
	if len(a.sub.cola) != 2 || len(a.sub.preparados) != 0 {
		t.Fatalf("la cola tiene %d ficheros", len(a.sub.cola))
	}
	a.sub.cola[0], a.sub.cola[1] = a.sub.cola[1], a.sub.cola[0]
	v, texto := a.sub.cola[0], a.sub.cola[1]
	if texto.Fase != Fallida || texto.Motivo == "" {
		t.Errorf("un .txt entra como %+v", texto)
	}
	if v.Fase != Procesando || v.Tamano != 17428 || v.Enviado != 17428 {
		t.Errorf("el vídeo queda %+v", v)
	}
	vistas := s.vistas()
	if !contiene(vistas, `POST /api/v1/videos? {"title":"dos-segundos","visibility":"unlisted"}`) {
		t.Errorf("el alta no es la esperada: %q", vistas)
	}
	if !contiene(vistas, "PUT /api/v1/videos/v99/original? <17428 bytes video/webm>") {
		t.Errorf("el original no se subió entero con su tipo: %q", vistas)
	}
	if a.sub.cuota == nil || a.sub.cuota.AvailableBytes != 143<<30 {
		t.Errorf("la cuota no se volvió a pedir: %+v", a.sub.cuota)
	}
	// El título se edita después y se guarda con Intro.
	a.sub.cola[0].Titulo.PonerTexto("Prueba de dos segundos")
	a.guardarTitulo(a.sub.cola[0].Clave)
	if !contiene(s.vistas(), `PATCH /api/v1/videos/v99? {"title":"Prueba de dos segundos"}`) {
		t.Errorf("el título no se guardó: %q", s.vistas())
	}
	a.quitarEnvio(texto.Clave)
	if len(a.sub.cola) != 1 {
		t.Errorf("quitar deja %d", len(a.sub.cola))
	}
}

// TestUnaSubidaSinSitioDiceCuantoQueda: el 507 del servicio se cuenta con lo que
// queda, y el registro que se creó se borra.
func TestUnaSubidaSinSitioDiceCuantoQueda(t *testing.T) {
	s := servicioCompleto()
	s.sinSitio = true
	a, _ := appDePrueba(t, s, true)
	a.Ir(RutaSubir)
	a.anadir([]string{fixtureWebM})
	a.empezarSubida()
	e := a.sub.cola[0]
	if e.Fase != Fallida || !strings.Contains(e.Motivo, "KB") {
		t.Errorf("sin sitio la fila queda %+v", e)
	}
	if !contiene(s.vistas(), "DELETE /api/v1/videos/v99") {
		t.Errorf("el registro sin original no se borró: %q", s.vistas())
	}
}

// avisado dice si hay un aviso (toast) con ese título.
func avisado(a *App, titulo string) bool {
	return slices.ContainsFunc(a.avisos.Avisos(), func(x *ui.Aviso) bool { return x.Titulo == titulo })
}

// TestGestionarGuardaComoLaWeb: título y descripción van con Guardar; la visibilidad y
// los interruptores se guardan al momento, cada uno solo lo suyo, y si el servicio no
// los acepta vuelven a lo de antes con «No se ha podido guardar».
func TestGestionarGuardaComoLaWeb(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.AbrirGestionar("v01")
	if a.gTitulo.Texto() != s.mios[0].Title || a.ges.video.ID != "v01" {
		t.Fatalf("no se cargó el vídeo: %q", a.gTitulo.Texto())
	}
	if a.detallesCambiados() {
		t.Fatal("recién cargado ya hay cambios")
	}
	a.gTitulo.PonerTexto("Resultados T3")
	a.guardar()
	if !contiene(s.vistas(), `PATCH /api/v1/videos/v01? {"title":"Resultados T3","description":`) {
		t.Errorf("el PATCH de los detalles no es el esperado: %q", s.vistas())
	}
	if !avisado(a, a.v.Traduce("manage.saved")) || a.detallesCambiados() {
		t.Errorf("tras guardar: avisos %v, cambiado %v", a.avisos.Avisos(), a.detallesCambiados())
	}
	a.cambiar(api.CastChanges{CommentsEnabled: new(false)})
	if !contiene(s.vistas(), `PATCH /api/v1/videos/v01? {"comments_enabled":false}`) || a.ges.video.CommentsEnabled {
		t.Errorf("el interruptor no se guardó solo: %q", s.vistas())
	}
	// Personas concretas: se guarda y se piden las personas; luego se comparte.
	a.cambiar(api.CastChanges{Visibility: new(api.CastPeople)})
	if !contiene(s.vistas(), `PATCH /api/v1/videos/v01? {"visibility":"people"}`) || !contiene(s.vistas(), "GET /api/v1/videos/v01/people") {
		t.Errorf("la visibilidad no se guardó al momento: %q", s.vistas())
	}
	a.gPersona.PonerTexto("@bea")
	a.anadirPersona()
	if !contiene(s.vistas(), `POST /api/v1/videos/v01/people? {"query":"@bea"}`) || len(a.ges.personas) == 0 {
		t.Errorf("no se compartió con @bea: %q, %+v", s.vistas(), a.ges.personas)
	}
	a.quitarPersona(api.CastPerson{UserID: "u9"})
	if !contiene(s.vistas(), "DELETE /api/v1/videos/v01/people/u9") {
		t.Errorf("no se quitó a la persona: %q", s.vistas())
	}
	// Si el servicio no lo acepta, vuelve a lo de antes.
	s.falla = 500
	a.cambiar(api.CastChanges{DownloadEnabled: new(!a.ges.video.DownloadEnabled)})
	s.falla = 0
	if a.ges.video.DownloadEnabled != s.mios[0].DownloadEnabled || !avisado(a, a.v.Traduce("manage.saveError")) {
		t.Errorf("un cambio rechazado no se deshizo: %+v", a.ges.video)
	}
}

// TestGestionarUsaElFotograma: con el vídeo listo, la tarjeta del póster lleva el
// reproductor, y «Usar este fotograma» pide el instante en el que está.
func TestGestionarUsaElFotograma(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	vaciar := colaDeInterfaz(t, a)
	a.AbrirMedia = nil // el cliente de verdad: HEAD y Range contra el servicio falso
	a.AbrirGestionar("v01")
	vaciar("el reproductor", func() bool { return a.reproductor != nil && a.visor != nil })
	a.reproductor.Buscar(time.Second)
	vaciar("el segundo 1", func() bool { return a.reproductor.Posicion() >= time.Second })
	a.posterDeFotograma()
	vaciar("el aviso", func() bool { return !a.ges.posterOcupado })
	if !contiene(s.vistas(), `POST /api/v1/videos/v01/poster:frame? {"at_ms":1000}`) {
		t.Errorf("no se pidió el fotograma del segundo 1: %q", s.vistas())
	}
	if !avisado(a, a.v.Traduce("manage.posterQueued")) {
		t.Error("no se avisó de que el póster se está sacando")
	}
	a.Ir(RutaInicio) // suelta el reproductor
}

// TestLaClaveDeOBS: oculta al llegar, se enseña y se oculta, se copia al portapapeles
// y regenerarla pide la nueva.
func TestLaClaveDeOBS(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.Ir(RutaEmitir)
	if a.emi.clave == nil || a.emi.visible {
		t.Fatalf("la clave no llegó oculta: %+v", a.emi)
	}
	if len(a.emi.directos) != 0 {
		t.Errorf("los directos de «mine» deberían ser los live de la lista: %+v", a.emi.directos)
	}
	a.pantallaEmitir() // monta los mandos
	a.emi.visible = true
	a.pintar()
	a.copiarEstudio(a.emi.clave.StreamKey, "studio.obs.copied")
	if got := fyne.CurrentApp().Clipboard().Content(); got != "sk_live_7Qf2mX9kR4tB8wNz0" {
		t.Errorf("el portapapeles tiene %q", got)
	}
	a.regenerar()
	if a.emi.clave.StreamKey != "sk_live_7Qf2mX9kR4tB8wNz1" || !contiene(s.vistas(), "POST /api/v1/studio/stream-key:rotate") {
		t.Errorf("regenerar no trajo la clave nueva: %+v", a.emi.clave)
	}
	// Programar para OBS da de alta un directo rtmp y lo pone el primero de la lista.
	a.tituloDirecto.PonerTexto("Formación de octubre")
	a.descDirecto.PonerTexto("Resultados del trimestre")
	a.programar()
	if !contiene(s.vistas(), `POST /api/v1/lives? {"title":"Formación de octubre","description":"Resultados del trimestre","visibility":"org","source":"rtmp"}`) {
		t.Errorf("el directo no se programó: %q", s.vistas())
	}
	if len(a.emi.directos) == 0 || a.emi.directos[0].Title != "Formación de octubre" || !avisado(a, a.v.Traduce("studio.schedule.done")) {
		t.Errorf("el directo programado no sale: %+v", a.emi.directos)
	}
}

// colaDeInterfaz hace que lo que la app manda al hilo de la interfaz espere en una
// cola que el test vacía a mano: con rutinas de verdad (la vista previa, la sala)
// no hay carreras con el test.
func colaDeInterfaz(t *testing.T, a *App) func(que string, hasta func() bool) {
	cola := make(chan func(), 4096)
	a.enLaInterfaz = func(f func()) { cola <- f }
	return func(que string, hasta func() bool) {
		t.Helper()
		limite := time.Now().Add(10 * time.Second)
		for !hasta() {
			if time.Now().After(limite) {
				t.Fatalf("no llega %s", que)
			}
			select {
			case f := <-cola:
				f()
			case <-time.After(5 * time.Millisecond):
			}
		}
	}
}

// TestEmitirDesdeAquiEntraEnLaSala: la cámara se abre para la vista previa (en
// espejo); «Emitir ahora» no vale sin título; con él programa un directo webrtc, suelta
// la vista previa, entra con el token de emisor y enciende la cámara. Terminar cuelga,
// avisa al servicio y vuelve a la vista previa.
func TestEmitirDesdeAquiEntraEnLaSala(t *testing.T) {
	s := servicioCompleto()
	a, w := appDePrueba(t, s, true)
	if os.Getenv("SYTUE_CAPTURA") != "" {
		recorrido.AMedida(w)
	}
	vaciar := colaDeInterfaz(t, a)
	sala := &salaFalsa{}
	var servidor, token string
	var micro bool
	a.Conectar = func(u, tk string, o llamadaOpciones) (Sala, error) {
		servidor, token, micro = u, tk, o.Micro
		return sala, nil
	}
	previas := &camarasFalsas{}
	a.Camara = previas.abrir
	a.Ir(RutaEmitir)
	vaciar("la página", func() bool { return a.emi.directos != nil })
	a.empezarEmision() // sin fuente ni título: nada
	if a.emi.ahora.fase != EmisionPreparando {
		t.Fatal("se empezó sin fuente ni título")
	}
	a.fuenteCamara()()
	vaciar("la cámara de la vista previa", func() bool { return a.emi.ahora.camara })
	vaciar("el primer fotograma", a.previa.TieneImagen)
	if px := a.previa.imagen.Image.At(0, 0); !esBlanco(px) {
		t.Errorf("la cámara no se ve en espejo: el primer píxel es %v", px)
	}
	if (Emision{Camara: true, Titulo: a.tituloAhora}).Listo() {
		t.Error("sin título no debería estar listo")
	}
	a.tituloAhora.PonerTexto("Desde la app")
	a.empezarEmision()
	vaciar("la sala", func() bool { return a.emi.ahora.fase == EmisionEnDirecto })
	if !contiene(s.vistas(), `POST /api/v1/lives? {"title":"Desde la app","visibility":"org","source":"webrtc"}`) {
		t.Errorf("no se programó el directo: %q", s.vistas())
	}
	if servidor != "wss://sfu.test" || token != "pub" || !micro || !sala.en().camara {
		t.Fatalf("no se entró a emitir: servidor %q token %q micro %v cámara %v", servidor, token, micro, sala.en().camara)
	}
	if a.previaCam != nil || previas.abiertas() != 0 {
		t.Error("la cámara de la vista previa sigue abierta con la sala emitiendo")
	}
	guardar(t, "emitir-en-directo-claro", w.Canvas().Capture())
	a.alternarMicro()
	if sala.en().micro || a.emi.ahora.micro {
		t.Error("el micro no se cerró")
	}
	a.acabar()
	vaciar("colgar", func() bool { return sala.en().colgada })
	if !contiene(s.vistas(), "POST /api/v1/lives/v50/end") || a.emi.ahora.fase != EmisionPreparando {
		t.Errorf("terminar no avisó al servicio: %q", s.vistas())
	}
	vaciar("la vista previa de vuelta", func() bool { return a.emi.ahora.camara && a.previaCam != nil })
}

// TestEmitirUnDirectoProgramado: «Emitir desde aquí» en un directo webrtc de la lista
// lo elige, y «Emitir ahora» pide SU token de emisor en vez de programar otro.
func TestEmitirUnDirectoProgramado(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	vaciar := colaDeInterfaz(t, a)
	a.Conectar = func(u, tk string, _ llamadaOpciones) (Sala, error) { return &salaFalsa{}, nil }
	a.Camara = (&camarasFalsas{}).abrir
	a.Ir(RutaEmitir)
	vaciar("la página", func() bool { return a.emi.directos != nil })
	programado := directoDePrueba()
	programado.ID, programado.Status, programado.Live.Source = "v11", api.CastStatusScheduled, "webrtc"
	a.elegirParaEmitir(programado)
	a.fuenteCamara()()
	vaciar("la cámara", func() bool { return a.emi.ahora.camara })
	a.empezarEmision()
	vaciar("la sala", func() bool { return a.emi.ahora.fase == EmisionEnDirecto })
	if !contiene(s.vistas(), "GET /api/v1/lives/v11/publish") || contiene(s.vistas(), "POST /api/v1/lives?") {
		t.Errorf("no se pidió el token del directo elegido: %q", s.vistas())
	}
	a.Cerrar()
}

func esBlanco(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xf000 && g > 0xf000 && b > 0xf000
}

// TestComentar: el comentario va con el texto y, al responder, con el comentario raíz;
// después se vacía el campo.
func TestComentar(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.AbrirVideo("v01")
	if len(a.ver.comentarios) != 3 {
		t.Fatalf("hay %d comentarios", len(a.ver.comentarios))
	}
	a.ver.respondiendo = &a.ver.comentarios[1] // una respuesta: se contesta a su raíz
	a.comentario.PonerTexto("Gracias, lo veo")
	a.comentar()
	if !contiene(s.vistas(), `POST /api/v1/videos/v01/comments? {"body":"Gracias, lo veo","reply_to":"k1"}`) {
		t.Errorf("el comentario no se mandó como respuesta a la raíz: %q", s.vistas())
	}
	if a.comentario.Texto() != "" || a.ver.respondiendo != nil || len(a.ver.comentarios) != 4 {
		t.Errorf("tras comentar: campo %q, respondiendo %v, %d comentarios", a.comentario.Texto(), a.ver.respondiendo, len(a.ver.comentarios))
	}
	a.borrarComentario(a.ver.comentarios[3])
	if !a.ver.comentarios[3].Deleted {
		t.Error("el comentario borrado sigue vivo")
	}
}

// TestCompartirCopiaElEnlaceYLaInsercion: el enlace es el de la web y la inserción el
// `<iframe>` de /embed, con el título escapado.
func TestCompartirCopiaElEnlaceYLaInsercion(t *testing.T) {
	s := servicioCompleto()
	a, w := appDePrueba(t, s, true)
	a.AbrirVideo("v01")
	a.abrirCompartir()
	if a.ver.compartir == nil || w.Canvas().Overlays().Top() == nil {
		t.Fatal("Compartir no abre el diálogo")
	}
	a.copiarEnlace()
	if a.ver.copiado == "" {
		t.Error("el diálogo no dice «Copiado»")
	}
	guardar(t, "compartir-claro", w.Canvas().Capture())
	if got := fyne.CurrentApp().Clipboard().Content(); got != "https://cast.sytue.eu/cast/watch/v01" {
		t.Errorf("el enlace es %q", got)
	}
	a.copiarInsercion()
	got := fyne.CurrentApp().Clipboard().Content()
	if !strings.HasPrefix(got, `<iframe src="https://cast.sytue.eu/cast/embed/v01" width="640" height="360"`) {
		t.Errorf("la inserción es %q", got)
	}
	if c := CodigoInsercion("https://x/cast/watch/1", "1", `A "B" <c>`); !strings.Contains(c, `title="A &quot;B&quot; &lt;c&gt;"`) {
		t.Errorf("el título no se escapa: %s", c)
	}
}

// TestLosEnlacesAbrenSuPantalla: las direcciones de la web y el esquema, con y sin la
// organización en el path.
func TestLosEnlacesAbrenSuPantalla(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	for _, c := range []struct {
		enlace string
		ruta   Ruta
	}{
		{"https://cast.sytue.eu/cast/watch/v02", RutaVer},
		{"sytue-cast:/cast/acme/manage/v01", RutaGestionar},
		{"https://cast.sytue.eu/cast/c/acme", RutaCanal},
		{"sytue-cast:/cast/studio", RutaEmitir},
		{"sytue-cast:/cast/upload", RutaSubir},
		{"https://cast.sytue.eu/cast/explore", RutaExplorar},
	} {
		u, _ := url.Parse(c.enlace)
		if !EsEnlaceDeCast(u) {
			t.Errorf("%s no se reconoce como de Cast", c.enlace)
		}
		if !a.AbrirEnlace(u) || a.Ruta() != c.ruta {
			t.Errorf("%s lleva a %v y debería a %v", c.enlace, a.Ruta(), c.ruta)
		}
	}
	u, _ := url.Parse("https://drive.sytue.eu/drive/x")
	if EsEnlaceDeCast(u) {
		t.Error("un enlace de Drive se toma por de Cast")
	}
}

// TestFormatos: duraciones y el filtro de Explorar.
func TestFormatos(t *testing.T) {
	for d, quiero := range map[time.Duration]string{
		4*time.Second + 400*time.Millisecond: "0:04", 125 * time.Second: "2:05", 3723 * time.Second: "1:02:03",
	} {
		if got := Duracion(d); got != quiero {
			t.Errorf("Duracion(%v) = %q", d, got)
		}
	}
	vs := deLaOrganizacion()
	if got := Filtrar(vs, "  SEGURIDAD "); len(got) != 1 || got[0].ID != "v22" {
		t.Errorf("el filtro por título da %+v", got)
	}
	if got := Filtrar(vs, "pau"); len(got) != 1 || got[0].ID != "v23" {
		t.Errorf("el filtro por autor da %+v", got)
	}
}

// TestRecorridoConSesion pulsa todo lo que se ve en cada pantalla con los datos del
// servicio falso y cierra con Escape cada modal (ver `ui/recorrido`).
//
// Con `SYTUE_CAPTURAS_PLAY` saca las capturas de la ficha de Google Play (ver
// apps/fyne/play/README.md): la ventana entera, con su barra, como en el teléfono; los
// pósters repintados al llegar, como en cmd/cast; y el reproductor de verdad contra el
// servicio falso, para que la página de un vídeo no enseñe «No se ha podido reproducir
// el vídeo».
func TestRecorridoConSesion(t *testing.T) {
	play := os.Getenv("SYTUE_CAPTURAS_PLAY") != ""
	for _, r := range []Ruta{RutaInicio, RutaExplorar, RutaSubir, RutaEmitir, RutaGestionar, RutaVer} {
		s := servicioCompleto()
		a, w := appDePrueba(t, s, true)
		a.Elegir = func() ([]string, error) { return nil, nil }
		if play {
			a.AbrirMedia = nil
			// Los pósters llegan en segundo plano; como en cmd/cast, al llegar se repinta.
			a.v.Teca.AlLlegar = func(string) { fyne.Do(a.Repintar) }
		}
		a.Arrancar()
		switch r {
		case RutaGestionar:
			a.AbrirGestionar("v01")
		case RutaVer:
			a.AbrirVideo("v01")
		default:
			a.Ir(r)
		}
		montaje.ComprobarRotacion(t, w, fmt.Sprintf("cast/ruta-%d", r), iconos.Cast, r == RutaVer,
			func() { prepararPosters(w, a) })
		// Sin el carril: si no, lo primero que se pulsa es la navegación y todas las
		// rutas acaban recorriendo lo mismo. La página entera a la vista. Para Play no:
		// sin el armazón la app no se entera de que la ventana es de teléfono
		// (`medirVentana`) y pinta la disposición de escritorio en 360 de ancho.
		if !play {
			w.SetContent(a.scroll)
			w.Resize(fyne.NewSize(ventanaAncho, 2400))
		} else {
			recorrido.AMedida(w)
			prepararPosters(w, a)
		}
		recorrido.Comprobar(t, w, 120)
		a.Cerrar()
	}
}

// prepararPosters deja los pósters preparados para el tamaño actual, sin cambiarlo.
// Lo usan tanto las rotaciones como Play, que aplica AMedida antes de llamarlo.
func prepararPosters(w fyne.Window, a *App) {
	recorrido.PrepararCaptura(w, a.v.Teca.Esperar, a.Repintar)
}

// TestRecorridoSinSesion: sin sesión todo se deja pulsar y lo que la necesita pide
// entrar.
func TestRecorridoSinSesion(t *testing.T) {
	s := servicioCompleto()
	a, w := appDePrueba(t, s, false)
	entrar := 0
	a.AlEntrar = func() { entrar++ }
	a.Arrancar()
	montaje.ComprobarRotacion(t, w, "cast/sin-sesion", iconos.Cast, false)
	recorrido.Comprobar(t, w, 120)
	a.Ir(RutaSubir)
	test.Tap(buscarBoton(t, w, "Iniciar sesión"))
	if entrar == 0 {
		t.Error("«Iniciar sesión» no llama a Sytue ID")
	}
}

// ---- Dobles ----

type llamadaOpciones = llamada.Opciones

// salaFalsa es la sala de LiveKit de los tests: apunta lo que se enciende. Se cuelga
// desde otra rutina, así que va con candado.
type salaFalsa struct {
	mu sync.Mutex
	estadoSala
}

type estadoSala struct{ camara, pantalla, micro, colgada bool }

func (s *salaFalsa) cambiar(f func(e *estadoSala)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.estadoSala)
	return nil
}

func (s *salaFalsa) en() estadoSala {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.estadoSala
}

func (s *salaFalsa) Participantes() []llamada.Participante { return nil }
func (s *salaFalsa) Camara(si bool) error                  { return s.cambiar(func(e *estadoSala) { e.camara = si }) }

func (s *salaFalsa) Pantalla(si bool) error {
	return s.cambiar(func(e *estadoSala) { e.pantalla = si })
}

func (s *salaFalsa) Micro(si bool) error { return s.cambiar(func(e *estadoSala) { e.micro = si }) }

func (s *salaFalsa) Colgar() { _ = s.cambiar(func(e *estadoSala) { e.colgada = true }) }

// camarasFalsas abre cámaras de mentira que dan UN fotograma: blanco a la derecha y
// negro a la izquierda, para ver el espejo. Cuenta las que siguen abiertas.
type camarasFalsas struct {
	mu        sync.Mutex
	abiertasN int
}

func (c *camarasFalsas) abrir() (camara.Fuente, error) {
	c.mu.Lock()
	c.abiertasN++
	c.mu.Unlock()
	return &camaraFalsaAbierta{de: c}, nil
}

func (c *camarasFalsas) abiertas() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.abiertasN
}

type camaraFalsaAbierta struct {
	de            *camarasFalsas
	mu            sync.Mutex
	dado, cerrada bool
}

func (f *camaraFalsaAbierta) Leer() (*image.YCbCr, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dado || f.cerrada {
		return nil, false
	}
	f.dado = true
	img := image.NewYCbCr(image.Rect(0, 0, 4, 2), image.YCbCrSubsampleRatio444)
	for i := range img.Cb {
		img.Cb[i], img.Cr[i] = 128, 128
	}
	for y := range 2 {
		img.Y[y*img.YStride+2], img.Y[y*img.YStride+3] = 255, 255
	}
	return img, true
}

func (f *camaraFalsaAbierta) Cerrar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cerrada {
		f.cerrada = true
		f.de.mu.Lock()
		f.de.abiertasN--
		f.de.mu.Unlock()
	}
}

// buscarBoton encuentra en la ventana el botón con ese rótulo.
func buscarBoton(t *testing.T, w fyne.Window, texto string) *ui.Boton {
	t.Helper()
	var hallado *ui.Boton
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if hallado != nil || o == nil || !o.Visible() {
			return
		}
		if b, ok := o.(*ui.Boton); ok && b.Texto == texto {
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
	if hallado == nil {
		t.Fatalf("no hay ningún botón «%s»", texto)
	}
	return hallado
}

// TestVerReproduceElWebM: la página de un vídeo abre el WebM por rangos contra el
// servicio, pinta el primer fotograma en pausa, y al reproducir llega al final con el
// reloj de pared (sin tarjeta de sonido). Lo que llega de las rutinas del reproductor se
// ejecuta aquí, en el hilo del test, vaciando la cola de la interfaz.
func TestVerReproduceElWebM(t *testing.T) {
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
	s := servicioCompleto()
	a, w := appDePrueba(t, s, true)
	if os.Getenv("SYTUE_CAPTURA") != "" {
		recorrido.AMedida(w)
	}
	a.enLaInterfaz = func(f func()) { cola <- f }
	a.AbrirMedia = nil // el cliente de verdad: HEAD y Range contra el servicio falso
	a.AbrirVideo("v01")
	fotograma := func() bool {
		img := a.visor.imagen.Image
		return a.reproductor != nil && img != nil && img.Bounds().Dx() == 320
	}
	vaciar("el primer fotograma", fotograma)
	if os.Getenv("SYTUE_CAPTURA") != "" {
		guardar(t, "cast-vod-fixture", w.Canvas().Capture())
	}
	if !a.reproductor.Pausado() || a.visor.Mando == nil {
		t.Fatal("el vídeo tiene que abrir en pausa y con sus mandos")
	}
	if d := a.reproductor.Duracion(); d < 2*time.Second-20*time.Millisecond || d > 2*time.Second+20*time.Millisecond {
		t.Errorf("la duración es %v", d)
	}
	a.tecla(&fyne.KeyEvent{Name: fyne.KeySpace}) // espacio: reproducir
	vaciar("el final", a.reproductor.Terminado)
	if a.reproductor.Err() != nil {
		t.Errorf("la reproducción acabó con %v", a.reproductor.Err())
	}
	a.tecla(&fyne.KeyEvent{Name: fyne.KeyLeft}) // ← 5 s: vuelve al principio
	if p := a.reproductor.Posicion(); p != 0 {
		t.Errorf("← desde el final deja la posición en %v", p)
	}
	a.tecla(&fyne.KeyEvent{Name: fyne.KeyM})
	if !a.reproductor.Silenciado() {
		t.Error("M no silencia")
	}
	a.Ir(RutaInicio) // suelta el reproductor
	if a.reproductor != nil || a.fuente != nil {
		t.Error("salir de la página no suelta el reproductor")
	}
}

// TestSubirDesdeDrive: el enlace de Drive abre la pantalla con el fichero y su nombre de
// título; «Subir a Cast» pide la copia con lo elegido y lleva a la gestión del vídeo.
// Un enlace sin identificador no pide nada, y lo que el servicio rechaza se sabe al
// llegar.
func TestSubirDesdeDrive(t *testing.T) {
	const id = "0B6F7A2E-3C1D-4E5F-8A9B-0C1D2E3F4A5B"
	s := servicioCompleto()
	a, w := appDePrueba(t, s, true)
	u, _ := url.Parse("sytue-cast:/upload?drive=" + id)
	if !a.AbrirEnlace(u) || a.Ruta() != RutaDesdeDrive {
		t.Fatalf("el enlace de Drive abre %v", a.Ruta())
	}
	if a.dri.fichero == nil || a.tituloDrive.Texto() != "Demo de producto" {
		t.Fatalf("no se leyó el fichero: %+v, título %q", a.dri, a.tituloDrive.Texto())
	}
	guardar(t, "subir-drive-claro", w.Canvas().Capture())
	a.cambiarVisDrive(api.CastUnlisted)
	a.subirDesdeDrive()
	if !contiene(s.vistas(), `POST /api/v1/videos:from-drive? {"file_id":"0b6f7a2e-3c1d-4e5f-8a9b-0c1d2e3f4a5b","title":"Demo de producto","visibility":"unlisted"}`) {
		t.Errorf("la copia no se pidió como toca: %q", s.vistas())
	}
	if a.Ruta() != RutaGestionar || a.ges.id != "v01" {
		t.Errorf("tras subir se queda en %v", a.Ruta())
	}

	antes := len(s.vistas())
	a.AbrirDesdeDrive("no-es-un-id")
	if a.dri.id != "" || len(s.vistas()) != antes {
		t.Error("un enlace sin identificador pidió algo al servicio")
	}
	guardar(t, "subir-drive-vacio-claro", w.Canvas().Capture())
	a.AbrirDesdeDrive(idDriveNoVideo)
	if a.dri.fichero != nil || motivoDrive(a.dri.fallo) != DriveNoVideo {
		t.Errorf("un fichero que no es vídeo da %v", a.dri.fallo)
	}
	guardar(t, "subir-drive-error-claro", w.Canvas().Capture())
}
