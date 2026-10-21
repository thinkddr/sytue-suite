// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package webauthn

import (
	"errors"
	"fmt"
	"math"
)

// cborDecoder es un decodificador CBOR mínimo (RFC 8949), suficiente para el
// attestationObject de WebAuthn: mapas con claves de texto y valores byte-string,
// texto, enteros, arrays y mapas anidados (estos últimos se descartan). No es un
// decodificador CBOR general; cubre el subconjunto requerido.
type cborDecoder struct {
	buf []byte
	pos int
}

const (
	majUint   = 0
	majNegInt = 1
	majBytes  = 2
	majText   = 3
	majArray  = 4
	majMap    = 5
	majTag    = 6
	majSimple = 7
)

var errTruncated = errors.New("webauthn: CBOR truncado")

// lenArg valida una longitud declarada en la cabecera CBOR contra los bytes
// que quedan en el buffer. Evita el overflow uint64→int con longitudes
// hostiles (p. ej. 2^63) que, convertidas a int negativo, se saltarían las
// comprobaciones de rango y provocarían un panic (DoS).
func (d *cborDecoder) lenArg(arg uint64) (int, error) {
	if arg > uint64(len(d.buf)-d.pos) { //nolint:gosec // G115: pos <= len(buf) siempre (invariante del decoder); la resta no es negativa.
		return 0, errTruncated
	}
	return int(arg), nil //nolint:gosec // G115: arg <= len(buf)-pos <= MaxInt tras el guard.
}

func (d *cborDecoder) readByte() (byte, error) {
	if d.pos >= len(d.buf) {
		return 0, errTruncated
	}
	b := d.buf[d.pos]
	d.pos++
	return b, nil
}

// readHead lee el major type y el argumento (longitud/valor).
func (d *cborDecoder) readHead() (major byte, arg uint64, err error) {
	b, err := d.readByte()
	if err != nil {
		return 0, 0, err
	}
	major = b >> 5
	info := b & 0x1f
	switch {
	case info < 24:
		return major, uint64(info), nil
	case info == 24:
		x, err := d.readByte()
		return major, uint64(x), err
	case info == 25:
		return d.readN(major, 2)
	case info == 26:
		return d.readN(major, 4)
	case info == 27:
		return d.readN(major, 8)
	default:
		return 0, 0, fmt.Errorf("webauthn: CBOR info no soportado: %d", info)
	}
}

func (d *cborDecoder) readN(major byte, n int) (byte, uint64, error) {
	if d.pos+n > len(d.buf) {
		return 0, 0, errTruncated
	}
	var v uint64
	for i := 0; i < n; i++ {
		v = v<<8 | uint64(d.buf[d.pos+i])
	}
	d.pos += n
	return major, v, nil
}

// readMap decodifica un mapa de nivel superior con claves de texto. Los valores
// se devuelven como string, []byte, int64 o se descartan (mapas/arrays anidados).
func (d *cborDecoder) readMap() (map[string]any, error) {
	major, n, err := d.readHead()
	if err != nil {
		return nil, err
	}
	if major != majMap {
		return nil, fmt.Errorf("webauthn: se esperaba un mapa CBOR, major=%d", major)
	}
	// El hint de capacidad se acota: n viene del atacante y un valor enorme
	// pre-asignaría memoria sin haber leído un solo byte del mapa.
	out := make(map[string]any, min(n, 64))
	for i := uint64(0); i < n; i++ {
		key, err := d.readTextKey()
		if err != nil {
			return nil, err
		}
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}

// readIntMap decodifica un mapa CBOR con claves ENTERAS (positivas o negativas),
// como la estructura COSE_Key (RFC 9052 §7). Los valores se devuelven como int64
// o []byte (el subconjunto que usa una clave pública COSE); mapas/arrays anidados
// se descartan (nil) igual que en readValue.
func (d *cborDecoder) readIntMap() (map[int64]any, error) {
	major, n, err := d.readHead()
	if err != nil {
		return nil, err
	}
	if major != majMap {
		return nil, fmt.Errorf("webauthn: se esperaba un mapa CBOR, major=%d", major)
	}
	out := make(map[int64]any, min(n, 64))
	for i := uint64(0); i < n; i++ {
		key, err := d.readIntKey()
		if err != nil {
			return nil, err
		}
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}

// readIntKey lee una clave de mapa entera (uint o negint), como exige COSE.
func (d *cborDecoder) readIntKey() (int64, error) {
	major, arg, err := d.readHead()
	if err != nil {
		return 0, err
	}
	switch major {
	case majUint:
		if arg > math.MaxInt64 {
			return 0, errors.New("webauthn: clave COSE fuera de rango")
		}
		return int64(arg), nil //nolint:gosec // G115: arg <= MaxInt64 tras el guard.
	case majNegInt:
		if arg > math.MaxInt64 {
			return 0, errors.New("webauthn: clave COSE fuera de rango")
		}
		return -1 - int64(arg), nil //nolint:gosec // G115: arg <= MaxInt64 tras el guard.
	default:
		return 0, fmt.Errorf("webauthn: clave de mapa COSE no entera, major=%d", major)
	}
}

func (d *cborDecoder) readTextKey() (string, error) {
	major, n, err := d.readHead()
	if err != nil {
		return "", err
	}
	if major != majText {
		return "", fmt.Errorf("webauthn: clave de mapa no textual, major=%d", major)
	}
	return d.readBytesN(n)
}

func (d *cborDecoder) readBytesN(arg uint64) (string, error) {
	n, err := d.lenArg(arg)
	if err != nil {
		return "", err
	}
	s := string(d.buf[d.pos : d.pos+n])
	d.pos += n
	return s, nil
}

// readValue decodifica un valor; mapas y arrays anidados se saltan devolviendo nil.
//
//nolint:nilnil // (nil, nil) es el contrato documentado: el valor se descarta.
func (d *cborDecoder) readValue() (any, error) {
	major, arg, err := d.readHead()
	if err != nil {
		return nil, err
	}
	switch major {
	case majUint:
		if arg > math.MaxInt64 {
			return nil, fmt.Errorf("webauthn: entero CBOR fuera de rango")
		}
		return int64(arg), nil
	case majNegInt:
		if arg > math.MaxInt64 {
			return nil, fmt.Errorf("webauthn: entero CBOR fuera de rango")
		}
		return -1 - int64(arg), nil
	case majBytes:
		n, err := d.lenArg(arg)
		if err != nil {
			return nil, errTruncated
		}
		b := d.buf[d.pos : d.pos+n]
		d.pos += n
		return b, nil
	case majText:
		s, err := d.readBytesN(arg)
		return s, err
	case majArray:
		for i := uint64(0); i < arg; i++ {
			if _, err := d.readValue(); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case majMap:
		for i := uint64(0); i < arg; i++ {
			if _, err := d.readValue(); err != nil { // clave
				return nil, err
			}
			if _, err := d.readValue(); err != nil { // valor
				return nil, err
			}
		}
		return nil, nil
	case majTag:
		return d.readValue() // contenido del tag
	case majSimple:
		return nil, nil
	default:
		return nil, fmt.Errorf("webauthn: major CBOR no soportado: %d", major)
	}
}
