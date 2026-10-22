// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Imágenes del render de PDF a SVG.
//
// Un PDF guarda las imágenes de dos maneras y cada una pide un camino distinto:
//
//   - COMPRIMIDAS EN UN FORMATO QUE EL NAVEGADOR YA ENTIENDE (DCTDecode es un
//     JPEG tal cual). Se reemiten sin tocar: ni se descomprimen ni se vuelven a
//     comprimir. Es lo más rápido y lo que mejor se ve.
//   - COMO MUESTRAS CRUDAS detrás de un Flate o un LZW. Ahí hay que interpretar
//     el espacio de color, los bits por componente y /Decode, y volver a
//     empaquetar en PNG, que es el formato sin pérdida que el navegador tiene.
//
// La transparencia va aparte: /SMask es un canal alfa en un flujo propio y sin
// aplicarlo un logotipo con fondo recortado sale con un rectángulo blanco
// encima, que es el fallo más visible de un visor a medio hacer.
//
// El tercer camino, que es el caro: **CCITTFaxDecode** (fax G3/G4), el formato de
// casi todo lo escaneado. El navegador no lo trae, así que se decodifica aquí
// —`pdfccitt.go`— a un bit por píxel y sigue el camino de las muestras crudas.
//
// Y el cuarto: **JBIG2**, el formato de los escaneos de «alta compresión», que
// también se decodifica aquí (`pdfjbig2.go`) — solo sus regiones genéricas, que
// son las que llevan la imagen de la página.
//
// LO QUE NO SE DECODIFICA: **JPEG 2000**, y los segmentos de JBIG2 que usan
// diccionarios de símbolos. En su sitio se dibuja un recuadro gris claro, para que
// se vea que ahí hay algo en vez de parecer que la página está incompleta.
package domain

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
)

// maxPixelesImagen acota lo que se decodifica. Un PDF puede declarar una imagen
// de 60000×60000 en pocos bytes; sin tope, decodificarla se come la memoria.
const maxPixelesImagen = 40 << 20

// dibujarImagen pinta un XObject de imagen en el cuadrado unidad del espacio de
// usuario, que es donde la especificación dice que va (§8.9.5.2).
func (l *lienzo) dibujarImagen(e *ejecucion, st *pdfStream) {
	// El cuadrado unidad se recorre de arriba abajo en la imagen y de abajo
	// arriba en el PDF: sin este volteo, toda foto sale del revés.
	m := matriz{1, 0, 0, -1, 0, 1}.por(e.estado.ctm)

	if esMascara(l, st.Dict) {
		l.dibujarEstarcido(e, st, m)
		return
	}

	l.ultimaImagen = nil
	href, ok := l.datosImagen(st, e.recursos)
	if !ok {
		l.escribir(`<rect width="1" height="1" transform="` + m.svg() +
			`" fill="#e5e5e5" stroke="#cbcbcb" stroke-width="0.004"/>` + "\n")
		if l.r != nil {
			// El mismo recuadro gris del SVG: que se vea que ahí había algo.
			l.r.rellenar([][]punto{{
				l.r.punto(m, 0, 0), l.r.punto(m, 1, 0), l.r.punto(m, 1, 1), l.r.punto(m, 0, 1),
			}}, color.NRGBA{R: 0xe5, G: 0xe5, B: 0xe5, A: 255}, e.estado.recorte)
		}
		return
	}
	if l.r != nil {
		if img := l.imagenParaRaster(href); img != nil {
			l.r.imagen(img, m, e.estado.recorte, e.estado.alfaRelleno)
		}
	}
	if l.soloRaster {
		return
	}
	var b strings.Builder
	b.WriteString(`<image x="0" y="0" width="1" height="1" preserveAspectRatio="none"`)
	b.WriteString(` transform="` + m.svg() + `"`)
	if e.estado.alfaRelleno < 1 {
		b.WriteString(` opacity="` + num(e.estado.alfaRelleno) + `"`)
	}
	if e.estado.mezcla != "" {
		b.WriteString(` style="mix-blend-mode:` + e.estado.mezcla + `"`)
	}
	// `image-rendering:pixelated` NO se pone a propósito: un escaneo ampliado se
	// ve mejor interpolado, que es lo que hace cualquier visor de PDF.
	b.WriteString(` xlink:href="` + href + `"/>` + "\n")
	l.escribir(b.String())
}

