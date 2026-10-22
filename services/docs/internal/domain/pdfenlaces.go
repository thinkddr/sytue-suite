// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Los enlaces de un PDF: anotaciones `/Link` con su acción (§12.5.6.5).
//
// El render los OCULTA a propósito —un enlace no se dibuja, se pulsa— así que
// hasta ahora un documento con índice interno o con direcciones web no navegaba:
// se veía el subrayado que hubiera puesto el autor y no pasaba nada al hacer
// clic. Cualquier lector lleva esto desde siempre.
//
// Se entregan como DATOS y no como `<a>` dentro del SVG, por dos motivos que
// pesan igual: el filtro de esquemas se aplica aquí, en el servidor, donde no se
// puede saltar; y la interfaz necesita distinguir un salto interno —que mueve el
// visor— de uno externo —que abre una pestaña—, cosa que un `<a href>` no dice.
package domain

import "strings"

// Enlace es una zona pulsable de una página.
type Enlace struct {
	Pagina int `json:"pagina"`
	// Rect es la zona en coordenadas del PDF [x0 y0 x1 y1].
	Rect [4]float64 `json:"rect"`
	// URL es el destino externo, ya filtrado. Vacío si el enlace es interno.
	URL string `json:"url,omitempty"`
	// Destino es la página a la que salta (0-based), o -1 si no es interno o no
	// se pudo resolver. Un enlace sin URL y sin destino NO se entrega.
	Destino int `json:"destino"`
}

// maxEnlacesPorDocumento acota un documento con un enlace por palabra.
const maxEnlacesPorDocumento = 5000

// EnlacesDelDocumento devuelve las zonas pulsables de todas las páginas.
func (d *PDFDoc) EnlacesDelDocumento() []Enlace {
	porObjeto := d.paginaPorObjeto()
	var out []Enlace
	for i := range d.paginas {
		if len(out) >= maxEnlacesPorDocumento {
			break
		}
		anots, _ := d.resolver(d.paginas[i].dict["Annots"]).(pdfArray)
		for _, a := range anots {
			ad, ok := d.resolver(a).(pdfDict)
			if !ok {
				continue
			}
			if sub, _ := d.resolver(ad["Subtype"]).(pdfName); sub != "Link" {
				continue
			}
			e, ok := d.enlaceDe(ad, i, porObjeto)
			if !ok {
				continue
			}
			out = append(out, e)
			if len(out) >= maxEnlacesPorDocumento {
				break
			}
		}
	}
	return out
}

// enlaceDe traduce una anotación /Link. Devuelve false si no lleva a ninguna
// parte: un enlace que no navega ocupa sitio sobre el texto e impide
// seleccionarlo, así que es peor que no ponerlo.
func (d *PDFDoc) enlaceDe(ad pdfDict, pagina int, porObjeto map[int]int) (Enlace, bool) {
	rect, ok := d.rectDeAnotacion(ad)
	if !ok {
		return Enlace{}, false
	}
	e := Enlace{Pagina: pagina, Rect: rect, Destino: -1}

	// El destino directo (/Dest) y la acción (/A) son excluyentes en la práctica;
	// si vienen los dos manda /Dest, como hace cualquier lector.
	if dest := ad["Dest"]; dest != nil {
		e.Destino = d.paginaDeDestino(dest, porObjeto, 0)
		return e, e.Destino >= 0
	}

	accion, _ := d.resolver(ad["A"]).(pdfDict)
	if accion == nil {
		return Enlace{}, false
	}
	switch s, _ := d.resolver(accion["S"]).(pdfName); s {
	case "URI":
		u, _ := d.resolver(accion["URI"]).(pdfString)
		url, ok := urlSegura(string(u))
		if !ok {
			return Enlace{}, false
		}
		e.URL = url
		return e, true
	case "GoTo":
		e.Destino = d.paginaDeDestino(accion["D"], porObjeto, 0)
		return e, e.Destino >= 0
	default:
		// GoToR (otro fichero), Launch (ejecutar un programa), JavaScript,
		// SubmitForm… Ninguna se entrega: son justo las que convierten un PDF
		// ajeno en un vector de ataque, y ninguna tiene sentido en un visor web.
		return Enlace{}, false
	}
}

// rectDeAnotacion normaliza el /Rect, que puede venir con las esquinas en
// cualquier orden.
func (d *PDFDoc) rectDeAnotacion(ad pdfDict) ([4]float64, bool) {
	arr, _ := d.resolver(ad["Rect"]).(pdfArray)
	if len(arr) < 4 {
		return [4]float64{}, false
	}
	var v [4]float64
	for i := 0; i < 4; i++ {
		v[i] = comoReal(d.resolver(arr[i]))
	}
	r := [4]float64{min(v[0], v[2]), min(v[1], v[3]), max(v[0], v[2]), max(v[1], v[3])}
	// Un rectángulo sin área no se puede pulsar; entregarlo solo añade ruido.
	if r[2]-r[0] < 0.5 || r[3]-r[1] < 0.5 {
		return [4]float64{}, false
	}
	return r, true
}

// urlSegura deja pasar SOLO los esquemas que tienen sentido en un enlace de
// documento, y lo hace aquí y no en el navegador.
//
// Un PDF lo aporta quien llama y puede venir de cualquier parte: `javascript:`
// dentro de un `href` que la interfaz pinte es ejecución de código con la sesión
// del usuario delante, y `file:` o `data:` son las variantes de lo mismo. La
// lista es blanca a propósito: enumerar lo prohibido deja fuera el esquema que
// alguien invente mañana.
func urlSegura(u string) (string, bool) {
	u = strings.TrimSpace(u)
	if u == "" || len(u) > 2048 {
		return "", false
	}
	// Un control dentro de la URL sirve para partirla y colar otra cosa donde se
	// pinte; y ningún destino legítimo los lleva.
	for _, r := range u {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	bajo := strings.ToLower(u)
	for _, ok := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(bajo, ok) {
			return u, true
		}
	}
	return "", false
}
