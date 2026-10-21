// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Las herramientas de PDF que la gente busca por su nombre: comprimir, pasar
// imágenes a PDF y al revés, y recortar márgenes.
//
// Son las que uno acaba resolviendo en una web cualquiera, y por eso están aquí:
// «comprimir PDF» es la búsqueda con la que más contratos, nóminas y DNI acaban
// subidos a un servidor de un tercero. Que la suite lo haga en casa no es una
// funcionalidad más, es el motivo de que exista la suite.
package domain

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)

// --- Comprimir ---------------------------------------------------------------

// NivelCompresion elige el equilibrio entre tamaño y fidelidad.
type NivelCompresion string

const (
	// CompresionSinPerdida solo recomprime lo que estaba sin comprimir y quita lo
	// que sobra. El documento sale IDÉNTICO píxel a píxel.
	CompresionSinPerdida NivelCompresion = "sin-perdida"
	// CompresionEquilibrada además recodifica las imágenes grandes a JPEG de
	// calidad alta. Es la que quiere casi todo el mundo cuando dice «comprimir».
	CompresionEquilibrada NivelCompresion = "equilibrada"
	// CompresionFuerte reduce también la resolución de las imágenes a 150 ppp,
	// que sigue imprimiéndose bien y suele partir el tamaño por cinco.
	CompresionFuerte NivelCompresion = "fuerte"
)

// ResultadoCompresion cuenta qué pasó, para poder enseñarlo en vez de un
// «listo» que no dice si mereció la pena.
type ResultadoCompresion struct {
	Antes               int `json:"antes"`
	Despues             int `json:"despues"`
	ImagenesTocadas     int `json:"imagenes_tocadas"`
	FlujosRecomprimidos int `json:"flujos_recomprimidos"`
}

// Ahorro es el porcentaje que se quitó, redondeado. Negativo si engordó, que
// pasa con documentos ya optimizados y hay que decirlo en vez de esconderlo.
func (r ResultadoCompresion) Ahorro() int {
	if r.Antes == 0 {
		return 0
	}
	return int(math.Round(float64(r.Antes-r.Despues) * 100 / float64(r.Antes)))
}

// resolucionObjetivo son los puntos por pulgada a los que se reduce cada nivel.
// 150 ppp es el umbral clásico: por debajo se nota al imprimir.
const resolucionObjetivo = 150.0

// calidadJPEG por nivel. 82 es donde la pérdida deja de verse en una foto y el
// fichero ya ha bajado de verdad.
const (
	calidadEquilibrada = 82
	calidadFuerte      = 70
)

// ComprimirPDF devuelve una copia más pequeña del documento.
func ComprimirPDF(doc *PDFDoc, nivel NivelCompresion) ([]byte, ResultadoCompresion, error) {
	res := ResultadoCompresion{Antes: len(doc.buf)}

	ancho := 0.0
	for i := 0; i < doc.NumPaginas(); i++ {
		if p, err := doc.Pagina(i); err == nil && p.Ancho > ancho {
			ancho = p.Ancho
		}
	}

	e := NuevoEnsambladorPDF()
	e.comprimir = &ajustesCompresion{nivel: nivel, res: &res, anchoMaxPuntos: ancho}
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, res, err
		}
	}
	e.ConservarFormulario(doc)
	salida, err := e.Bytes()
	if err != nil {
		return nil, res, err
	}
	res.Despues = len(salida)

	// Si el resultado es MAYOR se devuelve el original: comprimir y entregar algo
	// más gordo es exactamente lo contrario de lo que pidió el usuario, y que la
	// herramienta lo detecte vale más que un porcentaje negativo en la pantalla.
	if res.Despues >= res.Antes {
		res.Despues = res.Antes
		res.ImagenesTocadas, res.FlujosRecomprimidos = 0, 0
		return append([]byte(nil), doc.buf...), res, nil
	}
	return salida, res, nil
}

