// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TestListChildrenPasaElOrdenAlServicio: `?orden=&sentido=` llega al servicio y
// las subcarpetas de la primera página salen en el mismo orden que los ficheros.
func TestListChildrenPasaElOrdenAlServicio(t *testing.T) {
	hoy := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc := &fakeSvc{childFold: []domain.Folder{
		{ID: uuid.New(), Name: "antigua", UpdatedAt: hoy.Add(-48 * time.Hour)},
		{ID: uuid.New(), Name: "nueva", UpdatedAt: hoy},
	}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.ListChildren(rec, mkReq(http.MethodGet, "/x?orden=modificado&sentido=desc", "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d: %s", rec.Code, rec.Body.String())
	}
	if want := (store.OrdenFicheros{Campo: store.OrdenModificado, Desc: true}); svc.lastOrden != want {
		t.Fatalf("orden al servicio = %+v, quiero %+v", svc.lastOrden, want)
	}
	var got listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Folders) != 2 || got.Folders[0].Name != "nueva" {
		t.Fatalf("subcarpetas por fecha descendente: %+v", got.Folders)
	}
}

// TestListChildrenSinOrdenEsPorNombre: sin parámetros, el orden de siempre.
func TestListChildrenSinOrdenEsPorNombre(t *testing.T) {
	svc := &fakeSvc{}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.ListChildren(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if want := (store.OrdenFicheros{Campo: store.OrdenNombre}); svc.lastOrden != want {
		t.Fatalf("orden por defecto = %+v, quiero %+v", svc.lastOrden, want)
	}
}

// TestOrdenDesconocidoEs400: una errata no se convierte en otro orden en silencio,
// en ninguno de los tres listados que lo aceptan.
func TestOrdenDesconocidoEs400(t *testing.T) {
	casos := []string{"/x?orden=peso", "/x?orden=tamano&sentido=arriba", "/x?orden=Nombre"}
	for _, target := range casos {
		params := map[string]string{"id": uuid.New().String()}
		for nombre, listar := range map[string]http.HandlerFunc{
			"children":      newHandlers(&fakeSvc{}).ListChildren,
			"files":         newHandlers(&fakeSvc{}).ListFolderFiles,
			"team-children": newHandlers(&fakeSvc{}).ListTeamDriveChildren,
		} {
			rec := httptest.NewRecorder()
			listar(rec, mkReq(http.MethodGet, target, "", params))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s %s: código = %d, quiero 400", nombre, target, rec.Code)
			}
		}
	}
}

// TestListFolderFilesYUnidadDeEquipoPasanElOrden: los otros dos listados de
// ficheros también ordenan en el servicio.
func TestListFolderFilesYUnidadDeEquipoPasanElOrden(t *testing.T) {
	want := store.OrdenFicheros{Campo: store.OrdenTamano, Desc: true}

	svc := &fakeSvc{}
	rec := httptest.NewRecorder()
	newHandlers(svc).ListFolderFiles(rec, mkReq(http.MethodGet, "/x?orden=tamano&sentido=desc", "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK || svc.lastOrden != want {
		t.Fatalf("files: código %d, orden %+v", rec.Code, svc.lastOrden)
	}

	svc = &fakeSvc{}
	rec = httptest.NewRecorder()
	newHandlers(svc).ListTeamDriveChildren(rec, mkReq(http.MethodGet, "/x?orden=tamano&sentido=desc", "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK || svc.lastOrden != want {
		t.Fatalf("unidad de equipo: código %d, orden %+v", rec.Code, svc.lastOrden)
	}
}