// muestrasDe devuelve las muestras crudas de un flujo de imagen, decodificando
// el fax si lo lleva. Existe para que las TRES puertas que leen píxeles
// —estarcido, máscara suave y máscara de estarcido— no se olviden ninguna del
// CCITT: antes cada una comprobaba «¿queda filtro? pues me rindo», y una imagen
// escaneada usada como máscara desaparecía sin decir nada.
func (l *lienzo) muestrasDe(st *pdfStream) ([]byte, bool) {
	datos, terminal, err := l.d.datosDeFlujoHasta(st)
	if err != nil {
		return nil, false
	}
	switch terminal {
	case "":
		return datos, true
	case "CCITTFaxDecode", "CCF":
		bits, err := descomprimirCCITT(datos, l.parametrosDelFax(st))
		if err != nil {
			return nil, false
		}
		return bits, true
	case "JBIG2Decode":
		bits, err := l.descomprimirJBIG2DeFlujo(st, datos)
		if err != nil {
			return nil, false
		}
		return bits, true
	case "JPXDecode":
		muestras, _, _, err := DecodificarJPX(datos)
		if err != nil {
			return nil, false
		}
		return muestras, true
	default:
		return nil, false
	}
}

// streamJPXComoCrudo describe la imagen ya decodificada.
//
// Un JPEG 2000 lleva DENTRO su número de componentes y sus bits, y el
// diccionario del PDF puede no decirlo o decir otra cosa —la norma permite
// omitir /ColorSpace justo porque el códec lo sabe—. Manda el fichero.
func streamJPXComoCrudo(st *pdfStream, comps, bits int) *pdfStream {
	d := pdfDict{}
	for k, v := range st.Dict {
		d[k] = v
	}
	d["BitsPerComponent"] = int64(bits)
	delete(d, "Filter")
	delete(d, "DecodeParms")
	switch comps {
	case 1:
		d["ColorSpace"] = pdfName("DeviceGray")
	case 3:
		d["ColorSpace"] = pdfName("DeviceRGB")
	case 4:
		d["ColorSpace"] = pdfName("DeviceCMYK")
	}
	return &pdfStream{Dict: d, Raw: st.Raw}
}

// descomprimirJBIG2DeFlujo saca el tamaño del DICCIONARIO —el formato embebido no
// lo trae— y los segmentos globales de /DecodeParms, que es donde un PDF pone lo
// que comparten varias páginas.
func (l *lienzo) descomprimirJBIG2DeFlujo(st *pdfStream, datos []byte) ([]byte, error) {
	ancho, alto := l.tamanoImagen(st.Dict)
	var globales []byte
	for i, f := range l.d.listaDeNombres(st.Dict["Filter"]) {
		if f != "JBIG2Decode" {
			continue
		}
		if parms := l.d.parmsDeFiltro(st, i); parms != nil {
			if g, ok := l.d.resolver(parms["JBIG2Globals"]).(*pdfStream); ok {
				if b, err := l.d.datosDeFlujo(g); err == nil {
					globales = b
				}
			}
		}
		break
	}
	return descomprimirJBIG2(datos, globales, ancho, alto)
}

func esMascara(l *lienzo, dict pdfDict) bool {
	if v, ok := l.d.resolver(dict["ImageMask"]).(bool); ok && v {
		return true
	}
	v, ok := l.d.resolver(dict["IM"]).(bool)
	return ok && v
}

