// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

var (
	_ api.NotesDailyData = (*Repository)(nil)
	_ api.NotesTasksData = (*Repository)(nil)
)

// El diario sin servicio: las mismas reglas que `services/notes` (store/diario.go y
// store/tareas.go), sobre el catálogo privado. Un diario es un cuaderno con
// `daily_date`; «hoy» lo decide el reloj de quien lo pide, nunca este repositorio.

// Topes de un tramo del diario, los del servicio (domain.MaxDiasCuentas y MaxDiasNotas).
const (
	maxDiasCuentas = 62
	maxDiasNotas   = 31
	topeTareas     = 200
	colorDiario    = "#86198f"
)

// fechaDiaria dice si `s` es un día civil `AAAA-MM-DD` válido.
func fechaDiaria(s string) bool {
	d, err := time.Parse(time.DateOnly, s)
	return err == nil && d.Format(time.DateOnly) == s && d.Year() >= 1
}

func rangoDiario(from, to string, maximo int) bool {
	if !fechaDiaria(from) || !fechaDiaria(to) {
		return false
	}
	a, _ := time.Parse(time.DateOnly, from)
	b, _ := time.Parse(time.DateOnly, to)
	dias := int(b.Sub(a).Hours()/24) + 1
	return dias >= 1 && dias <= maximo
}

// tituloDiario valida la fecha y normaliza el título como el de cualquier cuaderno.
func tituloDiario(date, title string) (string, error) {
	in := model.NotebookInput{Title: title}
	if !fechaDiaria(date) || in.Normalize() != nil {
		return "", model.ErrInvalidInput
	}
	return in.Title, nil
}

// diarioDe es el diario vivo de ese día, si lo hay.
func (r *Repository) diarioDe(ctx context.Context, date string) (api.Notebook, bool, error) {
	books, err := r.notebooks(ctx)
	if err != nil {
		return api.Notebook{}, false, err
	}
	for _, nb := range books {
		if nb.DailyDate == date && nb.OwnerSub == r.cfg.Scope.Subject && visibleNotebook(books, nb.ID) {
			return nb, true, nil
		}
	}
	return api.Notebook{}, false, nil
}

