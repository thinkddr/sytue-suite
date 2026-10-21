// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// presignSvc arma un servicio con raíz + cuota holgada y un blob store fake con una
// URL prefirmada y un tamaño en S3 configurables. MaxUploadBytes=100 (de newSvc).
func presignSvc(t *testing.T) (*Service, *fakeStore, *fakeBlobs) {
	t.Helper()
	u, err := url.Parse("https://s3.example.test/bucket/key?X-Amz-Signature=abc")
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeStore{
		root:  domain.Folder{ID: uuid.New(), IsRoot: true},
		quota: domain.Quota{LimitBytes: 1000, UsedBytes: 0},
	}
	fb := &fakeBlobs{presignPut: u, statSize: 50}
	return newSvc(fs, fb, nil), fs, fb
}

func TestPresignUploadOK(t *testing.T) {
	s, fs, fb := presignSvc(t)
	res, err := s.PresignUpload(tenantCtx(t), "/big.bin", 50, "application/octet-stream", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	if res.URL == "" || res.Handle == "" {
		t.Fatalf("faltan URL/handle: %+v", res)
	}
	// La clave reservada vive bajo files/<carpeta>/... (acotada e impredecible): el
	// cliente no puede subir fuera de ella (la firma S3 la ata; aquí, la reserva).
	wantPrefix := "files/" + fs.root.ID.String() + "/"
	if !strings.HasPrefix(fb.lastPutKey, wantPrefix) {
		t.Fatalf("clave presignada %q fuera de %q", fb.lastPutKey, wantPrefix)
	}
	// El handle ata EXACTAMENTE esa clave, carpeta, nombre y tamaño (firmado HMAC).
	h, err := s.verifyHandle(res.Handle)
	if err != nil {
		t.Fatalf("verifyHandle: %v", err)
	}
	if h.StorageKey != fb.lastPutKey {
		t.Fatalf("el handle ata %q pero se firmó %q", h.StorageKey, fb.lastPutKey)
	}
	if h.Size != 50 || h.Name != "big.bin" || h.Tenant == "" {
		t.Fatalf("handle mal formado: %+v", h)
	}
}

func TestPresignUploadDeniedUnknownFolder(t *testing.T) {
	// Ruta a una subcarpeta inexistente: la resolución (RLS) no la encuentra → 404.
	// La verja de permiso corre ANTES de firmar: no se emite URL alguna.
	s, _, fb := presignSvc(t)
	_, err := s.PresignUpload(tenantCtx(t), "/desconocida/big.bin", 50, "", "user-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if fb.lastPutKey != "" {
		t.Fatalf("no debió firmarse ninguna URL (clave=%q)", fb.lastPutKey)
	}
}

func TestPresignUploadQuotaExceeded(t *testing.T) {
	s, fs, fb := presignSvc(t)
	fs.quota = domain.Quota{LimitBytes: 40, UsedBytes: 0} // 50 > 40
	_, err := s.PresignUpload(tenantCtx(t), "/big.bin", 50, "", "user-1")
	if !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("esperaba ErrQuotaExceeded, got %v", err)
	}
	if fb.lastPutKey != "" {
		t.Fatalf("no debió firmarse URL con la cuota superada")
	}
}

func TestPresignUploadSizeOutOfRange(t *testing.T) {
	s, _, fb := presignSvc(t)
	for _, size := range []int64{0, -1, 101} { // el tope del servicio de prueba es 100 bytes
		if _, err := s.PresignUpload(tenantCtx(t), "/big.bin", size, "", "user-1"); !errors.Is(err, domain.ErrInvalidName) {
			t.Fatalf("size=%d: esperaba ErrInvalidName, got %v", size, err)
		}
	}
	if fb.lastPutKey != "" {
		t.Fatalf("no debió firmarse URL con tamaño fuera de rango")
	}
}

