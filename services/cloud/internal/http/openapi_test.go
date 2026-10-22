// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	cloud "git.sytue.eu/sytue/one/services/cloud"
)

// TestOpenAPISpecEmbebida comprueba que el contrato viaja DENTRO del binario y no
// es un fichero que se pueda quedar fuera de la imagen: si el embed fallara, la
// spec quedaría vacía y el endpoint serviría un documento inútil sin avisar.
func TestOpenAPISpecEmbebida(t *testing.T) {
	if len(cloud.OpenAPISpec) == 0 {
		t.Fatal("la spec embebida está vacía")
	}
	head := string(cloud.OpenAPISpec[:min(200, len(cloud.OpenAPISpec))])
	if !strings.Contains(head, "openapi:") {
		t.Errorf("no parece un documento OpenAPI; empieza por: %q", head)
	}
	// El contrato tiene que describir rutas: una spec sin `paths` no genera SDK.
	if !strings.Contains(string(cloud.OpenAPISpec), "paths:") {
		t.Error("la spec no declara `paths`")
	}

	// Y tiene que PARSEAR. Buscar «paths:» dentro del texto no comprueba nada de
	// la estructura: el contrato estuvo publicado semanas siendo YAML inválido
	// —la definición de un campo pegada dentro de un bloque `example:`— y este
	// test seguía en verde, porque las dos cadenas de arriba estaban donde debían.
	// Quien lo carga (Swagger UI, redocly, un generador de SDK) no busca cadenas:
	// lo parsea, y se encontraba un error de sintaxis.
	var doc struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(cloud.OpenAPISpec, &doc); err != nil {
		t.Fatalf("la spec embebida no es YAML válido: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Error("la spec parsea pero no declara ni una ruta")
	}
}

func TestServeOpenAPI(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	h.ServeOpenAPI(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	if ct := rec.Header().Get("content-type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("content-type = %q, quiero application/yaml", ct)
	}
	// Cacheable y PÚBLICO: lo consumen generadores de SDK sin credenciales.
	if cc := rec.Header().Get("cache-control"); !strings.Contains(cc, "public") {
		t.Errorf("cache-control = %q, quiero que sea público", cc)
	}
	if rec.Body.Len() != len(cloud.OpenAPISpec) {
		t.Errorf("se sirvieron %d bytes de %d", rec.Body.Len(), len(cloud.OpenAPISpec))
	}
}
