// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Trasladar pone al día el índice de la videoteca de datos (`Opciones.Datos`), que ya
// está COPIADO en su sitio nuevo, cuando la carpeta de datos entera ha pasado de `de` a
// `a` (Ajustes → Datos, «mover la carpeta de datos»). El índice guarda la ruta absoluta
// de cada vídeo: los que estaban bajo `de` —los copiados en `Vídeos/`— pasan a estar
// bajo `a`; los apuntados de otra carpeta se quedan como estaban. Sin índice no hace
// nada. La videoteca no tiene que estar abierta.
func Trasladar(datos, de, a string) error {
	ruta := filepath.Join(datos, "videoteca.json")
	var lista []*Entrada
	if err := espacio.LeerJSON(ruta, &lista); err != nil || lista == nil {
		return err
	}
	de, a = filepath.Clean(de), filepath.Clean(a)
	cambiado := false
	for _, e := range lista {
		if e == nil || !filepath.IsAbs(e.Ruta) {
			continue
		}
		rel, err := filepath.Rel(de, e.Ruta)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		e.Ruta = filepath.Join(a, rel)
		cambiado = true
	}
	if !cambiado {
		return nil
	}
	return espacio.EscribirJSON(ruta, lista)
}
