// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// JPEG 2000 (ISO/IEC 15444-1), el filtro /JPXDecode.
//
// Era el único formato de imagen que este motor no sabía pintar: dejaba un
// recuadro gris con su aviso. En PDF aparece sobre todo en escaneos de archivo
// —expedientes, planos, digitalizaciones de patrimonio—, que es justo lo que
// alguien abre esperando VERLO.
//
// Se escribió porque la pieza más cara ya estaba: el decodificador aritmético MQ
// de `pdfjbig2.go` es EL MISMO que usa esta norma. Sin eso, esto no compensaba.
//
// El camino completo, de fuera adentro:
//
//  1. contenedor JP2 (cajas) o codestream J2K a pelo — aquí
//  2. marcadores de cabecera: SIZ, COD, QCD — aquí
//  3. Tier-2: cabeceras de paquete, con tag trees — aquí
//  4. Tier-1: EBCOT, tres pasadas por plano de bits sobre el MQ — `pdfjpxt1.go`
//  5. wavelet inversa 5/3 y 9/7, y la transformada de color — `pdfjpxidwt.go`
package domain

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Marcadores de la norma que se usan aquí (tabla A.2).
const (
	jpxSOC = 0xFF4F // arranque del codestream
	jpxSIZ = 0xFF51 // tamaño de imagen y de las componentes
	jpxCOD = 0xFF52 // estilo de codificación por defecto
	jpxCOC = 0xFF53 // estilo de codificación de UNA componente
	jpxQCD = 0xFF5C // cuantificación por defecto
	jpxQCC = 0xFF5D // cuantificación de UNA componente
	jpxSOT = 0xFF90 // arranque de tile
	jpxSOD = 0xFF93 // arranque de los datos del tile
	jpxEOC = 0xFFD9 // fin del codestream
)

// Topes. Un JPEG 2000 declara su tamaño en la cabecera y de ahí saldría una
// reserva de memoria enorme antes de leer un solo píxel.
const (
	maxJPXLado        = 20000
	maxJPXPixeles     = 50 << 20
	maxJPXComponentes = 4
	maxJPXNiveles     = 32
	maxJPXCapas       = 1000
)

// componenteJPX describe una componente de color de la imagen.
type componenteJPX struct {
	bits   int  // profundidad en bits (1-38 según la norma; aquí hasta 16)
	signo  bool // muestras con signo
	dx, dy int  // submuestreo horizontal y vertical
}

// codificacionJPX es lo que dice un COD/COC: cómo está partida y transformada la
// imagen. Vale para toda la imagen o para una componente.
type codificacionJPX struct {
	niveles    int  // niveles de descomposición wavelet
	cbAncho    int  // anchura del code-block (potencia de 2)
	cbAlto     int  // altura del code-block
	transf53   bool // true = 5/3 reversible (sin pérdida); false = 9/7
	capas      int
	progresion int
	// mct dice si hay transformada de componentes (RCT con 5/3, ICT con 9/7).
	mct bool
	// selectivo son los estilos de code-block del byte SPcod. Solo se admite 0:
	// los demás (terminación por pasada, bypass, RESET…) cambian cómo se lee el
	// MQ, y adivinarlos daría una imagen de ruido en vez de un error.
	selectivo byte
}

// imagenJPX es lo que sale de leer las cabeceras: el mapa del terreno.
type imagenJPX struct {
	ancho, alto    int // rejilla de referencia
	x0, y0         int // origen de la imagen dentro de la rejilla
	tileAncho      int
	tileAlto       int
	tileX0, tileY0 int
	comps          []componenteJPX
	cod            codificacionJPX
	// codPorComp guarda el COC de cada componente, si lo trae.
	codPorComp map[int]codificacionJPX
	// guarda y exponentes/mantisas de la cuantificación (QCD/QCC).
	estiloQ byte
	guarda  int
	pasos   []pasoCuantificacion
	// tiles son los trozos del codestream, en orden de aparición.
	tiles map[int][]byte
}

