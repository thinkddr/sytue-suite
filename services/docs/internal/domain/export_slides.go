// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Exportadores de PRESENTACIÓN (pptx/odp) a partir del modelo doc:slides del
// editor (slides_model.go). Una diapositiva por slide con sus cajas: texto (con
// formato), imágenes, formas, vídeo y audio, conectores y notas del ponente; y lo que
// la envuelve: el tema (fondo, tinta y fuente), el patrón con sus cajas y los campos
// de número de diapositiva, los diseños, el fondo propio de cada una, su transición y
// sus animaciones. Obra original de Sytue sobre ECMA-376 (PresentationML/DrawingML) y
// OASIS ODF.

// slidesModelOf obtiene el SlidesModel del registro doc:slides; si no existe,
// sintetiza una diapositiva con el título y el texto plano (compat. anterior).
func slidesModelOf(title string, s *State) slidesModel {
	if raw := s.contentString("doc:slides"); raw != "" {
		var m slidesModel
		if json.Unmarshal([]byte(raw), &m) == nil && len(m.Slides) > 0 {
			return m
		}
	}
	var box strings.Builder
	if title != "" {
		box.WriteString("<h1>" + html.EscapeString(title) + "</h1>")
	}
	if t := s.PlainText(); t != "" {
		box.WriteString("<p>" + html.EscapeString(t) + "</p>")
	}
	return slidesModel{Slides: []slideT{{Boxes: []slideBox{{X: 8, Y: 8, W: 84, H: 84, Kind: "text", HTML: box.String()}}}}}
}

// Lienzo 16:9 en EMU (914400 EMU/pulgada): 13.333in × 7.5in. El ancho es el mismo en
// todas las proporciones (slidesModel.slideEMU): así 1 px del escenario de 960 px del
// editor es 1 pt y los tamaños de letra no dependen de la proporción.
const (
	slideWEMU = 12192000
	slideHEMU = 6858000
)

func emu(pct float64, dim int) int {
	v := int(math.Round(pct / 100 * float64(dim)))
	if v < 1 {
		v = 1
	}
	return v
}

// --- OOXML: pptx ---

const (
	relSlide       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"
	relSlideLayout = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout"
	relSlideMaster = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster"
	relNotesSlide  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide"
	relNotesMaster = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesMaster"
	relTheme       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme"
	relImage       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
	relHyperlink   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	relVideo       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/video"
	relAudio       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/audio"
	relMedia       = "http://schemas.microsoft.com/office/2007/relationships/media"
	relSubtitles   = "http://schemas.microsoft.com/office/2011/relationships/subtitles"
)

// Espacios de nombres de la raíz de cada parte PresentationML.
const pptxNS = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`

// Cabecera del árbol de formas (grupo raíz) de master, diseños y diapositivas.
const pptxGrpHeader = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`

// relSet acumula relaciones OPC de una parte (slide/notesSlide).
type relSet struct {
	defs []string
	next int
}

func (r *relSet) add(typ, target, mode string) string {
	r.next++
	id := "rId" + strconv.Itoa(r.next)
	m := ""
	if mode != "" {
		m = ` TargetMode="` + mode + `"`
	}
	r.defs = append(r.defs, `<Relationship Id="`+id+`" Type="`+typ+`" Target="`+xmlEscape(target)+`"`+m+`/>`)
	return id
}

func (r *relSet) xml() string {
	return xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		strings.Join(r.defs, "") + `</Relationships>`
}

// pptxBuild lleva el estado compartido durante la construcción: medios, tamaño del
// lienzo y nº de diapositivas (el campo {total} del patrón).
type pptxBuild struct {
	media  map[string][]byte
	imgs   map[string]string // src → nombre en ppt/media, para no duplicar entre diseños y diapositivas
	types  map[string]string // extensión → content type de lo incrustado
	imgN   int
	medN   int
	subN   int // subtítulos (.vtt) escritos
	w, h   int
	total  int
	poster string // ruta del póster de reserva, si ya se escribió
}

// embedImage decodifica un dataURL, lo guarda en ppt/media y devuelve su rId.
func (pb *pptxBuild) embedImage(rels *relSet, src string) (string, bool) {
	if name, ok := pb.imgs[src]; ok {
		return rels.add(relImage, "../media/"+name, ""), true
	}
	ext, data, ok := decodeDataURL(src)
	if !ok {
		return "", false
	}
	pb.imgN++
	name := "image" + strconv.Itoa(pb.imgN) + "." + ext
	pb.media["ppt/media/"+name] = data
	if pb.imgs == nil {
		pb.imgs = map[string]string{}
	}
	pb.imgs[src] = name
	return rels.add(relImage, "../media/"+name, ""), true
}

