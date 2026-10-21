// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNotaCifradaIncluyeDatosAuxiliaresSinCopiaPublica(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	original := json.RawMessage(`{"attachments":[{"name":"private.pdf","dataUrl":"data:application/pdf;base64,U0VDUkVUTw=="}],"url":"https://private.invalid/path","icon":"key","unknown":{"keep":[1,2]},"client_id":"opaque","carry_count":4}`)
	in := NoteInput{Title: "secret title", Content: "<p>secret body</p>", Data: original}
	if err := CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(in)
	for _, secret := range []string{"private.pdf", "data:application", "private.invalid", "unknown", "secret title", "secret body"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("dato fuera del sobre: %s", secret)
		}
	}
	if in.CipherVersion != 2 || string(in.Data) != `{"client_id":"opaque"}` {
		t.Fatalf("escritura pública: %s v%d", in.Data, in.CipherVersion)
	}
	claro, _, err := AbrirContenidoNota(key, Note{ContentCipher: in.ContentCipher, Data: json.RawMessage(`{"client_id":"opaque","carry_count":5}`)})
	if err != nil || claro.Title != "secret title" || !bytes.Contains(claro.Data, []byte("private.pdf")) || !bytes.Contains(claro.Data, []byte(`"carry_count":5`)) {
		t.Fatalf("restauración: %+v, %v", claro, err)
	}
}

func TestLegadoNotasNoSePierdeNiSeSobrescribe(t *testing.T) {
	key := bytes.Repeat([]byte{8}, 32)
	old, _ := EmpaquetarNota(key, NotaEnClaro{Title: "old", Content: "<p>body</p>"})
	legacy := Note{ContentCipher: old, Data: json.RawMessage(`{"attachments":[{"dataUrl":"data:secret"}],"future":42}`)}
	c, m, err := AbrirContenidoNota(key, legacy)
	if err != nil || !m || !bytes.Contains(c.Data, []byte("data:secret")) {
		t.Fatalf("legado perdido: %+v %v %v", c, m, err)
	}
	plain := Note{Title: "historic title", Content: "historic body", Data: legacy.Data}
	c, m, err = AbrirContenidoNota(key, plain)
	if err != nil || !m || c.Title != plain.Title || !bytes.Equal(c.Data, plain.Data) {
		t.Fatal("toggle cifrado perdió históricos")
	}
	both, _ := EmpaquetarNota(key, NotaEnClaro{Data: json.RawMessage(`{"future":43}`)})
	legacy.ContentCipher = both
	if _, _, err = AbrirContenidoNota(key, legacy); err == nil {
		t.Fatal("conflicto de dos datos fue sobrescrito")
	}
	if !bytes.Contains(legacy.Data, []byte(`42`)) {
		t.Fatal("error modificó el original")
	}
}

func TestCifrarDatosCincoMiBNoExpandeHTMLNiPierdeUnicode(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	// Un valor válido de casi5MiB con caracteres que JSON.Marshal escapa seis veces.
	value := strings.Repeat("<>&", (5<<20)/3-64)
	// Simula el JSON legacy original sin escapes: el límite del servicio aplica a bytes.
	data := []byte(`{"value":"` + value + `"}`)
	in := NoteInput{Data: data}
	if err := CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	if len(in.ContentCipher) > 8<<20 {
		t.Fatalf("escape HTML infló el sobre: %d", len(in.ContentCipher))
	}
	claro, err := DesempaquetarNota(key, in.ContentCipher)
	if err != nil {
		t.Fatal(err)
	}
	var restored map[string]string
	if json.Unmarshal(claro.Data, &restored) != nil || restored["value"] != value {
		t.Fatal("dato grande cambiado")
	}
	too := NoteInput{Data: json.RawMessage(`{"value":"` + strings.Repeat("a", 13<<20) + `"}`)}
	before := bytes.Clone(too.Data)
	if CifrarContenidoNota(key, &too) == nil || !bytes.Equal(too.Data, before) || too.ContentCipher != "" {
		t.Fatal("límite descartó original o aceptó sobre excesivo")
	}
}

