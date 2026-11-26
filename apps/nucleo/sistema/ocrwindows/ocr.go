// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ocrwindows reconoce texto con el OCR propio de Windows (`Windows.Media.Ocr`,
// Windows 10 y posteriores) para CertiPDF de Sytue Office (PLAN-EDICION-LOCAL §6.3, P-07):
// la imagen de una página escaneada entra y salen sus palabras con su caja, como las da
// tesseract, sin que nada salga del equipo ni haya que instalar nada (usa los idiomas de
// reconocimiento que Windows ya trae, los de la persona).
//
// Go puro, sin cgo: la API es de WinRT y se llama por sus tablas de métodos con
// `nucleo/sistema/com`, como Media Foundation (`apps/fyne/video/sistema/mediafoundation`).
// Las clases se activan con `RoGetActivationFactory` (combase.dll), la imagen se le da como
// un `SoftwareBitmap` BGRA8 copiado de un `IBuffer` (`CryptographicBuffer`) y la operación
// asíncrona se espera preguntando su estado (`IAsyncInfo`), sin delegados. Se compila en
// cruzado (`GOOS=windows CGO_ENABLED=0 go vet`).
//
// Defensivo: si falta combase.dll o una clase (Windows Server sin la experiencia de
// escritorio, una versión vieja), no hay ningún idioma de reconocimiento instalado o algo
// falla, [Disponible] da false o [Reconocer] devuelve un error, y CertiPDF sigue con lo de
// siempre (tesseract si está; si no, el modelo de visión de la persona).
//
// Lo que no es WinRT (descodificar la imagen y reducirla al lado máximo del motor, pasarla
// a BGRA8, devolver las cajas a la imagen original y casar los idiomas de tesseract que usa
// la pantalla, `spa+eng`, con los de Windows, `es-ES`) está en este fichero, sin etiqueta
// de sistema, y se prueba en cualquiera.
//
// SIN PROBAR en un Windows de verdad (se escribió en un equipo sin Windows). Qué mirar la
// primera vez, con CertiPDF y un PDF escaneado:
//
//  1. Que arranca y no se cae al abrir las herramientas: los índices de los métodos y los
//     IID salen de las cabeceras del SDK de Windows (windows.media.ocr.h,
//     windows.graphics.imaging.h, windows.security.cryptography.h, windows.foundation.h);
//     un índice mal es un fallo de acceso, no un error.
//  2. Que la nota del OCR dice «OCR de Windows» y el selector de idiomas enseña los del
//     sistema (Configuración → Hora e idioma → Idioma; cada idioma con «Reconocimiento
//     óptico de caracteres»).
//  3. Una página A4 a 300 ppp (2480 × 3508, más que el lado máximo del motor, 2600 hoy):
//     el texto cae encima de lo escrito (las cajas vuelven a la escala original) y se puede
//     buscar y seleccionar.
//  4. Español con tildes y eñes, y un documento en inglés con el idioma en «English».
//  5. Sin ningún idioma de OCR instalado: Disponible da false y CertiPDF ofrece lo de
//     siempre (o nada).
//  6. Cancelar a mitad de un documento largo: que pare y no deje hilos colgados.
package ocrwindows

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	_ "image/jpeg" // las imágenes de un escaneo llegan en JPEG o PNG
	_ "image/png"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// Palabra es una palabra reconocida y su caja en PÍXELES de la imagen que se dio, con el
// origen arriba a la izquierda (`api.PDFPalabra`).
type Palabra struct {
	Texto             string
	X, Y, Ancho, Alto int
}

// ErrNoDisponible: en este sistema no hay OCR de Windows (o no tiene ningún idioma).
var ErrNoDisponible = errors.New("ocrwindows: el OCR de Windows no está en este sistema")

// preguntar y pasarAlMotor son lo del sistema (WinRT en Windows); las pruebas ponen otro.
var (
	preguntar    = consultar
	pasarAlMotor = reconocer
)

// maxLadoPorDefecto es el lado máximo que acepta el motor si no lo dice
// (`OcrEngine.MaxImageDimension`, 2600 píxeles hoy).
const maxLadoPorDefecto = 2600

// Disponible dice si se puede reconocer: Windows con su OCR y al menos un idioma.
func Disponible() bool {
	c, err := preguntar()
	return err == nil && len(c.etiquetas) > 0
}

