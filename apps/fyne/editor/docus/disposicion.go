// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los paneles de «Marca de agua» y «Borde» de Disposición.tsx (`WatermarkPanel` y
// `PageBorderPanel`), en el `w-64 p-2` de su desplegable, y el selector de «Columnas».

// AnchoPanelDecoracion es el `w-64` del desplegable, sin su `p-2`.
var AnchoPanelDecoracion = tema.Espacio(64) - 2*tema.Espacio(2)

// Los anchos de los selectores de los paneles y de «Columnas».
const (
	AnchoSelectorMarca    float32 = 104
	AnchoSelectorBorde    float32 = 116
	AnchoSelectorColumnas float32 = 80
)

// MaxImagenMarca es `WM_IMAGE_MAX_CHARS`: el tope de la data: URL de la imagen de la marca.
const MaxImagenMarca = 600 * 1024

// Los ajustes que ofrece la web (`WATERMARK_PRESETS`, `WM_OPACITIES`, los tamaños).
var (
	presetsMarca       = []string{"watermark.preset.draft", "watermark.preset.confidential", "watermark.preset.doNotCopy", "watermark.preset.urgent", "watermark.preset.sample"}
	opacidadesMarca    = []float64{0.08, 0.15, 0.25, 0.4, 0.6}
	tamanosTextoMarca  = []struct{ v, clave string }{{"80", "watermark.size.small"}, {"120", "watermark.size.medium"}, {"180", "watermark.size.large"}, {"240", "watermark.size.huge"}}
	tamanosImagenMarca = []int{40, 60, 80, 100}
)

// presetsBorde son los `PAGE_BORDER_PRESETS`.
var presetsBorde = []struct {
	clave string
	b     rico.BordePagina
}{
	{"border.preset.thinLine", rico.BordePagina{Estilo: "solid", Grosor: 1, Color: "#475569"}},
	{"border.preset.thickLine", rico.BordePagina{Estilo: "solid", Grosor: 3, Color: "#0A0A0A"}},
	{"border.style.double", rico.BordePagina{Estilo: "double", Grosor: 4, Color: "#475569"}},
	{"border.style.dashed", rico.BordePagina{Estilo: "dashed", Grosor: 2, Color: "#475569"}},
	{"border.style.dotted", rico.BordePagina{Estilo: "dotted", Grosor: 2, Color: "#94A3B8"}},
}

// botonPreset es PRESET_BTN: `rounded-sm border px-2 py-1 text-xs`.
func botonPreset(texto string, alPulsar func()) *editor.BotonPequeno {
	b := editor.NuevoBotonPequeno(texto, alPulsar)
	b.Alto = tema.TextoXS.Linea + 2*tema.Espacio(1) + 2*tema.Borde
	return b
}

// botonQuitarDecoracion es el «Quitar» de los paneles: sin caja, con la papelera `sm`.
func botonQuitarDecoracion(texto string, alPulsar func()) *editor.BotonPequeno {
	b := botonPreset(texto, alPulsar)
	b.SinBorde = true
	b.Glifo = iconos.TrazoEd(iconos.EdPapelera, 14, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	return b
}

// filaAjuste es `flex items-center justify-between gap-2`: el rótulo `text-xs
// text-text-muted` a la izquierda y el control a la derecha.
func filaAjuste(rotulo string, control fyne.CanvasObject) fyne.CanvasObject {
	return container.New(filaAcciones{}, ui.NuevoRotulo(rotulo, tema.TextoXS, ui.Normal, "text-muted"), control)
}

// campoPanel es FIELD_CLASS: `h-7 rounded-sm border border-border px-1.5 text-xs`.
func campoPanel(marca string, ancho float32) *ui.Entrada {
	e := ui.NuevaEntrada(marca).ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(1.5), MargenY: (tema.Espacio(7) - tema.TextoXS.Linea - 2*tema.Borde) / 2,
		Texto: tema.TextoXS, Borde: "border", Radio: tema.RadioSM,
	})
	e.Ancho = ancho
	return e
}

// panelMarca es `WatermarkPanel`: cada cambio se aplica al folio en el acto.
type panelMarca struct {
	c      *editor.Contexto
	m      rico.MarcaAgua // lo que se está editando; sin texto ni imagen, no hay marca
	cuerpo *fyne.Container
	texto  *ui.Entrada
	// poniendo calla el campo mientras se le pone texto desde fuera (un preset).
	poniendo bool
}

