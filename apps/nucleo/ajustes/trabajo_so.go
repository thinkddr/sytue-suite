// SPDX-License-Identifier: Apache-2.0
//go:build !android && !ios && !js

package ajustes

// TrabajoSesionCompartidoDisponible requires a shared native namespace. Desktop
// products use one OS-user configuration directory, including signed macOS apps.
func TrabajoSesionCompartidoDisponible() bool { return true }
