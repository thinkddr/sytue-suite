// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pdfdoc"
)

// PDF accesible (PDF/UA-1): el exportador de Docus sobre platform/pkg/pdfdoc. A diferencia del
// mínimo de export_pdf.go (catorce fuentes estándar, tablas aplanadas, sin imágenes), este
// escribe fuentes incrustadas, estructura lógica real (encabezados, listas, tablas con TH y
// Scope, figuras con /Alt, enlaces) y metadatos. La estructura no se adivina: sale del modelo
// que ya parsea el exportador a DOCX/ODT, así que el orden de lectura es el del documento.

// InformePDF resume lo que el exportador no pudo llevar al PDF, para avisar a quien descarga.
type InformePDF struct {
	ImagenesSinAlt   int          // imágenes escritas como artefacto por no tener texto alternativo
	Sustituidos      map[rune]int // caracteres sin glifo en ninguna fuente (escrituras no europeas)
	ImagenesPerdidas int          // imágenes que no se pudieron incrustar (formato, tamaño o URL externa)
}

// ExportPDFAccesible genera el PDF/UA de un documento, hoja o presentación. idioma es el BCP 47
// del documento («es», «el»…); vacío = «es».
func ExportPDFAccesible(kind Kind, title, idioma string, s *State) ([]byte, InformePDF, error) {
	d := pdfdoc.Nuevo(pdfdoc.Opciones{Titulo: title, Idioma: idioma, Fecha: time.Now()})
	var inf InformePDF
	switch kind {
	case KindDocument:
		if title != "" {
			d.Parrafo(pdfdoc.Parrafo{Rol: "H1", Fragmentos: []pdfdoc.Fragmento{{Texto: title, Negrita: true}}, Tam: 22, Despues: 8})
		}
		uaBloques(d, documentBlocksPDF(s), &inf)
	case KindSpreadsheet:
		uaHoja(d, title, s)
	case KindPresentation:
		uaDiapositivas(d, title, s, &inf)
	default:
		return nil, inf, ErrInvalidKind
	}
	out, err := d.Bytes()
	inf.ImagenesSinAlt, inf.Sustituidos = d.SinAlt(), d.Avisos()
	return out, inf, err
}

// documentBlocksPDF es documentBlocks con las imágenes como imágenes.
func documentBlocksPDF(s *State) []xblock {
	if h := strings.TrimSpace(s.documentHTML()); h != "" {
		if blk := parseHTMLDocCon(h, false, true).blocks; len(blk) > 0 {
			return blk
		}
	}
	return documentBlocks(KindDocument, s)
}

func uaBloques(d *pdfdoc.Doc, blocks []xblock, inf *InformePDF) {
	for _, blk := range blocks {
		switch {
		case blk.para != nil:
			uaParrafo(d, *blk.para, inf)
		case blk.table != nil:
			d.Tabla(uaTabla(d, blk.table, inf))
		case blk.hr:
			d.Regla()
		}
	}
}

var tamEncabezado = [...]float64{0, 20, 16, 14, 12, 12, 12}

func uaAlineacion(a string) pdfdoc.Alineacion {
	switch a {
	case "center":
		return pdfdoc.Centro
	case "right":
		return pdfdoc.Derecha
	default: // justify: pdfdoc no justifica (Identity-H no admite Tw), se alinea a la izquierda
		return pdfdoc.Izquierda
	}
}

// uaPartes convierte un párrafo en bloques: el texto va en Parrafos y cada imagen incrustada
// corta el párrafo y va como Imagen (una imagen en línea ocupa su propia línea).
func uaPartes(p xpara, inf *InformePDF) []pdfdoc.Bloque {
	var out []pdfdoc.Bloque
	var frs []pdfdoc.Fragmento
	tam := 11.0
	rol := ""
	if n, ok := strings.CutPrefix(p.style, "Heading"); ok {
		if lvl, err := strconv.Atoi(n); err == nil && lvl >= 1 && lvl <= 6 {
			rol, tam = "H"+n, tamEncabezado[lvl]
		}
	}
	base := pdfdoc.Parrafo{Rol: rol, Tam: tam, Alinear: uaAlineacion(p.align), Cita: p.style == "Quote", Despues: 4}
	if rol == "" {
		base.Despues = 6
	}
	vuelca := func() {
		if len(frs) > 0 {
			pp := base
			pp.Fragmentos = frs
			out = append(out, pdfdoc.Bloque{Parrafo: &pp})
			frs = nil
		}
	}
	for _, r := range p.runs {
		switch {
		case r.image != nil:
			if emb := r.image.embedded(); emb != nil {
				vuelca()
				out = append(out, pdfdoc.Bloque{Imagen: &pdfdoc.Imagen{
					Datos: emb.data, Ancho: float64(emb.width) * 0.75, Alto: float64(emb.height) * 0.75,
					Alt: r.image.alt, Alinear: base.Alinear,
				}})
			} else {
				inf.ImagenesPerdidas++
				if r.image.alt != "" { // lo único que se puede conservar de una imagen externa
					frs = append(frs, pdfdoc.Fragmento{Texto: "[" + r.image.alt + "]", Cursiva: true})
				}
			}
		case r.brk:
			frs = append(frs, pdfdoc.Fragmento{Texto: "\n"})
		case r.text != "":
			frs = append(frs, pdfdoc.Fragmento{
				Texto: r.text, Negrita: r.b || rol != "", Cursiva: r.i || p.style == "Quote", Subrayado: r.u, Tachado: r.strike,
				Color: r.color, Enlace: r.link, Idioma: r.lang,
			})
		}
	}
	vuelca()
	return out
}

