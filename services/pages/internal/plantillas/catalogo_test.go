// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// Guardián del catálogo de plantillas: recorre TODAS las plantillas y exige lo que una persona sin
// conocimientos informáticos necesita para publicar sin mirar el código. Compara por igualdad con
// sus listas de excepciones (no por pertenencia): si una excepción deja de hacer falta, falla.

// categoriasValidas son los grupos de la galería; la interfaz (NuevoSitioApp) tiene la misma lista.
var categoriasValidas = []string{
	"belleza", "comida", "comercio", "oficios", "motor", "profesionales", "salud", "deporte",
	"educacion", "creativos", "turismo", "local", "comunidad", "web",
}

// sinTelefono, sinMapa y sinHorario son los sectores a los que no les corresponde (con el porqué).
var (
	sinTelefono = map[string]string{
		"landing-app":         "Negocio online: el contacto es el formulario de aviso y el correo; teléfono y WhatsApp son opcionales.",
		"curriculum-personal": "Web personal: se contacta por correo y formulario; el teléfono y WhatsApp son opcionales y se añaden desde el editor.",
	}
	sinMapa = map[string]string{
		"landing-app":          "Producto digital sin local: no hay dirección que mostrar en un mapa.",
		"comunidad-vecinos":    "Un edificio no necesita mapa: los vecinos ya saben dónde viven y la dirección figura en el pie y en contacto.",
		"portfolio":            "Profesional que trabaja a distancia: no atiende al público en un local.",
		"tienda":               "Tienda solo online: no hay local abierto al público; la dirección que se muestra es la de devoluciones.",
		"producto":             "Venta online de un solo producto: no hay local abierto al público; la dirección que se muestra es la de devoluciones.",
		"curriculum-personal":  "Profesional sin local abierto al público: no hay dirección que mostrar en un mapa.",
		"traductor-interprete": "Trabaja a distancia: la dirección es solo postal, para originales y traducciones juradas en papel.",
		"blanco":               "Plantilla sin sector: no se sabe si hay local; el mapa se añade desde el editor si hace falta.",
	}
	sinHorario = map[string]string{
		"landing-app":         "Producto digital sin atención presencial: no tiene horario de apertura.",
		"portfolio":           "Sin horario de apertura: atiende por correo y mensaje, y la disponibilidad va en texto.",
		"curriculum-personal": "Sin horario de apertura: no atiende en un local; responde por correo.",
		"proximamente":        "Aún no ha abierto: no hay horario de apertura que dar; se añade desde el editor cuando abra.",
		"blanco":              "Plantilla sin sector: no hay horario que dar por supuesto; se añade desde el editor si hace falta.",
	}
)

const maxBytesPlantilla = 90_000

// minimoCatalogo es un trinquete: sube con cada tanda de plantillas y evita que una desaparezca sin
// querer. Hoy son 144: las 11 de partida, rehechas, y 133 sectores nuevos.
const minimoCatalogo = 144

var (
	reTelDemo  = regexp.MustCompile(`^tel:\+?\d{9,}$`)
	reLorem    = regexp.MustCompile(`(?i)lorem|ipsum|dolor sit amet`)
	reCategory = regexp.MustCompile(`^[a-z]+$`)
	reClave    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// reJerga caza la jerga de marketing y las promesas que no se pueden sostener sin datos del negocio.
	reJerga = regexp.MustCompile(`(?i)soluciones integrales|\blíder|excelencia|innovador|vanguardia|sinergia|experiencia única|los mejores|mejor del mercado|años de experiencia|garantizamos|número uno|primera calidad|a la última`)
	// reInstruccion caza los textos que le hablan a quien edita la plantilla («cámbialos por los tuyos»).
	reInstruccion = regexp.MustCompile(`(?i)c[aá]mbial|ajústal|cambia (esta|cada fila)|indica aqu[ií]|escribe aqu[ií]|rellena aqu[ií]|pon aqu[ií]|sustituye est|de ejemplo`)
)

// recorre visita cada nodo del árbol.
func recorre(nodos []ir.Nodo, f func(n ir.Nodo)) {
	for _, n := range nodos {
		f(n)
		recorre(n.Children, f)
	}
}

// nodosDe devuelve todos los nodos de una plantilla en orden de lectura de la portada: cabecera,
// secciones, pie.
func nodosPagina(s *ir.Sitio, pag ir.Pagina) []ir.Nodo {
	var raiz []ir.Nodo
	if pag.ShowHeader && s.Header != nil {
		raiz = append(raiz, *s.Header)
	}
	raiz = append(raiz, pag.Sections...)
	if pag.ShowFooter && s.Footer != nil {
		raiz = append(raiz, *s.Footer)
	}
	var out []ir.Nodo
	recorre(raiz, func(n ir.Nodo) { out = append(out, n) })
	return out
}

