// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

// Un bloque de la organización es el documento de OTRA persona: lo que salga va al
// documento de quien lo inserta, así que el HTML pasa por el saneado del importador.
func TestExtraerBloqueSaneaElHTML(t *testing.T) {
	hostil := `<h2 onclick="alert(1)">Pie</h2><script>alert(2)</script>` +
		`<p>Texto <img src=x onerror="alert(3)"><a href="javascript:alert(4)">enlace</a></p><iframe src="https://example.invalid"></iframe>`
	out, err := ExtraerBloque(KindDocument, hostil)
	if err != nil {
		t.Fatal(err)
	}
	for _, malo := range []string{"<script", "onclick", "onerror", "javascript:", "<iframe", "alert("} {
		if strings.Contains(strings.ToLower(out), malo) {
			t.Errorf("el bloque conserva %q: %s", malo, out)
		}
	}
	if !strings.Contains(out, "Pie") || !strings.Contains(out, "Texto") || !strings.Contains(out, "enlace") {
		t.Errorf("el saneado se llevó el contenido: %s", out)
	}
}

func TestExtraerBloqueDeHojaYPresentacion(t *testing.T) {
	hoja, err := ExtraerBloque(KindSpreadsheet, hojaConTema)
	if err != nil || !strings.Contains(hoja, `"cells"`) || strings.Contains(hoja, `"sheets"`) || strings.Contains(hoja, `"name"`) {
		t.Errorf("bloque de hoja: %v %s", err, hoja)
	}
	pres, err := ExtraerBloque(KindPresentation, `{"slides":[{"id":"s1","boxes":[{"id":"a","html":"<p>uno</p>"}]},{"id":"s2","boxes":[]}],"theme":"cielo"}`)
	if err != nil || !strings.HasPrefix(pres, `{"boxes"`) || strings.Contains(pres, "s2") || strings.Contains(pres, "cielo") {
		t.Errorf("bloque de presentación (solo la primera diapositiva): %v %s", err, pres)
	}
	for _, vacio := range []string{`{"slides":[]}`, `{}`} {
		if _, err := ExtraerBloque(KindPresentation, vacio); !errors.Is(err, ErrInvalidFormat) {
			t.Errorf("presentación vacía %s: %v", vacio, err)
		}
	}
}

// Las celdas que un bloque marcó como de marca se actualizan con la hoja, estén donde
// estén, y «papel» las deja limpias: así «Actualizar a la marca vigente» llega a los bloques.
func TestTemaAlcanzaLasCeldasDeUnBloque(t *testing.T) {
	const hoja = `{"sheets":[{"name":"H","cells":{"A1":"Título","A5":"Total","C5":"=C3+C4"},"styles":{"A5":{"tema":"marca","fill":"#111111","color":"#FFFFFF"}}}]}`
	v2 := marcaDePrueba()
	out, err := AplicarTema(KindSpreadsheet, hoja, v2.Tema())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"A5":{"color":"#FFFFFF","fill":"#B45309","font":"Georgia","tema":"marca"}`) {
		t.Errorf("la celda del bloque no recibió la marca vigente: %s", out)
	}
	if !strings.Contains(out, `"C5":"=C3+C4"`) {
		t.Errorf("el tema tocó una fórmula: %s", out)
	}
	papel, _ := TemaPorID(IDTemaPapel, nil)
	limpio, err := AplicarTema(KindSpreadsheet, out, papel)
	if err != nil || strings.Contains(limpio, "B45309") || strings.Contains(limpio, `"tema"`) {
		t.Errorf("papel no limpió las celdas de bloque: %v %s", err, limpio)
	}
}

// El bloque de una hoja o de una presentación también es de otra persona: el HTML de sus
// celdas y de sus cajas sale saneado, y un origen de imagen que no sea una imagen, fuera.
func TestExtraerBloqueSaneaCeldasYCajas(t *testing.T) {
	hoja, err := ExtraerBloque(KindSpreadsheet, `{"sheets":[{"name":"H","cells":{`+
		`"A1":"<b onclick=\"alert(1)\">Total</b><script>alert(2)</script>",`+
		`"A2":"<img src=x onerror=alert(3)>dos <a href=\"javascript:alert(4)\">enlace</a>",`+
		`"A3":"=SUM(B1:B2)","A4":"3 &lt; 4 &amp; 5","A5":"<i>uno</i><br><u>dos</u>"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, malo := range []string{"script", "onclick", "onerror", "javascript", "<img", "<a ", "alert(2)"} {
		if strings.Contains(hoja, malo) {
			t.Errorf("la celda conserva %q: %s", malo, hoja)
		}
	}
	for _, bueno := range []string{`"A1":"<b>Total</b>"`, `"A2":"dos enlace"`, `"A3":"=SUM(B1:B2)"`, `"A4":"3 &lt; 4 &amp; 5"`, `"A5":"<i>uno</i><br><u>dos</u>"`} {
		if !strings.Contains(hoja, bueno) {
			t.Errorf("falta %s en %s", bueno, hoja)
		}
	}
	pres, err := ExtraerBloque(KindPresentation, `{"slides":[{"id":"s","boxes":[`+
		`{"id":"a","x":1,"y":1,"w":1,"h":1,"html":"<h1 onclick=\"alert(1)\">Hola</h1><script>alert(2)</script><p><a href=\"javascript:alert(3)\">x</a></p>"},`+
		`{"id":"b","x":1,"y":1,"w":1,"h":1,"kind":"image","html":"","src":"javascript:alert(4)"},`+
		`{"id":"c","x":1,"y":1,"w":1,"h":1,"kind":"image","html":"","src":"data:image/svg+xml,<svg onload=alert(5)>"},`+
		`{"id":"d","x":1,"y":1,"w":1,"h":1,"kind":"image","html":"","src":"data:image/png;base64,AAAA"},`+
		`{"id":"e","x":1,"y":1,"w":1,"h":1,"kind":"media","html":"","src":"https://example.org/v.mp4","poster":"data:text/html,x"}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, malo := range []string{"script", "onclick", "javascript", "svg", "onload", "data:text"} {
		if strings.Contains(pres, malo) {
			t.Errorf("la caja conserva %q: %s", malo, pres)
		}
	}
	for _, bueno := range []string{"<h1>Hola</h1>", `"src":"data:image/png;base64,AAAA"`, `"src":"https://example.org/v.mp4"`} {
		if !strings.Contains(pres, bueno) {
			t.Errorf("falta %s en %s", bueno, pres)
		}
	}
}
