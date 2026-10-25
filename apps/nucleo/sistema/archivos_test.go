// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// Una credencial que la extensión no podría usar —sin https, sin usuario o sin clave— no
// llega al llavero: se rechaza antes, en cualquier sistema.
func TestGuardarCredencialArchivosValida(t *testing.T) {
	for _, c := range []CredencialArchivos{
		{URL: "http://drive.sytue.eu/dav", Usuario: "ana@sytue.eu", Clave: "x"},
		{URL: "https:///dav", Usuario: "ana@sytue.eu", Clave: "x"},
		{URL: "https://drive.sytue.eu/dav", Clave: "x"},
		{URL: "https://drive.sytue.eu/dav", Usuario: "ana@sytue.eu"},
	} {
		c.Identidad = strings.Repeat("a", 64)
		if err := GuardarCredencialArchivos(c); !errors.Is(err, errCredencialArchivos) {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

// La extensión de Swift lee la MISMA entrada: el grupo, el servicio, la cuenta y los
// nombres del JSON tienen que ser los de Proveedor.swift.
func TestLaExtensionLeeLoQueSeGuarda(t *testing.T) {
	swift, err := os.ReadFile("../../fyne/xcode/proveedor-archivos/Proveedor.swift")
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := json.Marshal(CredencialArchivos{})
	var campos map[string]string
	_ = json.Unmarshal(crudo, &campos)
	quiero := make([]string, 0, 3+len(campos))
	quiero = append(quiero, `"`+GrupoLlaveroArchivos+`"`, `"`+servicioArchivos+`"`, `"`+cuentaArchivos+`"`)
	for k := range campos {
		quiero = append(quiero, "let "+k+": String")
	}
	for _, q := range quiero {
		if !strings.Contains(string(swift), q) {
			t.Errorf("Proveedor.swift no dice %s", q)
		}
	}
}
