// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package wopi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// collaboraXML reproduce la FORMA del discovery de Collabora Online: el urlsrc termina
// ya en «?», así que añadir «?WOPISrc=…» detrás produciría una URL con dos signos de
// interrogación que el editor no resuelve.
const collaboraXML = `<?xml version="1.0" encoding="utf-8"?>
<wopi-discovery>
  <net-zone name="external-http">
    <app name="writer">
      <action name="edit" ext="odt" default="true" urlsrc="https://editor.test/browser/1a2b/cool.html?"/>
      <action name="edit" ext="docx" urlsrc="https://editor.test/browser/1a2b/cool.html?"/>
    </app>
    <app name="Capabilities">
      <action ext="" name="capabilities" urlsrc="https://editor.test/hosting/capabilities"/>
    </app>
    <app name="application/pdf">
      <action name="view" ext="pdf" urlsrc="https://editor.test/browser/1a2b/cool.html?"/>
    </app>
  </net-zone>
</wopi-discovery>`

// officeXML reproduce la FORMA del de Office for the web: el urlsrc trae PLANTILLAS
// entre <> que hay que quitar (no son parámetros: pasarlas rompe la carga).
const officeXML = `<?xml version="1.0" encoding="utf-8"?>
<wopi-discovery>
  <net-zone name="external-https">
    <app name="Word">
      <action name="view" ext="docx" urlsrc="https://office.test/wv/wordviewerframe.aspx?&lt;ui=UI_LLCC&amp;&gt;&lt;rs=DC_LLCC&amp;&gt;"/>
      <action name="edit" ext="docx" requires="update" urlsrc="https://office.test/we/wordeditorframe.aspx?&lt;ui=UI_LLCC&amp;&gt;"/>
    </app>
  </net-zone>
</wopi-discovery>`

func serveXML(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDescubrirCollabora: la URL compuesta tiene que ser cargable. El caso del urlsrc
// acabado en «?» es el que rompe si se concatena a ciegas.
func TestDescubrirCollabora(t *testing.T) {
	srv := serveXML(t, collaboraXML)
	d := NewDiscoveryDeConfianza(srv.URL, time.Hour)
	ctx := context.Background()

	got, ok := d.URLFor(ctx, "informe.docx", "https://drive.test/wopi/files/abc", true)
	if !ok {
		t.Fatal("no encontró acción para .docx")
	}
	if strings.Count(got, "?") != 1 {
		t.Fatalf("URL con más de un «?»: %s", got)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("URL no parseable: %v", err)
	}
	if u.Query().Get("WOPISrc") != "https://drive.test/wopi/files/abc" {
		t.Fatalf("WOPISrc = %q", u.Query().Get("WOPISrc"))
	}

	// Un PDF solo tiene acción de vista: pedir escritura debe caer a ella, no fallar.
	if _, ok := d.URLFor(ctx, "manual.pdf", "https://drive.test/wopi/files/x", true); !ok {
		t.Fatal("un PDF sin acción de edición debe abrirse en vista")
	}
	// Lo que el editor no declara NO se ofrece: una URL inventada cargaría en blanco.
	if _, ok := d.URLFor(ctx, "captura.png", "https://drive.test/wopi/files/y", false); ok {
		t.Fatal("ofreció abrir una extensión que el editor no declara")
	}
	// La acción `capabilities` no abre ficheros: no debe aparecer como extensión.
	for _, e := range d.Extensions(ctx) {
		if e == "" {
			t.Fatal("extensión vacía en el catálogo")
		}
	}
}

// TestDescubrirOfficeQuitaPlantillas: los `<ui=UI_LLCC&>` de Office no son parámetros.
// Dejarlos produce una URL que el editor rechaza.
func TestDescubrirOfficeQuitaPlantillas(t *testing.T) {
	srv := serveXML(t, officeXML)
	d := NewDiscoveryDeConfianza(srv.URL, time.Hour)

	got, ok := d.URLFor(context.Background(), "carta.docx", "https://drive.test/wopi/files/abc", true)
	if !ok {
		t.Fatal("no encontró acción para .docx")
	}
	if strings.ContainsAny(got, "<>") {
		t.Fatalf("quedaron plantillas sin limpiar: %s", got)
	}
	// Con permiso de escritura debe elegir el editor, no el visor: si abriera el visor,
	// la persona vería su documento en solo lectura sin explicación.
	if !strings.Contains(got, "wordeditorframe") {
		t.Fatalf("con write debe usar la acción de edición: %s", got)
	}
	got, _ = d.URLFor(context.Background(), "carta.docx", "https://drive.test/wopi/files/abc", false)
	if !strings.Contains(got, "wordviewerframe") {
		t.Fatalf("sin write debe usar la acción de vista: %s", got)
	}
}

// TestDiscoveryApagadoSinURL: sin editor configurado no se ofrece nada, y consultar no
// debe reventar (el resto de Drive sigue funcionando).
func TestDiscoveryApagadoSinURL(t *testing.T) {
	d := NewDiscovery("  ", time.Hour)
	if d != nil {
		t.Fatal("una URL vacía debe dejar el descubrimiento apagado")
	}
	if _, ok := d.URLFor(context.Background(), "x.docx", "src", true); ok {
		t.Fatal("sin editor no puede haber URL")
	}
	if len(d.Extensions(context.Background())) != 0 {
		t.Fatal("sin editor no hay extensiones")
	}
}

// TestDiscoveryConservaElCatalogoSiElEditorCae: un editor reiniciándose no debe dejar a
// nadie sin poder abrir sus documentos.
func TestDiscoveryConservaElCatalogoSiElEditorCae(t *testing.T) {
	vivo := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !vivo {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(collaboraXML))
	}))
	defer srv.Close()

	// TTL negativo ⇒ cada consulta reintenta la descarga: así se fuerza el fallo.
	d := NewDiscoveryDeConfianza(srv.URL, -time.Second)
	ctx := context.Background()
	if _, ok := d.URLFor(ctx, "a.docx", "src", true); !ok {
		t.Fatal("primera consulta debería resolver")
	}
	vivo = false
	if _, ok := d.URLFor(ctx, "a.docx", "src", true); !ok {
		t.Fatal("con el editor caído se perdió el catálogo ya descargado")
	}
}

// TestDiscoveryIgnoraBasura: un destino equivocado (una página HTML, por ejemplo) no
// debe dejar el catálogo en un estado a medias.
func TestDiscoveryIgnoraBasura(t *testing.T) {
	srv := serveXML(t, "<html><body>no soy un discovery</body></html>")
	d := NewDiscoveryDeConfianza(srv.URL, time.Hour)
	if _, ok := d.URLFor(context.Background(), "a.docx", "src", true); ok {
		t.Fatal("compuso una URL a partir de basura")
	}
}
