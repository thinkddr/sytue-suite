// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// # La actividad de una base, en el dispositivo (B-06)
//
// En la edición conectada la traza la escribe el servicio. En Sytue Office no hay
// servicio: si la app lo pide (`Repository.Actividad`), cada cambio que pasa por el
// repositorio deja una línea en un fichero JSONL en claro, con las mismas acciones
// (`table.create`, `record.update`…) y la misma forma (`api.BasesAuditEntry`) que la
// traza del servicio, para que la pantalla la cuente con las mismas frases. Sin
// Actividad —lo de siempre, y lo de las bases privadas de la conectada— no se escribe
// nada y `ListAudit` falla como antes.

// MaxActividad es lo más que se guarda: al pasarlo se olvida lo más antiguo.
const MaxActividad = 20000

// Actividad es el registro de actividad local, un fichero `actividad.jsonl`.
type Actividad struct {
	ruta string

	mu       sync.Mutex
	cargada  bool
	entradas []entradaActividad
	ultimo   int64
}

type entradaActividad struct {
	Base string `json:"base"`
	api.BasesAuditEntry
}

// FicheroActividad es el nombre del registro de actividad junto al almacén de Bases.
const FicheroActividad = "actividad.jsonl"

// NuevaActividad lleva el registro en ruta (se crea al primer cambio).
func NuevaActividad(ruta string) *Actividad { return &Actividad{ruta: ruta} }

// cargar lee el fichero una vez. Con a.mu. Una línea ilegible (la última a medias de un
// cierre brusco) se salta.
func (a *Actividad) cargar() error {
	if a.cargada {
		return nil
	}
	f, err := os.Open(a.ruta)
	if errors.Is(err, fs.ErrNotExist) {
		a.cargada = true
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var e entradaActividad
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.ID <= 0 {
			continue
		}
		a.entradas = append(a.entradas, e)
		a.ultimo = max(a.ultimo, e.ID)
	}
	a.cargada = true
	return sc.Err()
}

// anotar añade una entrada. Al pasar de MaxActividad se reescribe el fichero con las
// últimas.
func (a *Actividad) anotar(base string, e api.BasesAuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.cargar(); err != nil {
		return err
	}
	a.ultimo++
	e.ID = a.ultimo
	nueva := entradaActividad{Base: base, BasesAuditEntry: e}
	a.entradas = append(a.entradas, nueva)
	if len(a.entradas) > MaxActividad {
		a.entradas = slices.Clone(a.entradas[len(a.entradas)-MaxActividad:])
		return a.reescribir()
	}
	linea, err := json.Marshal(nueva)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.ruta), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(a.ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(linea, '\n'))
	return errors.Join(err, f.Close())
}

// reescribir deja en el fichero solo lo que hay en memoria, con temporal y renombrado.
func (a *Actividad) reescribir() error {
	tmp := a.ruta + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, e := range a.entradas {
		linea, err := json.Marshal(e)
		if err != nil {
			f.Close()
			return err
		}
		_, _ = w.Write(append(linea, '\n'))
	}
	if err := errors.Join(w.Flush(), f.Sync(), f.Close()); err != nil {
		return err
	}
	return os.Rename(tmp, a.ruta)
}

// leer son las entradas de base, de la más reciente a la más antigua; antes > 0, solo
// las anteriores a esa.
func (a *Actividad) leer(base string, antes int64, limite int) ([]api.BasesAuditEntry, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.cargar(); err != nil {
		return nil, err
	}
	out := []api.BasesAuditEntry{}
	for i := len(a.entradas) - 1; i >= 0 && len(out) < limite; i-- {
		e := a.entradas[i]
		if e.Base == base && (antes <= 0 || e.ID < antes) {
			out = append(out, e.BasesAuditEntry)
		}
	}
	return out, nil
}

// ListAudit es la actividad de la base. Sin registro de actividad, lo de siempre: las
// bases privadas no tienen traza.
func (r *Repository) ListAudit(_ context.Context, base string, antes int64, limite int) ([]api.BasesAuditEntry, error) {
	if r.Actividad == nil {
		return nil, api.ErrPrivateBasesOperation
	}
	return r.Actividad.leer(base, antes, limite)
}

var _ api.BasesAuditData = (*Repository)(nil)

// accionDe es la acción de la traza del servicio para cada operación del repositorio.
// Las que no están (las posiciones de cada campo al reordenar, las filas de un CSV, los
// enlaces que se sueltan al borrar…) no se anotan sueltas: las resume quien las hace.
var accionDe = map[string]string{
	"base-create":       "base.create",
	"base-update":       "base.update",
	"table-create":      "table.create",
	"table-rename":      "table.rename",
	"table-delete":      "table.delete",
	"field-create":      "field.create",
	"field-rename":      "field.rename",
	"field-delete":      "field.delete",
	"field-formula":     "field.formula",
	"field-choices":     "field.options",
	"record-create":     "record.create",
	"record-cell":       "record.update",
	"record-delete":     "record.delete",
	"view-create":       "view.create",
	"view-update":       "view.update",
	"view-delete":       "view.delete",
	"comment-create":    "comment.create",
	"comment-delete":    "comment.delete",
	"comment-tombstone": "comment.delete",
	"attachment-create": "attachment.add",
	"attachment-delete": "attachment.delete",
}

