// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// BLOQUES REUTILIZABLES (ROADMAP-OFFICE plantillas-1). Un bloque es un trozo de contenido
// que se inserta en un documento que ya existe:
//
//   - Documento: un fragmento del HTML del editor.
//   - Presentación: UNA diapositiva, `{id, boxes, …}`.
//   - Hoja: `{cells, formats, styles, charts}` de un trozo de hoja; quien lo inserta lo
//     desplaza a la celda activa.
//
// Los de Sytue salen de `plantillas.GenerarBloque`; los de la organización, de un documento
// marcado como bloque en su biblioteca (ExtraerBloque). Los dos se visten con el tema o la
// marca al insertarse (AplicarTemaBloque).

// AplicarTemaBloque viste un bloque con el tema `t` sin tocar su contenido. En una hoja solo
// cambian las celdas marcadas con `tema` en su estilo (las que el bloque declara de marca)
// y el color de sus gráficos; una diapositiva hereda el tema de su presentación.
func AplicarTemaBloque(kind Kind, contenido string, t TemaComun) (string, error) {
	switch kind {
	case KindDocument:
		return aplicarTemaTexto(contenido, t)
	case KindPresentation:
		return contenido, nil
	case KindSpreadsheet:
		m, err := decodificarJSON(contenido)
		if err != nil {
			return "", err
		}
		estilos, _ := m["styles"].(map[string]any)
		estilarCeldasDeTema(estilos, t, acentosConocidos(t), 0)
		if t.ID != IDTemaPapel {
			graficos, _ := m["charts"].([]any)
			for _, g := range graficos {
				if o, ok := g.(map[string]any); ok {
					o["colors"] = []any{t.Accent}
				}
			}
		}
		return codificarJSON(m)
	default:
		return "", ErrInvalidKind
	}
}

// estilarCeldasDeTema viste las celdas que un tema (o un bloque) marcó con `tema` en su
// estilo, salvo las de la fila `salvo` (la cabecera, que ya lleva su propio paso).
func estilarCeldasDeTema(estilos map[string]any, t TemaComun, conocidos []string, salvo int) {
	for ref, v := range estilos {
		est, _ := v.(map[string]any)
		if est == nil || est["tema"] == nil || (salvo != 0 && filaDeRef(ref) == salvo) {
			continue
		}
		estilarCabeceraHoja(est, t, conocidos)
		if len(est) == 0 {
			delete(estilos, ref)
		}
	}
}

// ExtraerBloque saca de `contenido` (el valor del registro de contenido de un documento) lo
// que se inserta como bloque: el HTML entero de un documento —saneado al subconjunto que el
// editor pinta, porque va a ir al documento de OTRA persona—, la primera diapositiva de una
// presentación o las celdas, formatos, estilos y gráficos de la primera hoja.
func ExtraerBloque(kind Kind, contenido string) (string, error) {
	switch kind {
	case KindDocument:
		res, err := Import("bloque.html", []byte(contenido))
		if err != nil {
			return "", err
		}
		return res.Content, nil
	case KindPresentation, KindSpreadsheet:
		m, err := decodificarJSON(contenido)
		if err != nil {
			return "", err
		}
		lista, campos := "slides", []string(nil)
		if kind == KindSpreadsheet {
			lista, campos = "sheets", []string{"cells", "formats", "styles", "charts"}
		}
		piezas, _ := m[lista].([]any)
		if len(piezas) == 0 {
			return "", fmt.Errorf("%w: el documento no tiene contenido que insertar", ErrInvalidFormat)
		}
		primera, _ := piezas[0].(map[string]any)
		if primera == nil {
			return "", fmt.Errorf("%w: contenido ilegible", ErrInvalidFormat)
		}
		if campos == nil {
			sanearDiapositiva(primera)
			return codificarJSON(primera)
		}
		out := map[string]any{}
		for _, c := range campos {
			if v, ok := primera[c]; ok {
				out[c] = v
			}
		}
		if celdas, _ := out["cells"].(map[string]any); celdas != nil {
			for ref, v := range celdas {
				if txt, ok := v.(string); ok {
					celdas[ref] = sanearCelda(txt)
				}
			}
		}
		return codificarJSON(out)
	default:
		return "", ErrInvalidKind
	}
}

// Contenido es el valor del registro de contenido del tipo ("" si no lo hay).
func (s *State) Contenido(kind Kind) string { return s.contentString(ContentKey(kind)) }

// sanearDiapositiva deja el HTML de cada caja en el subconjunto del editor (el mismo saneado
// que un documento) y quita los orígenes de imagen o medio que no sean una dirección web o
// un `data:` de imagen, audio o vídeo. Es la diapositiva de otra persona.
func sanearDiapositiva(d map[string]any) {
	cajas, _ := d["boxes"].([]any)
	for _, c := range cajas {
		caja, _ := c.(map[string]any)
		if caja == nil {
			continue
		}
		if h, _ := caja["html"].(string); h != "" {
			caja["html"] = importHTMLDoc([]byte(h))
		}
		for _, campo := range []string{"src", "poster"} {
			if v, hay := caja[campo]; hay {
				if u, _ := v.(string); !origenSeguro(u) {
					delete(caja, campo)
				}
			}
		}
	}
}

// origenSeguro: http(s) o un `data:` de imagen, audio o vídeo que no sea SVG (un SVG puede
// llevar guiones).
func origenSeguro(u string) bool {
	bajo := strings.ToLower(strings.TrimSpace(u))
	if tipo, ok := strings.CutPrefix(bajo, "data:"); ok {
		return !strings.HasPrefix(tipo, "image/svg") &&
			(strings.HasPrefix(tipo, "image/") || strings.HasPrefix(tipo, "audio/") || strings.HasPrefix(tipo, "video/"))
	}
	return (strings.HasPrefix(bajo, "http://") || strings.HasPrefix(bajo, "https://")) && enlaceSeguro(u)
}

// etiquetasDeCelda son las que el editor pone en una celda: formato de carácter y nada más.
var etiquetasDeCelda = map[atom.Atom]bool{
	atom.B: true, atom.Strong: true, atom.I: true, atom.Em: true, atom.U: true, atom.S: true, atom.Br: true,
}

// sanearCelda deja en el HTML de una celda solo el formato de carácter, sin atributos: el
// texto se conserva (escapado) y cualquier otra etiqueta desaparece. Una fórmula no lleva
// etiquetas y pasa tal cual.
func sanearCelda(celda string) string {
	if !strings.ContainsAny(celda, "<&") {
		return celda
	}
	var b strings.Builder
	z := xhtml.NewTokenizer(strings.NewReader(celda))
	omitir := 0 // dentro de <script> o <style>: su texto tampoco entra
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return b.String()
		case xhtml.TextToken:
			if omitir == 0 {
				b.WriteString(html.EscapeString(string(z.Text())))
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			nombre, _ := z.TagName()
			a := atom.Lookup(nombre)
			if a == atom.Script || a == atom.Style {
				omitir++
			} else if etiquetasDeCelda[a] && omitir == 0 {
				b.WriteString("<" + a.String() + ">")
			}
		case xhtml.EndTagToken:
			nombre, _ := z.TagName()
			a := atom.Lookup(nombre)
			if a == atom.Script || a == atom.Style {
				omitir = max(0, omitir-1)
			} else if etiquetasDeCelda[a] && a != atom.Br && omitir == 0 {
				b.WriteString("</" + a.String() + ">")
			}
		case xhtml.CommentToken, xhtml.DoctypeToken:
			// Ni comentarios ni DOCTYPE entran en una celda.
		}
	}
}
