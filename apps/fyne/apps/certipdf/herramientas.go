// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"path"
	"regexp"
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

// EstadoOCR es la respuesta de `op: estado` del reconocimiento.
type EstadoOCR struct {
	PaginasSinTexto []int    `json:"paginas_sin_texto"`
	Total           int      `json:"total"`
	Disponible      bool     `json:"disponible"`
	Idiomas         []string `json:"idiomas"`
}

// InformeArchivo es la respuesta de `op: comprobar-archivo`.
type InformeArchivo struct {
	CumplePDFA bool `json:"cumple_pdfa"`
	Problemas  []struct {
		Codigo  string `json:"codigo"`
		Detalle string `json:"detalle,omitempty"`
	} `json:"problemas"`
}

// maxPorTanda es `maxPaginasPorOCR` del motor, repetido a propósito: sin él la interfaz
// calla y el corte se descubre cuando faltan páginas.
const maxPorTanda = 25

// dialogoHerramientas es `Herramientas`: comprimir, recortar, reconocer texto, exportar
// las páginas como imágenes y preparar para archivo.
type dialogoHerramientas struct {
	a        *dialogoAbierto
	nivel    string
	margenes *ui.Entrada
	// ocr es el estado del reconocimiento; nil mientras no llega o si no hay servicio.
	ocr         *EstadoOCR
	idiomas     string
	paginasOCR  *ui.Entrada
	formato     string
	exportando  bool
	errorImagen bool
	informe     *InformeArchivo
	idioma      *ui.Entrada
}

func nuevoDialogoHerramientas(a *dialogoAbierto) *dialogoHerramientas {
	d := &dialogoHerramientas{a: a, nivel: "equilibrada", idiomas: "spa+eng", formato: "png"}
	d.preguntarOCR()
	return d
}

var nivelesCompresion = []struct{ nivel, rotulo, ayuda string }{
	{"sin-perdida", "pdf.tools.lossless", "pdf.tools.level.lossless"},
	{"equilibrada", "pdf.tools.balanced", "pdf.tools.level.balanced"},
	{"fuerte", "pdf.tools.strong", "pdf.tools.level.strong"},
}

func (d *dialogoHerramientas) vista(ancho float32) fyne.CanvasObject {
	secciones := []fyne.CanvasObject{d.comprimir(ancho), d.recortar(ancho)}
	if s := d.reconocer(ancho); s != nil {
		secciones = append(secciones, s)
	}
	if s := d.exportarImagenes(ancho); s != nil {
		secciones = append(secciones, s)
	}
	secciones = append(secciones, d.archivo(ancho))
	return container.New(ui.Columna{Hueco: tema.Espacio(5)}, secciones...)
}

// seccion es `<section class="flex flex-col gap-2">` con su `<h3 class="text-sm
// font-medium">`; las que no son la primera llevan `border-t pt-4`.
func seccion(titulo string, conBorde bool, partes ...fyne.CanvasObject) fyne.CanvasObject {
	col := container.New(ui.Columna{Hueco: tema.Espacio(2)},
		append([]fyne.CanvasObject{ui.NuevoRotulo(titulo, tema.TextoSM, ui.Medio, "text")}, partes...)...)
	if !conBorde {
		return col
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return container.New(conLineaArriba{sep: tema.Espacio(4)}, canvas.NewRectangle(tema.Color("border", v)), col)
}

func (d *dialogoHerramientas) primario(clave string, f func()) *Accion {
	b := botonPrimario(d.a.t(clave), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), f)
	b.Inerte = d.a.p.Trabajando
	return b
}

func (d *dialogoHerramientas) secundario(clave string, f func()) *Accion {
	b := botonPanel(d.a.t(clave), tema.TextoXS, tema.Espacio(1.5), f)
	b.PX, b.Inerte = tema.Espacio(3), d.a.p.Trabajando
	return b
}

func (d *dialogoHerramientas) comprimir(ancho float32) fyne.CanvasObject {
	a := d.a
	opciones := make([]string, len(nivelesCompresion))
	elegido, ayuda := 1, "pdf.tools.level.balanced"
	for i, n := range nivelesCompresion {
		opciones[i] = a.t(n.rotulo)
		if n.nivel == d.nivel {
			elegido, ayuda = i, n.ayuda
		}
	}
	return seccion(a.t("pdf.tools.compress"), false,
		selectorDialogo(opciones, elegido, ancho, func(i int) { d.nivel = nivelesCompresion[i].nivel; a.rehacer() }),
		notaDialogo(a.t(ayuda), ancho),
		container.New(alPrincipio{}, d.primario("pdf.tools.compress.run", func() {
			a.aplicar(api.PDFCambio{Op: "comprimir", Campos: map[string]string{"nivel": d.nivel}, Sufijo: a.t("pdf.suffix.compressed")})
		})))
}

