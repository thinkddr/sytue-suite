// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/config"
)

// TestSecretoOpcional: en dev cae al de desarrollo; fuera, vacío se queda vacío (la
// puerta no abre) y el de desarrollo se rechaza nombrando la variable.
func TestSecretoOpcional(t *testing.T) {
	const dev = "de-desarrollo"
	l := config.NewLoader("")
	if v := l.SecretoOpcional(config.EnvDev, "PRUEBA_SECRETO_OPCIONAL", dev); v != dev || l.Err() != nil {
		t.Fatalf("dev sin variable = %q, %v", v, l.Err())
	}
	if v := l.SecretoOpcional(config.EnvProd, "PRUEBA_SECRETO_OPCIONAL", dev); v != "" || l.Err() != nil {
		t.Fatalf("prod sin variable = %q, %v", v, l.Err())
	}
	t.Setenv("PRUEBA_SECRETO_OPCIONAL", dev)
	l.SecretoOpcional(config.EnvProd, "PRUEBA_SECRETO_OPCIONAL", dev)
	if l.Err() == nil || !strings.Contains(l.Err().Error(), "PRUEBA_SECRETO_OPCIONAL") {
		t.Fatalf("prod con el de desarrollo = %v", l.Err())
	}
}

// Docs y Drive reciben el MISMO Secret de la papelera compartida, que genera el
// chart paraguas. Si uno se queda sin él, su puerta cierra y la otra falla contra
// ella sin que nada lo diga.
func TestDocsYDriveRecibenElSecretoDeLaPapelera(t *testing.T) {
	raiz := filepath.Join("..", "..", "..")
	valores, err := os.ReadFile(filepath.Join(raiz, "deploy", "helm", "sytue-one", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^ {2}sytue-docs-drive-internal: secret$`).Match(valores) {
		t.Error("el chart paraguas no genera sytue-docs-drive-internal")
	}
	for chart, variable := range map[string]string{
		"docs":  "SYTUE_DOCS_DRIVE_INTERNAL_SECRET",
		"drive": "DOCS_INTERNAL_SECRET",
	} {
		d, err := os.ReadFile(filepath.Join(raiz, "deploy", "helm", "charts", chart, "templates", "deployment.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		patron := regexp.MustCompile(`- name: ` + variable +
			`\s+valueFrom:\s+secretKeyRef:\s+name: sytue-docs-drive-internal\s+key: secret`)
		if !patron.Match(d) {
			t.Errorf("el chart %s no pasa %s desde sytue-docs-drive-internal", chart, variable)
		}
	}
}