// dibujarEstarcido pinta una imagen-máscara: no tiene color propio, es una
// plantilla que se rellena con el color de relleno vigente. Así se imprimen los
// sellos de goma y los logotipos monocromos.
func (l *lienzo) dibujarEstarcido(e *ejecucion, st *pdfStream, m matriz) {
	ancho, alto := l.tamanoImagen(st.Dict)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesImagen {
		return
	}
	datos, ok := l.muestrasDe(st)
	if !ok {
		return
	}
	// En una máscara, la muestra 0 PINTA salvo que /Decode diga lo contrario.
	invertir := false
	if dec := l.reales(primeroDe(st.Dict, "Decode", "D")); len(dec) >= 1 && dec[0] == 1 {
		invertir = true
	}

	img := image.NewNRGBA(image.Rect(0, 0, ancho, alto))
	r, g, b := desdeCSS(e.estado.relleno)
	var deg *degradadoRaster
	if e.estado.patronRelleno != "" {
		deg = e.estado.degRelleno
	}
	porFila := (ancho + 7) / 8
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			idx := y*porFila + x/8
			if idx >= len(datos) {
				break
			}
			bit := (datos[idx] >> uint(7-x%8)) & 1
			pinta := bit == 0
			if invertir {
				pinta = !pinta
			}
			if !pinta {
				continue
			}
			if deg != nil {
				ux, uy := (float64(x)+0.5)/float64(ancho), (float64(y)+0.5)/float64(alto)
				img.SetNRGBA(x, y, deg.colorEn(m[0]*ux+m[2]*uy+m[4], m[1]*ux+m[3]*uy+m[5]))
			} else {
				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
			}
		}
	}
	if l.r != nil {
		l.r.imagen(img, m, e.estado.recorte, e.estado.alfaRelleno)
	}
	href, ok := l.comoDataPNG(img)
	if !ok || l.soloRaster {
		return
	}
	l.escribir(`<image x="0" y="0" width="1" height="1" preserveAspectRatio="none" transform="` +
		m.svg() + `" xlink:href="` + href + `"/>` + "\n")
}

func desdeCSS(css string) (byte, byte, byte) {
	if len(css) != 7 || css[0] != '#' {
		return 0, 0, 0
	}
	val := func(i int) byte {
		a, _ := valorHex(css[i])
		b, _ := valorHex(css[i+1])
		return a<<4 | b
	}
	return val(1), val(3), val(5)
}

func (l *lienzo) tamanoImagen(dict pdfDict) (int, int) {
	ancho, _ := comoEntero(l.d.resolver(primeroDe(dict, "Width", "W")))
	alto, _ := comoEntero(l.d.resolver(primeroDe(dict, "Height", "H")))
	return int(ancho), int(alto)
}

// primeroDe devuelve la primera clave presente. Las imágenes en línea usan
// abreviaturas (/W en vez de /Width) y el resto del código no debería enterarse.
func primeroDe(dict pdfDict, claves ...pdfName) pdfObj {
	for _, k := range claves {
		if v, ok := dict[k]; ok {
			return v
		}
	}
	return nil
}