func buildPptx(title string, s *State) ([]byte, error) {
	model := slidesModelOf(title, s)
	w, h := model.slideEMU()
	pb := &pptxBuild{media: map[string][]byte{}, types: map[string]string{}, w: w, h: h, total: len(model.Slides)}
	th := model.theme()

	files := map[string][]byte{
		"_rels/.rels":          []byte(relsRootPresentation),
		"ppt/theme/theme1.xml": []byte(pptxThemeXML(th)),
	}
	var ctOverrides strings.Builder

	// Patrón y diseños: los del modelo (o los tres del editor), tras el diseño en blanco
	// que llevan las diapositivas sin diseño.
	layoutList, layoutIndex := pptxLayoutsOf(model)
	masterRels := &relSet{}
	var layoutIDs strings.Builder
	for i, l := range layoutList {
		n := strconv.Itoa(i + 1)
		rid := masterRels.add(relSlideLayout, "../slideLayouts/slideLayout"+n+".xml", "")
		layoutIDs.WriteString(`<p:sldLayoutId id="` + strconv.Itoa(2147483649+i) + `" r:id="` + rid + `"/>`)
		lrels := &relSet{}
		lrels.add(relSlideMaster, "../slideMasters/slideMaster1.xml", "") // rId1
		files["ppt/slideLayouts/slideLayout"+n+".xml"] = []byte(pb.layoutXML(l, lrels))
		files["ppt/slideLayouts/_rels/slideLayout"+n+".xml.rels"] = []byte(lrels.xml())
		ctOverrides.WriteString(`<Override PartName="/ppt/slideLayouts/slideLayout` + n +
			`.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>`)
	}
	masterRels.add(relTheme, "../theme/theme1.xml", "")
	files["ppt/slideMasters/slideMaster1.xml"] = []byte(pb.masterXML(model, th, masterRels, layoutIDs.String()))
	files["ppt/slideMasters/_rels/slideMaster1.xml.rels"] = []byte(masterRels.xml())

	var sldIDLst strings.Builder
	presRels := &relSet{}
	presRels.add(relSlideMaster, "slideMasters/slideMaster1.xml", "") // rId1
	hasNotes := false

	for i, sl := range model.Slides {
		n := i + 1
		ns := strconv.Itoa(n)
		rels := &relSet{}
		rels.add(relSlideLayout, "../slideLayouts/slideLayout"+strconv.Itoa(layoutIndex[sl.Layout]+1)+".xml", "") // rId1

		if strings.TrimSpace(sl.Notes) != "" {
			hasNotes = true
			rels.add(relNotesSlide, "../notesSlides/notesSlide"+ns+".xml", "")
			files["ppt/notesSlides/notesSlide"+ns+".xml"] = []byte(pptxNotesSlide(sl.Notes))
			nrel := &relSet{}
			nrel.add(relNotesMaster, "../notesMasters/notesMaster1.xml", "")
			nrel.add(relSlide, "../slides/slide"+ns+".xml", "")
			files["ppt/notesSlides/_rels/notesSlide"+ns+".xml.rels"] = []byte(nrel.xml())
			ctOverrides.WriteString(`<Override PartName="/ppt/notesSlides/notesSlide` + ns +
				`.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"/>`)
		}

		files["ppt/slides/slide"+ns+".xml"] = []byte(pb.slideXML(sl, rels))
		files["ppt/slides/_rels/slide"+ns+".xml.rels"] = []byte(rels.xml())

		pid := presRels.add(relSlide, "slides/slide"+ns+".xml", "")
		sldIDLst.WriteString(`<p:sldId id="` + strconv.Itoa(256+i) + `" r:id="` + pid + `"/>`)
		ctOverrides.WriteString(`<Override PartName="/ppt/slides/slide` + ns +
			`.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`)
	}

	notesMasterIDLst := ""
	if hasNotes {
		nmID := presRels.add(relNotesMaster, "notesMasters/notesMaster1.xml", "")
		notesMasterIDLst = `<p:notesMasterIdLst><p:notesMasterId r:id="` + nmID + `"/></p:notesMasterIdLst>`
		files["ppt/notesMasters/notesMaster1.xml"] = []byte(pptxNotesMaster)
		nmRels := &relSet{}
		nmRels.add(relTheme, "../theme/theme1.xml", "")
		files["ppt/notesMasters/_rels/notesMaster1.xml.rels"] = []byte(nmRels.xml())
		ctOverrides.WriteString(`<Override PartName="/ppt/notesMasters/notesMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml"/>`)
	}

	// El orden de los hijos de p:presentation lo fija el esquema: el notesMasterIdLst
	// va ANTES del sldIdLst (salía detrás, y PowerPoint es estricto con el orden).
	sz := `<p:sldSz cx="` + strconv.Itoa(w) + `" cy="` + strconv.Itoa(h) + `"`
	if w == slideWEMU && h == slideHEMU {
		sz += ` type="screen16x9"`
	}
	files["ppt/presentation.xml"] = []byte(xmlHeader +
		`<p:presentation ` + pptxNS + `>` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		notesMasterIDLst +
		`<p:sldIdLst>` + sldIDLst.String() + `</p:sldIdLst>` +
		sz + `/><p:notesSz cx="6858000" cy="9144000"/>` +
		`<p:defaultTextStyle>` + pptxLevels(0, "+mn-lt") + `</p:defaultTextStyle></p:presentation>`)

	files["ppt/_rels/presentation.xml.rels"] = []byte(presRels.xml())

	for name, blob := range pb.media {
		files[name] = blob
	}

	var defaults strings.Builder
	for _, ext := range sortedKeys(pb.types) {
		defaults.WriteString(`<Default Extension="` + ext + `" ContentType="` + pb.types[ext] + `"/>`)
	}
	files["[Content_Types].xml"] = []byte(xmlHeader +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Default Extension="png" ContentType="image/png"/>` +
		`<Default Extension="jpg" ContentType="image/jpeg"/>` +
		`<Default Extension="jpeg" ContentType="image/jpeg"/>` +
		`<Default Extension="gif" ContentType="image/gif"/>` +
		defaults.String() +
		`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>` +
		`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>` +
		`<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>` +
		ctOverrides.String() +
		`</Types>`)

	return zipPackageBytes(files)
}

// shapesXML escribe las cajas como formas de un árbol (diapositiva o patrón) y
// devuelve, por caja, el id de forma que recibió (0 si no salió: una imagen o un medio
// sin origen válido). Los ids empiezan en 2: el 1 es el grupo raíz.
func (pb *pptxBuild) shapesXML(boxes []slideBox, rels *relSet, fields bool) (string, []int) {
	var sp strings.Builder
	spids := make([]int, len(boxes))
	id := 2
	for i, box := range boxes {
		var x string
		switch box.Kind {
		case "image":
			if embed, ok := pb.embedImage(rels, box.Src); ok {
				x = pptxPic(id, box, embed, pb)
			}
		case "media":
			x = pb.mediaPic(id, box, rels)
		case "shape":
			x = pb.shape(id, box, rels, fields)
		default: // text
			x = pb.textBox(id, box, rels, fields)
		}
		if x != "" {
			sp.WriteString(x)
			spids[i] = id
			id++
		}
	}
	return sp.String(), spids
}

// slideXML genera el XML de una diapositiva: fondo propio, cajas, conectores,
// transición y animaciones.
func (pb *pptxBuild) slideXML(sl slideT, rels *relSet) string {
	sp, spids := pb.shapesXML(sl.Boxes, rels, false)
	next := 2
	for _, id := range spids {
		next = max(next, id+1)
	}
	show := ""
	if sl.HideMaster {
		show = ` showMasterSp="0"`
	}
	return xmlHeader + `<p:sld ` + pptxNS + show + `><p:cSld>` + pptxBg(sl.Bg) + `<p:spTree>` + pptxGrpHeader +
		sp + pb.connectorsXML(sl, spids, next) +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		pptxTransition(sl.Transition) + pptxTiming(sl.Boxes, spids) + `</p:sld>`
}

// pptxBg es el fondo propio de una diapositiva; "" si usa el del tema.
func pptxBg(bg string) string {
	c := normalizeColor(bg)
	if c == "" {
		return ""
	}
	return `<p:bg><p:bgPr><a:solidFill><a:srgbClr val="` + c + `"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>`
}

// pptxTransition es la transición de entrada. El morph va en la extensión de
// PowerPoint 2019 con un fundido de reserva, que es lo que ve quien no la entiende
// (LibreOffice y PowerPoint antiguos). La duración es la del editor redondeada a la
// velocidad «rápida» del formato.
func pptxTransition(t string) string {
	switch t {
	case "fade":
		return `<p:transition spd="fast"><p:fade/></p:transition>`
	case "slide":
		return `<p:transition spd="fast"><p:push dir="l"/></p:transition>`
	case "morph":
		return `<mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006">` +
			`<mc:Choice xmlns:p159="http://schemas.microsoft.com/office/powerpoint/2015/09/main" Requires="p159">` +
			`<p:transition spd="fast"><p159:morph option="byObject"/></p:transition></mc:Choice>` +
			`<mc:Fallback><p:transition spd="fast"><p:fade/></p:transition></mc:Fallback></mc:AlternateContent>`
	}
	return ""
}

