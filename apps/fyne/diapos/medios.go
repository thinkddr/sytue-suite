// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/base64"
	"image/color"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// «Insertar → Multimedia»: `MediaPanel` de Insertar.tsx y `addMediaBox` de
// SlidesCanvas.tsx. Un vídeo o un audio por su URL, o un fichero pequeño incrustado.

// MaxMedioIncrustado es MEDIA_FILE_MAX_BYTES: lo que cabe incrustado como dataURL.
const MaxMedioIncrustado = 512 * 1024

// Las extensiones de VIDEO_EXT y AUDIO_EXT, con el MIME que el navegador pone en el
// dataURL de un fichero de ese tipo (`FileReader.readAsDataURL`).
var (
	extVideo = map[string]string{
		"mp4": "video/mp4", "webm": "video/webm", "ogv": "video/ogg", "ogm": "video/ogg",
		"mov": "video/quicktime", "m4v": "video/x-m4v", "mkv": "video/x-matroska",
	}
	extAudio = map[string]string{
		"mp3": "audio/mpeg", "wav": "audio/wav", "ogg": "audio/ogg", "oga": "audio/ogg",
		"m4a": "audio/mp4", "aac": "audio/aac", "flac": "audio/flac", "opus": "audio/opus",
		"weba": "audio/webm",
	}
	reEsquema = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:`)
	reDatos   = regexp.MustCompile(`^data:(?:audio|video)/`)
)

// TipoMedio es `mediaKindFor`: "video" o "audio" por el MIME o por la extensión de un
// nombre o una URL (sin su `?…` ni su `#…`); "" si no se sabe.
func TipoMedio(mimeONombre string) string {
	s := strings.ToLower(strings.TrimSpace(mimeONombre))
	switch {
	case s == "":
		return ""
	case strings.HasPrefix(s, "video/"):
		return "video"
	case strings.HasPrefix(s, "audio/"):
		return "audio"
	}
	s, _, _ = strings.Cut(s, "?")
	s, _, _ = strings.Cut(s, "#")
	ext := s[strings.LastIndexByte(s, '.')+1:]
	if _, ok := extVideo[ext]; ok {
		return "video"
	}
	if _, ok := extAudio[ext]; ok {
		return "audio"
	}
	return ""
}

// OrigenMedio es `safeMediaSrc`: un dataURL de audio o vídeo y http(s) pasan; un
// dominio sin esquema se completa con https; cualquier otro esquema (javascript:,
// file:, blob:…) se rechaza con "". Es la frontera de confianza de lo que se teclea.
func OrigenMedio(crudo string) string {
	s := strings.TrimSpace(crudo)
	l := strings.ToLower(s)
	switch {
	case s == "":
		return ""
	case reDatos.MatchString(l), strings.HasPrefix(l, "http://"), strings.HasPrefix(l, "https://"):
		return s
	case reEsquema.MatchString(l):
		return ""
	}
	return "https://" + s
}

// DatosMedio es el dataURL de un fichero de audio o vídeo, con el MIME de su extensión;
// "" si la extensión no es de un medio.
func DatosMedio(nombre string, datos []byte) string {
	ext := strings.ToLower(nombre[strings.LastIndexByte(nombre, '.')+1:])
	mime, ok := extVideo[ext]
	if !ok {
		if mime, ok = extAudio[ext]; !ok {
			return ""
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(datos)
}

// Medio es `MediaSpec`.
type Medio struct {
	Tipo, Src                string
	Controles, Auto, EnBucle bool
}

// NuevoMedio es `addMediaBox`: el audio en una barra ancha abajo, el vídeo en una caja
// grande centrada; solo se escriben las banderas que se apartan del defecto. Si no cabe
// en un guardado, se dice y no se inserta.
func (e *Editor) NuevoMedio(m Medio) {
	src := OrigenMedio(m.Src)
	if src == "" || (m.Tipo != "video" && m.Tipo != "audio") {
		e.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, e.t("slides.msg.mediaFailed"), e.t("slides.msg.mediaFailed.detail")))
		return
	}
	c := Caja{ID: uuid(), Tipo: "media", X: 15, Y: 12, W: 70, H: 68, Src: src, TipoMedio: m.Tipo}
	if m.Tipo == "audio" {
		c.X, c.Y, c.W, c.H = 18, 76, 64, 12
	}
	si, no := true, false
	if m.Auto {
		c.AutoPlay = &si
	}
	if m.EnBucle {
		c.Bucle = &si
	}
	if !m.Controles {
		c.Controles = &no
	}
	if !e.cabeCaja(c) {
		e.avisar("slides.msg.mediaTooBig", "slides.msg.mediaTooBig.detail")
		return
	}
	e.empujarCaja(c)
}

// ---- el panel ----

// AnchoPanelMedio es el `w-64` del formulario.
var AnchoPanelMedio = tema.Espacio(64)

// abrirMedio despliega el formulario bajo «Insertar audio o vídeo».
func (e *Editor) abrirMedio(disparador *editor.BotonCinta) {
	var em *ui.Emergente
	p := &panelMedio{e: e, tipo: "video", controles: true}
	p.cerrar = func() { em.Cerrar() }
	disparador.Activo = true
	disparador.Refresh()
	em = ui.MostrarEmergente(disparador, p.armar(), editor.RellenoPaleta)
	em.AlRecogerse = func() { disparador.Activo = false; disparador.Refresh() }
	if c := fyne.CurrentApp().Driver().CanvasForObject(p.url); c != nil {
		c.Focus(p.url) // teclear no sigue escribiendo en el lienzo
	}
}

type panelMedio struct {
	e                        *Editor
	url                      *ui.Entrada
	tipos                    [2]*opcionMedio
	insertar                 *ui.Boton
	tipo                     string
	tipoFijado               bool
	controles, auto, enBucle bool
	cerrar                   func()
}

func (p *panelMedio) armar() fyne.CanvasObject {
	e := p.e
	v := e.variante()
	p.url = ui.NuevaEntrada(e.t("insert.media.urlExample")).ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(1.5), MargenY: (tema.Espacio(7) - 2*tema.Borde - tema.TextoXS.Linea) / 2,
		Texto: tema.TextoXS, Borde: "border", Radio: tema.RadioSM,
	})
	p.url.AlCambiar(func(s string) {
		if !p.tipoFijado {
			if k := TipoMedio(s); k != "" {
				p.elegir(k, false)
			}
		}
		p.insertar.Inerte = strings.TrimSpace(s) == ""
		p.insertar.Refresh()
	})
	for i, k := range []string{"video", "audio"} {
		p.tipos[i] = nuevaOpcionMedio(e.t("insert.media."+k), func() { p.elegir(k, true) })
	}
	p.elegir("video", false)
	casilla := func(clave string, valor *bool) fyne.CanvasObject {
		return nuevaCasillaMedio(e.t(clave), *valor, func(b bool) { *valor = b })
	}
	p.insertar = ui.NuevoBoton(e.t("insert.media.insertUrl"), p.insertarURL).Con(ui.Primario, ui.TallaSM)
	p.insertar.Inerte = true
	subir := ui.NuevoBoton(e.t("insert.media.upload"), p.subir).Con(ui.Secundario, ui.TallaSM)
	subir.Inerte = e.ElegirMedio == nil
	etiqueta := ui.Texto(e.t("insert.media.url"), tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	return container.New(&columnaMedio{}, etiqueta, p.url,
		container.New(&filaMitades{}, p.tipos[0], p.tipos[1]),
		casilla("insert.media.controls", &p.controles), casilla("insert.media.autoplay", &p.auto),
		casilla("insert.media.loop", &p.enBucle),
		container.New(&filaBotones{}, p.insertar, subir))
}

