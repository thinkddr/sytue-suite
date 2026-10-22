// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import "os"

// marcarDeInternet escribe el flujo alternativo Zone.Identifier con la zona de
// Internet (3), que es lo que deja el navegador.
func marcarDeInternet(ruta string) error {
	return os.WriteFile(ruta+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o644) //nolint:gosec // G306: la marca la lee el sistema; no lleva nada secreto
}
