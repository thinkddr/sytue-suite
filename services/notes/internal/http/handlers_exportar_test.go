// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// La vista previa de un `.onex` se prueba SIN base de datos: no la toca. Es la
// única ruta del paquete que se puede ejercitar así de barato, y es justo la que
// está en la frontera de confianza —el fichero viene de fuera—, así que es la
// que más falta hacía.

func previsualizar(t *testing.T, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/preview", strings.NewReader(cuerpo))
	rec := httptest.NewRecorder()
	(&Handlers{}).PreviewNotebook(rec, req)
	return rec
}

func TestPreviewEsUnEnsayoSaneadoDeLaImportacion(t *testing.T) {
	// Una nota buena con un `<script>` dentro y SIN ancho —como la escribiría un
	// script que no sabe de anchos; tiene que entrar con el de siempre—, una de un
	// tipo que no existe y una con un ancho imposible.
	volcado := `{"formato":"sytue.notepads","version":1,
		"notebook":{"title":"Mudanza"},
		"notas":[
		  {"kind":"text","title":"Cocina","content":"<p>Vajilla</p><script>alert(1)</script>"},
		  {"kind":"no-existe","title":"Rota"},
		  {"kind":"text","title":"Estrecha","content":"<p>x</p>","width":5}
		]}`
	rec := previsualizar(t, volcado)
	if rec.Code != http.StatusOK {
		t.Fatalf("código %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Notebook    domain.Notebook `json:"notebook"`
		Notas       []domain.Note   `json:"notas"`
		Descartadas int             `json:"descartadas"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	// LAS NOTAS ROTAS SE CUENTAN, NO SE CALLAN: quien mira decide antes de importar.
	if out.Descartadas != 2 || len(out.Notas) != 1 {
		t.Fatalf("descartadas=%d, notas=%d; quiero 2 y 1", out.Descartadas, len(out.Notas))
	}
	// Y LO QUE SE ENSEÑA ESTÁ SANEADO: el fichero puede venir de cualquiera, y el
	// navegador de quien lo mira va a pintar esto como HTML.
	c := out.Notas[0].Content
	if strings.Contains(c, "<script") || strings.Contains(c, "alert(") {
		t.Errorf("el script del fichero llegó a la vista previa: %q", c)
	}
	if !strings.Contains(c, "Vajilla") {
		t.Errorf("el saneado se llevó el contenido bueno: %q", c)
	}
	if out.Notebook.Title != "Mudanza" {
		t.Errorf("título %q", out.Notebook.Title)
	}
	// Lo que el servidor decide no viaja de vuelta aunque el fichero lo traiga.
	if out.Notebook.PublicToken != "" {
		t.Error("el token público del fichero volvió en la vista previa")
	}
}

func TestPreviewRechazaLoQueNoEsUnVolcado(t *testing.T) {
	casos := map[string]string{
		// El caso REAL: `.onex` es también la extensión del índice de un cuaderno
		// de OneNote, que es binario.
		"un .onex de OneNote": "\x00\x01\xa1\x2f\xffbinario",
		"otro formato":        `{"formato":"otra.cosa","version":1,"notebook":{"title":"x"}}`,
		"versión futura":      `{"formato":"sytue.notepads","version":99,"notebook":{"title":"x"}}`,
		"cuaderno sin título": `{"formato":"sytue.notepads","version":1,"notebook":{"title":"  "}}`,
	}
	for nombre, cuerpo := range casos {
		if rec := previsualizar(t, cuerpo); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: código %d, quiero 400", nombre, rec.Code)
		}
	}
}

// El tope de tamaño es lo PRIMERO: sin él, un fichero de un gigabyte es la forma
// más barata de tumbar el servicio. 32 MiB + 1 tiene que rechazarse sin leerlo.
func TestPreviewCortaLosFicherosEnormes(t *testing.T) {
	enorme := strings.NewReader(strings.Repeat("x", 32<<20+1))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/preview", enorme)
	rec := httptest.NewRecorder()
	(&Handlers{}).PreviewNotebook(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código %d, quiero 400", rec.Code)
	}
}