// datosImagen devuelve el `data:` listo para el atributo href.
func (l *lienzo) datosImagen(st *pdfStream, recursos pdfDict) (string, bool) {
	datos, terminal, err := l.d.datosDeFlujoHasta(st)
	if err != nil {
		return "", false
	}
	switch terminal {
	case "DCTDecode", "DCT":
		// Un JPEG se sirve tal cual. Con /SMask hay que recomponerlo, porque un
		// JPEG no tiene canal alfa.
		if alfa := l.canalAlfa(st, recursos); alfa != nil {
			if s, ok := l.recomponerConAlfa(datos, alfa); ok {
				return s, true
			}
		}
		return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(datos), true
	case "CCITTFaxDecode", "CCF":
		// Fax G3/G4: el formato de casi todo lo escaneado. Se decodifica a 1 bit
		// por píxel y sigue el camino normal de muestras crudas.
		bits, err := descomprimirCCITT(datos, l.parametrosDelFax(st))
		if err != nil {
			return "", false
		}
		return l.muestrasComoPNG(l.streamComoBitonal(st), bits, recursos)
	case "JBIG2Decode":
		// El formato de los escaneos de «alta compresión». Se decodifica a un bit
		// por píxel y sigue el camino normal de muestras crudas.
		bits, err := l.descomprimirJBIG2DeFlujo(st, datos)
		if err != nil {
			return "", false
		}
		return l.muestrasComoPNG(l.streamComoBitonal(st), bits, recursos)
	case "JPXDecode":
		// JPEG 2000: el formato de los escaneos de archivo. Se decodifica a
		// muestras crudas y sigue el camino normal.
		muestras, comps, bits, err := DecodificarJPX(datos)
		if err != nil {
			return "", false
		}
		return l.muestrasComoPNG(streamJPXComoCrudo(st, comps, bits), muestras, recursos)
	case "":
		return l.muestrasComoPNG(st, datos, recursos)
	default:
		return "", false
	}
}

// parametrosDelFax localiza el /DecodeParms del filtro CCITT dentro de la cadena
// de filtros. Coger el primero desalinearía los parámetros en cuanto el flujo
// lleve dos filtros, que es corriente («ASCII85 + CCITT»).
func (l *lienzo) parametrosDelFax(st *pdfStream) parametrosCCITT {
	for i, f := range l.d.listaDeNombres(st.Dict["Filter"]) {
		if f == "CCITTFaxDecode" || f == "CCF" {
			return l.leerParametrosCCITT(st, i)
		}
	}
	return l.leerParametrosCCITT(st, 0)
}

// streamComoBitonal describe la imagen ya decodificada: un bit por píxel en gris
// y sin filtros. El flujo original decía «CCITT», y si se dejara puesto el
// decodificador volvería a entrar en el mismo camino.
func (l *lienzo) streamComoBitonal(st *pdfStream) *pdfStream {
	dict := pdfDict{
		"Width":            primeroDe(st.Dict, "Width", "W"),
		"Height":           primeroDe(st.Dict, "Height", "H"),
		"BitsPerComponent": int64(1),
		"ColorSpace":       pdfName("DeviceGray"),
	}
	// /Decode se conserva: un fax invertido a propósito lo declara ahí, y
	// perderlo devolvería la página en negativo.
	if d := primeroDe(st.Dict, "Decode", "D"); d != nil {
		dict["Decode"] = d
	}
	if m, ok := st.Dict["SMask"]; ok {
		dict["SMask"] = m
	}
	return &pdfStream{Dict: dict, Num: st.Num, Gen: st.Gen, sinCifrar: true}
}

