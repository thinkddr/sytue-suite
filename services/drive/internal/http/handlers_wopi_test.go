// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/wopi"
)

// fakeLocks es un almacén de bloqueos en memoria con la MISMA semántica que el real:
// conceder si no hay, si caducó o si es el mismo lock_id; conflicto en cualquier otro caso.
type fakeLocks struct {
	lock  store.WopiLock
	tiene bool
}

func (f *fakeLocks) AcquireWopiLock(_ context.Context, id uuid.UUID, lockID, holder string, ttl time.Duration) (store.WopiLock, error) {
	if f.tiene && f.lock.LockID != lockID && f.lock.ExpiresAt.After(time.Now()) {
		return store.WopiLock{}, store.ErrLockConflict
	}
	f.lock = store.WopiLock{FileID: id, LockID: lockID, HolderSub: holder, ExpiresAt: time.Now().Add(ttl)}
	f.tiene = true
	return f.lock, nil
}

func (f *fakeLocks) GetWopiLock(_ context.Context, _ uuid.UUID) (store.WopiLock, error) {
	if !f.tiene || !f.lock.ExpiresAt.After(time.Now()) {
		return store.WopiLock{}, domain.ErrNotFound
	}
	return f.lock, nil
}

func (f *fakeLocks) ReleaseWopiLock(_ context.Context, _ uuid.UUID, lockID string) error {
	if f.tiene && f.lock.LockID != lockID && f.lock.ExpiresAt.After(time.Now()) {
		return store.ErrLockConflict
	}
	f.tiene = false
	return nil
}

// wopiHarness monta el host con un fichero y devuelve el router, el id y un token válido.
func wopiHarness(t *testing.T, write bool) (http.Handler, uuid.UUID, string, *fakeLocks) {
	t.Helper()
	c, err := keycrypto.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("keycrypto: %v", err)
	}
	m := wopi.NewMinter(c, time.Hour)
	id := uuid.New()
	svc := &fakeSvc{file: domain.File{ID: id, Name: "informe.docx", SizeBytes: 12, Version: 3}}
	locks := &fakeLocks{}

	r := chi.NewRouter()
	mountWopi(r, &wopiHandlers{svc: svc, locks: locks, minter: m, maxUpload: 1 << 20})

	tok, err := m.Mint(wopi.Claims{Tenant: "acme", Subject: "u1", FileID: id, Write: write})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	_ = config.Config{}
	return r, id, tok, locks
}

func wopiReq(method, path, token string) *http.Request {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return httptest.NewRequest(method, path+sep+"access_token="+token, http.NoBody)
}

// TestWopiSinTokenNoPasa es la verja: el host WOPI no lleva el middleware de auth del
// resto del servicio, así que si el token no se comprobara aquí, /wopi/files/{id} sería
// una puerta ABIERTA a cualquier fichero del almacén.
func TestWopiSinTokenNoPasa(t *testing.T) {
	r, id, tok, _ := wopiHarness(t, true)
	for nombre, req := range map[string]*http.Request{
		"sin token":    httptest.NewRequest(http.MethodGet, "/wopi/files/"+id.String(), http.NoBody),
		"basura":       wopiReq(http.MethodGet, "/wopi/files/"+id.String(), "no-es-un-token"),
		"otro fichero": wopiReq(http.MethodGet, "/wopi/files/"+uuid.New().String(), tok),
	} {
		t.Run(nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusNotFound {
				t.Fatalf("código = %d; se esperaba rechazo", rec.Code)
			}
		})
	}
}

