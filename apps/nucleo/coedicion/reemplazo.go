// SPDX-License-Identifier: Apache-2.0
package coedicion

// IntervaloReemplazo conserva el prefijo y el sufijo comunes de una edición.
// El centro se borra y reinserta con identidades nuevas: una tumba RGA es
// permanente. Compartir esta política mantiene iguales edición y restauración.
func IntervaloReemplazo[T any](viejo, nuevo []T, igual func(T, T) bool) (prefijo, sufijo int) {
	for prefijo < len(viejo) && prefijo < len(nuevo) && igual(viejo[prefijo], nuevo[prefijo]) {
		prefijo++
	}
	for sufijo < len(viejo)-prefijo && sufijo < len(nuevo)-prefijo && igual(viejo[len(viejo)-1-sufijo], nuevo[len(nuevo)-1-sufijo]) {
		sufijo++
	}
	return prefijo, sufijo
}