func uaParrafo(d *pdfdoc.Doc, p xpara, inf *InformePDF) {
	partes := uaPartes(p, inf)
	if len(partes) == 0 {
		d.Hueco(11 * 1.4) // el párrafo vacío del editor es un renglón en blanco
		return
	}
	for i, b := range partes {
		switch {
		case b.Imagen != nil:
			if err := d.Imagen(*b.Imagen); err != nil {
				inf.ImagenesPerdidas++
				if b.Imagen.Alt != "" { // lo único que se puede conservar de una imagen que no se incrusta
					d.Parrafo(pdfdoc.Parrafo{Fragmentos: []pdfdoc.Fragmento{{Texto: "[" + b.Imagen.Alt + "]", Cursiva: true}}, Despues: 4})
				}
			}
		case p.list != "" && i == 0:
			d.ItemLista(p.level, p.list == "number", *b.Parrafo)
		default:
			d.Parrafo(*b.Parrafo)
		}
	}
}

// uaTabla pasa la rejilla ya normalizada (con las celdas cubiertas marcadas) a pdfdoc.
func uaTabla(_ *pdfdoc.Doc, t *xtable, inf *InformePDF) pdfdoc.Tabla {
	out := pdfdoc.Tabla{Filas: make([][]pdfdoc.Celda, len(t.rows))}
	for i, row := range t.rows {
		out.Filas[i] = make([]pdfdoc.Celda, len(row))
		for j, c := range row {
			if c.covered {
				out.Filas[i][j] = pdfdoc.Celda{Cubierta: true}
				continue
			}
			cs, rs := c.span()
			cel := pdfdoc.Celda{Encabezado: c.header, ColSpan: cs, RowSpan: rs}
			for _, p := range c.paras {
				if p.list != "" { // ponytail: una lista dentro de una celda sale con su viñeta en el texto, sin L/LI
					marca := "• "
					if p.list == "number" {
						marca = "- "
					}
					p.runs = append([]xrun{{text: strings.Repeat("  ", p.level) + marca}}, p.runs...)
					p.list = ""
				}
				cel.Bloques = append(cel.Bloques, uaPartes(p, inf)...)
			}
			out.Filas[i][j] = cel
		}
	}
	return out
}

func uaHoja(d *pdfdoc.Doc, title string, s *State) {
	if title != "" {
		d.Parrafo(pdfdoc.Parrafo{Rol: "H1", Fragmentos: []pdfdoc.Fragmento{{Texto: title, Negrita: true}}, Tam: 20, Despues: 8})
	}
	for _, g := range sheetGrids(sheetModelOrEmpty(s)) {
		d.Parrafo(pdfdoc.Parrafo{Rol: "H2", Fragmentos: []pdfdoc.Fragmento{{Texto: g.name, Negrita: true}}, Tam: 14, Despues: 4})
		cols := 0
		for _, row := range g.rows {
			cols = max(cols, len(row))
		}
		if cols == 0 {
			continue
		}
		filas := make([][]pdfdoc.Celda, len(g.rows))
		for i, row := range g.rows {
			filas[i] = make([]pdfdoc.Celda, cols)
			for j := range cols {
				txt := ""
				if j < len(row) {
					txt = row[j]
				}
				filas[i][j] = pdfdoc.Celda{Bloques: []pdfdoc.Bloque{{Parrafo: &pdfdoc.Parrafo{Fragmentos: []pdfdoc.Fragmento{{Texto: txt}}, Tam: 9}}}}
			}
		}
		d.Tabla(pdfdoc.Tabla{Filas: filas})
	}
}

func uaDiapositivas(d *pdfdoc.Doc, title string, s *State, inf *InformePDF) {
	model := slidesModelOf(title, s)
	if title != "" {
		d.Parrafo(pdfdoc.Parrafo{Rol: "H1", Fragmentos: []pdfdoc.Fragmento{{Texto: title, Negrita: true}}, Tam: 22, Despues: 8})
	}
	for i, sl := range model.Slides {
		if i > 0 {
			d.NuevaPagina()
		}
		d.Parrafo(pdfdoc.Parrafo{Rol: "H2", Fragmentos: []pdfdoc.Fragmento{{Texto: "Diapositiva " + strconv.Itoa(i+1), Negrita: true}}, Tam: 16, Despues: 6})
		for _, box := range sl.Boxes {
			switch box.Kind {
			case "image":
				if emb := (&ximage{src: box.Src}).embedded(); emb != nil {
					if d.Imagen(pdfdoc.Imagen{Datos: emb.data, Ancho: 360}) != nil {
						inf.ImagenesPerdidas++
					}
				} else {
					inf.ImagenesPerdidas++
				}
			case "shape":
			default:
				uaBloques(d, parseHTMLDocCon(box.HTML, false, true).blocks, inf)
			}
		}
		if notes := strings.TrimSpace(sl.Notes); notes != "" {
			d.Parrafo(pdfdoc.Parrafo{Fragmentos: []pdfdoc.Fragmento{{Texto: "Notas", Negrita: true}}, Despues: 2})
			for _, line := range splitParagraphs(notes) {
				d.Parrafo(pdfdoc.Parrafo{Fragmentos: []pdfdoc.Fragmento{{Texto: line, Cursiva: true}}, Despues: 2})
			}
		}
	}
}
