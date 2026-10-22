// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

/*
El panel de PÁGINAS (`Miniaturas` de `Paneles.tsx`): una fila por página con su dibujo en
pequeño, el número y el extracto, que es lo que permite RECONOCER una página sin verla.

	<button class="flex w-full gap-2 rounded-md p-1.5 text-left
	               {actual ? 'bg-selected ring-border-strong ring-1' : 'hover:bg-bg-subtle'}">
	  <span class="border-border block w-16 shrink-0 border bg-white" style="aspect-ratio">
	  <span class="min-w-0 flex-1">
	    <span class="text-text block text-xs font-medium tabular-nums">{n}</span>
	    <span class="text-text-subtle text-2xs mt-0.5 line-clamp-3 block">{extracto}</span>

El dibujo de la miniatura es el MISMO de la hoja, remuestreado: la web también reutiliza
el SVG ya pedido, y pedir otro más pequeño sería volver a subir el documento.
*/

// Las medidas de una fila de miniatura, con raíz de 16 px.
var (
	rellenoMiniatura = tema.Espacio(1.5)
	anchoMiniatura   = tema.Espacio(16)
	huecoMiniatura   = tema.Espacio(2)
)

// paginas es la lista `flex flex-col gap-2` de miniaturas.
func (p *PanelLateral) paginas() fyne.CanvasObject {
	p.miniaturas = make(map[int]*filaMiniatura, len(p.c.Paginas))
	filas := make([]fyne.CanvasObject, 0, len(p.c.Paginas))
	for _, pag := range p.c.Paginas {
		n := pag.Numero
		// Solo el de las páginas que el motor llegó a leer: de las demás no se sabe, y
		// «sin texto» sería mentir.
		texto := ""
		if pag.Extracto != nil {
			var ok bool
			if texto, ok = ExtractoLegible(*pag.Extracto); !ok {
				texto = p.t("pdf.page.noText", nil)
			}
		}
		f := nuevaFilaMiniatura(pag, texto, anchoUtil(), func() image.Image {
			if p.Imagen == nil {
				return nil
			}
			return p.Imagen(n)
		})
		f.actual = n == p.PaginaActual
		f.AlPulsar = func() {
			if p.AlIrA != nil {
				p.AlIrA(n)
			}
		}
		p.miniaturas[n] = f
		filas = append(filas, f)
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...)
}

// ExtractoLegible es `extractoLegible` de `extracto.ts`: si menos de dos de cada tres
// caracteres son letras, cifras o espacios, la capa de texto está rota —las fuentes con
// la tabla de caracteres mal hecha dicen `,035(62 '( 0$75Ê&8/$`— y enseñarla es peor que
// no enseñar nada.
func ExtractoLegible(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if utf8.RuneCountInString(t) < 4 {
		return "", false
	}
	var buenos, total int
	for _, c := range t {
		total++
		if unicode.IsLetter(c) || unicode.IsNumber(c) || unicode.IsSpace(c) {
			buenos++
		}
	}
	return t, 3*buenos >= 2*total
}

// filaMiniatura es un botón de la lista.
type filaMiniatura struct {
	ui.Pulsable
	pagina  Pagina
	texto   string
	ancho   float32
	actual  bool
	imagen  func() image.Image
	extract *ui.Parrafo
}

func nuevaFilaMiniatura(pag Pagina, texto string, ancho float32, imagen func() image.Image) *filaMiniatura {
	f := &filaMiniatura{pagina: pag, texto: texto, ancho: ancho, imagen: imagen}
	f.Nombre = numeroDePagina(pag.Numero)
	f.ExtendBaseWidget(f)
	return f
}

// altoMiniatura es el alto de la miniatura por su `aspect-ratio`.
func altoMiniatura(p Pagina) float32 {
	if p.Ancho <= 0 {
		return anchoMiniatura
	}
	return anchoMiniatura * float32(p.Alto/p.Ancho)
}

// anchoTextoMiniatura es lo que le queda a la columna del texto.
func (f *filaMiniatura) anchoTexto() float32 {
	return max(f.ancho-2*rellenoMiniatura-anchoMiniatura-huecoMiniatura, 0)
}

// MedidaFilaMiniatura es la ÚNICA cuenta: el `p-1.5` alrededor de lo más alto entre la
// miniatura y la columna del número y el extracto.
func MedidaFilaMiniatura(f *filaMiniatura) fyne.Size {
	extracto := ui.AltoParrafo(ui.NuevoParrafo(f.texto, tema.Texto2XS, ui.Normal, "text-subtle", tema.Claro).Recortado(3), f.anchoTexto())
	texto := tema.TextoXS.Linea + tema.Espacio(0.5) + extracto
	return fyne.NewSize(f.ancho, max(altoMiniatura(f.pagina), texto)+2*rellenoMiniatura)
}