func pptxXfrm(box slideBox, pb *pptxBuild) string {
	return `<a:xfrm><a:off x="` + strconv.Itoa(emu(box.X, pb.w)) + `" y="` + strconv.Itoa(emu(box.Y, pb.h)) +
		`"/><a:ext cx="` + strconv.Itoa(emu(box.W, pb.w)) + `" cy="` + strconv.Itoa(emu(box.H, pb.h)) + `"/></a:xfrm>`
}

func (pb *pptxBuild) textBox(id int, box slideBox, rels *relSet, fields bool) string {
	sid := strconv.Itoa(id)
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + sid + `" name="TextBox ` + sid + `"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr>` + pptxXfrm(box, pb) + `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>` +
		`<p:txBody><a:bodyPr wrap="square" rtlCol="0"><a:normAutofit/></a:bodyPr><a:lstStyle/>` +
		pb.paragraphs(box.HTML, rels, fields) + `</p:txBody></p:sp>`
}

// pptxShapePresets son la geometría DrawingML de cada forma del editor.
var pptxShapePresets = map[string]string{
	"rect": "rect", "ellipse": "ellipse", "line": "line", "arrow": "rightArrow",
	"triangle": "triangle", "diamond": "diamond", "pentagon": "pentagon", "hexagon": "hexagon",
	"star": "star5", "cylinder": "can", "callout": "wedgeRectCallout",
}