func todosLosNodos(s *ir.Sitio) []ir.Nodo {
	var raiz []ir.Nodo
	if s.Header != nil {
		raiz = append(raiz, *s.Header)
	}
	for _, pag := range s.Pages {
		raiz = append(raiz, pag.Sections...)
	}
	if s.Footer != nil {
		raiz = append(raiz, *s.Footer)
	}
	var out []ir.Nodo
	recorre(raiz, func(n ir.Nodo) { out = append(out, n) })
	return out
}

// textosPlanos recorre el JSON crudo y devuelve cada par {es, en} que encuentre como cadenas.
func parejasEsEn(v any, f func(es, en string)) {
	switch x := v.(type) {
	case map[string]any:
		es, tieneEs := x["es"]
		en, tieneEn := x["en"]
		if tieneEs && tieneEn {
			se, ok1 := es.(string)
			sn, ok2 := en.(string)
			if ok1 && ok2 {
				f(se, sn)
			}
			re, ok3 := es.([]any)
			rn, ok4 := en.([]any)
			if ok3 && ok4 {
				f(textoRico(re), textoRico(rn))
			}
		}
		for _, h := range x {
			parejasEsEn(h, f)
		}
	case []any:
		for _, h := range x {
			parejasEsEn(h, f)
		}
	}
}

func textoRico(tramos []any) string {
	var b strings.Builder
	for _, t := range tramos {
		if m, ok := t.(map[string]any); ok {
			s, _ := m["text"].(string)
			b.WriteString(s)
		}
	}
	return b.String()
}

func cadenasDe(v any, f func(s string)) {
	switch x := v.(type) {
	case string:
		f(x)
	case map[string]any:
		for _, h := range x {
			cadenasDe(h, f)
		}
	case []any:
		for _, h := range x {
			cadenasDe(h, f)
		}
	}
}

func paletaAA(colors map[string]string, usaInverso bool) []string {
	var malos []string
	pares := [][2]string{
		{"text", "background"},
		{"muted", "background"},
		{"on_primary", "primary"},
		{"primary", "background"},
		{"text", "surface"},
		{"muted", "surface"},
		{"primary", "surface"},
		{"on_inverse", "inverse"},
	}
	for _, par := range pares {
		a, okA := colors[par[0]]
		b, okB := colors[par[1]]
		if !okA || !okB {
			malos = append(malos, fmt.Sprintf("falta el color %s o %s", par[0], par[1]))
			continue
		}
		cr, err := calcularContraste(a, b)
		if err != nil || cr < 4.5 {
			malos = append(malos, fmt.Sprintf("contraste %s/%s = %.2f (AA pide 4,5)", par[0], par[1], cr))
		}
	}
	if usaInverso {
		cr, err := calcularContraste(colors["primary"], colors["inverse"])
		if err != nil || cr < 3 {
			malos = append(malos, fmt.Sprintf("usa fondo oscuro y primary/inverse = %.2f (pide 3)", cr))
		}
	}
	return malos
}

