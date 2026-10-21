// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import "golang.org/x/sys/windows"

func predeterminada(nombre string, _ []string) error {
	verbo, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	pagina, err := windows.UTF16PtrFromString(paginaPredeterminadas(nombre))
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verbo, pagina, nil, nil, windows.SW_SHOWNORMAL)
}
