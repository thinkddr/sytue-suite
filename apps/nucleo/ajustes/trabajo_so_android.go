// SPDX-License-Identifier: Apache-2.0
package ajustes

// Separate Android app UIDs require the trusted session broker's witness and
// work leases. A per-app file must never impersonate that cross-app guarantee.
func TrabajoSesionCompartidoDisponible() bool {
	disponible, err := EstadoTrabajoSesionCompartido()
	return err == nil && disponible
}