// ponerTexto escribe en el campo sin que cuente como teclear.
func (p *panelMarca) ponerTexto(s string) {
	p.poniendo = true
	p.texto.PonerTexto(s)
	p.poniendo = false
}

func nuevoPanelMarca(c *editor.Contexto, f *Folio) fyne.CanvasObject {
	p := &panelMarca{c: c, m: rico.MarcaAgua{Color: rico.ColorMarcaAgua, Opacidad: 0.15, Tamano: 120, Diagonal: true}}
	if actual := f.doc.MarcaAguaDe(); actual != nil {
		p.m = *actual
	}
	p.texto = campoPanel(c.Traducir("watermark.text.custom"), AnchoPanelDecoracion)
	p.texto.Rotulo = c.Traducir("watermark.text.label")
	if !p.m.Imagen {
		p.ponerTexto(p.m.Texto)
	}
	p.texto.AlCambiar(func(s string) {
		if !p.poniendo {
			p.cambiar(func(m *rico.MarcaAgua) { m.Texto, m.Imagen, m.Src = s, false, "" })
		}
	})
	p.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(1.5)})
	p.pintar()
	return container.New(anchoFijo{AnchoPanelDecoracion}, p.cuerpo)
}

// cambiar mezcla el cambio y lo aplica al folio (o quita la marca si se queda vacía).
func (p *panelMarca) cambiar(cambio func(*rico.MarcaAgua)) {
	cambio(&p.m)
	kind := "text"
	if p.m.Imagen {
		kind = "image"
	}
	diag := "0"
	if p.m.Diagonal {
		diag = "1"
	}
	m := rico.SanearMarcaAgua(kind == "image", p.m.Texto, p.m.Src, p.m.Color, strconv.FormatFloat(p.m.Opacidad, 'f', -1, 64),
		strconv.Itoa(p.m.Tamano), diag)
	p.c.Mandar(cmdMarcaAgua, valorMarca(m))
	p.pintar()
}

