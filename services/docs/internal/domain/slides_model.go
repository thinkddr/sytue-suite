// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Modelo doc:slides del editor (espejo de packages/office/src/editor/types.ts) que comparten
// la exportación (export_slides.go) y la importación (import_pptx.go, import_odf.go).
// Era DOS modelos —uno para leer, otro para escribir— con los mismos campos copiados, y
// cada característica nueva había que añadirla en los dos. Aquí solo está lo que pptx y
// odp pueden llevar; lo demás que guarda el editor (gráficos, SmartArt, enlaces…) se
// ignora al leer y no se emite al escribir.

type slidesModel struct {
	Slides []slideT `json:"slides"`
	// Theme es el id de un preset de SLIDE_THEMES; vacío = Papel.
	Theme string `json:"theme,omitempty"`
	// Master es el patrón: cajas que se pintan detrás de las de cada diapositiva.
	Master *slideMaster `json:"master,omitempty"`
	// Size es la proporción de la diapositiva (`SlideSize` de la web); nil es 16:9.
	Size *outSize `json:"size,omitempty"`
	// Themes son los temas propios de la presentación (colores y fuente); Theme puede
	// ser el id de uno de ellos.
	Themes []customTheme `json:"themes,omitempty"`
}

// outSize es el `SlideSize` de la web: un preset, o ancho y alto propios con su unidad.
type outSize struct {
	Preset string  `json:"preset"`
	W      float64 `json:"w,omitempty"`
	H      float64 `json:"h,omitempty"`
	Unit   string  `json:"unit,omitempty"`
}

type slideMaster struct {
	Boxes []slideBox `json:"boxes"`
	// Layouts son los diseños de la presentación (slides_disenos.go); nil = los tres de
	// serie del editor.
	Layouts []slideLayout `json:"layouts,omitempty"`
}

type slideT struct {
	ID         string           `json:"id"`
	Boxes      []slideBox       `json:"boxes"`
	Connectors []slideConnector `json:"connectors,omitempty"`
	// Transition: none/fade/slide/morph; vacío = none.
	Transition string `json:"transition,omitempty"`
	Notes      string `json:"notes,omitempty"`
	// Bg es el fondo propio (#RRGGBB); vacío o 'transparent' = el del tema.
	Bg         string `json:"bg,omitempty"`
	HideMaster bool   `json:"hideMaster,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	// Layout es el id del diseño de la diapositiva (de master.layouts); vacío = en blanco.
	Layout string `json:"layout,omitempty"`
}

type slideBox struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Kind   string  `json:"kind,omitempty"`
	HTML   string  `json:"html"`
	Src    string  `json:"src,omitempty"`
	Shape  string  `json:"shape,omitempty"`
	Fill   string  `json:"fill,omitempty"`
	Stroke string  `json:"stroke,omitempty"`
	// Solo kind 'media'.
	MediaType string `json:"mediaType,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Poster    string `json:"poster,omitempty"`
	Autoplay  bool   `json:"autoplay,omitempty"`
	Loop      bool   `json:"loop,omitempty"`
	// Controls ausente = con controles (la web solo persiste el false).
	Controls *bool `json:"controls,omitempty"`
	// Tracks son los subtítulos WebVTT del medio (slides_subtitulos.go).
	Tracks []slideTrack `json:"tracks,omitempty"`
	// Anims es la secuencia de animaciones; Anim, el modelo viejo de una sola entrada
	// (solo se lee: animsOf lo migra).
	Anims []slideAnim `json:"anims,omitempty"`
	Anim  *slideAnim  `json:"anim,omitempty"`
}

type slideAnim struct {
	Type    string        `json:"type"`
	Order   int           `json:"order"`
	Trigger string        `json:"trigger,omitempty"`
	Points  []motionPoint `json:"points,omitempty"`
}

type motionPoint struct {
	DX float64 `json:"dx"`
	DY float64 `json:"dy"`
}

