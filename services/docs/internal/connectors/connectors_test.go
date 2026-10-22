// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// --- Guard SQL: solo un SELECT de solo lectura ---

func TestGuardSelectAccepts(t *testing.T) {
	ok := []string{
		"SELECT 1",
		"  select * from users where id = $1  ",
		"SELECT 1;",         // un ';' final admisible
		"SELECT ';' AS sep", // ';' dentro de literal: no cuenta
		"WITH a AS (SELECT 1) SELECT * FROM a",
		"-- comentario\nSELECT 1",
		"/* bloque */ SELECT 1",
		"SELECT \"col;name\" FROM t", // ';' en identificador entre comillas
	}
	for _, s := range ok {
		if err := guardSelect(s); err != nil {
			t.Errorf("guardSelect(%q) = %v; se esperaba nil", s, err)
		}
	}
}

func TestGuardSelectRejects(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"DELETE FROM users",
		"UPDATE users SET x = 1",
		"INSERT INTO t VALUES (1)",
		"DROP TABLE users",
		"TRUNCATE t",
		"CREATE TABLE t (id int)",
		"SELECT 1; DROP TABLE users", // sentencias apiladas
		"SELECT 1; SELECT 2",         // dos statements
		"select 1; delete from t;",   // apiladas con ';' final
		"GRANT ALL ON t TO public",
		"COPY t FROM '/etc/passwd'",
	}
	for _, s := range bad {
		if err := guardSelect(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("guardSelect(%q) = %v; se esperaba ErrInvalid", s, err)
		}
	}
}

// --- Saneado de config ---

func TestSanitizeConfigREST(t *testing.T) {
	// Válida: api_key en header.
	raw := json.RawMessage(`{"base_url":"https://api.example.com/v1","auth":{"type":"api_key","in":"header","name":"X-Api-Key"},"junk":"drop"}`)
	out, err := SanitizeConfig(KindREST, raw)
	if err != nil {
		t.Fatalf("SanitizeConfig REST válida: %v", err)
	}
	var c RestConfig
	if err := json.Unmarshal(out, &c); err != nil {
		t.Fatalf("unmarshal salida: %v", err)
	}
	if c.BaseURL != "https://api.example.com/v1" || c.Auth.Name != "X-Api-Key" {
		t.Fatalf("config no canónica: %+v", c)
	}

	bad := []json.RawMessage{
		json.RawMessage(`{"base_url":"ftp://x/y"}`),                                                  // esquema no http
		json.RawMessage(`{"base_url":"https://x","auth":{"type":"api_key"}}`),                        // api_key sin in/name
		json.RawMessage(`{"base_url":"https://x","auth":{"type":"api_key","in":"body","name":"k"}}`), // in inválido
		json.RawMessage(`{"base_url":""}`),                                                           // sin URL
		json.RawMessage(`{"base_url":"https://x","auth":{"type":"weird"}}`),                          // auth desconocida
	}
	for _, b := range bad {
		if _, err := SanitizeConfig(KindREST, b); !errors.Is(err, ErrInvalid) {
			t.Errorf("SanitizeConfig REST(%s) = %v; se esperaba ErrInvalid", b, err)
		}
	}
}

func TestSanitizeConfigPostgres(t *testing.T) {
	raw := json.RawMessage(`{"host":"db.example.com","database":"app","user":"reader"}`)
	out, err := SanitizeConfig(KindPostgres, raw)
	if err != nil {
		t.Fatalf("SanitizeConfig PG válida: %v", err)
	}
	var c PostgresConfig
	_ = json.Unmarshal(out, &c)
	if c.Port != 5432 || c.SSLMode != "prefer" {
		t.Fatalf("defaults no aplicados: %+v", c)
	}
	bad := []json.RawMessage{
		json.RawMessage(`{"database":"app","user":"r"}`),                           // sin host
		json.RawMessage(`{"host":"h","user":"r"}`),                                 // sin database
		json.RawMessage(`{"host":"h","database":"a"}`),                             // sin user
		json.RawMessage(`{"host":"h","database":"a","user":"r","sslmode":"nope"}`), // sslmode inválido
		json.RawMessage(`{"host":"h","database":"a","user":"r","port":70000}`),     // puerto fuera de rango
	}
	for _, b := range bad {
		if _, err := SanitizeConfig(KindPostgres, b); !errors.Is(err, ErrInvalid) {
			t.Errorf("SanitizeConfig PG(%s) = %v; se esperaba ErrInvalid", b, err)
		}
	}
}

