// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"slices"
	"strconv"

	"fyne.io/fyne/v2"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // las que el navegador sabe abrir con `accept="image/*"`

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La pestaña «Insertar» de una presentación
//
// Es `InsertarTab` con `ctx.kind === 'presentation'`, grupo a grupo y en su orden:
// Tabla · Imagen · Formas · Conectores · SmartArt · Multimedia · Enlace · Elementos ·
// Comentario. Multimedia abre su panel (`medios.go`) y Comentario el de comentarios del
// cromo.
//
// La tabla, el enlace, la línea, el salto y el símbolo se escriben en el CURSOR de la
// caja de texto que se está escribiendo (el `exec` de la web): los hace su folio, con los
// paneles comunes de los editores. Sin caja abierta se ven encendidos y no hacen nada,
// como en la web.

// cuerpoInsertar es la banda de «Insertar».
func (e *Editor) cuerpoInsertar(c *editor.Contexto) fyne.CanvasObject {
	ro := c.SoloLectura
	boton := func(i iconos.Ed, rotulo string, inerte bool, f func()) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, rotulo, f)
		b.Inerte = inerte || ro
		return b
	}
	mandar := func(cmd editor.Comando) func() { return func() { c.Mandar(cmd, "") } }
	emergente := func(i iconos.Ed, clave string, cuerpo func(cerrar func()) fyne.CanvasObject) *editor.BotonCinta {
		b := editor.NuevoBotonEmergente(i, c.Traducir(clave), cuerpo)
		b.Inerte = ro
		return b
	}
	formas := make([]fyne.CanvasObject, len(formasUI))
	for k, f := range formasUI {
		formas[k] = boton(f.Icono, c.Traducir(f.Clave), false, func() { e.NuevaForma(f.ID) })
	}
	conectores := make([]fyne.CanvasObject, len(conectoresUI))
	for k, cn := range conectoresUI {
		conectores[k] = boton(cn.Icono, c.Traducir(cn.Clave), false, func() { e.NuevoConector(cn.ID) })
	}
	smart := make([]fyne.CanvasObject, len(disposicionesSmartUI))
	for k, d := range disposicionesSmartUI {
		smart[k] = boton(d.Icono, c.Traducir(d.Clave), false, func() { e.NuevoDiagrama(d.ID) })
	}
	medio := editor.NuevoBotonCinta(iconos.EdVideo, c.Traducir("insert.media"), nil)
	medio.AlPulsar = func() { e.abrirMedio(medio) }
	medio.Inerte = c.SoloLectura
	grupos := []fyne.CanvasObject{
		editor.NuevoGrupo(c.Traducir("settings.table"), emergente(iconos.EdTabla, "insert.table", func(cerrar func()) fyne.CanvasObject {
			return editor.NuevoPanelTabla(c, func(filas, cols int, cabecera bool) {
				cerrar()
				valor := strconv.Itoa(filas) + "x" + strconv.Itoa(cols)
				if cabecera {
					valor += "h"
				}
				c.Mandar(editor.InsertarTabla, valor)
			})
		})),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("slides.box.image"),
			boton(iconos.EdImagen, c.Traducir("insert.image"), e.ElegirImagen == nil, e.pedirImagen)),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("blocks.shapes"), formas...),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.connectors"), conectores...),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("smartart.title"), smart...),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.media"), medio),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.link"), emergente(iconos.EdEnlace, "insert.link", func(cerrar func()) fyne.CanvasObject {
			seleccion := ""
			if f := e.Folio(); f != nil {
				seleccion = f.TextoSeleccionado()
			}
			return editor.NuevoPanelEnlace(c, seleccion, func(href, texto string) {
				cerrar()
				c.Mandar(editor.InsertarEnlace, href+"\n"+texto)
			})
		})),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.elements"),
			boton(iconos.EdSeparador, c.Traducir("insert.horizontalRule"), false, mandar(editor.InsertarRegla)),
			boton(iconos.EdSaltoPagina, c.Traducir("insert.pageBreak"), false, mandar(editor.SaltoPagina)),
			emergente(iconos.EdSimbolo, "insert.symbol", func(cerrar func()) fyne.CanvasObject {
				return editor.NuevoPanelSimbolos(c, func(s string) {
					cerrar()
					c.Mandar(editor.InsertarTexto, s)
				})
			})),
		editor.SeparadorGrupos(),
		editor.NuevoGrupo(c.Traducir("group.comment"),
			editor.NuevoBotonCinta(iconos.EdComentario, c.Traducir("insert.comment"), func() { c.AbrirPanel(editor.PanelComentarios) })),
	}
	// Los bloques reutilizables de Sytue y de la organización, al final como en la web.
	if c.Bloques != nil {
		grupos = append(grupos, editor.SeparadorGrupos(), c.Bloques.Grupo(c))
	}
	return editor.Grupos(grupos...)
}

