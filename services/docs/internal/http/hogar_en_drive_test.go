// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
)

// driveDeMentira levanta un Sytue Drive mínimo: raíz, carpeta ya creada y subida.
// Devuelve el cliente ya apuntado a él y un puntero al último nombre subido y a
// los bytes recibidos, que es lo que estos tests miran.
type driveDeMentira struct {
	estadoSubida int
	nombre       string
	bytes        []byte
	subidas      int
}

func (d *driveDeMentira) cliente(t *testing.T) *drive.Cliente {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/folders/root", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"raiz","name":"Mi unidad"}`)
	})
	mux.HandleFunc("/api/v1/folders/raiz/children", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"folders":[{"id":"carp","name":"Sytue Docs"}],"files":[]}`)
	})
	mux.HandleFunc("/api/v1/folders/carp/children", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
	})
	mux.HandleFunc("/api/v1/folders/carp/files", func(w http.ResponseWriter, r *http.Request) {
		d.subidas++
		d.nombre = r.Header.Get("X-File-Name")
		d.bytes, _ = io.ReadAll(r.Body)
		if d.estadoSubida != 0 {
			w.WriteHeader(d.estadoSubida)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"fich-ada","name":"`+d.nombre+`","size_bytes":10}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return drive.Nuevo(srv.URL, 2*time.Second)
}

// peticionCrear arma el POST de creación con la credencial de la persona: el
// handler la reenvía a Drive, así que sin cabecera no hay unidad donde escribir.
func peticionCrear(t *testing.T, cuerpo string) *http.Request {
	t.Helper()
	r := req(t, http.MethodPost, "/api/v1/documents", &authn.Claims{Subject: "user-ada", Tenant: "acme"}, nil, cuerpo)
	r.Header.Set("Authorization", "Bearer tok-de-ada")
	return r
}

// LO QUE ESTE CAMBIO PROMETE: un documento nuevo existe también como fichero en
// la unidad de su dueño, y nace anclado a él («drive:<uuid>», mig 0006). Sin el
// ancla, la acción «Guardar en Sytue Drive» del editor no se ofrece siquiera.
func TestCrearDocumentoDejaElFicheroEnLaUnidad(t *testing.T) {
	d := &driveDeMentira{}
	h := newHandler(&fakeStore{})
	h.drive = d.cliente(t)

	rec := httptest.NewRecorder()
	h.createDocument(rec, peticionCrear(t, `{"kind":"document","title":"Acta del consejo"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("estado = %d: %s", rec.Code, rec.Body.String())
	}
	var resp documentResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if resp.Document.SourceRef != "drive:fich-ada" {
		t.Errorf("source_ref = %q, quiero «drive:fich-ada»", resp.Document.SourceRef)
	}
	// ODF y no OOXML: el estándar abierto, y un formato que el propio catálogo
	// WOPI de Docs sabe volver a abrir desde Drive.
	if d.nombre != "Acta del consejo.odt" {
		t.Errorf("nombre en la unidad = %q", d.nombre)
	}
	// Un fichero de cero bytes sería un documento roto para quien lo descargue
	// antes de escribir nada: se sube el documento vacío DE VERDAD.
	if len(d.bytes) == 0 {
		t.Error("el fichero subido está vacío; debería ser un .odt válido")
	}
}

// Un documento que VIENE de un fichero de Drive («abrir con Sytue Docs») ya tiene
// su fichero: crearle otro duplicaría el original y rompería la idempotencia que
// da el índice único de source_ref (mig 0006).
func TestCrearDocumentoConOrigenNoDuplicaElFichero(t *testing.T) {
	d := &driveDeMentira{}
	h := newHandler(&fakeStore{})
	h.drive = d.cliente(t)

	rec := httptest.NewRecorder()
	h.createDocument(rec, peticionCrear(t, `{"kind":"document","title":"Ventas","source_ref":"drive:ya-existe"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("estado = %d: %s", rec.Code, rec.Body.String())
	}
	if d.subidas != 0 {
		t.Errorf("se subieron %d ficheros; con origen no debe subirse ninguno", d.subidas)
	}
}

// LA CUOTA. Desde que el documento vive en la unidad, crearlo la consume, y quien
// la mide es Drive. Con la unidad llena el documento NO se crea y se dice por qué:
// crearlo igual dejaría un documento sin fichero, que es el estado que esta ola
// viene precisamente a eliminar.
func TestCrearDocumentoConLaUnidadLlena(t *testing.T) {
	d := &driveDeMentira{estadoSubida: http.StatusInsufficientStorage}
	h := newHandler(&fakeStore{})
	h.drive = d.cliente(t)

	rec := httptest.NewRecorder()
	h.createDocument(rec, peticionCrear(t, `{"kind":"document","title":"Acta"}`))

	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("estado = %d, quiero 507: %s", rec.Code, rec.Body.String())
	}
}

// Sin Drive configurado el documento se crea como siempre, solo en Docs. Es un
// despliegue a medias —no aparecerá en la unidad de nadie— pero no puede impedir
// que se trabaje.
func TestCrearDocumentoSinDriveConfigurado(t *testing.T) {
	h := newHandler(&fakeStore{}) // h.drive queda nil

	rec := httptest.NewRecorder()
	h.createDocument(rec, peticionCrear(t, `{"kind":"document","title":"Acta"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("estado = %d: %s", rec.Code, rec.Body.String())
	}
	var resp documentResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if resp.Document.SourceRef != "" {
		t.Errorf("source_ref = %q, quiero vacío", resp.Document.SourceRef)
	}
}

// El tipo se valida ANTES de tocar Drive: si no, un `kind` inventado dejaría un
// fichero huérfano en la unidad de alguien por cada petición mal formada.
func TestCrearDocumentoTipoInvalidoNoTocaDrive(t *testing.T) {
	d := &driveDeMentira{}
	h := newHandler(&fakeStore{})
	h.drive = d.cliente(t)

	rec := httptest.NewRecorder()
	h.createDocument(rec, peticionCrear(t, `{"kind":"holograma","title":"Acta"}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("estado = %d, quiero 400", rec.Code)
	}
	if d.subidas != 0 {
		t.Errorf("se subieron %d ficheros con un tipo inválido", d.subidas)
	}
}

// El export del documento VACÍO tiene que producirse para los tres tipos: si
// alguno fallara, ese tipo dejaría de poder crearse en cuanto Drive esté puesto.
func TestExportVacioParaLosTresTipos(t *testing.T) {
	for kind, ext := range extensionNativa {
		datos, contentType, _, err := domain.Export(kind, "Sin título", domain.NewState(), formatoNativo)
		if err != nil {
			t.Fatalf("Export(%s): %v", kind, err)
		}
		if len(datos) == 0 || contentType == "" {
			t.Errorf("Export(%s) devolvió %d bytes y content-type %q", kind, len(datos), contentType)
		}
		if ext == "" {
			t.Errorf("el tipo %s no tiene extensión asignada", kind)
		}
	}
}