func (d *dialogoHerramientas) recortar(ancho float32) fyne.CanvasObject {
	a := d.a
	if d.margenes == nil {
		d.margenes = entradaDialogo("20,20,20,20", ancho)
		d.margenes.PonerTexto("20,20,20,20")
	}
	// El label real reparte este rótulo a32px de alto a278px. El Rotulo natural
	// pide290px y ensancha todas las secciones y sus selectores en el móvil.
	campo := conRotulo(a.t("pdf.tools.crop.margins"), d.margenes, notaDialogo(a.t("pdf.tools.crop.help"), ancho))
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	campo.Objects[0] = ui.NuevoParrafo(a.t("pdf.tools.crop.margins"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho)
	return seccion(a.t("pdf.tools.crop"), true,
		campo,
		container.New(alPrincipio{}, d.secundario("pdf.tools.crop.run", func() {
			a.aplicar(api.PDFCambio{Op: "recortar", Campos: map[string]string{"margenes": d.margenes.Texto()}, Sufijo: a.t("pdf.suffix.cropped")})
		})))
}

// ---- Reconocer ----

// preguntarOCR pregunta ANTES de enseñar la sección: puede que el servidor no tenga
// reconocimiento, o que el documento ya tenga texto en todas sus páginas; en los dos
// casos no se ofrece el botón.
func (d *dialogoHerramientas) preguntarOCR() {
	p := d.a.p
	if p.doc == nil || p.doc.pdf == nil {
		return
	}
	doc := p.doc
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		var e EstadoOCR
		err := doc.Leer(ctx, "/pdf/ocr", map[string]string{"op": "estado"}, &e)
		fyne.Do(func() {
			defer p.avisarRed()
			if err == nil {
				d.ocr = &e
				if d.a.p.dialogo == d.a {
					d.a.rehacer()
				}
			}
		})
	}()
}

// opcionesDeIdioma son las combinaciones que se ofrecen: cada idioma con el inglés (la
// mezcla real de un documento de empresa) y luego los sueltos, sin `osd`.
func opcionesDeIdioma(instalados []string) []string {
	var utiles, pares []string
	conIngles := false
	for _, l := range instalados {
		if l == "eng" {
			conIngles = true
		}
	}
	for _, l := range instalados {
		if l == "osd" {
			continue
		}
		utiles = append(utiles, l)
		if l != "eng" && conIngles {
			pares = append(pares, l+"+eng")
		}
	}
	return append(pares, utiles...)
}

// nombreDeIdioma es el del idioma en su propia lengua, que es como se reconoce.
func nombreDeIdioma(codigo string) string {
	nombres := map[string]string{"spa": "Español", "eng": "English", "fra": "Français", "deu": "Deutsch", "ita": "Italiano"}
	var r []string
	for _, c := range strings.Split(codigo, "+") {
		if n, ok := nombres[c]; ok {
			r = append(r, n)
		} else {
			r = append(r, c)
		}
	}
	return strings.Join(r, " + ")
}

func (d *dialogoHerramientas) reconocer(ancho float32) fyne.CanvasObject {
	a := d.a
	if d.ocr == nil || !d.ocr.Disponible {
		return nil
	}
	sinTexto := d.ocr.PaginasSinTexto
	if len(sinTexto) == 0 {
		return seccion(a.t("pdf.tools.ocr"), true, notaTenue(a.t("pdf.tools.ocr.alreadyText"), "text-muted", ancho))
	}
	partes := []fyne.CanvasObject{notaTenue(a.p.tv("pdf.tools.ocr.help", i18n.Vars{"n": strconv.Itoa(len(sinTexto))}), "text-muted", ancho)}
	if len(sinTexto) > maxPorTanda {
		partes = append(partes, avisoAmbar(a.p.tv("pdf.tools.ocr.batch", i18n.Vars{"max": strconv.Itoa(maxPorTanda)}), ancho))
	}
	if len(d.ocr.Idiomas) > 0 {
		opciones := opcionesDeIdioma(d.ocr.Idiomas)
		rotulos := make([]string, len(opciones))
		elegida := 0
		for i, o := range opciones {
			rotulos[i] = nombreDeIdioma(o)
			if o == d.idiomas {
				elegida = i
			}
		}
		partes = append(partes, conRotulo(a.t("pdf.tools.ocr.languages"),
			selectorDialogo(rotulos, elegida, ancho, func(i int) { d.idiomas = opciones[i] }),
			notaDialogo(a.t("pdf.tools.ocr.languagesHelp"), ancho)))
	}
	if d.paginasOCR == nil {
		d.paginasOCR = entradaDialogo(a.t("pdf.tools.ocr.pagesExample"), ancho)
	}
	partes = append(partes,
		conRotulo(a.t("pdf.tools.ocr.pages"), d.paginasOCR, notaDialogo(a.t("pdf.tools.ocr.pagesHelp"), ancho)),
		container.New(alPrincipio{}, d.primario("pdf.tools.ocr.run", func() { a.aplicar(d.cambioOCR()) })),
		notaDialogo(a.t("pdf.tools.ocr.note"), ancho))
	return seccion(a.t("pdf.tools.ocr"), true, partes...)
}

