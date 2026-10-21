// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command cast es Sytue Cast de escritorio y móvil: vídeo bajo demanda y directo
// (ADR 0061).
//
// Es SU PROPIA aplicación (`docs/CONVENTIONS.md` §13.6): habla con services/cast en
// directo y con el token de la sesión transversal, sin pasar por el BFF de la web. Lo
// público (la portada, los canales, los vídeos públicos) va sin sesión por
// `/api/v1/public`. Los vídeos se ven en Go: el WebM VP9/Opus con `video` y el directo
// con la llamada nativa de LiveKit (`llamada`), la misma de Rooms.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// servidor gráfico, es `go test ./apps/cast/...`, que pinta las mismas pantallas con
// el pintor de software y, con `SYTUE_CAPTURA=/ruta`, deja las capturas en disco.
package main

import (
	"context"
	"io"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/cast"
	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/altavoz"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// Ventana inicial: la misma con la que se mide la web (1280×800).
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las apps de la suite.
	s := sesion.Abrir(a, "")
	w := a.NewWindow("Sytue Cast")
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	tr, err := cast.Traductor(s.Idioma())
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}
	cli := api.NewCast(s.URL("cast"), s.Sesion, nil)
	teca := photos.NuevaFototeca()
	movil := a.Driver().Device().IsMobile()
	v := cast.Vista{T: tr, Ventana: anchoInicial, Variante: tema.De(a.Settings().ThemeVariant()), Teca: teca, Movil: movil}
	p := cast.Nueva(cli, v, w)
	p.Movil = movil
	p.SoltarCopia = sesion.SoltarElegido
	teca.Pedir = p.PedirPoster
	// Un póster que llega repinta la página: las tarjetas lo piden a la caché al pintarse.
	teca.AlLlegar = func(string) { fyne.Do(p.Repintar) }

	p.ConSesion = func() bool { return s.Estado().Dentro() }
	p.Yo = func() string { return s.Estado().Sujeto }
	p.AlEntrar = func() { go func() { _ = s.Entrar(context.Background()) }() }
	p.WebDe = func(ruta string) string { return api.AppURL(s.Apex(), "cast", "", ruta) }
	p.AbrirDrive = func() { _ = api.AbrirEn(a, s.Apex(), "drive", s.Sesion.Organizacion(), "/") }
	p.Elegir = func() ([]string, error) {
		return ficheros.ElegirConTipos(tr.T("upload.choose", nil), "mp4", "m4v", "mov", "mkv", "webm", "avi")
	}
	p.ElegirImagen = func() (string, error) {
		fs, err := ficheros.ElegirConTipos(tr.T("manage.posterUpload", nil), "jpg", "jpeg", "png", "webp")
		if err != nil || len(fs) == 0 {
			return "", err
		}
		return fs[0], nil
	}
	p.Guardar = func(nombre string, r io.Reader) (string, error) {
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(carpeta, nombre, r)
	}
	// El sonido y el directo: la tarjeta del sistema y la llamada nativa, como Rooms.
	if llamada.Disponible {
		p.Audio = altavoz.Sistema{}
		p.Conectar = func(u, token string, o llamada.Opciones) (cast.Sala, error) {
			l, err := llamada.Entrar(u, token, o)
			if err != nil {
				return nil, err
			}
			return l, nil
		}
		p.Camara = func() (camara.Fuente, error) {
			pedido := camara.PedidoPorOmision
			pedido.ID = p.CamaraElegida()
			return camara.Abrir(pedido)
		}
		p.Pantalla = camara.AbrirPantalla
		p.Camaras = camara.Dispositivos
		p.CamaraConcedida = camara.Concedido
		p.PedirMicro = altavoz.PedirMicro
	}
	// En el teléfono: la pantalla no se apaga mientras se ve o se emite, y al irse al
	// fondo el vídeo se pausa (y vuelve a entrar al directo al volver).
	if movil {
		p.PantallaEncendida = func(si bool) { sesion.PantallaEncendida(a, si) }
		// La imagen dentro de imagen del sistema. Si el teléfono la tiene se pregunta al
		// abrir cada vídeo: aquí la actividad todavía no existe y la respuesta sería «no».
		p.PiPMovil = sesion.ImagenEnImagen
		sesion.AlSucesoPiP(func(que int) { p.SucesoPiP(que == sesion.PiPCerrada) })
		s.AlFondo(p.AlFondo)
		s.AlVolver(p.AlVolver)
		// El campo en el que se escribe, por encima del teclado: se mide cuando el teclado
		// ya ha salido, que tarda lo que tarda su animación (en el emulador, más de 400 ms).
		ui.AlEscribir = func(c fyne.Canvas) {
			go func() {
				// Hasta que el alto deja de crecer: durante la animación el teclado mide
				// solo lo que lleva, y con esa medida el campo quedaba a medias debajo.
				antes := float32(-1)
				for range 20 {
					time.Sleep(100 * time.Millisecond)
					// En unidades del lienzo, con la conversión de cada sistema.
					px := ui.AltoTeclado(c)
					if px > 0 && px == antes {
						fyne.Do(func() { p.MostrarCampo(px) })
						break
					}
					antes = px
				}
				// Y cuando se va, fuera el hueco que se le dejó debajo de la página.
				for sesion.AltoTeclado() > 0 {
					time.Sleep(300 * time.Millisecond)
				}
				fyne.Do(func() { p.MostrarCampo(0) })
			}()
		}
	}

	// Entrar o salir en esta app o en otra de la suite vuelve a pedir la pantalla.
	dentro := s.Estado().Dentro()
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() != dentro {
			dentro = e.Dentro()
			fyne.Do(p.Recargar)
		}
	})

	// Un enlace a un vídeo, a un canal o a una pantalla de Cast llega como
	// `sytue-cast:/cast/watch/<id>` (o su dirección web): se abre aquí.
	s.RutaPropia(cast.EsEnlaceDeCast)
	s.AlAbrirURL(func(u *url.URL) {
		fyne.Do(func() { p.AbrirEnlace(u) })
	})

	// Soltar un vídeo encima de la ventana lo prepara para subir.
	soltar.AlArrastrar(w, p.Arrastrando)

	// Cast SÍ sirve sin sesión —la portada pública, los canales y los vídeos públicos—
	// y cada pantalla que la necesita ofrece entrar: la barra no la tapa con su página
	// «Iniciar sesión», que dejaba Explorar inalcanzable sin entrar.
	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Cast).ConSinSesionPropia().Montar(p.Objeto()))
	w.SetOnClosed(p.Cerrar)
	p.Arrancar()
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
