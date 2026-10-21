// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

const claveRun = `Software\Microsoft\Windows\CurrentVersion\Run`

func alIniciarSesion(id, _, exe string, si bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, claveRun, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if !si {
		if err := k.DeleteValue(id); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	return k.SetStringValue(id, ordenRun(exe))
}

func arrancaAlIniciarSesion(id string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, claveRun, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer func() { _ = k.Close() }()
	_, _, err = k.GetStringValue(id)
	return err == nil
}
