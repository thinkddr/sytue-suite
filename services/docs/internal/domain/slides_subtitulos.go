// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Subtítulos de un vídeo o un audio de diapositiva: pistas WebVTT que el editor guarda en
// la caja `kind: "media"` (`tracks`, apps/frontend/packages/office/src/editor/slides/subtitulos.ts).
// Al exportar y al importar se pasan por el mismo saneado que usa el editor, porque el
// .vtt de un fichero ajeno es entrada no confiable y el modelo viaja entero en cada
// autosave.

// slideTrack es una pista: el .vtt entero, su idioma (BCP 47) y si es la de por defecto.
type slideTrack struct {
	Lang    string `json:"lang"`
	Label   string `json:"label,omitempty"`
	VTT     string `json:"vtt"`
	Default bool   `json:"default,omitempty"`
}

const (
	// maxPistas y maxVTT son los techos del editor (MAX_TRACKS y MAX_VTT_CHARS).
	maxPistas = 6
	maxVTT    = 120_000
	// maxEtiquetaPista acota el nombre propio de una pista.
	maxEtiquetaPista = 60
)

var (
	reCabeceraVTT = regexp.MustCompile(`^WEBVTT(?:[ \t\n]|$)`)
	reLangPrimero = regexp.MustCompile(`^[A-Za-z]{2,3}$`)
	reLangScript  = regexp.MustCompile(`^[A-Za-z]{4}$`)
	reLangRegion  = regexp.MustCompile(`^(?:[A-Za-z]{2}|\d{3})$`)
)

// limpiarVTT es el .vtt sin BOM y con saltos de línea `\n`; ok=false si no es WebVTT
// (le falta la cabecera) o pasa del techo.
func limpiarVTT(raw string) (string, bool) {
	t := strings.TrimPrefix(raw, "\uFEFF")
	t = strings.ReplaceAll(strings.ReplaceAll(t, "\r\n", "\n"), "\r", "\n")
	if !reCabeceraVTT.MatchString(t) || utf8.RuneCountInString(t) > maxVTT {
		return "", false
	}
	return t, true
}

// normalizarIdioma es la forma canónica de una etiqueta de idioma (`es`, `en-GB`,
// `zh-Hant-TW`: idioma en minúsculas, escritura con inicial y región en mayúsculas); ""
// si no tiene forma de BCP 47.
func normalizarIdioma(raw string) string {
	partes := strings.Split(strings.ReplaceAll(strings.TrimSpace(raw), "_", "-"), "-")
	if len(partes) > 4 || !reLangPrimero.MatchString(partes[0]) {
		return ""
	}
	out := []string{strings.ToLower(partes[0])}
	for _, p := range partes[1:] {
		switch {
		case reLangScript.MatchString(p):
			out = append(out, strings.ToUpper(p[:1])+strings.ToLower(p[1:]))
		case reLangRegion.MatchString(p):
			out = append(out, strings.ToUpper(p))
		default:
			return ""
		}
	}
	return strings.Join(out, "-")
}

// pistasValidas rehace una lista de pistas: solo las bien formadas, hasta el tope, sin
// idiomas repetidos y con una única pista por defecto (la primera que lo pida).
func pistasValidas(in []slideTrack) []slideTrack {
	var out []slideTrack
	vistas := map[string]bool{}
	hayDefecto := false
	for _, t := range in {
		if len(out) >= maxPistas {
			break
		}
		lang, vtt := normalizarIdioma(t.Lang), ""
		if v, ok := limpiarVTT(t.VTT); ok {
			vtt = v
		}
		if lang == "" || vtt == "" || vistas[lang] {
			continue
		}
		vistas[lang] = true
		label := strings.TrimSpace(t.Label)
		if utf8.RuneCountInString(label) > maxEtiquetaPista {
			label = string([]rune(label)[:maxEtiquetaPista])
		}
		out = append(out, slideTrack{Lang: lang, Label: label, VTT: vtt, Default: t.Default && !hayDefecto})
		hayDefecto = hayDefecto || t.Default
	}
	return out
}