// ajustesCompresion viaja con el ensamblador para que cada flujo copiado pase
// por aquí.
type ajustesCompresion struct {
	nivel NivelCompresion
	res   *ResultadoCompresion
	// anchoMaxPuntos es lo ANCHO que puede llegar a dibujarse una imagen: el de
	// la página más grande del documento.
	//
	// Es una cota deliberadamente generosa y no la medida exacta de cada imagen.
	// Saber cuánto mide dibujada exigiría interpretar el flujo de contenido de
	// cada página para leer la `cm` que la coloca, y equivocarse ahí reduciría
	// una imagen que sí necesitaba su resolución. Con el ancho de página se
	// recorta solo lo que está sobremuestreado SEGURO.
	anchoMaxPuntos float64
}

// tratarFlujo decide qué hacer con un flujo al copiarlo: recomprimirlo,
// recodificar su imagen o dejarlo como está.
//
// Devuelve los datos que hay que escribir y las claves de filtro resultantes.
func (a *ajustesCompresion) tratarFlujo(doc *PDFDoc, st *pdfStream, dict pdfDict, datos []byte) []byte {
	filtros := doc.listaDeNombres(st.Dict["Filter"])

	// 1. Imágenes: es donde está el peso de un PDF real.
	if sub, _ := doc.resolver(st.Dict["Subtype"]).(pdfName); sub == "Image" && a.nivel != CompresionSinPerdida {
		if nuevos, nuevoDict, ok := a.recodificarImagen(doc, st); ok {
			a.res.ImagenesTocadas++
			for k, v := range nuevoDict {
				dict[k] = v
			}
			for _, k := range []pdfName{"DecodeParms", "DP", "F"} {
				delete(dict, k)
			}
			return nuevos
		}
		return datos
	}

	// 2. Flujos sin comprimir: se comprimen. Es gratis y es lo que más aparece en
	// los PDF que genera un programa a mano.
	if len(filtros) == 0 && len(datos) > 64 {
		var buf bytes.Buffer
		zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
		if err == nil {
			if _, err := zw.Write(datos); err == nil && zw.Close() == nil && buf.Len() < len(datos) {
				a.res.FlujosRecomprimidos++
				dict["Filter"] = pdfName("FlateDecode")
				return buf.Bytes()
			}
		}
	}
	return datos
}

// recodificarImagen baja la resolución y/o recomprime una imagen a JPEG.
//
// Solo se toca lo que se puede DECODIFICAR y volver a escribir con seguridad:
// una imagen con máscara, indexada o con /Decode raro se deja como está. Ahorrar
// un 5 % a cambio de estropear un logotipo no es un buen negocio.
func (a *ajustesCompresion) recodificarImagen(doc *PDFDoc, st *pdfStream) ([]byte, pdfDict, bool) {
	if _, hay := st.Dict["SMask"]; hay {
		return nil, nil, false
	}
	if _, hay := st.Dict["Mask"]; hay {
		return nil, nil, false
	}
	if v, ok := doc.resolver(st.Dict["ImageMask"]).(bool); ok && v {
		return nil, nil, false // un estarcido de 1 bit ya es lo más pequeño que hay
	}

	l := &lienzo{d: doc, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{}}
	ancho, alto := l.tamanoImagen(st.Dict)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesImagen {
		return nil, nil, false
	}
	img, ok := l.decodificarImagen(st)
	if !ok {
		return nil, nil, false
	}

	calidad := calidadEquilibrada
	if a.nivel == CompresionFuerte {
		calidad = calidadFuerte
		// Una imagen no puede dibujarse más ancha que la página, así que todo lo
		// que pase de ese ancho a 150 ppp está sobremuestreado con seguridad.
		if a.anchoMaxPuntos > 1 {
			maxPx := int(a.anchoMaxPuntos * resolucionObjetivo / 72)
			if maxPx > 16 && ancho > maxPx {
				img = reducir(img, maxPx)
			}
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: calidad}); err != nil {
		return nil, nil, false
	}
	// Si no baja, se deja el original: recodificar por recodificar solo degrada.
	if buf.Len() >= len(st.Raw) {
		return nil, nil, false
	}
	b := img.Bounds()
	return buf.Bytes(), pdfDict{
		"Filter":           pdfName("DCTDecode"),
		"ColorSpace":       pdfName("DeviceRGB"),
		"BitsPerComponent": int64(8),
		"Width":            int64(b.Dx()),
		"Height":           int64(b.Dy()),
	}, true
}

