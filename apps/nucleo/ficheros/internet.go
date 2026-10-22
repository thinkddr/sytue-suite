// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import "io"

// GuardarDeInternet es Guardar para lo que llega de FUERA —el adjunto de un correo o
// de un canal de Rooms—: además lo marca como descargado de Internet, como hace el
// navegador (`Zone.Identifier` en Windows, `com.apple.quarantine` en macOS). De esa
// marca cuelgan SmartScreen, la Vista protegida de Office y Gatekeeper (FYS-02;
// decidido por el dueño: solo para los adjuntos de correo y de Rooms).
//
// Una marca que no se puede poner no impide la descarga: el fichero ya está, y quedarse
// sin él por eso sería peor.
func GuardarDeInternet(carpeta, nombre string, contenido io.Reader) (string, error) {
	ruta, err := Guardar(carpeta, nombre, contenido)
	if err != nil {
		return ruta, err
	}
	_ = marcarDeInternet(ruta)
	return ruta, nil
}
