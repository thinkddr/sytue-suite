// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// MaxSelectedPages bounds page expansion for device and HTTP requests.
const (
	MaxSelectedPages = 10000
	// MaxInputFiles bounds a single PDF combination request.
	MaxInputFiles = 64
)

// InputError describes an invalid PDF operation selected by its caller.
type InputError struct{ Message string }

func (e *InputError) Error() string   { return e.Message }
func inputError(message string) error { return &InputError{Message: message} }

// ParsePageSelection preserves ordering and repeated pages in a 1-based selection.
func ParsePageSelection(spec string, total int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, inputError(
			`Give the pages, for example "1,3,5-8".`,
		)
	}
	var out []int
	for _, trozo := range strings.Split(spec, ",") {
		trozo = strings.TrimSpace(trozo)
		if trozo == "" {
			continue
		}
		desde, hasta, err := parsePageRange(trozo)
		if err != nil {
			return nil, err
		}
		if desde < 1 || desde > total || hasta < 1 || hasta > total {
			return nil, inputError(fmt.Sprintf(
				"Page %s does not exist: the document has %d.", trozo, total,
			))
		}
		paso := 1
		if hasta < desde {
			paso = -1 // «8-5» es válido y significa esas páginas al revés
		}
		for p := desde; ; p += paso {
			out = append(out, p-1) // a 0-based, en un solo sitio
			if len(out) > MaxSelectedPages {
				return nil, inputError(fmt.Sprintf(
					"Too many pages in a single request (maximum %d).", MaxSelectedPages,
				))
			}
			if p == hasta {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, inputError(`Give the pages, for example "1,3,5-8".`)
	}
	return out, nil
}

// parsePageRange entiende «5» y «5-8». Devuelve ambos extremos 1-based.
func parsePageRange(trozo string) (int, int, error) {
	malo := inputError(fmt.Sprintf(
		"%q could not be understood. Use numbers and ranges, for example \"1,3,5-8\".", trozo,
	))

	if i := strings.IndexByte(trozo, '-'); i > 0 { // > 0: un «-» inicial sería negativo
		a, err1 := strconv.Atoi(strings.TrimSpace(trozo[:i]))
		b, err2 := strconv.Atoi(strings.TrimSpace(trozo[i+1:]))
		if err1 != nil || err2 != nil {
			return 0, 0, malo
		}
		return a, b, nil
	}
	n, err := strconv.Atoi(trozo)
	if err != nil {
		return 0, 0, malo
	}
	return n, n, nil
}

// ParsePageRotation validates a page rotation. PDF only accepts multiples of 90 in
// /Rotate, y aceptar 45 escribiría un fichero que los visores ignoran.
func ParsePageRotation(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, inputError("The rotation is missing: 90, 180, 270 or -90.")
	}
	g, err := strconv.Atoi(s)
	if err != nil || g%90 != 0 {
		return 0, inputError("The rotation must be a multiple of 90 degrees.")
	}
	return ((g % 360) + 360) % 360, nil
}

// ParseOptionalInteger reads a 1-based field with a default and an upper bound.
func ParseOptionalInteger(s string, defaultValue, maximum int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(s)
	if err != nil || value < 1 || value > maximum {
		return 0, inputError(fmt.Sprintf("%q is not valid: a number between 1 and %d is expected.", s, maximum))
	}
	return value, nil
}