// cambioOCR es `reconocer-texto`: las páginas vacías NO se mandan, porque el motor
// distingue «no me dijeron nada» (y elige las que no tienen texto) de una lista vacía.
func (d *dialogoHerramientas) cambioOCR() api.PDFCambio {
	campos := map[string]string{"idiomas": d.idiomas}
	if d.paginasOCR != nil {
		if p := strings.TrimSpace(d.paginasOCR.Texto()); p != "" {
			campos["paginas"] = p
		}
	}
	return api.PDFCambio{Op: "reconocer-texto", Campos: campos, Sufijo: d.a.t("pdf.suffix.recognised")}
}

// notaTenue es un `<p class="text-2xs">` con su tinta.
func notaTenue(texto, token string, ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, token, v).A(ancho)
}

// ---- Exportar imágenes ----

// paginasListas son las ya dibujadas: se exportan esas, sin pedir el resto, y se dice
// «N de M» para que nadie se sorprenda.
func (d *dialogoHerramientas) paginasListas() []int {
	var r []int
	for _, pg := range d.a.p.Contenido.Paginas {
		if d.a.p.imagenes[pg.Numero] != nil {
			r = append(r, pg.Numero)
		}
	}
	return r
}

func (d *dialogoHerramientas) exportarImagenes(ancho float32) fyne.CanvasObject {
	a := d.a
	listas := d.paginasListas()
	if len(listas) == 0 {
		return nil
	}
	formatos := []string{"png", "jpeg"}
	elegido := 0
	if d.formato == "jpeg" {
		elegido = 1
	}
	s := selectorDialogo([]string{"PNG", "JPEG"}, elegido, 0, func(i int) { d.formato = formatos[i] })
	exportar := d.secundario("pdf.tools.images.run", d.exportar)
	exportar.Inerte = d.exportando
	partes := []fyne.CanvasObject{
		notaTenue(a.p.tv("pdf.tools.images.help", i18n.Vars{"n": strconv.Itoa(len(listas)), "total": strconv.Itoa(len(a.p.Contenido.Paginas))}), "text-muted", ancho),
		container.New(alPrincipio{}, s),
		container.New(alPrincipio{}, exportar),
	}
	if d.errorImagen {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		partes = append(partes, ui.NuevoParrafo(a.t("pdf.tools.images.error"), tema.TextoXS, ui.Normal, "danger-text", v).A(ancho))
	}
	return seccion(a.t("pdf.tools.images"), true, partes...)
}

// baseDeNombre es el nombre del fichero sin ruta ni «.pdf».
func baseDeNombre(ruta string) string {
	b := regexp.MustCompile(`(?i)\.pdf$`).ReplaceAllString(path.Base(ruta), "")
	if b == "" || b == "." || b == "/" {
		return "documento"
	}
	return b
}

// nombreDePagina es `nombreDePagina`: la base, el número con tantos ceros como cifras
// tiene el total, y la extensión (la del formato: `jpeg`, como la web).
func nombreDePagina(base string, n, total int, ext string) string {
	return fmt.Sprintf("%s-%0*d.%s", base, len(strconv.Itoa(total)), n, ext)
}

// escalaDeImagen es la de exportar: 2, bajada si la página pasaría de 40 megapíxeles.
func escalaDeImagen(ancho, alto float64) float64 {
	e := 2.0
	if m := math.Sqrt(40e6 / (ancho * alto)); e > m {
		e = math.Max(1, m)
	}
	return e
}

// exportar pide al motor cada página lista a escala 2 y la deja en descargas.
func (d *dialogoHerramientas) exportar() {
	p := d.a.p
	if d.exportando || p.doc == nil {
		return
	}
	d.exportando, d.errorImagen = true, false
	d.a.rehacer()
	doc, formato, total := p.doc, d.formato, len(p.Contenido.Paginas)
	paginas := map[int]Pagina{}
	for _, pg := range p.Contenido.Paginas {
		paginas[pg.Numero] = pg
	}
	listas, base := d.paginasListas(), baseDeNombre(doc.Ruta())
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		err := func() error {
			carpeta, err := ficheros.Carpeta()
			if err != nil {
				return err
			}
			for _, n := range listas {
				pg := paginas[n]
				imgs, err := doc.Imagenes(ctx, n, n, escalaDeImagen(pg.Ancho, pg.Alto))
				if err != nil || imgs[n] == nil {
					return fmt.Errorf("página %d: %w", n, err)
				}
				datos, err := codificar(imgs[n], formato)
				if err != nil {
					return err
				}
				if _, err := ficheros.Guardar(carpeta, nombreDePagina(base, n, total, formato), bytes.NewReader(datos)); err != nil {
					return err
				}
			}
			return nil
		}()
		fyne.Do(func() {
			defer p.avisarRed()
			d.exportando, d.errorImagen = false, err != nil
			d.a.rehacer()
		})
	}()
}

