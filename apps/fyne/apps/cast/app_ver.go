// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg" // los pósters son JPEG
	_ "image/png"  // y los propios pueden ser PNG
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ---- Ver ----

// AbrirVideo enseña la página de un vídeo: con sesión por /api/v1 (la ACL decide) y
// sin ella por /api/v1/public (solo public y unlisted).
func (a *App) AbrirVideo(id string) {
	a.apuntar(lugar{ruta: RutaVer, param: id})
	a.salirDeVer()
	a.ruta = RutaVer
	a.gen++
	a.scroll.Offset = fyne.NewPos(0, 0)
	a.ver = estadoVer{id: id, estadoCarga: estadoCarga{cargando: true}}
	a.visor = NuevoVisor(a.v)
	a.visor.AlAmpliar = a.pantallaCompleta
	a.visor.AlSilencio = a.alternarSilencio
	a.visor.AlMedir = a.medidoVisor
	a.visor.Rehacer()
	a.comentario.PonerTexto("")
	a.mensaje.PonerTexto("")
	a.pintar()
	a.pedirVideo()
	// Mientras no se puede ver (se sube, se procesa, está programado o terminado a la
	// espera de la grabación) la ficha se vuelve a pedir, y en cuanto se puede ver
	// empieza sola; sin sesión, en directo, para el contador (useSondeo de la web).
	a.sondear(8*time.Second, a.hayQueSondearVer, a.refrescarVideo)
	// Abierto con el teléfono ya tumbado: a pantalla completa, como al girarlo.
	if a.Movil && a.movil.tumbado {
		a.entrarAmpliado()
	}
}

// reproducible dice si el vídeo se puede ver ya.
func reproducible(s string) bool { return s == api.CastStatusReady || s == api.CastStatusLive }

func (a *App) hayQueSondearVer() bool {
	vid := a.ver.video
	return vid.ID != "" && (!reproducible(vid.Status) || (vid.EnDirecto() && !a.dentro()))
}

// refrescarVideo vuelve a pedir la ficha: si ha pasado a verse, se vuelve a abrir la
// página (con su reproductor o su directo); si no, se pone al día lo que se enseña.
func (a *App) refrescarVideo() {
	if a.cli == nil {
		return
	}
	gen, id, dentro, antes := a.gen, a.ver.id, a.dentro(), a.ver.video.Status
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		var vid api.CastVideo
		var err error
		if dentro {
			vid, err = a.cli.Video(c, id)
		} else {
			vid, err = a.cli.PublicVideo(c, id)
		}
		a.hilo(gen, func() {
			if err != nil {
				return
			}
			if vid.Status != antes && reproducible(vid.Status) {
				a.AbrirVideo(id)
				return
			}
			a.ver.video = vid
			if vid.Live != nil {
				a.ver.viendo = vid.Live.ViewersNow
				if a.visor != nil {
					a.visor.Viendo = vid.Live.ViewersNow
					a.visor.Rehacer()
				}
			}
			if vid.Status != antes {
				a.prepararVisor(nil)
			}
			a.pintar()
		})
	})
}

func (a *App) pedirVideo() {
	if a.cli == nil {
		return
	}
	gen, id, dentro := a.gen, a.ver.id, a.dentro()
	a.ver.cargando, a.ver.fallo = true, nil
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		var vid api.CastVideo
		var play api.CastPlay
		var err error
		if dentro {
			vid, err = a.cli.Video(c, id)
		} else {
			vid, err = a.cli.PublicVideo(c, id)
		}
		var errPlay error
		if err == nil && (vid.Status == api.CastStatusReady || vid.Status == api.CastStatusLive) {
			if dentro {
				play, errPlay = a.cli.Play(c, id)
			} else {
				play, errPlay = a.cli.PublicPlay(c, id)
			}
		}
		a.hilo(gen, func() {
			a.ver.cargando, a.ver.fallo = false, err
			if err != nil {
				a.pintar()
				return
			}
			a.ver.video = vid
			if errPlay == nil && (play.WebMURL != "" || play.Live != nil) {
				a.ver.play = &play
			}
			if vid.Live != nil {
				a.ver.viendo = vid.Live.ViewersNow
			}
			a.recordarPosters([]api.CastVideo{vid})
			a.prepararVisor(errPlay)
			a.pintar()
			if vid.EnDirecto() {
				a.abrirChat()
			} else if vid.CommentsEnabled {
				a.pedirComentarios(false)
			}
		})
	})
}

