// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func TestEspejoDeCampos(t *testing.T) {
	ruta := filepath.Join("..", "..", "..", "..", "apps", "frontend", "packages", "pages-render", "src", "ir", "campos.ts")
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("no se pudo leer campos.ts en %s: %v", ruta, err)
	}
	contenido := string(b)

	// Extraer CAMPOS_NODO de TS
	reNodoBloque := regexp.MustCompile(`([a-z_]+):\s*conBase\((\{[\s\S]*?\})\)`)
	matchesNodo := reNodoBloque.FindAllStringSubmatch(contenido, -1)
	rePar := regexp.MustCompile(`"([a-z_]+)":\s*"([^"]+)"`)

	for _, m := range matchesNodo {
		tipoNodo := m[1]
		pares := rePar.FindAllStringSubmatch(m[2], -1)
		goTabla, ok := ir.CamposNodo[tipoNodo]
		if !ok {
			t.Errorf("tipo de nodo %s no existe en CamposNodo de Go", tipoNodo)
			continue
		}
		for _, p := range pares {
			k, d := p[1], p[2]
			if goD, ok := goTabla[k]; !ok || goD != d {
				t.Errorf("nodo %s campo %s: Go=%s, TS=%s", tipoNodo, k, goD, d)
			}
		}
	}

	// Comprobación por reflexión: ningún campo obligatorio debe llevar omitempty ni omitzero
	comprobarStruct := func(nombre string, st reflect.Type, tabla map[string]string) {
		tagsPorJSON := make(map[string]string)
		for i := 0; i < st.NumField(); i++ {
			f := st.Field(i)
			tag := f.Tag.Get("json")
			if tag != "" && tag != "-" {
				partes := strings.Split(tag, ",")
				tagsPorJSON[partes[0]] = tag
			}
		}
		for k, d := range tabla {
			if strings.HasSuffix(d, "!") {
				tag, ok := tagsPorJSON[k]
				if !ok {
					// Si es unión aplanada como en Nodo, algunos campos están en la tabla del tipo
					continue
				}
				if strings.Contains(tag, "omitempty") || strings.Contains(tag, "omitzero") {
					t.Errorf("struct %s campo %s es obligatorio (%s) pero lleva omit: %s", nombre, k, d, tag)
				}
			}
		}
	}

	comprobarStruct("Sitio", reflect.TypeOf(ir.Sitio{}), ir.CamposObjeto["sitio"])
	comprobarStruct("Tema", reflect.TypeOf(ir.Tema{}), ir.CamposObjeto["tema"])
	comprobarStruct("Pagina", reflect.TypeOf(ir.Pagina{}), ir.CamposObjeto["pagina"])
	comprobarStruct("Layout", reflect.TypeOf(ir.Layout{}), ir.CamposObjeto["layout"])
	comprobarStruct("Borde", reflect.TypeOf(ir.Borde{}), ir.CamposObjeto["borde"])
	comprobarStruct("FondoImagen", reflect.TypeOf(ir.FondoImagen{}), ir.CamposObjeto["fondo_imagen"])
}