func (p *panelMedio) elegir(tipo string, aMano bool) {
	p.tipo = tipo
	p.tipoFijado = p.tipoFijado || aMano
	for i, k := range []string{"video", "audio"} {
		p.tipos[i].activa = k == tipo
		p.tipos[i].Refresh()
	}
}

func (p *panelMedio) medio(src, tipo string) Medio {
	return Medio{Tipo: tipo, Src: src, Controles: p.controles, Auto: p.auto, EnBucle: p.enBucle}
}

// insertarURL es `submitUrl`: una dirección que no se puede usar se dice y el panel
// sigue abierto.
func (p *panelMedio) insertarURL() {
	src := OrigenMedio(p.url.Texto())
	if src == "" {
		p.e.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, p.e.t("insert.msg.badUrl"), ""))
		return
	}
	p.cerrar()
	p.e.NuevoMedio(p.medio(src, p.tipo))
}

// subir es «Subir fichero…»: el tipo lo manda el fichero, y uno grande se rechaza.
func (p *panelMedio) subir() {
	p.e.ElegirMedio(func(nombre string, datos []byte, grande bool) {
		if grande {
			p.e.avisar("insert.msg.fileTooBig", "insert.msg.fileTooBig.detail")
			return
		}
		url := DatosMedio(nombre, datos)
		if url == "" {
			p.e.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, p.e.t("insert.msg.fileReadFailed"), ""))
			return
		}
		tipo := TipoMedio(nombre)
		p.cerrar()
		p.e.NuevoMedio(p.medio(url, tipo))
	})
}

