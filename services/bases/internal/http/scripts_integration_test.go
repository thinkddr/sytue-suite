// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package baseshttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

func codigoDe(w *httptest.ResponseRecorder) string {
	var o struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &o)
	return o.Error.Code
}

// canonicoDe pasa un manifiesto por el validador y lo devuelve en su forma canónica: lo
// que guarda Postgres es jsonb, que no conserva el orden de las claves.
func canonicoDe(t *testing.T, crudo json.RawMessage) string {
	t.Helper()
	m, err := domain.ParseManifiesto(crudo)
	if err != nil {
		t.Fatalf("manifiesto devuelto no válido: %v (%s)", err, crudo)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// El ciclo entero por HTTP, con los estados que ve el cliente: 422 para un manifiesto
// que no vale, 404 para lo que no existe PARA QUIEN PREGUNTA, 403 sin aprobar o sin
// permiso, 409 en una transición o un hash que no casan, y nunca un 500.
func TestScriptsPorHTTP(t *testing.T) {
	srv := servidor(t)
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Contactos"}`))
	llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"bea","role":"viewer"}`)
	ruta := "/bases/" + base + "/scripts"
	exigir := func(w *httptest.ResponseRecorder, estado int, que string) {
		t.Helper()
		if w.Code != estado {
			t.Fatalf("%s: %d, esperado %d: %s", que, w.Code, estado, w.Body)
		}
	}

	// Lo que no entra: manifiestos fuera de la superficie, código vacío o enorme, ámbito inventado.
	for que, cuerpo := range map[string]string{
		"operación fuera de la superficie": `{"name":"G","code":"x","manifest":{"v":1,"ops":["permissions.grant"],"write":true}}`,
		"escribe sin declararlo":           `{"name":"G","code":"x","manifest":{"v":1,"ops":["records.delete"],"write":false}}`,
		"campo de más en el manifiesto":    `{"name":"G","code":"x","manifest":{"v":1,"write":false,"network":true}}`,
		"sin código":                       `{"name":"G","code":"","manifest":{"v":1,"write":false}}`,
		"código enorme":                    `{"name":"G","code":"` + strings.Repeat("x", 64*1024+1) + `","manifest":{"v":1,"write":false}}`,
		"sin nombre":                       `{"name":" ","code":"x","manifest":{"v":1,"write":false}}`,
		"ámbito inventado":                 `{"name":"G","scope":"org","code":"x","manifest":{"v":1,"write":false}}`,
		"tabla de ninguna base":            `{"name":"G","code":"x","manifest":{"v":1,"write":false,"tables":["11111111-1111-4111-8111-111111111111"]}}`,
	} {
		if w := llamar(t, srv, "ana", "acme", "POST", ruta, cuerpo); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, esperado 422: %s", que, w.Code, w.Body)
		}
	}

	// Bea (lectora) crea el suyo, compartido, que escribe en la tabla.
	w := llamar(t, srv, "bea", "acme", "POST", ruta, `{"name":"Altas","scope":"shared","code":"await sytue.v1.records.create(T,{})","manifest":{"v":1,"ops":["records.create"],"tables":["`+tabla+`"],"write":true}}`)
	exigir(w, http.StatusCreated, "crear")
	var creado struct {
		Script struct {
			ID string `json:"id"`
		} `json:"script"`
		Version struct {
			CodeHash string          `json:"code_hash"`
			Manifest json.RawMessage `json:"manifest"`
			Status   string          `json:"status"`
		} `json:"version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &creado); err != nil || creado.Version.Status != "draft" {
		t.Fatalf("creado: %s", w.Body)
	}
	// El manifiesto que vuelve es el normalizado: con las cotas por defecto puestas.
	if canonicoDe(t, creado.Version.Manifest) != `{"v":1,"ops":["records.create"],"tables":["`+tabla+`"],"write":true,"max_ops":100,"max_ms":5000}` {
		t.Errorf("manifiesto canónico: %s", creado.Version.Manifest)
	}
	sid := ruta + "/" + creado.Script.ID
	v1 := sid + "/versions/1"

	// La autora es lectora: ni ella ejecuta su propio borrador si escribe (403, anotado).
	w = llamar(t, srv, "bea", "acme", "POST", v1+"/runs", ``)
	exigir(w, http.StatusForbidden, "una lectora ejecuta su script que escribe")
	// Revisar: la autora no; la propietaria sí, con el hash; con otro hash, 409.
	exigir(llamar(t, srv, "ana", "acme", "POST", v1+"/review", `{"decision":"approve","code_hash":"`+creado.Version.CodeHash+`"}`), http.StatusConflict, "aprobar un borrador")
	exigir(llamar(t, srv, "bea", "acme", "POST", v1+"/propose", ``), http.StatusNoContent, "proponer")
	exigir(llamar(t, srv, "bea", "acme", "POST", v1+"/review", `{"decision":"approve","code_hash":"`+creado.Version.CodeHash+`"}`), http.StatusForbidden, "aprobarse a sí misma sin ser propietaria")
	w = llamar(t, srv, "ana", "acme", "POST", v1+"/review", `{"decision":"approve","code_hash":"`+strings.Repeat("a", 64)+`"}`)
	if w.Code != http.StatusConflict || codigoDe(w) != "hash_mismatch" {
		t.Errorf("otro hash: %d %s", w.Code, w.Body)
	}
	exigir(llamar(t, srv, "ana", "acme", "POST", v1+"/review", `{"decision":"si","code_hash":"`+creado.Version.CodeHash+`"}`), http.StatusUnprocessableEntity, "decisión inventada")
	exigir(llamar(t, srv, "ana", "acme", "POST", v1+"/review", `{"decision":"approve","code_hash":"`+creado.Version.CodeHash+`"}`), http.StatusNoContent, "aprobar")

	// La propietaria la ejecuta: recibe el código y el manifiesto autorizados, y la cierra.
	w = llamar(t, srv, "ana", "acme", "POST", v1+"/runs", ``)
	exigir(w, http.StatusCreated, "ejecutar")
	var aut struct {
		RunID    int64           `json:"run_id"`
		Code     string          `json:"code"`
		CodeHash string          `json:"code_hash"`
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &aut); err != nil || aut.RunID == 0 || aut.CodeHash != creado.Version.CodeHash ||
		!strings.Contains(aut.Code, "records.create") || canonicoDe(t, aut.Manifest) != canonicoDe(t, creado.Version.Manifest) {
		t.Fatalf("autorizada: %s", w.Body)
	}
	run := fmt.Sprintf("/bases/%s/script-runs/%d", base, aut.RunID)
	for que, cuerpo := range map[string]string{
		"resultado inventado":          `{"outcome":"running"}`,
		"código de error con texto":    `{"outcome":"error","error_code":"El registro de Ana"}`,
		"duración fuera de rango":      `{"outcome":"ok","duration_ms":999999}`,
		"operaciones fuera de rango":   `{"outcome":"ok","ops":100000}`,
		"código de error interminable": `{"outcome":"error","error_code":"` + strings.Repeat("a", 65) + `"}`,
	} {
		exigir(llamar(t, srv, "ana", "acme", "PUT", run, cuerpo), http.StatusUnprocessableEntity, que)
	}
	exigir(llamar(t, srv, "bea", "acme", "PUT", run, `{"outcome":"ok"}`), http.StatusNotFound, "cerrar la ejecución de otra persona")
	exigir(llamar(t, srv, "ana", "acme", "PUT", run, `{"outcome":"denied","error_code":"sin_permiso","duration_ms":7,"ops":2}`), http.StatusNoContent, "cerrar")
	exigir(llamar(t, srv, "ana", "acme", "PUT", run, `{"outcome":"ok"}`), http.StatusNotFound, "cerrar dos veces")

	// El registro: solo la propietaria; lleva el intento denegado de bea y la ejecución de ana.
	exigir(llamar(t, srv, "bea", "acme", "GET", "/bases/"+base+"/script-runs", ``), http.StatusForbidden, "el registro, para una lectora")
	w = llamar(t, srv, "ana", "acme", "GET", "/bases/"+base+"/script-runs?limit=1", ``)
	var reg struct {
		Runs []struct {
			Sub, Outcome string
			ErrorCode    string `json:"error_code"`
			Ops          int
		}
		Next int64 `json:"next_before"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reg); err != nil || len(reg.Runs) != 1 || reg.Next == 0 ||
		reg.Runs[0].Sub != "ana" || reg.Runs[0].Outcome != "denied" || reg.Runs[0].ErrorCode != "sin_permiso" || reg.Runs[0].Ops != 2 {
		t.Errorf("registro, primera página: %s", w.Body)
	}
	w = llamar(t, srv, "ana", "acme", "GET", fmt.Sprintf("/bases/%s/script-runs?before=%d", base, reg.Next), ``)
	if err := json.Unmarshal(w.Body.Bytes(), &reg); err != nil || len(reg.Runs) != 1 || reg.Runs[0].Sub != "bea" || reg.Runs[0].ErrorCode != "forbidden" {
		t.Errorf("registro, segunda página: %s", w.Body)
	}
	// La cuota: fija, y la denegada de bea no cuenta.
	w = llamar(t, srv, "bea", "acme", "GET", "/script-quota", ``)
	if !strings.Contains(w.Body.String(), `"user_runs":0`) || !strings.Contains(w.Body.String(), `"limits":{"org_runs":600,"user_ms":600000,"user_runs":60}`) {
		t.Errorf("cuota: %s", w.Body)
	}

	// Desde fuera: otra organización, ids mal formados y versiones que no existen son 404.
	for que, r := range map[string][2]string{
		"otra organización lista":       {"GET", ruta},
		"otra organización ve":          {"GET", sid},
		"otra organización ejecuta":     {"POST", v1 + "/runs"},
		"otra organización el registro": {"GET", "/bases/" + base + "/script-runs"},
	} {
		exigir(llamar(t, srv, "gus", "globex", r[0], r[1], ``), http.StatusNotFound, que)
	}
	exigir(llamar(t, srv, "ana", "acme", "GET", ruta+"/no-es-un-id", ``), http.StatusNotFound, "id mal formado")
	exigir(llamar(t, srv, "ana", "acme", "POST", sid+"/versions/0/runs", ``), http.StatusNotFound, "versión cero")
	exigir(llamar(t, srv, "ana", "acme", "POST", sid+"/versions/9/runs", ``), http.StatusNotFound, "versión que no existe")
	exigir(llamar(t, srv, "ana", "acme", "POST", sid+"/versions/1%20OR%201/runs", ``), http.StatusNotFound, "versión que no es un número")
	exigir(llamar(t, srv, "ana", "acme", "PUT", "/bases/"+base+"/script-runs/abc", `{"outcome":"ok"}`), http.StatusNotFound, "ejecución que no es un número")

	// Borrar: la autora; después no existe.
	exigir(llamar(t, srv, "bea", "acme", "DELETE", sid, ``), http.StatusNoContent, "borrar")
	exigir(llamar(t, srv, "ana", "acme", "GET", sid, ``), http.StatusNotFound, "ver tras borrar")
}
