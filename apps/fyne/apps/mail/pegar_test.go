// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	pngenc "image/png"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// TestElSaneadoEsElDeLaWeb: la lista blanca de `sanitizeHtml`: fuera los scripts y los
// estilos con su contenido, los `on*`, los enlaces `javascript:`, las imágenes `http:` y
// `data:`, las clases y los comentarios condicionales de Word; lo desconocido se
// desenvuelve; y de un `style` solo quedan las propiedades de la lista, sin `url(`.
func TestElSaneadoEsElDeLaWeb(t *testing.T) {
	for entrada, quiero := range map[string]string{
		`<p onclick="x()">Hola <script>alert(1)</script><b>equipo</b></p>`:                                                `<p>Hola <b>equipo</b></p>`,
		`<a href="javascript:alert(1)">malo</a> <a href="https://sytue.eu" target="_blank">bueno</a>`:                     `<a>malo</a> <a href="https://sytue.eu" rel="noopener noreferrer">bueno</a>`,
		`<img src="http://x/a.png"><img src="data:image/png;base64,AA"><img src="https://x/b.png" alt="b" onerror="y()">`: `<img src="https://x/b.png" alt="b">`,
		`<span style="color: red; position: absolute; background: url(http://x); font-weight: bold">t</span>`:             `<span style="color: red; font-weight: bold">t</span>`,
		"<p class=MsoNormal><o:p>uno\r\n  dos</o:p></p><!--[if gte mso 9]><xml></xml><![endif]-->":                        `<p>uno dos</p>`,
		`<html><head><style>p{color:red}</style></head><body><pre>a  b` + "\n" + `c</pre></body></html>`:                  "<pre>a  b\nc</pre>",
		`<table border="1" onmouseover="z()"><tr><td colspan="2">1</td></tr></table>`:                                     `<table border="1"><tbody><tr><td colspan="2">1</td></tr></tbody></table>`,
	} {
		if got := SanearHTML(entrada); got != quiero {
			t.Errorf("SanearHTML(%q)\n  = %q\n  y en la web %q", entrada, got, quiero)
		}
	}
}

// portapapelesDePrueba es el `fyne.Clipboard` con un texto.
type portapapelesDePrueba struct{ texto string }

func (p *portapapelesDePrueba) Content() string     { return p.texto }
func (p *portapapelesDePrueba) SetContent(s string) { p.texto = s }

// pegarEnElCuerpo es ⌘/Ctrl + V con el foco en el cuerpo.
func pegarEnElCuerpo(r *Redactor, texto string) {
	sc, _ := r.rico.Zona().(fyne.Shortcutable)
	sc.TypedShortcut(&fyne.ShortcutPaste{Clipboard: &portapapelesDePrueba{texto}})
}

// TestPegarConservaElFormato: lo pegado desde Word o un navegador entra con su negrita,
// sus enlaces seguros, sus títulos y sus listas, y sale así en la parte HTML del correo
// (y en la de texto, con las viñetas); nada de lo peligroso llega al cuerpo, y deshacer
// quita lo pegado de una vez.
func TestPegarConservaElFormato(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.LeerHTML = func() (string, error) {
		return `<html><head><style>p{}</style></head><body><!--StartFragment-->` +
			`<p class=MsoNormal><b>Plan</b> para el <a href="javascript:alert(1)">lunes</a> ` +
			`en <a href="https://sytue.eu">Sytue</a><script>robar()</script></p>` +
			`<h2>Tareas</h2><ul><li>uno</li><li>dos</li></ul><!--EndFragment--></body></html>`, nil
	}
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Plan"})
	r := b.redaccion.r
	sc, _ := r.rico.Zona().(fyne.Shortcutable)
	escribir(r, "borrar esto")
	sc.TypedShortcut(&fyne.ShortcutSelectAll{}) // se pega ENCIMA de lo seleccionado
	pegarEnElCuerpo(r, "texto que no se usa")
	h := r.B.HTML
	if strings.Contains(h, "borrar esto") {
		t.Errorf("lo seleccionado sigue ahí: %q", h)
	}
	for _, quiero := range []string{"<b>Plan</b>", `<a href="https://sytue.eu">Sytue</a>`, "<h2>Tareas</h2>", "<li>uno</li><li>dos</li>"} {
		if !strings.Contains(h, quiero) {
			t.Errorf("el cuerpo no lleva %s: %q", quiero, h)
		}
	}
	for _, fuera := range []string{"javascript", "script", "robar", "MsoNormal", "style"} {
		if strings.Contains(h, fuera) {
			t.Errorf("el cuerpo lleva %q: %q", fuera, h)
		}
	}
	_, s, _, err := b.componer(r.B, false)
	if err != nil || !strings.Contains(s.HTML, "<h2>Tareas</h2>") || !strings.Contains(s.Texto, "• uno") {
		t.Errorf("el correo: %v, HTML %q, texto %q", err, s.HTML, s.Texto)
	}
	sc.TypedShortcut(&fyne.ShortcutUndo{})
	if strings.Contains(r.B.HTML, "Tareas") || !strings.Contains(r.B.HTML, "borrar esto") {
		t.Errorf("deshacer no vuelve de una vez a lo que había: %q", r.B.HTML)
	}
}