// abrirDiario devuelve el diario del día, creándolo si no existe. Abrirlo otra vez no
// lo duplica ni lo reescribe.
func (r *Repository) abrirDiario(ctx context.Context, date, title string) (api.Notebook, error) {
	if nb, ok, err := r.diarioDe(ctx, date); err != nil || ok {
		return nb, err
	}
	raw, err := r.mutate(ctx, "open-daily", struct{ Date, Title string }{date, title}, func(ctx context.Context) (string, string, string, []byte, error) {
		books, err := r.notebooks(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		pos := 0
		for _, b := range books {
			pos = max(pos, b.Position)
		}
		now := time.Now().UTC()
		nb := api.Notebook{
			ID: uuid.NewString(), OwnerSub: r.cfg.Scope.Subject, Title: title, Color: colorDiario,
			Layout: "list", Visibility: "personal", Indexable: true, DailyDate: date, Position: pos + 1, UpdatedAt: now,
		}
		encoded, err := encodeNewNotebook(nb, now)
		return "notebook", nb.ID, "live", encoded, err
	})
	defer clear(raw)
	var nb api.Notebook
	if err == nil {
		err = json.Unmarshal(raw, &nb)
	}
	return nb, err
}

// DailyNotebook abre el diario del día civil elegido. `carry` traslada antes las tareas
// pendientes de días anteriores (solo se pide para hoy).
func (r *Repository) DailyNotebook(parent context.Context, date, title string, carry bool) (api.Notebook, error) {
	title, err := tituloDiario(date, title)
	if err != nil {
		return api.Notebook{}, err
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Notebook{}, err
	}
	defer done()
	nb, err := r.abrirDiario(ctx, date, title)
	if err != nil || !carry {
		return nb, err
	}
	return nb, r.trasladar(ctx, nb)
}

// personalEnClaro dice si un cuaderno se puede usar de origen o destino de un traslado:
// personal, sin cifrar. Un traslado nunca amplía quién puede leer una nota.
func personalEnClaro(nb api.Notebook) bool {
	return !nb.Encrypted && (nb.Visibility == "" || nb.Visibility == "personal" || nb.Visibility == "private")
}

// trasladar lleva al diario `destino` las tareas pendientes de días anteriores, cada una
// con todo lo que cuelga de ella. Son raíces las tareas pendientes sueltas y las que
// cuelgan de una tarea terminada de primer nivel (el padre se queda en su día). Una
// cancelada no se traslada nunca. Cada raíz suma uno a `data.carry_count` y recuerda el
// primer día del que salió en `data.carried_from`.
func (r *Repository) trasladar(ctx context.Context, destino api.Notebook) error {
	if !personalEnClaro(destino) {
		return nil
	}
	books, err := r.notebooks(ctx)
	if err != nil {
		return err
	}
	todas, err := r.nodos(ctx)
	if err != nil {
		return err
	}
	raices := raicesDelTraslado(todas, books, destino)
	fin := finalDe(todas, destino.ID)
	for i, n := range raices {
		if vetada(todas, n.ID, r.cfg.Scope.Subject) {
			continue
		}
		data, err := r.datosTrasladados(ctx, n.ID, books[n.NotebookID].DailyDate)
		if err != nil {
			return err
		}
		if err := r.moverRama(ctx, n.ID, destino.ID, fin+i+1, map[string]any{"data": data}); err != nil {
			return err
		}
	}
	return nil
}

// raicesDelTraslado son las raíces que trasladar lleva a destino, por día, posición,
// creación e id.
func raicesDelTraslado(todas map[string]nodo, books map[string]api.Notebook, destino api.Notebook) []nodo {
	pendiente := func(n nodo) bool {
		return n.Kind == "todo" && !n.Checked && n.Cancelled == nil && n.Deleted == nil
	}
	var raices []nodo
	for _, n := range todas {
		old, ok := books[n.NotebookID]
		if !ok || old.ID == destino.ID || old.DailyDate == "" || old.DailyDate >= destino.DailyDate || !personalEnClaro(old) || !pendiente(n) {
			continue
		}
		if n.GroupID != nil {
			padre, ok := todas[*n.GroupID]
			if !ok || padre.GroupID != nil || padre.Kind != "todo" || (!padre.Checked && padre.Cancelled == nil) {
				continue
			}
		}
		raices = append(raices, n)
	}
	slices.SortFunc(raices, func(a, b nodo) int {
		return cmp.Or(cmp.Compare(books[a.NotebookID].DailyDate, books[b.NotebookID].DailyDate),
			cmp.Compare(a.Position, b.Position), a.Created.Compare(b.Created), cmp.Compare(a.ID, b.ID))
	})
	return raices
}

// vetada: si un solo nodo de la rama es de otra persona o está cifrado, la rama entera
// se queda donde está.
func vetada(todas map[string]nodo, raiz, yo string) bool {
	for _, n := range rama(todas, raiz) {
		if n.Owner != yo || n.Cipher != "" {
			return true
		}
	}
	return false
}

// datosTrasladados es el `data` de la nota con la cuenta del traslado.
func (r *Repository) datosTrasladados(ctx context.Context, id, origen string) (json.RawMessage, error) {
	raw, err := r.ReadRaw(ctx, "note", id)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var n struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &n) != nil {
		return nil, ErrSource
	}
	if n.Data == nil {
		n.Data = map[string]json.RawMessage{}
	}
	if _, ok := n.Data["carried_from"]; !ok {
		_ = put(n.Data, "carried_from", origen)
	}
	cuenta := 0
	var texto string
	if json.Unmarshal(n.Data["carry_count"], &texto) == nil {
		cuenta, _ = strconv.Atoi(texto)
	} else {
		_ = json.Unmarshal(n.Data["carry_count"], &cuenta)
	}
	_ = put(n.Data, "carry_count", max(cuenta, 0)+1)
	return json.Marshal(n.Data)
}

// DailyRange son los días con notas de [from, to], en orden, con sus cuentas y, si se
// piden, sus notas.
func (r *Repository) DailyRange(ctx context.Context, from, to string, notas bool) ([]api.DailyDay, error) {
	maximo := maxDiasCuentas
	if notas {
		maximo = maxDiasNotas
	}
	if !rangoDiario(from, to, maximo) {
		return nil, model.ErrInvalidInput
	}
	return r.dias(ctx, func(d string) bool { return d >= from && d <= to }, false, notas)
}

// ThisDay es «Este día»: el mismo día y mes de años anteriores, del más reciente atrás.
func (r *Repository) ThisDay(ctx context.Context, date string) ([]api.DailyDay, error) {
	if !fechaDiaria(date) {
		return nil, model.ErrInvalidInput
	}
	return r.dias(ctx, func(d string) bool { return d[5:] == date[5:] && d < date }, true, true)
}

