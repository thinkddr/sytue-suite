// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privatedrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
)

type privatePDFStorageProof struct {
	original, survivor, trashed                api.File
	originalBytes, survivorBytes, trashedBytes []byte
}

func privatePDFClient(t *testing.T, repository *privatedrive.Repository) *api.PDF {
	t.Helper()
	pdf, err := api.NewPDF("https://unused.invalid", "https://unused.invalid", nil, nil).WithPrivateDrive(repository)
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

// The caller owns an actual verified Drive SDK identity. This does not assert
// that the separate Android CertiPDF UID has been delegated that identity.
func exercisePrivatePDFStorage(ctx context.Context, t *testing.T, repository *privatedrive.Repository) privatePDFStorageProof {
	t.Helper()
	pdf := privatePDFClient(t, repository)
	created, err := pdf.Motor(ctx, "/pdf/form", map[string]string{"op": "crear", "campos": `[{"tipo":"texto","nombre":"PrivateName","pagina":0,"rect":[20,20,240,45]}]`}, nil, api.PDFFichero{Nombre: "Private original.pdf", Datos: api.PDFEnBlanco()})
	if err != nil {
		t.Fatal("private local form creation", err)
	}
	filled, err := pdf.Motor(ctx, "/pdf/form", map[string]string{"op": "rellenar", "valores": `{"PrivateName":"Ana & José — 東京"}`}, nil, api.PDFFichero{Nombre: "Private original.pdf", Datos: created.Datos})
	if err != nil {
		t.Fatal("private local form fill", err)
	}
	proof := privatePDFStorageProof{originalBytes: append(filled.Datos, []byte("\n% private-pdf-original-storage-sentinel-20261030\n")...)}
	proof.original, err = pdf.Subir(ctx, "Private original.pdf", bytes.NewReader(proof.originalBytes), int64(len(proof.originalBytes)))
	if err != nil {
		t.Fatal("private PDF upload", err)
	}
	content, err := pdf.Contenido(ctx, api.PDFFichero{Nombre: proof.original.Name, Datos: proof.originalBytes}, "")
	if err != nil || content.Total != 1 || len(content.Paginas) != 1 || content.Paginas[0].Rotacion != 0 {
		t.Fatal("private local original metadata", err)
	}
	proof.trashedBytes, err = pdf.Paginas(ctx, api.PDFExtraer, "1,1", 0, "", api.PDFFichero{Nombre: proof.original.Name, Datos: proof.originalBytes})
	if err != nil {
		t.Fatal("private local page extraction", err)
	}
	compressed, err := pdf.Motor(ctx, "/pdf/tools", map[string]string{"op": "comprimir", "nivel": "sin-perdida"}, nil, api.PDFFichero{Nombre: "Private extracted.pdf", Datos: proof.trashedBytes})
	if err != nil || compressed.Cabecera.Get("X-Sytue-Ahorro") == "" {
		t.Fatal("private local compression retained for fresh recovery", err)
	}
	proof.trashedBytes = compressed.Datos
	bookmarked, err := pdf.Motor(ctx, "/pdf/edit", map[string]string{"op": "indice", "marcadores": `[{"Titulo":"Private retained section","Pagina":0,"Nivel":0}]`}, nil, api.PDFFichero{Nombre: "Private extracted.pdf", Datos: proof.trashedBytes})
	if err != nil {
		t.Fatal("private local bookmark retained for fresh recovery", err)
	}
	proof.trashedBytes = bookmarked.Datos
	proof.trashedBytes = append(proof.trashedBytes, []byte("\n% private-pdf-first-derived-storage-sentinel-20261030\n")...)
	proof.trashed, err = pdf.GuardarCopia(ctx, proof.original, "first", proof.trashedBytes)
	if err != nil {
		t.Fatal("private first PDF copy", err)
	}
	applied, err := pdf.Aplicar(ctx, proof.original, api.PDFFichero{Nombre: proof.original.Name, Datos: proof.originalBytes}, "", api.PDFCambio{Op: "paginas", Campos: map[string]string{"op": "rotar", "paginas": "1", "grados": "90"}, Copia: true, Sufijo: "second"})
	proof.survivor, proof.survivorBytes = applied.Fichero, applied.Datos
	if err != nil || proof.original.ID == proof.trashed.ID || proof.original.ID == proof.survivor.ID || proof.trashed.ID == proof.survivor.ID {
		t.Fatal("PDF copies replaced an original", err)
	}
	annotated, err := pdf.Aplicar(ctx, proof.survivor, api.PDFFichero{Nombre: proof.survivor.Name, Datos: proof.survivorBytes}, "", api.PDFCambio{Op: "anotar", Datos: []map[string]any{{"pagina": 0, "tipo": "nota", "rect": []float64{10, 10, 40, 40}, "texto": "private recovered annotation", "autor": "initial note"}}})
	if err != nil {
		t.Fatal("private local annotation persisted through Drive", err)
	}
	proof.survivor, proof.survivorBytes = annotated.Fichero, annotated.Datos
	replied, err := pdf.Aplicar(ctx, proof.survivor, api.PDFFichero{Nombre: proof.survivor.Name, Datos: proof.survivorBytes}, "", api.PDFCambio{Op: "responder", Campos: map[string]string{"indice": "1", "texto": "private recovered reply", "autor": "forged actor"}})
	if err != nil {
		t.Fatal("private local reply persisted through Drive", err)
	}
	proof.survivor, proof.survivorBytes = replied.Fichero, replied.Datos
	staleFile, staleBytes := proof.survivor, proof.survivorBytes
	metadata, err := pdf.Aplicar(ctx, proof.survivor, api.PDFFichero{Nombre: proof.survivor.Name, Datos: proof.survivorBytes}, "", api.PDFCambio{Op: "metadatos", Datos: map[string]string{"titulo": "Private recovered title", "autor": "Private recovered author"}})
	if err != nil {
		t.Fatal("private local metadata persisted through Drive", err)
	}
	proof.survivor, proof.survivorBytes = metadata.Fichero, metadata.Datos
	stale, err := pdf.Aplicar(ctx, staleFile, api.PDFFichero{Nombre: staleFile.Name, Datos: staleBytes}, "", api.PDFCambio{Op: "metadatos", Datos: map[string]string{"titulo": "Stale title must not overwrite"}})
	if !errors.Is(err, privatedrive.ErrConflict) || len(stale.Datos) != 0 {
		t.Fatal("stale private PDF silently replaced a newer version", err)
	}
	proof.trashed, err = pdf.Renombrar(ctx, proof.trashed.ID, "Private renamed copy")
	if err != nil || proof.trashed.Name != "Private renamed copy.pdf" {
		t.Fatal("private PDF rename", err)
	}
	checkPrivatePDFOriginal(ctx, t, pdf, proof.original, proof.originalBytes)
	if err = pdf.Eliminar(ctx, proof.trashed.ID); err != nil {
		t.Fatal("private PDF copy deletion", err)
	}
	checkPrivatePDFOriginal(ctx, t, pdf, proof.original, proof.originalBytes)
	checkPrivatePDFOriginal(ctx, t, pdf, proof.survivor, proof.survivorBytes)
	return proof
}

func checkPrivatePDFOriginal(ctx context.Context, t *testing.T, pdf *api.PDF, file api.File, want []byte) {
	t.Helper()
	actual, err := pdf.PorRuta(ctx, "/"+api.PDFCarpeta+"/"+file.Name)
	if err != nil || actual.ID != file.ID || actual.Version != file.Version {
		t.Fatal("PDF operation changed another original's identity or version", err)
	}
	raw, err := pdf.Descargar(ctx, file.ID)
	defer clear(raw)
	if err != nil || !bytes.Equal(raw, want) {
		t.Fatal("PDF operation changed original or independent copy bytes", err)
	}
}

func checkRecoveredPrivatePDFStorage(ctx context.Context, t *testing.T, repository *privatedrive.Repository, proof privatePDFStorageProof, gateway *fileGatewayFixture) {
	t.Helper()
	pdf := privatePDFClient(t, repository)
	checkPrivatePDFOriginal(ctx, t, pdf, proof.original, proof.originalBytes)
	checkPrivatePDFOriginal(ctx, t, pdf, proof.survivor, proof.survivorBytes)
	actualForm, err := pdf.Descargar(ctx, proof.original.ID)
	if err != nil {
		t.Fatal("fresh private form download", err)
	}
	form, err := pdf.Contenido(ctx, api.PDFFichero{Nombre: proof.original.Name, Datos: actualForm}, "")
	if err != nil || len(form.Campos) != 1 || form.Campos[0].Nombre != "PrivateName" || form.Campos[0].Valor != "Ana & José — 東京" {
		t.Fatal("fresh independent device lost private form data", err)
	}
	exported, err := pdf.Motor(ctx, "/pdf/form", map[string]string{"op": "exportar-datos"}, nil, api.PDFFichero{Nombre: proof.original.Name, Datos: actualForm})
	clear(actualForm)
	if err != nil || exported.Cabecera.Get("Content-Type") != "application/vnd.adobe.xfdf" || !bytes.Contains(exported.Datos, []byte("Ana &amp; José")) {
		t.Fatal("fresh local XFDF export lost recovered data", err)
	}
	clear(exported.Datos)

	content, err := pdf.Contenido(ctx, api.PDFFichero{Nombre: proof.survivor.Name, Datos: proof.survivorBytes}, "")
	if err != nil || content.Total != 1 || len(content.Paginas) != 1 || content.Paginas[0].Rotacion != 90 {
		t.Fatal("fresh local rotated PDF metadata", err)
	}
	if len(content.Anotaciones) != 2 || content.Anotaciones[0].Texto != "private recovered annotation" || content.Anotaciones[1].Texto != "private recovered reply" || content.Anotaciones[1].Autor != repository.PrivateStorageScope().Subject {
		t.Fatal("recovered PDF annotations or authenticated reply actor lost", content.Anotaciones)
	}
	if content.Huella.Titulo != "Private recovered title" || content.Huella.Autor != "Private recovered author" {
		t.Fatal("recovered private PDF metadata lost", content.Huella)
	}
	raster, err := pdf.RenderPNG(ctx, api.PDFFichero{Nombre: proof.survivor.Name, Datos: proof.survivorBytes}, "1", 1, "")
	if err != nil || len(raster) != 1 {
		t.Fatal("fresh private PDF local raster", err)
	}
	image, err := png.Decode(bytes.NewReader(raster[1]))
	if err != nil || image.Bounds().Dx() <= image.Bounds().Dy() {
		t.Fatal("fresh PDF rotation was not rendered locally", err)
	}
	files, err := pdf.Lista(ctx)
	if err != nil || len(files) != 2 || !slices.ContainsFunc(files, func(f api.File) bool { return f.ID == proof.original.ID }) || !slices.ContainsFunc(files, func(f api.File) bool { return f.ID == proof.survivor.ID }) {
		t.Fatal("fresh PDF listing lost survivors or revived deleted copy", files, err)
	}
	if _, err := pdf.PorRuta(ctx, "/"+api.PDFCarpeta+"/"+proof.trashed.Name); err == nil {
		t.Fatal("deleted PDF was exposed as live")
	}
	trash, err := repository.Trash(ctx)
	if err != nil || !slices.ContainsFunc(trash.Files, func(f api.File) bool { return f.ID == proof.trashed.ID }) {
		t.Fatal("fresh PDF copy trash lost", err)
	}
	if err := repository.Restore(ctx, api.DriveFile, proof.trashed.ID); err != nil {
		t.Fatal("fresh PDF copy restore", err)
	}
	checkPrivatePDFOriginal(ctx, t, pdf, proof.trashed, proof.trashedBytes)
	if err := pdf.Eliminar(ctx, proof.trashed.ID); err != nil {
		t.Fatal("fresh PDF copy deletion", err)
	}
	checkPrivatePDFOriginal(ctx, t, pdf, proof.original, proof.originalBytes)
	checkPrivatePDFOriginal(ctx, t, pdf, proof.survivor, proof.survivorBytes)
	gateway.mu.Lock()
	for _, ciphertext := range gateway.data {
		if bytes.Contains(ciphertext, []byte("private-pdf-original-storage-sentinel-20261030")) || bytes.Contains(ciphertext, []byte("private-pdf-first-derived-storage-sentinel-20261030")) || bytes.Contains(ciphertext, []byte("private-pdf-second-derived-storage-sentinel-20261030")) || bytes.Contains(ciphertext, []byte("%PDF-")) {
			gateway.mu.Unlock()
			t.Fatal("PDF plaintext reached opaque provider storage")
		}
	}
	gateway.mu.Unlock()
	t.Log("PRIVATE_PDF_STORAGE_FRESH_PASS real SDK private upload/open/copies/rename/delete/restore, exact independent originals, local metadata/pages and empty-journal recovery; native delegation/UI and Office CRDT remain separate")
}
