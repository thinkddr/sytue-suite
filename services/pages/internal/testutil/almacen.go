// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// Package testutil ofrece un almacén HTTP de pruebas al cliente S3 real.
package testutil

import (
	"crypto/md5" // #nosec G501 -- ETag compatible con S3, no autenticación.
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

type Almacen struct {
	Objetos    *objectstore.Client
	mu         sync.Mutex
	datos      map[string][]byte
	escrituras int
	fallar     bool
	fallarPut  string
}

func NuevoAlmacen(t *testing.T) *Almacen {
	t.Helper()
	a := &Almacen{datos: make(map[string][]byte)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.fallar {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		clave := strings.TrimPrefix(r.URL.Path, "/pages/")
		if r.Method == http.MethodPut {
			if a.fallarPut != "" && strings.HasPrefix(clave, a.fallarPut) {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			var body io.Reader = r.Body
			if strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") || strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
				body = httputil.NewChunkedReader(r.Body)
			}
			b, err := io.ReadAll(body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			a.datos[clave] = b
			a.escrituras++
			hash := md5.Sum(b) // #nosec G401 -- ETag de prueba.
			w.Header().Set("ETag", hex.EncodeToString(hash[:]))
			return
		}
		if r.Method == http.MethodDelete {
			delete(a.datos, clave)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		b, ok := a.datos[clave]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(b)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method != http.MethodHead {
			_, _ = w.Write(b)
		}
	}))
	t.Cleanup(srv.Close)
	var err error
	a.Objetos, err = objectstore.New(objectstore.Config{Endpoint: strings.TrimPrefix(srv.URL, "http://"), Region: "eu-local-1", AccessKey: "test", SecretKey: "test", Bucket: "pages"})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func (a *Almacen) Escribir(clave string, b []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.datos[clave] = b
}
func (a *Almacen) Borrar(clave string) { a.mu.Lock(); defer a.mu.Unlock(); delete(a.datos, clave) }
func (a *Almacen) Leer(clave string) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]byte(nil), a.datos[clave]...)
}
func (a *Almacen) Escrituras() int    { a.mu.Lock(); defer a.mu.Unlock(); return a.escrituras }
func (a *Almacen) Fallar(fallar bool) { a.mu.Lock(); defer a.mu.Unlock(); a.fallar = fallar }

func (a *Almacen) FallarPut(clave string) { a.mu.Lock(); defer a.mu.Unlock(); a.fallarPut = clave }
