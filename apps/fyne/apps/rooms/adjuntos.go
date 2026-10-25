// extensionDeTipo es la extensión de un tipo que se lee (`isReadableInline` de la web:
// un PDF o un texto, que se abren en vez de solo bajarse), o "" si no se lee.
// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image"
	_ "image/gif"  // las miniaturas de Drive son JPEG, PNG o GIF
	_ "image/jpeg" // (ver `api.Drive.Thumbnail`)
	_ "image/png"
	"io"
	"math"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/externo"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/nombres"
)

// En la web: apps/frontend/apps/rooms/src/components/Attachments.tsx y la subida de
// Composer.tsx (`addFiles`, la lista de pendientes y `file_ids` al enviar).
//
// Los adjuntos de un mensaje son ficheros de Sytue Drive con ámbito de CANAL: solo los
// sirve a quien sigue en el canal. Las imágenes van como miniaturas (las de Drive, no el
// original: un canal con treinta fotos no baja 240 MB al abrirse) que se amplían en un
// visor; lo demás, como una tarjeta con su tamaño, «Descargar» y «Guardar en mi Drive».
//
// ponytail: el vídeo y el audio van como ficheros (se descargan y se abren con el
// reproductor del sistema); la web los reproduce en el sitio y Fyne no trae
// reproductor.

// glifos de Attachments.tsx (los de `glyphs.tsx`).
const (
	glifoFichero   = `<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z" /><path d="M14 3v5h5" />`
	glifoDescargar = `<path d="M12 3v12" /><path d="M7 10l5 5 5-5" /><path d="M5 21h14" />`
	glifoSubir     = `<path d="M12 15V3" /><path d="M7 8l5-5 5 5" /><path d="M5 21h14" />`
)

// Adjunto es un fichero de un mensaje.
type Adjunto struct {
	ID, Nombre, Tipo string
	Bytes            int64
}

// EsImagen es `isImage`.
func (a Adjunto) EsImagen() bool { return strings.HasPrefix(a.Tipo, "image/") }

// extensionDeTipo es la extensión de un tipo que se lee (`isReadableInline` de la web:
// un PDF o un texto, que se abren en vez de solo bajarse), o "" si no se lee.
func extensionDeTipo(tipo string) string {
	switch {
	case tipo == "application/pdf":
		return ".pdf"
	case strings.HasPrefix(tipo, "text/plain"):
		return ".txt"
	}
	return ""
}

// SeAbre dice si «Abrir» se ofrece: el tipo declarado se lee Y la extensión del nombre
// es la suya. El sistema abre un fichero por su EXTENSIÓN, no por el tipo que declaró
// quien lo subió: un «.html» declarado `application/pdf` se abría como HTML, sin el
// aislamiento de la web (FYC-01; decidido por el dueño: «Abrir solo si casan»). Si no
// casan, solo «Descargar».
func (a Adjunto) SeAbre() bool {
	ext := extensionDeTipo(a.Tipo)
	return ext != "" && strings.EqualFold(filepath.Ext(a.Nombre), ext)
}

// nombreEnDisco es el nombre con el que se guarda: el del adjunto, con la extensión de
// su tipo declarado detrás si no la lleva (FYC-01). Así lo que abre el sistema desde
// Descargas es lo que se dijo que era.
func (a Adjunto) nombreEnDisco() string {
	if ext := extensionDeTipo(a.Tipo); ext != "" && !strings.EqualFold(filepath.Ext(a.Nombre), ext) {
		return a.Nombre + ext
	}
	return a.Nombre
}

// Ficheros es lo que Rooms le pide a Sytue Drive para sus adjuntos; lo cumple
// *api.Drive.
type Ficheros interface {
	UploadToChannel(ctx context.Context, canal, nombre, tipo string, cuerpo io.Reader, tamano int64) (api.File, error)
	ChannelContent(ctx context.Context, canal, fichero string, miniatura bool) (io.ReadCloser, error)
	SaveChannelFileToMyDrive(ctx context.Context, canal, fichero string) (api.File, error)
}

var _ Ficheros = (*api.Drive)(nil)

// ---- La vista ----

// Las medidas de Attachments.tsx. HOJA.
var (
	// AireAdjuntos es el `mt-1.5` del bloque y HuecoAdjuntos su `gap-2`.
	AireAdjuntos  = tema.Espacio(1.5)
	HuecoAdjuntos = tema.Espacio(2)
	// LadoMiniatura es el `max-h-64 max-w-[16rem]` de una miniatura: 224.
	LadoMiniatura = tema.Espacio(64)
	// AnchoFichero es el `max-w-sm` de la tarjeta de un fichero (24 rem).
	AnchoFichero float32 = 24 * tema.Rem
	// CajaFichero es el `h-9 w-9` del icono de la tarjeta.
	CajaFichero = tema.Espacio(9)
)

// VistaAdjuntos es `Attachments`: las miniaturas en una fila que parte línea y debajo las
// tarjetas de los ficheros.
type VistaAdjuntos struct {
	widget.BaseWidget
	T        *i18n.Translator
	Adjuntos []Adjunto
	// Miniatura pide la de una imagen; nil, sin miniaturas (se ve el hueco).
	Miniatura func(Adjunto, func(image.Image))
	// AlAmpliar abre el visor en esa imagen; AlDescargar, AlAbrir y AlGuardar son las
	// acciones de un fichero.
	AlAmpliar   func(imagenes []Adjunto, i int)
	AlDescargar func(Adjunto)
	AlAbrir     func(Adjunto)
	AlGuardar   func(Adjunto)

	col   *fyne.Container
	ancho float32
}