// muestrasComoPNG interpreta las muestras crudas y las reempaqueta en PNG.
func (l *lienzo) muestrasComoPNG(st *pdfStream, datos []byte, recursos pdfDict) (string, bool) {
	ancho, alto := l.tamanoImagen(st.Dict)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesImagen {
		return "", false
	}
	bpc := 8
	if v, ok := comoEntero(l.d.resolver(primeroDe(st.Dict, "BitsPerComponent", "BPC"))); ok {
		bpc = int(v)
	}
	ec := l.espacioDeImagen(st.Dict, recursos)
	nc := ec.componentes()
	if ec.clase == "Indexed" {
		nc = 1
	}
	if nc <= 0 || bpc <= 0 || bpc > 16 {
		return "", false
	}
	decode := l.reales(primeroDe(st.Dict, "Decode", "D"))
	alfa := l.canalAlfa(st, recursos)

	img := image.NewNRGBA(image.Rect(0, 0, ancho, alto))
	maxVal := float64(int(1)<<uint(bpc) - 1)
	bitsPorFila := ancho * nc * bpc
	bytesPorFila := (bitsPorFila + 7) / 8
	comp := make([]float64, nc)

	for y := 0; y < alto; y++ {
		base := y * bytesPorFila
		for x := 0; x < ancho; x++ {
			for c := 0; c < nc; c++ {
				bit := (x*nc + c) * bpc
				v := float64(leerBits(datos, base*8+bit, bpc))
				switch {
				case ec.clase == "Indexed":
					// El índice NO se normaliza: es una posición en la paleta.
					if len(decode) >= 2 {
						v = decode[0] + v*(decode[1]-decode[0])/maxVal
					}
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
			a := byte(255)
			if alfa != nil {
				a = alfa.valor(x, y, ancho, alto)
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return l.comoDataPNG(img)
}

// escalaLab devuelve el componente Lab en su rango real: sin esto, un a* de −128
// se leería como 0 y la imagen saldría lavada.
func escalaLab(c int, v float64) float64 {
	if c == 0 {
		return v * 100
	}
	return v*255 - 128
}

func (l *lienzo) espacioDeImagen(dict, recursos pdfDict) *espacioColor {
	cs := primeroDe(dict, "ColorSpace", "CS")
	if cs == nil {
		return espacioGris
	}
	// En una imagen en línea, /CS puede nombrar un recurso del /ColorSpace de la
	// página además de los espacios de dispositivo.
	if n, ok := l.d.resolver(cs).(pdfName); ok {
		switch n {
		case "G", "DeviceGray":
			return espacioGris
		case "RGB", "DeviceRGB":
			return espacioRGB
		case "CMYK", "DeviceCMYK":
			return espacioCMYK
		}
		if recursos != nil {
			if cat, ok := l.d.resolver(recursos["ColorSpace"]).(pdfDict); ok {
				if v, ok := cat[n]; ok {
					return l.espacio(v, 0)
				}
			}
		}
	}
	return l.espacio(cs, 0)
}

// leerBits saca un valor de `n` bits en el desplazamiento dado. Un PDF empaqueta
// 1, 2 y 4 bits por componente para ahorrar, y leerlos como bytes daría basura.
func leerBits(datos []byte, bitOff, n int) uint32 {
	if n == 8 {
		i := bitOff / 8
		if i >= len(datos) {
			return 0
		}
		return uint32(datos[i])
	}
	var v uint32
	for i := 0; i < n; i++ {
		idx := (bitOff + i) / 8
		if idx >= len(datos) {
			return v << uint(n-i)
		}
		v = v<<1 | uint32((datos[idx]>>uint(7-(bitOff+i)%8))&1)
	}
	return v
}

// --- Transparencia -----------------------------------------------------------

// mascaraAlfa es un canal alfa ya decodificado, con su propio tamaño: /SMask
// puede tener una resolución distinta de la imagen a la que acompaña.
type mascaraAlfa struct {
	datos       []byte // un byte por píxel
	ancho, alto int
}

// valor muestrea el alfa reescalando por vecino más próximo, que para una
// máscara —que casi siempre es una silueta— no se distingue de interpolar.
func (m *mascaraAlfa) valor(x, y, ancho, alto int) byte {
	if m.ancho <= 0 || m.alto <= 0 {
		return 255
	}
	sx := x * m.ancho / max(ancho, 1)
	sy := y * m.alto / max(alto, 1)
	i := sy*m.ancho + sx
	if i < 0 || i >= len(m.datos) {
		return 255
	}
	return m.datos[i]
}

// canalAlfa decodifica /SMask (máscara suave, con 256 niveles) o el /Mask de
// estarcido (todo o nada).
func (l *lienzo) canalAlfa(st *pdfStream, recursos pdfDict) *mascaraAlfa {
	if sm, ok := l.d.resolver(st.Dict["SMask"]).(*pdfStream); ok {
		return l.mascaraDeGris(sm)
	}
	if mk, ok := l.d.resolver(st.Dict["Mask"]).(*pdfStream); ok {
		return l.mascaraDeEstarcido(mk)
	}
	// /Mask como array es una clave de color: los píxeles dentro del rango se
	// vuelven transparentes. Se necesitaría comparar por componente antes de
	// convertir a RGB, así que hoy no se aplica.
	_ = recursos
	return nil
}

func (l *lienzo) mascaraDeGris(sm *pdfStream) *mascaraAlfa {
	ancho, alto := l.tamanoImagen(sm.Dict)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesImagen {
		return nil
	}
	datos, ok := l.muestrasDe(sm)
	if !ok {
		return nil
	}
	bpc := 8
	if v, ok := comoEntero(l.d.resolver(sm.Dict["BitsPerComponent"])); ok {
		bpc = int(v)
	}
	dec := l.reales(sm.Dict["Decode"])
	maxVal := float64(int(1)<<uint(bpc) - 1)
	bytesPorFila := (ancho*bpc + 7) / 8
	out := make([]byte, ancho*alto)
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			v := float64(leerBits(datos, y*bytesPorFila*8+x*bpc, bpc)) / maxVal
			if len(dec) >= 2 {
				v = dec[0] + v*(dec[1]-dec[0])
			}
			out[y*ancho+x] = byteDe(v)
		}
	}
	return &mascaraAlfa{datos: out, ancho: ancho, alto: alto}
}

func (l *lienzo) mascaraDeEstarcido(mk *pdfStream) *mascaraAlfa {
	if !esMascara(l, mk.Dict) {
		return nil
	}
	ancho, alto := l.tamanoImagen(mk.Dict)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesImagen {
		return nil
	}
	datos, ok := l.muestrasDe(mk)
	if !ok {
		return nil
	}
	invertir := false
	if dec := l.reales(mk.Dict["Decode"]); len(dec) >= 1 && dec[0] == 1 {
		invertir = true
	}
	porFila := (ancho + 7) / 8
	out := make([]byte, ancho*alto)
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			idx := y*porFila + x/8
			bit := byte(0)
			if idx < len(datos) {
				bit = (datos[idx] >> uint(7-x%8)) & 1
			}
			// En /Mask el 1 TAPA (al revés que en una imagen-máscara).
			oculto := bit == 1
			if invertir {
				oculto = !oculto
			}
			if !oculto {
				out[y*ancho+x] = 255
			}
		}
	}
	return &mascaraAlfa{datos: out, ancho: ancho, alto: alto}
}

// recomponerConAlfa decodifica un JPEG solo para pegarle el canal alfa. Se hace
// únicamente cuando hay /SMask: sin transparencia el JPEG se reemite intacto, y
// recomprimirlo a PNG lo engordaría diez veces sin ganar nada.
func (l *lienzo) recomponerConAlfa(jpegDatos []byte, alfa *mascaraAlfa) (string, bool) {
	src, err := jpeg.Decode(bytes.NewReader(jpegDatos))
	if err != nil {
		return "", false
	}
	b := src.Bounds()
	if b.Dx()*b.Dy() > maxPixelesImagen {
		return "", false
	}
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{
				R: byte(r >> 8 & 0xFF), G: byte(g >> 8 & 0xFF), B: byte(bl >> 8 & 0xFF),
				A: alfa.valor(x, y, b.Dx(), b.Dy()),
			})
		}
	}
	return l.comoDataPNG(out)
}

