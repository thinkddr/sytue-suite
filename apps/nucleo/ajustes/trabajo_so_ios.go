// SPDX-License-Identifier: Apache-2.0
package ajustes

// iOS needs the suite App Group entitlement. Its per-app fallback directory is
// suitable for appearance, but cannot authorize shared private work or teardown.
func TrabajoSesionCompartidoDisponible() bool {
	_, err := Carpeta()
	return err == nil && enGrupoApple.Load()
}