// Relleno y trazo por defecto de una forma: los SHAPE_FILL / SHAPE_STROKE del editor
// (pizarra y tinta). Una forma sin color salía sin relleno, y en el editor no lo está.
const (
	shapeFillDefault   = "475569"
	shapeStrokeDefault = "0A0A0A"
)

func (pb *pptxBuild) shape(id int, box slideBox, rels *relSet, fields bool) string {
	sid := strconv.Itoa(id)
	prst := pptxShapePresets[box.Shape]
	if prst == "" {
		prst = "rect"
	}
	fill := `<a:noFill/>`
	if c := shapeColor(box.Fill, shapeFillDefault); c != "" {
		fill = `<a:solidFill><a:srgbClr val="` + c + `"/></a:solidFill>`
	}
	ln := `<a:ln w="19050"><a:noFill/></a:ln>`
	if c := shapeColor(box.Stroke, shapeStrokeDefault); c != "" {
		ln = `<a:ln w="19050"><a:solidFill><a:srgbClr val="` + c + `"/></a:solidFill></a:ln>`
	}
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + sid + `" name="Shape ` + sid + `"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr>` + pptxXfrm(box, pb) + `<a:prstGeom prst="` + prst + `"><a:avLst/></a:prstGeom>` + fill + ln + `</p:spPr>` +
		`<p:txBody><a:bodyPr rtlCol="0" anchor="ctr"/><a:lstStyle/>` + pb.paragraphs(box.HTML, rels, fields) + `</p:txBody></p:sp>`
}

// shapeColor es el color de relleno/trazo de una forma: vacío = el de por defecto del
// editor, 'transparent' = ninguno.
func shapeColor(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return normalizeColor(v)
}

func pptxPic(id int, box slideBox, embed string, pb *pptxBuild) string {
	sid := strconv.Itoa(id)
	return `<p:pic><p:nvPicPr><p:cNvPr id="` + sid + `" name="Picture ` + sid + `"/>` +
		`<p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>` +
		`<p:blipFill><a:blip r:embed="` + embed + `"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>` +
		`<p:spPr>` + pptxXfrm(box, pb) + `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`
}

// mediaPic es un vídeo o un audio: un <p:pic> con su fichero (incrustado si es un
// data:URL, enlazado si es una URL) y su póster. Sin póster, uno liso de reserva: el
// formato exige una imagen. "" si el origen no vale.
func (pb *pptxBuild) mediaPic(id int, box slideBox, rels *relSet) string {
	relType, file := relVideo, "videoFile"
	if box.MediaType == "audio" {
		relType, file = relAudio, "audioFile"
	}
	var link, embed string
	if ext, mime, data, ok := decodeMediaDataURL(box.Src); ok {
		pb.medN++
		name := "media" + strconv.Itoa(pb.medN) + "." + ext
		pb.media["ppt/media/"+name] = data
		pb.types[ext] = mime
		link = rels.add(relType, "../media/"+name, "")
		embed = rels.add(relMedia, "../media/"+name, "")
	} else if u := strings.TrimSpace(box.Src); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		link = rels.add(relType, u, "External")
	} else {
		return ""
	}
	poster, ok := pb.embedImage(rels, box.Poster)
	if !ok {
		if pb.poster == "" {
			pb.poster = "ppt/media/poster.png"
			pb.media[pb.poster] = posterPNG()
		}
		poster = rels.add(relImage, "../media/poster.png", "")
	}
	ext := ""
	if embed != "" {
		ext = `<p:ext uri="{DAA4B4D4-6D71-4841-9C94-3DA282F5DA41}">` +
			`<p14:media xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main" r:embed="` + embed + `"/></p:ext>`
	}
	ext += pb.trackExt(rels, box)
	if ext != "" {
		ext = `<p:extLst>` + ext + `</p:extLst>`
	}
	sid := strconv.Itoa(id)
	return `<p:pic><p:nvPicPr><p:cNvPr id="` + sid + `" name="Media ` + sid + `"><a:hlinkClick r:id="" action="ppaction://media"/></p:cNvPr>` +
		`<p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr><a:` + file + ` r:link="` + link + `"/>` + ext + `</p:nvPr></p:nvPicPr>` +
		`<p:blipFill><a:blip r:embed="` + poster + `"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>` +
		`<p:spPr>` + pptxXfrm(box, pb) + `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`
}

