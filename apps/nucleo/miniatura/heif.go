// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package miniatura

import (
	"bytes"
	"encoding/binary"
	"image"

	// Registra HEIC en image.Decode/DecodeConfig. Ver «HEIC» abajo.
	"github.com/gen2brain/h265/heic"
)

// # HEIC (HEVC en un contenedor HEIF): las fotos del iPhone
//
// Hasta el 2026-11-25 aquí no se decodificaba HEIC: Go no trae decodificador y los que
// había eran libheif o libde265 por cgo o compilados a WASM, LGPL los dos, que la
// política de licencias no deja enlazar en el producto (docs/security.md §5). Un HEIC
// subido desde Windows o Linux, donde el sistema no lo convierte, quedaba en Drive sin
// salir en la galería. Ahora lo decodifica `github.com/gen2brain/h265`: HEVC y HEIC en
// Go puro, sin cgo, sin WASM y sin dependencias, MIT, que se declara exacto bit a bit
// con la batería de conformidad HEVC v1 del JCT-VC.
//
// En HEIF la orientación la dicen las propiedades del contenedor —`irot`, `imir` y el
// recorte `clap`— y el EXIF Orientation es solo informativo: quien aplique los dos gira
// la foto dos veces. Así que se decodifica con AutoRotate (la imagen sale derecha) y la
// cabecera da las dimensiones YA giradas, leyendo el `irot` del elemento principal
// (giroHEIF): la galería coloca el hueco con la forma que tendrá la miniatura. Quien lea
// el EXIF de un HEIC no debe volver a girar con él.

// esHEIF dice si image.DecodeConfig reconoció el fichero como HEIC.
func esHEIF(formato string) bool { return formato == "heic" }

// decodificarHEIF es heic.Decode con las transformaciones del contenedor aplicadas.
func decodificarHEIF(datos []byte) (image.Image, error) {
	return heic.Decode(bytes.NewReader(datos), heic.Options{AutoRotate: true})
}

// girarConfigHEIF intercambia ancho y alto si el elemento principal va girado un
// cuarto de vuelta: es lo que devolverá decodificarHEIF.
func girarConfigHEIF(cfg image.Config, datos []byte) image.Config {
	if g, ok := giroHEIF(datos); ok && (g == 90 || g == 270) {
		cfg.Width, cfg.Height = cfg.Height, cfg.Width
	}
	return cfg
}

// cajaISO es una caja de ISOBMFF: su tipo y su cuerpo, sin la cabecera.
type cajaISO struct {
	tipo   string
	cuerpo []byte
}

// cajasISO parte `b` en sus cajas hijas. Lo que no cuadra —un tamaño que se sale, uno
// imposible— corta la lista ahí: estos bytes vienen de fuera, y una longitud declarada
// dentro del fichero no se cree sin medirla contra lo que de verdad hay.
func cajasISO(b []byte) []cajaISO {
	var out []cajaISO
	for len(b) >= 8 {
		tam := uint64(binary.BigEndian.Uint32(b[:4]))
		tipo := string(b[4:8])
		cab := uint64(8)
		switch tam {
		case 0: // hasta el final
			tam = uint64(len(b))
		case 1: // tamaño de 64 bits
			if len(b) < 16 {
				return out
			}
			tam, cab = binary.BigEndian.Uint64(b[8:16]), 16
		}
		if tam < cab || tam > uint64(len(b)) {
			return out
		}
		out = append(out, cajaISO{tipo, b[cab:tam]})
		b = b[tam:]
	}
	return out
}

func buscarCaja(cajas []cajaISO, tipo string) (cajaISO, bool) {
	for _, c := range cajas {
		if c.tipo == tipo {
			return c, true
		}
	}
	return cajaISO{}, false
}