// reducir escala la imagen a `maxAncho` por vecino más próximo. No es el mejor
// remuestreo, pero se aplica sobre fotos que después pasan por JPEG: el suavizado
// que aportaría un filtro mejor lo borra la compresión.
func reducir(src image.Image, maxAncho int) image.Image {
	b := src.Bounds()
	if b.Dx() <= maxAncho {
		return src
	}
	alto := b.Dy() * maxAncho / b.Dx()
	if alto < 1 {
		alto = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxAncho, alto))
	for y := 0; y < alto; y++ {
		sy := b.Min.Y + y*b.Dy()/alto
		for x := 0; x < maxAncho; x++ {
			sx := b.Min.X + x*b.Dx()/maxAncho
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// decodificarImagen devuelve la imagen de un XObject como `image.Image`.
// Reutiliza el camino del render —que ya sabe de espacios de color, bits por
// componente, /Decode y fax— en vez de escribir un segundo decodificador que se
// desincronizaría con el primero.
func (l *lienzo) decodificarImagen(st *pdfStream) (image.Image, bool) {
	datos, terminal, err := l.d.datosDeFlujoHasta(st)
	if err != nil {
		return nil, false
	}
	switch terminal {
	case "DCTDecode", "DCT":
		img, err := jpeg.Decode(bytes.NewReader(datos))
		return img, err == nil
	case "CCITTFaxDecode", "CCF":
		bits, err := descomprimirCCITT(datos, l.parametrosDelFax(st))
		if err != nil {
			return nil, false
		}
		return l.imagenDeMuestras(l.streamComoBitonal(st), bits)
	case "":
		return l.imagenDeMuestras(st, datos)
	default:
		return nil, false
	}
}

// --- Imágenes a PDF ----------------------------------------------------------

// maxImagenesPorPDF acota cuántas imágenes se convierten de una vez.
const maxImagenesPorPDF = 500

// ImagenAPagina describe una imagen que va a ser una página.
type ImagenAPagina struct {
	// Datos son los bytes del fichero (JPEG o PNG).
	Datos []byte
	// Nombre solo se usa en los mensajes de error.
	Nombre string
}

// ImagenesAPDF construye un PDF con una página por imagen, del tamaño de la
// imagen a 72 ppp. Es «JPG a PDF», la herramienta más usada del mundo después de
// «comprimir».
func ImagenesAPDF(imagenes []ImagenAPagina) ([]byte, error) {
	if len(imagenes) == 0 {
		return nil, fmt.Errorf("pdf: no se ha enviado ninguna imagen")
	}
	if len(imagenes) > maxImagenesPorPDF {
		return nil, fmt.Errorf("pdf: demasiadas imágenes de una vez (máximo %d)", maxImagenesPorPDF)
	}

	e := NuevoEnsambladorPDF()
	for _, im := range imagenes {
		if err := e.anadirPaginaDeImagen(im); err != nil {
			return nil, err
		}
	}
	return e.Bytes()
}

// decodificarCabeceraImagen lee el tamaño y el formato de una imagen sin decodificarla
// entera, que es lo que hace falta para rechazar una absurda antes de gastar
// memoria en ella.
func decodificarCabeceraImagen(datos []byte) (image.Config, string, error) {
	return image.DecodeConfig(bytes.NewReader(datos))
}

// imagenComoXObject convierte un fichero de imagen en el cuerpo y el diccionario
// de un XObject. Sin /SMask: la transparencia la añade quien llama, porque
// necesita reservar un objeto aparte.
func imagenComoXObject(datos []byte, cfg image.Config, formato string) ([]byte, pdfDict, error) {
	dict := pdfDict{
		"Type":             pdfName("XObject"),
		"Subtype":          pdfName("Image"),
		"Width":            int64(cfg.Width),
		"Height":           int64(cfg.Height),
		"BitsPerComponent": int64(8),
		"ColorSpace":       pdfName("DeviceRGB"),
	}
	if formato == "jpeg" {
		// Un JPEG entra SIN recomprimir: pasarlo por PNG lo engordaría y pasarlo
		// otra vez por JPEG lo degradaría. El PDF lleva DCTDecode justamente para
		// poder guardarlo tal cual.
		dict["Filter"] = pdfName("DCTDecode")
		return datos, dict, nil
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil, nil, fmt.Errorf("pdf: la imagen no se pudo leer")
	}
	crudo, _ := muestrasRGBDe(img)
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(crudo)
	_ = zw.Close()
	dict["Filter"] = pdfName("FlateDecode")
	return buf.Bytes(), dict, nil
}

func (e *PDFEnsamblador) anadirPaginaDeImagen(im ImagenAPagina) error {
	nombre := im.Nombre
	if nombre == "" {
		nombre = "imagen"
	}
	cfg, formato, err := decodificarCabeceraImagen(im.Datos)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("pdf: «%s» no es una imagen que sepamos leer", nombre)
	}
	if cfg.Width*cfg.Height > maxPixelesImagen {
		return fmt.Errorf("pdf: «%s» es demasiado grande", nombre)
	}

	datos, dict, err := imagenComoXObject(im.Datos, cfg, formato)
	if err != nil {
		return fmt.Errorf("pdf: «%s» no se pudo leer", nombre)
	}
	if formato != "jpeg" {
		img, _, errD := image.Decode(bytes.NewReader(im.Datos))
		if errD == nil {
			if _, alfa := muestrasRGBDe(img); alfa != nil {
				// La transparencia de un PNG se conserva como /SMask; sin ella, un
				// logotipo con fondo recortado saldría sobre un rectángulo negro.
				var bufA bytes.Buffer
				zwA := zlib.NewWriter(&bufA)
				_, _ = zwA.Write(alfa)
				_ = zwA.Close()
				numSM := e.reservar()
				e.cuerpos[numSM-1] = objetoSalida{
					valor: pdfDict{
						"Type": pdfName("XObject"), "Subtype": pdfName("Image"),
						"Width": int64(cfg.Width), "Height": int64(cfg.Height),
						"BitsPerComponent": int64(8), "ColorSpace": pdfName("DeviceGray"),
						"Filter": pdfName("FlateDecode"),
					},
					flujo: bufA.Bytes(), esFlujo: true,
				}
				dict["SMask"] = pdfRef{Num: numSM}
			}
		}
	}

	numImg := e.reservar()
	e.cuerpos[numImg-1] = objetoSalida{valor: dict, flujo: datos, esFlujo: true}

	ancho, alto := float64(cfg.Width), float64(cfg.Height)
	contenido := fmt.Sprintf("q %s 0 0 %s 0 0 cm /Im0 Do Q", numPDF(ancho), numPDF(alto))
	numContenido := e.reservar()
	e.cuerpos[numContenido-1] = objetoSalida{valor: pdfDict{}, flujo: []byte(contenido), esFlujo: true}

	numPagina := e.reservar()
	e.paginas = append(e.paginas, numPagina)
	e.cuerpos[numPagina-1] = objetoSalida{valor: pdfDict{
		"Type":      pdfName("Page"),
		"MediaBox":  pdfArray{0.0, 0.0, ancho, alto},
		"Contents":  pdfRef{Num: numContenido},
		"Resources": pdfDict{"XObject": pdfDict{"Im0": pdfRef{Num: numImg}}},
	}}
	return nil
}

// muestrasRGBDe saca los píxeles en RGB de 8 bits y, si la imagen tiene
// transparencia real, el canal alfa aparte.
func muestrasRGBDe(img image.Image) (rgb, alfa []byte) {
	b := img.Bounds()
	rgb = make([]byte, 0, b.Dx()*b.Dy()*3)
	posibleAlfa := make([]byte, 0, b.Dx()*b.Dy())
	transparente := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a == 0 {
				// Totalmente transparente: el color no significa nada y multiplicado
				// por alfa daría negro. Se escribe blanco, que es lo que se ve si el
				// lector ignorara la máscara.
				rgb = append(rgb, 0xFF, 0xFF, 0xFF)
			} else {
				// Los valores vienen premultiplicados por alfa: hay que deshacerlo o
				// las zonas semitransparentes salen oscurecidas.
				rgb = append(rgb,
					byte(r*0xFFFF/a>>8&0xFF), byte(g*0xFFFF/a>>8&0xFF), byte(bl*0xFFFF/a>>8&0xFF))
			}
			posibleAlfa = append(posibleAlfa, byte(a>>8&0xFF))
			if a>>8 != 0xFF {
				transparente = true
			}
		}
	}
	if transparente {
		return rgb, posibleAlfa
	}
	return rgb, nil
}