// revisarPlantilla devuelve lo que está mal en una plantilla del catálogo ampliado. Vacío = bien.
func revisarPlantilla(p Plantilla, raw []byte, cat *ir.Catalogo) []string {
	var malos []string
	mal := func(f string, a ...any) { malos = append(malos, fmt.Sprintf(f, a...)) }

	// Identidad y metadatos.
	if !reClave.MatchString(p.Key) {
		mal("clave %q no válida", p.Key)
	}
	if !slices.Contains(categoriasValidas, p.Category) || !reCategory.MatchString(p.Category) {
		mal("categoría %q no es una de las del catálogo", p.Category)
	}
	if p.Vertical == "" {
		mal("falta el vertical")
	}
	for _, idioma := range []string{"es", "en"} {
		for nombre, t := range map[string]ir.Textos{"name": p.Name, "description": p.Description, "keywords": p.Keywords} {
			if strings.TrimSpace(t[idioma]) == "" {
				mal("%s sin texto en %s", nombre, idioma)
			}
		}
	}
	if len(raw) > maxBytesPlantilla {
		mal("pesa %d bytes (máximo %d)", len(raw), maxBytesPlantilla)
	}
	if p.IR.SEO.Business != nil {
		mal("no debe traer seo.business (§13.1)")
	}
	if p.IR.Languages.Default != "es" || len(p.IR.Languages.Others) != 1 || p.IR.Languages.Others[0] != "en" {
		mal("idiomas %v/%v: se exigen es + en", p.IR.Languages.Default, p.IR.Languages.Others)
	}

	// Estructura cruda: ningún campo en un nodo al que no corresponde (p. ej. columns de un delta).
	var envoltorio struct {
		IR json.RawMessage `json:"ir"`
	}
	if err := json.Unmarshal(raw, &envoltorio); err == nil {
		for _, pr := range ir.ValidarCrudo(envoltorio.IR) {
			mal("estructura: %s %s", pr.Code, pr.Path)
		}
	}

	// Validador del IR, el de publicar: sin errores ni avisos.
	for _, pr := range ir.Validar(&p.IR, cat, ir.ModoPublicar) {
		mal("validador: %s %s %s", pr.Code, pr.Path, pr.Detail)
	}

	// Sin emojis, sin lorem, sin doble exclamación.
	if contienePictografico(raw) {
		mal("contiene emojis")
	}
	if reLorem.Match(raw) {
		mal("contiene texto de relleno tipo lorem ipsum")
	}
	if reDobleExclam.Match(raw) {
		mal("contiene doble exclamación")
	}

	// Textos completos y traducidos de verdad.
	var generico any
	_ = json.Unmarshal(raw, &generico)
	parejasEsEn(generico, func(es, en string) {
		if strings.TrimSpace(es) == "" || strings.TrimSpace(en) == "" {
			if strings.TrimSpace(es) != strings.TrimSpace(en) {
				mal("texto con un idioma vacío: %q / %q", es, en)
			}
			return
		}
		if len([]rune(es)) >= 30 && es == en {
			mal("texto sin traducir al inglés: %q", es)
		}
	})

	// Sin jerga de marketing ni promesas sin datos (años de experiencia, «los mejores»...).
	cadenasDe(generico, func(s string) {
		if reJerga.MatchString(s) {
			mal("jerga o promesa sin datos: %q", s)
		}
	})

	// Un texto que da instrucciones a quien edita tiene que impedir publicar mientras siga ahí.
	soloIR := generico
	if m, ok := generico.(map[string]any); ok {
		soloIR = m["ir"]
	}
	cadenasDe(soloIR, func(s string) {
		if reInstruccion.MatchString(s) && !domain.TieneContenidoDemo([]byte(s), p.Key) {
			mal("instrucción visible sin marcador que impida publicar: %q", s)
		}
	})

	// Nada remoto: solo rellenos de imagen, y de enlaces https únicamente WhatsApp.
	cadenasDe(generico, func(s string) {
		if strings.Contains(s, "http://") {
			mal("enlace http sin cifrar: %q", s)
		}
		if strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "https://wa.me/") {
			mal("URL remota %q", s)
		}
	})
	var telefono, whatsapp, mapa, horario, formulario, productos, carrito bool
	for _, n := range todosLosNodos(&p.IR) {
		switch n.Type {
		case "image":
			if n.Src == nil || n.Src.Placeholder == nil {
				mal("imagen %s que no es un relleno", n.ID)
			}
			if !n.Decorative && (strings.TrimSpace(n.Alt["es"]) == "" || strings.TrimSpace(n.Alt["en"]) == "") {
				mal("imagen %s sin texto alternativo en es y en", n.ID)
			}
		case "button":
			if n.Action != nil && n.Action.URL != nil {
				u := *n.Action.URL
				telefono = telefono || reTelDemo.MatchString(u)
				whatsapp = whatsapp || strings.HasPrefix(u, "https://wa.me/")
			}
		case "embed":
			mapa = mapa || n.Provider == "osm"
		case "table":
			c := strings.ToLower(n.Caption["es"])
			horario = horario || strings.Contains(c, "horario")
		case "form":
			formulario = true
			if strings.TrimSpace(n.Consent["es"]) == "" {
				mal("formulario sin consentimiento")
			}
		case "product_grid":
			productos = true
		case "cart_button":
			carrito = true
		case "text":
			if strings.HasPrefix(n.Tag, "h") {
				horario = horario || strings.Contains(strings.ToLower(rico(n.Content["es"])), "horario")
			}
		}
	}
	if !telefono && sinTelefono[p.Key] == "" {
		mal("sin botón de llamada (tel:)")
	}
	if !whatsapp && sinTelefono[p.Key] == "" {
		mal("sin botón de WhatsApp")
	}
	if !mapa && sinMapa[p.Key] == "" {
		mal("sin mapa")
	}
	if !horario && sinHorario[p.Key] == "" {
		mal("sin horario")
	}
	if !formulario {
		mal("sin formulario de contacto o de cita")
	}
	if p.Store != (productos && carrito) {
		mal("store=%v pero producto=%v y carrito=%v", p.Store, productos, carrito)
	}
	if !p.Store && (productos || carrito) {
		mal("trae tienda sin store: true")
	}

	// Un h1 por página y encabezados en orden.
	for _, pag := range p.IR.Pages {
		h1, ultimo := 0, 0
		for _, n := range nodosPagina(&p.IR, pag) {
			if n.Type != "text" || len(n.Tag) != 2 || n.Tag[0] != 'h' || n.Tag[1] < '1' || n.Tag[1] > '6' {
				continue
			}
			nivel := int(n.Tag[1] - '0')
			if nivel == 1 {
				h1++
			}
			if ultimo == 0 && nivel != 1 {
				mal("página %s: el primer encabezado es h%d, no h1", pag.ID, nivel)
			}
			if ultimo != 0 && nivel > ultimo+1 {
				mal("página %s: salto de h%d a h%d", pag.ID, ultimo, nivel)
			}
			if strings.TrimSpace(rico(n.Content["es"])) == "" {
				mal("encabezado vacío en %s", pag.ID)
			}
			ultimo = nivel
		}
		if h1 != 1 {
			mal("página %s tiene %d h1", pag.ID, h1)
		}
	}
	if n := len(p.IR.Pages); n < 1 || n > 6 {
		mal("%d páginas: se piden entre 1 y 6", n)
	}

	// Contraste AA de la paleta, y que lo que se tiene que cambiar impida publicar tal cual.
	usaInverso := strings.Contains(string(raw), `"$inverse"`)
	malos = append(malos, paletaAA(p.IR.Theme.Colors, usaInverso)...)
	if !domain.TieneContenidoDemo(raw, p.Key) {
		mal("no tiene marcadores de relleno: se podría publicar sin cambiar nada")
	}
	return malos
}

