// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"math"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// FragmentoEditable es un trozo de texto de la página que se puede reescribir en su
// sitio, con el ancho que tiene para hacerlo.
type FragmentoEditable struct {
	ID              int     `json:"id"`
	Pagina          int     `json:"pagina"`
	Texto           string  `json:"texto"`
	AnchoDisponible float64 `json:"ancho_disponible"`
	// Subconjunto es que la fuente solo lleva las letras que ya usaba.
	Subconjunto bool `json:"subconjunto"`
	// LetrasDesconocidas es que la fuente no dice qué letras usa: se borra, no se reescribe.
	LetrasDesconocidas bool `json:"letras_desconocidas,omitempty"`
}

// ImagenEditable es una imagen de la página que se puede sustituir o quitar.
type ImagenEditable struct {
	ID          string `json:"id"`
	Pagina      int    `json:"pagina"`
	Ancho       int    `json:"ancho"`
	Alto        int    `json:"alto"`
	Formato     string `json:"formato"`
	TamanoBytes int64  `json:"tamano_bytes"`
}

// CapaDocumento es un grupo de contenido opcional (OCG).
type CapaDocumento struct {
	ID      int    `json:"id"`
	Nombre  string `json:"nombre"`
	Visible bool   `json:"visible"`
}

// AnalisisEdicion es lo que `op: analizar` dice que se puede editar en una página.
type AnalisisEdicion struct {
	Fragmentos []FragmentoEditable `json:"fragmentos"`
	Imagenes   []ImagenEditable    `json:"imagenes"`
	Capas      []CapaDocumento     `json:"capas"`
	Avisos     []string            `json:"avisos"`
}

// panelEditar es `Editar` de `Paneles.tsx`: lo editable de la página que se mira.
type panelEditar struct {
	l *PanelLateral
	// de y pagina son el documento y la página analizados: si cambian, se vuelve a
	// analizar. La web solo vuelve a mirar al cambiar de página; aquí también al
	// cambiar el documento, porque unos fragmentos de la versión anterior podrían no
	// existir ya.
	de       *Contenido
	pagina   int
	cargando bool
	analisis *AnalisisEdicion
	textos   map[int]string
	vista    *fyne.Container
}

func (l *PanelLateral) editar() fyne.CanvasObject {
	if l.editarP == nil {
		l.editarP = &panelEditar{l: l, vista: container.New(ui.Columna{})}
	}
	e := l.editarP
	if e.de != l.c || e.pagina != l.PaginaActual {
		e.analizar()
	}
	e.rehacer()
	return e.vista
}

func (e *panelEditar) t(clave string) string { return e.l.t(clave, nil) }

// alCambiarPagina vuelve a analizar si el panel está a la vista.
func (e *panelEditar) alCambiarPagina() {
	if e.pagina != e.l.PaginaActual {
		e.analizar()
		e.rehacer()
	}
}

// analizar pide `op: analizar` de la página que se mira. Una respuesta que llega
// cuando ya se mira otra se tira (el `vivo` de la web).
func (e *panelEditar) analizar() {
	e.de, e.pagina, e.cargando, e.analisis = e.l.c, e.l.PaginaActual, true, nil
	p := e.l.pant
	if p == nil || p.doc == nil {
		e.cargando = false
		return
	}
	doc, pagina, de := p.doc, e.pagina, e.de
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		var a AnalisisEdicion
		err := doc.Leer(ctx, "/pdf/edit", map[string]string{"op": "analizar", "pagina": strconv.Itoa(pagina)}, &a)
		fyne.Do(func() {
			defer p.avisarRed()
			if e.pagina != pagina || e.de != de {
				return
			}
			e.cargando = false
			if err == nil {
				e.analisis = &a
				e.textos = map[int]string{}
				for _, f := range a.Fragmentos {
					e.textos[f.ID] = f.Texto
				}
			}
			if e.l.on == PanelEditar {
				e.rehacer()
			}
		})
	}()
}

func (e *panelEditar) rehacer() {
	e.vista.Objects = []fyne.CanvasObject{e.cuerpo()}
	e.vista.Refresh()
}

