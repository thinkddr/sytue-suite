// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/local/imagenes"
)

// # HEIC
//
// Una foto HEIC entra en la fototeca como el JPEG que saca el sistema (`local/imagenes`,
// el mismo camino que las notas de imagen de Notepads): el del propio sistema, `sips`,
// `heif-convert` o el ffmpeg de la persona. Sin nada de eso, no se puede importar y se
// dice.

// conHEICConvertidas cambia, en lo elegido con «Añadir fotos», cada HEIC por un JPEG
// convertido en una carpeta temporal de la sesión (que el sistema limpia); las que no se
// pueden convertir se quedan tal cual y la importación dirá que no las sabe abrir.
func conHEICConvertidas(rutas []string) []string {
	out := make([]string, 0, len(rutas))
	for _, r := range rutas {
		if !imagenes.EsHEIC(r) {
			out = append(out, r)
			continue
		}
		ctx, fin := context.WithTimeout(context.Background(), 2*time.Minute)
		jpeg, _, err := imagenes.Convertir(ctx, r)
		fin()
		if err != nil {
			out = append(out, r)
			continue
		}
		// Con el nombre de la foto, no el del temporal.
		final := filepath.Join(filepath.Dir(jpeg), strings.TrimSuffix(filepath.Base(r), filepath.Ext(r))+".jpg")
		if os.Rename(jpeg, final) == nil {
			jpeg = final
		}
		out = append(out, jpeg)
	}
	return out
}
