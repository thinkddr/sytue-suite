// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package instaladores

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
)

// objeto es lo que el S3 de mentira guarda: contenido y metadatos de usuario.
type objeto struct {
	datos string
	meta  map[string]string
}

// s3Falso contesta lo justo del protocolo que usa minio: listar (V2), HEAD y GET.
func s3Falso(t *testing.T, objetos map[string]objeto) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clave := strings.TrimPrefix(r.URL.Path, "/descargas/")
		if r.URL.Path == "/descargas/" || r.URL.Path == "/descargas" {
			var b strings.Builder
			b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>descargas</Name><IsTruncated>false</IsTruncated>`)
			for k, o := range objetos {
				if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
					fmt.Fprintf(&b, `<Contents><Key>%s</Key><Size>%d</Size><LastModified>2026-09-24T10:00:00.000Z</LastModified><ETag>"e"</ETag></Contents>`, k, len(o.datos))
				}
			}
			b.WriteString(`</ListBucketResult>`)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, b.String())
			return
		}
		o, ok := objetos[clave]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code></Error>`)
			return
		}
		for k, v := range o.meta {
			w.Header().Set("X-Amz-Meta-"+k, v)
		}
		w.Header().Set("Last-Modified", "Thu, 24 Sep 2026 10:00:00 GMT")
		w.Header().Set("ETag", `"e"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprint(len(o.datos)))
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, o.datos)
		}
	}))
}

func catalogoDePrueba(t *testing.T, publico string) *Catalogo {
	t.Helper()
	srv := s3Falso(t, map[string]objeto{
		"latest/sytue-hub-macos-universal.dmg":         {"dmg-hub", map[string]string{"Sytue-Tag": "v3.6.0", "Sytue-Sha256": "ABC123"}},
		"latest/sytue-mail-macos-universal.dmg":        {"dmg-mail", map[string]string{"Sytue-Tag": "v3.6.0"}},
		"latest/sytue-hub-windows-amd64-installer.exe": {"exe", map[string]string{"Sytue-Tag": "v3.6.0"}},
		"latest/SHA256SUMS":                            {"sumas", nil},
		"latest/SytueOne-macos-universal.dmg":          {"wails", nil},
		"otra/sytue-hub-macos-universal.dmg":           {"fuera", nil},
	})
	t.Cleanup(srv.Close)
	cat, err := Nuevo(Config{
		Endpoint: srv.URL, EndpointPublico: publico, Region: "fr-par", Bucket: "descargas",
		Prefijo: "latest/", AccessKey: "a", SecretKey: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// TestElCatalogoDiceVersionSumaYDeDondeBajarlo: de todo lo que hay en el bucket solo
// salen los instaladores de ESA plataforma con el nombre de `empaquetar.sh`, con la
// versión sin «v» y la suma que dejó quien los subió. Con un almacén en claro (el del
// piloto) no hay URL firmada que el equipo de nadie alcance; con uno público, sí.
// VISTO EN ROJO quitando el filtro de plataforma (salía el .exe en macOS) y firmando
// siempre (salía una URL a un almacén en claro).
func TestElCatalogoDiceVersionSumaYDeDondeBajarlo(t *testing.T) {
	ctx := context.Background()
	lista, err := catalogoDePrueba(t, "").Listar(ctx, "macos")
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 2 {
		t.Fatalf("macOS tiene dos instaladores y salen %d: %+v", len(lista), lista)
	}
	por := map[string]Instalador{}
	for _, i := range lista {
		por[i.App] = i
	}
	hub := por["hub"]
	if hub.Version != "3.6.0" || hub.SHA256 != "abc123" || hub.Bytes != int64(len("dmg-hub")) ||
		hub.Fichero != "sytue-hub-macos-universal.dmg" || hub.Plataforma != "macos" {
		t.Errorf("el del Hub sale %+v", hub)
	}
	if hub.URL != "" {
		t.Errorf("un almacén en claro no da URL que abrir desde fuera: %q", hub.URL)
	}
	if por["mail"].SHA256 != "" {
		t.Errorf("sin suma en el bucket no se inventa una: %q", por["mail"].SHA256)
	}

	// Sin plataforma, las de todas: los dos .dmg y el .exe, y nada más del bucket.
	if todas, err := catalogoDePrueba(t, "").Listar(ctx, ""); err != nil || len(todas) != 3 {
		t.Errorf("sin plataforma salen %d: %+v (%v)", len(todas), todas, err)
	}

	firmado, err := catalogoDePrueba(t, "https://s3.sytue.eu").Listar(ctx, "macos")
	if err != nil || len(firmado) == 0 || !strings.HasPrefix(firmado[0].URL, "https://s3.sytue.eu/descargas/latest/") ||
		!strings.Contains(firmado[0].URL, "X-Amz-Signature=") {
		t.Errorf("con endpoint público la URL va firmada contra él: %+v (%v)", firmado, err)
	}
}

// TestSoloSeAbrenInstaladores: por el Hub se baja un instalador, y nada más del bucket.
func TestSoloSeAbrenInstaladores(t *testing.T) {
	cat := catalogoDePrueba(t, "")
	r, inst, err := cat.Abrir(context.Background(), "sytue-hub-macos-universal.dmg")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	_ = r.Close()
	if string(b) != "dmg-hub" || inst.SHA256 != "abc123" {
		t.Errorf("se ha bajado %q (%+v)", b, inst)
	}
	for _, malo := range []string{"SHA256SUMS", "../otra/sytue-hub-macos-universal.dmg", "sytue-otra-macos-universal.dmg"} {
		if _, _, err := cat.Abrir(context.Background(), malo); err == nil {
			t.Errorf("%q no es un instalador publicado y se ha abierto", malo)
		}
	}
}

// TestUnBundleSeListaUnaVezPorCadaAppQueLleva: el instalador de «Sytue One» sale como
// el de cada app que lleva, con su fichero y la clave del bundle, que es como el Hub y
// Drive —también las versiones ya instaladas, que buscan `hub`— encuentran el suyo. Una
// clave rara en el metadato no se cuela, y sin metadato el fichero es el de la app sola.
// VISTO EN ROJO sin repartir (salía una sola entrada `one` que no busca nadie).
func TestUnBundleSeListaUnaVezPorCadaAppQueLleva(t *testing.T) {
	srv := s3Falso(t, map[string]objeto{
		"latest/sytue-one-linux-amd64.tar.xz":   {"one", map[string]string{"Sytue-Tag": "v3.7.0", "Sytue-Sha256": "aa", "Sytue-Apps": "hub, mail,../x"}},
		"latest/sytue-drive-linux-amd64.tar.xz": {"drive", map[string]string{"Sytue-Tag": "v3.7.0", "Sytue-Sha256": "bb", "Sytue-Apps": "drive"}},
		"latest/sytue-notes-linux-amd64.tar.xz": {"notes", map[string]string{"Sytue-Tag": "v3.6.0"}},
	})
	t.Cleanup(srv.Close)
	cat, err := Nuevo(Config{Endpoint: srv.URL, Region: "fr-par", Bucket: "descargas", Prefijo: "latest/", AccessKey: "a", SecretKey: "b"})
	if err != nil {
		t.Fatal(err)
	}
	lista, err := cat.Listar(context.Background(), "linux")
	if err != nil {
		t.Fatal(err)
	}
	vistas := make([]string, 0, len(lista))
	for _, i := range lista {
		vistas = append(vistas, i.App+"="+i.Bundle+":"+i.Fichero+"@"+i.Version)
	}
	slices.Sort(vistas)
	quiero := []string{
		"drive=drive:sytue-drive-linux-amd64.tar.xz@3.7.0",
		"hub=one:sytue-one-linux-amd64.tar.xz@3.7.0",
		"mail=one:sytue-one-linux-amd64.tar.xz@3.7.0",
		"notes=:sytue-notes-linux-amd64.tar.xz@3.6.0",
	}
	if strings.Join(vistas, " ") != strings.Join(quiero, " ") {
		t.Errorf("el catálogo dice\n  %s\ny debería\n  %s", strings.Join(vistas, "\n  "), strings.Join(quiero, "\n  "))
	}
}

// TestArquitecturaDe son los casos de `arquitecturaDe` en
// apps/portal/src/lib/descargas.test.ts: la web y Fyne leen el procesador igual.
// VISTO EN ROJO con la expresión sin `universal`.
func TestArquitecturaDe(t *testing.T) {
	for nombre, quiero := range map[string]string{
		"sytue-one-linux-amd64.tar.xz":          "amd64",
		"sytue-one-linux-arm64.tar.xz":          "arm64",
		"sytue-one-windows-amd64-installer.exe": "amd64",
		"sytue-one-macos-universal.dmg":         "universal",
		"SytueOne.dmg":                          "",
		"sytue-hub-android.apk":                 "",
	} {
		if got := ArquitecturaDe(nombre); got != quiero {
			t.Errorf("ArquitecturaDe(%q) = %q; quiero %q", nombre, got, quiero)
		}
	}
}

// TestLaWebYElHubAceptanLoMismo: el Hub y la página de descargas del portal ofrecen los
// mismos ficheros, con la misma plataforma y procesador. Los vectores son los que pasa
// también `descargas.test.ts`. VISTO EN ROJO sin `arm64` en Linux, que era como estaba:
// un `linux-arm64` publicado salía en la web y no en las apps nativas.
func TestLaWebYElHubAceptanLoMismo(t *testing.T) {
	crudo, err := os.ReadFile("../../../../apps/frontend/apps/portal/src/lib/nombres-instaladores.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectores []struct {
		Nombre       string  `json:"nombre"`
		Plataforma   *string `json:"plataforma"`
		Arquitectura *string `json:"arquitectura"`
	}
	if err := json.Unmarshal(crudo, &vectores); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectores {
		p, a := Clasificar(v.Nombre)
		quieroP, quieroA := "", ""
		if v.Plataforma != nil {
			quieroP = *v.Plataforma
		}
		if v.Arquitectura != nil {
			quieroA = *v.Arquitectura
		}
		if p != quieroP || (p != "" && a != quieroA) {
			t.Errorf("%s: el Hub dice %q/%q y la web %q/%q", v.Nombre, p, a, quieroP, quieroA)
		}
	}
}
