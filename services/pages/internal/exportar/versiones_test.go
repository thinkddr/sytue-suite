// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"regexp"
	"strings"
	"testing"
)

func TestVersionesNoCaducadas(t *testing.T) {
	if err := ValidarCaducidad(); err != nil {
		t.Fatalf("ValidarCaducidad() falló: %v", err)
	}
}

func TestFichasSinVersionesCrudas(t *testing.T) {
	fichas, err := ListarFichas()
	if err != nil {
		t.Fatalf("ListarFichas() falló: %v", err)
	}
	if len(fichas) == 0 {
		t.Fatal("no se encontraron fichas técnicas")
	}

	// Regex para detectar versiones semánticas crudas tipo v1.2.3 o 1.2.3 fuera de marcadores {{v.*}}
	// Permitimos dígitos solos o fechas ISO (2026-09-30)
	reVersionCruda := regexp.MustCompile(`\bv\d+\.\d+(\.\d+)?\b|\b\d+\.\d+\.\d+\b`)

	for _, f := range fichas {
		baseName := strings.TrimSuffix(f, ".md")
		crudo, err := ObtenerFichaCruda(baseName)
		if err != nil {
			t.Errorf("ObtenerFichaCruda(%s) falló: %v", baseName, err)
			continue
		}

		// Comprobar que todos los marcadores {{v.<clave>}} existen en Tabla
		submatches := reMarcador.FindAllStringSubmatch(crudo, -1)
		for _, sm := range submatches {
			clave := sm[1]
			if _, ok := Tabla[clave]; !ok {
				t.Errorf("ficha %s contiene marcador desconocido {{v.%s}}", f, clave)
			}
		}

		// Reemplazar los marcadores válidos temporalmente por un texto neutro para comprobar si quedan versiones crudas
		sinMarcadores := reMarcador.ReplaceAllString(crudo, "VERSION_PLACEHOLDER")

		// Buscar si quedan versiones crudas escritas a mano
		coincidencias := reVersionCruda.FindAllString(sinMarcadores, -1)
		for _, c := range coincidencias {
			t.Errorf("ficha %s contiene un número de versión crudo '%s' que debería usar un marcador {{v.<clave>}}", f, c)
		}
	}
}

func TestReemplazarMarcadores(t *testing.T) {
	entrada := "Versión: {{v.shopify_theme_version}}, CLI: {{v.shopify_cli_version}}"
	esperado := "Versión: 1.0.0, CLI: 3.68.0"

	salida, err := ReemplazarMarcadores(entrada)
	if err != nil {
		t.Fatalf("ReemplazarMarcadores devolvió error inesperado: %v", err)
	}
	if salida != esperado {
		t.Errorf("esperaba %q, obtuvo %q", esperado, salida)
	}

	// Marcador inexistente
	_, errFallo := ReemplazarMarcadores("Invalido: {{v.version_que_no_existe}}")
	if errFallo == nil {
		t.Error("esperaba error con marcador desconocido, obtuvo nil")
	}
}