func TestMigracionCifradaUsaCASCompletoYNoRepiteUnFallo(t *testing.T) {
	calls := 0
	crypto := json.RawMessage(`{"v":1,"salt":"public","iterations":600000,"check":"encv1:public","extra":"preserved"}`)
	now := time.Date(2026, 10, 20, 12, 0, 0, 123456000, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/notes/id/encrypted-content" {
			t.Errorf("ruta %s %s", r.Method, r.URL.Path)
		}
		var in struct {
			CipherVersion     int             `json:"cipher_version"`
			ExpectedUpdatedAt time.Time       `json:"expected_updated_at"`
			ExpectedCrypto    json.RawMessage `json:"expected_crypto"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.CipherVersion != 2 || !in.ExpectedUpdatedAt.Equal(now) {
			t.Error("CAS incompleto")
		}
		var want, got any
		_ = json.Unmarshal(crypto, &want)
		_ = json.Unmarshal(in.ExpectedCrypto, &got)
		if !reflectEqual(want, got) {
			t.Error("crypto parcial")
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"note_changed"}}`))
	}))
	defer srv.Close()
	_, err := NewNotes(srv.URL, nil, nil).MigrateEncryptedContent(context.Background(), "id", NoteCipherMigration{CipherVersion: 2, ExpectedUpdatedAt: now, ExpectedCrypto: crypto, ContentCipher: "encv1:cipher"})
	if err == nil || calls != 1 {
		t.Fatalf("conflicto repetido: %d %v", calls, err)
	}
}

func reflectEqual(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(aa, bb)
}

func TestDatosJSONGrandesConservanPrecisionAlMigrar(t *testing.T) {
	key := bytes.Repeat([]byte{10}, 32)
	cipher, _ := EmpaquetarNota(key, NotaEnClaro{Title: "x", Content: "y"})
	n := Note{ContentCipher: cipher, Data: json.RawMessage(`{"future":9007199254740993123456789,"fraction":1.234567890123456789,"client_id":"opaque"}`)}
	c, _, err := AbrirContenidoNota(key, n)
	if err != nil || !bytes.Contains(c.Data, []byte("9007199254740993123456789")) || !bytes.Contains(c.Data, []byte("1.234567890123456789")) {
		t.Fatalf("JSON desconocido redondeado: %s %v", c.Data, err)
	}
}

func TestSobresAntiguosYNuevosConOverlaySonCompatibles(t *testing.T) {
	key := bytes.Repeat([]byte{11}, 32)
	old, _ := CifrarTextoNotas(key, `{"title":"old","content":"body","data":{"legacy":true}}`)
	legacy, err := DesempaquetarNota(key, old)
	if err != nil || !bytes.Contains(legacy.Data, []byte(`"legacy":true`)) {
		t.Fatalf("sobre antiguo ilegible: %+v %v", legacy, err)
	}
	raw := `{"future":9007199254740993123456789,"drawing":{"ratio":1.234567890123456789},"icon":"old","remove":"yes"}`
	stored, _ := json.Marshal(map[string]any{"title": "new", "content": "body", "data_json": raw, "data_patch": map[string]any{"icon": "new"}, "data_removed": []string{"remove"}})
	cipher, _ := CifrarTextoNotas(key, string(stored))
	claro, err := DesempaquetarNota(key, cipher)
	if err != nil || !bytes.Contains(claro.Data, []byte("9007199254740993123456789")) || !bytes.Contains(claro.Data, []byte("1.234567890123456789")) || !bytes.Contains(claro.Data, []byte(`"icon":"new"`)) || bytes.Contains(claro.Data, []byte(`"remove"`)) {
		t.Fatalf("overlay pierde datos: %s %v", claro.Data, err)
	}
	sealed, err := EmpaquetarNota(key, claro)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := DesempaquetarNota(key, sealed)
	if err != nil || !bytes.Equal(reopened.Data, claro.Data) {
		t.Fatal("edición Fyne redondea datos ajenos")
	}
}

func TestNotasPaginadasRestauranOrdenYDatosJSONExactos(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("raw_data") != "1" {
			t.Error("lectura sin límite/raw")
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"notes":[{"id":"a","position":9,"data_json":"{\"future\":9007199254740993123456789}"}],"next_cursor":"a"}`))
			return
		}
		if r.URL.Query().Get("cursor") != "a" {
			t.Error("cursor no conservado")
		}
		_, _ = w.Write([]byte(`{"notes":[{"id":"b","position":1,"data_json":"{}"}]}`))
	}))
	defer srv.Close()
	rows, err := NewNotes(srv.URL, nil, nil).ListNotesForEncryption(context.Background(), "nb")
	if err != nil || calls != 2 || len(rows) != 2 || rows[0].ID != "b" || !bytes.Contains(rows[1].Data, []byte("9007199254740993123456789")) {
		t.Fatalf("página pierde datos/orden: %+v %v", rows, err)
	}
}

func TestDatoNuloHistoricoExigeMigrarSinDestruirElSobre(t *testing.T) {
	key := bytes.Repeat([]byte{12}, 32)
	cipher, _ := EmpaquetarNota(key, NotaEnClaro{Title: "kept", Content: "body"})
	for _, data := range []json.RawMessage{nil, json.RawMessage(`null`)} {
		claro, migrar, err := AbrirContenidoNota(key, Note{ContentCipher: cipher, Data: data})
		if err != nil || !migrar || claro.Title != "kept" {
			t.Fatal("NULL queda bloqueado sin ruta de migración")
		}
	}
}
