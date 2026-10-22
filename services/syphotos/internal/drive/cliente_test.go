// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// almacen simula el object storage: solo atiende con la credencial en la QUERY,
// nunca con una cabecera. Es lo que hace que el test detecte una fuga de token.
func almacen(t *testing.T, contenido string, vistoAuth *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*vistoAuth = r.Header.Get("Authorization")
		rango := r.Header.Get("Range")
		if rango == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(contenido))
			return
		}
		var fin int
		if _, err := fmtSscan(rango, &fin); err != nil {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if fin >= len(contenido) {
			fin = len(contenido) - 1
		}
		w.Header().Set("Content-Range", "bytes 0-"+itoa(fin)+"/"+itoa(len(contenido)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(contenido[:fin+1]))
	}))
}

// LA LECTURA POR RANGO ES LA OPERACIÓN QUE MÁS AHORRA: se comprueba que de
// verdad pide un rango y que no se trae el fichero entero.
func TestCabecera_pideSoloLoNecesario(t *testing.T) {
	entero := strings.Repeat("A", 50_000) + "COLA"
	var authEnAlmacen string
	obj := almacen(t, entero, &authEnAlmacen)
	defer obj.Close()

	drv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("presign") != "1" {
			t.Errorf("no se pidió la URL prefirmada: %s", r.URL)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Drive recibió %q en vez del bearer", got)
		}
		http.Redirect(w, r, obj.URL+"/objeto?X-Amz-Signature=abc", http.StatusTemporaryRedirect)
	}))
	defer drv.Close()

	c := Nuevo(drv.URL, 5*time.Second)
	b, err := c.Cabecera(context.Background(), "tok", "f1", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1024 {
		t.Errorf("volvieron %d bytes, se pidieron 1024", len(b))
	}
	if strings.Contains(string(b), "COLA") {
		t.Error("se trajo el fichero entero pese a pedir un rango")
	}
}

// EL TOKEN DE LA PERSONA NO PUEDE LLEGAR AL ALMACÉN DE OBJETOS. Es la razón de
// dar el segundo salto a mano en vez de dejar que el cliente siga la redirección.
func TestCabecera_noFiltraElTokenAlAlmacen(t *testing.T) {
	var authEnAlmacen string
	obj := almacen(t, strings.Repeat("x", 100), &authEnAlmacen)
	defer obj.Close()

	drv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, obj.URL+"/objeto?X-Amz-Signature=abc", http.StatusTemporaryRedirect)
	}))
	defer drv.Close()

	c := Nuevo(drv.URL, 5*time.Second)
	if _, err := c.Cabecera(context.Background(), "secreto-de-la-persona", "f1", 50); err != nil {
		t.Fatal(err)
	}
	if authEnAlmacen != "" {
		t.Errorf("el almacén recibió una cabecera de autorización: %q", authEnAlmacen)
	}
}

// Si el almacén IGNORA el rango y manda el fichero entero, el corte lo tiene que
// hacer el lector y no la confianza en el servidor.
func TestCabecera_cortaAunqueElAlmacenIgnoreElRango(t *testing.T) {
	obj := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("z", 500_000)))
	}))
	defer obj.Close()
	drv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, obj.URL, http.StatusTemporaryRedirect)
	}))
	defer drv.Close()

	b, err := Nuevo(drv.URL, 5*time.Second).Cabecera(context.Background(), "t", "f1", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 4096 {
		t.Errorf("se leyeron %d bytes con un servidor que ignora el rango", len(b))
	}
}

// Una Location relativa o con otro esquema convertiría el segundo salto en algo
// que no es una descarga.
func TestCabecera_rechazaUnaLocationInutilizable(t *testing.T) {
	for _, loc := range []string{"", "/relativa", "file:///etc/passwd", "javascript:alert(1)"} {
		drv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if loc != "" {
				w.Header().Set("Location", loc)
			}
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		_, err := Nuevo(drv.URL, 5*time.Second).Cabecera(context.Background(), "t", "f1", 10)
		if err == nil {
			t.Errorf("se aceptó la Location %q", loc)
		}
		drv.Close()
	}
}