// prepararVisor pone en el visor lo que toca según el estado del vídeo: el aviso de lo
// que aún no se puede ver, el reproductor del WebM o la sala del directo.
func (a *App) prepararVisor(errPlay error) {
	vid, x := a.ver.video, a.visor
	a.cargarPosterEnVisor(vid.PosterURL)
	if !reproducible(vid.Status) {
		clave := map[string]string{
			api.CastStatusUploading: "uploading", api.CastStatusProcessing: "processing",
			api.CastStatusFailed: "failed", api.CastStatusScheduled: "scheduled", api.CastStatusEnded: "ended",
		}[vid.Status]
		if clave == "" {
			clave = "processing"
		}
		texto := a.v.Traduce("watch."+clave+".title") + ". " + a.v.Traduce("watch."+clave+".description")
		x.Aviso(texto, clave == "processing" || clave == "uploading")
		return
	}
	if errPlay != nil || a.ver.play == nil {
		x.Aviso(a.v.Traduce("player.error"), false)
		return
	}
	if vid.EnDirecto() {
		a.verDirecto()
		return
	}
	// El menú de calidad si hay más de una, e imagen dentro de imagen (solo bajo
	// demanda, como en la web).
	if len(a.ver.play.Calidades()) > 1 {
		x.AlCalidad = a.menuCalidad
	}
	switch {
	case !a.Movil:
		x.AlPiP = a.alternarPiP
	case a.PiPMovil != nil && a.PiPMovil.Disponible():
		// En el teléfono no hay otra ventana: la del sistema (app_movil.go).
		x.AlPiP = a.pipMovil
	}
	a.ver.enUso = porDefecto(a.ver.play.Calidades())
	ruta := a.ver.play.WebMURL
	if q := a.objetivoAuto(); q > 0 && q != a.ver.enUso {
		// El visor ya está medido: se abre directamente la que toca por tamaño.
		ruta, a.ver.enUso = a.ver.play.WebMURLDe(q), q
		a.ver.ultimoAuto = time.Now()
	}
	a.abrirReproductor(ruta)
}

// porDefecto es la calidad de webm_url: 720p si la hay, si no la mayor por debajo, y
// si no la menor (domain.WebMPorDefecto del servicio).
func porDefecto(qs []int) int {
	mejor := 0
	for _, q := range qs {
		if q <= 720 && q > mejor {
			mejor = q
		}
	}
	if mejor == 0 && len(qs) > 0 {
		mejor = slices.Min(qs)
	}
	return mejor
}

// cargarPosterEnVisor pone el póster mientras llega el primer fotograma.
func (a *App) cargarPosterEnVisor(ruta string) {
	gen := a.gen
	if ruta == "" || a.cli == nil {
		return
	}
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		datos, err := a.cli.Poster(c, ruta)
		if err != nil {
			return
		}
		img, _, err := image.Decode(bytes.NewReader(datos))
		if err != nil {
			return
		}
		a.hilo(gen, func() {
			if a.pip != nil && a.pip.id == a.ver.id {
				return // suena en la otra ventana: el cuadro se queda en negro con su aviso
			}
			if a.visor != nil && !a.visor.TieneImagen() {
				a.visor.Imagen(img)
			}
		})
	})
}

// abrirReproductor abre el WebM por rangos y arranca el reproductor en pausa. Si la
// ventana de imagen dentro de imagen tiene ya este vídeo, no se abre otro: la página
// enseña que suena allí (o, si se volvió con «volver», se lo queda).
func (a *App) abrirReproductor(ruta string) {
	gen := a.gen
	if a.recogerPiP() {
		a.pararReloj = make(chan struct{})
		go a.reloj(gen, a.pararReloj)
		return
	}
	a.lanzar(func() {
		f, r, err := a.abrirWebM(ruta)
		a.hilo(gen, func() {
			if err != nil {
				a.visor.Aviso(a.v.Traduce("player.error"), false)
				return
			}
			a.fuente, a.reproductor, a.ver.medida = f, r, f
			a.enchufar(r)
			r.Arrancar()
			a.pararReloj = make(chan struct{})
			go a.reloj(gen, a.pararReloj)
		})
	})
}

