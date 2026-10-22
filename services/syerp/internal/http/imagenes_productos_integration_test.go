// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestImagenesProductosPermisosYAdjunto(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	hs := manejadoresCompletos(t, st)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	w := comoPersona(t, "ada", []string{"owner"}, http.MethodPost, "/api/v1/producto",
		`{"data":{"nombre":"Asesoría","tipo":"servicio","precio":5000}}`,
		map[string]string{"modelo": "producto"}, hs.exigirModelo(http.HandlerFunc(hs.Crear)))
	var producto store.Registro
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &producto) != nil {
		t.Fatalf("crear producto: %d %s", w.Code, w.Body.String())
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jEtkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	w = comoPersona(t, "ada", []string{"owner"}, http.MethodPost,
		"/api/v1/adjuntos?modelo=producto&registro="+producto.ID+"&nombre=asesoria.png", string(png), nil,
		http.HandlerFunc(hs.SubirAdjunto))
	var subida struct {
		Adjunto store.Adjunto `json:"adjunto"`
	}
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &subida) != nil {
		t.Fatalf("reutiliza la subida autenticada: %d %s", w.Code, w.Body.String())
	}
	if err := st.GuardarPermisos(ctx, "lector", "lector@acme.es", "Lector", map[string]string{"productos": "lectura"}, nil, "ada"); err != nil {
		t.Fatal(err)
	}
	w = comoPersona(t, "lector", []string{"member"}, http.MethodGet,
		"/api/v1/productos/imagenes?registros="+producto.ID, "", nil, http.HandlerFunc(hs.ImagenesProductos))
	var respuesta struct {
		Imagenes map[string]string `json:"imagenes"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &respuesta) != nil || respuesta.Imagenes[producto.ID] != subida.Adjunto.ID {
		t.Fatalf("lectura basta para mostrar la foto: %d %s", w.Code, w.Body.String())
	}
	w = comoPersona(t, "lector", []string{"member"}, http.MethodGet,
		"/api/v1/adjuntos/"+subida.Adjunto.ID+"?ver=1", "", map[string]string{"id": subida.Adjunto.ID}, http.HandlerFunc(hs.DescargarAdjunto))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline;") {
		t.Fatalf("la descarga existente sirve la imagen con permisos: %d %v", w.Code, w.Header())
	}
	w = comoPersona(t, "lector", []string{"member"}, http.MethodPost,
		"/api/v1/adjuntos?modelo=producto&registro="+producto.ID, string(png), nil, http.HandlerFunc(hs.SubirAdjunto))
	if w.Code != http.StatusForbidden {
		t.Fatalf("un lector no puede cambiar la imagen: %d", w.Code)
	}
	if err := st.GuardarPermisos(ctx, "lector", "lector@acme.es", "Lector", map[string]string{"productos": "ninguno"}, nil, "ada"); err != nil {
		t.Fatal(err)
	}
	w = comoPersona(t, "lector", []string{"member"}, http.MethodGet,
		"/api/v1/productos/imagenes?registros="+producto.ID, "", nil, http.HandlerFunc(hs.ImagenesProductos))
	if w.Code != http.StatusForbidden {
		t.Fatalf("sin permiso de productos tampoco se revela el adjunto: %d", w.Code)
	}
}
