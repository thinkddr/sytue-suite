// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// # Traer un cuaderno de Sytue (HU-07)
//
// «Traer desde Sytue» (el Hub de Sytue Office, `nucleo/local/traer`) trae cada cuaderno de
// la copia de «Tus datos» con [Repository.TraerCuaderno]: es ImportNotebook —la misma
// lectura, el mismo saneado y las mismas reglas— con lo que una mudanza desde la edición
// conectada pide además y un volcado de otro programa no trae:
//
//   - las FECHAS: cada nota conserva cuándo se creó y cuándo cambió por última vez, y una
//     tarea, cuándo se hizo o se canceló; el cuaderno, cuándo se creó y cuándo cambió;
//   - las ETIQUETAS con estados (`states` de cada nota): la etiqueta y el estado se buscan
//     por su nombre entre las de aquí y, si no están, se crean con su emoji y su color;
//   - el ORIGEN: el cuaderno y cada nota guardan en su fila el identificador que tenían
//     en Sytue ([CampoOrigen]). Las escrituras de siempre reescriben sobre la fila que hay
//     y lo conservan, así que traer otra vez la misma copia no repite nada aunque entre
//     medias se hayan editado: el cuaderno que ya está recibe solo las notas que le faltan
//     (las que se borraron aquí no vuelven), y uno que no está se crea;
//   - el ÁRBOL: el cuaderno nuevo cuelga del cuaderno que se diga (el de aquí que vino de
//     su padre en Sytue);
//   - el DIARIO: un cuaderno del diario de Sytue (con `daily_date`) entra como el diario
//     de su día, el mismo que abre Notepads (diario.go): si aquí ya hay uno de ese día,
//     sus notas se suman a él (y guarda el origen, para no repetirlas); si no, se crea
//     con su título.
//
// Como en ImportNotebook, lo que el repositorio no puede guardar (una nota que apunta a
// un fichero de Drive o a un documento de Docs) se cuenta como descartado, y un cuaderno
// cifrado no entra.

// CampoOrigen es el campo de la fila de un cuaderno o una nota traídos con el
// identificador que tenían en Sytue. Ninguna pantalla lo lee: solo TraerCuaderno.
const CampoOrigen = "origen_sytue"

// CuadernoTraido es cómo ha ido TraerCuaderno.
type CuadernoTraido struct {
	// Notebook es el cuaderno de aquí: el nuevo o el que ya estaba de otra vez.
	Notebook api.Notebook
	// Origen es el identificador que tenía en Sytue.
	Origen string
	// Nuevo dice si el cuaderno se ha creado ahora.
	Nuevo bool
	// Importadas son las notas que han entrado ahora; Repetidas, las que ya estaban de
	// otra vez; Descartadas, las que no pasan la validación o no se pueden guardar aquí.
	Importadas, Repetidas, Descartadas int
}

// Traidos es lo que ya se trajo de Sytue a este repositorio, por el origen de cada
// cuaderno y cada nota: se lee una vez ([Repository.LoTraido]) para traer muchos
// cuadernos seguidos, y TraerCuaderno lo pone al día.
type Traidos struct {
	cuadernos map[string]string
	notas     map[string]bool
}

// Cuaderno es el cuaderno de aquí que vino del de Sytue con ese identificador, si se ve.
func (t *Traidos) Cuaderno(origen string) (string, bool) {
	id, ok := t.cuadernos[origen]
	return id, ok && origen != ""
}

