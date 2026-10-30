// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !((ios || darwin) && cgo)

package sistema

// Sin File Provider de Apple: los demás sistemas tienen su propio proveedor nativo.

func archivosDisponible() bool                     { return false }
func guardarCredencialArchivos([]byte) error       { return ErrArchivosNoDisponible }
func hayCredencialArchivos() (bool, error)         { return false, nil }
func hayCredencialArchivosDe(string) (bool, error) { return false, nil }
func registrarArchivos(string) error               { return ErrArchivosNoDisponible }
func olvidarArchivos() error                       { return nil }

func abrirArchivos() error                    { return ErrArchivosNoDisponible }
func abrirDominioArchivos(string, bool) error { return ErrArchivosNoDisponible }