// NuevaVistaAdjuntos crea la vista.
func NuevaVistaAdjuntos(t *i18n.Translator, as []Adjunto) *VistaAdjuntos {
	v := &VistaAdjuntos{T: t, Adjuntos: as}
	v.ExtendBaseWidget(v)
	return v
}

// AccessibilityLabel es el de la lista.
func (v *VistaAdjuntos) AccessibilityLabel() string { return v.T.T("composer.attachments", nil) }

// AccessibilityRole: un grupo.
func (v *VistaAdjuntos) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (v *VistaAdjuntos) imagenes() []Adjunto {
	return slices.DeleteFunc(slices.Clone(v.Adjuntos), func(a Adjunto) bool { return !a.EsImagen() })
}

// construir arma las dos partes.
func (v *VistaAdjuntos) construir() *fyne.Container {
	var partes []fyne.CanvasObject
	imgs := v.imagenes()
	if len(imgs) > 0 {
		ms := make([]fyne.CanvasObject, 0, len(imgs))
		for i, a := range imgs {
			m := NuevaMiniatura(v.T.T("attachment.enlarge", i18n.Vars{"name": a.Nombre}), func() {
				if v.AlAmpliar != nil {
					v.AlAmpliar(imgs, i)
				}
			})
			if v.Miniatura != nil {
				v.Miniatura(a, m.Poner)
			}
			ms = append(ms, m)
		}
		partes = append(partes, container.New(&envuelta{hueco: HuecoAdjuntos}, ms...))
	}
	for _, a := range v.Adjuntos {
		if a.EsImagen() {
			continue
		}
		partes = append(partes, container.New(anchoMax(AnchoFichero), v.fichero(a)))
	}
	return container.New(ui.Columna{Hueco: HuecoAdjuntos}, partes...)
}

// fichero es la tarjeta `border bg-surface rounded-lg max-w-sm pr-1.5 gap-1`: el enlace
// de descargar (icono, nombre, tamaño y flecha), «Abrir» si se lee y la nube.
func (v *VistaAdjuntos) fichero(a Adjunto) fyne.CanvasObject {
	t := v.T
	tamano := t.T("attachment.file", nil)
	if a.Bytes > 0 {
		tamano = i18n.FormatBytes(t.Locale(), float64(a.Bytes))
	}
	enlace := NuevoEnlaceFichero(a.Nombre, tamano, t.T("attachment.download", i18n.Vars{"name": a.Nombre}), func() {
		if v.AlDescargar != nil {
			v.AlDescargar(a)
		}
	})
	acciones := []fyne.CanvasObject{}
	if a.SeAbre() {
		abrir := ui.NuevoBotonIcono(iconos.FlechaArribaDerecha, t.T("attachment.open", i18n.Vars{"name": a.Nombre}), func() {
			if v.AlAbrir != nil {
				v.AlAbrir(a)
			}
		})
		abrir.Lado, abrir.Radio = tema.Espacio(8), tema.Forma("radius-control")
		acciones = append(acciones, abrir)
	}
	nube := ui.NuevoBotonIcono(iconos.Nube, t.T("attachment.saveToDrive", i18n.Vars{"name": a.Nombre}), func() {
		if v.AlGuardar != nil {
			v.AlGuardar(a)
		}
	})
	acciones = append(acciones, nube)
	tm := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	caja := ui.NuevaCaja(tema.Color("surface", tm), tema.Color("border", tm), tema.RadioLG)
	fila := container.NewBorder(nil, nil, nil,
		container.New(ui.Margen{Der: tema.Espacio(1.5)}, container.New(ui.Fila{Hueco: tema.Espacio(1)}, acciones...)), enlace)
	return container.NewStack(append(caja.Objetos(), fila)...)
}

// anchoMax es un `max-w-*`: el hijo a su ancho pedido, como mucho ese.
type anchoMax float32

func (a anchoMax) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(min(m.Width, float32(a)), m.Height)
}

func (a anchoMax) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.Position{})
	objs[0].Resize(fyne.NewSize(min(t.Width, float32(a)), t.Height))
}

// MedidaAdjuntos es lo que ocupa a ese ancho, con su `mt-1.5`.
func MedidaAdjuntos(v *VistaAdjuntos, ancho float32) fyne.Size {
	if v.col == nil {
		v.col = v.construir()
	}
	c := v.col
	c.Resize(fyne.NewSize(ancho, c.MinSize().Height)) // las filas que parten, al ancho
	return fyne.NewSize(ancho, AireAdjuntos+c.MinSize().Height)
}

func (v *VistaAdjuntos) CreateRenderer() fyne.WidgetRenderer {
	if v.col == nil {
		v.col = v.construir()
	}
	return &vistaAdjuntosRenderer{v: v}
}

type vistaAdjuntosRenderer struct{ v *VistaAdjuntos }

func (r *vistaAdjuntosRenderer) MinSize() fyne.Size { return MedidaAdjuntos(r.v, r.v.ancho) }

func (r *vistaAdjuntosRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.v.col} }
func (r *vistaAdjuntosRenderer) Destroy()                     {}
func (r *vistaAdjuntosRenderer) Refresh()                     { r.Layout(r.v.Size()) }
func (r *vistaAdjuntosRenderer) Layout(t fyne.Size) {
	r.v.ancho = t.Width
	r.v.col.Move(fyne.NewPos(0, AireAdjuntos))
	r.v.col.Resize(fyne.NewSize(t.Width, max(t.Height-AireAdjuntos, 0)))
}