// sinTraza en el contexto calla las operaciones sueltas de un cambio que se anota
// entero (importar un CSV, reordenar los campos).
type sinTraza struct{}

func callar(ctx context.Context) context.Context { return context.WithValue(ctx, sinTraza{}, true) }

// prepararFila es la preparación de una mutación del motor: la clase, el id, el estado y
// el JSON de la fila que se escribe.
type prepararFila = func(context.Context) (string, string, string, []byte, error)

// mutar es `core.Mutate` y, con registro de actividad, su entrada.
func (r *Repository) mutar(ctx context.Context, op string, input any, fn prepararFila) ([]byte, error) {
	return r.mutarCon(ctx, op, input, fn, nil)
}

// mutarCon es mutar con el contenido de la fila (un adjunto), si no es nil, por
// `core.MutateWithContent`.
func (r *Repository) mutarCon(ctx context.Context, op string, input any, fn prepararFila, contenido *records.MutationContent) ([]byte, error) {
	motor := func(fn prepararFila) ([]byte, error) {
		if contenido != nil {
			return r.core.MutateWithContent(ctx, op, input, fn, *contenido)
		}
		return r.core.Mutate(ctx, op, input, fn)
	}
	if r.Actividad == nil || ctx.Value(sinTraza{}) != nil || accionDe[op] == "" {
		return motor(fn)
	}
	var kind, id string
	raw, err := motor(func(c context.Context) (string, string, string, []byte, error) {
		k, i, s, v, e := fn(c)
		kind, id = k, i
		return k, i, s, v, e
	})
	if err != nil {
		return raw, err
	}
	if kind == "" {
		// Una operación cortada que se repite: el motor la retoma sin preparar, y la fila
		// escrita es la que devuelve. Se anota ahora, que es cuando se da por hecha.
		var fila struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &fila)
		kind, id = claseDe(accionDe[op]), fila.ID
	}
	r.anotar(ctx, accionDe[op], kind, id, raw, input)
	return raw, nil
}

// claseDe es la clase de fila que cambia una acción de la traza («record.update» →
// «record»; los adjuntos son filas «file»).
func claseDe(accion string) string {
	clase, _, _ := strings.Cut(accion, ".")
	if clase == "attachment" {
		return "file"
	}
	return clase
}

// anotar deja la entrada de un cambio hecho. No falla: perder una línea de actividad no
// puede deshacer lo que la persona acaba de hacer.
func (r *Repository) anotar(ctx context.Context, accion, kind, id string, val []byte, input any) {
	var fila struct {
		BaseID   string `json:"base_id"`
		TableID  string `json:"table_id"`
		RecordID string `json:"record_id"`
		FieldID  string `json:"field_id"`
		Name     string `json:"name"`
	}
	_ = json.Unmarshal(val, &fila)
	e := api.BasesAuditEntry{Actor: r.scope.Subject, Action: accion, At: time.Now().UTC()}
	base := ""
	switch kind {
	case "base":
		base = id
	case "table":
		base, e.TableID = fila.BaseID, id
	case "field":
		e.TableID, e.FieldID = fila.TableID, id
	case "view":
		e.TableID = fila.TableID
	case "record":
		e.TableID, e.RecordID = fila.TableID, id
		if m, ok := input.(map[string]string); ok {
			e.FieldID = m["field"]
		}
	case "comment", "file":
		e.RecordID, e.FieldID = fila.RecordID, fila.FieldID
		if rec, err := leerUno[api.BasesRecord](ctx, r, "record", fila.RecordID); err == nil {
			e.TableID = rec.TableID
		}
	}
	if fila.Name != "" && kind != "record" && kind != "comment" {
		e.After, _ = json.Marshal(map[string]string{"name": fila.Name})
	}
	r.apuntar(ctx, base, e)
}

// apuntar guarda la entrada en la base de su tabla.
func (r *Repository) apuntar(ctx context.Context, base string, e api.BasesAuditEntry) {
	if base == "" && e.TableID != "" {
		if raw, err := r.core.ReadRaw(ctx, "table", e.TableID); err == nil {
			var t api.BasesTable
			if json.Unmarshal(raw, &t) == nil {
				base = t.BaseID
			}
			clear(raw)
		}
	}
	if base == "" {
		return
	}
	_ = r.Actividad.anotar(base, e)
}

// resumir anota un cambio hecho con la traza callada.
func (r *Repository) resumir(ctx context.Context, accion, tabla, nombre string) {
	if r.Actividad == nil || ctx.Value(sinTraza{}) != nil {
		return
	}
	e := api.BasesAuditEntry{Actor: r.scope.Subject, Action: accion, At: time.Now().UTC(), TableID: tabla}
	if nombre != "" {
		e.After, _ = json.Marshal(map[string]string{"name": nombre})
	}
	r.apuntar(ctx, "", e)
}
