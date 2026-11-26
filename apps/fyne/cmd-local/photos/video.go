// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/altavoz"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/local/videos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/fyne/video/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/videoteca"
)

// # Vídeos en la galería (FO-09)
//
// Los vídeos de las carpetas del escritorio entran enlazados (importar.go), y los del
// carrete del teléfono, los de «Añadir fotos» y los que se sueltan, copiados (copiar.go),
// con lo mismo (`local/videos`, que usa también el Hub al traerlos de Sytue): la duración
// y los lados (WebM y MP4/MOV se leen en Go;
// lo demás, con el descodificador del sistema o el ffprobe del equipo, si los hay) y un
// fotograma de póster, con lo que ya usa Cast: el reproductor propio para WebM
// (`video.Fotograma`), el descodificador del sistema (`video/sistema`: GStreamer en el
// escritorio, MediaCodec en Android), el ffmpeg del equipo (`local/conversor`) o la
// miniatura que da el sistema (`local/videoteca`: la del gestor de ficheros, Quick Look,
// `MediaMetadataRetriever`).
//
// En el visor se REPRODUCEN con el `video.Reproductor` de Cast: WebM VP8/VP9 con Opus en
// Go; lo demás, con el descodificador del sistema si lo hay y sabe el formato (GStreamer,
// Media Foundation en Windows, MediaCodec en Android). Si no hay con qué, el visor enseña el
// póster y ofrece «Abrir en Cast» (que tiene el conversor) o el reproductor del sistema.
// Uno copiado se lee del fichero que le prepara copiasParaVer (un enlace al original del
// la fototeca con su extensión). Un WebM cuyo vídeo no saca Go (VP9 de perfil 1, AV1) también
// va por el descodificador del sistema (`video.Nuevo`).
//
// «Abrir en Cast» da a Cast el fichero: el enlazado, tal cual (Cast lo apunta sin
// copiarlo); uno copiado, dejado en la carpeta de los vídeos de Cast como un enlace duro
// al original de la fototeca o una copia (copiasParaVer.paraCast): la caché de Photos no es
// sitio para Cast, que se queda con lo que se le da. En el teléfono no se ofrece Cast con
// uno copiado: allí cada app tiene su carpeta de datos y la otra no la lee.

// videoMedido es lo que se sabe de un vídeo al enlazarlo.
type videoMedido struct {
	poster      []byte
	duracion    time.Duration
	ancho, alto int
}

// prepararVideos registra el descodificador del sistema (GStreamer en el escritorio, Media
// Foundation en Windows, MediaCodec en Android) y enchufa al visor el reproductor y el aviso
// del original que falta.
func prepararVideos(l *local.App, p *photos.App, cli *api.Photos, copias *copiasParaVer) {
	if (!esMovil() || runtime.GOOS == "android") && sistema.Registrar() {
		slog.Info("photos: descodificador del sistema disponible para ver lo que no es WebM", "sistema", runtime.GOOS)
	}
	p.Reproductor = func(f api.Photo) (fyne.CanvasObject, func()) {
		r := nuevoReproductorFoto(l, cli, copias, f)
		return r, r.cerrar
	}
	p.AvisoDeFoto = func(f api.Photo) string { return avisoDeOriginal(l, f) }
}

// avisoDeOriginal es lo que dice el visor de una foto cuyo original enlazado ya no está.
func avisoDeOriginal(l *local.App, f api.Photo) string {
	switch f.EstadoExterno {
	case api.ExternoFalta:
		return texto(l, "original.falta")
	case api.ExternoCambiado:
		return texto(l, "original.cambiado")
	}
	return ""
}

// medirVideo saca la duración, los lados y un póster de un vídeo, con lo que haya
// (`local/videos`, lo mismo que usa el Hub al traer los de una copia de Sytue). Lo que
// no se puede saber se queda a cero (sin póster, la fototeca pone uno neutro).
func medirVideo(ctx context.Context, ruta string) videoMedido {
	m := videos.Medir(ctx, ruta)
	return videoMedido{poster: m.Poster, duracion: m.Duracion, ancho: m.Ancho, alto: m.Alto}
}

// ----- El reproductor del visor -----

// reproductorFoto es lo que enseña el visor en vez de la foto en un vídeo: el vídeo con
// sus mandos (reproducir, la barra, el tiempo y «Abrir en Cast»), o su póster y por qué
// no se ve aquí.
type reproductorFoto struct {
	widget.BaseWidget
	l    *local.App
	foto api.Photo
	// copias da el fichero de un vídeo copiado a la fototeca (sin Externo).
	copias *copiasParaVer

	imagen *canvas.Image
	// reproducir y pausa son el mismo mando: se ve uno u otro (el glifo de un botón no
	// cambia después de montarlo).
	reproducir, pausa *ui.Boton
	barra             *widget.Slider
	tiempo            *widget.Label
	aviso             *widget.Label
	mandos            *fyne.Container
	cuerpo            *fyne.Container
	fichero           *os.File
	// carpetaCast es la de los vídeos de Cast (`videoteca.CarpetaEn`), donde se deja uno
	// copiado para «Abrir en Cast».
	carpetaCast string

	mu      sync.Mutex
	r       *video.Reproductor
	cerrado bool
	// ultimo es cuándo se puso el tiempo por última vez (en el hilo de la interfaz).
	ultimo time.Time
}

