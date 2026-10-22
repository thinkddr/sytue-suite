// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

func marcaDePrueba() MarcaOrg {
	return MarcaOrg{Version: 3, Name: "Acme", Bg: "#FFF7ED", Ink: "#1C1917", Accent: "#B45309", HeadingFont: "Georgia", BodyFont: "Trebuchet MS"}
}

// El catálogo común REUTILIZA los presets de Diapos: ninguno se queda sin acento, y un
// preset nuevo de Diapos que no esté aquí rompe este test en vez de salir sin tema.
func TestCatalogoTemasCubreLosPresetsDeDiapos(t *testing.T) {
	cat := CatalogoTemas()
	ids := map[string]bool{}
	for _, c := range cat {
		if ids[c.ID] {
			t.Errorf("id repetido %q", c.ID)
		}
		ids[c.ID] = true
		for _, col := range []string{c.Bg, c.Ink, c.Accent, c.AccentInk} {
			if normalizeColor(col) == "" || !strings.HasPrefix(col, "#") {
				t.Errorf("%s: color mal formado %q", c.ID, col)
			}
		}
	}
	if !ids[IDTemaPapel] {
		t.Error("falta el tema papel")
	}
	for _, p := range slideThemePresets {
		if !ids[p.ID] {
			t.Errorf("el preset de Diapos %q no está en el catálogo común", p.ID)
		}
		if _, ok := acentosTema[p.ID]; !ok {
			t.Errorf("el preset de Diapos %q no tiene acento", p.ID)
		}
	}
}

func TestMarcaNormalizar(t *testing.T) {
	m := marcaDePrueba()
	m.Accent = "b45309"
	if err := m.Normalizar(); err == nil {
		t.Error("un color sin # (y no hex de 3/6 con #) debe rechazarse")
	}
	m = marcaDePrueba()
	m.Accent = "#b45"
	if err := m.Normalizar(); err != nil || m.Accent != "#BB4455" {
		t.Errorf("hex corto: %v %s", err, m.Accent)
	}
	for _, mala := range []func(*MarcaOrg){
		func(m *MarcaOrg) { m.Name = "  " },
		func(m *MarcaOrg) { m.Name = strings.Repeat("x", 61) },
		func(m *MarcaOrg) { m.Bg = "rojo" },
		func(m *MarcaOrg) { m.HeadingFont = `Arial"; background:url(x)` },
	} {
		m := marcaDePrueba()
		mala(&m)
		if err := m.Normalizar(); !errors.Is(err, ErrInvalidOp) {
			t.Errorf("marca inválida aceptada: %+v (%v)", m, err)
		}
	}
	// La tinta sobre el acento se lee: blanca sobre oscuro, oscura sobre claro.
	oscuro, claro := marcaDePrueba(), marcaDePrueba()
	claro.Accent = "#FDE68A"
	if oscuro.Tema().AccentInk != "#FFFFFF" || claro.Tema().AccentInk != "#0A0A0A" {
		t.Errorf("tinta sobre acento: %s / %s", oscuro.Tema().AccentInk, claro.Tema().AccentInk)
	}
}

func textoPlano(t *testing.T, contenido string) string {
	t.Helper()
	nodos, err := xhtml.ParseFragment(strings.NewReader(contenido), nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	var rec func(n *xhtml.Node)
	rec = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data + "|")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	for _, n := range nodos {
		rec(n)
	}
	return b.String()
}

const docConTema = `<h1>Informe</h1><p>Hola <strong>mundo</strong></p>` +
	`<h2 style="color:#FF0000">Propio</h2><table><tbody><tr><th>Concepto</th><th style="background-color:#00FF00">Mío</th></tr>` +
	`<tr><td>uno</td><td>2</td></tr></tbody></table><ul><li>a</li></ul><blockquote>cita</blockquote>`