// posterPNG es el póster de reserva de un medio sin el suyo: 16:9 liso en pizarra.
func posterPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 160, 90))
	c := color.RGBA{0x47, 0x55, 0x69, 0xff}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// paragraphs mapea el HTML de una caja a párrafos DrawingML. Vacío => <a:p/>. Con
// `fields` (cajas del patrón), {n} y {total} salen como campos.
func (pb *pptxBuild) paragraphs(htmlStr string, rels *relSet, fields bool) string {
	blocks := bloquesDeCaja(htmlStr)
	if len(blocks) == 0 {
		return `<a:p/>`
	}
	var b strings.Builder
	for _, blk := range blocks {
		if blk.para == nil {
			continue
		}
		p := blk.para
		b.WriteString(`<a:p>`)
		if pr := pptxParaPr(*p); pr != "" {
			b.WriteString(pr)
		}
		sz := pb.aDiapositiva(slideRunSize(p.style))
		heading := strings.HasPrefix(p.style, "Heading")
		for _, r := range p.runs {
			b.WriteString(pb.run(r, sz, heading, rels, fields))
		}
		b.WriteString(`</a:p>`)
	}
	if b.Len() == 0 {
		return `<a:p/>`
	}
	return b.String()
}

func pptxParaPr(p xpara) string {
	var attrs, inner string
	switch p.align {
	case "center":
		attrs = ` algn="ctr"`
	case "right":
		attrs = ` algn="r"`
	case "justify":
		attrs = ` algn="just"`
	}
	switch p.list {
	case "bullet":
		attrs += ` marL="285750" indent="-285750"`
		inner = `<a:buChar char="&#8226;"/>`
	case "number":
		attrs += ` marL="285750" indent="-285750"`
		inner = `<a:buAutoNum type="arabicPeriod"/>`
	}
	// Nivel de la viñeta (DrawingML admite 0..8): sin él, una lista con subpuntos
	// se exporta y se reimporta como una única lista plana.
	if p.list != "" && p.level > 0 {
		lvl := min(p.level, 8)
		attrs += ` lvl="` + strconv.Itoa(lvl) + `"`
	}
	if attrs == "" && inner == "" {
		return ""
	}
	return `<a:pPr` + attrs + `>` + inner + `</a:pPr>`
}

// Campos del patrón: el número de diapositiva es el `slidenum` del formato; el total
// no tiene campo en DrawingML, así que va como uno propio con el total ya escrito
// (lo que ven PowerPoint y LibreOffice) y la importación lo devuelve a {total}.
var reMasterField = regexp.MustCompile(`\{n\}|\{total\}`)

const (
	fldSlideNum      = "slidenum"
	fldSlideTotal    = "sytue-total"
	fldSlideNumGUID  = "{6C1D2E4A-5B7F-4E21-9A3C-0D8E2F4B6A11}"
	fldSlideTotGUID  = "{6C1D2E4A-5B7F-4E21-9A3C-0D8E2F4B6A12}"
	slideNumCachedPh = "‹#›"
)

func (pb *pptxBuild) run(r xrun, sz int, heading bool, rels *relSet, fields bool) string {
	if r.brk {
		return `<a:br/>`
	}
	if r.tam > 0 { // un tamaño propio del tramo (font-size en px del escenario)
		sz = pb.aDiapositiva(int(math.Round(r.tam * 100)))
	}
	var rpr strings.Builder
	rpr.WriteString(`<a:rPr lang="es-ES" sz="` + strconv.Itoa(sz) + `"`)
	// Los títulos del editor van en negrita (h1 700, h2/h3 600).
	if r.b || heading {
		rpr.WriteString(` b="1"`)
	}
	if r.i {
		rpr.WriteString(` i="1"`)
	}
	if r.u || r.link != "" {
		rpr.WriteString(` u="sng"`)
	}
	if r.strike {
		rpr.WriteString(` strike="sngStrike"`)
	}
	rpr.WriteString(`>`)
	tinta := r.color
	if r.link != "" && tinta == "" {
		tinta = "0563C1"
	}
	if tinta != "" {
		rpr.WriteString(`<a:solidFill><a:srgbClr val="` + tinta + `"/></a:solidFill>`)
	}
	if r.link != "" && rels != nil {
		rpr.WriteString(`<a:hlinkClick r:id="` + rels.add(relHyperlink, r.link, "External") + `"/>`)
	}
	rpr.WriteString(`</a:rPr>`)
	if !fields || !reMasterField.MatchString(r.text) {
		return `<a:r>` + rpr.String() + `<a:t>` + xmlEscape(r.text) + `</a:t></a:r>`
	}
	var b strings.Builder
	last := 0
	for _, m := range reMasterField.FindAllStringIndex(r.text, -1) {
		if m[0] > last {
			b.WriteString(`<a:r>` + rpr.String() + `<a:t>` + xmlEscape(r.text[last:m[0]]) + `</a:t></a:r>`)
		}
		if r.text[m[0]:m[1]] == "{n}" {
			b.WriteString(`<a:fld id="` + fldSlideNumGUID + `" type="` + fldSlideNum + `">` + rpr.String() + `<a:t>` + slideNumCachedPh + `</a:t></a:fld>`)
		} else {
			b.WriteString(`<a:fld id="` + fldSlideTotGUID + `" type="` + fldSlideTotal + `">` + rpr.String() + `<a:t>` + strconv.Itoa(pb.total) + `</a:t></a:fld>`)
		}
		last = m[1]
	}
	if last < len(r.text) {
		b.WriteString(`<a:r>` + rpr.String() + `<a:t>` + xmlEscape(r.text[last:]) + `</a:t></a:r>`)
	}
	return b.String()
}