func (p *panelMarca) pintar() {
	c, v := p.c, tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	presets := make([]fyne.CanvasObject, 0, len(presetsMarca))
	for _, k := range presetsMarca {
		presets = append(presets, botonPreset(c.Traducir(k), func() {
			p.ponerTexto(c.Traducir(k))
			p.cambiar(func(m *rico.MarcaAgua) { m.Texto, m.Imagen, m.Src = c.Traducir(k), false, "" })
		}))
	}
	orientacion := make([]fyne.CanvasObject, 0, 2)
	for _, o := range []struct {
		diag  bool
		clave string
	}{{true, "watermark.orientation.diagonal"}, {false, "watermark.orientation.horizontal"}} {
		b := editor.NuevoBotonPequeno(c.Traducir(o.clave), func() { p.cambiar(func(m *rico.MarcaAgua) { m.Diagonal = o.diag }) })
		b.Opcion, b.Pulsado = true, p.m.Diagonal == o.diag
		orientacion = append(orientacion, b)
	}
	objs := []fyne.CanvasObject{
		ui.NuevoRotulo(c.Traducir("watermark.text"), tema.TextoXS, ui.Medio, "text-muted"),
		container.New(&envuelta{hueco: tema.Espacio(1), ancho: AnchoPanelDecoracion}, presets...),
		p.texto,
		filaAjuste(c.Traducir("watermark.orientation"), container.New(ui.Fila{Hueco: tema.Espacio(1)}, orientacion...)),
	}
	if !p.m.Imagen {
		color := editor.NuevaMuestraCinta(c, iconos.EdColorTexto, c.Traducir("watermark.color.label"), p.m.Color, editor.PaletaContenido,
			func(col string) { p.cambiar(func(m *rico.MarcaAgua) { m.Color = col }) })
		objs = append(objs, filaAjuste(c.Traducir("watermark.color"), color))
	}
	var tamanos []editor.Opcion
	if p.m.Imagen {
		for _, t := range tamanosImagenMarca {
			tamanos = append(tamanos, editor.Opcion{Valor: strconv.Itoa(t), Rotulo: i18n.FormatPercent(c.Loc, float64(t)/100)})
		}
	} else {
		for _, t := range tamanosTextoMarca {
			tamanos = append(tamanos, editor.Opcion{Valor: t.v, Rotulo: c.Traducir(t.clave)})
		}
	}
	tamano := editor.NuevoSelectorCinta(c.Traducir("watermark.size.label"), AnchoSelectorMarca, tamanos, strconv.Itoa(p.m.Tamano), func(s string) {
		n, _ := strconv.Atoi(s)
		p.cambiar(func(m *rico.MarcaAgua) { m.Tamano = n })
	})
	opacidades := make([]editor.Opcion, 0, len(opacidadesMarca))
	for _, o := range opacidadesMarca {
		opacidades = append(opacidades, editor.Opcion{Valor: strconv.FormatFloat(o, 'f', -1, 64), Rotulo: i18n.FormatPercent(c.Loc, o)})
	}
	opacidad := editor.NuevoSelectorCinta(c.Traducir("watermark.opacity.label"), AnchoSelectorMarca, opacidades,
		strconv.FormatFloat(p.m.Opacidad, 'f', -1, 64), func(s string) {
			o, _ := strconv.ParseFloat(s, 64)
			p.cambiar(func(m *rico.MarcaAgua) { m.Opacidad = o })
		})
	claveImagen := "watermark.image.pick"
	if p.m.Imagen {
		claveImagen = "watermark.image.change"
	}
	imagen := botonPreset(c.Traducir(claveImagen), func() {
		ElegirImagen(func(src string) {
			aviso := func(clave string) {
				if c.Notificar != nil {
					c.Notificar(ui.AvisoAdvertencia, c.Traducir(clave))
				}
			}
			switch {
			case len(src) > MaxImagenMarca:
				aviso("watermark.image.tooLarge")
			case !rico.ImagenMarcaSegura(src):
				aviso("watermark.image.badFormat")
			default:
				p.cambiar(func(m *rico.MarcaAgua) { m.Imagen, m.Src, m.Tamano = true, src, 60 })
			}
		})
	})
	imagen.Glifo = iconos.TrazoEd(iconos.EdImagen, 14, "text", v)
	quitar := botonQuitarDecoracion(c.Traducir("watermark.remove"), func() {
		p.ponerTexto("")
		p.cambiar(func(m *rico.MarcaAgua) { m.Texto, m.Imagen, m.Src = "", false, "" })
	})
	objs = append(objs,
		filaAjuste(c.Traducir("watermark.size"), tamano),
		filaAjuste(c.Traducir("watermark.opacity"), opacidad),
		container.New(filaAcciones{}, imagen, quitar))
	p.cuerpo.Objects = objs
	p.cuerpo.Refresh()
}

// nuevoPanelBorde es `PageBorderPanel`.
func nuevoPanelBorde(c *editor.Contexto, f *Folio, cerrar func()) fyne.CanvasObject {
	b := rico.BordePagina{Estilo: "solid", Grosor: 1, Color: rico.ColorBorde}
	if actual := f.doc.BordeDe(); actual != nil {
		b = *actual
	}
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(1.5)})
	var pintar func()
	cambiar := func(nuevo rico.BordePagina) {
		b = nuevo
		c.Mandar(cmdBorde, valorBorde(rico.SanearBorde(b.Estilo, strconv.Itoa(b.Grosor), b.Color)))
		pintar()
	}
	pintar = func() {
		presets := make([]fyne.CanvasObject, 0, len(presetsBorde))
		for _, pr := range presetsBorde {
			presets = append(presets, botonPreset(c.Traducir(pr.clave), func() { cambiar(pr.b) }))
		}
		estilos := []editor.Opcion{
			{Valor: "solid", Rotulo: c.Traducir("border.style.solid")},
			{Valor: "dashed", Rotulo: c.Traducir("border.style.dashed")},
			{Valor: "dotted", Rotulo: c.Traducir("border.style.dotted")},
			{Valor: "double", Rotulo: c.Traducir("border.style.double")},
		}
		grosores := []editor.Opcion{
			{Valor: "1", Rotulo: c.Traducir("border.width.thin")},
			{Valor: "2", Rotulo: c.Traducir("border.width.medium")},
			{Valor: "3", Rotulo: c.Traducir("border.width.thick")},
			{Valor: "6", Rotulo: c.Traducir("border.width.extraThick")},
		}
		estilo := editor.NuevoSelectorCinta(c.Traducir("border.style.label"), AnchoSelectorBorde, estilos, b.Estilo,
			func(s string) { n := b; n.Estilo = s; cambiar(n) })
		grosor := editor.NuevoSelectorCinta(c.Traducir("border.width.label"), AnchoSelectorBorde, grosores, strconv.Itoa(b.Grosor),
			func(s string) { n := b; n.Grosor, _ = strconv.Atoi(s); cambiar(n) })
		color := editor.NuevaMuestraCinta(c, iconos.EdColorTexto, c.Traducir("border.color.label"), b.Color, editor.PaletaContenido,
			func(s string) { n := b; n.Color = s; cambiar(n) })
		quitar := botonQuitarDecoracion(c.Traducir("border.remove"), func() {
			c.Mandar(cmdBorde, "")
			cerrar()
		})
		cuerpo.Objects = []fyne.CanvasObject{
			ui.NuevoRotulo(c.Traducir("border.presets"), tema.TextoXS, ui.Medio, "text-muted"),
			container.New(&envuelta{hueco: tema.Espacio(1), ancho: AnchoPanelDecoracion}, presets...),
			filaAjuste(c.Traducir("border.style"), estilo),
			filaAjuste(c.Traducir("border.width"), grosor),
			filaAjuste(c.Traducir("watermark.color"), color),
			container.New(ui.Fila{}, quitar),
		}
		cuerpo.Refresh()
	}
	pintar()
	return container.New(anchoFijo{AnchoPanelDecoracion}, cuerpo)
}

