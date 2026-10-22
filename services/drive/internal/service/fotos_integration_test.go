// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// La foto que Bob añade al álbum de Ada se COPIA al Drive de Ada: el fichero nuevo es
// suyo, va a /Fotos y cuenta contra la cuota; añadirla otra vez no la copia de nuevo; y
// si no cabe, es ErrQuotaExceeded, no una copia a medias.
func TestCopiaParaFotos(t *testing.T) {
	svc, _, _, ctx := newChannelService(t)
	ctxBob := store.WithOwner(ctx, "bob")
	ctxAda := store.WithOwner(ctx, "ada")
	root, err := svc.Root(ctxBob)
	if err != nil {
		t.Fatal(err)
	}
	src, err := svc.UploadFile(ctxBob, root.ID, "cala.jpg", "image/jpeg", "bob", strings.NewReader("JPEG de Bob"))
	if err != nil {
		t.Fatal(err)
	}
	antes, _ := svc.store.GetQuota(ctx)

	// Solo se copia un fichero DE quien lo añade: Carla no puede mandar el de Bob.
	if _, _, err := svc.CopiaParaFotos(ctx, src.ID, "ada", "carla"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("copiar un fichero ajeno: %v", err)
	}

	copia, reusada, err := svc.CopiaParaFotos(ctx, src.ID, "ada", "bob")
	if err != nil || reusada {
		t.Fatalf("copiar: reusada=%v err=%v", reusada, err)
	}
	if copia.ID == src.ID || copia.OwnerSub != "ada" || copia.SizeBytes != src.SizeBytes {
		t.Fatalf("la copia no es de Ada o no es una copia: %+v", copia)
	}
	carpeta, err := svc.ResolvePath(ctxAda, CarpetaFotos)
	if err != nil || !carpeta.IsDir() || copia.FolderID != carpeta.Folder.ID {
		t.Fatalf("la copia no va a /Fotos: %+v %v", copia, err)
	}
	despues, _ := svc.store.GetQuota(ctx)
	if despues.UsedBytes != antes.UsedBytes+src.SizeBytes {
		t.Errorf("la copia no cuenta en la cuota: %d → %d", antes.UsedBytes, despues.UsedBytes)
	}

	otra, reusada, err := svc.CopiaParaFotos(ctx, src.ID, "ada", "bob")
	if err != nil || !reusada || otra.ID != copia.ID {
		t.Fatalf("añadir la misma foto otra vez la copia de nuevo: %+v reusada=%v err=%v", otra, reusada, err)
	}
	if q, _ := svc.store.GetQuota(ctx); q.UsedBytes != despues.UsedBytes {
		t.Errorf("reutilizar cobra cuota: %d → %d", despues.UsedBytes, q.UsedBytes)
	}

	grande, err := svc.UploadFile(ctxBob, root.ID, "grande.jpg", "image/jpeg", "bob", strings.NewReader(strings.Repeat("x", 4096)))
	if err != nil {
		t.Fatal(err)
	}
	q, _ := svc.store.GetQuota(ctx)
	if _, err := svc.SetQuotaLimit(ctx, q.UsedBytes+10); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CopiaParaFotos(ctx, grande.ID, "ada", "bob"); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Errorf("sin espacio: %v", err)
	}
}
