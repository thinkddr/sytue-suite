// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// La clave de un blob viaja con el prefijo del tenant también en WebDAV: un
// mismo NAS puede servir a varios, y cruzar carpetas sería cruzar clientes.
func TestElWebDAVEscapaYPrefijaPorTenant(t *testing.T) {
	w, err := NewWebDAV("https://nas-acme.sytue.eu/", "Sytue Drive", "usuario", "clave")
	if err != nil {
		t.Fatal(err)
	}
	got := w.ruta(tenancy.Tenant{Slug: "acme"}, "files/con espacio/á.txt")
	want := "https://nas-acme.sytue.eu/Sytue%20Drive/tenants/acme/files/con%20espacio/%C3%A1.txt"
	if got != want {
		t.Fatalf("ruta:\n got %s\nwant %s", got, want)
	}
	// Solo https: un NAS publicado por el túnel siempre lo es, y aceptar http
	// sería mandar las credenciales del cliente en claro por Internet.
	if _, err := NewWebDAV("http://nas.example", "", "u", "p"); err == nil {
		t.Fatal("el WebDAV remoto tiene que ser https")
	}
}