func TestTemaEnDocumentoNoTocaElTexto(t *testing.T) {
	marca := marcaDePrueba().Tema()
	out, err := AplicarTema(KindDocument, docConTema, marca)
	if err != nil {
		t.Fatal(err)
	}
	if textoPlano(t, out) != textoPlano(t, docConTema) {
		t.Fatalf("el tema cambió el texto:\n%s", out)
	}
	for _, quiere := range []string{
		`<h1 style="color:#B45309;font-family:Georgia" data-tema="marca:color,font-family">`,
		`<p style="font-family:Trebuchet MS" data-tema="marca:font-family">`,
		`<th style="background-color:#B45309;color:#FFFFFF;font-family:Georgia" data-tema="marca:background-color,color,font-family">Concepto`,
		`<li style="font-family:Trebuchet MS"`,
		`<td style="font-family:Trebuchet MS"`,
	} {
		if !strings.Contains(out, quiere) {
			t.Errorf("falta %s en:\n%s", quiere, out)
		}
	}
	// Lo que el autor fijó a mano NO se pisa: el color del h2 y la cabecera con relleno propio.
	if !strings.Contains(out, `<h2 style="color:#FF0000;font-family:Georgia" data-tema="marca:font-family">`) {
		t.Errorf("el color propio del h2 debe conservarse (y solo recibir la fuente): %s", out)
	}
	if !strings.Contains(out, `<th style="background-color:#00FF00">Mío`) {
		t.Errorf("una cabecera con colores propios se queda entera: %s", out)
	}
	// Idempotente.
	if otra, _ := AplicarTema(KindDocument, out, marca); otra != out {
		t.Errorf("aplicar dos veces cambia el resultado:\n%s\n%s", out, otra)
	}
	// Otro tema reemplaza lo que puso el anterior, no se acumula.
	carbon, _ := TemaPorID("carbon", nil)
	cb, _ := AplicarTema(KindDocument, out, carbon)
	if strings.Contains(cb, "B45309") || strings.Contains(cb, `data-tema="marca`) || !strings.Contains(cb, `data-tema="carbon`) || !strings.Contains(cb, "#1F2937") {
		t.Errorf("cambio de tema: %s", cb)
	}
	// «Papel» deja el documento como estaba (a la normalización del HTML).
	papel, _ := TemaPorID(IDTemaPapel, nil)
	vuelta, _ := AplicarTema(KindDocument, cb, papel)
	base, _ := AplicarTema(KindDocument, docConTema, papel) // sin tema = sin cambio
	if base != docConTema {
		t.Errorf("papel sobre un documento sin tema debe dejarlo intacto: %s", base)
	}
	if strings.Contains(vuelta, "data-tema") || strings.Contains(vuelta, "Georgia") || strings.Contains(vuelta, "#1F2937") {
		t.Errorf("papel no quitó el tema: %s", vuelta)
	}
	if !strings.Contains(vuelta, `<h2 style="color:#FF0000">`) {
		t.Errorf("papel debe respetar lo propio: %s", vuelta)
	}
}

func TestTemaEnDocumentoConserva_ColasDeUrlEnElEstilo(t *testing.T) {
	// Un style con un url(data:…;base64,…) trae «;» dentro: partirlo no puede romperlo.
	doc := `<p style="background-image:url(data:image/png;base64,AAAA)">x</p>`
	marca := marcaDePrueba().Tema()
	out, err := AplicarTema(KindDocument, doc, marca)
	if err != nil || !strings.Contains(out, "url(data:image/png;base64,AAAA)") {
		t.Fatalf("%v %s", err, out)
	}
}

const hojaConTema = `{"active":0,"sheets":[{"id":"h1","name":"Datos","cells":{"A1":"<b>Mes</b>","B1":"Total","A2":"enero","B2":"10"},` +
	`"formats":{"B2":{"id":"currency","decimals":2}},"styles":{"B1":{"fill":"#00FF00","color":"#000000"},"A2":{"color":"#112233"}},` +
	`"charts":[{"id":"c1","kind":"bar","range":"A1:B2","extra":{"n":12345678901234567890}}],"freeze":{"cols":0,"rows":1}}],"names":[{"name":"Total","ref":"Datos!B2"}]}`

