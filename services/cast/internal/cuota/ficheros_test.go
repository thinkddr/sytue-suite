// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cuota

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

const idFichero = "7d1c6a3e-0b8e-4f39-9d3a-4a3e8a1f2b10"

func TestFicheroLeeLosMetadatosEnSnakeCase(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files/"+idFichero {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"` + idFichero + `","name":"Clase 3.mp4","size_bytes":1234,"content_type":"video/mp4"}`))
	}))
	defer srv.Close()

	f, err := New(srv.URL, "").Fichero(context.Background(), "tok", idFichero)
	if err != nil {
		t.Fatal(err)
	}
	// Drive contesta en snake_case: con las etiquetas en camelCase (como las lleva
	// hoy el cliente de Fotos) el tamaño y el tipo llegarían a cero y vacío.
	if f.Name != "Clase 3.mp4" || f.SizeBytes != 1234 || f.ContentType != "video/mp4" {
		t.Fatalf("metadatos = %+v", f)
	}
	if auth != "Bearer tok" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestFicheroTraduceLoQueContestaDrive(t *testing.T) {
	casos := []struct {
		nombre string
		codigo int
		cuerpo string
		quiero error
	}{
		{"no existe", http.StatusNotFound, "", ErrFicheroNoExiste},
		{"prohibido", http.StatusForbidden, "", ErrFicheroProhibido},
		{"caído", http.StatusBadGateway, "", ErrNoDisponible},
		{"sin sesión válida", http.StatusUnauthorized, "", ErrRechazado},
		{"en la papelera", http.StatusOK, `{"id":"x","name":"a.mp4","size_bytes":1,"content_type":"video/mp4","deleted_at":"2026-09-01T00:00:00Z"}`, ErrFicheroNoExiste},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.codigo)
				_, _ = w.Write([]byte(c.cuerpo))
			}))
			defer srv.Close()
			_, err := New(srv.URL, "").Fichero(context.Background(), "tok", idFichero)
			if !errors.Is(err, c.quiero) {
				t.Fatalf("err = %v; quiero %v", err, c.quiero)
			}
		})
	}
}

func TestAbrirDaElSegundoSaltoSinElToken(t *testing.T) {
	contenido := []byte("0123456789abcdef")
	authAlmacen := "sin pedir"
	almacen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authAlmacen = r.Header.Get("Authorization")
		if r.URL.Query().Get("X-Amz-Signature") != "firma" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write(contenido)
	}))
	defer almacen.Close()
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files/"+idFichero+"/content" || r.URL.Query().Get("presign") != "1" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, almacen.URL+"/bucket/clave?X-Amz-Signature=firma", http.StatusTemporaryRedirect)
	}))
	defer drive.Close()

	cuerpo, n, err := New(drive.URL, "").Abrir(context.Background(), "tok", idFichero)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cuerpo.Close() }()
	leido, err := io.ReadAll(cuerpo)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(contenido)) || !bytes.Equal(leido, contenido) {
		t.Fatalf("n=%d leído=%q", n, leido)
	}
	// El token de la persona no sale hacia el almacén de objetos.
	if authAlmacen != "" {
		t.Fatalf("el almacén recibió Authorization %q", authAlmacen)
	}
}

func TestAbrirSinURLFirmadaLeeDeDrive(t *testing.T) {
	// Una organización con almacén WebDAV: Drive no firma y sirve el fichero él.
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("hola!"))
	}))
	defer drive.Close()
	cuerpo, n, err := New(drive.URL, "").Abrir(context.Background(), "tok", idFichero)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cuerpo.Close() }()
	if n != 5 {
		t.Fatalf("n = %d", n)
	}
}

func TestAbrirTraduceLosErrores(t *testing.T) {
	casos := []struct {
		nombre string
		codigo int
		quiero error
	}{
		{"no existe", http.StatusNotFound, ErrFicheroNoExiste},
		{"prohibido", http.StatusForbidden, ErrFicheroProhibido},
		{"caído", http.StatusInternalServerError, ErrNoDisponible},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(c.codigo) }))
			defer srv.Close()
			_, _, err := New(srv.URL, "").Abrir(context.Background(), "tok", idFichero)
			if !errors.Is(err, c.quiero) {
				t.Fatalf("err = %v; quiero %v", err, c.quiero)
			}
		})
	}
}

func TestAbrirExigeContentLength(t *testing.T) {
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Sin Content-Length: la respuesta va troceada.
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("abc"))
	}))
	defer drive.Close()
	_, _, err := New(drive.URL, "").Abrir(context.Background(), "tok", idFichero)
	if !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("err = %v; quiero ErrNoDisponible", err)
	}
}

func TestAbrirRechazaUnaLocationQueNoEsUnaDescarga(t *testing.T) {
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "file:///etc/passwd")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer drive.Close()
	_, _, err := New(drive.URL, "").Abrir(context.Background(), "tok", idFichero)
	if !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("err = %v; quiero ErrNoDisponible", err)
	}
}

func TestUnCorteAMitadSeNotaAlLeer(t *testing.T) {
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(1000))
		_, _ = w.Write([]byte("solo un trozo"))
		// El manejador vuelve sin escribir los 1000: el servidor corta la conexión.
	}))
	defer drive.Close()
	cuerpo, n, err := New(drive.URL, "").Abrir(context.Background(), "tok", idFichero)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cuerpo.Close() }()
	if n != 1000 {
		t.Fatalf("n = %d", n)
	}
	if _, err := io.ReadAll(cuerpo); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("leer = %v; quiero io.ErrUnexpectedEOF", err)
	}
}
