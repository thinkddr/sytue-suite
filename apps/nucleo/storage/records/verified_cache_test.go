// SPDX-License-Identifier: Apache-2.0

package records_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// soloCrypto es el puerto local sin sus interfaces opcionales: el motor lo trata como a
// cualquier Crypto que no pide recordar lo comprobado (el de la edición conectada).
type soloCrypto struct{ records.Crypto }

// motorDePrueba abre un repositorio del motor sobre el almacén local de dir, con filas
// JSON sueltas y un ValidateRow que cuenta sus llamadas; recordar elige si el Crypto es
// el puerto tal cual o sin sus interfaces opcionales.
func motorDePrueba(t *testing.T, dir string, recordar bool, validadas *atomic.Int64) *records.Repository {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "notes", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rec.Cerrar)
	cfg := rec.Config
	if !recordar {
		cfg.Crypto = soloCrypto{rec.Puerto}
	}
	cfg.Product, cfg.EntityKind, cfg.ReceiptKind = "notes", "org.sytue.prueba.entity.v1", "org.sytue.prueba.snapshot.v1"
	cfg.ValidateRow = func(_ context.Context, kind, _ string, raw []byte) error {
		validadas.Add(1)
		if kind != "fila" || !json.Valid(raw) {
			return records.ErrSource
		}
		return nil
	}
	cfg.ValidateCatalog = func(context.Context, records.Catalog) error { return nil }
	r, err := records.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	if err := almacen.Preparar(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.VerifiedContentGeneration(); ok != recordar {
		t.Fatal("VerifiedContentGeneration", ok)
	}
	return r
}

// guardarFila escribe la fila id con texto.
func guardarFila(t *testing.T, r *records.Repository, id, texto string) {
	t.Helper()
	raw := fmt.Appendf(nil, `{"id":%q,"texto":%q}`, id, texto)
	_, err := r.Mutate(t.Context(), "guardar", id+texto, func(context.Context) (string, string, string, []byte, error) {
		return "fila", id, "live", bytes.Clone(raw), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// leerFilas lee el JSON de cada fila, por su id.
func leerFilas(t *testing.T, r *records.Repository) map[string]string {
	t.Helper()
	versiones, err := r.Selected("fila")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, v := range versiones {
		raw, err := r.ReadJSON(t.Context(), v)
		if err != nil {
			t.Fatal(err)
		}
		var fila struct{ ID string }
		if err := json.Unmarshal(raw, &fila); err != nil {
			t.Fatal(err)
		}
		out[fila.ID] = string(raw)
		clear(raw) // como los dominios: el motor no puede depender de este búfer
	}
	return out
}

// Con un Crypto que lo pide (el almacén local), el motor no vuelve a validar una
// revisión que ya comprobó y lee lo mismo que sin pedirlo; con uno que no lo pide, todo
// es como siempre (cada lectura valida). Una revisión nueva se comprueba entera.
func TestLoComprobadoSeLeeIgualYNoSeValidaDosVeces(t *testing.T) {
	dir := t.TempDir()
	var validadas atomic.Int64
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}

	sin := motorDePrueba(t, dir, false, &validadas)
	for i, id := range ids {
		guardarFila(t, sin, id, fmt.Sprintf("uno %d", i))
	}
	referencia := leerFilas(t, sin)
	antes := validadas.Load()
	if again := leerFilas(t, sin); fmt.Sprint(again) != fmt.Sprint(referencia) || validadas.Load()-antes != int64(len(ids)) {
		t.Fatal("sin recordar, cada lectura valida cada fila", validadas.Load()-antes)
	}
	sin.Close()

	var conValidadas atomic.Int64
	con := motorDePrueba(t, dir, true, &conValidadas)
	primera := leerFilas(t, con)
	if fmt.Sprint(primera) != fmt.Sprint(referencia) {
		t.Fatalf("recordando se lee otra cosa:\n%v\n%v", primera, referencia)
	}
	antes = conValidadas.Load()
	for range 3 {
		if again := leerFilas(t, con); fmt.Sprint(again) != fmt.Sprint(referencia) {
			t.Fatal("la segunda lectura es otra", again)
		}
	}
	if conValidadas.Load() != antes {
		t.Fatal("se volvió a validar lo ya comprobado", conValidadas.Load()-antes)
	}
	// Una fila cambiada es una revisión nueva: se comprueba entera al escribirla (el
	// motor relee lo publicado) y después se lee la nueva sin volver a validarla.
	antes = conValidadas.Load()
	guardarFila(t, con, ids[1], "dos")
	if conValidadas.Load() == antes {
		t.Fatal("la revisión nueva no se validó al escribirla")
	}
	antes = conValidadas.Load()
	despues := leerFilas(t, con)
	leerFilas(t, con)
	if despues[ids[1]] != fmt.Sprintf(`{"id":%q,"texto":"dos"}`, ids[1]) || despues[ids[0]] != referencia[ids[0]] || conValidadas.Load() != antes {
		t.Fatal("la fila cambiada", despues[ids[1]], conValidadas.Load()-antes)
	}
}
