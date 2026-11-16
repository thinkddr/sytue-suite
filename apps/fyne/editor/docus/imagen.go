// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La IMAGEN SELECCIONADA (`canvas/ImageOverlay.tsx`, `canvas/imageDom.ts` y
// `ImagenSeleccionadaGroup` de Insertar.tsx): un clic en una imagen la selecciona con su
// marco y sus cuatro tiradores —arrastrarlos cambia el tamaño sin deformarla, y arrastrar
// una anclada detrás o delante del texto la mueve— y la cinta enseña sus herramientas:
// ajuste del texto, alineación, texto alternativo, quitar el recorte y eliminar.
//
// Lo que la barra flotante de la web tiene y la cinta no —el ancho y el alto exactos y el
// recorte por lados en %— va en «Herramientas de imagen», el `aria-label` de esa barra:
// un emergente de la cinta, como el texto alternativo, para que reconstruir la cinta tras
// cada cifra no le quite el foco al campo en el que se escribe.

// imagenMaq es una imagen del folio con lo que se ve de ella, en px de maqueta.
type imagenMaq struct {
	q          punto // el bloque y la runa que ocupa
	img        *rico.Imagen
	x, y, w, h float32
	capa       int // 0 detrás del texto, 1 en la corriente, 2 delante
}

// imagenes son las del folio, en el orden en que se pintan.
func (f *Folio) imagenes() []imagenMaq {
	var out []imagenMaq
	mg := f.pagina.margenes()
	for i, c := range f.cajas {
		for _, l := range c.lineas {
			top := c.y + c.arribaTexto + l.y
			for _, p := range l.piezas {
				switch {
				case p.img == nil:
				case p.img.Absoluta():
					w, h := visibleImagen(p.img, 0)
					capa := 0
					if p.img.Ajuste == "front" {
						capa = 2
					}
					out = append(out, imagenMaq{punto{i, p.desde}, p.img, p.img.X - mg[3], p.img.Y - mg[0], w, h, capa})
				case p.img.Rodeada():
					for _, fm := range l.flotas {
						if fm.img == p.img {
							out = append(out, imagenMaq{punto{i, p.desde}, p.img, c.izq + fm.x, c.y + c.arribaTexto + fm.y, fm.w, fm.h, 1})
						}
					}
				default:
					out = append(out, imagenMaq{punto{i, p.desde}, p.img, c.izq + l.x0 + p.x, top + p.arriba, p.ancho, p.alto, 1})
				}
			}
		}
	}
	return out
}

// imagenBajo es la imagen que hay en (mx, my) de maqueta, la de más arriba.
func (f *Folio) imagenBajo(mx, my float32) (imagenMaq, bool) {
	var mejor imagenMaq
	hallada := false
	for _, im := range f.imagenes() {
		if mx >= im.x && mx < im.x+im.w && my >= im.y && my < im.y+im.h && (!hallada || im.capa >= mejor.capa) {
			mejor, hallada = im, true
		}
	}
	return mejor, hallada
}

// imagenSeleccionada es la seleccionada, si sigue en el documento.
func (f *Folio) imagenSeleccionada() (imagenMaq, bool) {
	if f.sel == nil {
		return imagenMaq{}, false
	}
	for _, im := range f.imagenes() {
		if im.q == *f.sel {
			return im, true
		}
	}
	return imagenMaq{}, false
}

// ImagenSeleccionada dice si hay una imagen seleccionada (`fmt.imageSelected`).
func (f *Folio) ImagenSeleccionada() bool {
	_, ok := f.imagenSeleccionada()
	return ok
}

// marcoImagen es la caja del ELEMENTO de la imagen —lo que mide `getBoundingClientRect`—,
// con lo recortado: lo que se ve más las bandas.
func marcoImagen(im imagenMaq) (x, y, w, h float32) {
	b := im.img.Bandas()
	return im.x - b[3], im.y - b[0], im.w + b[1] + b[3], im.h + b[0] + b[2]
}

// LadoTirador es el `h-2.5 w-2.5` de cada tirador, en px de pantalla: el marco va fuera
// del zoom, como en la web.
var LadoTirador = tema.Espacio(2.5)