// dias son los diarios que cumplen `dentro` con al menos una nota viva. Los cifrados
// dan cuentas pero no notas, que aquí serían criptogramas sin la clave.
func (r *Repository) dias(parent context.Context, dentro func(string) bool, desc, notas bool) ([]api.DailyDay, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	books, all, err := r.visibleNotes(ctx)
	if err != nil {
		return nil, err
	}
	porDia := map[string]*api.DailyDay{}
	for _, n := range all {
		nb := books[n.NotebookID]
		if nb.DailyDate == "" || nb.OwnerSub != r.cfg.Scope.Subject || len(nb.DailyDate) != len(time.DateOnly) || !dentro(nb.DailyDate) {
			continue
		}
		d := porDia[nb.ID]
		if d == nil {
			d = &api.DailyDay{Date: nb.DailyDate, NotebookID: nb.ID}
			porDia[nb.ID] = d
		}
		d.Total++
		if n.Kind == "todo" && !n.Checked && n.CancelledAt == nil {
			d.Pending++
		}
		if notas && !nb.Encrypted {
			d.Notes = append(d.Notes, n)
		}
	}
	out := make([]api.DailyDay, 0, len(porDia))
	for _, d := range porDia {
		sortNotes(d.Notes)
		out = append(out, *d)
	}
	slices.SortFunc(out, func(a, b api.DailyDay) int {
		if desc {
			return cmp.Or(cmp.Compare(b.Date, a.Date), cmp.Compare(a.NotebookID, b.NotebookID))
		}
		return cmp.Or(cmp.Compare(a.Date, b.Date), cmp.Compare(a.NotebookID, b.NotebookID))
	})
	return out[:min(len(out), maxDiasCuentas)], ctx.Err()
}

// ListTasks devuelve las tareas (`kind=todo`) de todos los cuadernos en un estado:
// abiertas, hechas o canceladas, de la más reciente a la más antigua por la fecha de
// ese estado. Ni las de cuadernos cifrados ni las que viven dentro del HTML de una nota.
func (r *Repository) ListTasks(parent context.Context, estado string) ([]api.Tarea, error) {
	if estado == "" {
		estado = api.TareaAbierta
	}
	if estado != api.TareaAbierta && estado != api.TareaHecha && estado != api.TareaCancelada {
		return nil, model.ErrInvalidInput
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	books, all, err := r.visibleNotes(ctx)
	if err != nil {
		return nil, err
	}
	out := []api.Tarea{}
	for _, n := range all {
		nb := books[n.NotebookID]
		if n.Kind != "todo" || nb.Encrypted || n.ContentCipher != "" {
			continue
		}
		switch estado {
		case api.TareaAbierta:
			if n.Checked || n.CancelledAt != nil {
				continue
			}
		case api.TareaHecha:
			if !n.Checked {
				continue
			}
		case api.TareaCancelada:
			if n.CancelledAt == nil {
				continue
			}
		}
		out = append(out, api.Tarea{Note: n, Notebook: nb.Title, DailyDate: nb.DailyDate})
	}
	fecha := func(t api.Tarea) time.Time {
		switch {
		case t.CheckedAt != nil:
			return *t.CheckedAt
		case t.CancelledAt != nil:
			return *t.CancelledAt
		}
		return t.CreatedAt
	}
	slices.SortFunc(out, func(a, b api.Tarea) int { return cmp.Or(fecha(b).Compare(fecha(a)), cmp.Compare(a.ID, b.ID)) })
	return out[:min(len(out), topeTareas)], ctx.Err()
}

// PostponeTask lleva una nota, con todo lo que cuelga de ella, al diario de `date`
// (creándolo si hace falta). Una nota cifrada, o de un cuaderno compartido o cifrado, no
// se mueve: contesta ErrNotFound, como una que no existe.
func (r *Repository) PostponeTask(parent context.Context, id, date, title string) (api.Notebook, error) {
	title, err := tituloDiario(date, title)
	if err != nil {
		return api.Notebook{}, err
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Notebook{}, err
	}
	defer done()
	note, origen, err := r.notaViva(ctx, id)
	if err != nil {
		return api.Notebook{}, err
	}
	nb, err := r.abrirDiario(ctx, date, title)
	if err != nil {
		return api.Notebook{}, err
	}
	if note.NotebookID == nb.ID || !personalEnClaro(origen) || !personalEnClaro(nb) {
		return api.Notebook{}, ErrNotFound
	}
	todas, err := r.nodos(ctx)
	if err != nil {
		return api.Notebook{}, err
	}
	if vetada(todas, id, r.cfg.Scope.Subject) {
		return api.Notebook{}, ErrNotFound
	}
	return nb, r.moverRama(ctx, id, nb.ID, finalDe(todas, nb.ID)+1, nil)
}
