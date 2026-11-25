// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// s3Falso es un bucket en memoria que entiende lo justo de S3 para la purga:
// ListObjectsV2, el borrado múltiple (POST ?delete) y el DELETE de un objeto.
type s3Falso struct {
	mu     sync.Mutex
	claves map[string]bool
}

func nuevoS3Falso(t *testing.T, claves ...string) (*s3Falso, string) {
	t.Helper()
	f := &s3Falso{claves: map[string]bool{}}
	for _, k := range claves {
		f.claves[k] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(f.servir))
	t.Cleanup(srv.Close)
	return f, strings.TrimPrefix(srv.URL, "http://")
}

func (f *s3Falso) servir(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodGet && q.Get("list-type") == "2":
		prefijo := q.Get("prefix")
		var claves []string
		for k := range f.claves {
			if strings.HasPrefix(k, prefijo) {
				claves = append(claves, k)
			}
		}
		sort.Strings(claves)
		_, _ = w.Write([]byte(listXML(claves, false, "")))
	case r.Method == http.MethodPost && q.Has("delete"):
		var cuerpo struct {
			Objetos []struct {
				Key string `xml:"Key"`
			} `xml:"Object"`
		}
		raw, _ := io.ReadAll(r.Body)
		if err := xml.Unmarshal(raw, &cuerpo); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var out strings.Builder
		out.WriteString("<DeleteResult>")
		for _, o := range cuerpo.Objetos {
			delete(f.claves, o.Key)
			out.WriteString("<Deleted><Key>")
			_ = xml.EscapeText(&out, []byte(o.Key))
			out.WriteString("</Key></Deleted>")
		}
		out.WriteString("</DeleteResult>")
		_, _ = w.Write([]byte(out.String()))
	case r.Method == http.MethodDelete:
		clave := strings.TrimPrefix(r.URL.Path, "/")
		clave = clave[strings.Index(clave, "/")+1:]
		delete(f.claves, clave)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func (f *s3Falso) quedan() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.claves))
	for k := range f.claves {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestBorrarTenantBarreLosDosAlmacenesYNadaMas(t *testing.T) {
	local, epLocal := nuevoS3Falso(t,
		"tenants/p-ana/viejo.bin",               // anterior al remoto: se quedó en local
		"tenants/p-ana-2/otro.bin",              // otro tenant cuyo slug empieza igual
		"shared/mensajes/p-ana/abc.eml",         // su correo deduplicado
		"shared/mensajes/p-anabel/def.eml",      // el de otra persona
		"shared/mensajes/legado-sin-tenant.eml", // compartido entre organizaciones
	)
	remoto, epRemoto := nuevoS3Falso(t, "tenants/p-ana/a/1.bin", "tenants/p-ana/a/2.bin", "tenants/acme/x.bin")
	c, err := New(Config{
		Endpoint: epLocal, Region: "eu-test-1", AccessKey: "a", SecretKey: "b", Bucket: "drive-blobs",
		Remote: &Config{Endpoint: epRemoto, Region: "eu-test-1", AccessKey: "c", SecretKey: "d", Bucket: "sytue-blobs"},
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := c.BorrarTenant(t.Context(), tenancy.Tenant{Slug: "p-ana"})
	if err != nil || n != 3 {
		t.Fatalf("BorrarTenant: %d, %v; quiero 3", n, err)
	}
	if n, err := c.BorrarPrefijo(t.Context(), "shared/mensajes/p-ana/"); err != nil || n != 1 {
		t.Fatalf("BorrarPrefijo compartido: %d, %v", n, err)
	}
	if err := c.BorrarClave(t.Context(), "shared/mensajes/legado-sin-tenant.eml"); err != nil {
		t.Fatal(err)
	}
	// Idempotente: lo que ya no existe no es un error.
	if err := c.BorrarClave(t.Context(), "shared/mensajes/legado-sin-tenant.eml"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(local.quedan(), ","); got != "shared/mensajes/p-anabel/def.eml,tenants/p-ana-2/otro.bin" {
		t.Fatalf("local tras purgar: %s", got)
	}
	if got := strings.Join(remoto.quedan(), ","); got != "tenants/acme/x.bin" {
		t.Fatalf("remoto tras purgar: %s", got)
	}
}

func TestBorrarPrefijoRechazaLoQueBarreriaAOtros(t *testing.T) {
	_, ep := nuevoS3Falso(t, "tenants/acme/x")
	c, err := New(Config{Endpoint: ep, Region: "eu-test-1", AccessKey: "a", SecretKey: "b", Bucket: "b"})
	if err != nil {
		t.Fatal(err)
	}
	for _, malo := range []string{
		"", "/", "tenants/", "tenants", "tenants/acme", "tenants/ACME/", "shared/", "shared/mensajes/",
		"tenants/acme/../otra/", "otra/cosa/", "/tenants/acme/", "tenants/acme//", "shared/mensajes/../x/",
	} {
		if _, err := c.BorrarPrefijo(t.Context(), malo); !errors.Is(err, ErrPrefijoDePurga) {
			t.Errorf("BorrarPrefijo(%q) = %v; quiero ErrPrefijoDePurga", malo, err)
		}
	}
	for _, mala := range []string{"tenants/acme/x", "shared/", "shared/a/../b", "shared/a/"} {
		if err := c.BorrarClave(t.Context(), mala); !errors.Is(err, ErrPrefijoDePurga) {
			t.Errorf("BorrarClave(%q) = %v; quiero ErrPrefijoDePurga", mala, err)
		}
	}
	if _, err := c.BorrarTenant(t.Context(), tenancy.Tenant{Slug: ""}); !errors.Is(err, ErrPrefijoDePurga) {
		t.Error("un tenant sin slug no se barre")
	}
}
