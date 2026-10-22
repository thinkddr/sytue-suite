// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	"encoding/json"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Los idiomas de la galería son los del editor: los importa el catálogo de la web.
// Un idioma nuevo allí sin textos aquí caería al inglés sin que nadie lo viera.
func TestLosIdiomasSonLosDelEditor(t *testing.T) {
	if err := cargar(); err != nil {
		t.Fatal(err)
	}
	indice := webref.Read(t, "apps/frontend/packages/office/src/i18n/index.ts")
	coincidencias := regexp.MustCompile(`from '\./messages/([a-z]+)'`).FindAllStringSubmatch(indice, -1)
	web := make([]string, 0, len(coincidencias))
	for _, m := range coincidencias {
		web = append(web, m[1])
	}
	slices.Sort(web)
	aqui := slices.Sorted(maps.Keys(textos))
	if len(web) < 26 || !slices.Equal(web, aqui) {
		t.Fatalf("idiomas de la web %v, con textos aquí %v", web, aqui)
	}
}

// Cada plantilla tiene su esquema en la galería de la web; sin él, la tarjeta
// enseñaría solo el dibujo genérico de su tipo.
func TestCadaPlantillaTieneMiniatura(t *testing.T) {
	galeria := webref.Read(t, "apps/frontend/packages/office/src/components/GaleriaPlantillas.tsx")
	for _, p := range catalogo {
		if !regexp.MustCompile(`(?m)^ {2}` + p.clave + `: \{`).MatchString(galeria) {
			t.Errorf("%q no tiene esquema en ESQUEMAS de GaleriaPlantillas.tsx", p.clave)
		}
	}
}

// Cada idioma tiene EXACTAMENTE las claves del inglés, y ninguna vacía.
func TestCadaIdiomaTieneLasClavesDelIngles(t *testing.T) {
	if err := cargar(); err != nil {
		t.Fatal(err)
	}
	base := textos[idiomaBase]
	for idioma, m := range textos {
		for k := range base {
			if strings.TrimSpace(m[k]) == "" {
				t.Errorf("%s: falta %q", idioma, k)
			}
		}
		for k := range m {
			if _, ok := base[k]; !ok {
				t.Errorf("%s: sobra %q (no está en inglés)", idioma, k)
			}
		}
	}
}

// Todo hueco de un modelo tiene texto, y todo texto lo usa algún modelo (o es el
// nombre o la descripción de la galería): una clave huérfana es texto que se
// traduce veintiséis veces para nada.
func TestHuecosYTextosCasan(t *testing.T) {
	if err := cargar(); err != nil {
		t.Fatal(err)
	}
	usadas := map[string]bool{}
	for _, p := range catalogo {
		usadas[p.clave+".nombre"], usadas[p.clave+".descripcion"] = true, true
		for _, ext := range []string{".html", ".json"} {
			datos, err := fsys.ReadFile("modelos/" + p.clave + ext)
			if err != nil {
				continue
			}
			for _, m := range hueco.FindAllStringSubmatch(string(datos), -1) {
				usadas[m[2]] = true
				if _, ok := textos[idiomaBase][m[2]]; !ok {
					t.Errorf("%s%s: el hueco %q no tiene texto", p.clave, ext, m[2])
				}
			}
		}
	}
	for _, b := range catalogoBloques {
		usadas[b.clave+".nombre"], usadas[b.clave+".descripcion"] = true, true
		for _, ext := range []string{".html", ".json"} {
			datos, err := fsys.ReadFile("bloques/" + b.clave + ext)
			if err != nil {
				continue
			}
			for _, m := range hueco.FindAllStringSubmatch(string(datos), -1) {
				usadas[m[2]] = true
				if _, ok := textos[idiomaBase][m[2]]; !ok {
					t.Errorf("bloque %s%s: el hueco %q no tiene texto", b.clave, ext, m[2])
				}
			}
		}
	}
	for k := range textos[idiomaBase] {
		if !usadas[k] {
			t.Errorf("la clave %q no la usa ningún modelo", k)
		}
	}
}

