// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

// UN SOLO FICHERO QUE SE LEE Y QUE SE PUEDE DEVOLVER.
//
// Markdown y texto plano se llevan el contenido y pierden lo demás: la posición
// en el lienzo, el ancho, el color de cada estado. Y hay algo que directamente
// NO PUEDEN llevarse: una nota cifrada. El servidor no tiene la clave, así que
// en un formato de lectura el criptograma solo serían bytes que nadie abre —y
// por eso una copia de seguridad de un cuaderno cifrado no existía.
//
// El HTML resuelve las dos cosas a la vez, que es lo que ningún otro formato
// hacía: se abre en cualquier navegador y se VE el cuaderno, y a la vez lleva
// dentro el volcado íntegro en un `<script type="application/json">` que ningún
// navegador pinta. Quien solo quiere leerlo, lo lee; quien quiere restaurarlo,
// tiene todo lo necesario en el mismo fichero.
//
// Es además lo que exporta BasKet Note Pads, que es la inspiración de este
// producto, y el contenido de una nota YA es HTML saneado (allowlist cerrada,
// ver html.go): no hay conversión de por medio que pueda perder nada.

// versionVolcado va DENTRO del fichero. Uno sin versión es uno que, dentro de dos
// cambios de esquema, nadie sabe si puede leer — y para entonces ya está en el
// disco de alguien.
const versionVolcado = 1

type volcadoEmbebido struct {
	Formato     string    `json:"formato"`
	Version     int       `json:"version"`
	ExportadoEn time.Time `json:"exportado_en"`
	Notebook    Notebook  `json:"notebook"`
	Notas       []Note    `json:"notas"`
}

// NotebookAHTML vuelca el cuaderno como una página autocontenida.
func NotebookAHTML(nb Notebook, notas []Note) string {
	// EL TOKEN DEL ENLACE PÚBLICO NO SALE, ni en la parte visible ni en el
	// volcado. Es la credencial que abre el cuaderno SIN sesión, y un fichero
	// exportado se guarda en un disco, se manda por correo y sube a una copia de
	// seguridad: ahí dentro seguiría abriendo la puerta.
	nb.PublicToken = ""

	ordenadas := enOrdenDePantalla(notas)
	titulo := tituloDe(nb.Title, "Cuaderno sin título")

	var b strings.Builder
	b.WriteString(`<!doctype html>` + "\n")
	b.WriteString(`<html lang="es"><head><meta charset="utf-8">` + "\n")
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">` + "\n")
	b.WriteString(`<title>` + html.EscapeString(titulo) + `</title>` + "\n")
	b.WriteString(`<style>` + estiloVolcado + `</style>` + "\n")
	b.WriteString(`</head><body>` + "\n")
	b.WriteString(`<h1>` + html.EscapeString(titulo) + `</h1>` + "\n")

	// UN ENLACE ENTRE NOTAS DEL MISMO CUADERNO SE VUELVE UN ANCLAJE. En la app
	// apunta a `?notebook=…&note=…`, que exige sesión; en este fichero la nota de
	// destino está unas líneas más abajo, y el enlace tiene que llevar allí. Es lo
	// que convierte el volcado en un wiki que se lee solo, en cualquier navegador.
	exportadas := make(map[string]bool, len(ordenadas))
	for _, n := range ordenadas {
		exportadas[n.ID.String()] = true
	}

	for _, n := range ordenadas {
		b.WriteString(`<article class="nota" id="nota-` + n.ID.String() + `">` + "\n")
		b.WriteString(`<h2>` + html.EscapeString(tituloDe(n.Title, "Nota sin título")) + `</h2>` + "\n")
		b.WriteString(enlacesInternosAAnclas(cuerpoHTMLDeNota(n), exportadas))
		b.WriteString(estadosHTML(n))
		b.WriteString(enlacesHTML(n))
		b.WriteString(`</article>` + "\n")
	}

	b.WriteString(volcadoEnScript(nb, ordenadas))
	b.WriteString("\n</body></html>\n")
	return b.String()
}

func cuerpoHTMLDeNota(n Note) string {
	if n.ContentCipher != "" {
		// Lo cifrado no se pinta, pero SÍ viaja en el volcado de abajo: es la
		// diferencia entre este formato y los de lectura. Aquí se dice para que
		// quien mira el fichero no crea que la nota estaba vacía.
		return `<p class="cifrada">Nota cifrada de extremo a extremo. Su contenido ` +
			`no se puede mostrar aquí, pero viaja completo en el volcado de este ` +
			`mismo fichero y se recupera al importarlo.</p>` + "\n"
	}
	if c := SanitizeHTML(n.Content); c != "" {
		// SIN ESCAPAR —escaparlo enseñaría las etiquetas en vez del texto con formato—
		// pero SANEADO AQUÍ, no «porque ya lo estaba»: hasta 2026-10 solo se saneaban al
		// guardar las notas de texto y de tarea, y este volcado sacaba tal cual el HTML
		// de las de cualquier otro tipo. Sobre lo ya saneado no cambia nada.
		return `<div class="cuerpo">` + c + `</div>` + "\n"
	}
	return `<p class="vacia">(vacía)</p>` + "\n"
}