// aDiapositiva pasa un tamaño del editor (centésimas de px del escenario de 960) a centésimas
// de punto de la diapositiva: la cuenta inversa de escalaTexto (import_pptx.go). En la 16:9 de
// 13,33 pulgadas son iguales; en una de 10 pulgadas (4:3, o la 16:9 de Google), 20 px son 15 pt,
// que es lo que mide ese texto en la pantalla del editor respecto al ancho.
func (pb *pptxBuild) aDiapositiva(px100 int) int {
	return int(math.Round(float64(px100) / escalaTexto(float64(pb.w))))
}

// slideRunSize es el tamaño (centésimas de punto) de cada estilo de párrafo: los del
// editor (BOX_CSS de SlidesCanvas.tsx, en px del escenario de 960 px), que en el
// lienzo de 13,333 in son puntos tal cual. Salían más pequeños (32 pt el título).
func slideRunSize(style string) int {
	switch style {
	case "Heading1":
		return 4400
	case "Heading2":
		return 3000
	case "Heading3":
		return 2400
	default:
		return 2000
	}
}

// pptxLevels son los nueve niveles de un estilo de texto: la tinta del tema (tx1) y su
// fuente; `sz` 0 = sin tamaño propio.
func pptxLevels(sz int, font string) string {
	s := ""
	if sz > 0 {
		s = ` sz="` + strconv.Itoa(sz) + `"`
	}
	var b strings.Builder
	for i := 1; i <= 9; i++ {
		n := strconv.Itoa(i)
		b.WriteString(`<a:lvl` + n + `pPr><a:defRPr` + s + `><a:solidFill><a:schemeClr val="tx1"/></a:solidFill>` +
			`<a:latin typeface="` + font + `"/></a:defRPr></a:lvl` + n + `pPr>`)
	}
	return b.String()
}

func pptxNotesSlide(notes string) string {
	var body strings.Builder
	for _, line := range splitParagraphs(notes) {
		if line == "" {
			body.WriteString(`<a:p/>`)
			continue
		}
		body.WriteString(`<a:p><a:r><a:rPr lang="es-ES"/><a:t>` + xmlEscape(line) + `</a:t></a:r></a:p>`)
	}
	return xmlHeader +
		`<p:notes ` + pptxNS + `><p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Placeholder"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>` +
		`<p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/>` + body.String() + `</p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:notes>`
}

// --- Patrón, diseños y tema ---

// pptxClrMap es el mapa de colores del patrón: en un tema oscuro el fondo es el dk1
// del esquema y la tinta el lt1 (como hacen las variantes oscuras de PowerPoint).
func pptxClrMap(th slideTheme) string {
	if esOscuro(th.Bg, th.Ink) {
		return `bg1="dk1" tx1="lt1" bg2="dk2" tx2="lt2"`
	}
	return `bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2"`
}