// LoTraido lee lo que ya se trajo: los cuadernos traídos que se ven y los orígenes de las
// notas traídas que hay en cualquier cuaderno que se vea, también las que están en la
// papelera (lo que se tiró no vuelve).
func (r *Repository) LoTraido(parent context.Context) (*Traidos, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	vivos, err := r.notebooks(ctx)
	if err != nil {
		return nil, err
	}
	type fila struct {
		ID       string `json:"id"`
		Notebook string `json:"notebook_id"`
		Origen   string `json:"origen_sytue"`
	}
	t := &Traidos{cuadernos: map[string]string{}, notas: map[string]bool{}}
	err = r.eachRow(ctx, "notebook", func(raw []byte, estado string) error {
		var f fila
		if json.Unmarshal(raw, &f) != nil {
			return ErrSource
		}
		if f.Origen != "" && estado == "live" && visibleNotebook(vivos, f.ID) {
			t.cuadernos[f.Origen] = f.ID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = r.eachRow(ctx, "note", func(raw []byte, _ string) error {
		var f fila
		if json.Unmarshal(raw, &f) != nil {
			return ErrSource
		}
		if f.Origen != "" && visibleNotebook(vivos, f.Notebook) {
			t.notas[f.Origen] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// TraerCuaderno trae el cuaderno del volcado (un `.onex`, o el que lleva dentro el HTML
// nativo: model.VolcadoDeHTML) sin repetir lo que ya se trajo. t es lo ya traído (nil: se
// lee ahora); padre, el cuaderno de aquí del que cuelga si es nuevo (vacío, la raíz).
func (r *Repository) TraerCuaderno(parent context.Context, t *Traidos, volcado []byte, nombre, padre string) (CuadernoTraido, error) {
	libro, fuente, leidas, descartadas, err := leerVolcadoEntero(volcado, nombre)
	if err != nil {
		return CuadernoTraido{}, err
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return CuadernoTraido{}, err
	}
	defer done()
	if t == nil {
		if t, err = r.LoTraido(ctx); err != nil {
			return CuadernoTraido{}, err
		}
	}
	res := CuadernoTraido{Descartadas: descartadas}
	if fuente.ID != uuid.Nil {
		res.Origen = fuente.ID.String()
	}
	if id, ya := t.Cuaderno(res.Origen); ya {
		if res.Notebook, err = r.GetNotebook(ctx, id); err != nil {
			return CuadernoTraido{}, err
		}
	} else {
		if res.Notebook, res.Nuevo, err = r.cuadernoNuevoTraido(ctx, libro, fuente, padre); err != nil {
			return CuadernoTraido{}, err
		}
		if err := r.marcarCuadernoTraido(ctx, t, &res, fuente); err != nil {
			return res, err
		}
	}
	// traerNotas rellena res: se llama antes de devolverlo.
	err = r.traerNotas(ctx, t, &res, leidas)
	return res, err
}

// cuadernoNuevoTraido es el cuaderno de aquí al que va uno que no se había traído: un
// cuaderno del diario entra como el diario de su día (el de aquí si ya lo hay, y sus
// notas se suman a las suyas, o uno nuevo con su título); otro, como cuaderno nuevo
// dentro de padre. nuevo dice si se creó.
func (r *Repository) cuadernoNuevoTraido(ctx context.Context, libro api.NotebookInput, fuente model.Notebook, padre string) (nb api.Notebook, nuevo bool, err error) {
	if fechaDiaria(fuente.DailyDate) {
		return r.diarioTraido(ctx, fuente.DailyDate, libro.Title)
	}
	if padre != "" {
		libro.ParentID = &padre
	}
	if nb, err = r.CreateNotebook(ctx, libro); err != nil {
		return api.Notebook{}, false, err
	}
	return nb, true, nil
}

// marcarCuadernoTraido apunta en el cuaderno de res su origen en Sytue (para no volver a
// traerlo) y, si es nuevo, las fechas del de Sytue; y lo vuelve a leer.
func (r *Repository) marcarCuadernoTraido(ctx context.Context, t *Traidos, res *CuadernoTraido, fuente model.Notebook) error {
	cambios := map[string]any{}
	if res.Nuevo {
		cambios = conFechas(cambios, fuente.CreatedAt, fuente.UpdatedAt)
	}
	if res.Origen != "" {
		cambios[CampoOrigen] = res.Origen
		t.cuadernos[res.Origen] = res.Notebook.ID
	}
	if len(cambios) == 0 {
		return nil
	}
	if err := r.reescribir(ctx, "traer-notebook", "notebook", res.Notebook.ID, cambios, false); err != nil {
		return err
	}
	var err error
	res.Notebook, err = r.GetNotebook(ctx, res.Notebook.ID)
	return err
}

// traerNotas crea en el cuaderno de res las notas leídas que no se habían traído, con
// sus etiquetas, y lleva la cuenta en res. Una nota que no se puede crear se descarta.
func (r *Repository) traerNotas(ctx context.Context, t *Traidos, res *CuadernoTraido, leidas []notaLeida) error {
	etiquetas := &etiquetasTraidas{r: r}
	for _, l := range leidas {
		origen := ""
		if l.fuente.ID != uuid.Nil {
			origen = l.fuente.ID.String()
		}
		if origen != "" && t.notas[origen] {
			res.Repetidas++
			continue
		}
		nota, err := r.crearTraida(ctx, res.Notebook.ID, l, origen)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			res.Descartadas++
			continue
		}
		if origen != "" {
			t.notas[origen] = true // un volcado con dos notas iguales no las repite
		}
		res.Importadas++
		for _, s := range l.fuente.States {
			// Una etiqueta que no se puede crear aquí no tira la nota: entra sin ella.
			_ = etiquetas.marcar(ctx, nota.ID, s)
		}
	}
	return nil
}

// diarioTraido es el diario del día fecha al que va un cuaderno del diario de Sytue: el
// de aquí, si lo hay, o uno nuevo con el título del de Sytue (nuevo dice cuál). Un
// diario va siempre en la raíz, como los que abre Notepads.
func (r *Repository) diarioTraido(ctx context.Context, fecha, titulo string) (nb api.Notebook, nuevo bool, err error) {
	if nb, ya, err := r.diarioDe(ctx, fecha); err != nil || ya {
		return nb, false, err
	}
	if titulo, err = tituloDiario(fecha, titulo); err != nil {
		titulo = fecha
	}
	nb, err = r.abrirDiario(ctx, fecha, titulo)
	return nb, err == nil, err
}

// crearTraida crea la nota leída en el cuaderno con sus fechas y su origen, en UNA
// escritura: la de crearla (prepareNote) con esos campos encima.
func (r *Repository) crearTraida(ctx context.Context, cuaderno string, l notaLeida, origen string) (api.Note, error) {
	cambios := conFechas(map[string]any{}, l.fuente.CreatedAt, l.fuente.UpdatedAt)
	if origen != "" {
		cambios[CampoOrigen] = origen
	}
	if l.fuente.Checked && l.fuente.CheckedAt != nil {
		cambios["checked_at"] = l.fuente.CheckedAt.UTC()
	}
	if l.fuente.CancelledAt != nil {
		cambios["cancelled_at"] = l.fuente.CancelledAt.UTC()
	}
	in := l.entrada
	raw, err := r.mutate(ctx, "traer-note", struct {
		Notebook string
		Input    api.NoteInput
		Cambios  map[string]any
	}{cuaderno, in, cambios}, func(ctx context.Context) (string, string, string, []byte, error) {
		kind, id, estado, encoded, err := r.prepareNote(ctx, "", cuaderno, in)
		if err != nil {
			return "", "", "", nil, err
		}
		fields, err := rawObject(encoded)
		clear(encoded)
		if err != nil {
			return "", "", "", nil, err
		}
		for k, v := range cambios {
			if err := put(fields, k, v); err != nil {
				return "", "", "", nil, err
			}
		}
		encoded, err = json.Marshal(fields)
		return kind, id, estado, encoded, err
	})
	defer clear(raw)
	var nota api.Note
	if err == nil {
		err = json.Unmarshal(raw, &nota)
	}
	return nota, err
}

// conFechas pone en cambios las fechas que se saben.
func conFechas(cambios map[string]any, creado, cambiado time.Time) map[string]any {
	if !creado.IsZero() {
		cambios["created_at"] = creado.UTC()
	}
	if !cambiado.IsZero() {
		cambios["updated_at"] = cambiado.UTC()
	}
	return cambios
}

// etiquetasTraidas busca y crea las etiquetas de las notas traídas, por su nombre.
type etiquetasTraidas struct {
	r      *Repository
	tags   []api.Tag
	leidas bool
}

// marcar pone en la nota el estado s, creando antes la etiqueta o el estado si faltan.
func (e *etiquetasTraidas) marcar(ctx context.Context, nota string, s model.NoteState) error {
	nombre, estado := strings.TrimSpace(s.TagName), strings.TrimSpace(s.Name)
	if nombre == "" || estado == "" {
		return nil
	}
	if !e.leidas {
		tags, err := e.r.ListTags(ctx)
		if err != nil {
			return err
		}
		e.tags, e.leidas = tags, true
	}
	i := -1
	for j, t := range e.tags {
		if strings.EqualFold(strings.TrimSpace(t.Name), nombre) {
			i = j
			break
		}
	}
	nuevo := api.TagStateInput{Name: estado, Emoji: s.Emoji, Color: s.Color}
	if i < 0 {
		t, err := e.r.SaveTag(ctx, "", api.TagInput{Name: nombre, States: []api.TagStateInput{nuevo}})
		if err != nil {
			return err
		}
		e.tags = append(e.tags, t)
		i = len(e.tags) - 1
	}
	t := e.tags[i]
	sid := ""
	for _, st := range t.States {
		if strings.EqualFold(strings.TrimSpace(st.Name), estado) {
			sid = st.ID
			break
		}
	}
	if sid == "" {
		// Se guardan TODOS los que tiene (con su id) y el nuevo al final.
		todos := make([]api.TagStateInput, 0, len(t.States)+1)
		for _, st := range t.States {
			id := st.ID
			todos = append(todos, api.TagStateInput{ID: &id, Name: st.Name, Emoji: st.Emoji, Color: st.Color, Shortcut: st.Shortcut})
		}
		guardada, err := e.r.SaveTag(ctx, t.ID, api.TagInput{Name: t.Name, States: append(todos, nuevo)})
		if err != nil {
			return err
		}
		e.tags[i], t = guardada, guardada
		for _, st := range t.States {
			if strings.EqualFold(strings.TrimSpace(st.Name), estado) {
				sid = st.ID
			}
		}
		if sid == "" {
			return ErrSource
		}
	}
	return e.r.SetNoteState(ctx, nota, t.ID, sid)
}