func estadosHTML(n Note) string {
	if len(n.States) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<p class="estados">`)
	for _, s := range n.States {
		nombre := strings.TrimSpace(s.Name)
		if nombre == "" {
			continue
		}
		etiqueta := nombre
		if tag := strings.TrimSpace(s.TagName); tag != "" {
			etiqueta = tag + ": " + nombre
		}
		// El color es del usuario y acaba dentro de un atributo `style`: se valida
		// contra el mismo `ValidHexColor` que ya guarda la entrada, en vez de
		// confiar en que lo de la base de datos siempre esté bien.
		color := "#565b62"
		if ValidHexColor(s.Color) {
			color = s.Color
		}
		fmt.Fprintf(&b, `<span class="estado" style="background:%s">%s%s</span>`,
			html.EscapeString(color),
			html.EscapeString(strings.TrimSpace(s.Emoji)+" "),
			html.EscapeString(etiqueta))
	}
	b.WriteString(`</p>` + "\n")
	return b.String()
}

func enlacesHTML(n Note) string {
	if len(n.Links) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul class="enlaces">` + "\n")
	for _, l := range n.Links {
		rotulo := strings.TrimSpace(l.Label)
		if rotulo == "" {
			rotulo = l.TargetID
		}
		// Solo se convierte en `<a>` lo que es una URL web comprobada. Un
		// `javascript:` guardado hace tiempo, o un destino interno que no es una
		// dirección, se pintan como texto: un volcado no debería ser la vía por la
		// que algo así llega a ejecutarse en el navegador de quien lo abre.
		if ValidWebURL(l.URL) {
			fmt.Fprintf(&b, `<li><a href="%s" rel="noopener noreferrer">%s</a></li>`+"\n",
				html.EscapeString(l.URL), html.EscapeString(rotulo))
			continue
		}
		fmt.Fprintf(&b, `<li>%s <span class="destino">(%s: %s)</span></li>`+"\n",
			html.EscapeString(rotulo), html.EscapeString(l.TargetKind), html.EscapeString(l.TargetID))
	}
	b.WriteString(`</ul>` + "\n")
	return b.String()
}

// volcadoEnScript embebe el cuaderno íntegro. Ningún navegador pinta un `script`
// de tipo desconocido, así que esto es invisible al abrir el fichero y está ahí
// entero para quien lo reimporte.
func volcadoEnScript(nb Notebook, notas []Note) string {
	if notas == nil {
		notas = []Note{}
	}
	datos, err := json.MarshalIndent(volcadoEmbebido{
		Formato:     "sytue.notepads",
		Version:     versionVolcado,
		ExportadoEn: time.Now().UTC(),
		Notebook:    nb,
		Notas:       notas,
	}, "", "  ")
	if err != nil {
		return ""
	}
	// `</script>` DENTRO del JSON cerraría el bloque antes de tiempo y el resto
	// del volcado se pintaría como texto en la página. Le puede pasar a cualquier
	// nota que hable de HTML, así que no es un caso raro: es el caso.
	seguro := strings.ReplaceAll(string(datos), "</", `<\/`)
	return "\n<script type=\"application/json\" id=\"sytue-notepads-volcado\">\n" +
		seguro + "\n</script>\n"
}

const estiloVolcado = `
:root{color-scheme:light dark}
body{margin:0 auto;padding:2rem 1.25rem;max-width:52rem;
 font:16px/1.6 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
h1{font-size:1.75rem;margin:0 0 1.5rem}
.nota{border:1px solid rgba(128,128,128,.35);border-radius:.5rem;padding:1rem 1.25rem;margin:0 0 1rem}
.nota h2{font-size:1.1rem;margin:0 0 .5rem}
.cuerpo{overflow-wrap:anywhere}
.cifrada,.vacia{color:#666;font-style:italic}
.estados{margin:.75rem 0 0}
.estado{display:inline-block;margin:0 .35rem .35rem 0;padding:.1rem .5rem;border-radius:999px;
 color:#fff;font-size:.8rem}
.enlaces{margin:.5rem 0 0;padding-left:1.25rem;font-size:.9rem}
.destino{color:#666}
`

// enlaceANota casa el `href` de un enlace a otra nota tal como lo escribe la
// herramienta de enlaces del editor: la URL pública de la app con `note=<uuid>`
// en la query. En un atributo ya saneado el `&` va como `&amp;`, y se admiten
// las dos formas. Se captura el uuid y, si viene, el título (`h=`).
var enlaceANota = regexp.MustCompile(
	`href="[^"]*[?&](?:amp;)?note=([0-9a-f-]{36})(?:(?:&|&amp;)h=([a-z0-9-]+))?[^"]*"`,
)

// enlacesInternosAAnclas reescribe, dentro de un cuerpo ya saneado, los enlaces a
// notas que también van en el fichero como anclajes `#nota-<uuid>`. Los que
// apuntan a notas de fuera se dejan como están: siguen llevando a la app.
func enlacesInternosAAnclas(cuerpo string, exportadas map[string]bool) string {
	return enlaceANota.ReplaceAllStringFunc(cuerpo, func(m string) string {
		partes := enlaceANota.FindStringSubmatch(m)
		if len(partes) < 2 || !exportadas[partes[1]] {
			return m
		}
		return `href="#nota-` + partes[1] + `"`
	})
}