// TestSinHTMLSePegaElTexto: si el portapapeles no trae HTML (o no se puede leer aquí), se
// pega su texto; y ⌘/Ctrl + Mayús + V pega el texto aunque haya HTML, como en la web.
func TestSinHTMLSePegaElTexto(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	b.LeerHTML = func() (string, error) { return "", errors.New("no hay") }
	pegarEnElCuerpo(r, "sin formato")
	if r.B.Cuerpo != "sin formato" || strings.Contains(r.B.HTML, "<b>") {
		t.Fatalf("sin HTML: texto %q, HTML %q", r.B.Cuerpo, r.B.HTML)
	}

	leido := false
	b.LeerHTML = func() (string, error) { leido = true; return "<b>con formato</b>", nil }
	fyne.CurrentApp().Clipboard().SetContent(" y plano")
	sc, _ := r.rico.Zona().(fyne.Shortcutable)
	sc.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyV, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
	if leido || strings.Contains(r.B.HTML, "<b>") || !strings.Contains(r.B.Cuerpo, "y plano") {
		t.Errorf("con Mayús: leído el HTML %v, cuerpo %q, HTML %q", leido, r.B.Cuerpo, r.B.HTML)
	}
}

// TestLasImagenesPegadasEntran: una imagen `https:` o `cid:` pegada entra en el cuerpo en
// su sitio, con el blanco de sus lados, y sin aviso; una `data:` de mapa de bits entra
// como parte del mensaje (`cid:`, en la lista de adjuntos); la que tira el saneado
// (`http:`, un SVG) no entra y se dice —en singular o en plural—, y un `<img>` dentro de
// lo que se va entero (un `<noscript>`, un `<template>`) ni cuenta ni se adjunta.
func TestLasImagenesPegadasEntran(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	antes := bajarImagen
	bajarImagen = func(context.Context, string) (image.Image, error) { return nil, errors.New("sin red") }
	t.Cleanup(func() { bajarImagen = antes })
	tr := traductor(t)
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	sc, _ := r.rico.Zona().(fyne.Shortcutable)
	pegar := func(h string) (string, string) {
		t.Helper()
		sc.TypedShortcut(&fyne.ShortcutSelectAll{})
		b.LeerHTML = func() (string, error) { return h, nil }
		antes := len(b.P.Avisos.Avisos())
		pegarEnElCuerpo(r, "")
		av := b.P.Avisos.Avisos()
		if len(av) == antes {
			return "", ""
		}
		return av[len(av)-1].Titulo, av[len(av)-1].Descripcion
	}
	if titulo, _ := pegar(`<p>Mira <img src="https://x/a.png" width="40"> esto</p>`); titulo != "" ||
		!strings.Contains(r.B.HTML, `Mira <img src="https://x/a.png" width="40"/> esto`) {
		t.Errorf("una imagen https: aviso %q, cuerpo %q", titulo, r.B.HTML)
	}
	if titulo, _ := pegar(`<p><img src="cid:a"></p>`); titulo != "" || !strings.Contains(r.B.HTML, `<img src="cid:a"/>`) {
		t.Errorf("una imagen cid: aviso %q, cuerpo %q", titulo, r.B.HTML)
	}
	var png bytes.Buffer
	_ = pngenc.Encode(&png, image.NewRGBA(image.Rect(0, 0, 4, 2)))
	datos := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png.Bytes())
	if titulo, _ := pegar(`<p>Foto: <img src="` + datos + `" width="4"></p>`); titulo != "" || len(r.B.Adjuntos) != 1 ||
		r.B.Adjuntos[0].Nombre != "image001.png" || r.B.Adjuntos[0].Tipo != "image/png" || !bytes.Equal(r.B.Adjuntos[0].Datos, png.Bytes()) ||
		!strings.Contains(r.B.HTML, `Foto: <img src="cid:`+r.B.Adjuntos[0].CID+`" width="4"/>`) {
		t.Errorf("una imagen data: aviso %q, adjuntos %+v, cuerpo %q", titulo, r.B.Adjuntos, r.B.HTML)
	}
	if titulo, desc := pegar(`<p>Mira <img src="http://x/a.png"> esto</p>`); titulo != tr.T("compose.pastedImageDropped", nil) ||
		desc != tr.T("compose.pastedImagesDroppedDescription", nil) || !strings.Contains(r.B.Cuerpo, "Mira esto") {
		t.Errorf("una imagen http: aviso %q / %q, cuerpo %q", titulo, desc, r.B.Cuerpo)
	}
	if titulo, _ := pegar(`<img src="http://x/a.png"><img src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="><img src="https://x/b.png">`); titulo != tr.T("compose.pastedImagesDropped", nil) {
		t.Errorf("dos que no entran: aviso %q", titulo)
	}
	if titulo, _ := pegar(`<p>solo texto</p><noscript><img src="http://x/p.gif"></noscript><template><img src="` + datos + `"></template>`); titulo != "" || len(r.B.Adjuntos) != 1 {
		t.Errorf("sin imágenes que se pierdan: aviso %q, adjuntos %d", titulo, len(r.B.Adjuntos))
	}
}