// masterXML es el patrón: el fondo del tema, las cajas del patrón del editor (con sus
// campos), los diseños y los estilos de texto con la tinta y la fuente del tema.
func (pb *pptxBuild) masterXML(m slidesModel, th slideTheme, rels *relSet, layoutIDs string) string {
	var boxes []slideBox
	if m.Master != nil {
		boxes = m.Master.Boxes
	}
	sp, _ := pb.shapesXML(boxes, rels, true)
	return xmlHeader + `<p:sldMaster ` + pptxNS + `><p:cSld>` +
		`<p:bg><p:bgPr><a:solidFill><a:schemeClr val="bg1"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>` +
		`<p:spTree>` + pptxGrpHeader + sp + `</p:spTree></p:cSld>` +
		`<p:clrMap ` + pptxClrMap(th) + ` accent1="accent1" accent2="accent2" accent3="accent3" ` +
		`accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`<p:sldLayoutIdLst>` + layoutIDs + `</p:sldLayoutIdLst>` +
		`<p:txStyles><p:titleStyle>` + pptxLevels(pb.aDiapositiva(4400), "+mj-lt") + `</p:titleStyle>` +
		`<p:bodyStyle>` + pptxLevels(pb.aDiapositiva(2000), "+mn-lt") + `</p:bodyStyle>` +
		`<p:otherStyle>` + pptxLevels(0, "+mn-lt") + `</p:otherStyle></p:txStyles></p:sldMaster>`
}

// pptxLayout es un diseño del patrón: los tres del editor (SLIDE_LAYOUTS, con las
// cajas de layoutBoxes de SlidesCanvas.tsx) como marcadores de posición, y uno en
// blanco, que es el que usan las diapositivas.
type pptxLayout struct {
	typ, name string
	phs       []pptxPh
	// decor, conectores y ocultaPatron son las formas, las líneas y el `showMasterSp` del
	// diseño (slideLayout).
	decor        []slideBox
	conectores   []slideConnector
	ocultaPatron bool
}

type pptxPh struct {
	typ        string // "title", "ctrTitle" o "" (contenido, con idx)
	idx        int
	x, y, w, h float64
	text       string // la muestra del marcador, ya como párrafos; "" = vacío
}

var pptxLayouts = []pptxLayout{
	{typ: "blank", name: "En blanco"},
	{typ: "title", name: "Título", phs: []pptxPh{{typ: "ctrTitle", x: 10, y: 35, w: 80, h: 20}}},
	{typ: "obj", name: "Título y cuerpo", phs: []pptxPh{{typ: "title", x: 8, y: 8, w: 84, h: 16}, {idx: 1, x: 8, y: 30, w: 84, h: 60}}},
	{typ: "twoObj", name: "Dos columnas", phs: []pptxPh{{typ: "title", x: 8, y: 8, w: 84, h: 16}, {idx: 1, x: 8, y: 30, w: 40, h: 60}, {idx: 2, x: 52, y: 30, w: 40, h: 60}}},
}

func (pb *pptxBuild) layoutXML(l pptxLayout, rels *relSet) string {
	// Las formas del diseño van debajo de sus marcadores, con los ids de después.
	deco, spids := pb.shapesXML(l.decor, rels, true)
	next := 2
	for _, id := range spids {
		next = max(next, id+1)
	}
	var sp strings.Builder
	sp.WriteString(deco + pb.connectorsXML(slideT{Boxes: l.decor, Connectors: l.conectores}, spids, next))
	next += len(l.conectores)
	for i, ph := range l.phs {
		sid := strconv.Itoa(next + i)
		attr := ` type="` + ph.typ + `"`
		if ph.typ == "" {
			attr = ` idx="` + strconv.Itoa(ph.idx) + `"`
		}
		sp.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="` + sid + `" name="Placeholder ` + sid + `"/>` +
			`<p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph` + attr + `/></p:nvPr></p:nvSpPr>` +
			`<p:spPr>` + pptxXfrm(slideBox{X: ph.x, Y: ph.y, W: ph.w, H: ph.h}, pb) + `</p:spPr>` +
			`<p:txBody><a:bodyPr/><a:lstStyle/>` + cmp.Or(ph.text, `<a:p><a:endParaRPr lang="es-ES"/></a:p>`) + `</p:txBody></p:sp>`)
	}
	show := ""
	if l.ocultaPatron {
		show = ` showMasterSp="0"`
	}
	return xmlHeader + `<p:sldLayout ` + pptxNS + ` type="` + l.typ + `" preserve="1"` + show + `><p:cSld name="` + xmlEscape(l.name) + `">` +
		`<p:spTree>` + pptxGrpHeader + sp.String() + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`
}

// pptxAccents son los acentos del esquema: la serie de colores de los gráficos del
// editor (CHART_SERIES de SlidesCanvas.tsx).
var pptxAccents = [6]string{"2563EB", "DC2626", "16A34A", "7C3AED", "D97706", "DB2777"}

// Fuente de un tema que no fija la suya: la de Office por defecto, que es la que
// veían hasta ahora quienes abrían lo exportado (la de la app, Figtree, casi nadie la
// tiene instalada).
const pptxFontDefault = "Calibri"

// pptxThemeXML es el tema: el esquema de colores (fondo y tinta del tema del editor,
// el oscuro en dk1 y el claro en lt1), su fuente y un formato mínimo pero completo,
// que PowerPoint exige para abrir el paquete sin reparación.
func pptxThemeXML(th slideTheme) string {
	dk, lt := th.Ink, th.Bg
	if esOscuro(th.Bg, th.Ink) {
		dk, lt = th.Bg, th.Ink
	}
	font := th.Font
	if font == "" {
		font = pptxFontDefault
	}
	clr := func(tag, v string) string { return `<a:` + tag + `><a:srgbClr val="` + v + `"/></a:` + tag + `>` }
	var acc strings.Builder
	for i, a := range pptxAccents {
		acc.WriteString(clr("accent"+strconv.Itoa(i+1), a))
	}
	name := "Sytue"
	if th.Name != "" {
		name = xmlEscape(th.Name)
	} else if th.ID != "" {
		name += " " + th.ID
	}
	fontLst := `<a:latin typeface="` + xmlEscape(font) + `"/><a:ea typeface=""/><a:cs typeface=""/>`
	return xmlHeader +
		`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="` + name + `"><a:themeElements>` +
		`<a:clrScheme name="` + name + `">` + clr("dk1", dk) + clr("lt1", lt) + clr("dk2", "475569") + clr("lt2", "E2E8F0") +
		acc.String() + clr("hlink", "0563C1") + clr("folHlink", "954F72") + `</a:clrScheme>` +
		`<a:fontScheme name="` + name + `"><a:majorFont>` + fontLst + `</a:majorFont><a:minorFont>` + fontLst + `</a:minorFont></a:fontScheme>` +
		`<a:fmtScheme name="Sytue">` +
		`<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
		`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
		`<a:lnStyleLst><a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
		`<a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
		`<a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
		`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle>` +
		`<a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
		`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
		`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// decodeDataURL descompone un data:URL de imagen en (extensión, bytes). Solo
// admite base64 con MIME de imagen; devuelve ok=false en cualquier otro caso.
func decodeDataURL(src string) (ext string, data []byte, ok bool) {
	if !strings.HasPrefix(src, "data:") {
		return "", nil, false
	}
	comma := strings.IndexByte(src, ',')
	if comma < 0 {
		return "", nil, false
	}
	header := src[5:comma]
	if !strings.Contains(header, ";base64") {
		return "", nil, false
	}
	mime := strings.SplitN(header, ";", 2)[0]
	switch mime {
	case "image/png":
		ext = "png"
	case "image/jpeg", "image/jpg":
		ext = "jpg"
	case "image/gif":
		ext = "gif"
	default:
		return "", nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[comma+1:]))
	if err != nil || len(raw) == 0 {
		return "", nil, false
	}
	return ext, raw, true
}

// --- Partes estáticas del paquete pptx (diseño y notas) ---

const pptxNotesMaster = xmlHeader +
	`<p:notesMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld>` +
	`<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Placeholder"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>` +
	`<p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/>` +
	`<p:txBody><a:bodyPr/><a:lstStyle/><a:p/></p:txBody></p:sp>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" ` +
	`accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/></p:notesMaster>`