// recogerPiP: la página del vídeo que suena en la ventana de imagen dentro de imagen
// no abre otro reproductor. Si la ventana se cerró con «volver», se queda con el suyo;
// si sigue abierta, sus mandos lo manejan y el cuadro dice dónde está.
func (a *App) recogerPiP() bool {
	if p := a.pendiente; p != nil {
		a.pendiente = nil
		if p.id == a.ver.id {
			a.devolver(p, true)
			return true
		}
		p.r.Cerrar()
		_ = p.f.Close()
	}
	if a.pip == nil || a.pip.id != a.ver.id || a.visor == nil {
		return false
	}
	a.ver.calidad, a.ver.enUso, a.ver.medida = a.pip.calidad, a.pip.enUso, a.pip.medida
	a.visor.Aviso(a.v.Traduce("player.pipActive"), false)
	a.conectar(a.pip.r)
	return true
}

// reloj mueve la barra y revisa la calidad automática mientras suena el vídeo.
// salirDeVer cancela también los pulsos ya enviados al hilo de la interfaz.
func (a *App) reloj(gen int, parar <-chan struct{}) {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-parar:
			return
		case <-t.C:
			a.hilo(gen, func() {
				select {
				case <-parar:
					return
				default:
				}
				if a.visor != nil {
					a.visor.Avanzar()
				}
				if r := a.deVer(); r != nil && !r.Pausado() {
					a.reevaluarAuto()
				}
			})
		}
	}
}

// verDirecto entra en la sala de LiveKit con el token de solo suscripción y pinta la
// pista de vídeo del emisor.
func (a *App) verDirecto() {
	x := a.visor
	x.Directo = true
	x.Viendo = a.ver.viendo
	x.Rehacer()
	conn := a.ver.play.Live
	if a.Conectar == nil || conn == nil {
		x.Aviso(a.v.Traduce("live.error"), false)
		return
	}
	x.Aviso(a.v.Traduce("live.connecting"), true)
	gen := a.gen
	a.lanzar(func() {
		sala := &salaCompartida{}
		o := llamada.Opciones{Micro: false, Audio: a.Audio}
		o.AlFotograma = func() {
			a.hilo(gen, func() {
				if img := imagenDelEmisor(sala.leer()); img != nil {
					a.visor.Aviso("", false)
					a.visor.Imagen(img)
				}
			})
		}
		o.AlSalir = func(string) {
			a.hilo(gen, func() { a.visor.Aviso(a.v.Traduce("watch.ended.title"), false) })
		}
		s, err := a.Conectar(conn.ServerURL, conn.Token, o)
		sala.poner(s)
		a.enLaInterfaz(func() {
			if gen != a.gen {
				// Se salió de la página mientras se conectaba: nadie la va a colgar.
				if err == nil {
					go s.Colgar()
				}
				return
			}
			if err != nil {
				x.Aviso(a.v.Traduce("live.error"), false)
				return
			}
			a.sala = s
			x.Aviso(a.v.Traduce("live.waiting"), true)
		})
	})
}

// salaCompartida guarda la sala para el aviso de fotograma, que llega de las rutinas de
// LiveKit y puede llegar antes de que Conectar haya devuelto.
type salaCompartida struct {
	mu sync.Mutex
	s  Sala
}

func (c *salaCompartida) poner(s Sala) { c.mu.Lock(); c.s = s; c.mu.Unlock() }

func (c *salaCompartida) leer() Sala {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.s
}

