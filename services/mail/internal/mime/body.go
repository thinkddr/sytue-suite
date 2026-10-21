// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mime

import (
	"bytes"
	"errors"
	"io"
	"strings"

	gomail "github.com/emersion/go-message/mail"
	xhtml "golang.org/x/net/html"
)

// Body es el CUERPO legible de un correo, para quien necesita el texto entero y no el
// extracto de 280 caracteres que basta para una lista.
//
// Hoy lo usa el correo a canal de Sytue Rooms: lo que se publica en el canal es esto.
type Body struct {
	// Text es el cuerpo en texto plano. Si el correo solo traía HTML —lo normal en las
	// herramientas que avisan por email—, es ese HTML reducido a texto.
	Text string
	// Attachments son los NOMBRES de los ficheros adjuntos, en orden.
	Attachments []string
}

// ReadBody extrae el cuerpo legible de un mensaje RFC 5322, acotado a `max` runas.
//
// Se prefiere text/plain y solo se cae al HTML si no hay: un correo bien formado trae
// las dos partes, y la de texto es la que su autor escribió.
func ReadBody(raw []byte, limite int) Body {
	var b Body
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return b
	}
	defer func() { _ = mr.Close() }()

	var texto, html strings.Builder
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break // degradar: se devuelve lo leído hasta aquí
		}
		switch ph := p.Header.(type) {
		case *gomail.InlineHeader:
			ct, _, _ := ph.ContentType()
			// Cota por parte: un correo puede traer muchas, y lo que se publica está
			// acotado igualmente al final. `limite*4` cubre el peor caso multibyte.
			data, _ := io.ReadAll(io.LimitReader(p.Body, int64(limite)*4))
			switch {
			case strings.HasPrefix(ct, "text/plain") && texto.Len() < limite*4:
				texto.Write(data)
			case strings.HasPrefix(ct, "text/html") && html.Len() < limite*4:
				html.Write(data)
			}
		case *gomail.AttachmentHeader:
			if n, _ := ph.Filename(); strings.TrimSpace(n) != "" {
				b.Attachments = append(b.Attachments, strings.TrimSpace(n))
			}
		}
	}

	t := strings.TrimSpace(texto.String())
	if t == "" {
		t = HTMLToText(html.String())
	}
	b.Text = limitRunes(t, limite)
	return b
}

// HTMLToText reduce HTML a texto legible con el tokenizador que ya usa Sytue Docs.
//
// Una expresión regular sobre «<...>» no vale: se traga el contenido de <style> y
// <script> —que acabaría publicado como si fuera texto del correo— y deja las entidades
// sin decodificar.
func HTMLToText(h string) string {
	if strings.TrimSpace(h) == "" {
		return ""
	}
	z := xhtml.NewTokenizer(strings.NewReader(h))
	var sb strings.Builder
	saltar := 0 // profundidad dentro de <script>/<style>
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return collapseBlankLines(sb.String())
		case xhtml.StartTagToken:
			switch name, _ := z.TagName(); string(name) {
			case "script", "style", "head":
				saltar++
			case "br", "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6":
				sb.WriteByte('\n')
			}
		case xhtml.EndTagToken:
			switch name, _ := z.TagName(); string(name) {
			case "script", "style", "head":
				if saltar > 0 {
					saltar--
				}
			case "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6":
				sb.WriteByte('\n')
			}
		case xhtml.TextToken:
			if saltar == 0 {
				sb.Write(z.Text())
			}
		// Un `<br/>` autocerrado separa líneas igual que `<br>`: sin esta rama, un
		// correo escrito en XHTML —que los generan varios clientes— salía como un
		// párrafo continuo. Los comentarios y el doctype no aportan texto visible y
		// se descartan a propósito; se enumeran para que añadir un tipo nuevo al
		// tokenizador obligue a decidir aquí en vez de caer en el silencio.
		case xhtml.SelfClosingTagToken:
			if name, _ := z.TagName(); saltar == 0 && string(name) == "br" {
				sb.WriteByte('\n')
			}
		case xhtml.CommentToken, xhtml.DoctypeToken:
		}
	}
}

// collapseBlankLines recorta cada línea y colapsa las series de líneas vacías: el HTML
// de un correo genera decenas por su maquetación en tablas.
func collapseBlankLines(s string) string {
	var out []string
	vacia := false
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			if vacia {
				continue
			}
			vacia = true
		} else {
			vacia = false
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// limitRunes corta por RUNAS: hacerlo por bytes partiría un carácter multibyte.
func limitRunes(s string, limite int) string {
	if limite <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= limite {
		return s
	}
	return strings.TrimSpace(string(r[:limite])) + "\n\n…"
}