// El destino de una conexión es a dónde viaja su credencial guardada: cambia con la URL
// base, el tipo de autenticación, la URL del token, el host, el puerto o el modo TLS, y
// con nada más (quien edita el resto no tiene que volver a escribir el secreto).
func TestDestino(t *testing.T) {
	canon := func(k Kind, raw string) json.RawMessage {
		t.Helper()
		out, err := SanitizeConfig(k, json.RawMessage(raw))
		if err != nil {
			t.Fatalf("SanitizeConfig(%s): %v", raw, err)
		}
		return out
	}
	const rest = `{"base_url":"https://api.example.com/v1","auth":{"type":"bearer"}}`
	const pg = `{"host":"db.example.com","database":"app","user":"reader"}`
	casos := []struct {
		nombre         string
		kind           Kind
		antes, despues string
		cambia         bool
	}{
		{"REST: cabeceras", KindREST, rest, `{"base_url":"https://api.example.com/v1","auth":{"type":"bearer"},"headers":{"X-A":"1"}}`, false},
		{"REST: otra URL base", KindREST, rest, `{"base_url":"https://otro.example.org/v1","auth":{"type":"bearer"}}`, true},
		{"REST: otro tipo de autenticación", KindREST, rest, `{"base_url":"https://api.example.com/v1","auth":{"type":"basic","username":"u"}}`, true},
		{"REST: URL del token", KindREST, rest, `{"base_url":"https://api.example.com/v1","auth":{"type":"oauth2_client_credentials","token_url":"https://id.example.org/token","client_id":"c"}}`, true},
		{"PostgreSQL: otra base y otro usuario", KindPostgres, pg, `{"host":"db.example.com","database":"otra","user":"otro"}`, false},
		{"PostgreSQL: otro host", KindPostgres, pg, `{"host":"db.example.org","database":"app","user":"reader"}`, true},
		{"PostgreSQL: otro puerto", KindPostgres, pg, `{"host":"db.example.com","port":5433,"database":"app","user":"reader"}`, true},
		{"PostgreSQL: sin TLS", KindPostgres, pg, `{"host":"db.example.com","database":"app","user":"reader","sslmode":"disable"}`, true},
	}
	for _, c := range casos {
		if got := Destino(c.kind, canon(c.kind, c.antes)) != Destino(c.kind, canon(c.kind, c.despues)); got != c.cambia {
			t.Errorf("%s: el destino cambia = %v, esperaba %v", c.nombre, got, c.cambia)
		}
	}
}

func TestValidateName(t *testing.T) {
	if _, err := ValidateName("  Ventas Q3 "); err != nil {
		t.Errorf("nombre válido rechazado: %v", err)
	}
	if _, err := ValidateName("   "); !errors.Is(err, ErrInvalid) {
		t.Errorf("nombre vacío: se esperaba ErrInvalid, got %v", err)
	}
}

func TestSanitizeSpecMismatch(t *testing.T) {
	// Spec REST contra conexión Postgres => inválida.
	if _, err := SanitizeSpec(KindPostgres, json.RawMessage(`{"rest":{"path":"/x"}}`)); !errors.Is(err, ErrInvalid) {
		t.Errorf("spec REST en conexión PG: se esperaba ErrInvalid, got %v", err)
	}
	// Spec Postgres con SQL no-SELECT => inválida.
	if _, err := SanitizeSpec(KindPostgres, json.RawMessage(`{"postgres":{"sql":"DELETE FROM t"}}`)); !errors.Is(err, ErrInvalid) {
		t.Errorf("spec PG con DELETE: se esperaba ErrInvalid, got %v", err)
	}
	// Spec Postgres válida.
	if _, err := SanitizeSpec(KindPostgres, json.RawMessage(`{"postgres":{"sql":"SELECT 1"}}`)); err != nil {
		t.Errorf("spec PG válida rechazada: %v", err)
	}
}