// Cada plantilla, en cada idioma, sale como un contenido que el editor carga y
// que el exportador sabe escribir (es lo que se deja en la unidad al crear).
func TestCadaPlantillaSaleValidaEnTodosLosIdiomas(t *testing.T) {
	if err := cargar(); err != nil {
		t.Fatal(err)
	}
	for idioma := range textos {
		lista, err := Listar(idioma)
		if err != nil || len(lista) != len(catalogo) {
			t.Fatalf("%s: Listar = %d, %v", idioma, len(lista), err)
		}
		for _, p := range lista {
			info, contenido, err := Generar(p.Key, idioma)
			if err != nil {
				t.Fatalf("%s/%s: %v", idioma, p.Key, err)
			}
			if info != p || info.Name == "" || info.Description == "" {
				t.Errorf("%s/%s: info %+v contra la galería %+v", idioma, p.Key, info, p)
			}
			if strings.Contains(contenido, "{{") {
				t.Errorf("%s/%s: quedan huecos sin sustituir", idioma, p.Key)
			}
			comprobarModelo(t, idioma+"/"+p.Key, p.Kind, contenido)

			op, err := domain.ContentOp(p.Kind, contenido)
			if err != nil {
				t.Fatal(err)
			}
			estado := domain.NewState()
			if _, err := estado.Apply(op); err != nil {
				t.Fatal(err)
			}
			for _, f := range []domain.BlobFormat{domain.FormatODF, domain.FormatOOXML} {
				if _, _, _, err := domain.Export(p.Kind, info.Name, estado, f); err != nil {
					t.Errorf("%s/%s: exportar a %s: %v", idioma, p.Key, f, err)
				}
			}
		}
	}
}

