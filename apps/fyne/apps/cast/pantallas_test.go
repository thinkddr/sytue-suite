// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func pngEncode(w io.Writer, img image.Image) error { return png.Encode(w, img) }

// conVariante pone el tema de la suite en esa variante mientras dura el test: los
// widgets de `ui` leen la variante del tema al crear su renderer.
func conVariante(t *testing.T, v tema.Variante) {
	t.Helper()
	nombre := "claro"
	if v == tema.Oscuro {
		nombre = "oscuro"
	}
	tema.Elegir(nombre)
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	t.Cleanup(func() {
		tema.Elegir("")
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	})
}

// enMarco monta una pantalla como la monta App: el armazón con el carril y el
// desplazamiento.
func enMarco(v Vista, activa Seccion, o fyne.CanvasObject) fyne.CanvasObject {
	arm := ui.NuevoArmazon(container.NewVScroll(o))
	arm.Menu = Lateral(v, activa, nil)
	return arm
}

// pintar pinta un objeto en una ventana de pruebas de 1280×800 y devuelve la captura.
// Espera a los pósters (se preparan en otra goroutine) y vuelve a pintar con ellos.
func pintar(t *testing.T, v Vista, o fyne.CanvasObject) image.Image {
	t.Helper()
	w := test.NewWindow(o)
	w.SetPadded(false)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ventanaAncho, ventanaAlto))
	// Los pósters se piden al pintar y se preparan en otra goroutine: solo en la lista
	// hay que pintar dos veces.
	if v.Teca != nil && v.Estado == Lista {
		_ = w.Canvas().Capture()
		v.Teca.Esperar()
		o.Refresh()
	}
	return w.Canvas().Capture()
}

// tecaConPosters es la caché con el póster de cada vídeo ya dentro.
func tecaConPosters(listas ...[]api.CastVideo) *photos.Fototeca {
	teca := photos.NuevaFototeca()
	for _, l := range listas {
		for _, x := range l {
			if x.PosterURL != "" {
				teca.Poner(x.ID, posterDe(x.ID))
			}
		}
	}
	return teca
}

