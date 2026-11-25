// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El texto fuera del PDF (`textoDelPdf`, `resumenDeComentarios` y «Copiar el texto de la
// página» de la web): el del documento entero, el de la página que se mira y el resumen
// de los comentarios, con los mismos vectores
// (`packages/office/test/vectores/certipdf-herramientas.json`).

// TextoDelPDF es `textoDelPdf`: una línea por renglón y una en blanco entre páginas; las
// que no tienen texto quedan vacías, para que cada página siga siendo la misma.
func TextoDelPDF(paginas [][]Fragmento) string {
	if len(paginas) == 0 {
		return ""
	}
	bloques := make([]string, len(paginas))
	for i, f := range paginas {
		bloques[i] = strings.Join(LineasDeTexto(f), "\n")
	}
	return strings.Join(bloques, "\n\n") + "\n"
}

// RotulosResumen son los rótulos del resumen, ya traducidos.
type RotulosResumen struct {
	Pagina  func(n int) string
	Tipo    func(tipo string) string
	Anonimo string
}

var blancoConSalto = regexp.MustCompile(`\s*\n\s*`)

// ResumenDeComentarios es `resumenDeComentarios`: por página, cada anotación con texto, su
// tipo y su autor, y debajo sus respuestas sangradas. Vacío si no hay ninguna con texto.
func ResumenDeComentarios(anotaciones []AnotacionLeida, r RotulosResumen) string {
	conTexto := func(a AnotacionLeida) bool { return strings.TrimSpace(a.Texto) != "" }
	linea := func(a AnotacionLeida) string {
		autor := strings.TrimSpace(a.Autor)
		if autor == "" {
			autor = r.Anonimo
		}
		return r.Tipo(a.Tipo) + " · " + autor + ": " + blancoConSalto.ReplaceAllString(strings.TrimSpace(a.Texto), " ")
	}
	porPagina := map[int][]string{}
	for i, a := range anotaciones {
		if a.RespuestaA != nil || !conTexto(a) {
			continue
		}
		porPagina[a.Pagina] = append(porPagina[a.Pagina], linea(a))
		for _, b := range anotaciones {
			if b.RespuestaA != nil && *b.RespuestaA == i && conTexto(b) {
				porPagina[a.Pagina] = append(porPagina[a.Pagina], "  ↳ "+linea(b))
			}
		}
	}
	paginas := make([]int, 0, len(porPagina))
	for p := range porPagina {
		paginas = append(paginas, p)
	}
	sort.Ints(paginas)
	bloques := make([]string, len(paginas))
	for i, p := range paginas {
		bloques[i] = strings.Join(append([]string{r.Pagina(p + 1)}, porPagina[p]...), "\n")
	}
	if len(bloques) == 0 {
		return ""
	}
	return strings.Join(bloques, "\n\n") + "\n"
}

// nombreTxt es el del PDF con `.txt` (y el sufijo que se le pida).
func (p *Pantalla) nombreTxt(sufijo string) string {
	nombre := ""
	if p.doc != nil {
		f, _ := p.doc.paraElMotor()
		nombre = f.Nombre
		if nombre == "" {
			nombre = p.doc.Ruta()
		}
	}
	nombre = strings.TrimSuffix(nombreDeRuta(nombre), filepath.Ext(nombreDeRuta(nombre)))
	if nombre == "" {
		nombre = "documento"
	}
	return nombre + sufijo + ".txt"
}

// guardarTexto deja `texto` en las descargas con los avisos de «Descargar».
func (p *Pantalla) guardarTexto(nombre, texto string) {
	go func() {
		ruta, err := guardarEnDescargas(nombre, []byte(texto))
		fyne.Do(func() {
			defer p.avisarRed()
			if err != nil {
				p.Aviso, p.Error = "", p.t("pdf.download.failed")
			} else {
				p.Error, p.Aviso = "", p.tv("pdf.download.done", i18n.Vars{"name": filepath.Base(ruta)})
			}
			p.Recomponer()
		})
	}()
}

// descargarTexto es «Descargar el texto (.txt)».
func (p *Pantalla) descargarTexto() {
	if p.Contenido == nil {
		return
	}
	p.cargarTexto(func(paginas [][]Fragmento) {
		if paginas == nil {
			p.Error, p.Aviso = "", p.t("pdf.ai.noText")
			p.Recomponer()
			return
		}
		p.guardarTexto(p.nombreTxt(""), TextoDelPDF(paginas))
	})
}

// copiarPagina es «Copiar el texto de la página»: el de la que se está mirando.
func (p *Pantalla) copiarPagina() {
	if p.Contenido == nil {
		return
	}
	p.cargarTexto(func(paginas [][]Fragmento) {
		var texto string
		if i := p.PaginaActual - 1; i >= 0 && i < len(paginas) {
			texto = strings.Join(LineasDeTexto(paginas[i]), "\n")
		}
		if texto == "" {
			p.Error, p.Aviso = "", p.t("pdf.page.noText")
		} else {
			fyne.CurrentApp().Clipboard().SetContent(texto)
			p.Error, p.Aviso = "", p.t("pdf.text.copied")
		}
		p.Recomponer()
	})
}

// descargarResumen es «Resumen de comentarios (.txt)».
func (p *Pantalla) descargarResumen() {
	if p.Contenido == nil {
		return
	}
	resumen := ResumenDeComentarios(p.Contenido.Anotaciones, RotulosResumen{
		Pagina:  func(n int) string { return p.tv("pdf.page.n", i18n.Vars{"n": n}) },
		Tipo:    func(k string) string { return p.t("pdf.kind." + k) },
		Anonimo: p.t("pdf.comments.anonymous"),
	})
	if resumen == "" {
		p.Error, p.Aviso = "", p.t("pdf.comments.empty")
		p.Recomponer()
		return
	}
	p.guardarTexto(p.nombreTxt(" - "+p.t("pdf.panel.comments")), resumen)
}

// pedirPagina es Ctrl+G, «Ir a una página»: el número sin buscar el campo de la cinta.
func (p *Pantalla) pedirPagina() {
	if p.Contenido == nil {
		return
	}
	p.preguntar(p.t("pdf.shortcuts.goTo"), strconv.Itoa(p.PaginaActual), true, func(v string) {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			p.IrA(n)
		}
	})
}
