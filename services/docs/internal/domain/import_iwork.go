// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"html"
	"image"
	_ "image/gif"  // imágenes incrustadas GIF
	_ "image/jpeg" // vista previa e imágenes JPEG
	_ "image/png"  // imágenes incrustadas PNG
	"io"
	"path"
	"strconv"
	"strings"
)

// # Abrir .pages, .numbers y .key
//
// Apple no publica estos formatos. Lo que se sabe de ellos, de dónde sale y con qué
// licencia está en el registro de fuentes de `docs/security.md` (§4.4, «Formatos de
// iWork»): este fichero solo usa la DISPOSICIÓN del paquete, que se ve listando el ZIP.
//
// Hay dos familias, y las dos llegan como un único ZIP (un paquete-carpeta no se puede
// subir como un fichero; cuando alguien lo comprime, el ZIP envuelve la carpeta):
//
//   - **2013 y posteriores**: `Index/*.iwa` (o un `Index.zip` que los contiene),
//     `Metadata/` y una vista previa `preview.jpg` en la raíz.
//   - **'09**: `index.xml` (Pages, Numbers) o `index.apxl` (Keynote), a veces en `.gz`,
//     con `buildVersionHistory.plist` y una vista previa en `QuickLook/`.
//
// El original NUNCA se toca: abrirlo crea un documento nuevo, y devolver los cambios al
// fichero no está ofrecido para estas extensiones (`save-to-drive.ts` no sabe escribirlas).
//
// El escalón que no falla: lo que no se convierte se abre como un documento con un aviso
// —en el idioma de quien lo abre— y, si el fichero la trae, la imagen de muestra que el
// propio fichero guarda. Nunca un error opaco ni un documento vacío.

const (
	// maxEntradasIWork acota el directorio del ZIP. Un paquete real tiene decenas o
	// cientos de entradas; miles de `Data/` siguen cabiendo.
	maxEntradasIWork = 20_000
	// maxVistaPrevia es lo más que se lee de la imagen de muestra. Las reales pesan
	// entre 10 y 200 KB.
	maxVistaPrevia = 4 << 20
	// maxImagenIWork y maxTotalImagenesIWork acotan lo que se lee de `Data/*` por imagen
	// y en todo el paquete.
	maxImagenIWork        = 4 << 20
	maxTotalImagenesIWork = 16 << 20
	// anchoVistaPrevia es el ancho con que se pinta la imagen en el folio.
	anchoVistaPrevia = 600
)

// paqueteIWork es lo que se sabe de un ZIP de iWork mirando solo su directorio.
type paqueteIWork struct {
	// ficheros por su nombre RELATIVO a la raíz del paquete.
	ficheros map[string]*zip.File
	// v09 es la familia XML; si no, la de 2013 en adelante.
	v09 bool
	// cifrado: el documento lleva contraseña y su contenido no se puede leer.
	cifrado bool
	// bytesImagenes cuenta lo ya leído de imágenes de `Data/*`.
	bytesImagenes int64
}

