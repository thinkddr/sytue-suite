// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// La lista crece con el renderizador, pero hay cosas que NUNCA pueden entrar: en
// *.sites no hay HTML ni JS libre (A2), así que cualquier vía de ejecutar código o
// de sacar datos a otro sitio que la lista abra es un agujero en las webs de todos
// los clientes. Este test las enumera; añadir una a la lista lo pone en rojo.
func TestPermitidoNoAbreNadaPeligroso(t *testing.T) {
	p, err := CargarPermitido()
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || len(p.HTML.Elementos) == 0 || len(p.SVG.Elementos) == 0 {
		t.Fatalf("permitido.json incompleto: %+v", p)
	}

	nuncaElementos := []string{
		"base", "object", "embed", "applet", "frame", "frameset", "style", "template",
		"noscript", "math", "svg", "dialog",
	}
	for _, e := range nuncaElementos {
		if _, ok := p.HTML.Elementos[e]; ok {
			t.Errorf("elemento HTML prohibido en la lista: <%s>", e)
		}
	}
	// SVG solo figuras: nada que cargue, anime, referencie o incruste HTML.
	for e := range p.SVG.Elementos {
		if !slices.Contains([]string{"svg", "g", "path", "rect", "circle", "line", "polyline", "polygon", "title", "desc"}, e) {
			t.Errorf("elemento SVG fuera de las figuras admitidas: <%s>", e)
		}
	}

	atributos := map[string][]string{"html:*": p.HTML.AtributosGlobales, "svg:*": p.SVG.AtributosComunes}
	for e, as := range p.HTML.Elementos {
		atributos["html:"+e] = as
	}
	for e, as := range p.SVG.Elementos {
		atributos["svg:"+e] = as
	}
	for donde, as := range atributos {
		for _, a := range as {
			if a != strings.ToLower(a) {
				t.Errorf("%s: %q tiene mayúsculas; los tokenizadores pliegan los nombres", donde, a)
			}
			if strings.HasPrefix(a, "on") || a == "style" || a == "srcdoc" || a == "formaction" ||
				a == "http-equiv" || a == "xlink:href" || (strings.HasPrefix(donde, "svg:") && a == "href") {
				t.Errorf("%s: el atributo %q ejecuta código, mete estilos en línea o carga de fuera", donde, a)
			}
		}
	}
	for _, pre := range p.HTML.PrefijosGlobales {
		if pre != "aria-" && pre != "data-" {
			t.Errorf("prefijo global %q: solo aria- y data- son inertes", pre)
		}
	}

	for _, e := range p.HTML.Esquemas {
		if e != "https:" && e != "mailto:" && e != "tel:" {
			t.Errorf("esquema de URL %q: solo https, mailto y tel", e)
		}
	}
	for _, o := range p.HTML.IframeOrigenes {
		if o != "$PAGES_CAST_ORIGIN" && (!strings.HasPrefix(o, "https://") || strings.Count(o, "/") != 2) {
			t.Errorf("origen de iframe %q: https:// y sin ruta", o)
		}
	}
	for _, r := range p.HTML.Valores["link.rel"] {
		if !slices.Contains([]string{"canonical", "alternate", "icon", "preload", "stylesheet"}, r) {
			t.Errorf("link rel=%q fuera de la lista cerrada (§7.4)", r)
		}
	}
	if !strings.HasPrefix(p.HTML.FormAction, "^/_pages/") {
		t.Errorf("form_action %q: un formulario solo puede ir a /_pages/ del mismo origen", p.HTML.FormAction)
	}

	// Scripts: solo del mismo origen y anclados, o JSON-LD en línea.
	anclado := regexp.MustCompile(`^\^/_[a-z]+/.*\$$`)
	for _, s := range p.HTML.Scripts {
		switch {
		case s.EnLinea != "":
			if s.EnLinea != "json" || s.Type != "application/ld+json" || s.Src != "" {
				t.Errorf("script en línea que no es JSON-LD: %+v", s)
			}
		case !anclado.MatchString(s.Src):
			t.Errorf("script %+v: src tiene que ser del mismo origen y estar anclado (^/_…$)", s)
		}
	}

	// Nadie sube JS: el de las islas lo pone el servicio desde el binario.
	for _, r := range p.Rutas {
		if !strings.HasPrefix(r, "^") || !strings.HasSuffix(r, "$") || strings.Contains(r, `\.js`) || strings.Contains(r, "..") {
			t.Errorf("ruta %q: anclada, sin .js y sin ..", r)
		}
	}

	if slices.Contains(p.CSS.ReglasAt, "import") || !slices.Contains(p.CSS.Prohibido, "@import") {
		t.Error("CSS: @import no puede entrar")
	}
	for _, u := range p.CSS.URLPrefijos {
		if u != "/_astro/" {
			t.Errorf("CSS: url() solo a /_astro/, no a %q", u)
		}
	}
	if p.CSS.DataURLMaxBytes <= 0 || p.CSS.DataURLMaxBytes > 2048 {
		t.Errorf("CSS: data: de como mucho 2 KB, no %d", p.CSS.DataURLMaxBytes)
	}

	for ext, tipo := range p.Tipos {
		if (ext == "html" && !strings.HasPrefix(tipo, "text/html")) || ext == "svg" {
			t.Errorf("tipo de %s: %q", ext, tipo)
		}
	}
	if p.Limites.Ficheros != 5000 || p.Limites.BytesHTML != 2<<20 || p.Limites.BytesTotal != 64<<20 {
		t.Errorf("límites distintos de §20.3: %+v", p.Limites)
	}
}
