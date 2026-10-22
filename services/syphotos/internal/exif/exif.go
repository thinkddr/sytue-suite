// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package exif lee los metadatos que hacen falta para la galería —orientación,
// fecha de captura, cámara y objetivo— sin decodificar un solo píxel.
//
// POR QUÉ ESCRITO A MANO. Es un formato de 1995 del que aquí solo se necesitan
// SIETE etiquetas; una biblioteca completa traería IPTC, XMP, MakerNotes de
// doce fabricantes y su superficie de ataque, para leer un entero y una fecha.
// Misma decisión que ya se tomó con PDF y con SMTP (CONVENTIONS §12).
//
// ESTOS BYTES VIENEN DE FUERA. Los sube quien quiera y el parser corre en el
// servidor, así que la regla es una: NUNCA indexar sin comprobar, NUNCA seguir
// un desplazamiento sin validarlo contra el tamaño real, y no fiarse jamás de
// una longitud declarada dentro del propio fichero. Un EXIF hostil puede
// declarar 4.000 millones de entradas o apuntar a un desplazamiento que se sale;
// aquí eso devuelve «no hay metadatos», no un pánico ni una reserva de memoria.
//
// LO QUE NO HACE: no decodifica la imagen. La ingesta solo necesita la cabecera,
// y por eso `BytesDeCabecera` alcanza — con eso se leen 64 KiB del original en
// vez de los 25 MB que puede pesar una foto.
package exif

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"time"
)

// ErrSinExif indica que el fichero no trae metadatos legibles. NO es un fallo:
// hay millones de imágenes sin EXIF —capturas de pantalla, imágenes editadas,
// PNG— y tratarlo como error obligaría a quien llama a distinguir dos nadas.
var ErrSinExif = errors.New("exif: el fichero no trae metadatos")

// BytesDeCabecera es cuánto hay que leer del original para encontrar el EXIF.
//
// El APP1 va justo detrás del SOI, antes de los datos de imagen, y el estándar
// limita un segmento JPEG a 64 KiB porque su longitud es de dos bytes. Con esto
// se cubre el segmento entero aunque venga precedido de un APP0 (JFIF), que es
// el caso normal.
const BytesDeCabecera = 128 * 1024

// Metadatos es lo que la galería necesita saber de una foto.
type Metadatos struct {
	// Orientacion es la etiqueta 0x0112 (1..8). Cero cuando no viene, que el
	// dominio trata como «sin rotación».
	Orientacion uint16
	// TomadaEn es DateTimeOriginal, con el desfase horario aplicado si el
	// fichero lo trae (OffsetTimeOriginal). Nulo cuando no hay fecha.
	TomadaEn *time.Time
	Camara   string
	Objetivo string
	// Lat y Lon son la posición en grados decimales, con el signo ya aplicado
	// (sur y oeste negativos). Nulas cuando el fichero no la trae o la trae
	// rota, que es lo corriente: muchas cámaras no llevan GPS y los móviles lo
	// quitan al compartir.
	Lat *float64
	Lon *float64
}

// Etiquetas que se leen. Las demás se ignoran a propósito.
// Etiquetas del IFD de GPS. Numeración PROPIA: en ese IFD el 0x0001 es la
// referencia de latitud, no lo que valga 0x0001 en el IFD0.
const (
	tagGPSLatRef = 0x0001
	tagGPSLat    = 0x0002
	tagGPSLonRef = 0x0003
	tagGPSLon    = 0x0004
)

const (
	tagMake         = 0x010F
	tagModel        = 0x0110
	tagOrientation  = 0x0112
	tagExifIFD      = 0x8769
	tagDateTimeOrig = 0x9003
	tagOffsetTime   = 0x9011
	tagLensModel    = 0xA434
	tagGPSIFD       = 0x8825
)

// Topes. Todos existen por lo mismo: la cifra la escribe el fichero.
const (
	maxEntradas = 1024 // entradas por IFD; una cámara real usa decenas
	maxCadena   = 256  // caracteres de un ASCII; un modelo de cámara no llega
)