// abrirPaqueteIWork reconoce un paquete de iWork por su disposición, sin leer ninguna
// entrada. nil si no lo es.
func abrirPaqueteIWork(data []byte) *paqueteIWork {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) == 0 || len(zr.File) > maxEntradasIWork {
		return nil
	}
	raiz := raizDelPaquete(zr.File)
	p := &paqueteIWork{ficheros: make(map[string]*zip.File, len(zr.File))}
	indiceIWA := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Como en readZip: hay herramientas de Windows que escriben «\» en las entradas.
		nombre := strings.TrimPrefix(strings.ReplaceAll(f.Name, `\`, "/"), raiz)
		p.ficheros[nombre] = f
		if strings.HasPrefix(nombre, "Index/") && strings.HasSuffix(nombre, ".iwa") {
			indiceIWA = true
		}
		// '09 cifrado: las entradas van con un método de compresión propio, no con uno
		// de los dos que define el formato ZIP.
		if f.Method != zip.Store && f.Method != zip.Deflate {
			p.cifrado = true
		}
	}
	switch {
	case indiceIWA || p.ficheros["Index.zip"] != nil:
		// 2013+ cifrado: el verificador de la contraseña (y su pista) van en la raíz.
		p.cifrado = p.ficheros[".iwpv2"] != nil || p.ficheros[".iwph"] != nil
		return p
	case p.tieneIndice09() && (p.ficheros["buildVersionHistory.plist"] != nil ||
		p.tieneCarpeta("QuickLook/") || p.ficheros["index.apxl"] != nil || p.ficheros["index.apxl.gz"] != nil):
		// Un ZIP cualquiera puede llevar un `index.xml`; uno de iWork '09 lleva además
		// el historial de versiones o la carpeta de la vista previa.
		p.v09 = true
		return p
	}
	return nil
}

// raizDelPaquete es la carpeta que envuelve el paquete cuando se comprimió la carpeta
// entera («Informe.pages/…»), o "" si las entradas están en la raíz del ZIP.
func raizDelPaquete(fs []*zip.File) string {
	primera, _, ok := strings.Cut(strings.ReplaceAll(fs[0].Name, `\`, "/"), "/")
	if !ok || extIWork(primera) == fmtUnknown {
		return ""
	}
	raiz := primera + "/"
	for _, f := range fs {
		if !strings.HasPrefix(strings.ReplaceAll(f.Name, `\`, "/"), raiz) {
			return ""
		}
	}
	return raiz
}

func (p *paqueteIWork) tieneIndice09() bool {
	for _, n := range []string{"index.xml", "index.xml.gz", "index.apxl", "index.apxl.gz"} {
		if p.ficheros[n] != nil {
			return true
		}
	}
	return false
}

func (p *paqueteIWork) tieneCarpeta(prefijo string) bool {
	for n := range p.ficheros {
		if strings.HasPrefix(n, prefijo) {
			return true
		}
	}
	return false
}

// leer devuelve una entrada del paquete, o nil si no está, no se puede leer (cifrada) o
// pasa de `tope` bytes descomprimida.
func (p *paqueteIWork) leer(nombre string, tope int64) []byte {
	return leerEntrada(p.ficheros[nombre], tope)
}

// leerEntrada es leer para una entrada cualquiera de un ZIP; nil si es nil.
func leerEntrada(f *zip.File, tope int64) []byte {
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, tope+1))
	if err != nil || int64(len(b)) > tope {
		return nil
	}
	return b
}

// extIWork es el formato de iWork que dice la extensión del nombre.
func extIWork(nombre string) srcFormat {
	switch strings.ToLower(path.Ext(nombre)) {
	case ".pages":
		return fmtPages
	case ".numbers":
		return fmtNumbers
	case ".key":
		return fmtKeynote
	}
	return fmtUnknown
}

// esIWork indica si el formato es uno de los de iWork.
func esIWork(f srcFormat) bool {
	return f == fmtPages || f == fmtNumbers || f == fmtKeynote || f == fmtIWork
}

// sniffIWork reconoce un ZIP de iWork y dice de qué aplicación es: por la extensión, y
// si no la trae, por lo que lleva dentro. fmtIWork si es de iWork pero no se sabe de cuál.
func sniffIWork(filename string, data []byte) srcFormat {
	p := abrirPaqueteIWork(data)
	if p == nil {
		return fmtUnknown
	}
	if f := extIWork(filename); f != fmtUnknown {
		return f
	}
	if p.ficheros["index.apxl"] != nil || p.ficheros["index.apxl.gz"] != nil || p.tieneCarpeta("Index/Slide") {
		return fmtKeynote
	}
	// ponytail: sin extensión, Pages y Numbers solo se distinguen leyendo el índice; el
	// aviso sale entonces sin el tipo, que es lo único para lo que hace falta saberlo.
	return fmtIWork
}

// importIWork convierte un fichero de iWork. Lo que todavía no se sabe convertir se abre
// con su vista previa y un aviso (documentoNoConvertido): nunca falla. Lo que se convierte
// a medias lleva delante el aviso de conversión parcial y, al final, la vista previa.
func importIWork(f srcFormat, data []byte, comments bool, idioma string) (Kind, string, []ImportedComment) {
	p := abrirPaqueteIWork(data)
	if p != nil && !p.cifrado {
		// Lo que no cabe en un documento (una hoja de 65 000 filas) tampoco es un error: se
		// abre como lo que no se convierte, con su vista previa.
		if kind, contenido, coms := convertirIWork(f, p, comments, idioma); contenido != "" {
			if _, err := caberEnElDocumento(contenido); err == nil {
				return kind, contenido, coms
			}
		}
	}
	return KindDocument, documentoNoConvertido(f, p, idioma), nil
}

// convertirIWork convierte lo que se sabe convertir; "" si nada. La aplicación la dice el
// propio documento (la raíz del índice '09 o el tipo del objeto raíz IWA), no la extensión.
func convertirIWork(f srcFormat, p *paqueteIWork, comments bool, idioma string) (Kind, string, []ImportedComment) {
	if p.v09 {
		raiz := p.indice09()
		if raiz == nil {
			return "", "", nil
		}
		switch raiz.nombre {
		case "sl:document":
			cuerpo, coms, omitido := importPages09(raiz, p, comments)
			k, c := documentoIWork(f, p, idioma, cuerpo, omitido)
			return k, c, coms
		case "ls:document":
			m, formulas, omitido := importNumbers09(raiz)
			k, c := hojaIWork(f, idioma, m, formulas, omitido)
			return k, c, nil
		case "key:presentation":
			m, omitido := importKeynote09(raiz, p)
			k, c := presentacionIWork(f, p, idioma, m, omitido)
			return k, c, nil
		}
		return "", "", nil
	}
	doc := p.iwa()
	switch {
	case doc.de(1, tipoIWAPages) != nil:
		cuerpo, coms, omitido := importPagesIWA(doc, p, comments)
		k, c := documentoIWork(f, p, idioma, cuerpo, omitido)
		return k, c, coms
	case esKeynoteIWA(doc):
		m, omitido := importKeynoteIWA(doc, p)
		k, c := presentacionIWork(f, p, idioma, m, omitido)
		return k, c, nil
	case doc.de(1, tipoIWADocumento) != nil:
		m, formulas, omitido := importNumbersIWA(doc)
		k, c := hojaIWork(f, idioma, m, formulas, omitido)
		return k, c, nil
	}
	return "", "", nil
}

// documentoIWork es el documento convertido; si algo se quedó fuera, con el aviso de
// conversión parcial delante y la vista previa del fichero al final.
func documentoIWork(f srcFormat, p *paqueteIWork, idioma, cuerpo string, omitido bool) (Kind, string) {
	if cuerpo == "" {
		return "", ""
	}
	if omitido {
		cuerpo = avisoIWork(f, idioma, "titulo.parcial", "texto.parcial") + cuerpo + p.vistaPreviaHTML()
	}
	return KindDocument, cuerpo
}

// hojaIWork es la hoja de cálculo convertida; con fórmulas (que salen con su valor) o con
// algo que se quedó fuera, lleva delante una hoja con el aviso.
func hojaIWork(f srcFormat, idioma string, m *sheetModel, formulas int, omitido bool) (Kind, string) {
	if m == nil || len(m.Sheets) == 0 {
		return "", ""
	}
	if formulas > 0 || omitido {
		usados := map[string]bool{}
		for _, s := range m.Sheets {
			usados[s.Name] = true
		}
		m.Sheets = append([]sheetData{hojaDeAviso(f, idioma, formulas, omitido, usados)}, m.Sheets...)
	}
	return KindSpreadsheet, marshalSheetModel(m)
}

// presentacionIWork es la presentación convertida; si algo se quedó fuera, termina con una
// diapositiva con el aviso y la vista previa del fichero.
func presentacionIWork(f srcFormat, p *paqueteIWork, idioma string, m *slidesModel, omitido bool) (Kind, string) {
	if m == nil || len(m.Slides) == 0 {
		return "", ""
	}
	if omitido {
		sl := slideT{ID: "s" + strconv.Itoa(len(m.Slides)+1)}
		sl.Boxes = []slideBox{{
			ID: sl.ID + "b1", Kind: "text", X: 5, Y: 4, W: 90, H: 30,
			HTML: avisoIWork(f, idioma, "titulo.parcial", "texto.parcial"),
		}}
		if src, w, h := p.vistaPreviaDataURL(); src != "" {
			// La imagen, sin deformar, en lo que queda de diapositiva: las cajas van en % del
			// ancho y del alto, así que su proporción depende de la de la diapositiva.
			lienzo := 16.0 / 9
			switch {
			case m.Size != nil && m.Size.Preset == "4:3":
				lienzo = 4.0 / 3
			case m.Size != nil && m.Size.W > 0 && m.Size.H > 0:
				lienzo = m.Size.W / m.Size.H
			}
			prop := float64(w) / float64(h) / lienzo
			alto := min(56, 90/prop)
			sl.Boxes = append(sl.Boxes, slideBox{
				ID: sl.ID + "b2", Kind: "image", Src: src,
				X: roundPct(50 - alto*prop/2), Y: 38, W: roundPct(alto * prop), H: roundPct(alto),
			})
		}
		m.Slides = append(m.Slides, sl)
	}
	return KindPresentation, marshalSlidesModel(m)
}

// documentoNoConvertido es el documento que se abre cuando el fichero no se puede
// convertir: un aviso que dice por qué y qué hacer con el original y, si el fichero
// trae una, su imagen de muestra. `p` es nil si ni siquiera es un paquete legible.
func documentoNoConvertido(f srcFormat, p *paqueteIWork, idioma string) string {
	img := ""
	if p != nil && !p.cifrado {
		img = p.vistaPreviaHTML()
	}
	titulo, texto := "titulo.no_convertido", "texto.sin_vista_previa"
	switch {
	case p != nil && p.cifrado:
		titulo, texto = "titulo.protegido", "texto.protegido"
	case img != "":
		titulo, texto = "titulo.vista_previa", "texto.vista_previa"
	}
	return avisoIWork(f, idioma, titulo, texto) + img
}

// avisoIWork es el aviso que el importador escribe en el documento: `titulo` · el tipo de
// fichero, y `texto`, en el idioma de quien lo abre.
func avisoIWork(f srcFormat, idioma, titulo, texto string) string {
	t := avisoImportar(idioma, titulo)
	if tipo, ok := tiposIWork[f]; ok {
		t += " · " + avisoImportar(idioma, tipo)
	}
	return "<blockquote><p><strong>" + html.EscapeString(t) + "</strong></p><p>" +
		html.EscapeString(avisoImportar(idioma, texto)) + "</p></blockquote>"
}

// vistaPreviaHTML es la imagen de muestra pintada a lo ancho del folio, o "".
func (p *paqueteIWork) vistaPreviaHTML() string {
	src, w, h := p.vistaPreviaDataURL()
	if src == "" {
		return ""
	}
	if w > anchoVistaPrevia {
		w, h = anchoVistaPrevia, h*anchoVistaPrevia/w
	}
	return "<p>" + imgTag(src, w, h) + "</p>"
}

// vistaPreviaDataURL es la imagen de muestra como data:URL y su tamaño, o "".
func (p *paqueteIWork) vistaPreviaDataURL() (src string, w, h int) {
	img, cfg := p.vistaPrevia()
	if img == nil {
		return "", 0, 0
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(img), cfg.Width, cfg.Height
}

// vistaPrevia es la imagen de muestra que guarda el propio fichero —la primera página,
// hoja o diapositiva— y su tamaño, o nil si no trae ninguna que se pueda leer.
func (p *paqueteIWork) vistaPrevia() ([]byte, image.Config) {
	for _, n := range []string{"preview.jpg", "preview-web.jpg", "QuickLook/Thumbnail.jpg"} {
		b := p.leer(n, maxVistaPrevia)
		if b == nil {
			continue
		}
		if cfg, formato, err := image.DecodeConfig(bytes.NewReader(b)); err == nil && formato == "jpeg" &&
			cfg.Width > 0 && cfg.Height > 0 {
			return b, cfg
		}
	}
	return nil, image.Config{}
}

// tiposIWork es la clave del nombre de cada tipo de fichero en los avisos; fmtIWork no
// está porque no se sabe cuál es.
var tiposIWork = map[srcFormat]string{fmtPages: "tipo.pages", fmtNumbers: "tipo.numbers", fmtKeynote: "tipo.key"}

// esMiniaturaIWork distingue las vistas previas y miniaturas del paquete (`QuickLook/`,
// `thumbs/`, `*-small*`, `mt-*`, `st-*`, `mt[0-9]*`, `st[0-9]*`, `PresetImageFill*`) de
// las imágenes del documento.
func esMiniaturaIWork(nombre string) bool {
	low := strings.ToLower(strings.ReplaceAll(nombre, `\`, "/"))
	if strings.HasPrefix(low, "quicklook/") || strings.Contains(low, "/quicklook/") ||
		strings.HasPrefix(low, "thumbs/") || strings.Contains(low, "/thumbs/") ||
		strings.HasPrefix(low, "templates/") || strings.HasPrefix(low, "charts/") {
		return true
	}
	base := path.Base(low)
	if strings.HasPrefix(base, "preview") || strings.HasPrefix(base, "mt-") || strings.HasPrefix(base, "st-") ||
		strings.HasPrefix(base, "presetimagefill") || strings.Contains(base, "-small") {
		return true
	}
	if (strings.HasPrefix(base, "mt") || strings.HasPrefix(base, "st")) && len(base) > 2 && base[2] >= '0' && base[2] <= '9' {
		return true
	}
	return false
}

func (p *paqueteIWork) buscarEnDatosIWork(pred func(string) bool) *zip.File {
	mejor := ""
	for n := range p.ficheros {
		if strings.HasPrefix(n, "Data/") && !esMiniaturaIWork(n) && pred(n) && (mejor == "" || n < mejor) {
			mejor = n
		}
	}
	return p.ficheros[mejor]
}

// buscarImagenIWork localiza una imagen del documento en el paquete a partir de las rutas
// o sufijos candidatos (`Data/foto.jpg`, `foto.jpg`, `-701.`), descartando miniaturas.
func (p *paqueteIWork) buscarImagenIWork(candidatos ...string) *zip.File {
	if p == nil {
		return nil
	}
	for _, c := range candidatos {
		c = strings.TrimSpace(strings.ReplaceAll(c, `\`, "/"))
		if c == "" {
			continue
		}
		if strings.HasPrefix(c, "-") && strings.HasSuffix(c, ".") {
			if f := p.buscarEnDatosIWork(func(n string) bool { return strings.Contains(n, c) }); f != nil {
				return f
			}
			continue
		}
		for _, ruta := range []string{"Data/" + strings.TrimPrefix(c, "Data/"), c} {
			if f := p.ficheros[ruta]; f != nil && !esMiniaturaIWork(ruta) {
				return f
			}
		}
		base := strings.ToLower(path.Base(c))
		if f := p.buscarEnDatosIWork(func(n string) bool { return strings.ToLower(path.Base(n)) == base }); f != nil {
			return f
		}
	}
	return nil
}

// imagenDataURL lee y convierte una imagen incrustada del paquete a data:URL con sus
// dimensiones en píxeles, respetando el tope por imagen y el tope total del paquete.
func (p *paqueteIWork) imagenDataURL(candidatos ...string) (src string, w, h int) {
	f := p.buscarImagenIWork(candidatos...)
	if f == nil || p.bytesImagenes >= maxTotalImagenesIWork {
		return "", 0, 0
	}
	tope := min(int64(maxImagenIWork), maxTotalImagenesIWork-p.bytesImagenes)
	raw := leerEntrada(f, tope)
	if len(raw) == 0 {
		return "", 0, 0
	}
	ext := strings.ToLower(path.Ext(f.Name))
	if ext == ".tif" || ext == ".tiff" {
		u := tiffADataURL(raw)
		if u == "" {
			return "", 0, 0
		}
		p.bytesImagenes += int64(len(raw))
		_, b64, _ := strings.Cut(u, ",")
		if dec, err := base64.StdEncoding.DecodeString(b64); err == nil {
			if cfg, _, err := image.DecodeConfig(bytes.NewReader(dec)); err == nil {
				return u, cfg.Width, cfg.Height
			}
		}
		return u, 0, 0
	}
	cfg, formato, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return "", 0, 0
	}
	switch formato {
	case "jpeg", "png", "gif":
	default:
		return "", 0, 0
	}
	p.bytesImagenes += int64(len(raw))
	return "data:image/" + formato + ";base64," + base64.StdEncoding.EncodeToString(raw), cfg.Width, cfg.Height
}

// imgHTML devuelve la etiqueta <img> escalada a lo ancho del folio sin deformarla.
func (p *paqueteIWork) imgHTML(src string, w, h int) string {
	if w > anchoVistaPrevia && h > 0 {
		w, h = anchoVistaPrevia, max(1, h*anchoVistaPrevia/w)
	}
	return imgTag(src, w, h)
}