// TestWopiCheckFileInfo: la primera llamada del editor. Lo que declare aquí determina qué
// botones enseña, así que UserCanWrite debe salir del permiso REAL del token.
func TestWopiCheckFileInfo(t *testing.T) {
	for _, write := range []bool{true, false} {
		r, id, tok, _ := wopiHarness(t, write)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, wopiReq(http.MethodGet, "/wopi/files/"+id.String(), tok))
		if rec.Code != http.StatusOK {
			t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"BaseFileName":"informe.docx"`) {
			t.Fatalf("sin nombre de fichero: %s", body)
		}
		quiere := `"UserCanWrite":true`
		if !write {
			quiere = `"UserCanWrite":false`
		}
		if !strings.Contains(body, quiere) {
			t.Fatalf("write=%v pero el cuerpo dice otra cosa: %s", write, body)
		}
	}
}

// TestWopiLockYConflicto es el corazón del protocolo.
//
// WOPI no fusiona: un PutFile reemplaza el contenido ENTERO. Si el bloqueo no se
// respetara, el segundo editor en guardar borraría el trabajo del primero sin que nadie se
// entere. El 409 con X-WOPI-Lock es lo que convierte esa pérdida silenciosa en un aviso.
func TestWopiLockYConflicto(t *testing.T) {
	r, id, tok, _ := wopiHarness(t, true)
	ruta := "/wopi/files/" + id.String()

	// El primer editor toma el bloqueo.
	req := wopiReq(http.MethodPost, ruta, tok)
	req.Header.Set("X-WOPI-Override", "LOCK")
	req.Header.Set("X-WOPI-Lock", "editor-A")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("LOCK inicial = %d", rec.Code)
	}

	// El segundo lo intenta con OTRO identificador: conflicto, y debe recibir el actual.
	req = wopiReq(http.MethodPost, ruta, tok)
	req.Header.Set("X-WOPI-Override", "LOCK")
	req.Header.Set("X-WOPI-Lock", "editor-B")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("LOCK de otro editor = %d, quiero 409", rec.Code)
	}
	if got := rec.Header().Get("X-WOPI-Lock"); got != "editor-A" {
		t.Fatalf("el 409 debe devolver el lock actual; devolvió %q", got)
	}

	// El primero refresca el suyo: se le concede.
	req = wopiReq(http.MethodPost, ruta, tok)
	req.Header.Set("X-WOPI-Override", "REFRESH_LOCK")
	req.Header.Set("X-WOPI-Lock", "editor-A")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("REFRESH del propio = %d", rec.Code)
	}

	// Y lo suelta.
	req = wopiReq(http.MethodPost, ruta, tok)
	req.Header.Set("X-WOPI-Override", "UNLOCK")
	req.Header.Set("X-WOPI-Lock", "editor-A")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("UNLOCK = %d", rec.Code)
	}
}

// TestWopiPutFileRespetaElLock: guardar con el lock de OTRO tiene que fallar, o el
// bloqueo no serviría de nada.
func TestWopiPutFileRespetaElLock(t *testing.T) {
	r, id, tok, locks := wopiHarness(t, true)
	locks.lock = store.WopiLock{FileID: id, LockID: "editor-A", ExpiresAt: time.Now().Add(time.Hour)}
	locks.tiene = true

	req := wopiReq(http.MethodPost, "/wopi/files/"+id.String()+"/contents", tok)
	req.Header.Set("X-WOPI-Lock", "editor-B")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("guardar con lock ajeno = %d, quiero 409", rec.Code)
	}
	if got := rec.Header().Get("X-WOPI-Lock"); got != "editor-A" {
		t.Fatalf("el 409 debe llevar el lock actual; llevó %q", got)
	}
}

// TestWopiSoloLecturaNoGuarda: un token de vista no puede escribir, por mucho que el
// editor lo intente.
func TestWopiSoloLecturaNoGuarda(t *testing.T) {
	r, id, tok, _ := wopiHarness(t, false)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, wopiReq(http.MethodPost, "/wopi/files/"+id.String()+"/contents", tok))
	if rec.Code == http.StatusOK {
		t.Fatal("un token de solo lectura guardó")
	}
}

// TestWopiOverrideDesconocidoEs501: un 200 vacío haría creer al editor que la operación
// surtió efecto.
func TestWopiOverrideDesconocidoEs501(t *testing.T) {
	r, id, tok, _ := wopiHarness(t, true)
	req := wopiReq(http.MethodPost, "/wopi/files/"+id.String(), tok)
	req.Header.Set("X-WOPI-Override", "PUT_RELATIVE")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("override no soportado = %d, quiero 501", rec.Code)
	}
}