// codificar es el `canvas.toBlob` de la web: PNG, o JPEG con calidad 0,92.
func codificar(img image.Image, formato string) ([]byte, error) {
	var b bytes.Buffer
	var err error
	if formato == "jpeg" {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 92})
	} else {
		err = png.Encode(&b, img)
	}
	return b.Bytes(), err
}

// ---- Archivo ----

// claveDeProblema traduce los incumplimientos de PDF/A del motor.
func claveDeProblema(codigo string) string {
	conocidos := map[string]string{
		"cifrado": "pdf.archive.encrypted", "fuenteSinIncrustar": "pdf.archive.fontNotEmbedded",
		"conJavaScript": "pdf.archive.javascript", "conAdjuntos": "pdf.archive.attachments",
		"conAccionDeApertura": "pdf.archive.openAction", "sinMetadatosXMP": "pdf.archive.noXMP",
		"sinEstructura": "pdf.archive.noStructure", "sinIdioma": "pdf.archive.noLang",
		"imagenSinAlternativo": "pdf.archive.noAltText",
	}
	if k, ok := conocidos[codigo]; ok {
		return k
	}
	return "pdf.archive.other"
}

func (d *dialogoHerramientas) archivo(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	partes := []fyne.CanvasObject{
		notaTenue(a.t("pdf.tools.archive.help"), "text-muted", ancho),
		container.New(alPrincipio{}, d.secundario("pdf.tools.archive.check", d.comprobar)),
	}
	if inf := d.informe; inf != nil {
		clave, tinta := "pdf.tools.archive.notYet", "warning-text"
		if inf.CumplePDFA {
			clave, tinta = "pdf.tools.archive.ok", "success-text"
		}
		lineas := []fyne.CanvasObject{ui.NuevoParrafo(a.t(clave), tema.TextoXS, ui.Normal, tinta, v).A(ancho)}
		for _, pr := range inf.Problemas {
			texto := "• " + a.t(claveDeProblema(pr.Codigo))
			if pr.Detalle != "" {
				texto += " (" + pr.Detalle + ")"
			}
			lineas = append(lineas, notaTenue(texto, "text-subtle", ancho))
		}
		if inf.CumplePDFA {
			declarar := d.primario("pdf.tools.archive.run", func() {
				a.aplicar(api.PDFCambio{Op: "archivo", Campos: map[string]string{"nivel": "2B", "idioma": d.idioma.Texto()}, Sufijo: a.t("pdf.suffix.archived")})
			})
			if d.idioma == nil {
				d.idioma = nuevoCampoPanel("", tema.Espacio(1), tema.RadioMD, 0)
				d.idioma.PonerTexto("es-ES")
			}
			d.idioma.Ancho = ancho - tema.Espacio(2) - declarar.MinSize().Width
			// `mt-1 flex items-end gap-2`: el campo con su rótulo y el botón abajo.
			campo := container.New(ui.Columna{Hueco: tema.Espacio(1)}, ui.NuevoRotulo(a.t("pdf.tools.archive.lang"), tema.Texto2XS, ui.Normal, "text-muted"), d.idioma)
			alto := campo.MinSize().Height
			lineas = append(lineas, container.New(ui.Margen{Arr: tema.Espacio(1)}, container.New(ui.Fila{Hueco: tema.Espacio(2)},
				campo, container.New(ui.Margen{Arr: alto - declarar.MinSize().Height}, declarar))))
		}
		partes = append(partes, container.New(ui.Columna{Hueco: tema.Espacio(1)}, lineas...))
	}
	return seccion(a.t("pdf.tools.archive"), true, partes...)
}

// comprobar pregunta si el documento se puede declarar PDF/A; un fallo no dice nada,
// como en la web.
func (d *dialogoHerramientas) comprobar() {
	p := d.a.p
	if p.doc == nil {
		return
	}
	doc := p.doc
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		var inf InformeArchivo
		err := doc.Leer(ctx, "/pdf/tools", map[string]string{"op": "comprobar-archivo"}, &inf)
		fyne.Do(func() {
			defer p.avisarRed()
			d.informe = nil
			if err == nil {
				d.informe = &inf
			}
			d.a.rehacer()
		})
	}()
}
