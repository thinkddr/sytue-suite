// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"bytes"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// La IP del registro no se la puede inventar quien llama. Antes se leía la
// PRIMERA entrada de X-Forwarded-For sin mirar quién conectaba, que es
// exactamente la que escribe el cliente: la columna de IP de TODOS los servicios
// era falsificable (auditoría del 2026-09-21, docs/security.md §6.3).
func TestLaIPDelRegistroNoSeLaInventaElCliente(t *testing.T) {
	casos := []struct {
		que, peer, xff, quiero string
	}{
		{"peer que no es proxy nuestro: su cabecera no se mira", "203.0.113.9:4444", "198.51.100.1", "203.0.113.9"},
		{"peer de confianza: la entrada de la derecha que no sea proxy nuestro", "10.0.0.5:5555", "198.51.100.1, 10.0.0.9", "198.51.100.1"},
		{"lo que el cliente añade a la izquierda no se mira", "10.0.0.5:5555", "1.2.3.4, 198.51.100.1, 10.0.0.9", "198.51.100.1"},
		{"sin cabecera: el peer", "10.0.0.5:5555", "", "10.0.0.5"},
		{"IPv6 sin corchetes ni puerto, que es lo que admite la columna", "[::1]:54321", "", "::1"},
	}
	for _, c := range casos {
		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if ip := clientIP(r); ip != c.quiero {
			t.Errorf("%s: %q, quiero %q", c.que, ip, c.quiero)
		}
	}
}

// GUARDIÁN: ninguna credencial de la URL llega al log de peticiones (ADR 0052). Una
// ruta con un token —el de un enlace compartido— se registra por su PATRÓN.
func TestRequestLoggerNoRegistraLaURL(t *testing.T) {
	var buf bytes.Buffer
	r := NewRouter(RouterOptions{Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
	r.Get("/api/v1/share/{token}", func(w http.ResponseWriter, r *http.Request) {
		log.From(r.Context()).Info("dentro") // lo que registre el handler, tampoco
		w.WriteHeader(http.StatusNoContent)
	})
	const secreto = "c0ffee5ecret70ken"
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/share/"+secreto+"?t="+secreto, http.NoBody))
	if strings.Contains(buf.String(), secreto) {
		t.Fatalf("el token aparece en el log:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"route":"/api/v1/share/{token}"`) {
		t.Fatalf("el log no lleva el patrón de la ruta:\n%s", buf.String())
	}
}

// GUARDIÁN (ADR 0052): un Traefik de la suite que escriba log de accesos no apunta la
// ruta, que puede llevar una credencial. Cualquier fichero de configuración de Traefik
// del repositorio que active `accessLog` tiene que quitar RequestPath y RequestLine.
func TestLosLogsDeAccesoNoLlevanLaRuta(t *testing.T) {
	raiz := "../../.."
	vistos := 0
	err := filepath.WalkDir(filepath.Join(raiz, "deploy"), func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".vendor" || d.Name() == "vendor" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(ruta); ext != ".yml" && ext != ".yaml" && ext != ".toml" {
			return nil
		}
		b, err := os.ReadFile(ruta)
		if err != nil {
			return err
		}
		txt := string(b)
		if !strings.Contains(txt, "accessLog") && !strings.Contains(txt, "accesslog") {
			return nil
		}
		vistos++
		if !strings.Contains(txt, "RequestPath: drop") || !strings.Contains(txt, "RequestLine: drop") {
			t.Errorf("%s activa el log de accesos de Traefik con la ruta (quita RequestPath y RequestLine)", ruta)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if vistos == 0 {
		t.Fatal("no se encontró ninguna configuración de Traefik con log de accesos: ¿se movió deploy/?")
	}
	// Y el nginx del frontal (y cualquier otro): sin log de accesos, que escribe la
	// línea de la petición entera.
	_ = filepath.WalkDir(filepath.Join(raiz, "deploy"), func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(ruta) != ".conf" {
			return err
		}
		b, _ := os.ReadFile(ruta)
		// nginx escribe el log de accesos POR DEFECTO: un `http {}` tiene que decir que no.
		if strings.Contains(string(b), "http {") && !strings.Contains(string(b), "access_log off") {
			t.Errorf("%s no apaga el log de accesos de nginx (access_log off), que lo escribe por defecto", ruta)
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "access_log") && !strings.HasPrefix(l, "access_log off") {
				t.Errorf("%s: %q escribe la URL entera en el log de accesos", ruta, l)
			}
		}
		return nil
	})
}