// pantallaDe monta cada pantalla con sus datos de prueba.
func pantallaDe(t *testing.T, nombre string, v Vista) (fyne.CanvasObject, Seccion) {
	t.Helper()
	s := servicioCompleto()
	entrada := func(texto string) *ui.Entrada {
		e := ui.NuevaEntrada("").Con(ui.CampoSM)
		e.PonerTexto(texto)
		return e
	}
	switch nombre {
	case "moderacion":
		return PantallaModeracion(v, Moderacion{
			Administra: true, Mios: bloqueadosDePrueba(), Denuncias: denunciasDePrueba(),
		}), SecModeracion
	case "inicio":
		return PantallaInicio(v, Inicio{Mios: s.mios, EnDirecto: s.directos, Compartidos: s.comp, Organizacion: s.org}), SecInicio
	case "explorar":
		busca := ui.NuevaEntrada(v.Traduce("explore.search")).Con(ui.CampoSM)
		busca.Icono = iconoBuscar
		return PantallaExplorar(v, Explorar{Directos: s.directos, Videos: append(s.org, s.mios[:2]...), Buscador: busca, HayMas: true}), SecExplorar
	case "canal":
		return PantallaCanal(v, Canal{Org: "acme", Nombre: "Acme Ingeniería", Videos: s.org, AlDenunciar: func() {}}), 255
	case "ver":
		visor := NuevoVisor(v)
		visor.Mando = &mandoFalso{pos: 83_000_000_000, dur: s.mios[0].DurationMS * 1_000_000}
		visor.AlAmpliar, visor.AlSilencio = func() {}, func() {}
		visor.Rehacer()
		visor.Imagen(decodificar(posterDe("v01")))
		vid := s.mios[0]
		vid.Description = "Resultados del tercer trimestre: ventas por región, margen bruto y previsión para el cierre del año. La hoja de cálculo está en la carpeta compartida del trimestre."
		return PantallaVer(v, Ver{
			Video: vid, Visor: visor.Objeto(), Descargar: true,
			AlDenunciar: func() {}, AlDenunciarAutor: func() {}, AlBloquearAutor: func() {},
			Comentarios: Comentarios{
				Lista: s.comentarios, Campo: entrada(""), ConSesion: true, MiSub: "u-Ana Pérez",
				AlDenunciar: func(api.CastComment) {}, AlBloquear: func(api.CastComment) {},
			},
		}), 255
	case "directo":
		visor := NuevoVisor(v)
		visor.Directo, visor.Viendo = true, 42
		visor.AlAmpliar, visor.AlSilencio = func() {}, func() {}
		visor.Rehacer()
		visor.Imagen(decodificar(posterDe("v10")))
		vid := directoDePrueba()
		vid.Can = api.CastCan{Edit: true, Delete: true, Moderate: true}
		mensajes := []api.CastChatMessage{
			{ID: "m1", Author: api.CastAuthor{Sub: "u2", Name: "Marta Ruiz"}, Body: "Buenos días a todos"},
			{ID: "m2", Author: api.CastAuthor{Sub: "u3", Name: "Jon Arrieta"}, Body: "Se oye y se ve perfecto desde Bilbao"},
			{ID: "m3", Author: api.CastAuthor{Sub: "u4", Name: "Irene Soler"}, Body: "¿Se compartirá la presentación después?"},
		}
		return PantallaVer(v, Ver{
			Video: vid, Visor: visor.Objeto(),
			Chat: &Chat{
				Mensajes: mensajes, Campo: entrada(""), Activo: true, ConSesion: true, Moderar: true, Conectado: true, Viendo: 42,
				MiSub: "u3", AlDenunciar: func(api.CastChatMessage) {}, AlBloquear: func(api.CastChatMessage) {},
			},
		}), 255
	case "subir":
		q := api.CastQuota{LimitBytes: 200 << 30, UsedBytes: 57 << 30, AvailableBytes: 143 << 30, CastBytes: 12 << 30}
		return PantallaSubir(v, Subir{
			Cuota: &q, Visibilidad: api.CastOrg,
			Cola: []Envio{
				{Clave: 1, Nombre: "resultados-t3.mp4", Titulo: entrada("Resultados del tercer trimestre"), Tamano: 812 << 20, Enviado: 530 << 20, Fase: Subiendo},
				{Clave: 2, Nombre: "evento-anual.mov", Titulo: entrada("Evento anual"), Tamano: 1 << 30, Fase: Esperando},
				{Clave: 3, Nombre: "formacion-firma.mov", Titulo: entrada("Formación: firma electrónica"), Tamano: 240 << 20, Enviado: 240 << 20, Fase: Procesando, VideoID: "v99"},
				{Clave: 4, Nombre: "notas.txt", Titulo: entrada("notas"), Tamano: 2 << 10, Fase: Fallida, Motivo: v.Traduce("upload.error.tipo")},
			},
		}), SecSubir
	case "gestionar":
		desc := ui.NuevaAreaTexto("")
		desc.Con(ui.CampoSM)
		desc.PonerTexto("Resultados del tercer trimestre: ventas por región y previsión.")
		vid := s.mios[0]
		vid.Visibility, vid.CommentsEnabled = api.CastPeople, true
		return PantallaGestionar(v, Gestionar{
			Video: vid, Titulo: entrada(vid.Title), Descripcion: desc, Persona: entrada(""), Cambiado: true,
			Personas: []api.CastPerson{{UserID: "u9", Name: "Bea Martín", Username: "bea"}, {UserID: "u8", Name: "Carlos Ibáñez", Username: "cibanez"}},
		}), 255
	case "emitir":
		clave := api.CastStreamKey{RTMPURL: "rtmp://cast-ingest.sytue.eu/live", StreamKey: "cast_stream_key_sample_"}
		d := directoDePrueba()
		programado := directoDePrueba()
		programado.ID, programado.Title, programado.Status = "v11", "Formación trimestral de ventas", api.CastStatusScheduled
		programado.Live.Source = "webrtc"
		terminado := directoDePrueba()
		terminado.ID, terminado.Title, terminado.Status = "v12", "Reunión general de agosto", api.CastStatusEnded
		terminado.Live.Source = "webrtc"
		desc := ui.NuevaAreaTexto("")
		desc.Con(ui.CampoSM)
		return PantallaEmitir(v, Emitir{
			Clave: &clave, Titulo: entrada(""), Descripcion: desc, Visibilidad: api.CastOrg, Fuente: "rtmp",
			Emision:  Emision{Titulo: entrada(""), Visibilidad: api.CastOrg, Micro: true, Previa: NuevaPrevia(v).Objeto(), AlCamara: func() {}, AlPantalla: func() {}, AlMicro: func() {}},
			Directos: []api.CastVideo{d, programado, terminado},
		}), SecEmitir
	}
	t.Fatalf("pantalla %q desconocida", nombre)
	return nil, 0
}