// imagenDelEmisor es el último fotograma de quien emite: su pantalla si la comparte,
// si no su cámara. El egress y los demás espectadores no publican vídeo.
func imagenDelEmisor(s Sala) image.Image {
	if s == nil {
		return nil
	}
	for _, p := range s.Participantes() {
		if p.Yo {
			continue
		}
		if p.Compartida != nil {
			return p.Compartida
		}
		if p.Video != nil {
			return p.Video
		}
	}
	return nil
}

// salirDeVer suelta lo de la página de un vídeo: reproductor, sala y chat. El de la
// ventana de imagen dentro de imagen NO: es de la ventana, y sigue sonando.
func (a *App) salirDeVer() {
	if a.ver.esperaAuto != nil {
		a.ver.esperaAuto.Stop()
	}
	if a.pip != nil {
		// Lo último que eligió la página se lo lleva la ventana.
		if a.pip.id == a.ver.id {
			a.pip.calidad, a.pip.enUso, a.pip.medida = a.ver.calidad, a.ver.enUso, a.ver.medida
		}
	}
	if a.pararReloj != nil {
		close(a.pararReloj)
		a.pararReloj = nil
	}
	if a.reproductor != nil {
		a.reproductor.Cerrar()
		a.reproductor = nil
	}
	if a.fuente != nil {
		_ = a.fuente.Close()
		a.fuente = nil
	}
	if a.sala != nil {
		s := a.sala
		a.sala = nil
		go s.Colgar()
	}
	if a.ctxChat != nil {
		a.ctxChat()
		a.ctxChat = nil
	}
	if a.chat != nil {
		a.chat.Close()
		a.chat = nil
	}
	if a.w != nil && a.w.FullScreen() {
		a.w.SetFullScreen(false)
	}
	a.salirDeAmpliado(false)
	a.prepararPiPMovil() // sin vídeo, la app ya no entra sola en la esquina
}