// pintarSeleccion añade el marco de 1 px en `border-primary` y los cuatro tiradores
// (`border-primary bg-surface rounded-[2px] border`) centrados en las esquinas.
func (f *Folio) pintarSeleccion(objs []fyne.CanvasObject, ox, oy, z float32, v tema.Variante) []fyne.CanvasObject {
	im, ok := f.imagenSeleccionada()
	if !ok {
		return objs
	}
	x, y, w, h := marcoImagen(im)
	x, y, w, h = ox+x*z, oy+y*z, w*z, h*z
	primario := tema.Color("primary", v)
	marco := canvas.NewRectangle(color.NRGBA{})
	marco.StrokeColor, marco.StrokeWidth = primario, 1
	marco.Move(fyne.NewPos(x-0.5, y-0.5)) // el trazo va centrado: así cae entero por fuera
	marco.Resize(fyne.NewSize(w+1, h+1))
	objs = append(objs, marco)
	for _, e := range esquinas(x, y, w, h) {
		fuera := canvas.NewRectangle(primario)
		fuera.CornerRadius = 2
		fuera.Move(fyne.NewPos(e.X-LadoTirador/2, e.Y-LadoTirador/2))
		fuera.Resize(fyne.NewSize(LadoTirador, LadoTirador))
		dentro := canvas.NewRectangle(tema.Color("surface", v))
		dentro.CornerRadius = 1
		dentro.Move(fyne.NewPos(e.X-LadoTirador/2+1, e.Y-LadoTirador/2+1))
		dentro.Resize(fyne.NewSize(LadoTirador-2, LadoTirador-2))
		objs = append(objs, fuera, dentro)
	}
	return objs
}

// esquinas son las cuatro, en el orden de `CORNERS`: nw, ne, sw, se.
func esquinas(x, y, w, h float32) [4]fyne.Position {
	return [4]fyne.Position{{X: x, Y: y}, {X: x + w, Y: y}, {X: x, Y: y + h}, {X: x + w, Y: y + h}}
}

// arrastreImagen es un arrastre en marcha: de un tirador (cambia el tamaño) o de una
// imagen anclada (la mueve).
type arrastreImagen struct {
	mover         bool
	dir           float32 // -1 en las esquinas de la izquierda, +1 en las de la derecha
	x0, y0        float32 // dónde empezó, en px de la página
	w0, h0        float32
	px0, py0      float32 // la posición de la anclada al empezar
	foto          paso
	hayMovimiento bool
}

// tiradorEn dice si en pos (px de la página) hay un tirador de la imagen seleccionada, y
// en qué dirección crece el ancho al arrastrarlo.
func (f *Folio) tiradorEn(pos fyne.Position) (float32, bool) {
	im, ok := f.imagenSeleccionada()
	if !ok {
		return 0, false
	}
	z := f.escala()
	ox, oy := f.pag.origen()
	x, y, w, h := marcoImagen(im)
	for i, e := range esquinas(ox+x*z, oy+y*z, w*z, h*z) {
		if abs(pos.X-e.X) <= LadoTirador && abs(pos.Y-e.Y) <= LadoTirador {
			return map[bool]float32{true: 1, false: -1}[i%2 == 1], true
		}
	}
	return 0, false
}

// pulsarImagen decide qué hace una pulsación del ratón en pos si tiene que ver con una
// imagen: empezar a arrastrar un tirador o la anclada seleccionada, o seleccionar la que
// hay debajo. Devuelve false si no es cosa de imágenes, y entonces lo que había
// seleccionado se suelta, como un clic en otro sitio del folio.
func (f *Folio) pulsarImagen(pos fyne.Position) bool {
	if dir, ok := f.tiradorEn(pos); ok && f.editable() {
		im, _ := f.imagenSeleccionada()
		f.arrastre = &arrastreImagen{dir: dir, x0: pos.X, y0: pos.Y, foto: paso{f.doc.HTML(), f.cur, f.anc}}
		f.arrastre.w0, f.arrastre.h0 = im.w+im.img.Bandas()[1]+im.img.Bandas()[3], im.h+im.img.Bandas()[0]+im.img.Bandas()[2]
		if im.img.Ancho > 0 && im.img.Alto > 0 {
			f.arrastre.w0, f.arrastre.h0 = im.img.Ancho, im.img.Alto
		}
		return true
	}
	z := f.escala()
	ox, oy := f.pag.origen()
	im, ok := f.imagenBajo((pos.X-ox)/z, (pos.Y-oy)/z)
	if !ok {
		if f.sel != nil {
			f.sel = nil
			f.pag.Refresh()
			f.programarFormato()
		}
		return false
	}
	if f.sel != nil && *f.sel == im.q && im.img.Absoluta() && f.editable() {
		f.arrastre = &arrastreImagen{
			mover: true, x0: pos.X, y0: pos.Y, px0: im.img.X, py0: im.img.Y,
			foto: paso{f.doc.HTML(), f.cur, f.anc},
		}
		return true
	}
	f.mover(punto{im.q.b, im.q.p + 1}, false)
	q := im.q
	f.sel = &q
	f.pag.Refresh()
	return true
}

