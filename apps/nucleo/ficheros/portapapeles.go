// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"os"
)

// CopiarImagen deja un PNG en el portapapeles del SISTEMA, que es donde se quiere un
// recorte para pegarlo en un correo: el portapapeles de Fyne solo lleva texto. Donde no
// se puede (sin `wl-copy` ni `xclip`, en el móvil) devuelve un error y quien llama cae
// en la descarga, que es lo que hace la web cuando `navigator.clipboard.write` falla.
func CopiarImagen(png []byte) error {
	f, err := os.CreateTemp("", "sytue-recorte-*.png")
	if err != nil {
		return err
	}
	ruta := f.Name()
	_, err = f.Write(png)
	if cerrar := f.Close(); err == nil {
		err = cerrar
	}
	if err == nil {
		err = copiarImagen(ruta)
	}
	// ponytail: el temporal se queda si el sistema lo lee después de volver (xclip lo
	// sirve desde su propio proceso, que ya lo ha leído); la carpeta temporal lo limpia.
	if err != nil {
		_ = os.Remove(ruta)
	}
	return err
}
