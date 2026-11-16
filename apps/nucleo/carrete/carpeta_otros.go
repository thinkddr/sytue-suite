// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !windows && !(darwin && cgo)

package carrete

// carpetasGaleria fuera de Windows es una: la de imágenes del perfil.
func carpetasGaleria() ([]string, error) {
	r, err := raiz()
	return []string{r}, err
}

// redMedida: Linux no tiene datos móviles que cuidar, y no hay una forma común de saber
// si la red es de pago (NetworkManager lo sabe, pero no siempre está).
func redMedida() bool { return false }