// Miniatura es el botón de una imagen: `border bg-surface-sunken rounded-lg
// max-w-[16rem] overflow-hidden` con la miniatura `max-h-64` dentro. Mientras llega, el
// hueco con el fondo; si no llega, se queda el hueco.
type Miniatura struct {
	ui.Pulsable
	img  image.Image
	tam  fyne.Size
	objs []fyne.CanvasObject
}

var _ desktop.Hoverable = (*Miniatura)(nil)

// HuecoMiniatura es el hueco de una imagen que aún no ha llegado: 4:3 al alto máximo.
var HuecoMiniatura = fyne.NewSize(LadoMiniatura, LadoMiniatura*3/4)

// NuevaMiniatura crea el hueco; Poner le da la imagen.
func NuevaMiniatura(nombre string, al func()) *Miniatura {
	m := &Miniatura{tam: HuecoMiniatura}
	m.Nombre, m.AlPulsar = nombre, al
	m.ExtendBaseWidget(m)
	return m
}

// MedidaMiniatura es la imagen escalada a `max-h-64 max-w-[16rem]` sin deformarla, más
// el borde.
func MedidaMiniatura(ancho, alto int) fyne.Size {
	if ancho <= 0 || alto <= 0 {
		return HuecoMiniatura
	}
	w, h := float32(ancho), float32(alto)
	f := min(1, LadoMiniatura/h, LadoMiniatura/w)
	return fyne.NewSize(w*f+2*tema.Borde, h*f+2*tema.Borde)
}

// Poner pinta la miniatura, a su tamaño. Se llama en el hilo de la interfaz y ANTES de
// medir la fila: la Sala la tiene ya en su caché cuando rehace el historial.
func (m *Miniatura) Poner(img image.Image) {
	m.img = img
	b := img.Bounds()
	m.tam = MedidaMiniatura(b.Dx(), b.Dy())
}

func (m *Miniatura) MinSize() fyne.Size { return m.tam }

func (m *Miniatura) CreateRenderer() fyne.WidgetRenderer { return &miniaturaRenderer{m: m} }

type miniaturaRenderer struct{ m *Miniatura }

func (r *miniaturaRenderer) MinSize() fyne.Size           { return r.m.tam }
func (r *miniaturaRenderer) Objects() []fyne.CanvasObject { return r.m.objs }
func (r *miniaturaRenderer) Destroy()                     {}
func (r *miniaturaRenderer) Refresh()                     { r.Layout(r.m.Size()) }
func (r *miniaturaRenderer) Layout(t fyne.Size) {
	m := r.m
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := "border"
	if m.Encima() {
		borde = "border-strong"
	}
	caja := ui.NuevaCaja(tema.Color("surface-sunken", v), tema.Color(borde, v), tema.RadioLG)
	ui.MoverCaja(caja, fyne.Position{}, t)
	m.objs = caja.Objetos()
	if m.img != nil {
		im := canvas.NewImageFromImage(m.img)
		im.FillMode = canvas.ImageFillContain
		im.CornerRadius = tema.RadioLG - tema.Borde
		im.Move(fyne.NewPos(tema.Borde, tema.Borde))
		im.Resize(fyne.NewSize(t.Width-2*tema.Borde, t.Height-2*tema.Borde))
		m.objs = append(m.objs, im)
	}
	aro := ui.NuevoAro(tema.RadioLG, ui.SinHueco, v)
	aro.Colocar(t)
	aro.Ver(m.Enfocado())
	m.objs = append(m.objs, aro.Objetos()...)
}

// EnlaceFichero es la parte pulsable de la tarjeta de un fichero: `flex items-center
// gap-3 py-2 pl-3`, el icono en su caja de 31,5, el nombre en `text-sm font-medium`, el
// tamaño en `sytue-meta-dense` y la flecha de descargar.
type EnlaceFichero struct {
	ui.Pulsable
	Rotulo, Tamano string
	objs           []fyne.CanvasObject
}

var _ desktop.Hoverable = (*EnlaceFichero)(nil)

// NuevoEnlaceFichero crea el enlace; `nombre` es el accesible («Descargar …»).
func NuevoEnlaceFichero(rotulo, tamano, nombre string, al func()) *EnlaceFichero {
	e := &EnlaceFichero{Rotulo: rotulo, Tamano: tamano}
	e.Nombre, e.AlPulsar = nombre, al
	e.ExtendBaseWidget(e)
	return e
}

// MinSize: `py-2` y la caja del icono manda; de ancho, el icono, el nombre y la flecha.
func (e *EnlaceFichero) MinSize() fyne.Size {
	return fyne.NewSize(tema.Espacio(3)+CajaFichero+tema.Espacio(3)+
		max(ui.Medir(e.Rotulo, tema.TextoSM, ui.Medio).Width, ui.Medir(e.Tamano, tema.Texto2XS, ui.Normal).Width)+
		tema.Espacio(3)+iconos.TallaSM, 2*tema.Espacio(2)+CajaFichero)
}

func (e *EnlaceFichero) CreateRenderer() fyne.WidgetRenderer { return &enlaceFicheroRenderer{e: e} }

type enlaceFicheroRenderer struct{ e *EnlaceFichero }