type slideConnector struct {
	ID         string       `json:"id"`
	Kind       string       `json:"kind"`
	From       connectorEnd `json:"from"`
	To         connectorEnd `json:"to"`
	StartArrow bool         `json:"startArrow,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	EndArrow   bool         `json:"endArrow,omitempty"`   //nolint:tagliatelle // el nombre lo fija la web
	Stroke     string       `json:"stroke,omitempty"`
}

// connectorEnd es un extremo anclado a una caja (BoxID, con su lado; vacío = 'auto')
// o un punto libre en % del lienzo.
type connectorEnd struct {
	BoxID  string   `json:"boxId,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Anchor string   `json:"anchor,omitempty"`
	X      *float64 `json:"x,omitempty"`
	Y      *float64 `json:"y,omitempty"`
}

// --- Tamaño del lienzo ---

// slideAspect es la proporción ancho/alto del modelo (slideAspect de SlidesCanvas.tsx).
func (m slidesModel) slideAspect() float64 {
	if m.Size != nil {
		switch m.Size.Preset {
		case "4:3":
			return 4.0 / 3
		case "custom":
			if m.Size.W > 0 && m.Size.H > 0 {
				return math.Min(5, math.Max(0.2, m.Size.W/m.Size.H))
			}
		}
	}
	return 16.0 / 9
}

// slideEMU es el tamaño de la diapositiva en EMU: siempre 13,333 in de ancho (el de
// PowerPoint en 16:9) y el alto que da la proporción. Con el ancho fijo, 1 px del
// escenario de 960 px del editor es 1 pt en todas las proporciones (un 4:3 de 10 in
// obligaría a encoger cada tamaño de letra).
func (m slidesModel) slideEMU() (w, h int) {
	return slideWEMU, int(math.Round(slideWEMU / m.slideAspect()))
}

// --- Temas ---

// slideTheme es un tema resuelto: fondo, tinta y fuente (vacía = la de por defecto).
type slideTheme struct {
	ID, Bg, Ink, Font string
	// Name es el nombre de un tema propio ("" en los de serie).
	Name string
}

// slideThemePresets son los SLIDE_THEMES de SlidesCanvas.tsx, con los hex de
// CONTENT_PALETTE / HIGHLIGHT_PALETTE ya resueltos.
//
// ponytail: espejo a mano de cinco filas; un tema que el editor añada y que aquí falte
// sale como Papel. Si crecen, que el editor los mande en el modelo (temas propios).
var slideThemePresets = []slideTheme{
	{ID: "carbon", Bg: "0A0A0A", Ink: "FFFFFF"},
	{ID: "pizarra", Bg: "475569", Ink: "FFFFFF"},
	{ID: "arena", Bg: "FED7AA", Ink: "0A0A0A", Font: "Georgia"},
	{ID: "menta", Bg: "BBF7D0", Ink: "0A0A0A"},
	{ID: "cielo", Bg: "BFDBFE", Ink: "0A0A0A"},
}

// temaPapel es el de sin tema: folio blanco y la tinta del editor.
var temaPapel = slideTheme{Bg: "FFFFFF", Ink: "0A0A0A"}

// theme es el tema efectivo del modelo.
func (m slidesModel) theme() slideTheme {
	for _, t := range slideThemePresets {
		if t.ID == m.Theme {
			return t
		}
	}
	for _, c := range m.Themes {
		if th, ok := c.resolved(); ok && c.ID == m.Theme {
			return th
		}
	}
	return temaPapel
}

// themeIDFor es el preset con ese fondo y esa tinta (y su fuente, si la fija); "" si
// ninguno: Papel, o un tema ajeno.
func themeIDFor(bg, ink, font string) string {
	for _, t := range slideThemePresets {
		if strings.EqualFold(t.Bg, bg) && strings.EqualFold(t.Ink, ink) && (t.Font == "" || strings.EqualFold(t.Font, font)) {
			return t.ID
		}
	}
	return ""
}

// esOscuro dice si el fondo es más oscuro que la tinta (luminancia relativa).
func esOscuro(bg, ink string) bool { return luminancia(bg) < luminancia(ink) }

