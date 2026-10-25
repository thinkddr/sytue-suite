// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"errors"
	"regexp"
)

var dominioDrive = regexp.MustCompile(`^[0-9a-f]{64}\.[0-9a-f]{32}$`)

func identificadorDominioArchivos(identidad, revision string) (string, error) {
	contexto := identidad + "." + revision
	if !dominioDrive.MatchString(contexto) {
		return "", errors.New("sistema: identidad o revisión de archivos inválida")
	}
	return "sytue-drive." + contexto, nil
}
