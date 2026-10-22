// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !((ios || (darwin && tienda)) && cgo)

package sistema

// Sin extensión File Provider: Linux, Windows, Android y el .dmg del Mac (que no va por
// Xcode y no la lleva). En el escritorio Drive se monta por WebDAV desde el Hub.

func archivosDisponible() bool               { return false }
func guardarCredencialArchivos([]byte) error { return ErrArchivosNoDisponible }
func hayCredencialArchivos() (bool, error)   { return false, nil }
func registrarArchivos(string) error         { return ErrArchivosNoDisponible }
func olvidarArchivos() error                 { return nil }