// arrastrar sigue el arrastre en marcha hasta pos (px de la página). Cambia el modelo en
// vivo, como el `writeModel(…, silent)` de la web; lo que se guarda es el final.
func (f *Folio) arrastrar(pos fyne.Position) {
	a := f.arrastre
	im, ok := f.imagenSeleccionada()
	if a == nil || !ok {
		return
	}
	z := f.escala()
	if a.mover {
		im.img.X = redondear2(a.px0 + (pos.X-a.x0)/z)
		im.img.Y = redondear2(a.py0 + (pos.Y-a.y0)/z)
	} else {
		proporcion := a.w0 / max(a.h0, 1)
		w := min(max(a.w0+(pos.X-a.x0)/z*a.dir, rico.MinImagen), max(f.anchoTexto(), rico.MinImagen))
		im.img.Ancho, im.img.Alto = redondear2(w), redondear2(w/proporcion)
	}
	f.completar(im)
	if f.doc.PonerImagen(im.img) {
		a.hayMovimiento = true
		f.remaquetar()
	}
}

// soltarArrastre acaba el arrastre: si ha cambiado algo, es UN paso de deshacer y se
// guarda.
func (f *Folio) soltarArrastre() {
	a := f.arrastre
	f.arrastre = nil
	if a == nil || !a.hayMovimiento {
		return
	}
	f.apuntar(a.foto, false)
	f.reestructurado()
}

// redondear2 es el `round2` de la web: a la centésima.
func redondear2(n float32) float32 { return float32(math.Floor(float64(n)*100+0.5) / 100) }

// completar es la primera parte de `applyImage`: una imagen que la web no ha tocado todavía
// toma el tamaño con el que se ve y, anclada, la posición en la que está.
func (f *Folio) completar(im imagenMaq) {
	b := im.img.Bandas()
	if im.img.Ancho <= 0 || im.img.Alto <= 0 {
		im.img.Ancho, im.img.Alto = float32(math.Round(float64(im.w+b[1]+b[3]))), float32(math.Round(float64(im.h+b[0]+b[2])))
	}
}

// Las órdenes de la imagen seleccionada. En la web no pasan por `exec`: cada control llama
// a `applyImage` o `removeImage`.
const (
	cmdAjusteImagen    editor.Comando = "imageWrap"  // valor: el ajuste
	cmdAlinearImagen   editor.Comando = "imageAlign" // valor: la alineación
	cmdAltImagen       editor.Comando = "imageAlt"   // valor: el texto alternativo
	cmdSinRecorte      editor.Comando = "imageRemoveCrop"
	cmdEliminarImagen  editor.Comando = "imageRemove"
	cmdSoltarSeleccion editor.Comando = "imageDeselect"
)

// ordenImagen ejecuta una orden de la imagen seleccionada. Devuelve false si no es una.
func (f *Folio) ordenImagen(c editor.Comando, valor string) bool {
	switch c {
	case cmdAjusteImagen, cmdAlinearImagen, cmdAltImagen, cmdSinRecorte, cmdEliminarImagen:
	default:
		return false
	}
	im, ok := f.imagenSeleccionada()
	if !ok {
		return true
	}
	foto := paso{f.doc.HTML(), f.cur, f.anc}
	if c == cmdEliminarImagen {
		if f.doc.QuitarImagen(im.img) {
			f.sel = nil
			f.apuntar(foto, false)
			f.cur, f.anc = im.q, im.q
			f.reestructurado()
		}
		return true
	}
	f.cambiarImagen(im, foto, c == cmdAltImagen, func(img *rico.Imagen) bool {
		switch c {
		case cmdAjusteImagen:
			img.Ajuste = valor
		case cmdAlinearImagen:
			img.Alinear = valor
		case cmdAltImagen:
			img.Alt = valor
		case cmdSinRecorte:
			img.Recorte = [4]float32{}
		default:
		}
		return true
	})
	return true
}

// Las medidas de la barra flotante. En la web no pasan por `exec` ni por el registro de
// comandos —cada campo llama a `applyImage`—, y aquí tampoco: no se graban en una macro.
const (
	medidaAncho = iota
	medidaAlto
)

