// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
)

// nombreOculto es un nombre libre y oculto en carpeta para dejar un fichero a medias: la
// videoteca de Cast no recoge lo oculto, así que nunca ve un vídeo sin acabar.
func nombreOculto(carpeta, ext string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return filepath.Join(carpeta, ".photos-"+hex.EncodeToString(b[:])+ext)
}
