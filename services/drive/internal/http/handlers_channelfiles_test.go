// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// TestChannelFileHandlersPropagateGate: las tres lecturas de un adjunto de canal
// (metadatos, contenido, miniatura) devuelven 404 cuando el servicio deniega —no
// miembro del canal o fichero de otro canal—, y 200 cuando concede. Un 403 aquí
// confirmaría que el fichero existe: la convención del repo es 404.
func TestChannelFileHandlersPropagateGate(t *testing.T) {
	cid, fid := uuid.New(), uuid.New()
	file := domain.File{ID: fid, Name: "plan.png", ContentType: "image/png", SizeBytes: 3}

	for _, tc := range []struct {
		name string
		call func(h *Handlers, w http.ResponseWriter, r *http.Request)
	}{
		{"metadatos", (*Handlers).GetChannelFile},
		{"contenido", (*Handlers).DownloadChannelFile},
		{"miniatura", (*Handlers).ThumbnailChannelFile},
	} {
		t.Run(tc.name+" denegado", func(t *testing.T) {
			h := newHandlers(&fakeSvc{chanErr: domain.ErrNotFound})
			rec := httptest.NewRecorder()
			req := withParams(withAuth(httptest.NewRequest(http.MethodGet, "/x", http.NoBody)),
				map[string]string{"channelID": cid.String(), "fileID": fid.String()})
			tc.call(h, rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("no-miembro => %d, quiero 404", rec.Code)
			}
		})
		t.Run(tc.name+" concedido", func(t *testing.T) {
			fs := &fakeSvc{chanFile: file, chanBody: "png"}
			rec := httptest.NewRecorder()
			req := withParams(withAuth(httptest.NewRequest(http.MethodGet, "/x", http.NoBody)),
				map[string]string{"channelID": cid.String(), "fileID": fid.String()})
			tc.call(newHandlers(fs), rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("miembro => %d, quiero 200", rec.Code)
			}
			if fs.lastChannelID != cid {
				t.Fatalf("el canal de la ruta no llega al servicio: %v", fs.lastChannelID)
			}
		})
	}
}

// TestUploadChannelFileNeedsNameAndChannel: la subida exige nombre y un canal válido,
// y pasa el canal (no una carpeta elegida por el cliente) al servicio.
func TestUploadChannelFileNeedsNameAndChannel(t *testing.T) {
	cid := uuid.New()
	fs := &fakeSvc{chanFile: domain.File{ID: uuid.New(), Name: "a.txt"}}
	h := newHandlers(fs)

	rec := httptest.NewRecorder()
	req := withParams(withAuth(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hola"))),
		map[string]string{"channelID": cid.String()})
	h.UploadChannelFile(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin nombre => %d, quiero 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = withParams(withAuth(httptest.NewRequest(http.MethodPost, "/x?name=a.txt", strings.NewReader("hola"))),
		map[string]string{"channelID": "no-es-uuid"})
	h.UploadChannelFile(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("canal inválido => %d, quiero 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = withParams(withAuth(httptest.NewRequest(http.MethodPost, "/x?name=a.txt", strings.NewReader("hola"))),
		map[string]string{"channelID": cid.String()})
	h.UploadChannelFile(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("subida válida => %d, quiero 201", rec.Code)
	}
	if fs.lastChannelID != cid || fs.lastUploadName != "a.txt" {
		t.Fatalf("el servicio recibió canal=%v nombre=%q", fs.lastChannelID, fs.lastUploadName)
	}
}

// TestRouterMontaLasRutasDeCanal: las cuatro rutas están registradas en el montaje de
// PRODUCCIÓN (no solo los handlers), y sin colisionar con /files/{id} (chi entra en
// pánico ante patrones incompatibles: mejor descubrirlo aquí que en el arranque).
func TestRouterMontaLasRutasDeCanal(t *testing.T) {
	r, ok := NewRouter(Deps{Cfg: config.Config{PublicURL: "https://drive.sytue.eu"}}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter no devuelve un router de chi")
	}
	registradas := map[string]bool{}
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registradas[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	for _, want := range []string{
		"POST /api/v1/channels/{channelID}/files",
		"GET /api/v1/channels/{channelID}/files/{fileID}",
		"GET /api/v1/channels/{channelID}/files/{fileID}/content",
		"GET /api/v1/channels/{channelID}/files/{fileID}/thumbnail",
		"GET /api/v1/files/{id}", // la ruta personal sigue intacta
	} {
		if !registradas[want] {
			t.Fatalf("falta la ruta %q", want)
		}
	}
}

// TestCopyChannelFileToMyDrive: «Guardar en mi Drive» pasa por la MISMA verja de canal
// que el resto de rutas de adjunto —un no-miembro recibe 404, sin distinguir si el
// fichero existe— y, cuando concede, crea un recurso: 201 y no 200.
func TestCopyChannelFileToMyDrive(t *testing.T) {
	cid, fid := uuid.New(), uuid.New()

	t.Run("no-miembro: 404", func(t *testing.T) {
		h := newHandlers(&fakeSvc{chanErr: domain.ErrNotFound})
		rec := httptest.NewRecorder()
		req := withParams(withAuth(httptest.NewRequest(http.MethodPost, "/x", http.NoBody)),
			map[string]string{"channelID": cid.String(), "fileID": fid.String()})
		h.CopyChannelFileToMyDrive(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("no-miembro => %d, quiero 404", rec.Code)
		}
	})

	t.Run("miembro: 201 con el fichero nuevo", func(t *testing.T) {
		fs := &fakeSvc{chanFile: domain.File{ID: fid, Name: "plan.png", SizeBytes: 3}}
		rec := httptest.NewRecorder()
		req := withParams(withAuth(httptest.NewRequest(http.MethodPost, "/x", http.NoBody)),
			map[string]string{"channelID": cid.String(), "fileID": fid.String()})
		newHandlers(fs).CopyChannelFileToMyDrive(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("miembro => %d, quiero 201", rec.Code)
		}
		if fs.lastChannelID != cid {
			t.Fatalf("el canal de la ruta no llega al servicio: %v", fs.lastChannelID)
		}
		// La respuesta debe traer el id de la COPIA: es lo que la interfaz usa para
		// enlazar «ver en Drive», y devolver el del origen mandaría al fichero del canal.
		var out domain.File
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("respuesta ilegible: %v", err)
		}
		if out.ID == uuid.Nil || out.ID == fid {
			t.Fatalf("id de la copia = %v; debe ser uno NUEVO", out.ID)
		}
	})
}
