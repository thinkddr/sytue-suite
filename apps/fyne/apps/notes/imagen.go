// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/externo"
)

// Paridad web: la nota de IMAGEN —el `case 'image'` de NoteCard.tsx—: la imagen de su
// `data.url`, `max-h-64 w-full rounded-md border object-contain`. La baja
// `apps/nucleo/externo`, con sus topes, una vez por dirección y en su rutina: un cuerpo
// que pide a la red al pintarse es un cuerpo que se cuelga al pintarse.

// imagenRemota es cómo va la de una dirección: cargando, lista o fallida.
type imagenRemota struct {
	img   image.Image
	err   error
	lista bool
}

// AltoMaxImagen es el `max-h-64`.
const AltoMaxImagen = 64 * 0.25 * tema.Rem

// imagenDe es la de esa dirección tal como vaya. La primera vez la pide.
func (p *Pantalla) imagenDe(url string) imagenRemota {
	if r, ok := p.imagenes[url]; ok {
		return r
	}
	if p.imagenes == nil {
		p.imagenes = map[string]imagenRemota{}
	}
	p.imagenes[url] = imagenRemota{}
	go p.cargarImagen(url)
	return imagenRemota{}
}

// cargarImagen la baja y, al tenerla, repinta.
func (p *Pantalla) cargarImagen(url string) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	img, err := externo.Imagen(ctx, url)
	enHilo(func() {
		if p.imagenes == nil {
			p.imagenes = map[string]imagenRemota{}
		}
		p.imagenes[url] = imagenRemota{img: img, err: err, lista: true}
		p.Repintar()
	})
}

// cuerpoImagen es el `<img>`: el marco a lo ancho con la imagen contenida. Mientras
// llega, el marco sin contenido (los 2 px de su borde, como la web); si no llega, su
// `alt` —el título de la nota— con el icono de imagen rota delante, como el navegador.
func (t *Tarjeta) cuerpoImagen(v tema.Variante) fyne.CanvasObject {
	n := t.nota
	if !esWeb(n.URL) || t.Imagen == nil {
		return container.New(&altoFijo{})
	}
	r := t.Imagen(n.URL)
	if r.lista && r.err != nil {
		return container.New(altRoto{}, Glifo(ImagenD, LadoImagenRota, "text-subtle", v),
			ui.NuevoParrafo(n.Titulo, tema.TextoBase, ui.Normal, "text-muted", v))
	}
	marco := canvas.NewRectangle(nil)
	marco.StrokeColor, marco.StrokeWidth, marco.CornerRadius = tema.Color("border", v), tema.Borde, tema.RadioMD
	m := &marcoImagen{}
	if r.img == nil {
		return container.New(m, marco)
	}
	m.w, m.h = r.img.Bounds().Dx(), r.img.Bounds().Dy()
	img := canvas.NewImageFromImage(r.img)
	img.FillMode, img.CornerRadius = canvas.ImageFillContain, tema.RadioMD
	img.ScaleMode = canvas.ImageScaleSmooth
	return container.New(m, marco, img)
}

// marcoImagen coloca el borde y la imagen: `w-full` y el alto de la proporción de la
// imagen, hasta el `max-h-64`, borde incluido (`box-sizing: border-box`).
type marcoImagen struct{ w, h int }

func (m *marcoImagen) AltoPara(_ []fyne.CanvasObject, ancho float32) float32 {
	b := 2 * float32(tema.Borde)
	if m.w <= 0 || ancho <= 0 {
		return b
	}
	return min((ancho-b)*float32(m.h)/float32(m.w)+b, AltoMaxImagen)
}

func (m *marcoImagen) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, m.AltoPara(objs, 0))
}

func (m *marcoImagen) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	if len(objs) > 1 {
		objs[1].Move(fyne.NewPos(tema.Borde, tema.Borde))
		objs[1].Resize(fyne.NewSize(t.Width-2*tema.Borde, t.Height-2*tema.Borde))
	}
}

// LadoImagenRota es el icono de imagen rota de Chromium: 16 × 16.
//
// ponytail: el de Chromium es un mapa de bits suyo (una foto con la esquina rota); aquí
// va el `image` de la casa en `text-subtle`, que se lee igual y sigue el tema.
const LadoImagenRota = 16

// altRoto coloca el icono y el `alt`: el icono apoyado en la línea base de la primera
// línea, como una imagen en línea, y el texto partiendo en lo que queda de ancho.
type altRoto struct{}

func (altRoto) AltoPara(objs []fyne.CanvasObject, ancho float32) float32 {
	parrafo, ok := objs[1].(*ui.Parrafo)
	if !ok {
		return LadoImagenRota
	}
	return max(LadoImagenRota, ui.AltoParrafo(parrafo, max(ancho-LadoImagenRota, 1)))
}

func (a altRoto) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(LadoImagenRota, max(LadoImagenRota, objs[1].MinSize().Height))
}

func (a altRoto) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	base := ui.LineaBase(tema.TextoBase, ui.Normal)
	objs[0].Move(fyne.NewPos(0, max(base-LadoImagenRota, 0)))
	objs[0].Resize(fyne.NewSize(LadoImagenRota, LadoImagenRota))
	objs[1].Move(fyne.NewPos(LadoImagenRota, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-LadoImagenRota, 0), a.AltoPara(objs, t.Width)))
}