func (r *enlaceFicheroRenderer) MinSize() fyne.Size           { return r.e.MinSize() }
func (r *enlaceFicheroRenderer) Objects() []fyne.CanvasObject { return r.e.objs }
func (r *enlaceFicheroRenderer) Destroy()                     {}
func (r *enlaceFicheroRenderer) Refresh()                     { r.Layout(r.e.Size()) }
func (r *enlaceFicheroRenderer) Layout(t fyne.Size) {
	e := r.e
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x := tema.Espacio(3)
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v), tema.RadioMD)
	ui.MoverCaja(caja, fyne.NewPos(x, (t.Height-CajaFichero)/2), fyne.NewSize(CajaFichero, CajaFichero))
	g := dibujo(glifoFichero, iconos.TallaLG, "text-muted", v).Objeto()
	g.Move(fyne.NewPos(x+(CajaFichero-iconos.TallaLG)/2, (t.Height-iconos.TallaLG)/2))
	g.Resize(fyne.NewSize(iconos.TallaLG, iconos.TallaLG))
	e.objs = append(caja.Objetos(), g)
	x += CajaFichero + tema.Espacio(3)
	flecha := "text-subtle"
	if e.Encima() {
		flecha = "text"
	}
	d := dibujo(glifoDescargar, iconos.TallaSM, flecha, v).Objeto()
	d.Move(fyne.NewPos(t.Width-iconos.TallaSM, (t.Height-iconos.TallaSM)/2))
	d.Resize(fyne.NewSize(iconos.TallaSM, iconos.TallaSM))
	cabe := max(t.Width-iconos.TallaSM-tema.Espacio(3)-x, 0)
	alto := tema.TextoSM.Linea + tema.Texto2XS.Linea
	y := (t.Height - alto) / 2
	nombre := ui.Texto("", tema.TextoSM, ui.Medio, tema.Color("text", v))
	ui.Truncar(nombre, e.Rotulo, tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(nombre, tema.TextoSM, x, y)
	tamano := ui.Texto(e.Tamano, tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v))
	ui.ColocarTexto(tamano, tema.Texto2XS, x, y+tema.TextoSM.Linea)
	e.objs = append(e.objs, nombre, tamano, d)
	aro := ui.NuevoAro(tema.RadioLG, ui.SinHueco, v)
	aro.Colocar(t)
	aro.Ver(e.Enfocado())
	e.objs = append(e.objs, aro.Objetos()...)
}

// ---- En la Sala ----

// adjuntosDe son los del mensaje del servicio.
func adjuntosDe(as []api.RoomsAttachment) []Adjunto {
	out := make([]Adjunto, 0, len(as))
	for _, a := range as {
		// El nombre lo pone quien lo subió: sin controles ni giros de escritura
		// (U+202E), que lo enseñaban con otra extensión (FYC-04).
		out = append(out, Adjunto{ID: a.FileID, Nombre: nombres.Limpio(a.Name), Tipo: a.ContentType, Bytes: a.SizeBytes})
	}
	return out
}

// miniatura entrega la de una imagen si ya está en la caché; si no, la pide y, al
// llegar, la guarda y rehace el historial, que así mide la fila con la imagen a su
// tamaño y no salta después. Una que falla se queda en su hueco y no se vuelve a pedir.
func (sa *Sala) miniatura(canal string) func(Adjunto, func(image.Image)) {
	if sa.Ficheros == nil {
		return nil
	}
	return func(a Adjunto, poner func(image.Image)) {
		if img, ok := sa.miniaturas[a.ID]; ok {
			if img != nil {
				poner(img)
			}
			return
		}
		sa.miniaturas[a.ID] = nil // pedida: no se vuelve a pedir en cada repintado
		sa.Lanzar(func() {
			rc, err := sa.Ficheros.ChannelContent(sa.ctx, canal, a.ID, true)
			if err != nil {
				return
			}
			defer func() { _ = rc.Close() }()
			img, _, err := image.Decode(rc)
			if err != nil {
				return
			}
			fyne.Do(func() {
				sa.miniaturas[a.ID] = img
				// Fuera de este pintado: la petición se hace mientras se construye el
				// historial, y rehacerlo desde dentro lo pisaría a medias.
				sa.Esperar(0, sa.P.Canal.Historial.Refresh)
			})
		})
	}
}

// bajar trae un adjunto del canal a la carpeta de descargas y devuelve dónde ha quedado.
func (sa *Sala) bajar(ctx context.Context, canal string, a Adjunto) (string, error) {
	rc, err := sa.Ficheros.ChannelContent(ctx, canal, a.ID, false)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	carpeta, err := ficheros.Carpeta()
	if err != nil {
		return "", err
	}
	return ficheros.GuardarDeInternet(carpeta, a.nombreEnDisco(), rc) // marcado como de Internet (FYS-02)
}

// Descargar deja el adjunto en la carpeta de descargas y lo enseña en el gestor de
// ficheros, que es lo que hace el navegador con su barra de descargas.
//
// ponytail: un fallo de disco o de red no se anuncia con texto propio; la web no tiene
// ese estado —la descarga la lleva el navegador— y no hay clave que lo diga.
func (sa *Sala) Descargar(canal string, a Adjunto) {
	if sa.Ficheros == nil {
		return
	}
	sa.Lanzar(func() {
		if ruta, err := sa.bajar(sa.ctx, canal, a); err == nil && sa.Revelar != nil {
			_ = sa.Revelar(ruta)
		}
	})
}

// AbrirAdjunto es «Abrir en una pestaña nueva» de un PDF o un texto: aquí, bajarlo y
// abrirlo con la aplicación del sistema, que es su pestaña.
func (sa *Sala) AbrirAdjunto(canal string, a Adjunto) {
	if sa.Ficheros == nil || !a.SeAbre() {
		return
	}
	sa.Lanzar(func() {
		if ruta, err := sa.bajar(sa.ctx, canal, a); err == nil && sa.AbrirFichero != nil {
			_ = sa.AbrirFichero(ruta)
		}
	})
}

