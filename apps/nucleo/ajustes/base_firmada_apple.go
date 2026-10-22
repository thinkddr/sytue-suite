// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || (darwin && tienda)

package ajustes

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// baseSinGrupoApple es la carpeta de antes de los widgets, la que se usa sin
// `group.eu.sytue.suite` en la firma (SYTUE_WIDGETS_APPLE apagado):
//
//   - iOS: la de os.UserConfigDir, dentro del contenedor de la propia app.
//   - Mac App Store: el contenedor de grupo de siempre, `<equipo>.eu.sytue.suite`. Con
//     el App Sandbox es la única carpeta que comparten las quince apps, y macOS lo
//     acepta sin registrarlo en Developer Portal. Encendidos, base() migra desde aquí.
//     La casa es la de la cuenta (`os/user`) y no `$HOME`: dentro del sandbox `$HOME`
//     es el contenedor de la app.
func baseSinGrupoApple() (string, error) {
	if runtime.GOOS == "ios" {
		return os.UserConfigDir()
	}
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(u.HomeDir, "Library", "Group Containers", GrupoSuite), nil
}
