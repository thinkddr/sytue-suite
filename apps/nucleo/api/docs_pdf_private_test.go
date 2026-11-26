// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privatePDFDataFixture struct {
	DriveData
	failure       error
	renamed       string
	deleted       string
	roots         int
	rootFailureAt int
}

func (*privatePDFDataFixture) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "owner", Product: "drive"}
}

func (f *privatePDFDataFixture) Root(ctx context.Context) (Folder, error) {
	f.roots++
	if err := ctx.Err(); err != nil {
		return Folder{}, err
	}
	if f.rootFailureAt > 0 && f.roots >= f.rootFailureAt {
		return Folder{}, errors.New("authorization revoked during local PDF processing")
	}
	return Folder{ID: "root"}, f.failure
}

func (f *privatePDFDataFixture) Rename(_ context.Context, k DriveKind, id, name string) error {
	if k != DriveFile || id != "private-copy" {
		return errors.New("wrong private rename target")
	}
	if f.failure == nil {
		f.renamed = name
	}
	return f.failure
}

func (f *privatePDFDataFixture) File(_ context.Context, id string) (File, error) {
	return File{ID: id, Name: f.renamed}, f.failure
}

func (f *privatePDFDataFixture) Delete(_ context.Context, k DriveKind, id string) error {
	if k != DriveFile || id != "private-copy" {
		return errors.New("wrong private delete target")
	}
	if f.failure == nil {
		f.deleted = id
	}
	return f.failure
}