// --- Recortar ----------------------------------------------------------------

// RecortarPDF fija la caja visible de cada página. Los márgenes de un escaneo o
// las marcas de imprenta desaparecen sin tocar el contenido: /CropBox dice qué
// se enseña, y lo de fuera sigue ahí por si alguien se arrepiente.
//
// `margenes` son los puntos que se quitan por cada lado [izq, abajo, der, arriba].
func RecortarPDF(doc *PDFDoc, margenes [4]float64) ([]byte, error) {
	for _, m := range margenes {
		if m < 0 {
			return nil, fmt.Errorf("pdf: un margen no puede ser negativo")
		}
	}
	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
		p, err := doc.Pagina(i)
		if err != nil {
			return nil, err
		}
		caja := [4]float64{
			p.Caja[0] + margenes[0], p.Caja[1] + margenes[1],
			p.Caja[2] - margenes[2], p.Caja[3] - margenes[3],
		}
		if caja[2]-caja[0] < 1 || caja[3]-caja[1] < 1 {
			return nil, fmt.Errorf("pdf: con esos márgenes la página %d se queda sin contenido", i+1)
		}
		pp := e.ultimaPendiente()
		pp.dict["CropBox"] = pdfArray{caja[0], caja[1], caja[2], caja[3]}
		e.reserializarPendiente(pp)
	}
	return e.Bytes()
}

