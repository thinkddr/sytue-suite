// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

var reDobleExclam = regexp.MustCompile(`!!`)

// esPictografico comprueba si un rune pertenece al conjunto Unicode Extended_Pictographic.
func esPictografico(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1F5FF: // Misc Symbols and Pictographs
		return true
	case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport and Map
		return true
	case r >= 0x1F700 && r <= 0x1F77F: // Alchemical Symbols
		return true
	case r >= 0x1F780 && r <= 0x1F7FF: // Geometric Shapes Extended
		return true
	case r >= 0x1F800 && r <= 0x1F8FF: // Supplemental Arrows-C
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols and Pictographs
		return true
	case r >= 0x1FA00 && r <= 0x1FA6F: // Chess Symbols
		return true
	case r >= 0x1FA70 && r <= 0x1FAFF: // Symbols and Pictographs Extended-A
		return true
	case r >= 0x2600 && r <= 0x26FF: // Misc Symbols
		return true
	case r >= 0x2700 && r <= 0x27BF: // Dingbats
		return true
	case r >= 0x2300 && r <= 0x23FF: // Misc Technical
		return true
	case r >= 0x2B50 && r <= 0x2B55:
		return true
	}
	return false
}

func contienePictografico(data []byte) bool {
	s := string(data)
	for _, r := range s {
		if esPictografico(r) {
			return true
		}
	}
	return false
}

func TestSinSlop_DetectorFallaConEmojiYDobleExclamacion(t *testing.T) {
	// Verificar que el guardián detecta emojis y doble exclamación (no es falso positivo ni falso negativo)
	if !contienePictografico([]byte("texto con emoji 😀")) {
		t.Errorf("contienePictografico no detectó emoji")
	}
	if !contienePictografico([]byte("diseño 🚀 innovador")) {
		t.Errorf("contienePictografico no detectó cohete")
	}
	if contienePictografico([]byte("texto normal sin símbolos")) {
		t.Errorf("contienePictografico dio falso positivo")
	}
	if !reDobleExclam.MatchString("¡¡Oferta!!") {
		t.Errorf("reDobleExclam no detectó doble exclamación")
	}
	if reDobleExclam.MatchString("¡Oferta normal!") {
		t.Errorf("reDobleExclam dio falso positivo en exclamación simple")
	}
}

func TestSinSlop_CeroEmojisYDobleExclamacion(t *testing.T) {
	entries, err := plantillasFS.ReadDir(".")
	if err != nil {
		t.Fatalf("leer directorio raiz: %v", err)
	}

	comprobarFichero := func(nombre string, data []byte) {
		if contienePictografico(data) {
			t.Errorf("fichero %s contiene caracteres pictográficos (emojis)", nombre)
		}
		if reDobleExclam.Match(data) {
			t.Errorf("fichero %s contiene doble exclamación (!!)", nombre)
		}
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := plantillasFS.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("leer %s: %v", e.Name(), err)
		}
		comprobarFichero(e.Name(), data)
	}

	secEntries, err := plantillasFS.ReadDir("secciones")
	if err != nil {
		t.Fatalf("leer directorio secciones: %v", err)
	}
	for _, e := range secEntries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join("secciones", e.Name())
		data, err := plantillasFS.ReadFile(path)
		if err != nil {
			t.Fatalf("leer %s: %v", path, err)
		}
		comprobarFichero(path, data)
	}
}

func TestSinSlop_PlantillasCompletas(t *testing.T) {
	clavesEsperadas := []string{
		"tienda", "fabrica", "producto", "restaurante", "servicios",
		"reformas", "clinica", "alojamiento", "academia", "portfolio", "blanco",
	}

	for _, clave := range clavesEsperadas {
		t.Run(clave, func(t *testing.T) {
			p, err := ObtenerPlantilla(clave)
			if err != nil {
				t.Fatalf("ObtenerPlantilla(%s) error = %v", clave, err)
			}

			// Validar crudo y semántica
			raw, err := plantillasFS.ReadFile(clave + ".json")
			if err != nil {
				t.Fatalf("leer crudo: %v", err)
			}
			var wrapper struct {
				IR json.RawMessage `json:"ir"`
			}
			if err := json.Unmarshal(raw, &wrapper); err != nil {
				t.Fatalf("unmarshal wrapper: %v", err)
			}
			probsCrudo := ir.ValidarCrudo(wrapper.IR)
			for _, pr := range probsCrudo {
				if pr.Severity == "error" {
					t.Errorf("error de validación crudo en %s: %s (%s)", clave, pr.Code, pr.Path)
				}
			}

			cat := &ir.Catalogo{
				Fuentes:    []string{"inter", "lora", "playfair-display", "source-serif-4", "montserrat", "work-sans", "manrope", "ibm-plex-sans", "dm-sans", "figtree", "noto-sans", "noto-serif"},
				Biblioteca: []string{"galeria", "logo"},
			}
			probs := ir.Validar(&p.IR, cat, ir.ModoBorrador)
			for _, pr := range probs {
				if pr.Severity == "error" {
					t.Errorf("error semántico en %s: %s (%s) %s", clave, pr.Code, pr.Path, pr.Detail)
				}
			}

			// Sin seo.business en plantillas
			if p.IR.SEO.Business != nil {
				t.Errorf("plantilla %s no debe traer seo.business (§13.1)", clave)
			}

			// Exactamente 1 h1 por página
			for idx, pag := range p.IR.Pages {
				h1Count := 0
				contarH1EnNodos(pag.Sections, &h1Count)
				if pag.ShowHeader && p.IR.Header != nil {
					contarH1EnNodos([]ir.Nodo{*p.IR.Header}, &h1Count)
				}
				if pag.ShowFooter && p.IR.Footer != nil {
					contarH1EnNodos([]ir.Nodo{*p.IR.Footer}, &h1Count)
				}
				if h1Count != 1 {
					t.Errorf("plantilla %s página %d (%s) tiene %d etiquetas h1; want exactamente 1", clave, idx, pag.ID, h1Count)
				}
			}

			// Comprobaciones específicas de cada plantilla
			switch clave {
			case "tienda":
				if !p.Store {
					t.Errorf("plantilla tienda debe tener store: true")
				}
			case "producto":
				if !p.Store {
					t.Errorf("plantilla producto debe tener store: true")
				}
			case "fabrica":
				if p.Store {
					t.Errorf("plantilla fabrica no debe tener store: true")
				}
				// Debe tener tabla técnica y botón «Solicitar ficha técnica»
				rawStr := string(raw)
				if !strings.Contains(rawStr, `"type":"table"`) && !strings.Contains(rawStr, `"type": "table"`) {
					t.Errorf("plantilla fabrica debe incluir tabla técnica")
				}
				if !strings.Contains(rawStr, "Solicitar ficha técnica") {
					t.Errorf("plantilla fabrica debe incluir botón «Solicitar ficha técnica»")
				}
			case "alojamiento", "academia", "portfolio", "blanco":
				if p.Store {
					t.Errorf("plantilla %s no debe tener store: true", clave)
				}
				if clave == "blanco" {
					if len(p.IR.Pages) != 2 {
						t.Errorf("plantilla blanco debe tener exactamente 2 páginas, got %d", len(p.IR.Pages))
					}
				} else {
					if len(p.IR.Pages) < 3 || len(p.IR.Pages) > 5 {
						t.Errorf("plantilla %s debe tener entre 3 y 5 páginas, got %d", clave, len(p.IR.Pages))
					}
				}
			}
		})
	}
}

