// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package anfitrion es lo del sistema que necesitan las apps de la suite y que NO es
// sesión: el alto del teclado en pantalla y leer un fichero de la máquina
// (PLAN-EDICION-LOCAL §5.4, C-04).
//
// Vivía en `sesion`, y por eso `ui` (los diálogos) y la cáscara de Sytue Office
// arrastraban `sesion` —y con él el login OIDC— a binarios que no inician sesión.
// `sesion` delega aquí sin cambiar lo que hace (`sesion.AltoTeclado`, `sesion.LeerLocal`).
package anfitrion

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// LeerLocal trae el contenido de un fichero de la máquina (file://, content:// o una
// ruta absoluta) con el mismo tope que tiene el conversor, para no subir cuarenta megas
// y recibir un 413.
func LeerLocal(crudo string) (string, []byte, error) {
	return LeerLocalHasta(crudo, api.MaxImportar)
}

// LeerLocalHasta es LeerLocal con el tope que elija quien llama (bytes; 0 o menos, el
// del conversor): CertiPDF lee un `content://` de Android hasta los 100 MiB del motor.
func LeerLocalHasta(crudo string, tope int) (string, []byte, error) {
	if tope <= 0 {
		tope = api.MaxImportar
	}
	uri, err := storage.ParseURI(crudo)
	if err != nil {
		return "", nil, err
	}
	nombre := uri.Name()
	if nombre == "" {
		nombre = filepath.Base(crudo)
	}

	// Si es un fichero del sistema (file:// o ruta absoluta), usamos la vía segura
	// del núcleo que maneja security scopes en iOS y lectura limpia sin depender
	// de punteros nativos efímeros.
	if uri.Scheme() == "file" || strings.HasPrefix(crudo, "/") {
		datos, errSeguro := ficheros.LeerLocalSeguro(crudo)
		if errSeguro == nil && len(datos) > 0 {
			if len(datos) > tope {
				return "", nil, demasiado(nombre, tope)
			}
			return nombre, datos, nil
		}
	}

	r, err := storage.Reader(uri)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = r.Close() }()
	datos, err := io.ReadAll(io.LimitReader(r, int64(tope)+1))
	if err != nil {
		return "", nil, err
	}
	if len(datos) > tope {
		return "", nil, demasiado(nombre, tope)
	}
	return nombre, datos, nil
}

// demasiado conserva el texto de siempre (con su prefijo `sesion:`): es lo que ya
// enseñaban las apps conectadas.
func demasiado(nombre string, tope int) error {
	if tope != api.MaxImportar {
		return fmt.Errorf("%q pasa de los %d MiB que se pueden abrir", nombre, tope>>20)
	}
	return fmt.Errorf("sesion: %q pasa de los %d MiB que convierte el servicio", nombre, api.MaxImportar>>20)
}