// columnaMedio es el `flex flex-col gap-1.5` del formulario, de `w-64`, con la
// etiqueta pegada a su campo (`gap-0.5`).
type columnaMedio struct{}

func (columnaMedio) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := tema.TextoXS.Linea + tema.Espacio(0.5)
	for _, o := range objs[1:] {
		h += o.MinSize().Height
	}
	return fyne.NewSize(AnchoPanelMedio, h+float32(len(objs)-2)*tema.Espacio(1.5))
}

func (columnaMedio) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if tx, ok := objs[0].(*canvas.Text); ok {
		ui.ColocarTexto(tx, tema.TextoXS, 0, 0)
	}
	y := tema.TextoXS.Linea + tema.Espacio(0.5)
	for _, o := range objs[1:] {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h + tema.Espacio(1.5)
	}
}

// filaMitades es `flex gap-1` con dos `flex-1`.
type filaMitades struct{}

func (filaMitades) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1, objs[0].MinSize().Height)
}

func (filaMitades) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	w := (t.Width - tema.Espacio(1)) / 2
	for i, o := range objs {
		o.Move(fyne.NewPos(float32(i)*(w+tema.Espacio(1)), 0))
		o.Resize(fyne.NewSize(w, t.Height))
	}
}

// filaBotones es `flex gap-1.5` con los botones a su ancho.
type filaBotones struct{}

func (filaBotones) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1, objs[0].MinSize().Height)
}

func (filaBotones) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	x := float32(0)
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, 0))
		o.Resize(m)
		x += m.Width + tema.Espacio(1.5)
	}
}

// ---- las piezas del formulario ----

// opcionMedio es un `role="radio"` del tipo: `h-7 rounded-sm border text-xs`, con
// `border-primary bg-selected text-text` la elegida y `border-border bg-surface
// text-text-muted hover:bg-bg-subtle` la otra.
type opcionMedio struct {
	widget.BaseWidget
	texto    string
	activa   bool
	encima   bool
	alPulsar func()
}

var (
	_ fyne.Tappable     = (*opcionMedio)(nil)
	_ desktop.Hoverable = (*opcionMedio)(nil)
)

func nuevaOpcionMedio(texto string, f func()) *opcionMedio {
	o := &opcionMedio{texto: texto, alPulsar: f}
	o.ExtendBaseWidget(o)
	return o
}

func (o *opcionMedio) Tapped(*fyne.PointEvent)        { o.alPulsar() }
func (o *opcionMedio) MouseIn(*desktop.MouseEvent)    { o.encima = true; o.Refresh() }
func (o *opcionMedio) MouseMoved(*desktop.MouseEvent) {}
func (o *opcionMedio) MouseOut()                      { o.encima = false; o.Refresh() }

func (o *opcionMedio) AccessibilityLabel() string             { return o.texto }
func (o *opcionMedio) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (o *opcionMedio) MinSize() fyne.Size {
	o.ExtendBaseWidget(o)
	return fyne.NewSize(1, tema.Espacio(7))
}