// GuardarEnDrive es `guardarEnDrive`: copia el adjunto a «Mi unidad» y lo dice, y el
// fallo también: es una acción explícita, y callar haría creer que ya está guardado.
func (sa *Sala) GuardarEnDrive(canal string, a Adjunto) {
	if sa.Ficheros == nil || sa.guardandoAdjunto != "" {
		return
	}
	t := sa.P.T
	sa.guardandoAdjunto = a.ID
	pedir(sa, func(ctx context.Context) (api.File, error) {
		return sa.Ficheros.SaveChannelFileToMyDrive(ctx, canal, a.ID)
	},
		func(_ api.File, err error) {
			sa.guardandoAdjunto = ""
			if err != nil {
				sa.P.Avisar(ui.AvisoPeligro, t.T("attachment.saveToDriveFailed", nil))
				return
			}
			sa.P.Avisar(ui.AvisoExito, t.T("attachment.savedToDrive", i18n.Vars{"name": a.Nombre}))
		})
}

// adjuntosParaFila cablea la vista de los adjuntos de un mensaje con su canal: los
// adjuntos viven en el ámbito del canal y Drive exige ser miembro para servirlos.
func (sa *Sala) adjuntosParaFila(m Mensaje, v *VistaAdjuntos) {
	canal := m.CanalID
	if canal == "" {
		canal = sa.abierto
	}
	v.Miniatura = sa.miniatura(canal)
	v.AlAmpliar = func(imgs []Adjunto, i int) { sa.AbrirVisor(canal, imgs, i) }
	v.AlDescargar = func(a Adjunto) { sa.Descargar(canal, a) }
	v.AlAbrir = func(a Adjunto) { sa.AbrirAdjunto(canal, a) }
	v.AlGuardar = func(a Adjunto) { sa.GuardarEnDrive(canal, a) }
}

// ---- Subir: lo pendiente de la caja ----

// MaxAdjuntos es `MAX_ATTACHMENTS` de `lib/fileIds.ts`.
const MaxAdjuntos = 10

// maxImagenVisor es lo más que se lee del original de una imagen para verla grande
// (FYC-02). Una foto de cámara cabe de sobra; lo que pase de aquí no se amplía.
const maxImagenVisor = 64 << 20

// Pendiente es un adjunto de la caja antes de enviar: subiéndose, subido o fallido.
type Pendiente struct {
	Local   string
	Nombre  string
	Subido  *api.RoomsAttachment
	Fallido bool
	// Progreso es lo subido, de 0 a 1, mientras no ha llegado.
	Progreso float64
	// cancelar corta la subida en curso (el AbortController de la web): quitar la ficha
	// a medio subir no deja el fichero llegando al canal sin adjuntarse.
	cancelar context.CancelFunc
}

// SubirDelEquipo es «Subir desde tu equipo»: el selector de ficheros del sistema y cada
// uno subido al ÁMBITO del canal abierto —Drive lo saca de «Mi unidad», que es de toda
// la organización— hasta el tope por mensaje.
//
// Cada ficha dice su porcentaje mientras sube, como el `progress` de XHR de la web.
// Quitar una ficha a medio subir corta la subida.
//
// ponytail: sin el tope de 2 GB por fichero en el cliente: el que manda es el de Drive,
// que contesta 413.
func (sa *Sala) SubirDelEquipo() {
	canal, t := sa.abierto, sa.P.T
	c := sa.P.Canal.Compositor
	if canal == "" || sa.Ficheros == nil || sa.ElegirFicheros == nil {
		return
	}
	ambito := sa.ambitoAdjunto()
	pedir(sa, func(context.Context) ([]string, error) { return sa.ElegirFicheros() }, func(rutas []string, err error) {
		if err != nil || len(rutas) == 0 || !sa.adjuntoVigente(ambito) {
			return
		}
		sitio := MaxAdjuntos - len(c.Pendientes)
		if sitio <= 0 || len(rutas) > sitio {
			c.PonerAvisoComando(t.T("composer.maxAttachments", i18n.Vars{"count": MaxAdjuntos}))
		}
		for _, ruta := range rutas[:max(0, min(len(rutas), sitio))] {
			sa.subir(ambito, ruta)
		}
	})
}

// Los glifos de DrivePickerModal: `FolderGlyph` y `FileGlyph` en una sola `d`, que es lo
// que pide el selector de la suite.
const (
	rutaCarpetaDrive = "M3 7a2 2 0 0 1 2-2h3.5l2 2.5H19a2 2 0 0 1 2 2V18a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"
	rutaFicheroDrive = "M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z M14 3v5h5"
)