// cuerpo es el `<div class="flex flex-col gap-3">`, o el párrafo de «mirando» o de
// «no se pudo».
func (e *panelEditar) cuerpo() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := anchoUtil()
	if e.cargando || e.analisis == nil {
		clave := "pdf.edit.failed"
		if e.cargando {
			clave = "pdf.edit.loading"
		}
		m := tema.Espacio(3)
		return container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m},
			ui.NuevoParrafo(e.t(clave), tema.TextoXS, ui.Normal, "text-subtle", v).A(ancho-2*m))
	}
	a := e.analisis
	var partes []fyne.CanvasObject
	for _, aviso := range a.Avisos {
		partes = append(partes, avisoAmbar(e.t(claveDeAviso(aviso)), ancho))
	}
	if len(a.Capas) > 0 {
		partes = append(partes, e.capas(ancho))
	}
	if len(a.Imagenes) > 0 {
		partes = append(partes, e.imagenes(ancho))
	}
	if len(a.Fragmentos) == 0 {
		partes = append(partes, ui.NuevoParrafo(e.t("pdf.edit.noText"), tema.TextoXS, ui.Normal, "text-subtle", v).A(ancho))
	} else {
		partes = append(partes, e.formulario(ancho))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// claveDeAviso traduce los códigos de aviso del motor.
func claveDeAviso(codigo string) string {
	switch codigo {
	case "avisoSubconjunto":
		return "pdf.edit.warn.subset"
	case "avisoSinTexto":
		return "pdf.edit.warn.noText"
	case "avisoImagenSinPintar":
		return "pdf.edit.warn.imageFormat"
	}
	return "pdf.edit.warn.noReflow"
}

// capas es el `<fieldset>` de las capas: una casilla por capa, y marcarla o desmarcarla
// reescribe cuáles se ven (`op: capas`, sobre el mismo fichero).
func (e *panelEditar) capas(ancho float32) fyne.CanvasObject {
	capas := e.analisis.Capas
	filas := make([]fyne.CanvasObject, 0, len(capas))
	for _, c := range capas {
		nombre := c.Nombre
		if nombre == "" {
			nombre = e.t("pdf.edit.layerUnnamed")
		}
		id := c.ID
		casilla := nuevaCasillaNativa(nombre, c.Visible, func(marcado bool) {
			var visibles []string
			for _, x := range capas {
				if x.ID == id && marcado || x.ID != id && x.Visible {
					visibles = append(visibles, strconv.Itoa(x.ID))
				}
			}
			if p := e.l.pant; p != nil {
				p.Aplicar(api.PDFCambio{Op: "capas", Campos: map[string]string{"visibles": strings.Join(visibles, ",")}},
					e.t("pdf.saved.annotated"), nil)
			}
		})
		casilla.esc, casilla.hueco, casilla.tinta = tema.TextoXS, tema.Espacio(2), "text"
		casilla.Inerte = e.l.trabajando()
		// `py-0.5`.
		filas = append(filas, container.New(ui.Margen{Arr: tema.Espacio(0.5), Abj: tema.Espacio(0.5)}, container.New(alPrincipio{}, casilla)))
	}
	return nuevaLeyenda(e.t("pdf.edit.layers"), container.New(ui.Columna{}, filas...), ancho)
}

// imagenes es `ImagenesDeLaPagina`: cada imagen con su ficha y «Sustituir…» y «Quitar».
// Quitar es sustituir SIN fichero; son dos botones porque son dos intenciones.
func (e *panelEditar) imagenes(ancho float32) fyne.CanvasObject {
	dentro := ancho - 2*tema.Borde - 2*tema.Espacio(2)
	filas := make([]fyne.CanvasObject, 0, len(e.analisis.Imagenes))
	for _, im := range e.analisis.Imagenes {
		sustituir := botonPanel(e.t("pdf.edit.imageReplace"), tema.Texto2XS, tema.Espacio(1), func() { e.sustituir(im) })
		quitar := botonPanel(e.t("pdf.edit.imageRemove"), tema.Texto2XS, tema.Espacio(1), func() { e.quitar(im) })
		quitar.Tinta = "danger-text"
		sustituir.Inerte, quitar.Inerte = e.l.trabajando(), e.l.trabajando()
		botones := container.New(ui.Fila{Hueco: tema.Espacio(1)}, sustituir, quitar)
		ficha := e.l.t("pdf.edit.imageInfo", i18n.Vars{
			"w": strconv.Itoa(im.Ancho), "h": strconv.Itoa(im.Alto), "formato": strings.ToUpper(im.Formato),
			"size": i18n.FormatBytes(e.l.idioma(), float64(im.TamanoBytes)),
		})
		cabe := dentro - botones.MinSize().Width - tema.Espacio(2)
		filas = append(filas, container.New(aLosLados{},
			rotuloRecortado(ficha, tema.Texto2XS, ui.Normal, "text-subtle", cabe), botones))
	}
	return nuevaLeyenda(e.t("pdf.edit.images"), container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, filas...), ancho)
}