func luminancia(hex string) float64 {
	if len(hex) != 6 {
		return 1
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 1
	}
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// --- Animaciones ---

// animsOf es la secuencia de una caja: `anims`, o el viejo `anim` único migrado.
func animsOf(b slideBox) []slideAnim {
	if len(b.Anims) > 0 {
		return b.Anims
	}
	if b.Anim != nil && b.Anim.Type != "" {
		return []slideAnim{*b.Anim}
	}
	return nil
}

// animRef es una animación de la diapositiva con la caja a la que pertenece.
type animRef struct {
	box  int // índice en slide.Boxes
	anim slideAnim
}

// animSteps reparte las animaciones de la diapositiva en PASOS (un clic cada uno) y,
// dentro de cada paso, en GRUPOS que se encadenan («tras la anterior»); las de un mismo
// grupo van a la vez. Es el slidePlan de slides/anim.ts: la primera siempre abre paso.
func animSteps(boxes []slideBox) [][][]animRef {
	var seq []animRef
	for i, b := range boxes {
		for _, a := range animsOf(b) {
			seq = append(seq, animRef{box: i, anim: a})
		}
	}
	sort.SliceStable(seq, func(i, j int) bool { return seq[i].anim.Order < seq[j].anim.Order })
	var steps [][][]animRef
	for i, r := range seq {
		switch {
		case i == 0 || r.anim.Trigger == "" || r.anim.Trigger == "click":
			steps = append(steps, [][]animRef{{r}})
		case r.anim.Trigger == "after":
			s := &steps[len(steps)-1]
			*s = append(*s, []animRef{r})
		default: // with
			s := steps[len(steps)-1]
			s[len(s)-1] = append(s[len(s)-1], r)
		}
	}
	return steps
}

// motionPresetPoints es la geometría de las trayectorias predefinidas (motionPoints de
// slides/anim.ts): desfases en % del lienzo desde {0,0}. Sirve para exportarlas como
// trayectoria y para reconocerlas al importar. ponytail: espejo de anim.ts.
var motionPresetPoints = func() map[string][]motionPoint {
	const s, d = 30.0, 21.0
	line := func(dx, dy float64) []motionPoint { return []motionPoint{{}, {DX: dx, DY: dy}} }
	arc := make([]motionPoint, 0, 13)
	for i := 0; i <= 12; i++ {
		t := float64(i) / 12
		u := 1 - t
		arc = append(arc, motionPoint{DX: 2*u*t*20 + t*t*40, DY: 2 * u * t * 26})
	}
	return map[string][]motionPoint{
		"motion-right":      line(s, 0),
		"motion-left":       line(-s, 0),
		"motion-up":         line(0, -s),
		"motion-down":       line(0, s),
		"motion-up-right":   line(d, -d),
		"motion-up-left":    line(-d, -d),
		"motion-down-right": line(d, d),
		"motion-down-left":  line(-d, d),
		"motion-loop":       {{}, {DX: 30}, {DX: 30, DY: 22}, {DY: 22}, {}},
		"motion-arc":        arc,
	}
}()

// motionPointsOf es la trayectoria de una animación de movimiento.
func motionPointsOf(a slideAnim) []motionPoint {
	if pts, ok := motionPresetPoints[a.Type]; ok {
		return pts
	}
	if len(a.Points) >= 2 {
		return a.Points
	}
	return []motionPoint{{}, {DX: 25, DY: 15}} // la de por defecto de sanitizeMotionPoints
}

// motionTypeFor reconoce una trayectoria predefinida por su geometría; "" si es propia.
func motionTypeFor(pts []motionPoint) string {
	for typ, ref := range motionPresetPoints {
		if len(ref) != len(pts) {
			continue
		}
		igual := true
		for i := range ref {
			if math.Abs(ref[i].DX-pts[i].DX) > 0.5 || math.Abs(ref[i].DY-pts[i].DY) > 0.5 {
				igual = false
				break
			}
		}
		if igual {
			return typ
		}
	}
	return ""
}

// --- Medios ---

var (
	// reOrigenMedio es `safeMediaSrc` de la web y `OrigenMedio` de la app nativa: un
	// medio enlazado por http(s) o incrustado como data: de audio o de vídeo.
	reOrigenMedio = regexp.MustCompile(`(?i)^(?:https?://|data:(?:audio|video)/)`)
	// reIDCajaMedio: un uuid, o el «s3b2» de una importación. Sin barras ni puntos.
	reIDCajaMedio = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// ValidarMedios rechaza un `set` de doc:slides con una caja de medio cuyo `src` no sea
// http(s) ni un data: de audio o vídeo, o cuyo `id` no sea un identificador llano.
//
// Es la regla de los dos editores, puesta donde no se puede saltar: quien escribe el
// modelo por la API no pasa por ninguno, y un cliente que reproduce el medio con el
// sistema dejaba en disco lo que dijera el `src`, con el `id` en el nombre del fichero.
func ValidarMedios(ops []Op) error {
	for i := range ops {
		if ops[i].Kind != OpSet || ops[i].Key != claveDiapos {
			continue
		}
		var crudo string
		var modelo any
		if json.Unmarshal(ops[i].Value, &crudo) != nil || json.Unmarshal([]byte(crudo), &modelo) != nil {
			continue // no es un modelo: ningún cliente saca de él una caja
		}
		if !mediosValidos(modelo) {
			return fmt.Errorf("%w: caja de medio con un origen o un identificador no admitidos", ErrInvalidOp)
		}
	}
	return nil
}

// mediosValidos recorre el modelo ENTERO y no la lista de sitios donde hoy hay cajas
// (diapositivas, patrón, diseños): una caja de medio no vale por estar en otro.
func mediosValidos(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if !mediosValidos(e) {
				return false
			}
		}
	case map[string]any:
		if x["kind"] == "media" {
			id, _ := x["id"].(string)
			src, esTexto := x["src"].(string)
			if !reIDCajaMedio.MatchString(id) || (x["src"] != nil && !esTexto) || (src != "" && !reOrigenMedio.MatchString(src)) {
				return false
			}
		}
		for _, e := range x {
			if !mediosValidos(e) {
				return false
			}
		}
	}
	return true
}