func contarH1EnNodos(nodos []ir.Nodo, count *int) {
	for _, n := range nodos {
		if n.Type == "text" && n.Tag == "h1" {
			*count++
		}
		if len(n.Children) > 0 {
			contarH1EnNodos(n.Children, count)
		}
	}
}

func TestSinSlop_SeccionesPlantilla(t *testing.T) {
	secciones, err := ListarSecciones()
	if err != nil {
		t.Fatalf("ListarSecciones() error = %v", err)
	}

	// 34 secciones en total
	total := 0
	for _, secList := range secciones {
		total += len(secList)
	}
	if total != 34 {
		t.Errorf("total secciones = %d; want 34", total)
	}

	// 2 por cada kind de TIPOS_SECCION, 3 para header y footer
	for _, kind := range ir.TiposSeccion {
		count := len(secciones[kind])
		want := 2
		if kind == "header" || kind == "footer" {
			want = 3
		}
		if count != want {
			t.Errorf("secciones[%q] = %d; want %d", kind, count, want)
		}
	}

	// Cada sección debe ser un nodo válido
	for kind, secList := range secciones {
		for i, sec := range secList {
			if sec.Type != "section" {
				t.Errorf("seccion %s-%d tiene type %q; want section", kind, i+1, sec.Type)
			}
			if sec.Kind != kind {
				t.Errorf("seccion %s-%d tiene kind %q; want %s", kind, i+1, sec.Kind, kind)
			}
			if sec.Layout == nil {
				t.Errorf("seccion %s-%d debe tener layout definido", kind, i+1)
			}
		}
	}
}

func TestSinSlop_ContrasteAA_Temas(t *testing.T) {
	catalogo, err := CargarTemas()
	if err != nil {
		t.Fatalf("CargarTemas() error = %v", err)
	}

	for _, pal := range catalogo.Palettes {
		t.Run(pal.ID, func(t *testing.T) {
			colors := pal.Colors
			// text / background >= 4.5
			crTextBg, err := calcularContraste(colors["text"], colors["background"])
			if err != nil {
				t.Fatalf("calcularContraste text/background: %v", err)
			}
			if crTextBg < 4.5 {
				t.Errorf("paleta %s text/background ratio = %.2f; want >= 4.5 (AA)", pal.ID, crTextBg)
			}

			// on_primary / primary >= 4.5
			crPri, err := calcularContraste(colors["on_primary"], colors["primary"])
			if err != nil {
				t.Fatalf("calcularContraste on_primary/primary: %v", err)
			}
			if crPri < 4.5 {
				t.Errorf("paleta %s on_primary/primary ratio = %.2f; want >= 4.5 (AA)", pal.ID, crPri)
			}

			// muted / background >= 4.5
			crMuted, err := calcularContraste(colors["muted"], colors["background"])
			if err != nil {
				t.Fatalf("calcularContraste muted/background: %v", err)
			}
			if crMuted < 4.5 {
				t.Errorf("paleta %s muted/background ratio = %.2f; want >= 4.5 (AA)", pal.ID, crMuted)
			}
		})
	}
}

func hexToRGB(hex string) (float64, float64, float64, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("hex inválido: %s", hex)
	}
	val, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, err
	}
	r := float64((val>>16)&0xFF) / 255.0
	g := float64((val>>8)&0xFF) / 255.0
	b := float64(val&0xFF) / 255.0
	return r, g, b, nil
}

func sRGBToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func luminancia(r, g, b float64) float64 {
	return 0.2126*sRGBToLinear(r) + 0.7152*sRGBToLinear(g) + 0.0722*sRGBToLinear(b)
}

func calcularContraste(hex1, hex2 string) (float64, error) {
	r1, g1, b1, err := hexToRGB(hex1)
	if err != nil {
		return 0, err
	}
	r2, g2, b2, err := hexToRGB(hex2)
	if err != nil {
		return 0, err
	}
	l1 := luminancia(r1, g1, b1)
	l2 := luminancia(r2, g2, b2)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05), nil
}
