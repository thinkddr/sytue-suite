// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package wopi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// permitirLocal deja pasar las IP locales mientras dura el test: los servidores
// de `httptest` viven en 127.0.0.1, que es justo lo que la barrera SSRF bloquea.
// Se restaura al terminar para que ningún otro test herede la política abierta.
func permitirLocal(t *testing.T) {
	t.Helper()
	restore := ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })
	t.Cleanup(restore)
}

func TestCheckFileInfoYGetFile(t *testing.T) {
	permitirLocal(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") == "" {
			t.Error("el token tiene que viajar en la query, como manda el protocolo")
		}
		if strings.HasSuffix(r.URL.Path, "/contents") {
			_, _ = w.Write([]byte("PK\x03\x04contenido"))
			return
		}
		_, _ = w.Write([]byte(`{"BaseFileName":"Informe.docx","Size":9,"UserCanWrite":true}`))
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	info, err := c.CheckFileInfo(context.Background(), srv.URL+"/wopi/files/1", "t")
	if err != nil {
		t.Fatalf("CheckFileInfo: %v", err)
	}
	if info.BaseFileName != "Informe.docx" || !info.UserCanWrite {
		t.Fatalf("info = %+v", info)
	}
	if info.Extension() != "docx" {
		t.Errorf("extensión = %q", info.Extension())
	}
	datos, err := c.GetFile(context.Background(), srv.URL+"/wopi/files/1", "t")
	if err != nil || !strings.Contains(string(datos), "contenido") {
		t.Fatalf("GetFile = %q, err=%v", datos, err)
	}
}

// TestPutFileDistingueElBloqueo: 409 con X-WOPI-Lock no es «no se pudo guardar».
//
// Si se tratara como un fallo genérico, la interfaz diría «error al guardar» y la
// persona volvería a intentarlo en bucle, cuando lo que pasa es que el fichero lo
// tiene abierto otro y hay que decirlo.
func TestPutFileDistingueElBloqueo(t *testing.T) {
	permitirLocal(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-WOPI-Lock", "sesion-de-ana")
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	err := c.PutFile(context.Background(), srv.URL+"/wopi/files/1", "t", []byte("x"), "mi-bloqueo")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, quería ErrLocked", err)
	}
	if !strings.Contains(err.Error(), "sesion-de-ana") {
		t.Error("el error debe decir QUIÉN lo tiene bloqueado")
	}
}

// TestWOPISrcNoLlevaANingunSitioInterno: el destino lo aporta quien llama.
//
// `WOPISrc` viaja en la URL con la que se abre el editor, así que un enlace
// preparado haría que Docs pidiera por dentro lo que el atacante quisiera y le
// devolviera la respuesta. Es SSRF de manual, y por eso el esquema se restringe y
// la barrera de plataforma valida el destino ANTES de abrir la conexión.
func TestWOPISrcNoLlevaANingunSitioInterno(t *testing.T) {
	c := New(2 * time.Second)
	ctx := context.Background()
	for _, src := range []string{
		"http://drive.sytue.eu/wopi/files/1", // sin TLS
		"file:///etc/passwd",
		"gopher://interno/",
		"https://169.254.169.254/latest/meta-data/", // metadatos de la nube
		"https://127.0.0.1/wopi/files/1",
		"https://10.0.0.5/wopi/files/1",
	} {
		if _, err := c.CheckFileInfo(ctx, src, "t"); err == nil {
			t.Errorf("%s: debería rechazarse y no lo hizo", src)
		}
	}
	// Y sin token tampoco se llama a nadie.
	if _, err := c.CheckFileInfo(ctx, "https://drive.sytue.eu/wopi/files/1", ""); err == nil {
		t.Error("sin token no se debe llamar al host")
	}
}

func TestGetFileNoSeCreeElTamanoDelOtroExtremo(t *testing.T) {
	permitirLocal(t)
	// El host declara 9 bytes y manda 30 MiB. Creerse el Size de CheckFileInfo
	// sería dejar que el otro extremo decida cuánta memoria gasta este proceso.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		grande := strings.Repeat("A", MaxFileBytes+1024)
		_, _ = w.Write([]byte(grande))
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	if _, err := c.GetFile(context.Background(), srv.URL+"/wopi/files/1", "t"); err == nil {
		t.Fatal("un fichero por encima del tope tiene que fallar, no llenarse en memoria")
	}
}

func TestCatalogoAnunciaSoloLoQueSeSabeAbrir(t *testing.T) {
	xmlDoc, err := Descubrimiento("https://docs.sytue.eu")
	if err != nil {
		t.Fatalf("Descubrimiento: %v", err)
	}
	s := string(xmlDoc)
	for _, esperado := range []string{
		`<wopi-discovery>`,
		`ext="docx"`,
		`ext="xlsx"`,
		`ext="pptx"`,
		`name="view"`,
		`https://docs.sytue.eu/wopi/view?WOPISrc=`,
	} {
		if !strings.Contains(s, esperado) {
			t.Errorf("el catálogo no contiene %q", esperado)
		}
	}
	// Y NO se anuncia `edit` mientras el guardado no exista. El catálogo es una
	// promesa: un host que lea `edit` ofrecerá «editar», y quien pulse escribirá
	// en algo que al cerrar no conserva nada. Este check se invierte —pasando a
	// exigir `name="edit"`— el día que se encienda `anunciarEdicion`.
	if strings.Contains(s, `name="edit"`) {
		t.Error("no se anuncia edición mientras no se sepa guardar")
	}
	// El PDF se VE, no se edita como documento ofimático: anunciar `edit` para él
	// llevaría al host a ofrecer un botón que abre un editor que no lo es.
	if strings.Contains(s, `<action name="edit" ext="pdf"`) {
		t.Error("el catálogo no debe anunciar edición de PDF")
	}
	ver, editar := Soporta(".PDF")
	if !ver || editar {
		t.Errorf("Soporta(pdf) = ver:%v editar:%v", ver, editar)
	}
	if ver, _ := Soporta("exe"); ver {
		t.Error("no se anuncia lo que el importador no sabe convertir")
	}
	// Sin URL pública no hay catálogo: uno con `urlsrc` vacío haría que el host
	// abriera una pantalla en blanco sin decir por qué.
	if _, err := Descubrimiento(""); err == nil {
		t.Error("sin URL pública debe fallar")
	}
	if _, err := Descubrimiento("http://docs.sytue.eu"); err == nil {
		t.Error("sin https debe fallar")
	}
}