// pasoCuantificacion es el par exponente/mantisa de una subbanda.
type pasoCuantificacion struct {
	exp, mant int
}

// LeerCabecerasJPX interpreta el codestream hasta saber qué hay dentro.
//
// Se separa de la decodificación a propósito: con esto ya se puede DECIR qué
// trae un fichero que no se sepa pintar, en vez de dejar el hueco mudo.
func LeerCabecerasJPX(datos []byte) (*imagenJPX, error) {
	cs, err := codestreamDeJP2(datos)
	if err != nil {
		return nil, err
	}
	if len(cs) < 4 || binary.BigEndian.Uint16(cs) != jpxSOC {
		return nil, fmt.Errorf("jpx: no empieza por SOC")
	}

	img := &imagenJPX{codPorComp: map[int]codificacionJPX{}, tiles: map[int][]byte{}}
	i := 2
	vistoSIZ := false
	for i+4 <= len(cs) {
		marca := binary.BigEndian.Uint16(cs[i:])
		if marca == jpxEOC {
			break
		}
		if marca>>8 != 0xFF {
			return nil, fmt.Errorf("jpx: se esperaba un marcador en el byte %d", i)
		}
		largo := int(binary.BigEndian.Uint16(cs[i+2:]))
		if largo < 2 || i+2+largo > len(cs) {
			return nil, fmt.Errorf("jpx: marcador %04X con longitud imposible", marca)
		}
		cuerpo := cs[i+4 : i+2+largo]

		// SOT va aparte del resto: a partir de él vienen los datos comprimidos del
		// tile, que NO son una lista de marcadores, así que hay que saltar por la
		// longitud declarada en vez de avanzar al siguiente marcador.
		if marca == jpxSOT {
			_, siguiente, err := img.leerTile(cs, i, cuerpo)
			if err != nil {
				return nil, err
			}
			i = siguiente
			continue
		}

		vistoSIZ, err = img.aplicarCabecera(marca, cuerpo, vistoSIZ)
		if err != nil {
			return nil, err
		}
		i += 2 + largo
	}
	if !vistoSIZ {
		return nil, fmt.Errorf("jpx: no trae SIZ, así que no dice ni cuánto mide")
	}
	if len(img.tiles) == 0 {
		return nil, fmt.Errorf("jpx: no trae datos de imagen")
	}
	return img, nil
}

// aplicarCabecera interpreta UN marcador de cabecera y devuelve si ya se ha visto
// el SIZ. Sale del bucle de `LeerCabecerasJPX` porque allí sumaba complejidad
// cognitiva por encima del techo del linter (36 sobre 30), y lo que costaba seguir
// era el hilo del recorrido —dónde se avanza y dónde se salta—, no qué hace cada
// marcador por su cuenta.
//
// El orden importa y por eso se comprueba: COD y COC describen componentes que solo
// existen si SIZ ya dijo cuántos hay.
func (img *imagenJPX) aplicarCabecera(marca uint16, cuerpo []byte, vistoSIZ bool) (bool, error) {
	switch marca {
	case jpxSIZ:
		if err := img.leerSIZ(cuerpo); err != nil {
			return vistoSIZ, err
		}
		return true, nil
	case jpxCOD:
		if !vistoSIZ {
			return vistoSIZ, fmt.Errorf("jpx: COD antes que SIZ")
		}
		c, err := leerCOD(cuerpo)
		if err != nil {
			return vistoSIZ, err
		}
		img.cod = c
	case jpxCOC:
		if !vistoSIZ {
			return vistoSIZ, fmt.Errorf("jpx: COC antes que SIZ")
		}
		n, c, err := leerCOC(cuerpo, len(img.comps), img.cod)
		if err != nil {
			return vistoSIZ, err
		}
		img.codPorComp[n] = c
	case jpxQCD:
		estilo, guarda, pasos, err := leerQCD(cuerpo)
		if err != nil {
			return vistoSIZ, err
		}
		img.estiloQ, img.guarda, img.pasos = estilo, guarda, pasos
	}
	return vistoSIZ, nil
}

