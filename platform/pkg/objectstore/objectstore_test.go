// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/minio/minio-go/v7"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestNewValidation(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("esperaba error con endpoint vacío")
	}
	if _, err := New(Config{Endpoint: "minio:9000"}); err == nil {
		t.Fatal("esperaba error sin credenciales")
	}
	if _, err := New(Config{Endpoint: "minio:9000", AccessKey: "ak"}); err == nil {
		t.Fatal("esperaba error con solo access key")
	}
}

func TestNewSuccess(t *testing.T) {
	c, err := New(Config{
		Endpoint:  "s3.fr-par.scw.cloud",
		Region:    "fr-par",
		AccessKey: "ak",
		SecretKey: "sk",
		UseSSL:    true,
		Bucket:    "drive-blobs",
	})
	if err != nil {
		t.Fatalf("New válido: %v", err)
	}
	if c == nil || c.bucket != "drive-blobs" {
		t.Fatalf("cliente mal construido: %+v", c)
	}
}

func TestIsNotFound(t *testing.T) {
	// Un error genérico no es "not found".
	if isNotFound(errors.New("otro")) {
		t.Error("error genérico no debe ser not found")
	}
	// Una ErrorResponse de minio con status 404 sí lo es.
	resp := minio.ErrorResponse{StatusCode: http.StatusNotFound, Code: "NoSuchKey"}
	if !isNotFound(resp) {
		t.Error("404/NoSuchKey debe ser not found")
	}
}

func TestTenantKey(t *testing.T) {
	tn := tenancy.Tenant{Slug: "acme"}
	cases := map[string]string{
		"file.txt":      "tenants/acme/file.txt",
		"/file.txt":     "tenants/acme/file.txt",
		"///a/b.txt":    "tenants/acme/a/b.txt",
		"docs/x/y.docx": "tenants/acme/docs/x/y.docx",
	}
	for in, want := range cases {
		if got := tenantKey(tn, in); got != want {
			t.Errorf("tenantKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTenantKeyRespetaElEspacioCompartido(t *testing.T) {
	acme := tenancy.Tenant{Slug: "acme"}
	if got := tenantKey(acme, "/messages/a.eml"); got != "tenants/acme/messages/a.eml" {
		t.Fatalf("clave propia = %q", got)
	}
	// Una clave compartida NO lleva el prefijo del tenant: es la misma para todos.
	if got := tenantKey(acme, PrefijoCompartido+"mensajes/abc.eml"); got != "shared/mensajes/abc.eml" {
		t.Fatalf("clave compartida = %q", got)
	}
}

func TestComprimirSoloSiCompensa(t *testing.T) {
	texto := bytes.Repeat([]byte("Received: from mx.example.org\r\n"), 200)
	original, comprimido, err := comprimir(bytes.NewReader(texto))
	if err != nil || !bytes.Equal(original, texto) {
		t.Fatalf("original alterado o error: %v", err)
	}
	if comprimido == nil || len(comprimido) >= len(texto) {
		t.Fatalf("texto repetido debería comprimir: %d → %v", len(texto), len(comprimido))
	}
	gz, err := gzip.NewReader(bytes.NewReader(comprimido))
	if err != nil {
		t.Fatal(err)
	}
	vuelta, _ := io.ReadAll(gz)
	if !bytes.Equal(vuelta, texto) {
		t.Fatal("la ida y vuelta por gzip no devuelve el original")
	}
	// Lo ya comprimido (bytes al azar) se guarda tal cual: gzip lo haría crecer.
	azar := make([]byte, 4096)
	_, _ = rand.Read(azar)
	if _, comprimido, _ := comprimir(bytes.NewReader(azar)); comprimido != nil {
		t.Fatal("bytes al azar no deberían marcarse como comprimidos")
	}
}

func TestCodificacionDeAdmiteLasFormasDelProveedor(t *testing.T) {
	for _, meta := range []map[string]string{
		{"Sytue-Codificacion": "gzip"},
		{"X-Amz-Meta-Sytue-Codificacion": "GZIP"},
		{"sytue-codificacion": "gzip"},
	} {
		if got := codificacionDe(meta); got != "gzip" {
			t.Errorf("codificacionDe(%v) = %q", meta, got)
		}
	}
	if got := codificacionDe(map[string]string{"Original-Name": "x"}); got != "" {
		t.Errorf("sin metadato debería ser vacío, got %q", got)
	}
}