// medirSeleccionada pone el ancho o el alto exactos de la imagen seleccionada.
func (f *Folio) medirSeleccionada(cual int, valor string) {
	if im, ok := f.imagenSeleccionada(); ok && f.editable() {
		f.cambiarImagen(im, paso{f.doc.HTML(), f.cur, f.anc}, true, func(img *rico.Imagen) bool {
			return f.medirImagen(img, cual == medidaAncho, valor)
		})
	}
}

// recortarSeleccionada pone lo recortado de un lado («lado=%») de la imagen seleccionada;
// «» la deja sin recorte.
func (f *Folio) recortarSeleccionada(valor string) {
	if im, ok := f.imagenSeleccionada(); ok && f.editable() {
		f.cambiarImagen(im, paso{f.doc.HTML(), f.cur, f.anc}, valor != "", func(img *rico.Imagen) bool {
			if valor == "" {
				img.Recorte = [4]float32{}
				return true
			}
			return recortarLado(img, valor)
		})
	}
}

// cambiarImagen es `applyImage`: completa el tamaño de una imagen que la web no ha tocado,
// aplica el cambio y, si pasa a anclada, la deja donde está. `agrupar` junta en un paso de
// deshacer lo que se teclea seguido, como el texto.
func (f *Folio) cambiarImagen(im imagenMaq, foto paso, agrupar bool, cambio func(*rico.Imagen) bool) {
	antes := im.img.Ajuste
	f.completar(im)
	if !cambio(im.img) {
		return
	}
	if im.img.Absoluta() && antes != "behind" && antes != "front" {
		// Pasa a anclada donde está ahora: la esquina de su elemento, en el folio.
		x, y, _, _ := marcoImagen(im)
		mg := f.pagina.margenes()
		im.img.X, im.img.Y = x+mg[3], y+mg[0]
	}
	if f.doc.PonerImagen(im.img) {
		f.apuntar(foto, agrupar)
		f.reestructurado()
	}
}

// MaxAltoImagen es el tope del alto exacto (`clamp(…, MIN_SIZE, 4000)` de `setHeight`).
const MaxAltoImagen = 4000

// MaxRecorteLado es el tope de lo recortado de un lado, en % (`clamp(…, 0, 90)`).
const MaxRecorteLado = 90

// medirImagen es `setWidth`/`setHeight` de la barra flotante: la medida pedida, acotada
// —el ancho al del texto, el alto a [MaxAltoImagen]—, y la otra con la proporción de ahora.
// Un texto que no es una cifra no cambia nada: la web lo lleva a `MIN_SIZE` porque su
// campo solo avisa al confirmar, y aquí se aplica según se escribe.
func (f *Folio) medirImagen(img *rico.Imagen, ancho bool, valor string) bool {
	n, err := strconv.ParseFloat(strings.TrimSpace(valor), 32)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return false
	}
	proporcion := img.Ancho / max(img.Alto, 1)
	if proporcion <= 0 {
		proporcion = 1
	}
	if ancho {
		w := min(max(float32(n), rico.MinImagen), max(f.anchoTexto(), rico.MinImagen))
		img.Ancho, img.Alto = redondear2(w), redondear2(w/proporcion)
	} else {
		h := min(max(float32(n), rico.MinImagen), MaxAltoImagen)
		img.Ancho, img.Alto = redondear2(h*proporcion), redondear2(h)
	}
	return true
}

// ladosRecorte son los de `data-crop`, en su orden: arriba, derecha, abajo, izquierda.
var ladosRecorte = [4]string{"t", "r", "b", "l"}

// clavesRecorte son las de `CROP_SIDE_KEY`, en el mismo orden.
var clavesRecorte = [4]string{"image.crop.top", "image.crop.right", "image.crop.bottom", "image.crop.left"}

// recortarLado es `setCropSide`: «lado=%» pone ese lado entre 0 y 90 % y acota el conjunto
// con [rico.ClampRecorte], como `applyImage`. Lo que no se entiende no cambia nada.
func recortarLado(img *rico.Imagen, valor string) bool {
	lado, pct, ok := strings.Cut(valor, "=")
	i := slices.Index(ladosRecorte[:], lado)
	n, err := strconv.ParseFloat(strings.TrimSpace(pct), 32)
	if !ok || i < 0 || err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return false
	}
	r := img.Recorte
	r[i] = float32(min(max(n, 0), MaxRecorteLado)) / 100
	img.Recorte = rico.ClampRecorte(r)
	return true
}

