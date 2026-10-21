// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import (
	"encoding/binary"
	"errors"
)

// ErrInvalidAnimatedGIF rechaza archivos estáticos, incompletos o cuyo coste
// de composición excede el presupuesto del selector.
var ErrInvalidAnimatedGIF = errors.New("validation: invalid or oversized animated GIF")

// AnimatedGIF comprueba la estructura antes de descomprimir con image/gif.
// El presupuesto cuenta el lienzo completo por fotograma: el cliente nativo
// compone esos lienzos, incluso cuando un fotograma modifica una zona pequeña.
// No sustituye al decodificador: limita lo que este puede llegar a reservar.
func AnimatedGIF(data []byte, maxBytes, maxSide, maxPixels int) error {
	if len(data) < 13 || len(data) > maxBytes {
		return ErrInvalidAnimatedGIF
	}
	if string(data[:6]) != "GIF89a" && string(data[:6]) != "GIF87a" {
		return ErrInvalidAnimatedGIF
	}
	w, h := gifWord(data[6:8]), gifWord(data[8:10])
	if w == 0 || h == 0 || w > maxSide || h > maxSide {
		return ErrInvalidAnimatedGIF
	}
	s := gifScanner{data: data, offset: 13, width: w, height: h, maxFrames: maxPixels / w / h}
	if !s.palette(data[10]) {
		return ErrInvalidAnimatedGIF
	}
	return s.inspect()
}

func gifWord(b []byte) int { return int(binary.LittleEndian.Uint16(b)) }

type gifScanner struct {
	data                                     []byte
	offset, width, height, maxFrames, frames int
}

func (s *gifScanner) skip(n int) bool {
	if n < 0 || n > len(s.data)-s.offset {
		return false
	}
	s.offset += n
	return true
}

func (s *gifScanner) palette(packed byte) bool {
	if packed&0x80 == 0 {
		return true
	}
	return s.skip(3 * (1 << (1 + (packed & 7))))
}

func (s *gifScanner) blocks() bool {
	for s.offset < len(s.data) {
		n := int(s.data[s.offset])
		s.offset++
		if n == 0 {
			return true
		}
		if !s.skip(n) {
			return false
		}
	}
	return false
}

func (s *gifScanner) frame() bool {
	if !s.skip(9) {
		return false
	}
	b := s.data[s.offset-9 : s.offset]
	x, y, w, h := gifWord(b[0:2]), gifWord(b[2:4]), gifWord(b[4:6]), gifWord(b[6:8])
	if w == 0 || h == 0 || x+w > s.width || y+h > s.height {
		return false
	}
	s.frames++
	if s.frames > s.maxFrames || s.frames > 512 {
		return false
	}
	return s.palette(b[8]) && s.skip(1) && s.blocks()
}

func (s *gifScanner) inspect() error {
	for s.offset < len(s.data) {
		kind := s.data[s.offset]
		s.offset++
		switch kind {
		case 0x3b: // trailer: nunca admitimos un archivo estático.
			if s.frames >= 2 && s.offset == len(s.data) {
				return nil
			}
			return ErrInvalidAnimatedGIF
		case 0x21: // extension label followed by data sub-blocks.
			if !s.skip(1) || !s.blocks() {
				return ErrInvalidAnimatedGIF
			}
		case 0x2c:
			if !s.frame() {
				return ErrInvalidAnimatedGIF
			}
		default:
			return ErrInvalidAnimatedGIF
		}
	}
	return ErrInvalidAnimatedGIF
}
