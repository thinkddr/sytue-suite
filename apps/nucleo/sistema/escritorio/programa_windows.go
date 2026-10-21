// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import "golang.org/x/sys/windows"

// lanzarPrograma es `ShellExecute` con el nombre del ejecutable: lo resuelve por el
// «App Paths» del registro, que es donde se apuntan Thunderbird y Outlook al instalarse.
// Sin él, devuelve error.
func lanzarPrograma(nombre string) error {
	verbo, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	exe, err := windows.UTF16PtrFromString(nombre)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verbo, exe, nil, nil, windows.SW_SHOWNORMAL)
}