// gruposDecoracion son los tres grupos nuevos de Disposición: marca de agua, borde y
// columnas. La marca y el borde son un grupo con un solo botón que abre su panel
// (`PopoverGroup`).
func (in *inicio) gruposDecoracion(c *editor.Contexto, f *Folio) (marca, borde, columnas fyne.CanvasObject) {
	bm := editor.NuevoBotonEmergente(iconos.EdMarcaAgua, c.Traducir("watermark.group"), func(func()) fyne.CanvasObject {
		return nuevoPanelMarca(c, f)
	})
	bb := editor.NuevoBotonEmergente(iconos.EdBordePagina, c.Traducir("border.group"), func(cerrar func()) fyne.CanvasObject {
		return nuevoPanelBorde(c, f, cerrar)
	})
	bm.Inerte, bb.Inerte = c.SoloLectura, c.SoloLectura
	opciones := []editor.Opcion{
		{Valor: "1", Rotulo: c.Traducir("layout.columns.one")},
		{Valor: "2", Rotulo: c.Traducir("layout.columns.two")},
		{Valor: "3", Rotulo: c.Traducir("layout.columns.three")},
	}
	if in.columnas == "" {
		in.columnas = "1"
	}
	sel := editor.NuevoSelectorCinta(c.Traducir("layout.columns.label"), AnchoSelectorColumnas, opciones, in.columnas,
		func(v string) { in.columnas = v; c.Mandar(cmdColumnas, v) })
	sel.Inerte = c.SoloLectura
	return editor.NuevoGrupo(c.Traducir("watermark.group"), bm), editor.NuevoGrupo(c.Traducir("border.group"), bb),
		editor.NuevoGrupo(c.Traducir("layout.columns"), sel)
}

// envuelta es `flex flex-wrap gap-N` en un ancho dado: los hijos a su medida, uno tras
// otro, bajando de línea cuando no caben.
type envuelta struct{ hueco, ancho float32 }

func (e *envuelta) lineas(objs []fyne.CanvasObject, ancho float32) (pos []fyne.Position, alto float32) {
	x, y, altoLinea := float32(0), float32(0), float32(0)
	for _, o := range objs {
		m := o.MinSize()
		if x > 0 && x+m.Width > ancho {
			x, y = 0, y+altoLinea+e.hueco
			altoLinea = 0
		}
		pos = append(pos, fyne.NewPos(x, y))
		x += m.Width + e.hueco
		altoLinea = max(altoLinea, m.Height)
	}
	return pos, y + altoLinea
}

func (e *envuelta) MinSize(objs []fyne.CanvasObject) fyne.Size {
	_, alto := e.lineas(objs, e.ancho)
	return fyne.NewSize(e.ancho, alto)
}

func (e *envuelta) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	pos, _ := e.lineas(objs, t.Width)
	for i, o := range objs {
		o.Move(pos[i])
		o.Resize(o.MinSize())
	}
}