// extensionesPNGyJPEG es el `accept="image/png,image/jpeg"` de la web.
var extensionesPNGyJPEG = []string{"png", "jpg", "jpeg"}

// sustituir pide la imagen nueva con el selector del sistema —PNG y JPEG, el `accept`
// de la web— y la pone en su sitio, en una copia.
func (e *panelEditar) sustituir(im ImagenEditable) {
	p := e.l.pant
	if p == nil {
		return
	}
	titulo := e.t("pdf.edit.imageReplace")
	go func() {
		rutas, err := ficheros.ElegirConTipos(titulo, extensionesPNGyJPEG...)
		if err != nil || len(rutas) == 0 {
			return
		}
		datos, err := os.ReadFile(rutas[0])
		if err != nil {
			return
		}
		fyne.Do(func() {
			p.Aplicar(api.PDFCambio{
				Op:     "editar-imagen",
				Campos: map[string]string{"pagina": strconv.Itoa(im.Pagina + 1), "id": im.ID},
				Partes: []api.PDFParte{{Campo: "imagen", Nombre: "imagen", Datos: datos}},
				Sufijo: e.t("pdf.suffix.edited"),
			}, e.t("pdf.saved.imageReplaced"), nil)
		})
	}()
}

// quitar manda la sustitución sin imagen, que el motor entiende como quitarla. Deja
// copia, como toda la edición de contenido; se pregunta antes.
func (e *panelEditar) quitar(im ImagenEditable) {
	p := e.l.pant
	if p == nil {
		return
	}
	p.confirmarCon(e.t("pdf.edit.imageRemove.title"), e.t("pdf.edit.imageRemove.confirm"), e.t("pdf.edit.imageRemove"), func() {
		p.Aplicar(api.PDFCambio{
			Op:     "editar-imagen",
			Campos: map[string]string{"pagina": strconv.Itoa(im.Pagina + 1), "id": im.ID},
			Sufijo: e.t("pdf.suffix.edited"),
		}, e.t("pdf.saved.imageRemoved"), nil)
	})
}

// formulario es el `<form class="flex flex-col gap-2">` de los fragmentos: cuánto cabe
// (y si la fuente está incompleta) encima de cada campo, y «Aplicar los cambios».
func (e *panelEditar) formulario(ancho float32) fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(e.analisis.Fragmentos)+1)
	for _, f := range e.analisis.Fragmentos {
		cabe := ui.NuevoRotulo(e.l.t("pdf.edit.fits", i18n.Vars{"pt": strconv.Itoa(int(math.Round(f.AnchoDisponible)))}),
			tema.Texto2XS, ui.Normal, "text-muted")
		var derecha fyne.CanvasObject = container.New(ui.Fila{})
		switch {
		case f.LetrasDesconocidas:
			derecha = ui.NuevoRotulo(e.t("pdf.edit.unknownLettersShort"), tema.Texto2XS, ui.Normal, "warning-text")
		case f.Subconjunto:
			derecha = ui.NuevoRotulo(e.t("pdf.edit.subsetShort"), tema.Texto2XS, ui.Normal, "warning-text")
		}
		campo := nuevoCampoPanel("", tema.Espacio(1), tema.RadioMD, ancho)
		campo.PonerTexto(e.textos[f.ID])
		id := f.ID
		campo.AlCambiar(func(s string) { e.textos[id] = s })
		filas = append(filas, container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, container.New(aLosLados{}, cabe, derecha), campo))
	}
	aplicar := botonPrimario(e.t("pdf.edit.save"), tema.TextoXS, tema.Espacio(2), tema.Espacio(1.5), e.guardar)
	aplicar.Ancho, aplicar.Inerte = ancho, e.l.trabajando()
	filas = append(filas, aplicar)
	forma := container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...)
	// El `<form onSubmit>`: Intro en un fragmento guarda los cambios.
	conIntro(forma, func() {
		if !e.l.trabajando() {
			e.guardar()
		}
	})
	return forma
}