// cifra es como la web enseña una medida en su campo numérico: redondeada a la unidad.
func cifra(n float32) string { return strconv.Itoa(int(math.Round(float64(n)))) }

// AjustesImagen son los de `IMAGE_WRAPS`, con la clave de su nombre.
var AjustesImagen = []struct{ Valor, Clave string }{
	{"inline", "image.wrap.inline"},
	{"square", "image.wrap.square"},
	{"tight", "image.wrap.tight"},
	{"topBottom", "image.wrap.topBottom"},
	{"behind", "image.wrap.behind"},
	{"front", "image.wrap.front"},
}

// Los anchos de los dos selectores de la cinta (`width={150}` y `width={104}`).
const (
	AnchoSelectorAjuste  float32 = 150
	AnchoSelectorAlinear float32 = 104
)

// grupoImagen es `ImagenSeleccionadaGroup`: el ajuste, la alineación (solo si flota o va
// arriba y abajo), el texto alternativo, quitar el recorte y eliminar.
func (f *Folio) grupoImagen(c *editor.Contexto) fyne.CanvasObject {
	im, ok := f.imagenSeleccionada()
	if !ok {
		return nil
	}
	mandar := func(cmd editor.Comando) func(string) { return func(v string) { c.Mandar(cmd, v) } }
	var ajustes []editor.Opcion
	for _, a := range AjustesImagen {
		ajustes = append(ajustes, editor.Opcion{Valor: a.Valor, Rotulo: c.Traducir(a.Clave)})
	}
	ajuste := editor.NuevoSelectorCinta(c.Traducir("image.wrap"), AnchoSelectorAjuste, ajustes, im.img.Ajuste, mandar(cmdAjusteImagen))
	ajuste.Inerte = c.SoloLectura
	controles := []fyne.CanvasObject{ajuste}
	if im.img.Rodeada() || im.img.Ajuste == "topBottom" {
		opciones := []editor.Opcion{{Valor: "left", Rotulo: c.Traducir("image.align.left")}}
		if !im.img.Rodeada() {
			opciones = append(opciones, editor.Opcion{Valor: "center", Rotulo: c.Traducir("image.align.center")})
		}
		opciones = append(opciones, editor.Opcion{Valor: "right", Rotulo: c.Traducir("image.align.right")})
		alinear := editor.NuevoSelectorCinta(c.Traducir("image.align.picker"), AnchoSelectorAlinear, opciones, im.img.Alinear, mandar(cmdAlinearImagen))
		alinear.Inerte = c.SoloLectura
		controles = append(controles, alinear)
	}
	alt := editor.NuevoBotonEmergente(iconos.EdImagen, c.Traducir("image.alt"), func(cerrar func()) fyne.CanvasObject {
		return f.panelAlt(c, cerrar)
	})
	boton := func(i iconos.Ed, clave string, cmd editor.Comando) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, c.Traducir(clave), func() { c.Mandar(cmd, "") })
		b.Inerte = c.SoloLectura
		return b
	}
	alt.Inerte = c.SoloLectura
	herramientas := editor.NuevoBotonEmergente(iconos.EdTamanoPagina, c.Traducir("image.toolbar"), func(func()) fyne.CanvasObject {
		return f.panelMedidas(c)
	})
	herramientas.Inerte = c.SoloLectura
	controles = append(controles, alt, herramientas, boton(iconos.EdRecorte, "image.removeCrop", cmdSinRecorte), boton(iconos.EdPapelera, "image.remove", cmdEliminarImagen))
	return editor.NuevoGrupo(c.Traducir("group.image"), controles...)
}

// Los anchos de los campos del panel: `w-16` los de la medida y `w-12` los del recorte.
var (
	anchoCampoMedidaImagen  = tema.Espacio(16)
	anchoCampoRecorteImagen = tema.Espacio(12)
)