// giroHEIF es el giro de `irot` del elemento principal, en grados en sentido
// antihorario (0, 90, 180 o 270). Solo recorre la caja `meta` (pitm, iprp/ipco e
// iprp/ipma), que va en la cabecera. ok es falso si no se sabe.
func giroHEIF(datos []byte) (int, bool) {
	meta, ok := buscarCaja(cajasISO(datos), "meta")
	if !ok || len(meta.cuerpo) < 4 {
		return 0, false
	}
	hijos := cajasISO(meta.cuerpo[4:]) // FullBox: versión y banderas
	principal, ok := elementoPrincipal(hijos)
	if !ok {
		return 0, false
	}
	iprp, ok := buscarCaja(hijos, "iprp")
	if !ok {
		return 0, false
	}
	nietos := cajasISO(iprp.cuerpo)
	ipco, ok1 := buscarCaja(nietos, "ipco")
	ipma, ok2 := buscarCaja(nietos, "ipma")
	if !ok1 || !ok2 {
		return 0, false
	}
	propiedades := cajasISO(ipco.cuerpo)
	for _, i := range propiedadesDe(ipma.cuerpo, principal) {
		if i < 1 || i > len(propiedades) {
			continue
		}
		if p := propiedades[i-1]; p.tipo == "irot" && len(p.cuerpo) >= 1 {
			return int(p.cuerpo[0]&3) * 90, true
		}
	}
	return 0, true
}

// elementoPrincipal es el `item_ID` de `pitm`.
func elementoPrincipal(cajas []cajaISO) (uint32, bool) {
	pitm, ok := buscarCaja(cajas, "pitm")
	if !ok || len(pitm.cuerpo) < 6 {
		return 0, false
	}
	if pitm.cuerpo[0] == 0 {
		return uint32(binary.BigEndian.Uint16(pitm.cuerpo[4:6])), true
	}
	if len(pitm.cuerpo) < 8 {
		return 0, false
	}
	return binary.BigEndian.Uint32(pitm.cuerpo[4:8]), true
}

// propiedadesDe son los índices (desde 1) de las propiedades asociadas a `elemento` en
// `ipma` (ISO/IEC 23008-12 §9.3.3).
func propiedadesDe(ipma []byte, elemento uint32) []int {
	if len(ipma) < 8 {
		return nil
	}
	version, anchos := ipma[0], ipma[3]&1 // bandera 1: índices de 15 bits
	n := binary.BigEndian.Uint32(ipma[4:8])
	b := ipma[8:]
	for range n {
		var id uint32
		if version < 1 {
			if len(b) < 2 {
				return nil
			}
			id, b = uint32(binary.BigEndian.Uint16(b)), b[2:]
		} else {
			if len(b) < 4 {
				return nil
			}
			id, b = binary.BigEndian.Uint32(b), b[4:]
		}
		indices, resto, ok := indicesIpma(b, anchos)
		if !ok {
			return nil
		}
		b = resto
		if id == elemento {
			return indices
		}
	}
	return nil
}

// indicesIpma lee de `b` la lista de índices de propiedades de un elemento de `ipma`: un
// byte con cuántos y cada índice de 7 bits o, con `anchos` a 1, de 15. Devuelve lo que
// queda de `b` y false si el fichero está cortado o miente.
func indicesIpma(b []byte, anchos byte) ([]int, []byte, bool) {
	if len(b) < 1 {
		return nil, nil, false
	}
	cuantas := int(b[0])
	b = b[1:]
	// Cada índice ocupa al menos un byte: si el fichero dice más de los que quedan, no se
	// reserva memoria por lo que dice (CodeQL, go/uncontrolled-allocation-size).
	if cuantas > len(b) {
		return nil, nil, false
	}
	indices := make([]int, 0, cuantas)
	for range cuantas {
		if anchos == 1 {
			if len(b) < 2 {
				return nil, nil, false
			}
			indices, b = append(indices, int(binary.BigEndian.Uint16(b)&0x7fff)), b[2:]
			continue
		}
		if len(b) < 1 {
			return nil, nil, false
		}
		indices, b = append(indices, int(b[0]&0x7f)), b[1:]
	}
	return indices, b, true
}