// ElegirDeDrive es «Elegir de Sytue Drive» (`DrivePickerModal`): el selector de la suite
// en modo Drive —carpetas y migas— y el fichero elegido entra en la caja POR REFERENCIA:
// sin subir bytes, sigue en Drive con sus permisos. No se repite el mismo, y cuenta para
// el tope por mensaje. Lo dice la nota bajo la lista (`drive.byReference`).
func (sa *Sala) ElegirDeDrive() {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil || sa.ListarDrive == nil {
		return
	}
	t := sa.P.T
	tx := func(k string) string { return t.T(k, nil) }
	c := sa.P.Canal.Compositor
	sel := &ui.SelectorSuite{
		Origen: api.ElegirDrive, Locale: sa.P.Locale,
		Textos: ui.TextosSelector{
			Titulo: tx("drive.pickTitle"), Migas: tx("drive.path"), Raiz: tx("drive.myDrive"),
			Vacio: tx("drive.emptyFolder"), Fallo: tx("drive.loadFailed"), Reintentar: tx("action.retry"),
			MasResultados: tx("action.loadMore"), Cancelar: tx("action.cancel"), Cerrar: tx("action.close"),
			Pie: tx("drive.byReference"),
		},
		GlifoCarpeta: rutaCarpetaDrive, GlifoElemento: rutaFicheroDrive,
		Cargar:   sa.ListarDrive,
		AlElegir: func(e api.Elegible) { sa.adjuntarDeDrive(c, e) }, // en el hilo de la interfaz: es la pulsación
	}
	sa.selectorDrive = sel
	sel.Mostrar(lienzo)
}

// adjuntarDeDrive pone el fichero elegido en la caja como adjunto ya subido.
func (sa *Sala) adjuntarDeDrive(c *Compositor, e api.Elegible) {
	if slices.ContainsFunc(c.Pendientes, func(p Pendiente) bool { return p.Subido != nil && p.Subido.FileID == e.ID }) {
		return
	}
	if len(c.Pendientes) >= MaxAdjuntos {
		c.PonerAvisoComando(sa.P.T.T("composer.maxAttachments", i18n.Vars{"count": MaxAdjuntos}))
		return
	}
	c.PonerPendientes(append(c.Pendientes, Pendiente{
		Local: "d-" + e.ID, Nombre: e.Nombre,
		Subido: &api.RoomsAttachment{FileID: e.ID, Name: e.Nombre, SizeBytes: e.Bytes},
	}))
}

// subir sube un fichero y pone su ficha en la caja.
type ambitoSubida struct {
	canal, yo, org string
	compositor     *Compositor
	archivos       Ficheros
	epoch          uint64
}

func (sa *Sala) ambitoAdjunto() ambitoSubida {
	org := ""
	if sa.Org != nil {
		org = sa.Org()
	}
	return ambitoSubida{canal: sa.abierto, yo: sa.Yo, org: org, compositor: sa.P.Canal.Compositor, archivos: sa.Ficheros, epoch: sa.adjuntosEpoch}
}

func (sa *Sala) adjuntoVigente(a ambitoSubida) bool {
	org := ""
	if sa.Org != nil {
		org = sa.Org()
	}
	return a.canal != "" && a.canal == sa.abierto && a.yo == sa.Yo && a.org == org && a.compositor == sa.P.Canal.Compositor && a.epoch == sa.adjuntosEpoch
}

func olvidarSubida(f Ficheros, canal, id string) {
	if id != "" {
		if seguro, ok := f.(interface{ ForgetUpload(string, string) }); ok {
			seguro.ForgetUpload(canal, id)
		}
	}
}

func (sa *Sala) cancelarAdjuntos() {
	sa.adjuntosEpoch++
	if sa.P == nil || sa.P.Canal == nil || sa.P.Canal.Compositor == nil {
		return
	}
	c := sa.P.Canal.Compositor
	for _, p := range c.Pendientes {
		if p.cancelar != nil {
			p.cancelar()
		}
	}
	if seguro, ok := sa.Ficheros.(interface{ ForgetUploads(string) }); ok {
		seguro.ForgetUploads(sa.abierto)
	}
	c.PonerPendientes(nil)
}

func (sa *Sala) subir(ambito ambitoSubida, ruta string) {
	if !sa.adjuntoVigente(ambito) {
		return
	}
	c, canal := ambito.compositor, ambito.canal
	sa.locales++
	ctx, cancelar := context.WithCancel(sa.ctx)
	var subido string // Sólo se lee y escribe en el hilo de la interfaz.
	p := Pendiente{Local: "u-" + strconv.Itoa(sa.locales), Nombre: filepath.Base(ruta), cancelar: func() {
		cancelar()
		olvidarSubida(ambito.archivos, canal, subido)
	}}
	c.PonerPendientes(append(c.Pendientes, p))
	pedir(sa, func(context.Context) (api.File, error) {
		defer cancelar()
		if err := ctx.Err(); err != nil {
			return api.File{}, err
		}
		f, err := os.Open(ruta) //nolint:gosec // G304: la ruta la eligió la persona en el selector del sistema
		if err != nil {
			return api.File{}, err
		}
		defer func() { _ = f.Close() }()
		info, err := f.Stat()
		if err != nil {
			return api.File{}, err
		}
		tipo := mime.TypeByExtension(filepath.Ext(ruta))
		avance := ficheros.ConProgreso(f, info.Size(), func(x float64) {
			fyne.Do(func() {
				if sa.adjuntoVigente(ambito) {
					sa.avanzar(p.Local, x)
				}
			})
		})
		file, err := ambito.archivos.UploadToChannel(ctx, canal, p.Nombre, tipo, avance, info.Size())
		if err == nil && ctx.Err() != nil {
			olvidarSubida(ambito.archivos, canal, file.ID)
			return api.File{}, ctx.Err()
		}
		return file, err
	}, func(f api.File, err error) {
		i := slices.IndexFunc(c.Pendientes, func(x Pendiente) bool { return x.Local == p.Local })
		if i < 0 || !sa.adjuntoVigente(ambito) {
			olvidarSubida(ambito.archivos, canal, f.ID)
			return // la quitó mientras subía
		}
		ps := slices.Clone(c.Pendientes)
		if err != nil {
			ps[i].Fallido = true
		} else {
			subido = f.ID
			ps[i].Nombre = f.Name
			ps[i].Subido = &api.RoomsAttachment{FileID: f.ID, Name: f.Name, ContentType: f.ContentType, SizeBytes: f.SizeBytes}
		}
		c.PonerPendientes(ps)
	})
}

