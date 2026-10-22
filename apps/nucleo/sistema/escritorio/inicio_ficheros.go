// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (linux && !android) || (darwin && !ios)

package escritorio

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// ficheroInicio es dónde guarda este escritorio la entrada de inicio de `id`, y lo que
// lleva dentro.
func ficheroInicio(id, nombre, exe string) (ruta, contenido string, err error) {
	if runtime.GOOS == "darwin" {
		casa, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		return filepath.Join(casa, "Library", "LaunchAgents", id+".plist"), agenteLaunchd(id, exe), nil
	}
	config, err := os.UserConfigDir() // $XDG_CONFIG_HOME, o ~/.config
	if err != nil {
		return "", "", err
	}
	return filepath.Join(config, "autostart", id+".desktop"), entradaAutostart(nombre, exe), nil
}

func alIniciarSesion(id, nombre, exe string, si bool) error {
	ruta, contenido, err := ficheroInicio(id, nombre, exe)
	if err != nil {
		return err
	}
	if !si {
		if err := os.Remove(ruta); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ruta, []byte(contenido), 0o644) //nolint:gosec // lo lee el sistema, no es secreto
}

func arrancaAlIniciarSesion(id string) bool {
	ruta, _, err := ficheroInicio(id, "", "")
	if err != nil {
		return false
	}
	_, err = os.Stat(ruta)
	return err == nil
}
