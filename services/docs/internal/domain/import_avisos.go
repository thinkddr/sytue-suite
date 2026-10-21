// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// Los avisos que el importador escribe DENTRO del documento —«esto es una vista previa»,
// «este fichero lleva contraseña»— son texto que lee una persona, así que salen en su
// idioma (CONVENTIONS §13.1: también las funciones que generan texto). Viven aquí y no
// en `plantillas/textos` porque `plantillas` importa este paquete, y no al revés.
//
// El inglés es la base y el respaldo; `import_avisos_test.go` exige que cada idioma
// tenga exactamente sus claves.

//go:embed import_avisos.json
var avisosJSON []byte

var (
	avisosUnaVez sync.Once
	avisos       map[string]map[string]string
)

// avisoImportar es el aviso `clave` en `idioma` («es», «es-ES», «pt_PT»…), con el
// inglés de respaldo.
func avisoImportar(idioma, clave string) string {
	avisosUnaVez.Do(func() {
		if err := json.Unmarshal(avisosJSON, &avisos); err != nil {
			panic("domain: import_avisos.json: " + err.Error()) // lo atrapa el test, nunca llega a producción
		}
	})
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(idioma)), "-")
	base, _, _ = strings.Cut(base, "_")
	if s, ok := avisos[base][clave]; ok {
		return s
	}
	return avisos["en"][clave]
}
