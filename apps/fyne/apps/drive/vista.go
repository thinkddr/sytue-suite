// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	// El AVIF se descodifica con libavif y dav1d compilados a WASM, sin CGO: el repo no
	// tenía nada que leyera AV1 y un descodificador propio de AV1 no es razonable.
	_ "github.com/gen2brain/avif"
	// El WebP no lo abre SyPhotos (sin miniatura en el servicio) y sí la vista previa.
	_ "golang.org/x/image/webp"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/externo"
)

// # La vista previa
//
// `PreviewDialog` de FileBrowser.tsx, pero solo para lo que NO tiene app en la suite:
// un PDF abre CertiPDF, una foto SyPhotos y un documento Docus/Calcus/Diapos
// (CONVENTIONS.md §13.6). Lo que queda es el texto —el `<pre>` de la web, para
// `TEXT_EXTS` y hasta 1 MB— y las imágenes que SyPhotos no sabe leer (WebP, AVIF). El
// SVG no tiene vista previa nativa desde FYS-07: se descarga.
//
// Un AVIF, con `gen2brain/avif` (libavif + dav1d en WASM, sin CGO), que se registra en
// `image.Decode`: si el sistema tiene libavif la usa, y si no, la suya.

// extensionesTexto es el `TEXT_EXTS` de la web.
var extensionesTexto = map[string]bool{
	"txt": true, "md": true, "csv": true, "json": true, "log": true, "xml": true, "yaml": true,
	"yml": true, "ini": true, "toml": true, "js": true, "ts": true, "css": true, "html": true,
	"htm": true, "sh": true, "go": true, "py": true, "sql": true,
}

// TopeVistaImagen es lo que se lee de una imagen para enseñarla. La web no tiene tope
// (es un `<img>`); este solo evita llenar la memoria con un fichero que dice ser una foto.
const TopeVistaImagen = 64 << 20

// TopeVistaTexto es el MB a partir del cual un texto se descarga en vez de enseñarse: un
// `<pre>` enorme congela la pestaña, y aquí un lienzo con cien mil líneas también.
const TopeVistaTexto = 1_000_000

// TipoVista es qué visor le toca a un fichero sin app: "texto", "imagen" o "".
func TipoVista(el Elemento) string {
	if el.Carpeta {
		return ""
	}
	ext := Extension(el.Nombre)
	switch {
	case extensionesTexto[ext] && el.Tamano <= TopeVistaTexto:
		return "texto"
	// El SVG ya no tiene vista previa nativa (FYS-07, decidido por el dueño): un SVG de
	// 90 bytes desbordaba la pila de oksvg y cerraba Drive. Se descarga, como lo que no
	// tiene visor.
	case ext == "webp" || ext == "avif":
		return "imagen"
	}
	return ""
}

// pedirVista abre la vista previa de un fichero de «Mi unidad».
func (p *Pantalla) pedirVista(el Elemento) {
	if p.svc == nil {
		return
	}
	p.abrirVista(el, func(ctx context.Context) (io.ReadCloser, error) { return p.svc.Download(ctx, el.ID) },
		func() { go p.descargar(el) })
}

// abrirVista abre la vista previa y trae el contenido con `abrir`; `bajar` es el botón
// «Descargar» del pie. Sirve igual para «Mi unidad» y para una unidad de equipo, que
// descargan por rutas distintas.
func (p *Pantalla) abrirVista(el Elemento, abrir func(context.Context) (io.ReadCloser, error), bajar func()) {
	if p.win == nil {
		return
	}
	v := p.variante()
	cuerpo := container.NewStack(container.New(&altoFijo{alto: AltoCargaVersiones},
		container.NewCenter(ui.NuevaRueda(20, "text", v))))
	var d *ui.Dialogo
	descargar := conGlifo(ui.NuevoBoton(p.tr.T("action.download", nil), bajar).Con(ui.Secundario, ui.TallaMD), glifoDescargar, v)
	// Secundario: el acento es para la acción principal, y cerrar no lo es.
	cerrar := ui.NuevoBoton(p.tr.T("preview.close", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(el.Nombre, cuerpo).Con(ui.DialogoLG, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, descargar, cerrar))
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.win.Canvas())
	alto := p.win.Canvas().Size().Height * 0.65
	p.enSegundoPlano(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var datos []byte
		r, err := abrir(ctx)
		if err == nil {
			tope := int64(TopeVistaTexto + 1)
			if TipoVista(el) == "imagen" {
				tope = TopeVistaImagen
			}
			datos, err = io.ReadAll(io.LimitReader(r, tope))
			_ = r.Close()
		}
		enHilo(func() {
			cuerpo.Objects = []fyne.CanvasObject{p.contenidoVista(el, datos, err, alto, v)}
			cuerpo.Refresh()
			d.Recolocar()
		})
	})
}

// contenidoVista es lo que va dentro: la imagen, el texto, o «no se pudo».
func (p *Pantalla) contenidoVista(el Elemento, datos []byte, err error, alto float32, v tema.Variante) fyne.CanvasObject {
	fallo := func() fyne.CanvasObject {
		t := lineaTexto(ui.Texto(p.tr.T("preview.failed", nil), tema.TextoBase, ui.Normal, tema.Color("text-muted", v)), tema.TextoBase)
		return container.New(&altoFijo{alto: AltoCargaVersiones}, container.NewCenter(t))
	}
	if err != nil {
		return fallo()
	}
	if TipoVista(el) == "imagen" {
		c, proporcion, ok := ImagenVista(el.Nombre, datos)
		if !ok {
			return fallo()
		}
		// `mx-auto max-h-[65dvh] max-w-full object-contain`.
		c.FillMode = canvas.ImageFillContain
		return container.New(&imagenVista{proporcion: proporcion, tope: alto}, c)
	}
	if !utf8.Valid(datos) {
		datos = bytes.ToValidUTF8(datos, []byte("�"))
	}
	return nuevoTextoVista(string(datos), alto, v)
}