// recoger vuelca en `m` las etiquetas que interesan de un IFD.
//
// FUERA DE `Leer` PORQUE ES OTRA COSA: `Leer` decide qué bloques del fichero se
// siguen y con cuánto cuidado —los punteros de un fichero hostil—, y esto solo
// traduce etiquetas a campos. Juntas pasaban del tope de complejidad, y la
// mezcla era lo que hacía difícil ver dónde estaba la protección.
//
// La fecha y su desfase salen por puntero y no en el struct: pueden llegar en
// cualquier orden dentro del mismo IFD, así que se recogen crudos y se combinan
// al final, cuando ya están los dos.
func (m *Metadatos) recoger(campos []campo, fechaCruda, desfaseCrudo *string) {
	for _, c := range campos {
		switch c.tag {
		case tagOrientation:
			if v, ok := c.entero(); ok && v <= 8 {
				m.Orientacion = uint16(v)
			}
		case tagMake, tagModel:
			m.Camara = juntar(m.Camara, c.texto())
		case tagLensModel:
			m.Objetivo = c.texto()
		case tagDateTimeOrig:
			*fechaCruda = c.texto()
		case tagOffsetTime:
			*desfaseCrudo = c.texto()
		}
	}
}

// Leer extrae los metadatos de una imagen. `datos` puede ser el fichero entero o
// solo su cabecera (ver BytesDeCabecera).
//
// Devuelve ErrSinExif cuando no hay nada que leer. Cualquier otra malformación
// se trata igual: se devuelve lo que se haya podido leer hasta ahí, sin error.
// Media foto con fecha es mejor que ninguna, y un fichero raro no puede impedir
// que se suba.
func Leer(datos []byte) (Metadatos, error) {
	tiff, orden, ok := localizarTIFF(datos)
	if !ok {
		return Metadatos{}, ErrSinExif
	}
	var m Metadatos
	// El desfase horario vive en el IFD de Exif y la fecha también, pero pueden
	// llegar en cualquier orden dentro del mismo IFD: se recogen crudos y se
	// combinan al final.
	var fechaCruda, desfaseCrudo string

	visitar := func(campos []campo) { m.recoger(campos, &fechaCruda, &desfaseCrudo) }

	ifd0 := leerIFD(tiff, orden, 0)
	visitar(ifd0)

	// El puntero al IFD de Exif se sigue UNA vez y solo hacia delante. Un
	// fichero hostil puede hacer que apunte a sí mismo, y seguirlo en bucle es
	// una denegación de servicio de dos líneas.
	for _, c := range ifd0 {
		if c.tag == tagExifIFD {
			if off, ok := c.entero(); ok && off > 0 {
				sub := leerIFD(tiff, orden, int(off))
				visitar(sub)
			}
			break
		}
	}

	// El IFD de GPS, con el mismo cuidado que el de Exif: se sigue UNA vez y
	// solo hacia delante, que un fichero hostil puede apuntarlo a sí mismo.
	for _, c := range ifd0 {
		if c.tag == tagGPSIFD {
			if off, ok := c.entero(); ok && off > 0 {
				sub := leerIFD(tiff, orden, int(off))
				m.Lat, m.Lon = posicion(sub)
			}
			break
		}
	}

	if t, ok := fecha(fechaCruda, desfaseCrudo); ok {
		m.TomadaEn = &t
	}
	if m.Orientacion == 0 && m.TomadaEn == nil && m.Camara == "" && m.Lat == nil {
		return Metadatos{}, ErrSinExif
	}
	return m, nil
}

// posicion saca latitud y longitud del IFD de GPS, o dos nulos.
//
// LAS DOS O NINGUNA: media coordenada no sitúa nada, y guardar solo la latitud
// dejaría una foto colocada en el meridiano de Greenwich sin que nadie lo
// pidiera.
func posicion(campos []campo) (*float64, *float64) {
	var lat, lon []float64
	var refLat, refLon string
	for _, c := range campos {
		switch c.tag {
		case tagGPSLat:
			lat = c.racionales(3)
		case tagGPSLon:
			lon = c.racionales(3)
		case tagGPSLatRef:
			refLat = c.texto()
		case tagGPSLonRef:
			refLon = c.texto()
		}
	}
	gLat, okLat := grados(lat, refLat, 90)
	gLon, okLon := grados(lon, refLon, 180)
	if !okLat || !okLon {
		return nil, nil
	}
	// (0, 0) SE DESCARTA. Es un punto real en el Atlántico y es también lo que
	// escriben las cámaras cuando el GPS no llegó a fijar posición. Perder la
	// foto de quien esté justo ahí sale más barato que colocar en el golfo de
	// Guinea todas las que no tienen posición.
	if gLat == 0 && gLon == 0 {
		return nil, nil
	}
	return &gLat, &gLon
}