func nuevoReproductorFoto(l *local.App, cli *api.Photos, copias *copiasParaVer, f api.Photo) *reproductorFoto {
	p := &reproductorFoto{l: l, foto: f, copias: copias, carpetaCast: videoteca.CarpetaEn(l.Espacio.Rutas.Datos)}
	p.ExtendBaseWidget(p)
	p.imagen = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	p.imagen.FillMode = canvas.ImageFillContain
	p.imagen.ScaleMode = canvas.ImageScaleSmooth
	fondo := canvas.NewRectangle(color.Black)
	p.aviso = widget.NewLabel("")
	p.aviso.Wrapping = fyne.TextWrapWord
	p.aviso.Alignment = fyne.TextAlignCenter
	p.aviso.Hide()
	p.reproducir = ui.NuevoBoton(texto(l, "video.reproducir"), p.Alternar).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Reproducir, 0)
	p.pausa = ui.NuevoBoton(texto(l, "video.pausa"), p.Alternar).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Pausa, 0)
	p.pausa.Hide()
	p.barra = widget.NewSlider(0, 1)
	p.barra.Step = 0.001
	p.barra.OnChangeEnded = p.buscar
	p.tiempo = widget.NewLabel("0:00")
	cast := ui.NuevoBoton(texto(l, "video.cast"), p.abrirEnCast).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Emitir, 0)
	if !p.ofreceCast() {
		cast.Hide()
	}
	p.mandos = container.NewBorder(nil, nil, container.NewStack(p.reproducir, p.pausa), container.NewHBox(p.tiempo, cast), p.barra)
	p.cuerpo = container.NewBorder(nil, container.NewPadded(p.mandos), nil, nil,
		container.NewStack(fondo, p.imagen, container.NewCenter(p.aviso)))
	// El póster mientras se abre, o para siempre si no se puede reproducir aquí.
	go p.ponerPoster(cli)
	go p.abrir()
	return p
}

func (p *reproductorFoto) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.cuerpo)
}

func (p *reproductorFoto) ponerPoster(cli *api.Photos) {
	c, fin := context.WithTimeout(context.Background(), 30*time.Second)
	defer fin()
	r, err := cli.VariantDe(c, p.foto.ID, photos.VarianteGrande, "")
	if err != nil {
		return
	}
	datos, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return
	}
	fyne.Do(func() {
		// Si ya llegó un fotograma del vídeo, el póster sobra.
		if p.ultimo.IsZero() {
			p.imagen.Image = img
			p.imagen.Refresh()
		}
	})
}

// errVideoFalta: el fichero enlazado de un vídeo ya no está.
var errVideoFalta = errors.New("photos: el vídeo enlazado ya no está")

// rutaDelVideo es el fichero del que se lee el vídeo: el enlazado o, si se copió a la
// fototeca, el que le prepara copiasParaVer (la primera vez puede tardar: si no se puede
// enlazar, se copia).
func (p *reproductorFoto) rutaDelVideo() (string, error) {
	if p.foto.Externo != "" {
		if p.foto.EstadoExterno == api.ExternoFalta {
			return "", errVideoFalta
		}
		return p.foto.Externo, nil
	}
	if p.copias == nil {
		return "", errVideoFalta
	}
	c, fin := context.WithTimeout(context.Background(), 30*time.Minute)
	defer fin()
	return p.copias.ruta(c, p.foto)
}

// ofreceCast dice si se ofrece «Abrir en Cast»: siempre con uno enlazado; con uno
// copiado, solo en el escritorio, donde se le deja en la carpeta de Cast (rutaParaCast);
// en el teléfono Cast es otra app con su propia carpeta de datos, que esta no alcanza.
func (p *reproductorFoto) ofreceCast() bool { return p.foto.Externo != "" || !esMovil() }