// decodeMediaDataURL descompone un data:URL de audio o vídeo en (extensión, MIME,
// bytes); ok=false si no lo es.
func decodeMediaDataURL(src string) (ext, mime string, data []byte, ok bool) {
	if !strings.HasPrefix(src, "data:") {
		return "", "", nil, false
	}
	comma := strings.IndexByte(src, ',')
	if comma < 0 || !strings.Contains(src[5:comma], ";base64") {
		return "", "", nil, false
	}
	mime = strings.ToLower(strings.SplitN(src[5:comma], ";", 2)[0])
	ext = mediaExtFor(mime)
	if ext == "" {
		return "", "", nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[comma+1:]))
	if err != nil || len(raw) == 0 {
		return "", "", nil, false
	}
	return ext, mime, raw, true
}

// mediaExts son las extensiones de audio/vídeo que se incrustan, con su MIME.
var mediaExts = map[string]string{
	"mp4": "video/mp4", "webm": "video/webm", "mov": "video/quicktime", "m4v": "video/mp4",
	"ogv": "video/ogg", "mp3": "audio/mpeg", "wav": "audio/wav", "ogg": "audio/ogg",
	"oga": "audio/ogg", "m4a": "audio/mp4", "aac": "audio/aac", "flac": "audio/flac",
	"opus": "audio/ogg",
}

func mediaExtFor(mime string) string {
	switch mime {
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "video/quicktime":
		return "mov"
	case "video/ogg":
		return "ogv"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return "wav"
	case "audio/ogg":
		return "ogg"
	case "audio/mp4", "audio/x-m4a":
		return "m4a"
	case "audio/aac":
		return "aac"
	case "audio/flac":
		return "flac"
	}
	return ""
}

// bloquesDeCaja son los bloques del HTML de una caja de diapositiva para escribirlos como
// párrafos (.pptx y .odp): una tabla —las que trae una presentación importada— se escribe
// celda a celda, cada párrafo de cada celda en su sitio. Sin esto se perdía su texto al guardar.
//
// ponytail: la tabla sale como texto; escribirla como a:tbl o table:table es la mejora.
func bloquesDeCaja(htmlStr string) []xblock {
	var out []xblock
	for _, b := range parseHTMLBlocks(htmlStr) {
		if b.table == nil {
			out = append(out, b)
			continue
		}
		for _, fila := range b.table.rows {
			for _, c := range fila {
				for i := range c.paras {
					if !c.covered {
						out = append(out, xblock{para: &c.paras[i]})
					}
				}
			}
		}
	}
	return out
}