func comoDataPNG(img image.Image) (string, bool) {
	datos, ok := bytesPNG(img)
	if !ok {
		return "", false
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(datos), true
}

// comoDataPNG del lienzo guarda la imagen ya decodificada y, si nadie va a leer
// el SVG, SE AHORRA CODIFICARLA: codificar un PNG para volver a decodificarlo
// acto seguido y rasterizarlo sería un viaje de ida y vuelta sin nada dentro,
// y en un escaneo es la parte cara de la página.
func (l *lienzo) comoDataPNG(img image.Image) (string, bool) {
	l.ultimaImagen = img
	if l.soloRaster {
		return "raster", true
	}
	return comoDataPNG(img)
}

// imagenParaRaster devuelve el mapa de bits a pintar: el que acaba de construir
// el camino de muestras, o el JPEG que se reemite sin tocar, decodificado.
func (l *lienzo) imagenParaRaster(href string) image.Image {
	if l.ultimaImagen != nil {
		return l.ultimaImagen
	}
	_, b64, ok := strings.Cut(href, ";base64,")
	if !ok {
		return nil
	}
	datos, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil
	}
	return img
}

// bytesPNG codifica la imagen. La compresión por defecto ya deja el PNG pequeño;
// la máxima tarda el triple para ahorrar un 3 %, y aquí quien espera es el
// usuario mirando la página.
func bytesPNG(img image.Image) ([]byte, bool) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// --- Imágenes en línea -------------------------------------------------------