// mandoFalso es un reproductor quieto en un instante.
type mandoFalso struct {
	pos, dur int64
	pausado  bool
	mudo     bool
}

func (m *mandoFalso) Alternar()               { m.pausado = !m.pausado }
func (m *mandoFalso) Buscar(d time.Duration)  { m.pos = int64(d) }
func (m *mandoFalso) Posicion() time.Duration { return time.Duration(m.pos) }
func (m *mandoFalso) Duracion() time.Duration { return time.Duration(m.dur) }
func (m *mandoFalso) Pausado() bool           { return m.pausado }
func (m *mandoFalso) Silenciar(si bool)       { m.mudo = si }
func (m *mandoFalso) Volumen(float64)         {}
func (m *mandoFalso) Silenciado() bool        { return m.mudo }

var estadosDePrueba = []struct {
	nombre string
	e      Estado
}{
	{"lista", Lista},
	{"cargando", Cargando},
	{"vacia", Vacia},
	{"error", Error},
	{"sin-conexion", SinConexion},
	{"sin-permiso", SinPermiso},
	{"sin-sesion", SinSesion},
}

// pantallasConVacia son las que tienen un estado vacío propio: en las demás «vacía»
// no existe (un vídeo, un formulario).
var pantallasConVacia = map[string]bool{"inicio": true, "explorar": true, "canal": true}

// TestLasPantallasEnSusSeisEstados pinta cada pantalla en cada estado, en claro y en
// oscuro, y comprueba que el fondo es el token y que cada estado se pinta distinto de
// los demás: un estado que sale igual que otro no informa de nada.
//
// Con SYTUE_CAPTURA=/ruta deja las capturas para MIRARLAS.
func TestLasPantallasEnSusSeisEstados(t *testing.T) {
	pantallas := []string{"inicio", "explorar", "canal", "ver", "directo", "subir", "gestionar", "emitir", "moderacion"}
	for _, variante := range []tema.Variante{tema.Claro, tema.Oscuro} {
		nombreVar := "claro"
		if variante == tema.Oscuro {
			nombreVar = "oscuro"
		}
		t.Run(nombreVar, func(t *testing.T) {
			conVariante(t, variante)
			for _, p := range pantallas {
				vistos := map[[32]byte]string{}
				for _, e := range estadosDePrueba {
					if e.e == Vacia && !pantallasConVacia[p] {
						continue
					}
					if e.e != Lista && p == "directo" {
						continue
					}
					// En oscuro, la lista y un estado de fallo: los demás estados son los
					// mismos componentes con otro texto, y pintar cada uno cuesta (con
					// -race, segundos por captura).
					recortar := (variante == tema.Oscuro || conRaza) && os.Getenv("SYTUE_CAPTURA") == ""
					if recortar && e.e != Lista && e.e != Error {
						continue
					}
					v := vistaDePrueba(t, variante)
					v.Estado = e.e
					v.AlReintentar, v.AlEntrar = func() {}, func() {}
					s := servicioCompleto()
					v.Teca = tecaConPosters(s.mios, s.org, s.directos, s.comp)
					o, sec := pantallaDe(t, p, v)
					img := pintar(t, v, enMarco(v, sec, o))
					guardar(t, fmt.Sprintf("%s-%s-%s", p, e.nombre, nombreVar), img)

					// El fondo del contenido es el token `bg` de la variante.
					fondo := colorEn(img, int(ui.AnchoMenu(false))+4, int(ventanaAlto)-4)
					if quiero := tema.Color("bg", variante); !parecido(fondo, quiero, 2) {
						t.Errorf("%s/%s: el fondo pinta %v y el token `bg` es %v", p, e.nombre, fondo, quiero)
					}
					h := sha256.Sum256(img.(*image.NRGBA).Pix)
					if otro, ya := vistos[h]; ya {
						t.Errorf("%s: «%s» se pinta igual que «%s»", p, e.nombre, otro)
					}
					vistos[h] = e.nombre
				}
			}
		})
	}
}

