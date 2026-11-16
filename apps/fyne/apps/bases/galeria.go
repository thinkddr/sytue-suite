// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"image"
	"image/color"
	_ "image/jpeg" // las miniaturas que sirve el servicio son JPEG
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La rejilla de la galería (Galeria.tsx): `grid-cols-[repeat(auto-fill,minmax(14rem,1fr))]
// gap-4`, y dentro de cada tarjeta el `p-3` bajo la portada.
var (
	rejillaGaleria = ui.Rejilla{Min: 14 * tema.Rem, Hueco: tema.Espacio(4)}
	margenGaleria  = tema.Espacio(3)
)

// portadas guarda las miniaturas ya pedidas de una base, por id de adjunto. Una que no
// llegó se queda como nil: la tarjeta pinta su hueco y no se vuelve a pedir en bucle.
type portadas struct {
	imagenes map[string]image.Image
	pidiendo map[string]bool
	// recortadas son las portadas ya recortadas a la tarjeta (ui.RecorteRedondeadoArriba),
	// por adjunto y ancho: recortar en cada repintado costaría lo que la imagen entera.
	recortadas map[portadaRecortada]image.Image
}

// portadaRecortada es la clave de una portada recortada.
type portadaRecortada struct {
	adjunto string
	ancho   float32
}

// portadaDe es la portada de una tarjeta: el primer adjunto de imagen del registro,
// recorriendo los campos de adjuntos en el orden de la tabla. Solo vale una imagen con
// miniatura (la que el servicio sacó del CONTENIDO): la tarjeta nunca baja el original.
// Espejo de `portadaDe` de la web.
func portadaDe(campos []api.BasesField, r api.BasesRecord) (api.BasesAttachment, bool) {
	for _, c := range campos {
		if c.Kind != api.BasesKindAttachment {
			continue
		}
		for _, a := range r.Attachments[c.ID] {
			if a.HasThumb && strings.HasPrefix(a.Mime, "image/") {
				return a, true
			}
		}
	}
	return api.BasesAttachment{}, false
}

// objetoGaleria es Galeria.tsx: una tarjeta por registro con su portada, su nombre y
// dos campos más. La tarjeta entera abre la ficha, que es donde se edita.
func (p *Pantalla) objetoGaleria(v tema.Variante, puedeEditar bool) fyne.CanvasObject {
	b := p.abierta
	hijos := []fyne.CanvasObject{}
	if !puedeEditar {
		hijos = append(hijos, ui.NuevoRotulo(p.t("grid.readOnly"), tema.TextoXS, ui.Normal, "text-muted"))
	}
	registros := b.vent.registros()
	if len(registros) == 0 {
		hijos = append(hijos, ui.NuevoRotulo(p.t("grid.empty"), tema.TextoSM, ui.Normal, "text-muted"))
		return container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...)
	}
	// Como en el tablero, sin enlaces ni casillas, y sin los adjuntos, que ya son la portada.
	otros := make([]api.BasesField, 0, len(b.Campos))
	for _, c := range b.Campos {
		if c.Kind != api.BasesKindLink && c.Kind != api.BasesKindCheckbox && c.Kind != api.BasesKindAttachment {
			otros = append(otros, c)
		}
	}
	ancho := rejillaGaleria.AnchoColumna(p.anchoContenido())
	tarjetas := make([]fyne.CanvasObject, len(registros))
	for i, r := range registros {
		tarjetas[i] = p.tarjetaDeGaleria(v, r, otros, ancho)
	}
	hijos = append(hijos, container.New(rejillaGaleria, tarjetas...))
	pie := []fyne.CanvasObject{}
	if b.vent.primera > 0 {
		pie = append(pie, botonSM(p.t("grid.loadPrevious"), ui.Fantasma, func() { p.cargarRegistros("antes") }))
	}
	if b.vent.siguiente != "" {
		pie = append(pie, botonSM(p.t("grid.loadMore"), ui.Fantasma, func() { p.cargarRegistros("mas") }))
	}
	if len(pie) > 0 {
		hijos = append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(2)}, pie...))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...)
}