// abrir monta el reproductor sobre el fichero del vídeo. Fuera del hilo de la interfaz:
// abrir con GStreamer lanza un proceso, y con MediaCodec descodifica ya el primer fotograma.
func (p *reproductorFoto) abrir() {
	ruta, err := p.rutaDelVideo()
	if errors.Is(err, errVideoFalta) {
		p.decir(fmt.Sprintf(texto(p.l, "video.falta"), p.foto.Externo))
		return
	}
	if err != nil {
		slog.Warn("photos: preparar el vídeo para verlo", "nombre", p.foto.Name, "error", err)
		p.decir(texto(p.l, "video.noSeVe"))
		return
	}
	f, err := os.Open(ruta) //nolint:gosec // el vídeo enlazado o su fichero en la caché
	if err != nil {
		p.decir(fmt.Sprintf(texto(p.l, "video.falta"), ruta))
		return
	}
	r, err := video.Nuevo(f, altavoz.Sistema{})
	if err != nil {
		_ = f.Close()
		slog.Info("photos: este vídeo no se reproduce aquí", "nombre", p.foto.Name, "error", err)
		p.decir(texto(p.l, "video.noSeVe"))
		return
	}
	r.AlFotograma = func(img image.Image, t time.Duration) {
		fyne.Do(func() {
			p.imagen.Image = img
			p.imagen.Refresh()
			p.ponerTiempo(t, false)
		})
	}
	r.AlCambiar = func() { fyne.Do(p.ponerEstado) }
	p.mu.Lock()
	if p.cerrado {
		p.mu.Unlock()
		r.Cerrar()
		_ = f.Close()
		return
	}
	p.r, p.fichero = r, f
	p.mu.Unlock()
	r.Arrancar()
	fyne.Do(p.ponerEstado)
}

// decir enseña por qué no se reproduce (sobre el póster) y quita los mandos que no
// sirven; «Abrir en Cast» se queda.
func (p *reproductorFoto) decir(s string) {
	fyne.Do(func() {
		p.aviso.SetText(s)
		p.aviso.Show()
		p.reproducir.Hide()
		p.pausa.Hide()
		p.barra.Hide()
		p.tiempo.Hide()
	})
}

func (p *reproductorFoto) reproductor() *video.Reproductor {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r
}

// Alternar es el botón y la barra espaciadora del visor.
func (p *reproductorFoto) Alternar() {
	if r := p.reproductor(); r != nil {
		r.Alternar()
		p.ponerEstado()
	}
}

func (p *reproductorFoto) buscar(v float64) {
	r := p.reproductor()
	if r == nil || r.Duracion() <= 0 {
		return
	}
	r.Buscar(time.Duration(v * float64(r.Duracion())))
}

// ponerEstado pone el botón como toca: reproducir o pausa. En el hilo de la interfaz.
func (p *reproductorFoto) ponerEstado() {
	r := p.reproductor()
	if r == nil {
		return
	}
	if err := r.Err(); err != nil {
		p.decir(texto(p.l, "video.noSeVe"))
		return
	}
	if r.Pausado() {
		p.pausa.Hide()
		p.reproducir.Show()
	} else {
		p.reproducir.Hide()
		p.pausa.Show()
	}
}

// ponerTiempo pone el «0:12 / 1:23» y la barra, cuatro veces por segundo como mucho.
func (p *reproductorFoto) ponerTiempo(t time.Duration, ya bool) {
	if !ya && time.Since(p.ultimo) < 250*time.Millisecond {
		return
	}
	p.ultimo = time.Now()
	total := time.Duration(p.foto.DurationMs) * time.Millisecond
	if r := p.reproductor(); r != nil && r.Duracion() > 0 {
		total = r.Duracion()
	}
	p.tiempo.SetText(photos.DuracionCorta(t.Milliseconds()+1) + " / " + photos.DuracionCorta(total.Milliseconds()))
	if total > 0 {
		p.barra.Value = min(1, float64(t)/float64(total))
		p.barra.Refresh()
	}
}

// hayApp es local.HayApp; las pruebas lo cambian.
var hayApp = local.HayApp

// rutaParaCast es el fichero que se da a Cast: el enlazado, tal cual; uno copiado, el que
// le deja copiasParaVer.paraCast en su carpeta. Sin Cast instalado (si se sabe), el de verlo
// aquí, que se abre con el reproductor del sistema y no deja nada en la carpeta de Cast.
func (p *reproductorFoto) rutaParaCast() (string, error) {
	if p.foto.Externo != "" || p.copias == nil || p.carpetaCast == "" {
		return p.rutaDelVideo()
	}
	if hay, sabido := hayApp("cast"); sabido && !hay {
		return p.rutaDelVideo()
	}
	c, fin := context.WithTimeout(context.Background(), 30*time.Minute)
	defer fin()
	return p.copias.paraCast(c, p.foto, p.carpetaCast)
}

func (p *reproductorFoto) abrirEnCast() {
	if !p.ofreceCast() {
		return
	}
	// Uno copiado puede tardar en prepararse la primera vez: fuera del hilo de la interfaz.
	go func() {
		ruta, err := p.rutaParaCast()
		if err != nil {
			slog.Warn("photos: abrir en Cast", "error", err)
			return
		}
		fyne.Do(func() {
			if err := p.l.AbrirEn("cast", ruta); err != nil {
				slog.Warn("photos: abrir en Cast", "error", err)
			}
		})
	}()
}

// cerrar para el vídeo y suelta el fichero: al pasar a otra foto o cerrar el visor.
func (p *reproductorFoto) cerrar() {
	p.mu.Lock()
	p.cerrado = true
	r, f := p.r, p.fichero
	p.r, p.fichero = nil, nil
	p.mu.Unlock()
	if r != nil {
		r.Cerrar()
	}
	if f != nil {
		_ = f.Close()
	}
}