func rico(tramos []ir.Tramo) string {
	var b strings.Builder
	for _, t := range tramos {
		b.WriteString(t.Text)
	}
	return b.String()
}

func catalogoReal(t *testing.T) *ir.Catalogo {
	t.Helper()
	c, err := catalogo.CargarPorDefecto()
	if err != nil {
		t.Fatalf("catalogo: %v", err)
	}
	fuentes, biblioteca := c.IDs()
	return &ir.Catalogo{Fuentes: fuentes, Biblioteca: biblioteca}
}

func TestCatalogo_CadaPlantillaCumple(t *testing.T) {
	cat := catalogoReal(t)
	lista, err := ListarPlantillas()
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) < minimoCatalogo {
		t.Errorf("el catálogo tiene %d plantillas y no puede bajar de %d", len(lista), minimoCatalogo)
	}
	claves := map[string]bool{}
	nombres := map[string]string{}
	firmas := map[string]string{}
	for _, info := range lista {
		if claves[info.Key] {
			t.Errorf("clave repetida %q", info.Key)
		}
		claves[info.Key] = true
		if previa, ok := nombres[info.Name["es"]]; ok {
			t.Errorf("el nombre %q lo tienen %s y %s", info.Name["es"], previa, info.Key)
		}
		nombres[info.Name["es"]] = info.Key
		if !slices.Contains(categoriasValidas, info.Category) {
			t.Errorf("%s: categoría %q fuera del catálogo", info.Key, info.Category)
		}
		t.Run(info.Key, func(t *testing.T) {
			raw, err := plantillasFS.ReadFile(info.Key + ".json")
			if err != nil {
				t.Fatalf("la clave no coincide con el fichero: %v", err)
			}
			p, err := ObtenerPlantilla(info.Key)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range revisarPlantilla(p, raw, cat) {
				t.Error(m)
			}
		})
		// Una plantilla por sector, no variantes cosméticas: paleta, fuentes y estructura de la
		// portada no se repiten juntas.
		p, err := ObtenerPlantilla(info.Key)
		if err != nil {
			continue
		}
		firma := firmaDeAspecto(p)
		if otra, ok := firmas[firma]; ok {
			t.Errorf("%s y %s tienen el mismo aspecto y la misma estructura de portada", info.Key, otra)
		}
		firmas[firma] = info.Key
	}
}

