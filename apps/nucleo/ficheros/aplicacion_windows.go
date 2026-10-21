// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

// elegirAplicacion es el cuadro «Abrir» filtrado a programas.
func elegirAplicacion(titulo string) (string, error) {
	rutas, err := ElegirConTipos(titulo, "exe")
	if err != nil || len(rutas) == 0 {
		return "", err
	}
	return rutas[0], nil
}

// abrirCon ejecuta el programa con el fichero como argumento, que es como se abre un
// documento con una aplicación en Windows.
func abrirCon(app, fichero string) error { return lanzar(app, fichero) }