// --- Parseo REST JSON -> tabla ---

func TestJSONToResultArrayOfObjects(t *testing.T) {
	body := []byte(`[{"b":1,"a":"x"},{"a":"y","c":true}]`)
	res, err := jsonToResult(body, "", 100)
	if err != nil {
		t.Fatalf("jsonToResult: %v", err)
	}
	// Columnas unión ordenadas alfabéticamente: a, b, c.
	if len(res.Columns) != 3 || res.Columns[0].Name != "a" || res.Columns[1].Name != "b" || res.Columns[2].Name != "c" {
		t.Fatalf("columnas inesperadas: %+v", res.Columns)
	}
	if res.Columns[0].Type != "text" || res.Columns[1].Type != "number" || res.Columns[2].Type != "boolean" {
		t.Fatalf("tipos inferidos mal: %+v", res.Columns)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("se esperaban 2 filas, got %d", len(res.Rows))
	}
	// Fila 0: a="x", b=1, c=nil (ausente).
	if res.Rows[0][0] != "x" || res.Rows[0][1].(float64) != 1 || res.Rows[0][2] != nil {
		t.Fatalf("fila 0 inesperada: %+v", res.Rows[0])
	}
	if res.Rows[1][2].(bool) != true {
		t.Fatalf("fila 1 c debería ser true: %+v", res.Rows[1])
	}
}

func TestJSONToResultRootPath(t *testing.T) {
	body := []byte(`{"data":{"items":[{"x":1},{"x":2}]}}`)
	res, err := jsonToResult(body, "data.items", 100)
	if err != nil {
		t.Fatalf("jsonToResult root_path: %v", err)
	}
	if len(res.Rows) != 2 || res.Columns[0].Name != "x" {
		t.Fatalf("root_path no navegó bien: %+v", res)
	}
	// Índice de array en el path.
	body2 := []byte(`{"results":[{"rows":[{"k":"v"}]}]}`)
	res2, err := jsonToResult(body2, "results[0].rows", 100)
	if err != nil || len(res2.Rows) != 1 || res2.Columns[0].Name != "k" {
		t.Fatalf("root_path con índice falló: %+v err=%v", res2, err)
	}
	// Path inexistente => ErrInvalid.
	if _, err := jsonToResult(body, "data.nope", 100); !errors.Is(err, ErrInvalid) {
		t.Errorf("root_path inexistente: se esperaba ErrInvalid, got %v", err)
	}
}

func TestJSONToResultScalarsAndSingle(t *testing.T) {
	// Array de escalares => una columna "value".
	res, err := jsonToResult([]byte(`["a","b","c"]`), "", 100)
	if err != nil || len(res.Columns) != 1 || res.Columns[0].Name != "value" || len(res.Rows) != 3 {
		t.Fatalf("array escalar: %+v err=%v", res, err)
	}
	// Objeto único => una fila.
	res2, err := jsonToResult([]byte(`{"a":1,"b":2}`), "", 100)
	if err != nil || len(res2.Rows) != 1 || len(res2.Columns) != 2 {
		t.Fatalf("objeto único: %+v err=%v", res2, err)
	}
}