// Idiomas son los de reconocimiento instalados, con los códigos de tesseract que usa la
// pantalla (`spa`, `eng`), sin repetir y en el orden de Windows. Uno que tesseract no
// conoce va con su etiqueta BCP-47 tal cual.
func Idiomas() ([]string, error) {
	c, err := preguntar()
	if err != nil {
		return nil, err
	}
	if len(c.etiquetas) == 0 {
		return nil, ErrNoDisponible
	}
	var out []string
	for _, e := range c.etiquetas {
		codigo := e
		if t, ok := codigoTesseract(e); ok {
			codigo = t
		}
		if !contiene(out, codigo) {
			out = append(out, codigo)
		}
	}
	return out, nil
}

// Reconocer lee una imagen (JPEG o PNG) en el primero de idiomas («spa+eng», con los
// códigos de [Idiomas]) que Windows sepa reconocer; sin ninguno, en los idiomas de la
// persona. El motor de Windows reconoce un idioma cada vez: en un documento de empresa
// en español con algo de inglés, el español lee bien los dos.
func Reconocer(ctx context.Context, imagen []byte, idiomas string) ([]Palabra, error) {
	c, err := preguntar()
	if err != nil {
		return nil, err
	}
	if len(c.etiquetas) == 0 {
		return nil, ErrNoDisponible
	}
	img, err := paraOCR(imagen, c.maxLado)
	if err != nil {
		return nil, err
	}
	crudas, err := pasarAlMotor(ctx, img.bgra, img.ancho, img.alto, elegirIdioma(idiomas, c.etiquetas))
	if err != nil {
		return nil, err
	}
	out := make([]Palabra, 0, len(crudas))
	for _, w := range crudas {
		if p, ok := img.aOriginal(w); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// consulta es lo que se sabe del OCR del sistema: los idiomas instalados (BCP-47) y el
// lado máximo de imagen.
type consulta struct {
	etiquetas []string
	maxLado   int
}

// palabraCruda es una palabra tal como la da el motor: su caja en coma flotante sobre la
// imagen que se le dio (la reducida).
type palabraCruda struct {
	texto      string
	x, y, w, h float32
}

// ----- La imagen -----

// imagenOCR es la imagen que se le da al motor: BGRA8 sin relleno entre filas, reducida
// si hacía falta, y lo que hace falta para devolver las cajas a la original.
type imagenOCR struct {
	bgra                []byte
	ancho, alto         int
	anchoOrig, altoOrig int
	escala              float64 // de la original a la reducida, ≤ 1
}

// paraOCR descodifica la imagen, la reduce si un lado pasa de maxLado (manteniendo la
// proporción) y la pasa a BGRA8 opaca.
func paraOCR(datos []byte, maxLado int) (imagenOCR, error) {
	if maxLado <= 0 {
		maxLado = maxLadoPorDefecto
	}
	src, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return imagenOCR{}, errors.New("ocrwindows: la imagen de la página no se puede leer")
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return imagenOCR{}, errors.New("ocrwindows: la imagen de la página está vacía")
	}
	out := imagenOCR{anchoOrig: b.Dx(), altoOrig: b.Dy(), escala: 1}
	ancho, alto := b.Dx(), b.Dy()
	if lado := max(ancho, alto); lado > maxLado {
		out.escala = float64(maxLado) / float64(lado)
		ancho = max(1, int(math.Round(float64(ancho)*out.escala)))
		alto = max(1, int(math.Round(float64(alto)*out.escala)))
	}
	rgba := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	// Debajo, blanco: un PNG con transparencia es papel, no negro.
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)
	if out.escala == 1 {
		draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Over)
	} else {
		xdraw.CatmullRom.Scale(rgba, rgba.Bounds(), src, b, draw.Over, nil)
	}
	out.ancho, out.alto = ancho, alto
	out.bgra = rgba.Pix // RGBA sin relleno: Stride == 4·ancho en una imagen nueva
	for i := 0; i+3 < len(out.bgra); i += 4 {
		out.bgra[i], out.bgra[i+2] = out.bgra[i+2], out.bgra[i]
		out.bgra[i+3] = 0xff
	}
	return out, nil
}