// leerSIZ saca el tamaño de la imagen, de los tiles y de cada componente.
func (img *imagenJPX) leerSIZ(b []byte) error {
	if len(b) < 36 {
		return fmt.Errorf("jpx: SIZ demasiado corto")
	}
	u32 := func(o int) int { return int(binary.BigEndian.Uint32(b[o:])) }
	xsiz, ysiz := u32(2), u32(6)
	img.x0, img.y0 = u32(10), u32(14)
	img.tileAncho, img.tileAlto = u32(18), u32(22)
	img.tileX0, img.tileY0 = u32(26), u32(30)
	n := int(binary.BigEndian.Uint16(b[34:]))

	img.ancho, img.alto = xsiz-img.x0, ysiz-img.y0
	if img.ancho <= 0 || img.alto <= 0 || img.ancho > maxJPXLado || img.alto > maxJPXLado {
		return fmt.Errorf("jpx: tamaño imposible (%d×%d)", img.ancho, img.alto)
	}
	if img.ancho*img.alto > maxJPXPixeles {
		return fmt.Errorf("jpx: la imagen pasa de %d píxeles", maxJPXPixeles)
	}
	if img.tileAncho <= 0 || img.tileAlto <= 0 {
		return fmt.Errorf("jpx: tiles de tamaño cero")
	}
	if n <= 0 || n > maxJPXComponentes {
		return fmt.Errorf("jpx: %d componentes (se admiten hasta %d)", n, maxJPXComponentes)
	}
	if len(b) < 36+3*n {
		return fmt.Errorf("jpx: SIZ dice %d componentes y no caben", n)
	}
	img.comps = make([]componenteJPX, n)
	for k := 0; k < n; k++ {
		s := b[36+3*k]
		img.comps[k] = componenteJPX{
			bits:  int(s&0x7F) + 1,
			signo: s&0x80 != 0,
			dx:    int(b[37+3*k]),
			dy:    int(b[38+3*k]),
		}
		if img.comps[k].dx <= 0 || img.comps[k].dy <= 0 {
			return fmt.Errorf("jpx: componente %d con submuestreo cero", k)
		}
		if img.comps[k].bits > 16 {
			return fmt.Errorf("jpx: componente %d de %d bits (se admiten hasta 16)",
				k, img.comps[k].bits)
		}
	}
	return nil
}

// leerCOD interpreta el estilo de codificación por defecto.
func leerCOD(b []byte) (codificacionJPX, error) {
	if len(b) < 10 {
		return codificacionJPX{}, fmt.Errorf("jpx: COD demasiado corto")
	}
	var c codificacionJPX
	scod := b[0]
	c.progresion = int(b[1])
	c.capas = int(binary.BigEndian.Uint16(b[2:]))
	c.mct = b[4] != 0
	if err := leerSPcod(b[5:], &c); err != nil {
		return codificacionJPX{}, err
	}
	// Bit 0 de Scod: precintos declarados uno a uno. Sin leerlos, los paquetes
	// se cuentan mal y lo que sale es ruido; mejor decirlo.
	if scod&0x01 != 0 {
		return codificacionJPX{}, fmt.Errorf("jpx: precintos a medida, que este decodificador no lee")
	}
	if c.capas <= 0 || c.capas > maxJPXCapas {
		return codificacionJPX{}, fmt.Errorf("jpx: %d capas de calidad", c.capas)
	}
	return c, nil
}