func (f *filaMiniatura) MinSize() fyne.Size { f.ExtendBaseWidget(f); return MedidaFilaMiniatura(f) }

func (f *filaMiniatura) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaMiniaturaRenderer{
		f:      f,
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v).Abierto(),
		fondo:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{A: 1}, tema.RadioMD+tema.Borde),
		papel:  ui.NuevaCaja(ColorPapel, tema.Color("border", v), 0),
		numero: nuevoTabular(numeroDePagina(f.pagina.Numero), tema.TextoXS, ui.Medio, tema.Color("text", v)),
	}
	r.dibujo = canvas.NewRaster(r.generar)
	r.dibujo.ScaleMode = canvas.ImageScalePixels
	f.extract = ui.NuevoParrafo(f.texto, tema.Texto2XS, ui.Normal, "text-subtle", v).Recortado(3).A(f.anchoTexto())
	r.Refresh()
	return r
}

type filaMiniaturaRenderer struct {
	f      *filaMiniatura
	aro    *ui.Aro
	fondo  *ui.Caja
	papel  *ui.Caja
	dibujo *canvas.Raster
	numero *tabular

	hecha image.Image
}

// generar remuestrea el dibujo de la hoja a la miniatura, por dentro del borde.
func (r *filaMiniaturaRenderer) generar(w, h int) image.Image {
	img := r.f.imagen()
	if img == nil || w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	}
	if r.hecha != nil && r.hecha.Bounds().Dx() == w && r.hecha.Bounds().Dy() == h {
		return r.hecha
	}
	r.hecha = RecortarPagina(img, float64(anchoMiniatura), float64(altoMiniatura(r.f.pagina)), w, h)
	return r.hecha
}

func (r *filaMiniaturaRenderer) MinSize() fyne.Size { return MedidaFilaMiniatura(r.f) }
func (r *filaMiniaturaRenderer) Destroy()           {}

func (r *filaMiniaturaRenderer) Objects() []fyne.CanvasObject {
	objs := append([]fyne.CanvasObject{}, r.fondo.Objetos()...)
	objs = append(objs, r.papel.Objetos()...)
	objs = append(append(objs, r.dibujo), r.numero.Objetos()...)
	return r.aro.Antes(append(objs, r.f.extract)...)
}

func (r *filaMiniaturaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	// El `ring-1` va POR FUERA de la caja: la caja del fondo se agranda un píxel por
	// lado y su borde es el anillo.
	ui.MoverCaja(r.fondo, fyne.NewPos(-tema.Borde, -tema.Borde), fyne.NewSize(t.Width+2*tema.Borde, t.Height+2*tema.Borde))
	alto := altoMiniatura(r.f.pagina)
	ui.MoverCaja(r.papel, fyne.NewPos(rellenoMiniatura, rellenoMiniatura), fyne.NewSize(anchoMiniatura, alto))
	r.dibujo.Move(fyne.NewPos(rellenoMiniatura+tema.Borde, rellenoMiniatura+tema.Borde))
	r.dibujo.Resize(fyne.NewSize(anchoMiniatura-2*tema.Borde, max(alto-2*tema.Borde, 0)))
	x := rellenoMiniatura + anchoMiniatura + huecoMiniatura
	r.numero.Colocar(x, rellenoMiniatura)
	r.f.extract.Move(fyne.NewPos(x, rellenoMiniatura+tema.TextoXS.Linea+tema.Espacio(0.5)))
	r.f.extract.Resize(fyne.NewSize(r.f.anchoTexto(), r.f.extract.MinSize().Height))
}

func (r *filaMiniaturaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, anillo := color.NRGBA{}, color.NRGBA{}
	switch {
	case r.f.actual:
		// `bg-selected` es TRANSLÚCIDO (el azul al 10 %) y la caja pinta su relleno
		// ENCIMA del anillo: sin aplanarlo sobre la superficie del panel, el interior
		// salía del color del anillo. La fila va siempre sobre `surface`, que es lo que
		// hay detrás en la web.
		sup := tema.Color("surface", v)
		fondo = tokens.SobreSRGB(tema.Color("selected", v), sup)
		anillo = tokens.SobreSRGB(tema.Color("border-strong", v), sup)
	case r.f.Encima():
		fondo = tema.Color("bg-subtle", v)
		anillo = fondo
	}
	r.fondo.Pintar(fondo, anillo)
	r.papel.Pintar(ColorPapel, tema.Color("border", v))
	r.numero.Tenir(tema.Color("text", v))
	r.hecha = nil
	r.dibujo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.Layout(r.f.Size())
}