func firmaDeAspecto(p Plantilla) string {
	var kinds []string
	if len(p.IR.Pages) > 0 {
		for _, s := range p.IR.Pages[0].Sections {
			kinds = append(kinds, s.Kind)
		}
	}
	c := p.IR.Theme.Colors
	return fmt.Sprintf("%s|%s|%s|%s|%s", c["background"], c["primary"], p.IR.Theme.Fonts.Heading, p.IR.Theme.Fonts.Body, strings.Join(kinds, ","))
}

// Cada excepción tiene que seguir haciendo falta y estar justificada.
func TestCatalogo_LasExcepcionesSigueSiendoValidas(t *testing.T) {
	for nombre, m := range map[string]map[string]string{"sinTelefono": sinTelefono, "sinMapa": sinMapa, "sinHorario": sinHorario} {
		for k, motivo := range m {
			if strings.TrimSpace(motivo) == "" {
				t.Errorf("%s[%s] sin motivo", nombre, k)
			}
			if _, err := ObtenerPlantilla(k); err != nil {
				t.Errorf("%s[%s]: la plantilla no existe", nombre, k)
			}
		}
	}
}

// El guardián tiene que fallar con una plantilla mala a propósito (hay que verlo en rojo).
func TestCatalogo_DetectaUnaPlantillaMala(t *testing.T) {
	cat := catalogoReal(t)
	buena, err := ObtenerPlantilla("peluqueria")
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := plantillasFS.ReadFile("peluqueria.json")
	if m := revisarPlantilla(buena, crudo, cat); len(m) != 0 {
		t.Fatalf("la plantilla de referencia debería estar bien: %v", m)
	}

	casos := map[string]func(s string) string{
		"instruccion sin marcador": func(s string) string {
			return strings.Replace(s, "Tu pelo, en buenas manos", "Rellena aquí tu titular", 1)
		},
		"estructura": func(s string) string {
			return strings.Replace(s, `"mobile":{"layout":{"columns":1}}`, `"mobile":{"columns":1}`, 1)
		},
		"jerga": func(s string) string {
			return strings.Replace(s, "Tu pelo, en buenas manos", "Soluciones integrales de vanguardia", 1)
		},
		"emoji": func(s string) string { return strings.Replace(s, "Tu pelo, en buenas manos", "Tu pelo \U0001F487", 1) },
		"lorem": func(s string) string {
			return strings.Replace(s, "Tu pelo, en buenas manos", "Lorem ipsum dolor sit amet", 1)
		},
		"imagen remota": func(s string) string { return strings.Replace(s, `{"placeholder":"photo"}`, `{"asset":"x"}`, 1) },
		"url remota":    func(s string) string { return strings.Replace(s, "tel:+34600000000", "https://ejemplo.org/", 1) },
		"sin telefono":  func(s string) string { return strings.ReplaceAll(s, "tel:+34600000000", "mailto:a@b.es") },
		"sin traducir": func(s string) string {
			return strings.Replace(s, "We use professional brands and explain which ones to use at home and which you can skip.", "Trabajamos con marcas profesionales y te explicamos cuáles usar en casa y cuáles no hacen falta.", 1)
		},
		"sin marcadores": func(s string) string {
			for _, m := range domain.TextosDemo {
				s = strings.ReplaceAll(s, string(m), "Ana")
			}
			return s
		},
		"dos h1": func(s string) string { return strings.Replace(s, `"tag":"h2"`, `"tag":"h1"`, 1) },
		"sin consent": func(s string) string {
			return strings.Replace(s, "He leído y acepto la política de privacidad.", "", 1)
		},
		"categoria": func(s string) string { return strings.Replace(s, `"category":"belleza"`, `"category":"zzz"`, 1) },
		"pesa demasiado": func(s string) string {
			return strings.Replace(s, `"components":[]`, `"x":"`+strings.Repeat("a", maxBytesPlantilla)+`","components":[]`, 1)
		},
	}
	for nombre, rompe := range casos {
		t.Run(nombre, func(t *testing.T) {
			raw := []byte(rompe(string(crudo)))
			if bytes.Equal(raw, crudo) {
				t.Fatalf("el caso %q no cambió nada: el guardián no se está probando", nombre)
			}
			var p Plantilla
			if err := json.Unmarshal(raw, &p); err != nil {
				t.Fatalf("el caso %q rompió el JSON: %v", nombre, err)
			}
			if len(revisarPlantilla(p, raw, cat)) == 0 {
				t.Errorf("el guardián no detecta %q", nombre)
			}
		})
	}
}