// --- .pptx ---

// Los subtítulos de un medio de un .pptx son partes `ppt/media/subtitlesN.vtt` con una
// relación del tipo de subtítulos de Office desde la diapositiva, y una extensión propia
// en las propiedades del medio que dice a qué idioma corresponde cada una y cuál es la de
// por defecto. Una extensión (`p:ext`) con un `uri` que el lector no conoce se ignora: es
// para lo que está. PowerPoint no la entiende y deja las pistas sin usar.
// ponytail: no validado con PowerPoint (no hay uno aquí) ni escrito el vínculo de
// PowerPoint entre el medio y su pista; los .vtt de PowerPoint no se leen (su idioma no
// consta en el fichero).
const (
	pptxTrackExtURI = "{8C3F7A52-2B1E-4D6A-9F40-5E7B1C0D3A19}"
	pptxTrackNS     = "urn:sytue:names:ooxml:1.0"
)

// trackExt es la extensión de las propiedades de un medio con sus pistas ("" si no tiene).
func (pb *pptxBuild) trackExt(rels *relSet, box slideBox) string {
	pistas := pistasValidas(box.Tracks)
	if len(pistas) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<p:ext uri="` + pptxTrackExtURI + `">`)
	for _, t := range pistas {
		pb.subN++
		name := "subtitles" + strconv.Itoa(pb.subN) + ".vtt"
		pb.media["ppt/media/"+name] = []byte(t.VTT)
		pb.types["vtt"] = "text/vtt"
		attrs := ` lang="` + xmlEscape(t.Lang) + `"`
		if t.Label != "" {
			attrs += ` label="` + xmlEscape(t.Label) + `"`
		}
		if t.Default {
			attrs += ` default="1"`
		}
		b.WriteString(`<sytue:track xmlns:sytue="` + pptxTrackNS + `"` + attrs + ` r:id="` + rels.add(relSubtitles, "../media/"+name, "") + `"/>`)
	}
	b.WriteString(`</p:ext>`)
	return b.String()
}

// pptxTrackRef es una pista citada por las propiedades de un medio: su idioma, su nombre,
// si es la de por defecto y la relación de su fichero.
type pptxTrackRef struct {
	lang, label, rid string
	def              bool
}

// readTracks lee los .vtt de las pistas de un medio y los pasa por el saneado del editor.
// Las que no dicen su idioma (las de PowerPoint: su .vtt no lo lleva) toman el del nombre
// del fichero (`clip.es.vtt`) o, si no, el del texto de la diapositiva.
func (p *pptxDec) readTracks(refs []pptxTrackRef) []slideTrack {
	var in []slideTrack
	for _, r := range refs {
		target := p.rels[r.rid]
		if target == "" {
			continue
		}
		file := path.Clean(p.dir + "/" + target)
		raw := p.files[file]
		if len(raw) > 4*maxVTT { // 4 bytes por carácter como mucho: lo demás ya sobra
			continue
		}
		if p.vtt == nil {
			p.vtt = map[string]bool{}
		}
		p.vtt[r.rid] = true
		lang := r.lang
		if lang == "" {
			lang = idiomaDeFichero(file)
		}
		if lang == "" {
			lang = p.idioma
		}
		if lang == "" {
			lang = "und" // ponytail: sin idioma conocido; el editor lo enseña como «desconocido»
		}
		in = append(in, slideTrack{Lang: lang, Label: r.label, VTT: string(raw), Default: r.def})
	}
	// ponytail: dos pistas sin idioma de un mismo medio chocan en la misma etiqueta y
	// pistasValidas se queda con la primera; PowerPoint no da cómo distinguirlas.
	return pistasValidas(in)
}

var (
	reLangDeFichero = regexp.MustCompile(`[._-]([A-Za-z]{2,3}(?:[-_][A-Za-z0-9]{2,4})*)\.vtt$`)
	reLangDeRun     = regexp.MustCompile(`\blang="([A-Za-z]{2,3}(?:-[A-Za-z]{2,4})?)"`)
)

// idiomaDeFichero es el idioma que dice el nombre de un .vtt (`clip.es.vtt`,
// `captions_en-GB.vtt`); "" si no dice ninguno.
func idiomaDeFichero(name string) string {
	if m := reLangDeFichero.FindStringSubmatch(path.Base(name)); m != nil {
		return normalizarIdioma(m[1])
	}
	return ""
}

// idiomaDeTexto es el idioma del primer texto con `lang` de una parte DrawingML.
func idiomaDeTexto(raw []byte) string {
	if m := reLangDeRun.FindSubmatch(raw); m != nil {
		return normalizarIdioma(string(m[1]))
	}
	return ""
}

// esRelacionVTT dice si el destino de una relación es un fichero de subtítulos.
func esRelacionVTT(target string) bool {
	return strings.HasSuffix(strings.ToLower(target), ".vtt")
}

// pistasSueltas son los .vtt que una diapositiva cita por relación sin que ninguna caja
// los reclame: la forma de PowerPoint, que cuelga las pistas de la diapositiva y no del
// medio. Si la diapositiva tiene un único medio sin pistas, son las suyas.
func (p *pptxDec) pistasSueltas(sl *slideT) {
	var medio *slideBox
	for i := range sl.Boxes {
		if sl.Boxes[i].Kind == "media" {
			if medio != nil {
				return // con dos medios no se sabe de cuál es
			}
			medio = &sl.Boxes[i]
		}
	}
	if medio == nil || len(medio.Tracks) > 0 {
		return
	}
	var refs []pptxTrackRef
	rids := make([]string, 0, len(p.rels))
	for rid := range p.rels {
		rids = append(rids, rid)
	}
	sort.Strings(rids)
	for _, rid := range rids {
		if esRelacionVTT(p.rels[rid]) && !p.vtt[rid] {
			refs = append(refs, pptxTrackRef{rid: rid})
		}
	}
	medio.Tracks = p.readTracks(refs)
}

// pistasJunto son las pistas de un medio que viajan en el paquete junto a él con su
// mismo nombre (`clip.mp4` + `clip.vtt` o `clip.es.vtt`); el idioma, del nombre del
// fichero o, si no lo dice, `porDefecto`. Para los paquetes que no dicen de qué medio es
// cada pista (ODF no tiene pistas de texto).
func pistasJunto(files map[string][]byte, medio, porDefecto string) []slideTrack {
	base := strings.TrimSuffix(medio, path.Ext(medio))
	var nombres []string
	for name := range files {
		if esRelacionVTT(name) && (name == base+".vtt" || strings.HasPrefix(name, base+".")) {
			nombres = append(nombres, name)
		}
	}
	sort.Strings(nombres)
	var in []slideTrack
	for _, name := range nombres {
		raw := files[name]
		if len(raw) > 4*maxVTT {
			continue
		}
		lang := idiomaDeFichero(name)
		if lang == "" {
			lang = porDefecto
		}
		if lang == "" {
			lang = "und"
		}
		in = append(in, slideTrack{Lang: lang, VTT: string(raw)})
	}
	return pistasValidas(in)
}

// pistasJuntoAlMedio son las pistas que viajan junto al fichero del medio de una caja.
func (p *pptxDec) pistasJuntoAlMedio(nv nvProps) []slideTrack {
	for _, rid := range []string{nv.mediaEmbed, nv.mediaLink} {
		if target := p.rels[rid]; target != "" && !strings.Contains(target, "://") {
			if t := pistasJunto(p.files, path.Clean(p.dir+"/"+target), p.idioma); len(t) > 0 {
				return t
			}
		}
	}
	return nil
}
