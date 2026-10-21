// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// «Subir a Cast» desde Drive de punta a punta contra un PostgreSQL real: el vídeo
// nace, la copia llega al almacén en segundo plano y deja el vídeo procesándose
// con su trabajo en la cola; y si Drive se corta a mitad, el vídeo queda `failed`
// con un motivo que se puede leer.
package service

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

func servicioConBase(t *testing.T, d *driveFalso) (*Servicio, *store.Store, *objetosEnMemoria) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	cliente := d.servir(t)
	obj := &objetosEnMemoria{}
	return New(Deps{Store: st, Objetos: obj, Cuota: cliente, Drive: cliente, Firmante: media.New("clave")}), st, obj
}

func ctxDePrueba(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSubirDeDriveCopiaYEncola(t *testing.T) {
	ctx := ctxDePrueba(t)
	contenido := bytes.Repeat([]byte("fotograma "), 100_000) // 1 MB, en varios trozos de red
	d := &driveFalso{nombre: "Clase 3.mp4", tipo: "video/mp4", tamano: int64(len(contenido)), contenido: contenido, libres: 1 << 30}
	svc, st, obj := servicioConBase(t, d)

	v, err := svc.SubirDeDrive(ctx, ana, DeDrive{FileID: idDeDrive})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != domain.StatusUploading || v.Title != "Clase 3" || v.Visibility != domain.VisibilityOrg || v.OwnerSub != "ana" {
		t.Fatalf("recién creado = %s %q %s %s", v.Status, v.Title, v.Visibility, v.OwnerSub)
	}
	svc.EsperarCopias()

	v, err = st.EnOrganizacion(ctx, "acme", v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != domain.StatusProcessing || v.BytesOriginal != int64(len(contenido)) || v.OriginalKey != Prefijo(v)+media.RutaOriginal {
		t.Fatalf("tras copiar = %s, %d bytes, clave %q", v.Status, v.BytesOriginal, v.OriginalKey)
	}
	if got := obj.datos["acme/"+v.OriginalKey]; !bytes.Equal(got, contenido) {
		t.Fatalf("el almacén tiene %d bytes; quiero los %d de Drive", len(got), len(contenido))
	}
	j, err := st.Reclamar(ctx, "prueba")
	if err != nil {
		t.Fatalf("no hay trabajo en la cola: %v", err)
	}
	if j.VideoID != v.ID || j.Kind != domain.JobUpload {
		t.Fatalf("trabajo = %+v", j)
	}
}

func TestSubirDeDriveConTituloYVisibilidad(t *testing.T) {
	ctx := ctxDePrueba(t)
	d := &driveFalso{nombre: "x.webm", tipo: "video/webm", tamano: 3, contenido: []byte("abc"), libres: 1 << 30}
	svc, _, _ := servicioConBase(t, d)
	v, err := svc.SubirDeDrive(ctx, ana, DeDrive{FileID: idDeDrive, Title: "  Mi  vídeo ", Description: "desc", Visibility: domain.VisibilityUnlisted})
	if err != nil {
		t.Fatal(err)
	}
	svc.EsperarCopias()
	if v.Title != "Mi vídeo" || v.Description != "desc" || v.Visibility != domain.VisibilityUnlisted {
		t.Fatalf("vídeo = %q %q %s", v.Title, v.Description, v.Visibility)
	}
}

func TestSubirDeDriveCortadoAMitadQuedaFallido(t *testing.T) {
	ctx := ctxDePrueba(t)
	contenido := bytes.Repeat([]byte("x"), 64<<10)
	d := &driveFalso{nombre: "a.mp4", tipo: "video/mp4", tamano: int64(len(contenido)), contenido: contenido, libres: 1 << 30, cortarEn: 1000}
	svc, st, obj := servicioConBase(t, d)

	v, err := svc.SubirDeDrive(ctx, ana, DeDrive{FileID: idDeDrive})
	if err != nil {
		t.Fatal(err)
	}
	svc.EsperarCopias()

	v, err = st.EnOrganizacion(ctx, "acme", v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != domain.StatusFailed || !strings.Contains(v.Failure, "se cortó") {
		t.Fatalf("tras el corte = %s, motivo %q", v.Status, v.Failure)
	}
	if obj.cuantos() != 0 {
		t.Fatal("quedó un original a medias en el almacén")
	}
	if _, err := st.Reclamar(ctx, "prueba"); err == nil {
		t.Fatal("se encoló un trabajo para un vídeo sin original")
	}
}

func TestSubirDeDriveBorradoMientrasCopiaNoDejaNada(t *testing.T) {
	ctx := ctxDePrueba(t)
	d := &driveFalso{nombre: "a.mp4", tipo: "video/mp4", tamano: 3, contenido: []byte("abc"), libres: 1 << 30}
	svc, st, obj := servicioConBase(t, d)
	// La copia espera turno mientras se borra: se ocupan los cuatro huecos.
	for range maxCopiando {
		svc.turnoDeCopia <- struct{}{}
	}
	v, err := svc.SubirDeDrive(ctx, ana, DeDrive{FileID: idDeDrive})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Borrar(ctx, ana, v.ID); err != nil {
		t.Fatal(err)
	}
	// La purga del borrado termina ANTES de que la copia empiece: si no, sería
	// ella la que se llevara el original y el test no probaría nada.
	svc.enMarcha.Wait()
	for range maxCopiando {
		<-svc.turnoDeCopia
	}
	svc.EsperarCopias()
	if obj.cuantos() != 0 {
		t.Fatal("quedó el original de un vídeo borrado en el almacén")
	}
	if _, err := st.Reclamar(ctx, "prueba"); err == nil {
		t.Fatal("se encoló un trabajo para un vídeo borrado")
	}
}