func (o *opcionMedio) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &opcionMedioRenderer{
		o: o,
		// Con borde desde el principio: `ui.Caja` decide al nacer si lo pinta.
		caja:  ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM),
		texto: ui.Texto(o.texto, tema.TextoXS, ui.Normal, color.NRGBA{}),
	}
	r.Refresh()
	return r
}

type opcionMedioRenderer struct {
	o     *opcionMedio
	caja  *ui.Caja
	texto *canvas.Text
}

func (r *opcionMedioRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	w := ui.Medir(r.o.texto, tema.TextoXS, ui.Normal).Width
	ui.ColocarTexto(r.texto, tema.TextoXS, (t.Width-w)/2, (t.Height-tema.TextoXS.Linea)/2)
}

func (r *opcionMedioRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := tema.Color("surface", v), tema.Color("border", v), tema.Color("text-muted", v)
	switch {
	case r.o.activa:
		fondo, borde, tinta = tema.Color("selected", v), tema.Color("primary", v), tema.Color("text", v)
	case r.o.encima:
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, borde)
	r.texto.Color = tinta
	r.texto.Refresh()
	r.Layout(r.o.Size())
}

func (r *opcionMedioRenderer) MinSize() fyne.Size { return r.o.MinSize() }
func (r *opcionMedioRenderer) Objects() []fyne.CanvasObject {
	return append(r.caja.Objetos(), r.texto)
}
func (r *opcionMedioRenderer) Destroy() {}

// casillaMedio es la casilla NATIVA del formulario (`<input type="checkbox"
// class="accent-primary">` con su `<label>` `gap-1.5 text-xs`): el cuadrado de 13 del
// navegador, con el borde gris de Chromium y, marcada, el acento con el tic blanco. No es
// la `Checkbox` del sistema de diseño (`ui.Eleccion`, de 14 y con `text-base`).
type casillaMedio struct {
	widget.BaseWidget
	texto     string
	marcada   bool
	alCambiar func(bool)
}

var _ fyne.Tappable = (*casillaMedio)(nil)

// ladoCasillaNativa es el 13 × 13 del `<input type="checkbox">` de Chromium, con
// `accent-primary`: lo pinta la pieza de Chromium de `ui`.
const ladoCasillaNativa float32 = 13

func nuevaCasillaMedio(texto string, marcada bool, f func(bool)) *casillaMedio {
	c := &casillaMedio{texto: texto, marcada: marcada, alCambiar: f}
	c.ExtendBaseWidget(c)
	return c
}

func (c *casillaMedio) Tapped(*fyne.PointEvent) {
	c.marcada = !c.marcada
	c.alCambiar(c.marcada)
	c.Refresh()
}

func (c *casillaMedio) AccessibilityLabel() string             { return c.texto }
func (c *casillaMedio) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (c *casillaMedio) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(1, tema.TextoXS.Linea)
}

func (c *casillaMedio) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &casillaMedioRenderer{
		c:     c,
		pieza: ui.NuevaPiezaCasilla(ladoCasillaNativa, false, false, v),
		texto: ui.Texto(c.texto, tema.TextoXS, ui.Normal, tema.Color("text", v)),
	}
	r.Refresh()
	return r
}

type casillaMedioRenderer struct {
	c     *casillaMedio
	pieza *ui.PiezaCasilla
	texto *canvas.Text
}

func (r *casillaMedioRenderer) Layout(t fyne.Size) {
	y := (t.Height - ladoCasillaNativa) / 2
	r.pieza.Colocar(fyne.NewPos(0, y))
	ui.ColocarTexto(r.texto, tema.TextoXS, ladoCasillaNativa+tema.Espacio(1.5), (t.Height-tema.TextoXS.Linea)/2)
}

func (r *casillaMedioRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.pieza.Pintar(ui.EstadoCasilla{Marcada: r.c.marcada}, v)
	r.Layout(r.c.Size())
}

func (r *casillaMedioRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *casillaMedioRenderer) Objects() []fyne.CanvasObject {
	return append(r.pieza.Objetos(), r.texto)
}
func (r *casillaMedioRenderer) Destroy() {}
