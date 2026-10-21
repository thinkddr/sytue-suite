// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// driveFalso imita las cuatro rutas de Sytue Drive que este cliente usa, y anota
// lo que le llega: es lo que permite comprobar el CONTRATO (rutas, cabeceras,
// identidad propagada) sin levantar el servicio entero.
type driveFalso struct {
	carpetaExiste bool
	ficheros      string // JSON del array `files` de la carpeta de documentos
	estadoSubida  int

	creoCarpeta  bool
	nombreSubido string
	tokenVisto   string
	tipoSubido   string
	bytesSubidos int
}

func (d *driveFalso) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/folders/root", func(w http.ResponseWriter, r *http.Request) {
		d.tokenVisto = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"id":"raiz-1","name":"Mi unidad"}`)
	})
	mux.HandleFunc("/api/v1/folders/raiz-1/children", func(w http.ResponseWriter, _ *http.Request) {
		if d.carpetaExiste {
			_, _ = io.WriteString(w, `{"folders":[{"id":"carp-1","name":"Sytue Docs"}],"files":[]}`)
			return
		}
		_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
	})
	mux.HandleFunc("/api/v1/folders/carp-1/children", func(w http.ResponseWriter, _ *http.Request) {
		ficheros := d.ficheros
		if ficheros == "" {
			ficheros = "[]"
		}
		_, _ = io.WriteString(w, `{"folders":[],"files":`+ficheros+`}`)
	})
	mux.HandleFunc("/api/v1/folders", func(w http.ResponseWriter, _ *http.Request) {
		d.creoCarpeta = true
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"carp-1","name":"Sytue Docs"}`)
	})
	mux.HandleFunc("/api/v1/folders/carp-1/files", func(w http.ResponseWriter, r *http.Request) {
		d.nombreSubido = r.Header.Get("X-File-Name")
		d.tipoSubido = r.Header.Get("Content-Type")
		cuerpo, _ := io.ReadAll(r.Body)
		d.bytesSubidos = len(cuerpo)
		if d.estadoSubida != 0 {
			w.WriteHeader(d.estadoSubida)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"fich-9","name":"`+d.nombreSubido+`","size_bytes":3}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// El camino feliz de una unidad estrenada: no hay carpeta «Sytue Docs», se crea,
// y el fichero aterriza dentro con el token de la persona (no uno del servicio).
func TestCrearDocumentoCreaLaCarpetaLaPrimeraVez(t *testing.T) {
	d := &driveFalso{}
	c := Nuevo(d.servidor(t).URL, time.Second)

	f, err := c.CrearDocumento(context.Background(), "tok-de-ada", "Acta", "odt", "application/vnd.oasis.opendocument.text", []byte("abc"))
	if err != nil {
		t.Fatalf("CrearDocumento: %v", err)
	}
	if !d.creoCarpeta {
		t.Error("la carpeta «Sytue Docs» debía crearse la primera vez")
	}
	if f.ID != "fich-9" {
		t.Errorf("id del fichero = %q", f.ID)
	}
	if d.nombreSubido != "Acta.odt" {
		t.Errorf("nombre subido = %q", d.nombreSubido)
	}
	if d.bytesSubidos != 3 {
		t.Errorf("bytes subidos = %d, quiero 3", d.bytesSubidos)
	}
	// Lo que hace que Drive aplique SU autorización y no la nuestra.
	if d.tokenVisto != "Bearer tok-de-ada" {
		t.Errorf("Authorization = %q, quiero el token de quien pide", d.tokenVisto)
	}
}

// Con la carpeta ya creada NO se vuelve a crear: si se intentara, Drive daría un
// conflicto de nombre en cada documento nuevo a partir del segundo.
func TestCrearDocumentoReutilizaLaCarpeta(t *testing.T) {
	d := &driveFalso{carpetaExiste: true}
	c := Nuevo(d.servidor(t).URL, time.Second)

	if _, err := c.CrearDocumento(context.Background(), "tok", "Notas", "odt", "text/plain", []byte("x")); err != nil {
		t.Fatalf("CrearDocumento: %v", err)
	}
	if d.creoCarpeta {
		t.Error("la carpeta ya existía y no debía crearse otra")
	}
}

// El nombre se desambigua contra lo que YA hay en la carpeta: es lo que impide
// que el segundo «Sin título» sustituya al primero (mismo id de fichero).
func TestCrearDocumentoDesambiguaElNombre(t *testing.T) {
	d := &driveFalso{carpetaExiste: true, ficheros: `[{"id":"f1","name":"Sin título.odt"}]`}
	c := Nuevo(d.servidor(t).URL, time.Second)

	if _, err := c.CrearDocumento(context.Background(), "tok", "Sin título", "odt", "text/plain", []byte("x")); err != nil {
		t.Fatalf("CrearDocumento: %v", err)
	}
	if d.nombreSubido != "Sin título (2).odt" {
		t.Errorf("nombre subido = %q, quiero «Sin título (2).odt»", d.nombreSubido)
	}
}

// La unidad llena tiene que llegar DISTINGUIBLE hasta arriba: es lo único que
// quien crea el documento puede arreglar, y el handler lo traduce a un 507.
func TestCrearDocumentoCuotaLlena(t *testing.T) {
	d := &driveFalso{carpetaExiste: true, estadoSubida: http.StatusInsufficientStorage}
	c := Nuevo(d.servidor(t).URL, time.Second)

	_, err := c.CrearDocumento(context.Background(), "tok", "Acta", "odt", "text/plain", []byte("x"))
	if !errors.Is(err, ErrCuota) {
		t.Fatalf("error = %v, quiero ErrCuota", err)
	}
}

func TestCrearDocumentoSinAutorizacion(t *testing.T) {
	d := &driveFalso{carpetaExiste: true, estadoSubida: http.StatusForbidden}
	c := Nuevo(d.servidor(t).URL, time.Second)

	_, err := c.CrearDocumento(context.Background(), "tok", "Acta", "odt", "text/plain", []byte("x"))
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("error = %v, quiero ErrNoAutorizado", err)
	}
}

// Sin URL no hay cliente, y quien lo use tiene que comprobarlo: es lo que permite
// que una instalación sin Drive siga creando documentos en vez de no crear ninguno.
func TestNuevoSinURLDevuelveNil(t *testing.T) {
	if c := Nuevo("   ", time.Second); c != nil {
		t.Error("Nuevo con URL vacía debe devolver nil")
	}
}
