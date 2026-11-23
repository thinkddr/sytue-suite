// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// El diagnóstico se prueba antes de proyectar un registro real del bot.

const renovateHTTPDiagnosticoRegistro = `{"msg":"synthetic-secret","err":{"statusCode":404,"options":{"method":"GET","url":"https://api.github.com/repos/private-owner/private-repo/pulls/1260/reviews?token=synthetic-secret#synthetic-secret","headers":{"authorization":"Bearer synthetic-secret"}},"response":{"body":"synthetic-secret"}}}
{"err":{"status":403,"options":{"method":"POST","url":"https://api.github.com/repos/private-owner/private-repo/branches/synthetic-secret/protection"}}}
{"err":{"statusCode":401,"options":{"method":"synthetic-secret","url":"https://user:synthetic-secret@api.github.com/repos/private-owner/private-repo/issues/1"}}}
{"statusCode":200,"url":"https://api.github.com/repos/private-owner/private-repo/issues/1"}
`

func TestRenovateHTTPDiagnosticoSinCredencialesMensajesContenidoNiIdentificadores(t *testing.T) {
	registro := filepath.Join(t.TempDir(), "http-log")
	guiontest.Escribir(t, registro, renovateHTTPDiagnosticoRegistro)
	r := guiontest.Guion(t, guiontest.Opciones{}, "scripts/renovate-resultado.sh", "--diagnostico", registro)
	if r.Codigo != 0 {
		t.Fatalf("el diagnóstico salió con %d: %s", r.Codigo, r.Error)
	}
	var filas []map[string]any
	if err := json.Unmarshal([]byte(r.Salida), &filas); err != nil {
		t.Fatalf("el diagnóstico no es un array JSON de objetos: %v\n%s", err, r.Salida)
	}
	if len(filas) != 3 {
		t.Fatalf("el diagnóstico tiene %d filas y se esperaban 3: %s", len(filas), r.Salida)
	}
	hay := func(http float64, metodos, rutas []any) bool {
		return slices.ContainsFunc(filas, func(f map[string]any) bool {
			return reflect.DeepEqual(f["http"], []any{http}) &&
				reflect.DeepEqual(f["methods"], metodos) && reflect.DeepEqual(f["routes"], rutas)
		})
	}
	if !hay(404, []any{"GET"}, []any{"/repos/*/*/pulls/:id/reviews"}) {
		t.Errorf("falta el 404 de GET con su ruta sin identificadores: %s", r.Salida)
	}
	if !hay(403, []any{"POST"}, []any{"/repos/*/*/branches/*/protection"}) {
		t.Errorf("falta el 403 de POST con su ruta sin identificadores: %s", r.Salida)
	}
	if !hay(401, []any{}, []any{}) {
		t.Errorf("falta el 401 sin método ni ruta (ni uno ni otra eran de fiar): %s", r.Salida)
	}
	for _, f := range filas {
		if !reflect.DeepEqual(claves(f), []string{"http", "methods", "routes"}) {
			t.Errorf("una fila lleva campos de más o de menos: %v", claves(f))
		}
	}
	if prohibido := regexp.MustCompile(`synthetic-secret|private-owner|private-repo|authorization|token|Bearer|1260`).FindString(r.Salida); prohibido != "" {
		t.Errorf("el diagnóstico deja pasar %q: %s", prohibido, r.Salida)
	}
}
