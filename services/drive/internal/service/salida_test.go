// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// El almacén de terceros lo configura un administrador de la organización: su
// dirección es entrada de fuera. Un WebDAV que apunta a una IP que no es pública
// no se llega a conectar (lo corta la barrera SSRF, no un fallo de certificado), y
// un S3 sin TLS no se abre.
func TestElAlmacenDeTercerosNoSaleHaciaDentro(t *testing.T) {
	pedido := false
	local := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pedido = true
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer local.Close()

	w, err := objectstore.NewWebDAV(local.URL, "", "usuario", "clave")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Probar(context.Background()); !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Errorf("WebDAV hacia una IP local: %v, esperaba que lo cortara la barrera SSRF", err)
	}
	if _, err := w.Get(context.Background(), tenancy.Tenant{Slug: "acme"}, "ext/files/x"); !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Errorf("leer un blob de un WebDAV en una IP local: %v, esperaba que lo cortara la barrera SSRF", err)
	}
	if pedido {
		t.Error("se llegó a pedir algo al servidor local")
	}

	r := &blobRouter{}
	if _, err := r.abrir(tenancy.Tenant{Slug: "acme"}, store.ExternalStorage{
		Kind: "s3", Endpoint: "minio.interno:9000", Bucket: "datos", AccessKey: "a", UseSSL: false,
	}, "s"); !errors.Is(err, ErrDestinoNoPermitido) {
		t.Errorf("abrir un almacén S3 sin TLS: %v, esperaba ErrDestinoNoPermitido", err)
	}
	// Con TLS se abre, y su conexión pasa por la misma barrera.
	b, err := r.abrir(tenancy.Tenant{Slug: "acme"}, store.ExternalStorage{
		Kind: "s3", Endpoint: "127.0.0.1:9000", Bucket: "datos", AccessKey: "a", UseSSL: true,
	}, "s")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.(blobStat).Healthz(context.Background()); !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Errorf("S3 hacia una IP local: %v, esperaba que lo cortara la barrera SSRF", err)
	}
}