// imagenEnLinea atiende `BI … ID <datos crudos> EI`. Los datos NO son tokens, así
// que hay que saltarlos con el lexer en la mano: dejar que el intérprete los
// parsee produciría operadores inventados a partir de bytes de píxel.
func (e *ejecucion) imagenEnLinea(lx *pdfLexer) {
	dict := pdfDict{}
	for {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			return
		}
		if lx.aceptaPalabra("ID") {
			break
		}
		if lx.byteActual() != '/' {
			// Cualquier cosa que no sea una clave antes de ID es un flujo roto.
			lx.pos++
			continue
		}
		clave := lx.nombre()
		antes := lx.pos
		valor, err := e.l.d.parsearObjeto(lx)
		if err != nil || lx.pos == antes {
			lx.pos = antes + 1
			continue
		}
		dict[clave] = valor
	}
	// Exactamente UN byte de separación tras ID (§8.9.7).
	if lx.pos < len(lx.buf) && esEspacioPDF(lx.buf[lx.pos]) {
		lx.pos++
	}
	inicio := lx.pos
	fin := buscarFinDeImagen(lx.buf, inicio)
	datos := lx.buf[inicio:fin]
	lx.pos = fin
	lx.saltarEspacios()
	lx.aceptaPalabra("EI")

	st := &pdfStream{Dict: dict, Raw: datos}
	// Los filtros abreviados ya los conoce `datosDeFlujoHasta`; la clave sí hay
	// que normalizarla porque en línea se llama /F.
	if _, ok := dict["Filter"]; !ok {
		if f, ok := dict["F"]; ok {
			dict["Filter"] = f
		}
	}
	if _, ok := dict["DecodeParms"]; !ok {
		if p, ok := dict["DP"]; ok {
			dict["DecodeParms"] = p
		}
	}
	e.l.dibujarImagen(e, st)
}

// buscarFinDeImagen localiza el `EI` que cierra los datos. El delimitador puede
// aparecer por casualidad dentro de los píxeles, así que se exige que vaya
// rodeado de espacios y que lo que sigue parezca un operador y no más bytes
// binarios: es la heurística que usan todos los lectores, porque la
// especificación no da longitud.
func buscarFinDeImagen(buf []byte, desde int) int {
	for i := desde; i+1 < len(buf); i++ {
		if buf[i] != 'E' || buf[i+1] != 'I' {
			continue
		}
		if i > desde && !esEspacioPDF(buf[i-1]) {
			continue
		}
		if i+2 < len(buf) && !esEspacioPDF(buf[i+2]) && !esDelimitadorPDF(buf[i+2]) {
			continue
		}
		fin := i
		if fin > desde {
			fin-- // el espacio de delante pertenece al delimitador, no a los datos
		}
		return fin
	}
	return len(buf)
}