// grados convierte grados/minutos/segundos a grados decimales y le aplica el
// signo de la referencia. Sin referencia no hay coordenada: «41 grados» está en
// Barcelona o en Nueva Zelanda según diga N o S.
func grados(dms []float64, ref string, tope float64) (float64, bool) {
	if len(dms) != 3 {
		return 0, false
	}
	g := dms[0] + dms[1]/60 + dms[2]/3600
	if math.IsNaN(g) || math.IsInf(g, 0) || g > tope {
		return 0, false
	}
	switch strings.ToUpper(strings.TrimSpace(ref)) {
	case "N", "E":
		return g, true
	case "S", "W":
		return -g, true
	}
	return 0, false
}

// localizarTIFF recorre los segmentos JPEG hasta el APP1 con la firma «Exif».
// Devuelve el bloque TIFF (los desplazamientos del EXIF son relativos a SU
// inicio, no al del fichero) y el orden de bytes.
func localizarTIFF(b []byte) ([]byte, binary.ByteOrder, bool) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 { // SOI
		return nil, nil, false
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return nil, nil, false // fuera de sincronía: no seguimos adivinando
		}
		marcador := b[i+1]
		// SOS (0xDA) empieza los datos comprimidos: a partir de ahí no hay más
		// segmentos que recorrer, y seguir sería interpretar píxeles como
		// cabeceras.
		if marcador == 0xDA || marcador == 0xD9 {
			return nil, nil, false
		}
		// La longitud INCLUYE sus propios dos bytes; menor que 2 es basura y
		// restarle 2 daría un tamaño negativo.
		long := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if long < 2 || i+2+long > len(b) {
			return nil, nil, false
		}
		cuerpo := b[i+4 : i+2+long]
		if marcador == 0xE1 && len(cuerpo) >= 6 && string(cuerpo[:6]) == "Exif\x00\x00" {
			return cabeceraTIFF(cuerpo[6:])
		}
		i += 2 + long
	}
	return nil, nil, false
}

// cabeceraTIFF valida el encabezado y devuelve el bloque con su orden de bytes.
func cabeceraTIFF(t []byte) ([]byte, binary.ByteOrder, bool) {
	if len(t) < 8 {
		return nil, nil, false
	}
	var orden binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		orden = binary.LittleEndian
	case "MM":
		orden = binary.BigEndian
	default:
		return nil, nil, false
	}
	if orden.Uint16(t[2:4]) != 42 { // la firma del formato
		return nil, nil, false
	}
	return t, orden, true
}

// campo es una entrada de IFD ya acotada al bloque.
type campo struct {
	tag   uint16
	tipo  uint16
	n     uint32
	datos []byte // ya recortado y validado
	orden binary.ByteOrder
}

// entero devuelve el primer valor como entero, para SHORT y LONG.
func (c campo) entero() (uint32, bool) {
	switch c.tipo {
	case 3: // SHORT
		if len(c.datos) < 2 {
			return 0, false
		}
		return uint32(c.orden.Uint16(c.datos[:2])), true
	case 4, 9: // LONG, SLONG
		if len(c.datos) < 4 {
			return 0, false
		}
		return c.orden.Uint32(c.datos[:4]), true
	}
	return 0, false
}

// racionales devuelve los primeros `cuantos` RATIONAL de un campo.
//
// El EXIF guarda una coordenada como TRES fracciones —grados, minutos y
// segundos—, y cada fracción es un par de enteros de 32 bits. Un denominador
// cero no es «cero»: es un fichero que miente, y dividir por él daría ±Inf que
// luego se guardaría como posición.
func (c campo) racionales(cuantos int) []float64 {
	if c.tipo != 5 || int(c.n) < cuantos || len(c.datos) < cuantos*8 {
		return nil
	}
	out := make([]float64, 0, cuantos)
	for i := range cuantos {
		den := c.orden.Uint32(c.datos[i*8+4 : i*8+8])
		if den == 0 {
			return nil
		}
		out = append(out, float64(c.orden.Uint32(c.datos[i*8:i*8+4]))/float64(den))
	}
	return out
}

// texto devuelve un ASCII limpio: sin el NUL final y sin espacios de relleno,
// que los fabricantes ponen a discreción.
func (c campo) texto() string {
	if c.tipo != 2 {
		return ""
	}
	s := string(c.datos)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > maxCadena {
		s = s[:maxCadena]
	}
	return s
}

