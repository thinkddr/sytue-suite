// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// El reparto por tenant es lo único que decide dónde acaban los bytes: si falla,
// lo nuevo llena el disco del piloto o, peor, lo que debía quedarse en local
// sale a Scaleway.
func TestElAlmacenRemotoRecibeTodoMenosLoQueSeQuedoEnLocal(t *testing.T) {
	c, err := New(Config{
		Endpoint: "seaweedfs:8333", AccessKey: "a", SecretKey: "b", Bucket: "drive-blobs",
		Remote: &Config{
			Endpoint: "s3.fr-par.scw.cloud", UseSSL: true, AccessKey: "c", SecretKey: "d",
			Bucket: "sytue-blobs", StorageClass: "ONEZONE_IA",
		},
		LocalTenants: []string{"corporate", "p-0123456789abcdef"},
	})
	if err != nil {
		t.Fatal(err)
	}
	remoto := func(slug string) (string, string) { _, b, c := c.backend(tenancy.Tenant{Slug: slug}, "x"); return b, c }
	if b, cl := remoto("acme-nueva"); b != "sytue-blobs" || cl != "ONEZONE_IA" {
		t.Fatalf("una organización nueva va al remoto con su clase: %s %q", b, cl)
	}
	if b, cl := remoto("corporate"); b != "drive-blobs" || cl != "" {
		t.Fatalf("la que se quedó en local sigue en local: %s %q", b, cl)
	}
	if b, _ := remoto("p-0123456789abcdef"); b != "sytue-blobs" {
		t.Fatalf("una cuenta personal va al remoto aunque alguien la liste en local: %s", b)
	}
	// El espacio compartido (deduplicación) se queda donde estaba, sea de quien sea.
	if _, b, _ := c.backend(tenancy.Tenant{Slug: "acme-nueva"}, PrefijoCompartido+"mensajes/abc"); b != "drive-blobs" {
		t.Fatalf("un blob compartido vive en el almacén del servicio: %s", b)
	}
	// Sin remoto configurado, todo al mismo sitio.
	solo, _ := New(Config{Endpoint: "seaweedfs:8333", AccessKey: "a", SecretKey: "b", Bucket: "drive-blobs"})
	if _, b, _ := solo.backend(tenancy.Tenant{Slug: "p-0123456789abcdef"}, "x"); b != "drive-blobs" {
		t.Fatalf("sin remoto: %s", b)
	}
	// Un remoto sin bucket no arranca: mejor que descubrirlo al primer Put.
	if _, err := New(Config{Endpoint: "x", AccessKey: "a", SecretKey: "b", Bucket: "y", Remote: &Config{Endpoint: "z", AccessKey: "a", SecretKey: "b"}}); err == nil {
		t.Fatal("un remoto sin bucket debe fallar al crear el cliente")
	}
}

func TestRemote(t *testing.T) {
	if Remote(Config{}) != nil {
		t.Fatal("sin endpoint no hay almacén remoto")
	}
	c := Remote(Config{Endpoint: "https://s3.fr-par.scw.cloud", Bucket: "b"})
	if c == nil || c.Endpoint != "s3.fr-par.scw.cloud" || !c.UseSSL || c.Bucket != "b" {
		t.Fatalf("el esquema del endpoint fija el TLS: %+v", c)
	}
}