func TestPrivatePDFStorageDispatchesWithoutChangingOriginalOrSendingContents(t *testing.T) {
	calls := 0
	original := NewPDF("https://docs.example", "https://drive.example", nil, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"legacy"}`)), Header: http.Header{}, Request: r}, nil
	})})
	data := &privatePDFDataFixture{}
	private, err := original.WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	if actual, err := private.Renombrar(t.Context(), "private-copy", "Renamed private"); err != nil || actual.Name != "Renamed private.pdf" || data.renamed != actual.Name || calls != 0 {
		t.Fatal("private PDF rename used legacy HTTP", actual, err, calls)
	}
	if err := private.Eliminar(t.Context(), "private-copy"); err != nil || data.deleted != "private-copy" || calls != 0 {
		t.Fatal("private PDF delete used legacy HTTP", err, calls)
	}
	unavailable := errors.New("private work authorization changed")
	data.failure = unavailable
	if _, err := private.Renombrar(t.Context(), "private-copy", "stale"); !errors.Is(err, unavailable) || calls != 0 {
		t.Fatal("private rename failure fell back to legacy", err, calls)
	}
	if err := private.Eliminar(t.Context(), "private-copy"); !errors.Is(err, unavailable) || calls != 0 {
		t.Fatal("private delete failure fell back to legacy", err, calls)
	}
	if _, err := private.Contenido(t.Context(), PDFFichero{Nombre: "private.pdf", Datos: PDFEnBlanco()}, ""); !errors.Is(err, unavailable) || calls != 0 {
		t.Fatal("private PDF content ignored revoked authorization", err, calls)
	}
	if _, err := private.Motor(t.Context(), "/pdf/signature-requests", nil, nil, PDFFichero{Nombre: "private.pdf", Datos: PDFEnBlanco()}); !errors.Is(err, ErrPrivatePDFOperation) || calls != 0 {
		t.Fatal("private PDF edit reached the server motor", err, calls)
	}
	if _, err := original.Renombrar(t.Context(), "legacy", "legacy"); err != nil || calls != 1 {
		t.Fatal("private facade changed original client", err, calls)
	}
	if _, err := private.WithPrivateDrive(data); !errors.Is(err, ErrPrivatePDFOperation) {
		t.Fatal("nested private PDF facade accepted", err)
	}
	if _, err := original.WithPrivateDrive(nil); !errors.Is(err, ErrPrivatePDFOperation) {
		t.Fatal("nil private repository accepted", err)
	}
}

func TestPrivatePDFLocalPagesPreserveOriginalAndNeverUpload(t *testing.T) {
	calls := 0
	original := NewPDF("https://docs.example", "https://drive.example", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected plaintext transport")
	})})
	data := &privatePDFDataFixture{}
	private, err := original.WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	source := PDFEnBlanco()
	before := bytes.Clone(source)
	doc := PDFFichero{Nombre: "Private.pdf", Datos: source}
	content, err := private.Contenido(t.Context(), doc, "")
	if err != nil || content.Total != 1 || len(content.Paginas) != 1 || content.Paginas[0].Rotacion != 0 {
		t.Fatal("local metadata", content, err)
	}
	combined, err := private.Paginas(t.Context(), PDFCombinar, "", 0, "", doc, doc)
	if err != nil {
		t.Fatal("local combination", err)
	}
	for _, tc := range []struct {
		op, pages                        string
		degrees, pagesWant, rotationWant int
	}{
		{PDFExtraer, "2,1,2", 0, 3, 0},
		{PDFBorrar, "2", 0, 1, 0},
		{PDFRotar, "2-1", 90, 2, 90},
	} {
		t.Run(tc.op, func(t *testing.T) {
			raw, err := private.Paginas(t.Context(), tc.op, tc.pages, tc.degrees, "", PDFFichero{Nombre: "combined.pdf", Datos: combined})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := localpdf.LeerPDF(raw)
			if err != nil || parsed.NumPaginas() != tc.pagesWant {
				t.Fatal("local result pages", err)
			}
			for i := 0; i < parsed.NumPaginas(); i++ {
				page, err := parsed.Pagina(i)
				if err != nil || page.Rotacion != tc.rotationWant {
					t.Fatal("local result rotation", i, page, err)
				}
			}
		})
	}
	if calls != 0 || !bytes.Equal(source, before) {
		t.Fatal("local operation exposed plaintext or changed original", calls)
	}
}

func TestPrivatePDFLocalErrorsAndAuthorization(t *testing.T) {
	data := &privatePDFDataFixture{}
	private, err := NewPDF("https://unused.invalid", "https://unused.invalid", nil, nil).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := PDFFichero{Nombre: "Private.pdf", Datos: PDFEnBlanco()}
	for _, selection := range []string{"2-1", "1-0", "1-2", "", "1-999999999999999999999999999"} {
		if out, err := private.Paginas(t.Context(), PDFExtraer, selection, 0, "", doc); err == nil || len(out) > 0 {
			t.Fatal("invalid selection returned PDF", selection)
		}
	}
	if _, err := private.Paginas(t.Context(), PDFBorrar, "1", 0, "", doc); PDFMensaje(err) != "All the pages of the document cannot be deleted." {
		t.Fatal("delete all pages error", err)
	}
	if _, err := private.Contenido(t.Context(), PDFFichero{Nombre: "not-pdf", Datos: []byte("not pdf")}, ""); PDFMensaje(err) != "“not-pdf” is not a PDF." {
		t.Fatal("read error lost useful code/message", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if out, err := private.Paginas(ctx, PDFRotar, "1", 90, "", doc); !errors.Is(err, context.Canceled) || len(out) > 0 {
		t.Fatal("cancelled processing returned PDF", err)
	}
	data.rootFailureAt = data.roots + 2
	if out, err := private.Paginas(t.Context(), PDFRotar, "1", 90, "", doc); err == nil || len(out) > 0 {
		t.Fatal("revoked authorization returned output", err)
	}
	data.rootFailureAt = 0
	data.failure = errors.New("identity changed")
	if _, err := private.Contenido(t.Context(), doc, ""); !errors.Is(err, data.failure) {
		t.Fatal("metadata ignored authorization", err)
	}
}

func TestPrivatePDFPasswordAndSignatureConfirmation(t *testing.T) {
	private, err := NewPDF("https://unused.invalid", "https://unused.invalid", nil, nil).WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := localpdf.LeerPDF(PDFEnBlanco())
	if err != nil {
		t.Fatal(err)
	}
	protected, err := localpdf.ProtegerPDF(doc, localpdf.OpcionesProteccion{ContrasenaUsuario: "synthetic-private-pdf-password", Permitir: localpdf.PermisosTodos()})
	if err != nil {
		t.Fatal(err)
	}
	file := PDFFichero{Nombre: "protected.pdf", Datos: protected}
	if _, err := private.Contenido(t.Context(), file, ""); !PDFPideClave(err) {
		t.Fatal("password prompt lost", err)
	}
	if _, err := private.Contenido(t.Context(), file, "wrong"); !PDFPideClave(err) {
		t.Fatal("wrong password accepted", err)
	}
	if content, err := private.Contenido(t.Context(), file, "synthetic-private-pdf-password"); err != nil || content.Total != 1 {
		t.Fatal("correct password rejected", err)
	}
	signed, err := os.ReadFile(filepath.Join("..", "pdf", "testdata", "pdf", "certificacion-p1.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := private.Paginas(t.Context(), PDFRotar, "1", 90, "", PDFFichero{Nombre: "signed.pdf", Datos: signed})
	var failure *Error
	if len(out) > 0 || !errors.As(err, &failure) || failure.Status != http.StatusConflict || failure.Code() != "pdf_invalidates_signatures" {
		t.Fatal("signed document rewritten without confirmation", err)
	}

	response, err := private.Motor(t.Context(), "/pdf/pages", map[string]string{"op": "rotar", "paginas": "1", "grados": "90", "invalidar_firmas": "1"}, nil, PDFFichero{Nombre: "signed.pdf", Datos: signed})
	if err != nil || len(response.Datos) == 0 {
		t.Fatal("explicit signature confirmation rejected", err)
	}
	metadata, err := private.Motor(t.Context(), "/pdf/content", nil, nil, PDFFichero{Nombre: "signed.pdf", Datos: signed})
	if err != nil || !bytes.Contains(metadata.Datos, []byte(`"caja"`)) {
		t.Fatal("native studio metadata lost page coordinates", err)
	}
}