// leerCOC interpreta el estilo de UNA componente, partiendo del de por defecto.
func leerCOC(b []byte, ncomp int, base codificacionJPX) (int, codificacionJPX, error) {
	if len(b) < 2 {
		return 0, codificacionJPX{}, fmt.Errorf("jpx: COC demasiado corto")
	}
	var n, off int
	if ncomp < 257 {
		n, off = int(b[0]), 1
	} else {
		n, off = int(binary.BigEndian.Uint16(b)), 2
	}
	if n >= ncomp {
		return 0, codificacionJPX{}, fmt.Errorf("jpx: COC de la componente %d, que no existe", n)
	}
	c := base // capas, progresión y MCT los fija el COD; el COC no los toca
	if len(b) < off+6 {
		return 0, codificacionJPX{}, fmt.Errorf("jpx: COC sin SPcoc")
	}
	if err := leerSPcod(b[off+1:], &c); err != nil {
		return 0, codificacionJPX{}, err
	}
	return n, c, nil
}

// leerSPcod lee la parte común de COD y COC: niveles, code-block y transformada.
func leerSPcod(b []byte, c *codificacionJPX) error {
	if len(b) < 5 {
		return fmt.Errorf("jpx: SPcod demasiado corto")
	}
	c.niveles = int(b[0])
	if c.niveles > maxJPXNiveles {
		return fmt.Errorf("jpx: %d niveles de descomposición", c.niveles)
	}
	// El tamaño va como exponente menos 2: un 4 significa 2^6 = 64.
	xcb, ycb := int(b[1]&0x0F)+2, int(b[2]&0x0F)+2
	if xcb < 2 || xcb > 10 || ycb < 2 || ycb > 10 || xcb+ycb > 12 {
		return fmt.Errorf("jpx: code-block de 2^%d×2^%d, fuera de la norma", xcb, ycb)
	}
	c.cbAncho, c.cbAlto = 1<<xcb, 1<<ycb
	c.selectivo = b[3]
	if c.selectivo != 0 {
		// Los estilos opcionales cambian cómo se lee el MQ (bypass, terminación
		// por pasada, RESET). Aplicar el camino normal daría ruido con pinta de
		// imagen, que es peor que no pintar.
		return fmt.Errorf("jpx: estilo de code-block 0x%02X, que este decodificador no lee", c.selectivo)
	}
	c.transf53 = b[4] == 1
	return nil
}

// leerQCD interpreta la cuantificación por defecto.
func leerQCD(b []byte) (estilo byte, guarda int, pasos []pasoCuantificacion, err error) {
	if len(b) < 1 {
		return 0, 0, nil, fmt.Errorf("jpx: QCD vacío")
	}
	sq := b[0]
	estilo, guarda = sq&0x1F, int(sq>>5)
	cuerpo := b[1:]
	switch estilo {
	case 0: // sin cuantificar: un exponente por subbanda, en 5 bits
		pasos = make([]pasoCuantificacion, 0, len(cuerpo))
		for _, v := range cuerpo {
			pasos = append(pasos, pasoCuantificacion{exp: int(v >> 3)})
		}
	case 1, 2: // escalar derivado (1) o expuesto (2): 16 bits por subbanda
		if len(cuerpo)%2 != 0 {
			return 0, 0, nil, fmt.Errorf("jpx: QCD con longitud impar")
		}
		pasos = make([]pasoCuantificacion, 0, len(cuerpo)/2)
		for k := 0; k+1 < len(cuerpo); k += 2 {
			v := binary.BigEndian.Uint16(cuerpo[k:])
			pasos = append(pasos, pasoCuantificacion{exp: int(v >> 11), mant: int(v & 0x7FF)})
		}
	default:
		return 0, 0, nil, fmt.Errorf("jpx: estilo de cuantificación %d desconocido", estilo)
	}
	if len(pasos) == 0 {
		return 0, 0, nil, fmt.Errorf("jpx: QCD sin ningún paso")
	}
	return estilo, guarda, pasos, nil
}