// conectoresUI son CONNECTOR_KINDS_UI.
var conectoresUI = []struct {
	ID, Clave string
	Icono     iconos.Ed
}{
	{"straight", "slides.connector.straight", iconos.EdConRecto},
	{"elbow", "slides.connector.elbow", iconos.EdConCodo},
	{"curved", "slides.connector.curved", iconos.EdConCurvo},
}

// empujarCaja es `pushBox`: la caja al final de lo que se edita —la diapositiva activa
// o el patrón— y elegida.
func (e *Editor) empujarCaja(c Caja) {
	e.cambiar(func() {
		if l := e.m.listaCajas(e.objetivo(), true); l != nil {
			*l = append(*l, c)
			e.sel, e.extra = c.ID, nil
		}
	}, true)
}

// NuevaForma es `addShapeBox`: centrada, las lineales finas y el resto de 30 × 36.
func (e *Editor) NuevaForma(forma string) {
	lineal := forma == "line" || forma == "arrow"
	c := Caja{ID: uuid(), Tipo: "shape", Forma: forma, X: 35, Y: 32, W: 30, H: 36, Relleno: rellenoForma, Trazo: trazoForma}
	if lineal {
		c.Y, c.H = 47, 6
	}
	e.empujarCaja(c)
}

// NuevoDiagrama es `addSmartArtBox`, con los elementos de arranque de su disposición en
// el idioma de la sesión, y abre su panel.
func (e *Editor) NuevoDiagrama(disp string) {
	e.abajo = abajoSmart
	e.empujarCaja(Caja{ID: uuid(), Tipo: "smartart", X: 14, Y: 12, W: 72, H: 72, Diagrama: &Diagrama{
		Disposicion: disp, Elementos: e.elementosDeArranque(disp),
	}})
}

// elementosDeArranque es `defaultSmartArtItems`: la jerarquía nace con una raíz y dos
// hijos para que se vea el árbol de inmediato.
func (e *Editor) elementosDeArranque(disp string) []Elemento {
	n := func(clave string, cuantos int) []Elemento {
		out := make([]Elemento, cuantos)
		for i := range out {
			out[i].Texto = e.ctx.Traducir(clave, map[string]any{"n": i + 1})
		}
		return out
	}
	switch disp {
	case "process":
		return n("slides.smartart.sample.step", 3)
	case "cycle":
		return n("slides.smartart.sample.phase", 4)
	case "hierarchy":
		uno := 1.0
		return []Elemento{
			{Texto: e.t("slides.smartart.sample.root")},
			{Texto: e.ctx.Traducir("slides.smartart.sample.team", map[string]any{"n": 1}), Nivel: &uno},
			{Texto: e.ctx.Traducir("slides.smartart.sample.team", map[string]any{"n": 2}), Nivel: &uno},
		}
	}
	return n("slides.smartart.sample.item", 3)
}

// NuevoConector es `addConnector`: entre las dos primeras cajas elegidas o, si no las
// hay, suelto en el centro, siempre con punta al final. En el patrón no hace nada: los
// conectores son de las diapositivas.
func (e *Editor) NuevoConector(tipo string) {
	if e.patron {
		return
	}
	elegidas := e.explicita()
	c := Conector{ID: uuid(), Trazado: tipo}
	verdad := true
	c.PuntaFin = &verdad
	if len(elegidas) >= 2 {
		c.Desde, c.Hasta = Extremo{Caja: elegidas[0]}, Extremo{Caja: elegidas[1]}
	} else {
		x0, y0, x1, y1 := 28.0, 42.0, 64.0, 62.0
		c.Desde, c.Hasta = Extremo{X: &x0, Y: &y0}, Extremo{X: &x1, Y: &y1}
	}
	e.cambiar(func() {
		d := &e.m.Diapos[e.activa]
		d.Conectores = append(d.Conectores, c)
		e.sel, e.extra, e.selCon = "", nil, c.ID
	}, true)
}