// ImagenVista prepara la imagen de la vista previa y su proporción (alto/ancho), con el
// tope de píxeles común: un WebP o un AVIF de pocos KB podía declarar cientos de
// megapíxeles (FYS-07). Un SVG no tiene vista previa.
func ImagenVista(nombre string, datos []byte) (*canvas.Image, float32, bool) {
	if Extension(nombre) == "svg" {
		return nil, 0, false
	}
	img, err := externo.Decodificar(datos)
	if err != nil {
		return nil, 0, false
	}
	b := img.Bounds()
	return canvas.NewImageFromImage(img), float32(b.Dy()) / float32(max(b.Dx(), 1)), true
}

// imagenVista coloca la imagen a todo el ancho y con su proporción, hasta el tope de alto.
type imagenVista struct{ proporcion, tope float32 }

func (i *imagenVista) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, min(i.tope, 40*tema.Rem*i.proporcion))
}

func (i *imagenVista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// nuevoTextoVista es el `<pre>`: `border rounded-md bg-surface-sunken p-4 font-mono
// text-sm whitespace-pre-wrap break-words`, con su propio desplazamiento hasta el 65 %
// del alto.
func nuevoTextoVista(texto string, tope float32, v tema.Variante) fyne.CanvasObject {
	t := &textoVista{lineas: strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n"), tinta: tema.Color("text", v)}
	caja := ui.NuevaCaja(tema.Color("surface-sunken", v), tema.Color("border", v), tema.RadioMD)
	t.cont = container.New(t)
	desplazable := container.NewVScroll(t.cont)
	return container.New(&conTope{tope: tope, alto: t.alto}, append(caja.Objetos(), desplazable)...)
}

// textoVista pinta las líneas partidas al ancho, con la letra monoespaciada.
type textoVista struct {
	lineas []string
	tinta  color.NRGBA
	ancho  float32
	textos []*canvas.Text
	cont   *fyne.Container
}

func (t *textoVista) estilo() fyne.TextStyle { return fyne.TextStyle{Monospace: true} }

// partir reparte una línea en trozos que caben (`break-words`). La letra es de ancho
// fijo, así que basta con cuántas caben: una medida, no una por letra.
func (t *textoVista) partir(ancho float32) []string {
	avance := fyne.MeasureText("M", tema.TextoSM.Tam, t.estilo()).Width
	caben := max(int(ancho/max(avance, 1)), 1)
	var out []string
	for _, l := range t.lineas {
		r := []rune(l)
		if len(r) == 0 {
			out = append(out, "")
			continue
		}
		for len(r) > 0 {
			n := min(caben, len(r))
			out = append(out, string(r[:n]))
			r = r[n:]
		}
	}
	return out
}

func (t *textoVista) alto(ancho float32) float32 {
	return float32(len(t.partir(max(ancho, 1)))) * tema.TextoSM.Linea
}

func (t *textoVista) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if t.ancho <= 0 {
		return fyne.NewSize(0, float32(len(t.lineas))*tema.TextoSM.Linea)
	}
	return fyne.NewSize(0, float32(len(t.textos))*tema.TextoSM.Linea)
}

// Layout rehace las líneas solo cuando cambia el ancho: partir mide letra a letra.
//
// ponytail: un `canvas.Text` por línea. Hasta el tope de 1 MB son decenas de miles; si
// se nota, se pinta solo lo que se ve con una lista virtual.
func (t *textoVista) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	if s.Width != t.ancho {
		t.ancho = s.Width
		t.textos = nil
		objetos := make([]fyne.CanvasObject, 0, len(t.lineas))
		for _, l := range t.partir(s.Width) {
			c := canvas.NewText(l, t.tinta)
			c.TextSize, c.TextStyle = tema.TextoSM.Tam, t.estilo()
			t.textos = append(t.textos, c)
			objetos = append(objetos, c)
		}
		t.cont.Objects = objetos
	}
	for i, c := range t.textos {
		ui.ColocarTexto(c, tema.TextoSM, 0, float32(i)*tema.TextoSM.Linea)
	}
}

// conTope pone la caja a tamaño completo y el desplazable dentro con el `p-4`, y mide
// lo que mide el texto hasta el tope de alto (`max-h-[65dvh]`).
type conTope struct {
	tope  float32
	alto  func(float32) float32
	ancho float32
}

func (c *conTope) MinSize([]fyne.CanvasObject) fyne.Size {
	p := tema.Espacio(4)
	ancho := c.ancho
	if ancho <= 0 {
		ancho = 40 * tema.Rem
	}
	return fyne.NewSize(0, min(c.tope, c.alto(ancho-2*p)+2*p))
}

func (c *conTope) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.ancho = t.Width
	p := tema.Espacio(4)
	for i, o := range objs {
		if i == len(objs)-1 {
			o.Move(fyne.NewPos(p, p))
			o.Resize(fyne.NewSize(max(t.Width-2*p, 0), max(t.Height-2*p, 0)))
			continue
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}