func TestTemaEnHojaSoloTocaLaCabecera(t *testing.T) {
	marca := marcaDePrueba().Tema()
	out, err := AplicarTema(KindSpreadsheet, hojaConTema, marca)
	if err != nil {
		t.Fatal(err)
	}
	var antes, despues map[string]any
	dec := func(s string, m *map[string]any) {
		d := json.NewDecoder(strings.NewReader(s))
		d.UseNumber()
		if err := d.Decode(m); err != nil {
			t.Fatal(err)
		}
	}
	dec(hojaConTema, &antes)
	dec(out, &despues)
	h0 := antes["sheets"].([]any)[0].(map[string]any)
	h1 := despues["sheets"].([]any)[0].(map[string]any)
	// Todo lo que no es el estilo de la cabecera es IGUAL, también lo que este código no conoce.
	for _, k := range []string{"id", "name", "cells", "formats", "charts", "freeze"} {
		if !reflect.DeepEqual(h0[k], h1[k]) {
			t.Errorf("%s cambió: %v -> %v", k, h0[k], h1[k])
		}
	}
	if !reflect.DeepEqual(antes["names"], despues["names"]) {
		t.Error("los nombres definidos cambiaron")
	}
	est := h1["styles"].(map[string]any)
	a1 := est["A1"].(map[string]any)
	if a1["fill"] != "#B45309" || a1["color"] != "#FFFFFF" || a1["font"] != "Georgia" || a1["tema"] != "marca" {
		t.Errorf("cabecera A1: %v", a1)
	}
	if b1 := est["B1"].(map[string]any); b1["fill"] != "#00FF00" || b1["tema"] != nil {
		t.Errorf("la cabecera con relleno propio no se pisa: %v", b1)
	}
	if a2 := est["A2"].(map[string]any); len(a2) != 1 || a2["color"] != "#112233" {
		t.Errorf("las celdas de datos no se tocan: %v", a2)
	}
	if strings.Contains(out, "\\u003c") {
		t.Error("el HTML de las celdas no debe salir escapado")
	}
	if otra, _ := AplicarTema(KindSpreadsheet, out, marca); otra != out {
		t.Errorf("no es idempotente:\n%s\n%s", out, otra)
	}
	// Cambiar de tema reemplaza el relleno; papel lo quita.
	carbon, _ := TemaPorID("carbon", nil)
	cb, _ := AplicarTema(KindSpreadsheet, out, carbon)
	if !strings.Contains(cb, "#1F2937") || strings.Contains(cb, "#B45309") {
		t.Errorf("cambio de tema: %s", cb)
	}
	papel, _ := TemaPorID(IDTemaPapel, nil)
	vuelta, _ := AplicarTema(KindSpreadsheet, cb, papel)
	var v map[string]any
	dec(vuelta, &v)
	vs := v["sheets"].([]any)[0].(map[string]any)["styles"].(map[string]any)
	if _, hay := vs["A1"]; hay {
		t.Errorf("papel dejó estilo de tema en A1: %v", vs["A1"])
	}
	// Una hoja sin nada que tematizar queda exactamente igual.
	if sin, _ := AplicarTema(KindSpreadsheet, `{"sheets":[{"name":"V","cells":{}}]}`, marca); sin != `{"sheets":[{"name":"V","cells":{}}]}` {
		t.Errorf("hoja vacía: %s", sin)
	}
}