func TestJSONToResultRowCap(t *testing.T) {
	res, err := jsonToResult([]byte(`[{"x":1},{"x":2},{"x":3}]`), "", 2)
	if err != nil {
		t.Fatalf("jsonToResult: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("tope de filas no aplicado: %d", len(res.Rows))
	}
}

// --- Construcción de URL REST (sin red) ---

func TestBuildRestURL(t *testing.T) {
	cfg := RestConfig{
		BaseURL: "https://api.example.com/v1/",
		Auth:    RestAuth{Type: RestAuthAPIKey, In: "query", Name: "key"},
	}
	u, err := buildRestURL(cfg, Credentials{Secret: "SEKRET"}, RestQuery{Path: "users", Query: map[string]string{"page": "2"}})
	if err != nil {
		t.Fatalf("buildRestURL: %v", err)
	}
	// El host no cambia; se añaden page y la api_key.
	if got := u; got != "https://api.example.com/v1/users?key=SEKRET&page=2" {
		t.Fatalf("URL inesperada: %s", got)
	}
	// Una ruta absoluta (intento de cambiar de host) se rechaza.
	if _, err := buildRestURL(cfg, Credentials{}, RestQuery{Path: "https://evil.example.net/steal"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("ruta absoluta: se esperaba ErrInvalid, got %v", err)
	}
	// Ruta protocolo-relativa //evil también se rechaza.
	if _, err := buildRestURL(cfg, Credentials{}, RestQuery{Path: "//evil.example.net/x"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("ruta //host: se esperaba ErrInvalid, got %v", err)
	}
}

// --- Transform ---

func TestApplyTransform(t *testing.T) {
	res := &Result{
		Columns: []Column{{Name: "id", Type: "number"}, {Name: "name", Type: "text"}, {Name: "role", Type: "text"}},
		Rows: [][]any{
			{float64(1), "Ana", "admin"},
			{float64(2), "Beto", "user"},
			{float64(3), "Cid", "admin"},
		},
	}
	tr := &Transform{
		Filter: []ColumnFilter{{Column: "role", Op: "eq", Value: "admin"}},
		Select: []string{"name", "id"},
		Rename: map[string]string{"name": "nombre"},
	}
	out := applyTransform(res, tr)
	if len(out.Columns) != 2 || out.Columns[0].Name != "nombre" || out.Columns[1].Name != "id" {
		t.Fatalf("columnas transform: %+v", out.Columns)
	}
	if len(out.Rows) != 2 { // solo admins
		t.Fatalf("filtro no aplicado: %d filas", len(out.Rows))
	}
	if out.Rows[0][0] != "Ana" || out.Rows[0][1].(float64) != 1 {
		t.Fatalf("proyección/orden mal: %+v", out.Rows[0])
	}
}

// --- Sealer (crypto) ida y vuelta ---

func TestSealerRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	s, err := NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	blob, err := s.Seal([]byte("s3cr3t-token"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if string(blob) == "s3cr3t-token" {
		t.Fatal("el blob no debe contener el texto en claro")
	}
	got, err := s.Open(blob)
	if err != nil || string(got) != "s3cr3t-token" {
		t.Fatalf("Open: got %q err %v", got, err)
	}
	// Sin clave: Seal/Open dan ErrNoCipher.
	var nilS *Sealer
	if _, err := nilS.Seal([]byte("x")); !errors.Is(err, ErrNoCipher) {
		t.Errorf("Seal sin clave: se esperaba ErrNoCipher, got %v", err)
	}
	// Clave de longitud inválida.
	if _, err := NewSealer([]byte("short")); err == nil {
		t.Error("NewSealer con clave corta debería fallar")
	}
}

// TestCSVToResult: un CSV de Excel en español (BOM, «;», comillas con separador dentro,
// una fila corta y una cabecera vacía) sale como tabla de texto; y el tipo lo decide la
// cabecera Content-Type o, con texto que no es JSON, el propio contenido.
func TestCSVToResult(t *testing.T) {
	body := []byte("\xef\xbb\xbfZona;Importe;\r\nNorte;\"1.234,5\";x\r\n\"Sur; este\";7\r\n")
	res, err := bodyToResult(body, "text/csv; charset=utf-8", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(res.Columns); got != "[{Zona text} {Importe text} {column3 text}]" {
		t.Errorf("columnas %s", got)
	}
	if got := fmt.Sprint(res.Rows); got != "[[Norte 1.234,5 x] [Sur; este 7 ]]" {
		t.Errorf("filas %s", got)
	}
	if res, _ := bodyToResult([]byte("a,b\n1,2\n3,4\n"), "text/plain", "", 1); len(res.Rows) != 1 || res.Columns[1].Name != "b" {
		t.Errorf("texto plano con comas: %+v", res)
	}
	if res, err := bodyToResult([]byte(`[{"a":1}]`), "text/plain", "", 10); err != nil || res.Columns[0].Type != "number" {
		t.Errorf("un JSON servido como texto se lee como JSON: %+v %v", res, err)
	}
	if _, err := bodyToResult([]byte("<html>"), "application/json", "", 10); !errors.Is(err, ErrUpstream) {
		t.Errorf("lo que dice ser JSON y no lo es sigue fallando: %v", err)
	}
}
