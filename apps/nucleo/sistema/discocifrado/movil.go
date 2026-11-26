// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package discocifrado

// # Android e iOS
//
// Los dos cifran siempre el almacenamiento de las apps: Android desde la 10 con el
// cifrado por fichero obligatorio (y su carpeta de datos es la privada de la app); iOS
// con la Protección de datos de cada fichero, atada al código del teléfono. No hace falta
// preguntar nada.

// deMovil es el resultado en el teléfono: goos es "android" o "ios".
func deMovil(goos string) Resultado {
	switch goos {
	case "android":
		return cifradoCon("Android")
	case "ios":
		return cifradoCon("iOS")
	}
	return noSeSabe
}
