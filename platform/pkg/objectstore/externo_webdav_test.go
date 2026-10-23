// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	got, err := w.ruta(tenancy.Tenant{Slug: "acme"}, "files/con espacio/á.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := "/Sytue Drive/tenants/acme/files/con espacio/á.txt"
	if got != want {
		t.Fatalf("ruta:\n got %s\nwant %s", got, want)
	}
	// Solo https: un NAS publicado por el túnel siempre lo es, y aceptar http
	// sería mandar las credenciales del cliente en claro por Internet.
	if _, err := NewWebDAV("http://nas.example", "", "u", "p"); err == nil {
		t.Fatal("el WebDAV remoto tiene que ser https")
	}
}

func TestWebDAVRechazaBasesAmbiguasOSinTLS(t *testing.T) {
	for _, tc := range []struct{ base, path string }{
		{"http://nas.example/", ""},
		{"https:///sin-host", ""},
		{"https:opaque", ""},
		{"https://usuario:clave@nas.example/", ""},
		{"https://nas.example/../otra", ""},
		{"https://nas.example/%2e%2e/otra", ""},
		{"https://nas.example/a/./b", ""},
		{"https://nas.example/a//b", ""},
		{"https://nas.example/a%5cb", ""},
		{"https://nas.example/a%00b", ""},
		{"https://nas.example/", "../otra"},
		{"https://nas.example/", "carpeta/./otra"},
		{"https://nas.example/", "carpeta\\otra"},
		{"https://nas.example/", "carpeta\n"},
	} {
		t.Run(tc.base+"+"+tc.path, func(t *testing.T) {
			if _, err := NewWebDAV(tc.base, tc.path, "usuario", "clave"); err == nil {
				t.Fatal("se aceptó una base que puede reinterpretarse fuera de la carpeta configurada")
			}
		})
	}
}

// Los metadatos de un blob controlan una ruta, nunca el origen de la petición.
// Los escapes deben aplicarse una sola vez, incluso si la base ya venía escapada.
func TestWebDAVMantieneOrigenYEscapeDeLosMetadatos(t *testing.T) {
	w, err := NewWebDAV("https://nas.example:8443/dav%20root/?ignorado=1#ignorado", "Sytue Drive", "usuario", "clave")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	w.http.Transport = dropboxTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "nas.example:8443" || r.URL.User != nil || r.URL.Opaque != "" || r.URL.RawQuery != "" || r.URL.Fragment != "" {
			t.Fatalf("los metadatos alteraron el origen: %s", r.URL)
		}
		if r.URL.RawPath != "" {
			t.Fatalf("se conservó un escape obsoleto de la base: %q", r.URL.RawPath)
		}
		want := "https://nas.example:8443/dav%20root/Sytue%20Drive/tenants/acme/files/127.0.0.1:8080%3Fx=y%23z%252f%C3%A1%20+.txt"
		if r.URL.String() != want {
			t.Fatalf("ruta alterada:\n got %s\nwant %s", r.URL, want)
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "usuario" || pass != "clave" {
			t.Fatal("no se autenticó contra el origen configurado")
		}
		return dropboxResponse(r, http.StatusOK, "bytes"), nil
	})
	rc, err := w.Get(context.Background(), tenancy.Tenant{Slug: "acme"}, "///files/127.0.0.1:8080?x=y#z%2fá +.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	if calls != 1 {
		t.Fatalf("peticiones: %d", calls)
	}
}