// avanzar pone el porcentaje de la ficha que sigue subiendo.
func (sa *Sala) avanzar(local string, x float64) {
	c := sa.P.Canal.Compositor
	i := slices.IndexFunc(c.Pendientes, func(p Pendiente) bool { return p.Local == local })
	if i < 0 || c.Pendientes[i].Subido != nil || c.Pendientes[i].Fallido {
		return
	}
	ps := slices.Clone(c.Pendientes)
	ps[i].Progreso = x
	c.PonerPendientes(ps)
}

// FichaPendiente es un adjunto de la caja: `border bg-bg rounded-md px-2 py-1.5 text-xs
// gap-2`, el icono de estado en su caja de 21, el nombre (`max-w-[10rem] truncate`), el
// estado y el aspa de quitar.
type FichaPendiente struct {
	widget.BaseWidget
	T         *i18n.Translator
	Pendiente Pendiente
	quitar    *ui.BotonIcono
	objs      []fyne.CanvasObject
}

// NuevaFichaPendiente crea la ficha.
func NuevaFichaPendiente(t *i18n.Translator, p Pendiente, alQuitar func()) *FichaPendiente {
	f := &FichaPendiente{T: t, Pendiente: p}
	f.quitar = ui.NuevoBotonIcono(iconos.Cerrar, t.T("action.removeNamed", i18n.Vars{"name": p.Nombre}), alQuitar)
	f.quitar.Lado, f.quitar.TamIcono, f.quitar.Trazo = 24, iconos.TallaXXS, "text-subtle"
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es su nombre.
func (f *FichaPendiente) AccessibilityLabel() string { return f.Pendiente.Nombre }

// AccessibilityRole: texto con un botón.
func (f *FichaPendiente) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (f *FichaPendiente) estado() string {
	switch {
	case f.Pendiente.Fallido:
		return f.T.T("state.error", nil)
	case f.Pendiente.Subido == nil:
		return strconv.Itoa(int(math.Round(f.Pendiente.Progreso*100))) + "%"
	}
	return ""
}

// pesoEstado es el del estado: `font-medium` el error y normal el porcentaje.
//
// ponytail: sin `tabular-nums`, que Fyne no pide a la fuente; el porcentaje ensancha la
// ficha cuando gana una cifra.
func (f *FichaPendiente) pesoEstado() ui.Peso {
	if f.Pendiente.Fallido {
		return ui.Medio
	}
	return ui.Normal
}

// MedidaFichaPendiente es `px-2`, la caja de 21, el nombre (10 rem como mucho), el
// estado y el aspa, con `gap-2`, por `py-1.5` y el aspa de 24.
func MedidaFichaPendiente(f *FichaPendiente) fyne.Size {
	ancho := 2*tema.Espacio(2) + 2*tema.Borde + tema.Espacio(6) + tema.Espacio(2) +
		min(ui.Medir(f.Pendiente.Nombre, tema.TextoXS, ui.Medio).Width, 10*tema.Rem) + tema.Espacio(2) + ui.SueloTactil(24)
	if e := f.estado(); e != "" {
		ancho += ui.Medir(e, tema.TextoXS, f.pesoEstado()).Width + tema.Espacio(2)
	}
	return fyne.NewSize(ancho, 2*tema.Espacio(1.5)+ui.SueloTactil(24)+2*tema.Borde)
}

// medidaEnFila deja que el nombre ceda al estrecharse el canal entre dos paneles.
// La misma medida se usa para envolver y colocar, sin reducir el botón Quitar.
func (f *FichaPendiente) medidaEnFila(ancho float32) fyne.Size {
	m := f.MinSize()
	m.Width = min(m.Width, max(ancho, 0))
	return m
}

func (f *FichaPendiente) CreateRenderer() fyne.WidgetRenderer { return &fichaPendienteRenderer{f: f} }

type fichaPendienteRenderer struct{ f *FichaPendiente }

func (r *fichaPendienteRenderer) MinSize() fyne.Size           { return MedidaFichaPendiente(r.f) }
func (r *fichaPendienteRenderer) Objects() []fyne.CanvasObject { return r.f.objs }
func (r *fichaPendienteRenderer) Destroy()                     {}
func (r *fichaPendienteRenderer) Refresh()                     { r.Layout(r.f.Size()) }
func (r *fichaPendienteRenderer) Layout(t fyne.Size) {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	caja := ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), tema.RadioMD)
	ui.MoverCaja(caja, fyne.Position{}, t)
	f.objs = caja.Objetos()
	x := tema.Borde + tema.Espacio(2)
	icono := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v), tema.RadioSM)
	ui.MoverCaja(icono, fyne.NewPos(x, (t.Height-tema.Espacio(6))/2), fyne.NewSize(tema.Espacio(6), tema.Espacio(6)))
	f.objs = append(f.objs, icono.Objetos()...)
	var g fyne.CanvasObject
	switch {
	case f.Pendiente.Fallido:
		g = iconos.Trazo(iconos.Aviso, iconos.TallaXXS, "danger-text", v).Objeto()
	case f.Pendiente.Subido == nil:
		g = ui.NuevaRueda(iconos.TallaXXS, "text-muted", v)
	default:
		g = dibujo(glifoClip, iconos.TallaXXS, "text-muted", v).Objeto()
	}
	g.Move(fyne.NewPos(x+(tema.Espacio(6)-iconos.TallaXXS)/2, (t.Height-iconos.TallaXXS)/2))
	g.Resize(fyne.NewSize(iconos.TallaXXS, iconos.TallaXXS))
	f.objs = append(f.objs, g)
	x += tema.Espacio(6) + tema.Espacio(2)
	ladoQuitar := ui.SueloTactil(24)
	quitarX := max(t.Width-tema.Borde-tema.Espacio(2)-ladoQuitar+tema.Espacio(1), 0)
	libre := max(quitarX-tema.Espacio(2)-x, 0)
	estado := f.estado()
	anchoEstado := min(ui.Medir(estado, tema.TextoXS, f.pesoEstado()).Width, libre)
	anchoNombre := max(libre-anchoEstado, 0)
	if estado != "" {
		anchoNombre = max(anchoNombre-tema.Espacio(2), 0)
	}
	nombre := ui.Texto("", tema.TextoXS, ui.Medio, tema.Color("text", v))
	x += ui.Truncar(nombre, f.Pendiente.Nombre, tema.TextoXS, ui.Medio, min(10*tema.Rem, anchoNombre)) + tema.Espacio(2)
	ui.ColocarTexto(nombre, tema.TextoXS, x-tema.Espacio(2)-ui.Medir(nombre.Text, tema.TextoXS, ui.Medio).Width, (t.Height-tema.TextoXS.Linea)/2)
	f.objs = append(f.objs, nombre)
	if estado != "" {
		token := "text-subtle"
		if f.Pendiente.Fallido {
			token = "danger-text"
		}
		tx := ui.Texto("", tema.TextoXS, f.pesoEstado(), tema.Color(token, v))
		ui.Truncar(tx, estado, tema.TextoXS, f.pesoEstado(), anchoEstado)
		ui.ColocarTexto(tx, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
		f.objs = append(f.objs, tx)
	}
	f.quitar.Move(fyne.NewPos(quitarX, (t.Height-ladoQuitar)/2))
	f.quitar.Resize(fyne.NewSize(ladoQuitar, ladoQuitar))
	f.objs = append(f.objs, f.quitar)
}