// tarjetaDeGaleria es `<li class="rounded-lg border bg-surface overflow-hidden">` con el
// botón que la llena: la portada a sangre y, debajo, el nombre y dos campos con valor.
func (p *Pantalla) tarjetaDeGaleria(v tema.Variante, r api.BasesRecord, otros []api.BasesField, ancho float32) fyne.CanvasObject {
	b := p.abierta
	nombre := etiquetaDeRegistro(b.Campos, r)
	interior := ancho - 2*margenGaleria
	textos := []fyne.CanvasObject{ui.NuevoParrafo(nombre, tema.TextoSM, ui.SemiNegrita, "text", v).A(interior)}
	// Como la web (`.slice(1, 3)`): el primero con valor suele ser el propio nombre.
	conValor := 0
	for _, c := range otros {
		texto := textoVisible(c, r.Data[c.ID], p.tr.Locale())
		if texto == "" {
			continue
		}
		conValor++
		if conValor == 1 {
			continue
		}
		if conValor > 3 {
			break
		}
		textos = append(textos, ui.NuevoParrafo(c.Name+": "+texto, tema.TextoXS, ui.Normal, "text-muted", v).A(interior).Recortado(1))
	}
	cuerpo := container.New(ui.Columna{},
		p.portadaDeTarjeta(v, r, ancho),
		container.New(ui.Margen{Izq: margenGaleria, Arr: margenGaleria, Der: margenGaleria, Abj: margenGaleria},
			container.New(ui.Columna{Hueco: tema.Espacio(1)}, textos...)))
	boton := ui.NuevaFilaPulsable(p.t("board.open", i18n.Vars{"name": nombre}), cuerpo, func() { p.abrirRegistro(r, false) })
	boton.SinRelleno, boton.Radio = true, tema.RadioLG
	fondo := canvas.NewRectangle(tema.Color("surface", v))
	fondo.CornerRadius = tema.RadioLG
	// El borde va ENCIMA: el `hover` del botón no lo tapa.
	borde := canvas.NewRectangle(color.NRGBA{})
	borde.CornerRadius, borde.StrokeWidth, borde.StrokeColor = tema.RadioLG, tema.Borde, tema.Color("border", v)
	return container.NewStack(fondo, boton, borde)
}

// portadaDeTarjeta es la miniatura a `aspect-[4/3] object-cover`, o el hueco
// `bg-bg-subtle` con el icono de imagen mientras llega o si no hay ninguna.
func (p *Pantalla) portadaDeTarjeta(v tema.Variante, r api.BasesRecord, ancho float32) fyne.CanvasObject {
	tam := fyne.NewSize(ancho, ancho*3/4)
	if a, hay := portadaDe(p.abierta.Campos, r); hay {
		if img := p.miniatura(r.ID, a.ID); img != nil {
			// La tarjeta es `rounded-lg overflow-hidden`: la portada lleva las esquinas de
			// arriba redondeadas como ella, o asomaría por fuera del borde.
			clave := portadaRecortada{a.ID, ancho}
			rec, ya := p.abierta.portadas.recortadas[clave]
			if !ya {
				if p.abierta.portadas.recortadas == nil {
					p.abierta.portadas.recortadas = map[portadaRecortada]image.Image{}
				}
				rec = ui.RecorteRedondeadoArriba(img, tam.Width, tam.Height, tema.RadioLG)
				p.abierta.portadas.recortadas[clave] = rec
			}
			im := canvas.NewImageFromImage(rec)
			im.FillMode = canvas.ImageFillCover // `object-cover`
			im.ScaleMode = canvas.ImageScaleSmooth
			return container.New(tamFijo{tam}, im)
		}
	}
	hueco := canvas.NewRectangle(tema.Color("bg-subtle", v))
	hueco.TopLeftCornerRadius, hueco.TopRightCornerRadius = tema.RadioLG, tema.RadioLG
	icono := iconos.Trazo(iconos.Imagen, 32, "text-muted", v).Objeto()
	return container.New(tamFijo{tam}, container.NewStack(hueco, container.NewCenter(icono)))
}

// miniatura es la imagen ya bajada de ese adjunto, o nil. La primera vez la pide en
// segundo plano y repinta al llegar.
func (p *Pantalla) miniatura(registro, adjunto string) image.Image {
	b := p.abierta
	if b.portadas.imagenes == nil {
		b.portadas = portadas{imagenes: map[string]image.Image{}, pidiendo: map[string]bool{}}
	}
	if img, ya := b.portadas.imagenes[adjunto]; ya {
		return img
	}
	if b.portadas.pidiendo[adjunto] || p.svc == nil {
		return nil
	}
	b.portadas.pidiendo[adjunto] = true
	// Con un `fondo` que no es de verdad en segundo plano (las pruebas), la miniatura
	// llega antes de volver: entonces se devuelve y no se repinta a mitad de pintar.
	sincrono := true
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var img image.Image
		if cuerpo, err := p.svc.DownloadThumb(ctx, registro, adjunto); err == nil {
			img, _, _ = image.Decode(cuerpo)
			_ = cuerpo.Close()
		}
		enHilo(func() {
			delete(b.portadas.pidiendo, adjunto)
			b.portadas.imagenes[adjunto] = img
			if !sincrono && p.abierta == b && img != nil {
				p.Repintar()
			}
		})
	})
	sincrono = false
	return b.portadas.imagenes[adjunto]
}

// tamFijo pone a su hijo a esa medida exacta.
type tamFijo struct{ Tam fyne.Size }

func (f tamFijo) MinSize([]fyne.CanvasObject) fyne.Size { return f.Tam }

func (f tamFijo) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(f.Tam)
	}
}
