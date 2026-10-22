// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"slices"
	"strings"
)

// NoPintado son las claves de catálogo de lo que hay en la presentación y este lienzo
// NO dibuja como la web, sin repetir y en orden estable.
//
// Es lo que va en el cartel de arriba, y es la mitad honesta del contrato: la otra,
// `Fiel`, dice si el documento se podría volver a escribir sin perder nada. Las dos
// pueden discrepar y por eso son dos: un campo que la web añada mañana no se conserva
// (`Fiel` dice que no) aunque no haya nada nuevo que dibujar.
//
// Formas, gráficos, diagramas, conectores y el marcador de un medio se pintan con el
// mismo SVG que la web (`formas.go`), y la Georgia del tema «arena» con la del equipo,
// como el navegador. Queda una imagen que no viene como `data:` —una URL—, que no se
// descarga al pintar.
//
// Vacío = la diapositiva que se ve es la que hay.
func (m *Modelo) NoPintado() []string {
	var claves []string
	añadir := func(c string) {
		if !slices.Contains(claves, c) {
			claves = append(claves, c)
		}
	}
	for i := range m.Diapos {
		for _, c := range m.CajasResueltas(i) {
			if c.Tipo == "image" {
				if _, ok := imagenDe(c.Src); !ok {
					añadir(claveDe(c))
				}
			}
		}
	}
	return claves
}

// Resumen junta las claves de NoPintado en una línea, ya traducidas.
func Resumen(claves []string, rotulo func(string) string) string {
	fuera := make([]string, len(claves))
	for i, c := range claves {
		fuera[i] = rotulo(c)
	}
	return strings.Join(fuera, ", ")
}