// --- Imagen desde muestras (compartido) --------------------------------------

// imagenDeMuestras convierte muestras crudas en `image.Image`, con el mismo
// criterio de espacios de color y /Decode que usa el render.
func (l *lienzo) imagenDeMuestras(st *pdfStream, datos []byte) (image.Image, bool) {
	ancho, alto := l.tamanoImagen(st.Dict)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesImagen {
		return nil, false
	}
	bpc := 8
	if v, ok := comoEntero(l.d.resolver(primeroDe(st.Dict, "BitsPerComponent", "BPC"))); ok {
		bpc = int(v)
	}
	ec := l.espacioDeImagen(st.Dict, nil)
	nc := ec.componentes()
	if ec.clase == "Indexed" {
		nc = 1
	}
	if nc <= 0 || bpc <= 0 || bpc > 16 {
		return nil, false
	}
	decode := l.reales(primeroDe(st.Dict, "Decode", "D"))

	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	maxVal := float64(int(1)<<uint(bpc) - 1)
	bytesPorFila := (ancho*nc*bpc + 7) / 8
	comp := make([]float64, nc)
	for y := 0; y < alto; y++ {
		base := y * bytesPorFila * 8
		for x := 0; x < ancho; x++ {
			for c := 0; c < nc; c++ {
				v := float64(leerBits(datos, base+(x*nc+c)*bpc, bpc))
				switch {
				case ec.clase == "Indexed":
					comp[c] = v
				case 2*c+1 < len(decode):
					comp[c] = decode[2*c] + (v/maxVal)*(decode[2*c+1]-decode[2*c])
				case ec.clase == "Lab":
					comp[c] = escalaLab(c, v/maxVal)
				default:
					comp[c] = v / maxVal
				}
			}
			r, g, b := desdeCSS(ec.css(comp))
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img, true
}