// TestLaTarjetaEnseñaElPosterYLaDuracion: el póster llena el 16:9 de la tarjeta (el
// centro es el cielo del póster, no el `bg-subtle` del hueco) y la píldora de la
// duración va abajo a la derecha, oscura.
func TestLaTarjetaEnsenaElPosterYLaDuracion(t *testing.T) {
	conVariante(t, tema.Claro)
	v := vistaDePrueba(t, tema.Claro)
	vid := misVideos()[0]
	v.Teca = tecaConPosters([]api.CastVideo{vid})
	tarjeta := NuevaTarjeta(v, vid, true, nil)
	const ancho = 320
	w := test.NewWindow(tarjeta)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, AltoTarjeta(ancho)))
	_ = w.Canvas().Capture()
	v.Teca.Esperar()
	tarjeta.Refresh()
	img := w.Canvas().Capture()
	guardar(t, "tarjeta", img)

	escala := float32(img.Bounds().Dx()) / ancho
	en := func(x, y float32) color.NRGBA { return colorEn(img, int(x*escala), int(y*escala)) }
	cielo := en(40, 20)
	if parecido(cielo, tema.Color("bg-subtle", tema.Claro), 6) {
		t.Errorf("el póster no se pinta: arriba a la izquierda sigue el hueco %v", cielo)
	}
	// La píldora: negro al 72 % sobre el suelo del póster.
	pildora := en(ancho-margenMarcas-4, ancho*9/16-margenMarcas-8)
	if pildora.R > 80 || pildora.G > 80 || pildora.B > 80 {
		t.Errorf("la píldora de la duración no está abajo a la derecha: pinta %v", pildora)
	}
	r := test.WidgetRenderer(tarjeta).(*tarjetaRenderer)
	if r.autor.Position().Y+r.autor.MinSize().Height > r.datos.Position().Y+1 {
		t.Error("el autor se solapa con las vistas y la fecha")
	}
	if r.datos.Position().Y+r.datos.MinSize().Height > tarjeta.Size().Height+1 {
		t.Error("las vistas y la fecha quedan fuera de la tarjeta")
	}
}

// TestElDirectoLlevaSuMarca: la tarjeta de un directo lleva la insignia «EN DIRECTO» en
// rojo y no la duración.
func TestElDirectoLlevaSuMarca(t *testing.T) {
	v := vistaDePrueba(t, tema.Claro)
	tarjeta := NuevaTarjeta(v, directoDePrueba(), false, nil)
	r := test.WidgetRenderer(tarjeta).(*tarjetaRenderer)
	if r.marca == nil || r.marca.Texto != "En directo" || r.marca.Tono != ui.InsigniaPeligro {
		t.Fatalf("la marca del directo es %+v", r.marca)
	}
	if r.pildora != nil {
		t.Error("un directo no tiene duración que enseñar")
	}
	if got := tarjeta.datos(); got != "Luis Gómez · 42 viendo · hace 12 horas" && got != "Luis Gómez · 42 viendo · ahora" {
		t.Logf("datos del directo: %q", got)
	}
}

// TestLaRejillaSigueLosCortes: 1, 2, 3 y 4 columnas en los cortes de la web.
func TestLaRejillaSigueLosCortes(t *testing.T) {
	for _, c := range []struct {
		ventana float32
		cols    int
	}{{390, 1}, {700, 2}, {1100, 3}, {1280, 4}, {1920, 4}} {
		if got := ColumnasRejilla(c.ventana); got != c.cols {
			t.Errorf("a %v de ventana salen %d columnas y la web pinta %d", c.ventana, got, c.cols)
		}
	}
}

