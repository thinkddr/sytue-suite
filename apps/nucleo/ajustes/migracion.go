// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// migrarPreferencias sólo importa preferencias antiguas cuando el contenedor común
// todavía no tiene las suyas. No copia tokens, marcas de sesión ni cerrojos. La
// publicación sin reemplazo conserva cualquier elección simultánea de otra app.
func migrarPreferencias(base string, anteriores ...string) error {
	dir := filepath.Join(base, "sytue")
	destino := filepath.Join(dir, ficheroAjustes)
	if _, err := os.Stat(destino); err == nil {
		return nil
	}
	for _, anterior := range anteriores {
		origen := filepath.Join(anterior, "sytue", ficheroAjustes)
		if origen == destino {
			continue
		}
		datos, err := os.ReadFile(origen)
		if err != nil {
			continue
		}
		var preferencias Ajustes
		if json.Unmarshal(datos, &preferencias) != nil {
			continue
		}
		// Sólo preferencias conocidas: un campo histórico desconocido no debe
		// terminar en el contenedor compartido con extensiones.
		datos, err = json.Marshal(preferencias)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, permisoCarpeta); err != nil {
			return err
		}
		return escribirEntero(destino, datos, true)
	}
	return nil
}