// comprobarModelo mira lo que el editor necesita de cada tipo.
func comprobarModelo(t *testing.T, donde string, kind domain.Kind, contenido string) {
	t.Helper()
	switch kind {
	case domain.KindDocument:
		// HTML canónico del editor: el importador (que lo normaliza al subconjunto
		// que el editor pinta) lo devuelve tal cual.
		res, err := domain.Import("plantilla.html", []byte(contenido))
		if err != nil || res.Content != contenido {
			t.Errorf("%s: no es HTML canónico del editor (%v)\n quiere %s\n sale   %s", donde, err, contenido, res.Content)
		}
	case domain.KindSpreadsheet:
		var m struct {
			Sheets []struct {
				Name        string            `json:"name"`
				Cells       map[string]string `json:"cells"`
				Validations []struct {
					Values []string `json:"values"`
				} `json:"validations"`
			} `json:"sheets"`
		}
		if err := json.Unmarshal([]byte(contenido), &m); err != nil || len(m.Sheets) == 0 {
			t.Fatalf("%s: SheetModel inválido: %v", donde, err)
		}
		for _, h := range m.Sheets {
			// Excel y ODF no admiten más de 31 caracteres ni estos en el nombre.
			if n := len([]rune(h.Name)); n == 0 || n > 31 || strings.ContainsAny(h.Name, `[]:*?/\`) {
				t.Errorf("%s: nombre de hoja %q no se puede exportar", donde, h.Name)
			}
			for _, v := range h.Validations {
				if len(v.Values) == 0 || slices.Contains(v.Values, "") {
					t.Errorf("%s: lista de validación vacía", donde)
				}
			}
		}
	case domain.KindPresentation:
		var m struct {
			Slides []struct {
				ID    string `json:"id"`
				Boxes []struct {
					ID string `json:"id"`
				} `json:"boxes"`
			} `json:"slides"`
		}
		if err := json.Unmarshal([]byte(contenido), &m); err != nil || len(m.Slides) == 0 {
			t.Fatalf("%s: SlidesModel inválido: %v", donde, err)
		}
		ids := map[string]bool{}
		for _, s := range m.Slides {
			for _, b := range append([]struct {
				ID string `json:"id"`
			}{{ID: s.ID}}, s.Boxes...) {
				if b.ID == "" || ids[b.ID] {
					t.Errorf("%s: id %q vacío o repetido", donde, b.ID)
				}
				ids[b.ID] = true
			}
		}
	}
}

// El texto se escapa: lo que una traducción traiga de HTML sale como texto, y una
// comilla no rompe el JSON de una hoja.
func TestElTextoSeEscapa(t *testing.T) {
	if err := cargar(); err != nil {
		t.Fatal(err)
	}
	trampa := maps.Clone(textos[idiomaBase])
	trampa["carta.asunto"] = `<img src=x onerror=alert(1)>`
	trampa["presupuesto.titulo"] = `"comillas" y \barra`
	trampa["gastos.cat_viajes"] = `a "b" <c>`
	textos["zz"] = trampa
	t.Cleanup(func() { delete(textos, "zz") })

	_, carta, err := Generar("carta", "zz")
	if err != nil || strings.Contains(carta, "<img") || !strings.Contains(carta, "&lt;img") {
		t.Errorf("carta sin escapar: %v %s", err, carta)
	}
	for _, clave := range []string{"presupuesto", "gastos"} {
		_, hoja, err := Generar(clave, "zz")
		var m map[string]any
		if err != nil || json.Unmarshal([]byte(hoja), &m) != nil {
			t.Errorf("%s: JSON roto: %v", clave, err)
		}
	}
	_, gastos, _ := Generar("gastos", "zz")
	// La lista de validación va CRUDA (no es HTML): las comillas, escapadas para JSON.
	if !strings.Contains(gastos, `"a \"b\" <c>"`) {
		t.Errorf("valor crudo mal escapado: %s", gastos)
	}
}

func TestIdioma(t *testing.T) {
	for pedido, quiere := range map[string]string{
		"es": "es", "es-ES": "es", "PT_pt": "pt", "nb-NO": "nb", "xx": "en", "": "en",
	} {
		if got := Idioma(pedido); got != quiere {
			t.Errorf("Idioma(%q) = %q, quiere %q", pedido, got, quiere)
		}
	}
	if _, _, err := Generar("no-existe", "es"); !errors.Is(err, ErrNoExiste) {
		t.Errorf("clave desconocida: %v", err)
	}
}

// Sobrio: ni un pictograma en textos ni modelos.
func TestSinEmojis(t *testing.T) {
	ficheros, _ := fsys.ReadDir("textos")
	modelos, _ := fsys.ReadDir("modelos")
	for _, dir := range []struct {
		nombre   string
		entradas int
	}{{"textos", len(ficheros)}, {"modelos", len(modelos)}} {
		if dir.entradas == 0 {
			t.Fatalf("%s vacío", dir.nombre)
		}
	}
	for _, e := range append(ficheros, modelos...) {
		ruta := "textos/" + e.Name()
		if _, err := fsys.ReadFile(ruta); err != nil {
			ruta = "modelos/" + e.Name()
		}
		datos, _ := fsys.ReadFile(ruta)
		for _, r := range string(datos) {
			if unicode.Is(unicode.So, r) || (r >= 0x1F000 && r <= 0x1FAFF) {
				t.Errorf("%s: símbolo %U", ruta, r)
				break
			}
		}
	}
}

// Las plantillas y los bloques de Sytue son contenido legítimo: pasan por el saneador de
// HTML de Docs (domain.SanearContenido, el mismo contrato que el del navegador) sin que
// cambie un byte, en los veintiséis idiomas. Si alguno cambiara, crear un documento desde
// esa plantilla lo guardaría distinto de como se generó.
func TestLasPlantillasPasanElSaneadoSinCambiarUnByte(t *testing.T) {
	if err := cargar(); err != nil {
		t.Fatal(err)
	}
	vistas := 0
	for _, idioma := range slices.Sorted(maps.Keys(textos)) {
		lista, err := Listar(idioma)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range lista {
			_, contenido, err := Generar(p.Key, idioma)
			if err != nil {
				t.Fatalf("%s/%s: %v", idioma, p.Key, err)
			}
			if limpio := domain.SanearContenido(domain.ContentKey(p.Kind), contenido); limpio != contenido {
				t.Errorf("la plantilla %s (%s) cambia al sanearla:\n   era: %s\n queda: %s", p.Key, idioma, contenido, limpio)
			}
			vistas++
		}
		for _, kind := range []domain.Kind{domain.KindDocument, domain.KindSpreadsheet, domain.KindPresentation} {
			bloques, err := ListarBloques(kind, idioma)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range bloques {
				_, contenido, err := GenerarBloque(b.Key, idioma, nil)
				if err != nil {
					t.Fatalf("%s/%s: %v", idioma, b.Key, err)
				}
				// Un bloque de hoja son las celdas de UNA hoja: se mira como la hoja de un libro.
				entero := contenido
				if kind == domain.KindSpreadsheet {
					entero = `{"sheets":[` + contenido + `]}`
				}
				if limpio := domain.SanearContenido(domain.ContentKey(kind), entero); limpio != entero {
					t.Errorf("el bloque %s (%s) cambia al sanearlo:\n   era: %s\n queda: %s", b.Key, idioma, entero, limpio)
				}
				vistas++
			}
		}
	}
	if vistas < 26*20 {
		t.Fatalf("solo se miraron %d plantillas y bloques: el recorrido no las encuentra", vistas)
	}
}