func (a *App) pantallaVer() fyne.CanvasObject {
	e := a.ver
	v := a.vistaDe(e.estadoCarga, e.video.ID == "", false, a.pedirVideo)
	vid := e.video
	// Lo de quien se ha bloqueado en esta sesión se quita de la pantalla.
	yo := ""
	if a.Yo != nil {
		yo = a.Yo()
	}
	comentarios := slices.DeleteFunc(slices.Clone(e.comentarios), func(c api.CastComment) bool { return a.bloqueado(c.Author.Sub) })
	d := Ver{
		Video:              vid,
		Descargar:          e.play != nil && e.play.DownloadURL != "" && a.Guardar != nil,
		DescripcionAbierta: e.descAbierta,
		AlAlternarDescripcion: func() {
			a.ver.descAbierta = !a.ver.descAbierta
			a.pintar()
		},
		AlCompartir: a.abrirCompartir,
		AlDescargar: a.descargar,
		AlGestionar: func() { a.AbrirGestionar(vid.ID) },
		AlTerminar:  func() { a.confirmarTerminar(vid) },
		Comentarios: Comentarios{
			Lista: comentarios, Cargando: e.cargandoCom, Error: e.errorCom, Campo: a.comentario,
			RespondiendoA: e.respondiendo, ConSesion: a.dentro(), Enviando: e.enviando,
			ErrorCarga: e.errorCargaCom, HayMas: e.cursorCom != "",
			AlComentar: a.comentar, AlEntrar: a.AlEntrar,
			AlReintentar: func() { a.pedirComentarios(false) }, AlMas: func() { a.pedirComentarios(true) },
			AlResponder: func(c api.CastComment) {
				a.ver.respondiendo = &c
				a.pintar()
				a.enfocar(a.comentario)
			},
			AlCancelarRespuesta: func() { a.ver.respondiendo = nil; a.pintar() },
			AlBorrar: func(k api.CastComment) {
				a.confirmar(a.v.Traduce("comments.deleteTitle"), a.v.Traduce("comments.deleteConfirm"), a.v.Traduce("comments.delete"),
					func() { a.borrarComentario(k) })
			},
			MiSub: yo,
			AlDenunciar: func(k api.CastComment) {
				a.AbrirDenuncia(api.CastNewReport{Kind: api.CastReportComment, VideoID: vid.ID, Target: k.ID}, "")
			},
			AlBloquear: func(k api.CastComment) { a.confirmarBloqueo(k.Author.Sub, k.Author.Name) },
		},
	}
	if vid.Owner.Sub != yo || yo == "" {
		d.AlDenunciar = func() { a.AbrirDenuncia(api.CastNewReport{Kind: api.CastReportVideo, VideoID: vid.ID}, "") }
		d.AlDenunciarAutor = func() {
			a.AbrirDenuncia(api.CastNewReport{Kind: api.CastReportPerson, VideoID: vid.ID, Target: vid.Owner.Sub}, vid.Owner.Name)
		}
		if a.dentro() {
			d.AlBloquearAutor = func() { a.confirmarBloqueo(vid.Owner.Sub, vid.Owner.Name) }
		}
	}
	if a.visor != nil {
		// El visor se conserva al girar el teléfono; sus esquinas siguen la vista actual.
		a.visor.v.Ventana, a.visor.v.Movil = v.Ventana, v.Movil
		d.Visor = a.visor.Objeto()
		if a.movil.ampliado != nil {
			// El visor está en la capa de pantalla completa: un objeto no puede estar en
			// dos sitios, y la página de debajo no se ve.
			d.Visor = canvas.NewRectangle(color.NRGBA{A: 255})
		}
	}
	if vid.Visibility == api.CastPublic {
		d.AlCanal = func() { a.AbrirCanal(vid.Tenant) }
	}
	if vid.EnDirecto() {
		mensajes := slices.DeleteFunc(slices.Clone(e.mensajes), func(m api.CastChatMessage) bool { return a.bloqueado(m.Author.Sub) })
		d.Chat = &Chat{
			MiSub: yo,
			AlDenunciar: func(m api.CastChatMessage) {
				a.AbrirDenuncia(api.CastNewReport{Kind: api.CastReportChat, VideoID: vid.ID, Target: m.ID}, "")
			},
			AlBloquear: func(m api.CastChatMessage) { a.confirmarBloqueo(m.Author.Sub, m.Author.Name) },
			Mensajes:   mensajes, Campo: a.mensaje, Activo: vid.ChatEnabled, ConSesion: a.dentro(),
			Moderar: vid.Can.Moderate, Conectado: e.conectado, Error: e.errorChat, Viendo: e.viendo,
			Lista: a.chatLista, AlEnviar: a.enviarChat, AlBorrar: a.borrarMensaje, AlEntrar: a.AlEntrar,
		}
	}
	return PantallaVer(v, d)
}

// enfocar pone el foco en un campo.
func (a *App) enfocar(o fyne.Focusable) {
	if a.w != nil {
		a.w.Canvas().Focus(o)
	}
}

// ---- Compartir ----

func (a *App) enlace() string {
	ruta := "/watch/" + a.ver.video.ID
	if a.WebDe != nil {
		return a.WebDe(ruta)
	}
	return ruta
}

// CodigoInsercion es el `<iframe>` de /cast/embed/<id>, el mismo que da oEmbed.
func CodigoInsercion(web, id, titulo string) string {
	src := strings.Replace(web, "/watch/"+id, "/embed/"+id, 1)
	titulo = strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(titulo)
	return `<iframe src="` + src + `" width="640" height="360" frameborder="0" ` +
		`allow="autoplay; fullscreen; picture-in-picture" allowfullscreen title="` + titulo + `"></iframe>`
}

// datosCompartir son los del diálogo, con lo último copiado.
func (a *App) datosCompartir() Compartir {
	vid := a.ver.video
	d := Compartir{
		Video: vid, Enlace: a.enlace(), Copiado: a.ver.copiado,
		AlCopiarEnlace: a.copiarEnlace, AlCopiarInsercion: a.copiarInsercion,
	}
	if vid.Visibility == api.CastPublic || vid.Visibility == api.CastUnlisted {
		d.Insercion = CodigoInsercion(d.Enlace, vid.ID, vid.Title)
	}
	return d
}