func TestFinalizeUploadOK(t *testing.T) {
	s, fs, fb := presignSvc(t)
	ctx := tenantCtx(t)
	fs.byKeyErr = domain.ErrNotFound // aún no registrado → crea
	created := domain.File{ID: uuid.New(), Name: "big.bin"}
	fs.file = created

	res, err := s.PresignUpload(ctx, "/big.bin", 50, "text/plain", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	fb.statSize = 50 // S3 tiene el objeto con el tamaño autorizado

	got, err := s.FinalizeUpload(ctx, res.Handle)
	if err != nil {
		t.Fatalf("FinalizeUpload: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("finalize devolvió %v, esperaba %v", got.ID, created.ID)
	}
	if fs.newFile == nil {
		t.Fatal("no se registró el metadato (CreateFile no fue llamado)")
	}
	// Se verifica en S3 EXACTAMENTE la clave reservada (tamaño LEÍDO de S3, no el
	// declarado por el cliente) y el metadato registra la clave COMPARTIDA por
	// contenido —el hash del objeto, que ahora se lee una vez—, tras copiar allí la
	// subida y borrar la propia. CreateFile reserva cuota.
	// Lo que se mide es la copia puesta a salvo, no la clave que el cliente escribe.
	firme := fb.lastPutKey + sufijoFirme
	if fb.lastStatKey != firme {
		t.Fatalf("medida %q, esperaba la copia a salvo %q", fb.lastStatKey, firme)
	}
	suma := sha256.Sum256([]byte(fb.getBody))
	compartida := "shared/drive/acme/" + hex.EncodeToString(suma[:])
	if fs.newFile.StorageKey != compartida || fs.newFile.Checksum != hex.EncodeToString(suma[:]) {
		t.Fatalf("clave registrada %q (checksum %q), esperaba %q", fs.newFile.StorageKey, fs.newFile.Checksum, compartida)
	}
	if len(fb.copias) != 2 || fb.copias[0] != fb.lastPutKey+"→"+firme || fb.copias[1] != firme+"→"+compartida {
		t.Fatalf("copias = %v", fb.copias)
	}
	if len(fb.delKeys) != 2 || fb.delKeys[0] != fb.lastPutKey || fb.delKeys[1] != firme {
		t.Fatalf("la subida y su copia a salvo debían borrarse tras compartirla: %v", fb.delKeys)
	}
	if fs.newFile.SizeBytes != 50 {
		t.Fatalf("tamaño registrado %d, esperaba 50 (verificado por HEAD)", fs.newFile.SizeBytes)
	}
}

func TestFinalizeUploadIdempotent(t *testing.T) {
	s, fs, fb := presignSvc(t)
	ctx := tenantCtx(t)
	existing := domain.File{ID: uuid.New(), Name: "big.bin"}
	res, err := s.PresignUpload(ctx, "/big.bin", 50, "", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	// El blob ya está registrado (reintento de finalize): se devuelve tal cual.
	fs.byKey, fs.byKeyErr = existing, nil
	fb.statSize = 50

	got, err := s.FinalizeUpload(ctx, res.Handle)
	if err != nil {
		t.Fatalf("FinalizeUpload: %v", err)
	}
	if got.ID != existing.ID {
		t.Fatalf("idempotencia: devolvió %v, esperaba el existente %v", got.ID, existing.ID)
	}
	if fs.newFile != nil {
		t.Fatal("idempotencia: no debió recrear el metadato")
	}
}

func TestFinalizeUploadNotArrived(t *testing.T) {
	s, fs, fb := presignSvc(t)
	ctx := tenantCtx(t)
	fs.byKeyErr = domain.ErrNotFound
	res, _ := s.PresignUpload(ctx, "/big.bin", 50, "", "user-1")
	fb.statErr = objectstore.ErrNotFound // el objeto no llegó a S3

	if _, err := s.FinalizeUpload(ctx, res.Handle); !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("esperaba ErrUploadNotFound, got %v", err)
	}
	if fs.newFile != nil {
		t.Fatal("no debió registrar metadato sin objeto en S3")
	}
}

func TestFinalizeUploadSizeMismatchCleansUp(t *testing.T) {
	s, fs, fb := presignSvc(t)
	ctx := tenantCtx(t)
	fs.byKeyErr = domain.ErrNotFound
	res, _ := s.PresignUpload(ctx, "/big.bin", 50, "", "user-1")
	fb.statSize = 999 // el cliente subió algo distinto de lo autorizado

	if _, err := s.FinalizeUpload(ctx, res.Handle); !errors.Is(err, ErrUploadSizeMismatch) {
		t.Fatalf("esperaba ErrUploadSizeMismatch, got %v", err)
	}
	if fs.newFile != nil {
		t.Fatal("no debió registrar metadato con tamaño no conforme")
	}
	// Limpieza: el objeto no conforme se borra de S3 (no queda huérfano registrable).
	if len(fb.delKeys) != 1 || fb.delKeys[0] != fb.lastStatKey {
		t.Fatalf("esperaba limpieza del blob %q, delKeys=%v", fb.lastStatKey, fb.delKeys)
	}
}

func TestFinalizeUploadTenantIsolation(t *testing.T) {
	s, fs, fb := presignSvc(t)
	fs.byKeyErr = domain.ErrNotFound
	fb.statSize = 50
	// Handle emitido en el tenant "acme".
	res, err := s.PresignUpload(tenantCtx(t), "/big.bin", 50, "", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	// Finalizar en OTRO tenant con el mismo handle → denegado (deny-by-default).
	other, err := tenancy.New("other")
	if err != nil {
		t.Fatal(err)
	}
	otherCtx := tenancy.Into(context.Background(), other)
	if _, err := s.FinalizeUpload(otherCtx, res.Handle); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("esperaba ErrPermission (cross-tenant), got %v", err)
	}
}

func TestFinalizeUploadForgeryRejected(t *testing.T) {
	s, fs, _ := presignSvc(t)
	fs.byKeyErr = domain.ErrNotFound
	ctx := tenantCtx(t)
	res, _ := s.PresignUpload(ctx, "/big.bin", 50, "", "user-1")

	// (a) Payload manipulado → la firma HMAC no casa.
	tampered := flipFirstRune(res.Handle)
	if _, err := s.FinalizeUpload(ctx, tampered); !errors.Is(err, ErrPresignHandleInvalid) {
		t.Fatalf("handle manipulado: esperaba ErrPresignHandleInvalid, got %v", err)
	}

	// (b) Handle firmado con OTRO secreto (no el del servicio) → rechazado.
	s2 := newSvc(&fakeStore{}, &fakeBlobs{}, nil)
	s2.cfg.S3SecretKey = "otro-secreto"
	forged, err := s2.signHandle(uploadHandle{
		Tenant: "acme", FolderID: uuid.NewString(), Name: "x",
		StorageKey: "files/x/y", Size: 50, Expires: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinalizeUpload(ctx, forged); !errors.Is(err, ErrPresignHandleInvalid) {
		t.Fatalf("handle forjado con otro secreto: esperaba ErrPresignHandleInvalid, got %v", err)
	}
}

func TestFinalizeUploadExpiredHandle(t *testing.T) {
	s, fs, _ := presignSvc(t)
	fs.byKeyErr = domain.ErrNotFound
	// Firma un handle ya caducado directamente (evita esperar la TTL real).
	expired, err := s.signHandle(uploadHandle{
		Tenant: "acme", FolderID: fs.root.ID.String(), Name: "x",
		StorageKey: "files/x/y", Size: 50, Expires: time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinalizeUpload(tenantCtx(t), expired); !errors.Is(err, ErrPresignHandleInvalid) {
		t.Fatalf("handle caducado: esperaba ErrPresignHandleInvalid, got %v", err)
	}
}

// TestFinalizeUploadReusoDeHandleTrasCompartir (FILE-08): una vez finalizada y
// deduplicada una subida directa (la fila apunta a shared/drive/... y la clave
// reservada se borró), el cliente no puede volver a subir otro objeto a la misma
// URL prefirmada (aún vigente durante PresignTTL) y reutilizar el mismo handle
// para crear o versionar otro fichero sin pasar por ReservarSubida.
func TestFinalizeUploadReusoDeHandleTrasCompartir(t *testing.T) {
	s, fs, fb := presignSvc(t)
	ctx := tenantCtx(t)
	fs.byKeyErr = domain.ErrNotFound
	fs.file = domain.File{ID: uuid.New(), Name: "big.bin"}
	fb.statSize = 50
	fb.getBody = strings.Repeat("a", 50)

	res, err := s.PresignUpload(ctx, "/big.bin", 50, "text/plain", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	if _, err := s.FinalizeUpload(ctx, res.Handle); err != nil {
		t.Fatalf("primer FinalizeUpload: %v", err)
	}

	// El cliente vuelve a hacer PUT a la misma URL prefirmada y reintenta FinalizeUpload.
	fs.newFile = nil
	fb.delKeys = nil
	fb.getBody = strings.Repeat("b", 50)
	if _, err := s.FinalizeUpload(ctx, res.Handle); !errors.Is(err, ErrPresignHandleInvalid) {
		t.Fatalf("reuso de handle tras compartir: esperaba ErrPresignHandleInvalid, got %v", err)
	}
	if fs.newFile != nil {
		t.Fatal("el segundo FinalizeUpload no debía crear ni versionar otro fichero")
	}
	if len(fb.delKeys) != 1 || fb.delKeys[0] != fb.lastPutKey {
		t.Fatalf("el objeto resubido a la clave ya consumida debía borrarse de S3: delKeys=%v", fb.delKeys)
	}
}

// TestFinalizeUploadCortaFlujoMayorQueLoAutorizado (FILE-08): si al leer el blob
// para calcular su hash el flujo entrega más bytes de los autorizados en el
// handle (p. ej. un objeto subido con metadato de compresión o un backend cuyo
// flujo supera Stat), FinalizeUpload corta con LimitReader, borra la copia y
// rechaza con ErrUploadSizeMismatch.
func TestFinalizeUploadCortaFlujoMayorQueLoAutorizado(t *testing.T) {
	s, fs, fb := presignSvc(t)
	ctx := tenantCtx(t)
	fs.byKeyErr = domain.ErrNotFound
	fb.statSize = 50
	fb.getBody = strings.Repeat("z", 51) // 51 > 50 autorizados

	res, err := s.PresignUpload(ctx, "/big.bin", 50, "text/plain", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	if _, err := s.FinalizeUpload(ctx, res.Handle); !errors.Is(err, ErrUploadSizeMismatch) {
		t.Fatalf("flujo mayor que h.Size: esperaba ErrUploadSizeMismatch, got %v", err)
	}
	if fs.newFile != nil {
		t.Fatal("no debió registrar el fichero cuando el flujo supera el tamaño autorizado")
	}
}

// flipFirstRune cambia el primer carácter para invalidar la firma del handle.
func flipFirstRune(s string) string {
	if s == "" {
		return s
	}
	c := byte('A')
	if s[0] == 'A' {
		c = 'B'
	}
	return string(c) + s[1:]
}