// No existe y sin permiso son el MISMO error desde fuera: distinguirlos filtra
// qué ficheros hay.
func TestErrores_noEncontradoYSinPermisoSonElMismo(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		c := Nuevo(srv.URL, 5*time.Second)
		if _, err := c.Metadatos(context.Background(), "t", "f1"); !errors.Is(err, ErrNoEncontrado) {
			t.Errorf("metadatos con %d: %v", code, err)
		}
		if _, err := c.Abrir(context.Background(), "t", "f1"); !errors.Is(err, ErrNoEncontrado) {
			t.Errorf("abrir con %d: %v", code, err)
		}
		srv.Close()
	}
}

func TestMetadatos_leeLoQueDevuelveDrive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// La forma REAL de `domain.File` de Drive: snake_case, como toda su API. Este
		// servidor falso contestaba en camelCase, igual que las etiquetas del cliente,
		// así que el test salía verde mientras cada foto se guardaba con tipo vacío y
		// tamaño 0.
		_, _ = w.Write([]byte(`{"id":"f1","name":"playa.jpg","content_type":"image/jpeg","size_bytes":25000000}`))
	}))
	defer srv.Close()

	f, err := Nuevo(srv.URL, 5*time.Second).Metadatos(context.Background(), "t", "f1")
	if err != nil {
		t.Fatal(err)
	}
	if f.Nombre != "playa.jpg" || f.ContentType != "image/jpeg" || f.Bytes != 25_000_000 {
		t.Errorf("volvió %+v", f)
	}
}

// ── ayudas ───────────────────────────────────────────────────────────────────

// fmtSscan lee el final de un «bytes=0-N».
func fmtSscan(rango string, fin *int) (int, error) {
	i := strings.LastIndexByte(rango, '-')
	if !strings.HasPrefix(rango, "bytes=0-") || i < 0 {
		return 0, errors.New("rango no soportado")
	}
	n := 0
	for _, r := range rango[i+1:] {
		if r < '0' || r > '9' {
			return 0, errors.New("rango ilegible")
		}
		n = n*10 + int(r-'0')
	}
	*fin = n
	return 1, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// LAS RUTAS SON PARTE DEL CONTRATO, y aquí no lo eran: los servidores falsos de
// los demás tests contestan a cualquier URL, así que el cliente estuvo pidiendo
// `/api/files/…` —sin el `/v1` que monta Drive— y todo salía verde mientras
// subir una foto daba 500 en producción. Este test fija el prefijo de verdad.
func TestRutas_llevanElPrefijoDeDrive(t *testing.T) {
	const id = "f-123"
	var vistas []string
	obj := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("bytes"))
	}))
	defer obj.Close()

	drv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vistas = append(vistas, r.URL.Path)
		if !strings.HasPrefix(r.URL.Path, "/api/v1/files/") {
			// Drive no tiene nada fuera de /api/v1: responde como responde él.
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("presign") == "1" {
			http.Redirect(w, r, obj.URL+"/objeto", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + id + `","name":"f.jpg","content_type":"image/jpeg","size_bytes":9}`))
	}))
	defer drv.Close()

	c := Nuevo(drv.URL, 5*time.Second)
	f, err := c.Metadatos(context.Background(), "tok", id)
	if err != nil {
		t.Fatalf("Metadatos: %v", err)
	}
	if f.ContentType == "" || f.Bytes == 0 {
		t.Errorf("Metadatos volvió tipo %q y %d bytes: las etiquetas no casan con el JSON de Drive", f.ContentType, f.Bytes)
	}
	if _, err := c.Cabecera(context.Background(), "tok", id, 128); err != nil {
		t.Fatalf("Cabecera: %v", err)
	}
	for _, p := range vistas {
		if !strings.HasPrefix(p, "/api/v1/files/") {
			t.Errorf("pidió %q, que en Drive es un 404: el prefijo es /api/v1/files/", p)
		}
	}
	if len(vistas) == 0 {
		t.Fatal("no se registró ninguna llamada")
	}
}