// abrirCompartir abre el ShareDialog: el enlace y, si el vídeo es abierto, el código
// de inserción, cada uno con su botón de copiar.
func (a *App) abrirCompartir() {
	a.ver.copiado = ""
	if a.w == nil {
		return
	}
	d := ui.NuevoDialogo(a.v.Traduce("share.title"), ui.AlAncho(anchoDialogo(a), CuerpoCompartir(a.v, a.datosCompartir())))
	d.RotuloCerrar = a.v.Traduce("ui.close")
	d.AlCerrar = func() { a.ver.compartir = nil; d.Cerrar() }
	a.ver.compartir = d
	d.Mostrar(a.w.Canvas())
}

// anchoDialogo es el ancho útil del cuerpo de un diálogo `md`.
func anchoDialogo(a *App) float32 {
	var lienzo fyne.Size
	if a.w != nil {
		lienzo = a.w.Canvas().Size()
	}
	return ui.AnchoCuerpoTalla(ui.DialogoMD, lienzo)
}

func (a *App) copiar(texto string) {
	if ui.Copiar(fyne.CurrentApp().Clipboard(), texto) {
		a.ver.copiado = a.v.Traduce("share.copied")
	} else {
		a.ver.copiado = a.v.Traduce("errors.description")
	}
	if d := a.ver.compartir; d != nil {
		d.CambiarCuerpo(ui.AlAncho(anchoDialogo(a), CuerpoCompartir(a.v, a.datosCompartir())))
	}
}

func (a *App) copiarEnlace() {
	a.ultimoEnlace = a.enlace()
	a.copiar(a.ultimoEnlace)
}

func (a *App) copiarInsercion() {
	a.ultimaInsercion = CodigoInsercion(a.enlace(), a.ver.video.ID, a.ver.video.Title)
	a.copiar(a.ultimaInsercion)
}

// descargar baja el original a la carpeta de descargas.
func (a *App) descargar() {
	if a.ver.play == nil || a.Guardar == nil || a.cli == nil {
		return
	}
	ruta, titulo, gen := a.ver.play.DownloadURL, a.ver.video.Title, a.gen
	a.lanzar(func() {
		m, err := a.cli.Media(context.Background(), ruta)
		var donde string
		if err == nil {
			donde, err = a.Guardar(nombreDescarga(titulo), m)
			_ = m.Close()
		}
		a.hilo(gen, func() {
			if err != nil {
				a.avisar(ui.AvisoPeligro, textoDeFallo(a.v, err))
			} else {
				a.avisar(ui.AvisoExito, donde) // la ruta del fichero guardado: se entiende sola
			}
		})
	})
}

// nombreDescarga es el título como nombre de fichero, sin lo que un sistema no admite.
func nombreDescarga(titulo string) string {
	limpio := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(titulo))
	if limpio == "" {
		limpio = "video"
	}
	return limpio + ".mp4"
}

// confirmarTerminar pide confirmación antes de cortar un directo.
func (a *App) confirmarTerminar(x api.CastVideo) {
	a.confirmar(a.v.Traduce("studio.browser.stopTitle"), a.v.Traduce("studio.browser.stopDescription"), a.v.Traduce("studio.browser.stop"),
		func() { a.terminar(x) })
}

// confirmar es el ConfirmDialog de peligro: un párrafo, cancelar y el verbo.
func (a *App) confirmar(titulo, mensaje, verbo string, alAceptar func()) {
	if a.w == nil {
		alAceptar() // sin ventana (los guardianes) no hay a quién preguntar
		return
	}
	ancho := ui.AnchoCuerpoTalla(ui.DialogoSM, a.w.Canvas().Size())
	texto := ui.NuevoParrafo(mensaje, tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}, ui.Normal, "text", a.v.Variante)
	var d *ui.Dialogo
	d = ui.Confirmacion(titulo, a.v.Traduce("ui.cancel"), verbo, true, ui.AlAncho(ancho, texto),
		func() { d.Cerrar() }, func() { d.Cerrar(); alAceptar() })
	d.RotuloCerrar = a.v.Traduce("ui.close")
	d.Mostrar(a.w.Canvas())
}