// guardar manda SOLO los fragmentos que cambiaron (`op: editar-texto`, siempre en copia).
func (e *panelEditar) guardar() {
	cambios := fragmentosCambiados(e.analisis.Fragmentos, e.textos)
	if len(cambios) == 0 || e.l.pant == nil {
		return
	}
	e.l.pant.Aplicar(api.PDFCambio{Op: "editar-texto", Datos: cambios, Sufijo: e.t("pdf.suffix.edited")}, "", nil)
}

// cambioDeTexto es una fila de `op: editar-texto`.
type cambioDeTexto struct {
	Pagina int    `json:"pagina"`
	ID     int    `json:"id"`
	Nuevo  string `json:"nuevo"`
}

// fragmentosCambiados son los fragmentos cuyo texto escrito difiere del de la página.
func fragmentosCambiados(fs []FragmentoEditable, textos map[int]string) []cambioDeTexto {
	var r []cambioDeTexto
	for _, f := range fs {
		if t, ok := textos[f.ID]; ok && t != f.Texto {
			r = append(r, cambioDeTexto{Pagina: f.Pagina, ID: f.ID, Nuevo: t})
		}
	}
	return r
}

// ---- El `<fieldset>` con su `<legend>` ----

// leyenda es `<fieldset class="border-border rounded-md border p-2">` con `<legend
// class="text-text-muted text-2xs px-1">`: la leyenda se sienta SOBRE el borde de
// arriba, que va a media altura de su línea y se corta donde está ella. Medido en
// Chromium a rem16: borde en7,5, leyenda en9 (con px-1) y contenido24 px más abajo
// (línea16 y p-2 de8).
type leyenda struct {
	caja   *ui.Caja
	hueco  *canvas.Rectangle
	rotulo *ui.Rotulo
	dentro fyne.CanvasObject
}

func nuevaLeyenda(texto string, dentro fyne.CanvasObject, ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l := &leyenda{
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD),
		hueco:  canvas.NewRectangle(tema.Color("surface", v)),
		rotulo: ui.NuevoRotulo(texto, tema.Texto2XS, ui.Normal, "text-muted"),
		dentro: dentro,
	}
	objs := append(l.caja.Objetos(), l.hueco, l.rotulo, dentro)
	return container.New(anchoFijo{ancho: ancho}, container.New(l, objs...))
}

// Las medidas del `<fieldset>`: el borde a media línea de la leyenda y la leyenda a
// `p-2` del borde izquierdo.
var (
	bordeLeyenda = (tema.Texto2XS.Linea - tema.Borde) / 2
	xLeyenda     = tema.Borde + tema.Espacio(2)
)

func (l *leyenda) MinSize([]fyne.CanvasObject) fyne.Size {
	m := l.dentro.MinSize()
	p := tema.Espacio(2)
	return fyne.NewSize(m.Width+2*p+2*tema.Borde, tema.Texto2XS.Linea+2*p+m.Height+tema.Borde)
}

func (l *leyenda) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	l.caja.Redimensionar(fyne.NewSize(t.Width, t.Height-bordeLeyenda))
	// Redimensionar coloca en (0,0): se baja la caja entera a la altura del borde.
	caja := l.caja.Objetos()
	caja[0].Move(fyne.NewPos(0, bordeLeyenda))
	caja[1].Move(fyne.NewPos(tema.Borde, bordeLeyenda+tema.Borde))
	r := l.rotulo.MinSize()
	px := tema.Espacio(1)
	l.hueco.Move(fyne.NewPos(xLeyenda, bordeLeyenda))
	l.hueco.Resize(fyne.NewSize(r.Width+2*px, tema.Borde))
	l.rotulo.Move(fyne.NewPos(xLeyenda+px, 0))
	l.rotulo.Resize(r)
	p := tema.Espacio(2)
	l.dentro.Move(fyne.NewPos(tema.Borde+p, tema.Texto2XS.Linea+p))
	l.dentro.Resize(fyne.NewSize(t.Width-2*tema.Borde-2*p, t.Height-tema.Texto2XS.Linea-2*p-tema.Borde))
}
