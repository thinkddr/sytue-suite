// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package ficheros

import "io"

// Fuera de Android no hay árboles de documentos: la carpeta de Drive es una carpeta del
// disco, que se elige con [ElegirCarpeta] (arbol.go).

// ElegirArbol no existe fuera de Android.
func ElegirArbol() (ArbolElegido, error) { return ArbolElegido{}, ErrNoDisponible }

// ArbolVigente: fuera de Android no hay permisos guardados de árboles.
func ArbolVigente(string) bool { return false }

// SoltarArbol no hace nada fuera de Android.
func SoltarArbol(string) {}

// Raiz no existe fuera de Android.
func (Arbol) Raiz() (DocumentoArbol, error) { return DocumentoArbol{}, ErrNoDisponible }

// Datos no existe fuera de Android.
func (Arbol) Datos(string) (DocumentoArbol, error) { return DocumentoArbol{}, ErrNoDisponible }

// Hijos no existe fuera de Android.
func (Arbol) Hijos(string) ([]DocumentoArbol, error) { return nil, ErrNoDisponible }

// Crear no existe fuera de Android.
func (Arbol) Crear(string, string, bool) (DocumentoArbol, error) {
	return DocumentoArbol{}, ErrNoDisponible
}

// Renombrar no existe fuera de Android.
func (Arbol) Renombrar(string, string) (DocumentoArbol, error) {
	return DocumentoArbol{}, ErrNoDisponible
}

// Mover no existe fuera de Android.
func (Arbol) Mover(string, string, string) (DocumentoArbol, error) {
	return DocumentoArbol{}, ErrNoDisponible
}

// Borrar no existe fuera de Android.
func (Arbol) Borrar(string) error { return ErrNoDisponible }

// Leer no existe fuera de Android.
func (Arbol) Leer(string) (io.ReadCloser, error) { return nil, ErrNoDisponible }

// Escribir no existe fuera de Android.
func (Arbol) Escribir(string) (io.WriteCloser, error) { return nil, ErrNoDisponible }

// Espacio no existe fuera de Android.
func (Arbol) Espacio() (int64, int64, error) { return 0, 0, ErrNoDisponible }

// Ver no existe fuera de Android.
func (Arbol) Ver(string, bool) error { return ErrNoDisponible }

// VerEn no existe fuera de Android.
func (Arbol) VerEn(string, string, string) (bool, error) { return false, ErrNoDisponible }