func (a *App) terminar(x api.CastVideo) {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		vid, err := a.cli.EndLive(c, x.ID)
		a.hilo(gen, func() {
			switch a.ruta {
			case RutaVer:
				if err != nil {
					a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
				} else {
					a.ver.video = vid
					a.salirDeVer()
					a.visor.Directo = false
					a.visor.Rehacer()
					a.visor.Aviso(a.v.Traduce("watch.ended.title"), false)
				}
			case RutaEmitir:
				if err != nil {
					a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
				} else {
					a.pedirDirectos()
				}
			default:
			}
			a.pintar()
		})
	})
}

// ---- Comentarios ----

func (a *App) pedirComentarios(mas bool) {
	if a.cli == nil {
		return
	}
	gen, id, dentro := a.gen, a.ver.id, a.dentro()
	cursor := ""
	if mas {
		cursor = a.ver.cursorCom
	}
	a.ver.cargandoCom = true
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		var cs []api.CastComment
		var sig string
		var err error
		if dentro {
			cs, sig, err = a.cli.Comments(c, id, cursor)
		} else {
			cs, sig, err = a.cli.PublicComments(c, id, cursor)
		}
		a.hilo(gen, func() {
			a.ver.cargandoCom = false
			switch {
			case err != nil && !mas:
				a.ver.errorCargaCom = true
			case err != nil:
				a.ver.errorCom = a.v.Traduce("comments.loadError")
			case mas:
				a.ver.comentarios = append(a.ver.comentarios, cs...)
				a.ver.cursorCom = sig
			default:
				a.ver.comentarios, a.ver.cursorCom, a.ver.errorCargaCom, a.ver.errorCom = cs, sig, false, ""
			}
			a.pintar()
		})
	})
}

func (a *App) comentar() {
	texto := strings.TrimSpace(a.comentario.Texto())
	if texto == "" || a.cli == nil || a.ver.enviando {
		return
	}
	respondeA := ""
	if a.ver.respondiendo != nil {
		respondeA = a.ver.respondiendo.ID
		if a.ver.respondiendo.ReplyTo != "" {
			respondeA = a.ver.respondiendo.ReplyTo // los hilos son de un nivel
		}
	}
	gen, id := a.gen, a.ver.id
	a.ver.enviando = true
	a.pintar()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		k, err := a.cli.Comment(c, id, texto, respondeA)
		a.hilo(gen, func() {
			a.ver.enviando = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce(claveSiBloqueado(err, "comments.sendError")))
			} else {
				a.ver.comentarios = append(a.ver.comentarios, k)
				a.ver.respondiendo, a.ver.errorCom = nil, ""
				a.comentario.PonerTexto("")
			}
			a.pintar()
		})
	})
}

func (a *App) borrarComentario(k api.CastComment) {
	if a.cli == nil {
		return
	}
	gen, id := a.gen, a.ver.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		err := a.cli.DeleteComment(c, id, k.ID)
		a.hilo(gen, func() {
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
			} else {
				for i := range a.ver.comentarios {
					if a.ver.comentarios[i].ID == k.ID {
						a.ver.comentarios[i].Deleted = true
					}
				}
			}
			a.pintar()
		})
	})
}

// ---- Chat ----

// abrirChat trae el historial y abre el WebSocket. Sin sesión se ve el historial
// que haya... no: el chat es con sesión (el servicio lo exige), así que sin ella solo
// se ofrece entrar.
//
//nolint:contextcheck // lo que llega por el chat repinta la pantalla, que no es del contexto del chat
func (a *App) abrirChat() {
	if a.cli == nil || !a.dentro() || !a.ver.video.ChatEnabled {
		return
	}
	c, cancelar := context.WithCancel(context.Background())
	a.ctxChat = cancelar
	gen, id := a.gen, a.ver.id
	a.lanzar(func() {
		if hist, err := a.cli.ChatHistory(c, id, time.Time{}); err == nil {
			a.hilo(gen, func() { a.ver.mensajes = hist; a.pintar(); a.chatAbajo() })
		}
	})
	a.conectarChat(c, gen, id, 0)
}