// aOriginal pasa una palabra del motor a la imagen original: deshace la reducción y la
// recorta a la imagen. Sin texto o sin área, no vale.
func (m imagenOCR) aOriginal(w palabraCruda) (Palabra, bool) {
	texto := strings.TrimSpace(w.texto)
	if texto == "" || !(w.w > 0) || !(w.h > 0) {
		return Palabra{}, false
	}
	x0 := clamp(math.Floor(float64(w.x)/m.escala), m.anchoOrig)
	y0 := clamp(math.Floor(float64(w.y)/m.escala), m.altoOrig)
	x1 := clamp(math.Ceil(float64(w.x+w.w)/m.escala), m.anchoOrig)
	y1 := clamp(math.Ceil(float64(w.y+w.h)/m.escala), m.altoOrig)
	if x1 <= x0 || y1 <= y0 {
		return Palabra{}, false
	}
	return Palabra{Texto: texto, X: x0, Y: y0, Ancho: x1 - x0, Alto: y1 - y0}, true
}

func clamp(v float64, tope int) int {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > float64(tope) {
		return tope
	}
	return int(v)
}

// ----- Los idiomas -----

// deTesseract son los códigos de tesseract de los idiomas que el OCR de Windows reconoce
// (la lista de `Windows.Media.Ocr`), por la etiqueta BCP-47 base. El chino y el serbio
// dependen de la escritura y se miran aparte (codigoTesseract).
var deTesseract = map[string]string{
	"ar": "ara", "cs": "ces", "da": "dan", "de": "deu", "el": "ell", "en": "eng", "es": "spa",
	"fi": "fin", "fr": "fra", "hu": "hun", "it": "ita", "ja": "jpn", "ko": "kor", "nb": "nor",
	"nl": "nld", "pl": "pol", "pt": "por", "ro": "ron", "ru": "rus", "sk": "slk", "sl": "slv",
	"sv": "swe", "tr": "tur", "bg": "bul", "hr": "hrv", "et": "est", "lt": "lit", "lv": "lav",
	"ca": "cat", "eu": "eus", "gl": "glg", "uk": "ukr", "he": "heb", "hi": "hin", "th": "tha",
	"vi": "vie", "id": "ind", "ms": "msa", "is": "isl", "ga": "gle", "mt": "mlt",
}

// codigoTesseract es el código de tesseract de una etiqueta de Windows (`es-ES` → `spa`,
// `zh-Hans-CN` → `chi_sim`, `sr-Latn-RS` → `srp_latn`).
func codigoTesseract(etiqueta string) (string, bool) {
	partes := strings.Split(strings.ReplaceAll(etiqueta, "_", "-"), "-")
	base := strings.ToLower(partes[0])
	escritura := ""
	for _, p := range partes[1:] {
		if len(p) == 4 {
			escritura = strings.ToLower(p)
		}
	}
	switch base {
	case "zh":
		if escritura == "hant" || contieneSinMayusculas(partes[1:], "tw", "hk", "mo") {
			return "chi_tra", true
		}
		return "chi_sim", true
	case "sr":
		if escritura == "latn" {
			return "srp_latn", true
		}
		return "srp", true
	case "no", "nn":
		return "nor", true
	}
	c, ok := deTesseract[base]
	return c, ok
}

// elegirIdioma es la etiqueta de Windows con que reconocer: la del primer idioma de
// idiomas («spa+eng», códigos de tesseract o etiquetas) que esté instalado; "" si ninguno
// (entonces se usan los idiomas de la persona).
func elegirIdioma(idiomas string, instaladas []string) string {
	for _, pedido := range strings.FieldsFunc(idiomas, func(r rune) bool { return r == '+' || r == ',' || r == ' ' }) {
		for _, e := range instaladas {
			if strings.EqualFold(e, pedido) {
				return e
			}
			if c, ok := codigoTesseract(e); ok && c == pedido {
				return e
			}
		}
	}
	return ""
}

func contiene(lista []string, s string) bool {
	for _, x := range lista {
		if x == s {
			return true
		}
	}
	return false
}

func contieneSinMayusculas(lista []string, cuales ...string) bool {
	for _, x := range lista {
		for _, c := range cuales {
			if strings.EqualFold(x, c) {
				return true
			}
		}
	}
	return false
}