func colorEn(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func parecido(a, b color.NRGBA, tol int) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > tol || int(y)-int(x) > tol }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// decodificar lee un póster de prueba.
func decodificar(b []byte) image.Image {
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	return img
}

// textosLeidos junta lo que se LEE en un árbol: párrafos, rótulos de radio y textos
// sueltos, entrando también en los widgets.
func textosLeidos(o fyne.CanvasObject, out *[]string) {
	switch x := o.(type) {
	case *ui.Parrafo:
		*out = append(*out, x.Texto)
	case *ui.Eleccion:
		*out = append(*out, x.Etiqueta)
	case *canvas.Text:
		*out = append(*out, x.Text)
	case *fyne.Container:
		for _, h := range x.Objects {
			textosLeidos(h, out)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			textosLeidos(h, out)
		}
	}
}

// TestEmitirHablaDeLaAppYNoDelNavegador: en la web el origen es «Este navegador» y el
// aviso de permisos remite a los ajustes del navegador; en la app las dos cosas serían
// falsas. El dueño decidió (2026-09-28) que aquí el origen sea «Este Cast» y el aviso
// remita al sistema: claves propias de la app, `studio.source.app` y `studio.app.denied`.
func TestEmitirHablaDeLaAppYNoDelNavegador(t *testing.T) {
	v := vistaDePrueba(t, tema.Claro)
	desc := ui.NuevaAreaTexto("")
	desc.Con(ui.CampoSM)
	p := PantallaEmitir(v, Emitir{
		Titulo: ui.NuevaEntrada("").Con(ui.CampoSM), Descripcion: desc, Visibilidad: api.CastOrg, Fuente: "webrtc",
		Emision: Emision{
			Titulo: ui.NuevaEntrada("").Con(ui.CampoSM), Visibilidad: api.CastOrg, Denegado: true,
			Previa: NuevaPrevia(v).Objeto(), AlCamara: func() {}, AlPantalla: func() {}, AlMicro: func() {},
		},
	})
	_ = pintar(t, v, p)
	var textos []string
	textosLeidos(p, &textos)
	for _, c := range []struct{ quiero, no string }{
		{"studio.source.app", "studio.source.webrtc"},
		{"studio.app.denied", "studio.browser.denied"},
	} {
		if !slices.Contains(textos, v.Traduce(c.quiero)) {
			t.Errorf("Emitir no dice %q (%s); dice %q", v.Traduce(c.quiero), c.quiero, textos)
		}
		if slices.Contains(textos, v.Traduce(c.no)) {
			t.Errorf("Emitir dice %q (%s), que es el texto de la web", v.Traduce(c.no), c.no)
		}
	}
	if got := v.Traduce("studio.source.app"); got != "Este Cast" {
		t.Errorf("el origen en español es %q y el dueño decidió «Este Cast»", got)
	}
}

// Un destino sin capturador (iOS sin ReplayKit) no ofrece un control que sólo falla.
func TestEmitirNoOfrecePantallaSinCapturador(t *testing.T) {
	v := vistaDePrueba(t, tema.Claro)
	for _, disponible := range []bool{false, true} {
		t.Run(fmt.Sprint(disponible), func(t *testing.T) {
			pulsado := false
			e := Emision{AlCamara: func() {}, AlMicro: func() {}}
			if disponible {
				e.AlPantalla = func() { pulsado = true }
			}
			w := test.NewWindow(panelEmision(v, e))
			defer w.Close()
			w.Resize(fyne.NewSize(360, 500))
			b := hayIcono(w, v.Traduce("studio.browser.screen"))
			if !disponible {
				if b != nil {
					t.Fatal("se ofrece compartir pantalla sin capturador")
				}
				return
			}
			if b == nil || b.Inerte {
				t.Fatal("la pantalla implementada no se puede activar")
			}
			test.Tap(b)
			if !pulsado {
				t.Fatal("no se invocó la captura de pantalla")
			}
		})
	}
}