// --- HTML: las láminas, de solo lectura ---

// slidesHTML pinta cada diapositiva como un lienzo 16:9 con sus cajas en su sitio
// (x/y/w/h del modelo son porcentajes): es el visor de la vista pública `/s/{token}`
// y lo que sale de «Descargar como HTML». Antes una presentación salía como texto
// plano, sin láminas. Las notas del ponente no salen: son de quien presenta.
//
// ponytail: el tamaño de letra escala con el ancho del lienzo (`cqw`), una
// aproximación al lienzo del editor; si hace falta fidelidad, llevar su escala.
func slidesHTML(b *strings.Builder, title string, s *State) {
	b.WriteString(`<style>body{margin:0;padding:16px;background:#e3dcd5;font-family:system-ui,sans-serif}` +
		`.lamina{position:relative;aspect-ratio:16/9;max-width:1100px;margin:0 auto 16px;background:#fff;` +
		`box-shadow:0 1px 3px rgba(0,0,0,.2);overflow:hidden;container-type:inline-size;font-size:2.4cqw}` +
		`.caja{position:absolute;box-sizing:border-box;overflow:hidden}.caja img{width:100%;height:100%;object-fit:contain}` +
		`.caja p,.caja h1,.caja h2,.caja h3,.caja ul{margin:0 0 .3em}</style>` + "\n")
	for i, sl := range slidesModelOf(title, s).Slides {
		b.WriteString(`<section class="lamina" aria-label="` + strconv.Itoa(i+1) + `">`)
		for _, box := range sl.Boxes {
			st := fmt.Sprintf("left:%.3f%%;top:%.3f%%;width:%.3f%%;height:%.3f%%", box.X, box.Y, box.W, box.H)
			if c := normalizeColor(box.Fill); c != "" {
				st += ";background:#" + c
			}
			if c := normalizeColor(box.Stroke); c != "" {
				st += ";border:2px solid #" + c
			}
			if box.Shape == "ellipse" {
				st += ";border-radius:50%"
			}
			b.WriteString(`<div class="caja" style="` + st + `">`)
			if box.Kind == "image" && box.Src != "" {
				b.WriteString(`<img alt="" src="` + html.EscapeString(box.Src) + `">`)
			} else {
				// El HTML del editor, saneado al salir (ver exportHTML). La página pública
				// lo mete además en un iframe con sandbox y sin scripts.
				b.WriteString(SanearHTML(box.HTML))
			}
			b.WriteString(`</div>`)
		}
		b.WriteString("</section>\n")
	}
}