// leerTile guarda los bytes comprimidos de un tile-part y devuelve dónde sigue
// el codestream.
//
// Un tile puede venir partido en varios tile-parts, y hay que CONCATENARLOS en
// orden: quedarse con el primero deja la imagen a medias sin ningún error.
func (img *imagenJPX) leerTile(cs []byte, pos int, cuerpo []byte) (int, int, error) {
	if len(cuerpo) < 8 {
		return 0, 0, fmt.Errorf("jpx: SOT demasiado corto")
	}
	idx := int(binary.BigEndian.Uint16(cuerpo))
	largoTile := int(binary.BigEndian.Uint32(cuerpo[2:]))

	// Psot = 0 significa «hasta el final del codestream» (§A.4.2).
	fin := len(cs)
	if largoTile > 0 {
		fin = pos + largoTile
		if fin > len(cs) {
			return 0, 0, fmt.Errorf("jpx: el tile %d dice medir más que el fichero", idx)
		}
	}
	// Dentro del tile-part puede haber más marcadores antes del SOD.
	sod := -1
	for k := pos + 2 + int(binary.BigEndian.Uint16(cs[pos+2:])); k+2 <= fin; {
		m := binary.BigEndian.Uint16(cs[k:])
		if m == jpxSOD {
			sod = k + 2
			break
		}
		if m>>8 != 0xFF || k+4 > fin {
			return 0, 0, fmt.Errorf("jpx: basura dentro del tile %d", idx)
		}
		k += 2 + int(binary.BigEndian.Uint16(cs[k+2:]))
	}
	if sod < 0 {
		return 0, 0, fmt.Errorf("jpx: el tile %d no trae SOD", idx)
	}
	img.tiles[idx] = append(img.tiles[idx], cs[sod:fin]...)
	return idx, fin, nil
}

// codestreamDeJP2 saca el codestream de un contenedor JP2, o lo devuelve tal
// cual si ya venía a pelo.
//
// Un PDF puede traer las dos cosas: el contenedor con sus cajas (cuando la
// imagen viene de un fichero .jp2) o el codestream desnudo. Distinguirlos por la
// firma es más fiable que por lo que declare el diccionario.
func codestreamDeJP2(datos []byte) ([]byte, error) {
	if len(datos) < 4 {
		return nil, fmt.Errorf("jpx: fichero vacío")
	}
	if binary.BigEndian.Uint16(datos) == jpxSOC {
		return datos, nil
	}
	// Contenedor: una lista de cajas, cada una con longitud y tipo de 4 bytes.
	// La que interesa es «jp2c», que envuelve el codestream.
	for i := 0; i+8 <= len(datos); {
		largo := int(binary.BigEndian.Uint32(datos[i:]))
		tipo := string(datos[i+4 : i+8])
		cabecera := 8
		switch {
		case largo == 1:
			// Longitud extendida de 64 bits.
			if i+16 > len(datos) {
				return nil, fmt.Errorf("jpx: caja con longitud extendida truncada")
			}
			l64 := binary.BigEndian.Uint64(datos[i+8:])
			// El tope se compara contra `math.MaxInt` ADEMÁS de contra lo que queda:
			// en 32 bits un valor de 5 GB cabría en la segunda comprobación y no en
			// un `int`, y la conversión daría un largo negativo con el que el
			// recorte de abajo lee fuera del slice. Es lo que avisa gosec G115, y
			// aquí el dato lo escribe un fichero de terceros.
			if l64 > uint64(len(datos)-i) || l64 > math.MaxInt {
				return nil, fmt.Errorf("jpx: caja %q más larga que el fichero", tipo)
			}
			largo, cabecera = int(l64), 16
		case largo == 0:
			// Hasta el final del fichero.
			largo = len(datos) - i
		case largo < 8 || i+largo > len(datos):
			return nil, fmt.Errorf("jpx: caja %q con longitud imposible", tipo)
		}
		if tipo == "jp2c" {
			return datos[i+cabecera : i+largo], nil
		}
		i += largo
	}
	return nil, fmt.Errorf("jpx: el contenedor no trae ninguna caja de imagen")
}

// codificacionDe devuelve el estilo que rige para una componente.
func (img *imagenJPX) codificacionDe(comp int) codificacionJPX {
	if c, ok := img.codPorComp[comp]; ok {
		return c
	}
	return img.cod
}