// leerIFD lee las entradas de un IFD situado en `off` dentro del bloque TIFF.
//
// TODO desplazamiento y TODA longitud se comprueban contra `len(t)`. Es
// repetitivo a propósito: cada `if` que falta aquí es un pánico servido por un
// fichero de fuera.
func leerIFD(t []byte, orden binary.ByteOrder, off int) []campo {
	// El IFD0 vive donde diga la cabecera; los demás donde diga su puntero.
	if off == 0 {
		if len(t) < 8 {
			return nil
		}
		off = int(orden.Uint32(t[4:8]))
	}
	if off < 8 || off+2 > len(t) {
		return nil
	}
	n := int(orden.Uint16(t[off : off+2]))
	if n > maxEntradas {
		n = maxEntradas
	}
	// `fin` era una variable porque después se leía el puntero al IFD siguiente
	// en `t[fin:fin+4]`; ese puntero apuntaba a la miniatura embebida y no lo
	// usaba nadie, así que aquí solo queda la comprobación de que el IFD cabe.
	if off+2+n*12 > len(t) {
		// Se lee lo que quepa en vez de descartar el IFD entero: un fichero
		// truncado suele conservar las primeras etiquetas, que son las que
		// interesan.
		n = (len(t) - off - 2) / 12
		if n <= 0 {
			return nil
		}
	}

	campos := make([]campo, 0, n)
	for k := range n {
		e := t[off+2+k*12 : off+2+(k+1)*12]
		c := campo{
			tag:   orden.Uint16(e[0:2]),
			tipo:  orden.Uint16(e[2:4]),
			n:     orden.Uint32(e[4:8]),
			orden: orden,
		}
		ancho := anchoDeTipo(c.tipo)
		if ancho == 0 {
			continue // tipo desconocido: no se puede saber cuánto ocupa
		}
		// El producto se hace en 64 bits: `n` lo escribe el fichero y puede ser
		// 4.000 millones, que en 32 bits desbordaría y daría un tamaño pequeño
		// y creíble — que es justo como se consigue una lectura fuera de rango.
		total := int64(c.n) * int64(ancho)
		if total <= 0 || total > int64(len(t)) {
			continue
		}
		if total <= 4 {
			c.datos = e[8 : 8+total]
		} else {
			d := int64(orden.Uint32(e[8:12]))
			if d < 0 || d+total > int64(len(t)) {
				continue
			}
			c.datos = t[d : d+total]
		}
		campos = append(campos, c)
	}
	return campos
}

// anchoDeTipo son los bytes por valor de cada tipo TIFF. Cero = desconocido.
func anchoDeTipo(tipo uint16) int {
	switch tipo {
	case 1, 2, 6, 7: // BYTE, ASCII, SBYTE, UNDEFINED
		return 1
	case 3, 8: // SHORT, SSHORT
		return 2
	case 4, 9, 11: // LONG, SLONG, FLOAT
		return 4
	case 5, 10, 12: // RATIONAL, SRATIONAL, DOUBLE
		return 8
	}
	return 0
}

// fecha compone DateTimeOriginal con su desfase.
//
// EL EXIF NO LLEVA ZONA HORARIA salvo que traiga OffsetTimeOriginal, que es de
// 2016 y muchas cámaras siguen sin escribir. Sin él, la hora se interpreta como
// UTC: es lo único honesto —no sabemos dónde se tomó— y además es estable, que
// importa porque la línea de tiempo ordena por este campo. Elegir la zona del
// servidor movería todas las fotos al cambiar de despliegue.
func fecha(crudo, desfase string) (time.Time, bool) {
	crudo = strings.TrimSpace(crudo)
	if crudo == "" {
		return time.Time{}, false
	}
	const formato = "2006:01:02 15:04:05" // el separador es «:», no «-»
	t, err := time.Parse(formato, crudo)
	if err != nil {
		return time.Time{}, false
	}
	// Una cámara sin reloj puesto escribe ceros, y esa fecha mandaría la foto al
	// principio de la línea de tiempo delante de todo.
	if t.Year() < 1900 {
		return time.Time{}, false
	}
	if d := strings.TrimSpace(desfase); d != "" {
		if conZona, err := time.Parse(formato+" -07:00", crudo+" "+d); err == nil {
			return conZona.UTC(), true
		}
	}
	return t, true
}

// juntar añade una parte al fabricante sin repetirla: muchas cámaras escriben
// «NIKON CORPORATION» en Make y «NIKON D750» en Model, y concatenar a ciegas
// deja «NIKON CORPORATION NIKON D750».
func juntar(ya, parte string) string {
	parte = strings.TrimSpace(parte)
	if parte == "" {
		return ya
	}
	if ya == "" {
		return parte
	}
	if strings.HasPrefix(strings.ToUpper(parte), strings.ToUpper(primeraPalabra(ya))) {
		return parte
	}
	return ya + " " + parte
}

func primeraPalabra(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}