// Comprobar el error antes del transporte cubre también PUT y COPY: crear un
// padre antes de validar el resto de la clave ya sería una escritura indebida.
func TestWebDAVNoEmitePeticionesParaClavesOTenantsInvalidos(t *testing.T) {
	ops := []struct {
		name string
		run  func(*WebDAV, tenancy.Tenant, string) error
	}{
		{"Get", func(w *WebDAV, tenant tenancy.Tenant, key string) error {
			rc, err := w.Get(context.Background(), tenant, key)
			if rc != nil {
				_ = rc.Close()
			}
			return err
		}},
		{"Put", func(w *WebDAV, tenant tenancy.Tenant, key string) error {
			return w.Put(context.Background(), tenant, key, strings.NewReader("bytes"), 5, PutOptions{})
		}},
		{"Delete", func(w *WebDAV, tenant tenancy.Tenant, key string) error {
			return w.Delete(context.Background(), tenant, key)
		}},
		{"Stat", func(w *WebDAV, tenant tenancy.Tenant, key string) error {
			_, err := w.Stat(context.Background(), tenant, key)
			return err
		}},
		{"Existe", func(w *WebDAV, tenant tenancy.Tenant, key string) error {
			_, err := w.Existe(context.Background(), tenant, key)
			return err
		}},
		{"Copiar-origen", func(w *WebDAV, tenant tenancy.Tenant, key string) error {
			return w.Copiar(context.Background(), tenant, key, "destino/nuevo/blob")
		}},
		{"Copiar-destino", func(w *WebDAV, tenant tenancy.Tenant, key string) error {
			return w.Copiar(context.Background(), tenant, "origen/blob", key)
		}},
	}
	cases := []struct{ slug, key string }{
		{"acme", ""}, {"acme", "///"}, {"acme", "../beta/blob"},
		{"acme", "files/../../beta/blob"}, {"acme", "files/./blob"},
		{"acme", "files//blob"}, {"acme", "files\\..\\beta\\blob"},
		{"acme", "files/\x00blob"}, {"acme", "files/\r\nblob"},
		{"acme", "files/\x7fblob"}, {"acme", "files/\u0085blob"},
		{"", "files/blob"}, {"../beta", "files/blob"},
		{"acme/../../beta", "files/blob"},
	}
	for _, op := range ops {
		for _, tc := range cases {
			t.Run(op.name+"/"+tc.slug+"/"+tc.key, func(t *testing.T) {
				w, err := NewWebDAV("https://nas.example/", "Sytue", "usuario", "clave")
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				w.http.Transport = dropboxTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					return dropboxResponse(r, http.StatusOK, ""), nil
				})
				if err := op.run(w, tenancy.Tenant{Slug: tc.slug}, tc.key); err == nil {
					t.Fatal("la operación aceptó metadatos que escapan del tenant")
				}
				if calls != 0 {
					t.Fatalf("se emitieron %d peticiones antes de rechazar los metadatos", calls)
				}
			})
		}
	}
}

func TestWebDAVCopiarConservaDestinoYPadresDentroDelTenant(t *testing.T) {
	w, err := NewWebDAV("https://nas.example/dav/", "Sytue Drive", "usuario", "clave")
	if err != nil {
		t.Fatal(err)
	}
	var parents []string
	copies := 0
	w.http.Transport = dropboxTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "nas.example" || !strings.HasPrefix(r.URL.Path, "/dav/Sytue Drive/tenants/") {
			t.Fatalf("petición fuera del tenant: %s", r.URL)
		}
		if r.Method == "MKCOL" {
			parents = append(parents, r.URL.Path)
			return dropboxResponse(r, http.StatusCreated, ""), nil
		}
		if r.Method != "COPY" || r.URL.Path != "/dav/Sytue Drive/tenants/acme/origen/á.txt" {
			t.Fatalf("operación inesperada: %s %s", r.Method, r.URL)
		}
		copies++
		dest, err := url.Parse(r.Header.Get("Destination"))
		if err != nil || dest.Scheme != "https" || dest.Host != "nas.example" || dest.User != nil || dest.Path != "/dav/Sytue Drive/tenants/acme/nueva carpeta/b%#?.txt" || dest.RawQuery != "" || dest.Fragment != "" {
			t.Fatalf("COPY perdió el destino confinado: %q (%v)", r.Header.Get("Destination"), err)
		}
		return dropboxResponse(r, http.StatusCreated, ""), nil
	})
	if err := w.Copiar(context.Background(), tenancy.Tenant{Slug: "acme"}, "origen/á.txt", "nueva carpeta/b%#?.txt"); err != nil {
		t.Fatal(err)
	}
	// Los padres incluyen las colecciones tenants/acme y la carpeta de destino;
	// nunca se crea fuera de la base ni se interpreta el nombre como otra URL.
	wantParents := []string{"/dav/Sytue Drive/tenants/", "/dav/Sytue Drive/tenants/acme/", "/dav/Sytue Drive/tenants/acme/nueva carpeta/"}
	if copies != 1 || strings.Join(parents, "\n") != strings.Join(wantParents, "\n") {
		t.Fatalf("COPY=%d padres=%q", copies, parents)
	}
}

func TestWebDAVNoRedirigeCredenciales(t *testing.T) {
	for _, target := range []string{
		"https://otra.nas.example/destino", "https://nas.example:9443/destino",
		"https://127.0.0.1/destino", "http://nas.example/destino", "/fuera-de-base",
	} {
		t.Run(target, func(t *testing.T) {
			w, err := NewWebDAV("https://nas.example/dav/", "Sytue", "usuario", "clave")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			w.http.Transport = dropboxTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls > 1 {
					t.Fatalf("se siguió la redirección, Authorization=%q destino=%s", r.Header.Get("Authorization"), r.URL)
				}
				res := dropboxResponse(r, http.StatusTemporaryRedirect, "")
				res.Header.Set("Location", target)
				return res, nil
			})
			rc, err := w.Get(context.Background(), tenancy.Tenant{Slug: "acme"}, "files/blob")
			if rc != nil {
				_, _ = io.Copy(io.Discard, rc)
				_ = rc.Close()
			}
			if err == nil || calls != 1 {
				t.Fatalf("redirección no rechazada: err=%v peticiones=%d", err, calls)
			}
		})
	}
}
