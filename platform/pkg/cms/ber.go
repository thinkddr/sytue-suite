// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cms

import "errors"

// errBER es un TLV que ni en BER se lee.
var errBER = errors.New("cms: codificación BER ilegible")

// berADer reescribe en DER el primer TLV de b, que puede venir en BER: longitudes
// indefinidas (0x80 … 00 00) y OCTET STRING construidos (X.690 §8.1.3.6 y
// §8.7). Lo que haya detrás —el relleno de ceros del hueco de una firma PDF—
// se ignora. Algunos firmantes (medido: un PDF del corpus de pdf.js) emiten así
// la firma entera, y `encoding/asn1` solo lee DER.
//
// ponytail: no reordena los SET OF (DER los quiere ordenados): lo firmado son
// los atributos tal cual vinieron, y reordenarlos rompería la firma.
func berADer(b []byte) ([]byte, error) {
	out, _, err := tlvADer(b, 0)
	return out, err
}

// tlvADer convierte un TLV y devuelve también cuántos bytes de b ocupaba.
func tlvADer(b []byte, prof int) ([]byte, int, error) {
	if prof > 64 || len(b) < 2 {
		return nil, 0, errBER
	}
	i := largoEtiqueta(b)
	if i >= len(b) {
		return nil, 0, errBER
	}
	etiqueta, construido := b[:i], b[0]&0x20 != 0
	contenido, usados, indefinida, err := contenidoBER(b, i, construido, prof)
	if err != nil {
		return nil, 0, err
	}
	if construido && !indefinida {
		// Los hijos de un construido de longitud definida también pueden ser BER.
		if contenido, err = hijosADer(contenido, prof); err != nil {
			return nil, 0, err
		}
	}
	if len(etiqueta) == 1 && etiqueta[0] == 0x24 {
		// OCTET STRING construido: en DER es uno primitivo con todo el contenido.
		if contenido, err = juntarOctetos(contenido); err != nil {
			return nil, 0, err
		}
		etiqueta = []byte{0x04}
	}
	out := append(append([]byte(nil), etiqueta...), largoDer(len(contenido))...)
	return append(out, contenido...), usados, nil
}

// largoEtiqueta: uno o varios bytes (forma alta si los cinco bits bajos son 1).
func largoEtiqueta(b []byte) int {
	if b[0]&0x1F != 0x1F {
		return 1
	}
	i := 1
	for i < len(b) && b[i]&0x80 != 0 {
		i++
	}
	return i + 1
}

// contenidoBER lee la longitud en i y devuelve el contenido (ya en DER si era
// indefinida), los bytes usados y si la longitud era indefinida.
func contenidoBER(b []byte, i int, construido bool, prof int) ([]byte, int, bool, error) {
	l := int(b[i])
	i++
	switch {
	case l == 0x80: // indefinida: hijos hasta 00 00
		if !construido {
			return nil, 0, false, errBER
		}
		var contenido []byte
		for p := i; p+2 <= len(b); {
			if b[p] == 0 && b[p+1] == 0 {
				return contenido, p + 2, true, nil
			}
			hijo, n, err := tlvADer(b[p:], prof+1)
			if err != nil {
				return nil, 0, false, err
			}
			contenido = append(contenido, hijo...)
			p += n
		}
		return nil, 0, false, errBER
	case l < 0x80:
		if i+l > len(b) {
			return nil, 0, false, errBER
		}
		return b[i : i+l], i + l, false, nil
	}
	k := l & 0x7F
	if k > 4 || i+k > len(b) {
		return nil, 0, false, errBER
	}
	n := 0
	for _, x := range b[i : i+k] {
		n = n<<8 | int(x)
	}
	i += k
	if n < 0 || i+n > len(b) {
		return nil, 0, false, errBER
	}
	return b[i : i+n], i + n, false, nil
}

func hijosADer(contenido []byte, prof int) ([]byte, error) {
	var hijos []byte
	for p := 0; p < len(contenido); {
		hijo, n, err := tlvADer(contenido[p:], prof+1)
		if err != nil {
			return nil, err
		}
		hijos = append(hijos, hijo...)
		p += n
	}
	return hijos, nil
}

func juntarOctetos(contenido []byte) ([]byte, error) {
	var junto []byte
	for p := 0; p < len(contenido); {
		_, n, ini, err := cabeceraDer(contenido[p:])
		if err != nil {
			return nil, err
		}
		junto = append(junto, contenido[p+ini:p+n]...)
		p += n
	}
	return junto, nil
}

// cabeceraDer lee un TLV ya en DER: su etiqueta, cuánto ocupa entero y dónde
// empieza el contenido.
func cabeceraDer(b []byte) (byte, int, int, error) {
	if len(b) < 2 {
		return 0, 0, 0, errBER
	}
	l, i := int(b[1]), 2
	if l >= 0x80 {
		k := l & 0x7F
		if k == 0 || k > 4 || 2+k > len(b) {
			return 0, 0, 0, errBER
		}
		l = 0
		for _, x := range b[2 : 2+k] {
			l = l<<8 | int(x)
		}
		i += k
	}
	if i+l > len(b) {
		return 0, 0, 0, errBER
	}
	return b[0], i + l, i, nil
}

func largoDer(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n & 0x7F)}
	}
	var bs []byte
	for v := n; v > 0; v >>= 8 {
		bs = append([]byte{byte(v & 0xFF)}, bs...)
	}
	return append([]byte{0x80 | byte(len(bs)&0x7F)}, bs...)
}
