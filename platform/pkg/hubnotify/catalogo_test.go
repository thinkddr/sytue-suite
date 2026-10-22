// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sinClave son los tipos que hoy se mandan sin título en el catálogo común, con su
// motivo. Se compara por IGUALDAD: si a uno le llega su clave, sobra aquí.
var sinClave = map[string]string{
	"cloud.deploy": "su título lleva el estado y el proyecto («Despliegue fallido: web»); hasta que tenga clave con parámetros, la campana enseña el título en castellano que manda el servicio",
}

// Cada tipo de aviso que un servicio manda al Hub tiene su título en el catálogo
// común (`notifications.kind.<tipo>`): sin él, la campana de la web y la de las
// apps nativas enseñan el título que escribió el servicio, en castellano, a quien
// lee en cualquiera de los 26 idiomas. Les pasó a `cast.report` y `cast.live`.
func TestCadaTipoDeAvisoTieneTitulo(t *testing.T) {
	raiz := filepath.Join("..", "..", "..")
	crudo, err := os.ReadFile(filepath.Join(raiz, "apps", "nucleo", "i18n", "locales", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalogo map[string]any
	if err := json.Unmarshal(crudo, &catalogo); err != nil {
		t.Fatal(err)
	}
	tipo := regexp.MustCompile(`\bType:\s+"([a-z]+(?:\.[a-z_]+)+)"`)
	vistos := map[string]string{}
	err = filepath.WalkDir(filepath.Join(raiz, "services"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range tipo.FindAllSubmatch(b, -1) {
			vistos[string(m[1])] = p
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vistos) < 10 {
		t.Fatalf("solo %d tipos de aviso en los servicios: el patrón ya no los encuentra", len(vistos))
	}
	faltan := map[string]string{}
	for k, p := range vistos {
		if _, ok := catalogo["notifications.kind."+k]; !ok {
			faltan[k] = p
		}
	}
	for k, p := range faltan {
		if _, ok := sinClave[k]; !ok {
			t.Errorf("%s (en %s) no tiene `notifications.kind.%s` en el catálogo común", k, p, k)
		}
	}
	for k := range sinClave {
		if _, ok := faltan[k]; !ok {
			t.Errorf("%s ya tiene título o ya no se manda: sobra en sinClave", k)
		}
	}
}
