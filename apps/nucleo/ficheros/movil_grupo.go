// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(ios && cgo)

package ficheros

// CarpetaDeGrupo es el contenedor compartido de un App Group de Apple, que solo existe en
// iOS (movil_grupo_ios.go). En Android no hay equivalente sin un proveedor de contenido
// firmado; en escritorio las apps de la edición ya comparten la carpeta de ajustes.
func CarpetaDeGrupo(string) (string, error) { return "", ErrNoDisponible }