func TestTemaEnPresentacion(t *testing.T) {
	const pres = `{"slides":[{"id":"s1","boxes":[{"id":"b","x":1,"y":2,"w":3,"h":4,"kind":"text","html":"<p>hola</p>"}]}],"themes":[{"id":"custom-mio","name":"Mío","bg":"#111111","ink":"#EEEEEE"}],"unknown":{"a":[1,2]}}`
	var antes map[string]any
	_ = json.Unmarshal([]byte(pres), &antes)
	comprobarIntacto := func(out string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(m["slides"], antes["slides"]) || !reflect.DeepEqual(m["unknown"], antes["unknown"]) {
			t.Errorf("el tema tocó el contenido: %s", out)
		}
		return m
	}
	// Un preset del catálogo: solo `theme`.
	pizarra, _ := TemaPorID("pizarra", nil)
	out, err := AplicarTema(KindPresentation, pres, pizarra)
	if err != nil {
		t.Fatal(err)
	}
	if m := comprobarIntacto(out); m["theme"] != "pizarra" || len(m["themes"].([]any)) != 1 {
		t.Errorf("preset: %s", out)
	}
	// La marca: un tema propio «custom-marca» que se REEMPLAZA al volver a aplicarla.
	marca := marcaDePrueba().Tema()
	out, _ = AplicarTema(KindPresentation, out, marca)
	m := comprobarIntacto(out)
	temas := m["themes"].([]any)
	if m["theme"] != "custom-marca" || len(temas) != 2 {
		t.Fatalf("marca: %s", out)
	}
	nueva := marcaDePrueba()
	nueva.Version, nueva.Bg = 4, "#FFFFFF"
	out2, _ := AplicarTema(KindPresentation, out, nueva.Tema())
	if m2 := comprobarIntacto(out2); len(m2["themes"].([]any)) != 2 || !strings.Contains(out2, `"#FFFFFF"`) {
		t.Errorf("nueva versión de la marca: %s", out2)
	}
	if otra, _ := AplicarTema(KindPresentation, out, marca); otra != out {
		t.Errorf("no es idempotente:\n%s\n%s", out, otra)
	}
	// Papel: sin tema, y la marca ya no está; el tema propio del autor sí.
	papel, _ := TemaPorID(IDTemaPapel, nil)
	vuelta, _ := AplicarTema(KindPresentation, out, papel)
	mv := comprobarIntacto(vuelta)
	if mv["theme"] != nil || len(mv["themes"].([]any)) != 1 {
		t.Errorf("papel: %s", vuelta)
	}
	// Con el tope de temas propios llenos, no cabe la marca y se dice.
	lleno := map[string]any{"slides": []any{}, "themes": []any{}}
	for i := 0; i < maxTemasPropios; i++ {
		lleno["themes"] = append(lleno["themes"].([]any), map[string]any{"id": "custom-x" + string(rune('a'+i))})
	}
	b, _ := json.Marshal(lleno)
	if _, err := AplicarTema(KindPresentation, string(b), marca); !errors.Is(err, ErrInvalidOp) {
		t.Errorf("tope de temas: %v", err)
	}
}

func TestOpTema(t *testing.T) {
	st := stateWithContent(t, "doc:html", docConTema)
	marca := marcaDePrueba().Tema()
	op, cambia, err := OpTema(KindDocument, st, marca)
	if err != nil || !cambia || op.Kind != OpSet || op.Key != "doc:html" || op.Lamport <= st.Registers["doc:html"].Lamport {
		t.Fatalf("OpTema: %+v %v %v", op, cambia, err)
	}
	if _, err := st.Apply(op); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.documentHTML(), `data-tema="marca:`) {
		t.Fatal("la operación no aplicó el tema")
	}
	// Ya aplicado: nada que escribir.
	if _, cambia, _ := OpTema(KindDocument, st, marca); cambia {
		t.Error("aplicar de nuevo no debe escribir")
	}
	// Un documento sin contenido no se tematiza.
	if _, _, err := OpTema(KindDocument, NewState(), marca); !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("sin contenido: %v", err)
	}
	// Un contenido que no es JSON (hoja) es un formato inválido, no un pánico.
	if _, err := AplicarTema(KindSpreadsheet, "no soy json", marca); !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("hoja ilegible: %v", err)
	}
}
