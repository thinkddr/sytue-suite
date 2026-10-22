// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// EL LOGOTIPO DE LA ORGANIZACIÓN va con su marca (ROADMAP-OFFICE plantillas-1):
//
//   - Documento: un párrafo con la imagen al principio (la cabecera), marcado con
//     `data-logo-marca="<versión>"`.
//   - Presentación: una caja de imagen `logo-marca` en la esquina de la portada (la primera
//     diapositiva).
//   - Hoja: no se incrusta. El modelo de la hoja no tiene imágenes; su cabecera lleva los
//     colores de la marca.
//
// Como el resto del tema, no toca el contenido: lo que pone lo reconoce por su marca y lo
// cambia (otra versión del logo) o lo quita (otro tema, «papel», una marca sin logo) sin
// tocar nada más, y aplicarlo dos veces no cambia nada. El PNG llega ya validado y
// recodificado (internal/logoorg): aquí solo se escribe como `data:image/png;base64,…`.

const (
	attrLogoMarca  = "data-logo-marca"
	idCajaLogo     = "logo-marca"
	altoLogoDocPx  = 48  // alto del logo en la cabecera de un documento
	maxAnchoLogoPx = 240 // un logo muy apaisado se limita por el ancho
	altoLogoDiapo  = 10  // % del alto de la diapositiva
	maxAnchoDiapo  = 30  // % del ancho
)

func dataURLLogo(l *LogoMarca) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(l.PNG)
}

// logoDe es el logotipo que el tema pone, o nil si no pone ninguno.
func logoDe(t TemaComun) *LogoMarca {
	if !t.Marca || t.Logo == nil || len(t.Logo.PNG) == 0 || t.Logo.Ancho <= 0 || t.Logo.Alto <= 0 {
		return nil
	}
	return t.Logo
}

// logoEnTexto pone, cambia o quita el párrafo del logo en los nodos de un documento y
// dice si cambió algo.
func logoEnTexto(nodos []*xhtml.Node, t TemaComun) ([]*xhtml.Node, bool) {
	logo := logoDe(t)
	cambio := false
	hay := false
	out := nodos[:0:0]
	for _, n := range nodos {
		if n.Type != xhtml.ElementNode || attrVal(n, attrLogoMarca) == "" {
			out = append(out, n)
			continue
		}
		// Solo uno, y solo si es el vigente; lo demás (otra versión, duplicados) se quita.
		if logo != nil && !hay && attrVal(n, attrLogoMarca) == logo.Hash {
			hay = true
			out = append(out, n)
			continue
		}
		cambio = true
	}
	if logo == nil || hay {
		return out, cambio
	}
	ancho, alto := float64(logo.Ancho)*altoLogoDocPx/float64(logo.Alto), float64(altoLogoDocPx)
	if ancho > maxAnchoLogoPx {
		ancho, alto = maxAnchoLogoPx, float64(logo.Alto)*maxAnchoLogoPx/float64(logo.Ancho)
	}
	w, h := strconv.Itoa(int(math.Round(ancho))), strconv.Itoa(max(1, int(math.Round(alto))))
	img := &xhtml.Node{Type: xhtml.ElementNode, Data: "img", DataAtom: atom.Img, Attr: []xhtml.Attribute{
		{Key: "src", Val: dataURLLogo(logo)}, {Key: "alt", Val: t.Name}, {Key: "width", Val: w}, {Key: "height", Val: h},
	}}
	p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p", DataAtom: atom.P, Attr: []xhtml.Attribute{{Key: attrLogoMarca, Val: logo.Hash}}}
	p.AppendChild(img)
	return append([]*xhtml.Node{p}, out...), true
}

// logoEnDiapos pone, cambia o quita la caja del logo en la portada de una presentación.
func logoEnDiapos(m map[string]any, t TemaComun) {
	diapos, _ := m["slides"].([]any)
	logo := logoDe(t)
	for i, d := range diapos {
		diapo, _ := d.(map[string]any)
		cajas, _ := diapo["boxes"].([]any)
		if diapo == nil {
			continue
		}
		limpias := make([]any, 0, len(cajas)+1)
		for _, c := range cajas {
			if o, _ := c.(map[string]any); o != nil && o["id"] == idCajaLogo {
				continue
			}
			limpias = append(limpias, c)
		}
		if i == 0 && logo != nil {
			// En % del lienzo 16:9: el alto manda y el ancho sale de la proporción del logo.
			ancho := float64(altoLogoDiapo) * float64(logo.Ancho) / float64(logo.Alto) * 9 / 16
			alto := float64(altoLogoDiapo)
			if ancho > maxAnchoDiapo {
				ancho, alto = maxAnchoDiapo, float64(maxAnchoDiapo)*float64(logo.Alto)/float64(logo.Ancho)*16/9
			}
			limpias = append(limpias, map[string]any{
				"id": idCajaLogo, "kind": "image", "html": "", "src": dataURLLogo(logo),
				"x": 4, "y": 4, "w": redondear2(ancho), "h": redondear2(alto),
			})
		}
		if len(limpias) != len(cajas) || (i == 0 && logo != nil) {
			diapo["boxes"] = limpias
		}
	}
}

func redondear2(f float64) float64 { return math.Round(f*100) / 100 }

// versionLogo es la versión del logo de marca que lleva un contenido ("" si no lleva).
func versionLogo(kind Kind, contenido string) string {
	switch kind {
	case KindDocument:
		_, resto, ok := strings.Cut(contenido, attrLogoMarca+`="`)
		if !ok {
			return ""
		}
		v, _, _ := strings.Cut(resto, `"`)
		return v
	case KindPresentation:
		m, err := decodificarJSON(contenido)
		if err != nil {
			return ""
		}
		diapos, _ := m["slides"].([]any)
		if len(diapos) == 0 {
			return ""
		}
		diapo, _ := diapos[0].(map[string]any)
		cajas, _ := diapo["boxes"].([]any)
		for _, c := range cajas {
			if o, _ := c.(map[string]any); o != nil && o["id"] == idCajaLogo {
				// La caja no guarda la versión: se deduce de la imagen (todo PNG acaba igual
				// en base64, así que un trozo no sirve).
				src, _ := o["src"].(string)
				suma := sha256.Sum256([]byte(src))
				return hex.EncodeToString(suma[:8])
			}
		}
	case KindSpreadsheet:
		// La hoja no lleva logo: el modelo de Calcus no tiene imágenes.
	}
	return ""
}