// pedirImagen abre el selector de imágenes de quien monta el editor (el del sistema).
func (e *Editor) pedirImagen() {
	if e.ElegirImagen == nil {
		return
	}
	e.ElegirImagen(func(datos []byte, jpeg bool) {
		// Reescalar una foto de doce megapíxeles cuesta; fuera del hilo de la interfaz.
		go func() {
			url, err := CodificarImagen(datos, jpeg)
			fyne.Do(func() {
				switch {
				case errors.Is(err, ErrImagenGrande):
					e.avisar("slides.msg.imageTooBig", "insert.msg.imageTooBig.detail")
				case err != nil:
					e.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, e.t("insert.msg.imageReadFailed"), ""))
				default:
					e.NuevaImagen(url)
				}
			})
		}()
	})
}

// NuevaImagen es `addImageBox`: con el 40 % de ancho (y 70 % de alto como mucho),
// centrada y con su aspecto, si el modelo entero sigue cabiendo en un guardado.
func (e *Editor) NuevaImagen(src string) {
	img, ok := imagenDe(src)
	if !ok {
		return
	}
	b := img.Bounds()
	x, y, w, h := RectImagen(b.Dx(), b.Dy(), float64(e.m.AltoEscenario()))
	c := Caja{ID: uuid(), Tipo: "image", X: x, Y: y, W: w, H: h, Src: src}
	if !e.cabeCaja(c) {
		e.avisar("slides.msg.imageTooBig", "slides.msg.imageTooBig.detail")
		return
	}
	e.empujarCaja(c)
}

// RectImagen es `imageBoxRect`: el encuadre inicial en % del lienzo.
func RectImagen(nw, nh int, sh float64) (x, y, w, h float64) {
	w = 40
	h = w * (float64(nh) / math.Max(1, float64(nw))) * float64(AnchoEscenario) / sh
	if h > 70 {
		w, h = w*70/h, 70
	}
	w, h = math.Max(4, w), math.Max(4, h)
	return (100 - w) / 2, (100 - h) / 2, w, h
}

// Topes de `encodeSlideImage`: el dataURL por debajo de 700 KiB y 1600 px de lado.
const (
	maxCaracteresImagen = 700 * 1024
	maxLadoImagen       = 1600
)

// ErrImagenGrande: ni al mínimo cabe la imagen en el tope.
var ErrImagenGrande = errors.New("diapos: la imagen no cabe en una diapositiva")

// CodificarImagen es `encodeSlideImage`: reescala y recodifica hasta que el dataURL
// baje del tope —1600 px de lado y JPEG de calidad 82, o PNG si tiene transparencia—,
// bajando tamaño y calidad por pasos. `esJPEG` dice si el fichero ya era JPEG (entonces
// no se mira el alfa).
func CodificarImagen(datos []byte, esJPEG bool) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	lado := math.Min(maxLadoImagen, math.Max(float64(b.Dx()), float64(b.Dy())))
	calidad := 82
	usarPNG := !esJPEG && tieneAlfa(img)
	for range 7 {
		k := lado / math.Max(1, math.Max(float64(b.Dx()), float64(b.Dy())))
		w, h := max(1, int(math.Round(float64(b.Dx())*k))), max(1, int(math.Round(float64(b.Dy())*k)))
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		if !usarPNG {
			draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src) // JPEG no tiene alfa
		}
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
		var buf bytes.Buffer
		tipo := "image/jpeg"
		if usarPNG {
			tipo = "image/png"
			err = png.Encode(&buf, dst)
		} else {
			err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: calidad})
		}
		if err != nil {
			return "", err
		}
		url := "data:" + tipo + ";base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
		if len(url) <= maxCaracteresImagen {
			return url, nil
		}
		lado = math.Round(lado * 0.75)
		calidad = max(50, calidad-7)
	}
	return "", ErrImagenGrande
}

// tieneAlfa es `canvasHasAlpha`: algún píxel translúcido.
func tieneAlfa(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				return true
			}
		}
	}
	return false
}

// cabeCaja dice si el modelo, con `c` añadida a lo que se edita, sigue cabiendo en un
// guardado (`modelWithBoxes` y MODEL_MAX_CHARS de la web).
func (e *Editor) cabeCaja(c Caja) bool {
	prueba := *e.m
	prueba.Diapos = append([]Diapo(nil), e.m.Diapos...)
	if e.m.Patron != nil {
		copia := *e.m.Patron
		prueba.Patron = &copia
	}
	if l := prueba.listaCajas(e.objetivo(), true); l != nil {
		*l = append(slices.Clone(*l), c)
	}
	return prueba.cabe()
}