// Visor es el `Lightbox`: la imagen original a pantalla completa sobre el velo, con el
// nombre, «2 / 5», descargar y cerrar arriba, y las flechas si hay varias. Escape
// cierra y las flechas del teclado pasan.
//
// ponytail: el original llega entero antes de pintarse (sin carga progresiva), y
// mientras tanto se ve la miniatura si ya estaba.
func (sa *Sala) AbrirVisor(canal string, imgs []Adjunto, i int) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil || len(imgs) == 0 || sa.Ficheros == nil {
		return
	}
	t := sa.P.T
	var d *ui.Dialogo
	var pintar func()
	cuerpo := container.NewStack()
	pintar = func() {
		a := imgs[i]
		d.Titulo = a.Nombre
		if len(imgs) > 1 {
			d.Titulo = a.Nombre + " · " + strconv.Itoa(i+1) + " / " + strconv.Itoa(len(imgs))
		}
		cuerpo.Objects = []fyne.CanvasObject{container.NewCenter(ui.NuevaRueda(24, "text-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))}
		cuerpo.Refresh()
		d.Refresh()
		sa.Lanzar(func() {
			rc, err := sa.Ficheros.ChannelContent(sa.ctx, canal, a.ID, false)
			if err != nil {
				return
			}
			defer func() { _ = rc.Close() }()
			// Acotado al leer y al decodificar (FYC-02): la imagen la sube cualquier
			// miembro del canal, también de otra organización, y una bomba de 98 KB
			// declaraba 400 MP (1,6 GB) y tumbaba la app al ampliarla.
			datos, err := io.ReadAll(io.LimitReader(rc, maxImagenVisor))
			if err != nil {
				return
			}
			img, err := externo.Decodificar(datos)
			if err != nil {
				return
			}
			fyne.Do(func() {
				if imgs[i].ID != a.ID {
					return
				}
				im := canvas.NewImageFromImage(img)
				im.FillMode = canvas.ImageFillContain
				im.SetMinSize(fyne.NewSize(640, 440))
				cuerpo.Objects = []fyne.CanvasObject{im}
				cuerpo.Refresh()
				d.Recolocar()
			})
		})
	}
	mover := func(paso int) {
		if len(imgs) > 1 {
			i = (i + paso + len(imgs)) % len(imgs)
			pintar()
		}
	}
	anterior := ui.NuevoBoton(t.T("attachment.previousImage", nil), func() { mover(-1) }).Con(ui.Secundario, ui.TallaSM)
	siguiente := ui.NuevoBoton(t.T("attachment.nextImage", nil), func() { mover(1) }).Con(ui.Secundario, ui.TallaSM)
	descargar := ui.NuevoBoton(t.T("action.download", nil), func() { sa.Descargar(canal, imgs[i]) }).Con(ui.Primario, ui.TallaSM)
	pie := []fyne.CanvasObject{descargar}
	if len(imgs) > 1 {
		pie = []fyne.CanvasObject{anterior, siguiente, descargar}
	}
	d = ui.NuevoDialogo(imgs[i].Nombre, cuerpo).Con(ui.DialogoLG, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, pie...))
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	d.Mostrar(lienzo)
	pintar()
}