// conectarChat abre el WebSocket; si se corta, vuelve a probar con una espera que
// crece hasta 30 s mientras se siga en la página (el «Reconectando…» de la web).
//
//nolint:contextcheck // lo que llega por el chat repinta la pantalla, que no es del contexto del chat
func (a *App) conectarChat(c context.Context, gen int, id string, intento int) {
	reintentar := func() {
		espera := min(time.Duration(1<<min(intento, 5))*time.Second, 30*time.Second)
		time.AfterFunc(espera, func() {
			if c.Err() == nil {
				a.conectarChat(c, gen, id, intento+1)
			}
		})
	}
	a.lanzar(func() {
		ch, err := a.cli.Chat(c, id, func(f api.CastChatFrame) {
			a.hilo(gen, func() { a.trama(f) })
		}, func(err error) {
			a.hilo(gen, func() {
				a.ver.conectado = false
				if err != nil {
					a.ver.errorChat = a.v.Traduce("chat.reconnecting")
					reintentar()
				}
				a.pintar()
			})
		})
		a.hilo(gen, func() {
			if err != nil {
				a.ver.errorChat = a.v.Traduce("chat.reconnecting")
				if c.Err() == nil {
					reintentar()
				}
			} else {
				a.chat, a.ver.conectado, a.ver.errorChat = ch, true, ""
			}
			a.pintar()
		})
	})
}

// trama aplica una trama del chat.
func (a *App) trama(f api.CastChatFrame) {
	switch f.Type {
	case "message":
		m := api.CastChatMessage{ID: f.ID, Body: f.Body}
		if f.Author != nil {
			m.Author = *f.Author
		}
		if f.CreatedAt != nil {
			m.CreatedAt = *f.CreatedAt
		}
		a.ver.mensajes = append(a.ver.mensajes, m)
		// ponytail: el panel guarda los últimos 300; el historial entero está en el servicio.
		if len(a.ver.mensajes) > 300 {
			a.ver.mensajes = a.ver.mensajes[len(a.ver.mensajes)-300:]
		}
	case "deleted":
		for i, m := range a.ver.mensajes {
			if m.ID == f.ID {
				a.ver.mensajes = append(a.ver.mensajes[:i], a.ver.mensajes[i+1:]...)
				break
			}
		}
	case "viewers":
		a.ver.viendo = f.Count
		if a.visor != nil {
			a.visor.Viendo = f.Count
			a.visor.Rehacer()
		}
	case "error":
		// El límite de mensajes se dice con su texto; lo demás, «no se pudo enviar».
		a.ver.errorChat = a.v.Traduce("chat.sendError")
		switch f.Code {
		case "too_many_requests":
			a.ver.errorChat = a.v.Traduce("chat.tooFast")
		case "blocked":
			// Quien emite, o su organización, le ha bloqueado.
			a.ver.errorChat = a.v.Traduce("block.blockedYou")
		}
	}
	a.pintar()
	a.chatAbajo()
}

func (a *App) chatAbajo() {
	a.chatLista.Refresh()
	a.chatLista.ScrollToBottom()
}

func (a *App) enviarChat() {
	texto := strings.TrimSpace(a.mensaje.Texto())
	if texto == "" || a.chat == nil {
		return
	}
	ch := a.chat
	a.mensaje.PonerTexto("")
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		if err := ch.Send(c, texto); err != nil {
			fyne.Do(func() { a.ver.errorChat = a.v.Traduce("chat.sendError"); a.pintar() })
		}
	})
}

func (a *App) borrarMensaje(m api.CastChatMessage) {
	if a.cli == nil {
		return
	}
	gen, id := a.gen, a.ver.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		err := a.cli.DeleteChat(c, id, m.ID)
		if err != nil {
			a.hilo(gen, func() { a.ver.errorChat = textoDeFallo(a.v, err); a.pintar() })
		}
	})
}

// claveSiBloqueado es `block.blockedYou` cuando el servicio rechaza por un bloqueo, y
// si no la clave de siempre.
func claveSiBloqueado(err error, otra string) string {
	var e *api.Error
	if errors.As(err, &e) && e.Code() == "blocked" {
		return "block.blockedYou"
	}
	return otra
}
