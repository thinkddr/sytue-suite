// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var reObjeto = regexp.MustCompile(`(?s)(\d+) 0 obj\n(.*?)\nendobj\n`)

// objetos devuelve el cuerpo de cada objeto del PDF por su número.
func objetos(b []byte) map[int]string {
	out := map[int]string{}
	for _, m := range reObjeto.FindAllSubmatch(b, -1) {
		n, _ := strconv.Atoi(string(m[1]))
		out[n] = string(m[2])
	}
	return out
}

// flujo descomprime el flujo de un objeto (Flate) o lo devuelve tal cual.
func flujo(t *testing.T, cuerpo string) string {
	t.Helper()
	i := strings.Index(cuerpo, "stream\n")
	j := strings.LastIndex(cuerpo, "\nendstream")
	if i < 0 || j < 0 {
		t.Fatalf("objeto sin flujo: %.60s", cuerpo)
	}
	datos := cuerpo[i+7 : j]
	if !strings.Contains(cuerpo[:i], "FlateDecode") {
		return datos
	}
	r, err := zlib.NewReader(strings.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

var (
	reBDC   = regexp.MustCompile(`/(\w+) (?:<<([^>]*)>>) BDC`)
	reMCID  = regexp.MustCompile(`/MCID (\d+)`)
	rePagN  = regexp.MustCompile(`/Type /Page /Parent`)
	reContN = regexp.MustCompile(`/Contents (\d+) 0 R`)
)

// problemasDeMarcado es un verificador MÍNIMO del marcado de contenido de cada página: todo
// texto está dentro de una secuencia marcada (real, con MCID, o artefacto), las secuencias
// están balanceadas y los MCID son 0…n-1 sin huecos. Es lo que PDF/UA 7.1 pide del flujo de
// contenido; veraPDF comprueba además el resto (estructura, fuentes, metadatos).
func problemasDeMarcado(t *testing.T, pdf []byte) []string {
	t.Helper()
	var out []string
	obj := objetos(pdf)
	pag := 0
	for n, c := range obj {
		if !rePagN.MatchString(c) {
			continue
		}
		pag++
		m := reContN.FindStringSubmatch(c)
		cn, _ := strconv.Atoi(m[1])
		cont := flujo(t, obj[cn])
		prof, esperado := 0, 0
		for i, ln := range strings.Split(cont, "\n") {
			switch {
			case strings.HasSuffix(ln, "BDC") || strings.HasSuffix(ln, "BMC"):
				if prof > 0 {
					out = append(out, fmt.Sprintf("objeto %d línea %d: secuencia marcada anidada", n, i))
				}
				prof++
				if mm := reMCID.FindStringSubmatch(ln); mm != nil {
					if id, _ := strconv.Atoi(mm[1]); id != esperado {
						out = append(out, fmt.Sprintf("objeto %d: MCID %d, esperaba %d", n, id, esperado))
					}
					esperado++
				}
			case ln == "EMC":
				prof--
			case strings.Contains(ln, " BT ") || strings.HasPrefix(ln, "BT "):
				if prof == 0 {
					out = append(out, fmt.Sprintf("objeto %d línea %d: texto fuera de contenido marcado", n, i))
				}
			case strings.Contains(ln, " l S") || strings.Contains(ln, " re f"):
				if prof == 0 {
					out = append(out, fmt.Sprintf("objeto %d línea %d: gráfico sin marcar", n, i))
				}
			}
			if prof < 0 {
				out = append(out, fmt.Sprintf("objeto %d línea %d: EMC de más", n, i))
				prof = 0
			}
		}
		if prof != 0 {
			out = append(out, fmt.Sprintf("objeto %d: secuencia sin cerrar", n))
		}
		if esperado == 0 && strings.Contains(cont, "BT") {
			out = append(out, fmt.Sprintf("objeto %d: texto sin ningún MCID", n))
		}
	}
	if pag == 0 {
		out = append(out, "no hay páginas")
	}
	return out
}

func TestElContenidoVaMarcado(t *testing.T) {
	b, err := completo("Marcado").Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if p := problemasDeMarcado(t, b); len(p) > 0 {
		t.Fatalf("marcado incorrecto:\n%s", strings.Join(p, "\n"))
	}
}

// El verificador tiene que verse en rojo: a un flujo real se le quitan las marcas y se queja.
func TestElVerificadorDetectaLaFaltaDeMarcado(t *testing.T) {
	b, _ := completo("Sin marcado").Bytes()
	obj := objetos(b)
	for _, c := range obj {
		if !rePagN.MatchString(c) {
			continue
		}
		cn, _ := strconv.Atoi(reContN.FindStringSubmatch(c)[1])
		sin := strings.ReplaceAll(reBDC.ReplaceAllString(flujo(t, obj[cn]), ""), "EMC\n", "")
		pdf := "1 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 3 0 R >>\nendobj\n3 0 obj\n<< /Length 1 >>\nstream\n" +
			sin + "\nendstream\nendobj\n"
		if p := problemasDeMarcado(t, []byte(pdf)); len(p) == 0 {
			t.Fatal("el verificador no ha visto que el texto no está marcado")
		}
		return
	}
	t.Fatal("sin páginas")
}

// El árbol de estructura: tipos estándar, encabezados sin saltos, listas anidadas, enlaces
// con su anotación y los idiomas de los tramos.
func TestArbolDeEstructura(t *testing.T) {
	b, _ := completo("Estructura").Bytes()
	todo := string(b)
	for _, quiero := range []string{
		"/Type /StructTreeRoot", "/ParentTree ", "/MarkInfo << /Marked true >>", "/DisplayDocTitle true",
		"/S /Document", "/S /H1", "/S /L ", "/S /LI ", "/S /Lbl ", "/S /LBody ", "/S /Link ", "/S /BlockQuote ",
		"/Type /OBJR", "/Subtype /Link", "/StructParent ", "/Lang (el)", "/Lang (fr)", "/Tabs /S",
		"<pdfuaid:part>1</pdfuaid:part>", "<dc:title>",
	} {
		if !strings.Contains(todo, quiero) {
			t.Errorf("falta %q", quiero)
		}
	}
	if strings.Contains(todo, "/S /H4") || !strings.Contains(todo, "/S /H2") {
		t.Error("H1 seguido de H4 debe etiquetarse H2 (sin saltar niveles)")
	}
	if strings.Contains(todo, "/URI (javascript") {
		t.Error("enlace peligroso")
	}
}

func TestSoloEnlacesSeguros(t *testing.T) {
	for h, ok := range map[string]bool{
		"https://sytue.eu/a b": true, "mailto:a@b.eu": true, "tel:+34123": true,
		"javascript:alert(1)": false, "file:///etc/passwd": false, "data:text/html,x": false, "//x": false, "https://": false,
	} {
		if _, got := urlSegura(h); got != ok {
			t.Errorf("urlSegura(%q) = %v, quería %v", h, got, ok)
		}
	}
}
