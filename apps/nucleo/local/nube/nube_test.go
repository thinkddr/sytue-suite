// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

func TestWebDAVListaDescargaYSube(t *testing.T) {
	var mu sync.Mutex
	ficheros := map[string][]byte{"/dav/Informes/a.docx": []byte("DOCX")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "ana" || p != "app-pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "PROPFIND":
			if r.Header.Get("Depth") != "1" {
				t.Error("Depth")
			}
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">
<d:response><d:href>/dav/Informes/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/Informes/a.docx</d:href><d:propstat><d:prop><d:getcontentlength>4</d:getcontentlength><d:getlastmodified>Mon, 02 Nov 2026 10:00:00 GMT</d:getlastmodified><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/Informes/Fotos%20de%20viaje/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
</d:multistatus>`)
		case http.MethodGet:
			_, _ = w.Write(ficheros[r.URL.Path])
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			ficheros[r.URL.Path] = b
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()
	d := &DAV{Base: srv.URL + "/dav", Usuario: "ana", Clave: "app-pass", HTTP: srv.Client()}
	es, err := d.Listar(t.Context(), "/Informes")
	if err != nil || len(es) != 2 || !es[0].Carpeta || es[0].Nombre != "Fotos de viaje" || es[1].ID != "/Informes/a.docx" || es[1].Tamano != 4 {
		t.Fatalf("listar: %+v %v", es, err)
	}
	datos, nombre, err := d.Descargar(t.Context(), es[1])
	if err != nil || string(datos) != "DOCX" || nombre != "a.docx" {
		t.Fatal(string(datos), nombre, err)
	}
	if _, err := d.Subir(t.Context(), "/Informes", "b c.pdf", []byte("%PDF")); err != nil {
		t.Fatal(err)
	}
	if string(ficheros["/dav/Informes/b c.pdf"]) != "%PDF" {
		t.Fatal("no subió", ficheros)
	}
	d.Clave = "mala"
	if _, err := d.Listar(t.Context(), ""); err == nil || !strings.Contains(err.Error(), "credenciales") {
		t.Fatal(err)
	}
}

func token(context.Context) (string, error) { return "tk", nil }

func TestGoogleDriveExportaLosDocumentosDeGoogle(t *testing.T) {
	var subido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tk" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/drive/v3/files" && r.Method == http.MethodGet:
			if !strings.Contains(r.URL.Query().Get("q"), "'root' in parents") {
				t.Error(r.URL.Query().Get("q"))
			}
			_, _ = io.WriteString(w, `{"files":[{"id":"1","name":"Plan","mimeType":"application/vnd.google-apps.document"},{"id":"2","name":"Fotos","mimeType":"application/vnd.google-apps.folder"}]}`)
		case r.URL.Path == "/drive/v3/files/1/export":
			if !strings.Contains(r.URL.Query().Get("mimeType"), "wordprocessingml") {
				t.Error("export", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, "DOCX")
		case r.URL.Path == "/upload/drive/v3/files" && r.Method == http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			subido = string(b)
			_, _ = io.WriteString(w, `{"id":"3","name":"Nuevo.xlsx"}`)
		default:
			t.Error("ruta", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	g := &GoogleDrive{Token: token, HTTP: srv.Client(), API: srv.URL + "/drive/v3", Subidas: srv.URL + "/upload/drive/v3"}
	es, err := g.Listar(t.Context(), "")
	if err != nil || len(es) != 2 || !es[0].Carpeta {
		t.Fatal(es, err)
	}
	datos, nombre, err := g.Descargar(t.Context(), es[1])
	if err != nil || string(datos) != "DOCX" || nombre != "Plan.docx" {
		t.Fatal(string(datos), nombre, err)
	}
	if e, err := g.Subir(t.Context(), "", "Nuevo.xlsx", []byte("XLSX")); err != nil || e.ID != "3" || !strings.Contains(subido, `"name":"Nuevo.xlsx"`) || !strings.Contains(subido, "XLSX") {
		t.Fatal(e, err, subido)
	}
}

func TestOneDriveListaPaginadoYSube(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.0/me/drive/root/children" && r.URL.Query().Get("$skiptoken") == "":
			_, _ = io.WriteString(w, `{"value":[{"id":"a","name":"b.pptx","size":9,"file":{"mimeType":"x"}}],"@odata.nextLink":"`+srv.URL+`/v1.0/me/drive/root/children?$skiptoken=2"}`)
		case r.URL.Path == "/v1.0/me/drive/root/children":
			_, _ = io.WriteString(w, `{"value":[{"id":"c","name":"A","folder":{}}]}`)
		case r.URL.Path == "/v1.0/me/drive/items/a/content":
			_, _ = io.WriteString(w, "PPTX")
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, ":/content"):
			if !strings.Contains(r.URL.Path, "/me/drive/items/c:/nota.md:") {
				t.Error(r.URL.Path)
			}
			_, _ = io.WriteString(w, `{"id":"d","name":"nota.md","file":{}}`)
		default:
			t.Error("ruta", r.URL.String())
		}
	}))
	defer srv.Close()
	o := &OneDriveGraph{Token: token, HTTP: srv.Client(), API: srv.URL + "/v1.0"}
	es, err := o.Listar(t.Context(), "")
	if err != nil || len(es) != 2 || es[0].Nombre != "A" || !es[0].Carpeta {
		t.Fatal(es, err)
	}
	if d, _, err := o.Descargar(t.Context(), es[1]); err != nil || string(d) != "PPTX" {
		t.Fatal(err)
	}
	if e, err := o.Subir(t.Context(), "c", "nota.md", []byte("#")); err != nil || e.ID != "d" {
		t.Fatal(e, err)
	}
}

func TestOAuthConPKCEYLoopback(t *testing.T) {
	var verificador string
	tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "c0d1g0" || r.Form.Get("client_id") != "cli" {
			t.Error(r.Form)
		}
		verificador = r.Form.Get("code_verifier")
		_, _ = io.WriteString(w, `{"access_token":"a","refresh_token":"r","expires_in":3600}`)
	}))
	defer tokens.Close()
	o := OAuth{Autorizar: "https://login.ejemplo/authorize", Token: tokens.URL, Cliente: "cli", Alcances: []string{"Files.ReadWrite"}}
	// El «navegador» sigue la redirección como haría la persona tras aceptar.
	navegador := func(destino string) error {
		u, _ := url.Parse(destino)
		q := u.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
			t.Error("sin PKCE", q)
		}
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?code=c0d1g0&state=" + url.QueryEscape(q.Get("state")))
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	got, err := o.Conectar(t.Context(), tokens.Client(), navegador)
	if err != nil || got.Refresco != "r" || verificador == "" {
		t.Fatal(got, err)
	}
}

func TestRegistroGuardaSinSecretos(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	e, err := espacio.Abrir("")
	if err != nil {
		t.Fatal(err)
	}
	r := Abrir(e, secretos.SoloFichero(t.TempDir()))
	c, err := r.Añadir(Cuenta{Tipo: SytueDrive, Usuario: "ana@sytue.eu"}, "contraseña-de-app")
	if err != nil || c.URL != DireccionSytueDrive || c.Rotulo != "Sytue Drive" {
		t.Fatal(c, err)
	}
	raw, _ := os.ReadFile(e.Ajuste("nube.json"))
	if strings.Contains(string(raw), "contraseña-de-app") {
		t.Fatal("el secreto acabó en nube.json")
	}
	if _, err := Abrir(e, r.secretos).Proveedor(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Añadir(Cuenta{Tipo: WebDAV, URL: "http://casa/dav"}, "x"); err == nil {
		t.Fatal("aceptó WebDAV sin TLS")
	}
	if err := r.Quitar(c.ID); err != nil || len(r.Cuentas()) != 0 {
		t.Fatal(err)
	}
}

func TestSincronizadasEncuentraLasCarpetasConocidas(t *testing.T) {
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("OneDrive", "")
	t.Setenv("OneDriveConsumer", "")
	t.Setenv("OneDriveCommercial", "")
	for _, d := range []string{"OneDrive", "Sytue Drive – Acme", "Library/CloudStorage/GoogleDrive-ana@gmail.com/My Drive", "Documentos"} {
		if err := os.MkdirAll(filepath.Join(casa, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cs := Sincronizadas()
	nombres := ""
	for _, c := range cs {
		nombres += c.Nombre + "|"
	}
	for _, quiero := range []string{"OneDrive|", "Sytue Drive – Acme|", "Google Drive (ana@gmail.com)|"} {
		if !strings.Contains(nombres, quiero) {
			t.Fatal("falta", quiero, nombres)
		}
	}
	if strings.Contains(nombres, "Documentos") {
		t.Fatal(nombres)
	}
}