// panelMedidas es lo que la barra flotante de la web (`ImageOverlay.tsx`) añade a la cinta:
// «An» y «Al» en px, que guardan la proporción, y el recorte de cada lado en % con
// «Reiniciar». Cada cifra válida se aplica según se escribe, y los demás campos pasan a
// decir lo que quedó: el alto que sigue al ancho, o el recorte acotado del lado opuesto.
func (f *Folio) panelMedidas(c *editor.Contexto) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	im, ok := f.imagenSeleccionada()
	if !ok {
		return ui.NuevoParrafo(c.Traducir("image.none"), tema.TextoXS, ui.Normal, "text-muted", v)
	}
	rotulo := func(clave string) fyne.CanvasObject {
		return ui.NuevoRotulo(c.Traducir(clave), tema.TextoXS, ui.Normal, "text-muted")
	}
	// poniendo calla los campos mientras se les pone texto desde aquí: sin ello, poner el
	// alto que sigue al ancho mandaría ese alto, que volvería a poner el ancho, y así.
	poniendo := false
	campo := func(nombre string, ancho float32, valor string, alCambiar func(string)) *ui.Entrada {
		e := campoPanel("", ancho)
		e.Rotulo = c.Traducir(nombre)
		e.PonerTexto(valor)
		e.AlCambiar(func(s string) {
			if !poniendo && strings.TrimSpace(s) != "" {
				alCambiar(s)
			}
		})
		return e
	}
	var ancho, alto *ui.Entrada
	var lados [4]*ui.Entrada
	// releer pone en los campos lo que tiene ahora la imagen, salvo en el que se escribe:
	// el alto sigue al ancho, y al revés, sin pisar lo que se está tecleando.
	releer := func(menos *ui.Entrada) {
		im, ok := f.imagenSeleccionada()
		if !ok {
			return
		}
		poniendo = true
		defer func() { poniendo = false }()
		for _, x := range []struct {
			e *ui.Entrada
			n float32
		}{{ancho, im.img.Ancho}, {alto, im.img.Alto}} {
			if x.e != menos && x.n > 0 {
				x.e.PonerTexto(cifra(x.n))
			}
		}
		for i, e := range lados {
			if e != menos {
				e.PonerTexto(cifra(im.img.Recorte[i] * 100))
			}
		}
	}
	medir := func(cual int, desde **ui.Entrada) func(string) {
		return func(s string) {
			f.medirSeleccionada(cual, s)
			releer(*desde)
		}
	}
	w, h := im.img.Ancho, im.img.Alto
	if w <= 0 || h <= 0 {
		_, _, w, h = marcoImagen(im)
	}
	ancho = campo("image.width", anchoCampoMedidaImagen, cifra(w), medir(medidaAncho, &ancho))
	alto = campo("image.height", anchoCampoMedidaImagen, cifra(h), medir(medidaAlto, &alto))
	medidas := container.New(ui.Fila{Hueco: tema.Espacio(1)},
		rotulo("image.width.short"), ancho, rotulo("image.height.short"), alto)
	recorte := []fyne.CanvasObject{rotulo("image.crop.percent")}
	for i := range lados {
		lado := ladosRecorte[i]
		lados[i] = campo(clavesRecorte[i], anchoCampoRecorteImagen, cifra(im.img.Recorte[i]*100), func(s string) {
			f.recortarSeleccionada(lado + "=" + s)
			releer(lados[i])
		})
		recorte = append(recorte, lados[i])
	}
	reiniciar := ui.NuevoBoton(c.Traducir("image.crop.reset"), func() {
		f.recortarSeleccionada("")
		releer(nil)
	}).Con(ui.Fantasma, ui.TallaXS)
	recorte = append(recorte, reiniciar)
	for _, e := range append([]*ui.Entrada{ancho, alto}, lados[:]...) {
		e.Inerte(c.SoloLectura)
	}
	reiniciar.Inerte = c.SoloLectura
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, medidas, container.New(ui.Fila{Hueco: tema.Espacio(1)}, recorte...))
}

// panelAlt es `ImagenAltPanel`: el texto alternativo, que se aplica mientras se escribe;
// Intro cierra. Sin imagen, «Selecciona una imagen».
func (f *Folio) panelAlt(c *editor.Contexto, cerrar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	im, ok := f.imagenSeleccionada()
	if !ok {
		return ui.NuevoParrafo(c.Traducir("image.none"), tema.TextoXS, ui.Normal, "text-muted", v)
	}
	campo := ui.NuevaEntrada(c.Traducir("image.alt.placeholder")).Con(ui.CampoSM)
	campo.Ancho = tema.Espacio(64) // `w-64`
	campo.PonerTexto(im.img.Alt)
	campo.AlCambiar(func(s string) { c.Mandar(cmdAltImagen, s) })
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			cerrar()
			return true
		}
		return false
	}
	fyne.Do(func() {
		if lz := fyne.CurrentApp().Driver().CanvasForObject(campo); lz != nil {
			lz.Focus(campo)
		}
	})
	etiqueta := ui.NuevoParrafo(c.Traducir("image.alt"), tema.TextoXS, ui.Medio, "text", v)
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, etiqueta, campo)
}
