// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

// arbol es un Drive con carpetas de verdad: el fake general devuelve lo mismo para
// cualquier carpeta, y así el recorrido no terminaría nunca.
type arbol struct {
	*fakeSvc
	subcarpetas map[uuid.UUID][]domain.Folder
	archivos    map[uuid.UUID][]domain.File
}

func (a *arbol) ListSubfolders(_ context.Context, id uuid.UUID) ([]domain.Folder, error) {
	return a.subcarpetas[id], nil
}

// ListFilesPage entrega los archivos de uno en uno, para que el cursor tenga que avanzar.
func (a *arbol) ListFilesPage(_ context.Context, id uuid.UUID, cursor string, _ int) (service.FilePage, error) {
	fs := a.archivos[id]
	i := 0
	if cursor != "" {
		i = int(cursor[0] - '0')
	}
	if i >= len(fs) {
		return service.FilePage{}, nil
	}
	pagina := service.FilePage{Files: fs[i : i+1]}
	if i+1 < len(fs) {
		pagina.NextCursor = string(rune('0' + i + 1))
	}
	return pagina, nil
}

func (a *arbol) OpenFileContent(_ context.Context, id uuid.UUID) (domain.File, io.ReadCloser, error) {
	for _, fs := range a.archivos {
		for _, f := range fs {
			if f.ID == id {
				return f, io.NopCloser(strings.NewReader("bytes de " + f.Name)), nil
			}
		}
	}
	return domain.File{}, nil, domain.ErrNotFound
}

func TestExportRecorreElArbolYSoloLoPropio(t *testing.T) {
	raiz, facturas := uuid.New(), uuid.New()
	a := &arbol{
		fakeSvc:     &fakeSvc{root: domain.Folder{ID: raiz, IsRoot: true}},
		subcarpetas: map[uuid.UUID][]domain.Folder{raiz: {{ID: facturas, Name: "Facturas"}}},
		archivos: map[uuid.UUID][]domain.File{
			raiz: {{ID: uuid.New(), Name: "cv.pdf", OwnerSub: "user-1"}},
			facturas: {
				{ID: uuid.New(), Name: "enero.pdf", OwnerSub: "user-1"},
				{ID: uuid.New(), Name: "de-otra.pdf", OwnerSub: "user-2"},
				{ID: uuid.New(), Name: "febrero.pdf", OwnerSub: "user-1"},
			},
		},
	}
	exportar := func(slug string) map[string]string {
		r := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/export?format=json", http.NoBody))
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: slug}))
		rec := httptest.NewRecorder()
		newHandlers(a).Export(rec, r)
		out := map[string]string{}
		for parte, err := range portabilidad.Leer(rec.Result()) {
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(parte)
			out[parte.Ruta] = string(b)
		}
		return out
	}

	org := exportar("acme")
	if org["files/Facturas/febrero.pdf"] != "bytes de febrero.pdf" || org["files/cv.pdf"] == "" {
		t.Fatalf("faltan archivos propios: %v", keysDe(org))
	}
	if _, ok := org["files/Facturas/de-otra.pdf"]; ok {
		t.Fatal("en una organización no sale lo que subió otra persona")
	}
	var filas []archivoExportado
	if err := json.Unmarshal([]byte(org["files.json"]), &filas); err != nil || len(filas) != 3 {
		t.Fatalf("tabla: %v %v", filas, err)
	}
	// En una cuenta personal todo el árbol es de la persona.
	if _, ok := exportar(tenancy.PersonalSlugPrefix + "ana")["files/Facturas/de-otra.pdf"]; !ok {
		t.Fatal("en una cuenta personal sale todo")
	}
}

func keysDe(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
