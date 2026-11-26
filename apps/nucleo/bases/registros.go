// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"
)

// mundo es lo que hace falta para leer registros como el servicio: los campos de cada
// tabla, todos los registros (los enlazados de otra tabla también: de ellos salen los
// agregados de las fórmulas) y las fichas de adjunto.
type mundo struct {
	campos    map[string][]dominio.Campo
	registros map[string]api.BasesRecord
	adjuntos  map[string]map[string][]api.BasesAttachment // registro → campo → fichas
}

func (r *Repository) mundo(ctx context.Context) (mundo, error) {
	m := mundo{registros: map[string]api.BasesRecord{}, adjuntos: map[string]map[string][]api.BasesAttachment{}}
	var err error
	if m.campos, err = r.porTabla(ctx); err != nil {
		return m, err
	}
	regs, err := leerTodos[api.BasesRecord](ctx, r, "record")
	if err != nil {
		return m, err
	}
	for _, x := range regs {
		m.registros[x.ID] = x
	}
	fichas, err := leerTodos[fichaAdjunto](ctx, r, "file")
	if err != nil {
		return m, err
	}
	slices.SortFunc(fichas, func(a, b fichaAdjunto) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for _, f := range fichas {
		if m.adjuntos[f.RecordID] == nil {
			m.adjuntos[f.RecordID] = map[string][]api.BasesAttachment{}
		}
		m.adjuntos[f.RecordID][f.FieldID] = append(m.adjuntos[f.RecordID][f.FieldID], f.BasesAttachment)
	}
	return m, nil
}

// completo es el registro como lo enseña el servicio: con sus fórmulas calculadas
// (también las que agregan los enlazados), sus adjuntos y sin enlaces a registros que
// ya no existen.
func (m mundo) completo(rec api.BasesRecord) api.BasesRecord {
	campos := m.campos[rec.TableID]
	links := map[string][]string{}
	for campo, ids := range rec.Links {
		for _, id := range ids {
			if _, ok := m.registros[id]; ok {
				links[campo] = append(links[campo], id)
			}
		}
	}
	rec.Links = links
	if a := m.adjuntos[rec.ID]; len(a) > 0 {
		rec.Attachments = a
	}
	hay := slices.ContainsFunc(campos, func(c dominio.Campo) bool { return c.Kind == dominio.TipoFormula })
	if !hay {
		return rec
	}
	datos, _ := crudos(rec.Data)
	enlaces := map[string]int{}
	agregados := map[dominio.RefFormula][]json.RawMessage{}
	for _, c := range campos {
		if c.Kind == dominio.TipoRelacion {
			enlaces[c.ID] = len(links[c.ID])
		}
		if c.Kind != dominio.TipoFormula {
			continue
		}
		f, err := dominio.AnalizarFormula(c.Formula)
		if err != nil {
			continue
		}
		for _, ref := range f.Refs {
			if ref.Enlace == "" {
				continue
			}
			var vs []json.RawMessage
			for _, id := range links[ref.Enlace][:min(len(links[ref.Enlace]), dominio.MaxEnlaces)] {
				otro, _ := crudos(m.registros[id].Data)
				vs = append(vs, otro[ref.Campo])
			}
			agregados[ref] = vs
		}
	}
	calculado := dominio.EvaluarFormulas(campos, datos, enlaces, agregados)
	nuevo := make(map[string]any, len(rec.Data)+len(calculado))
	for k, v := range rec.Data {
		nuevo[k] = v
	}
	for k, v := range valoresConNulos(calculado) {
		if v == nil {
			delete(nuevo, k)
		} else {
			nuevo[k] = v
		}
	}
	rec.Data = nuevo
	return rec
}

func valoresConNulos(datos map[string]json.RawMessage) map[string]any {
	out := make(map[string]any, len(datos))
	for k, raw := range datos {
		var v any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &v)
		}
		out[k] = v
	}
	return out
}

// cursorLocal es la posición en una lectura: el desplazamiento y la huella de la
// lectura (tabla, vista, búsqueda, orden), para que un cursor no valga en otra.
type cursorLocal struct {
	Desde  int    `json:"o"`
	Huella string `json:"h"`
}

func huellaDeLectura(tabla, vista string, l api.BasesLectura) string {
	s := sha256.Sum256([]byte(tabla + "\x00" + vista + "\x00" + l.Q + "\x00" + l.Sort))
	return hex.EncodeToString(s[:8])
}

// ListRecords es una página de registros de la tabla con el filtro y el orden de la
// vista, evaluados en el dispositivo como los evalúa el SQL del servicio (B-02).
func (r *Repository) ListRecords(ctx context.Context, tabla, vista, cursor string, limite int) (api.BasesRecordPage, error) {
	return r.ListRecordsCon(ctx, tabla, vista, cursor, limite, api.BasesLectura{})
}

// ListRecordsCon es ListRecords con la búsqueda y el orden de un momento (los de
// `BasesLectura`), sobre la tabla entera y paginando con ellos.
func (r *Repository) ListRecordsCon(ctx context.Context, tabla, vista, cursor string, limite int, l api.BasesLectura) (api.BasesRecordPage, error) {
	todos, campos, err := r.leerTabla(ctx, tabla, vista, l)
	if err != nil {
		return api.BasesRecordPage{}, err
	}
	if limite <= 0 {
		limite = dominio.PaginaPorDefecto
	}
	limite = min(limite, dominio.MaxPagina)
	huella := huellaDeLectura(tabla, vista, l)
	desde := 0
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		var c cursorLocal
		if err != nil || len(cursor) > dominio.MaxCursor || json.Unmarshal(raw, &c) != nil || c.Huella != huella || c.Desde < 0 {
			return api.BasesRecordPage{}, errorDe(dominio.ErrInvalido)
		}
		desde = min(c.Desde, len(todos))
	}
	hasta := min(desde+limite, len(todos))
	pagina := api.BasesRecordPage{Fields: campos, Records: todos[desde:hasta], Role: api.BasesOwner}
	if hasta < len(todos) {
		raw, _ := json.Marshal(cursorLocal{Desde: hasta, Huella: huella})
		pagina.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return pagina, nil
}

// leerTabla son los registros de la tabla que cumplen la vista y la lectura, en su
// orden, ya completos.
func (r *Repository) leerTabla(ctx context.Context, tabla, vista string, l api.BasesLectura) ([]api.BasesRecord, []api.BasesField, error) {
	campos, err := r.camposDeTabla(ctx, tabla)
	if err != nil {
		return nil, nil, err
	}
	var cfg dominio.ConfigVista
	if vista != "" {
		v, err := leerUno[api.BasesView](ctx, r, "view", vista)
		if err != nil || v.TableID != tabla {
			return nil, nil, errorDe(dominio.ErrNoExiste)
		}
		if cfg, err = configDe(v.Config); err != nil {
			return nil, nil, errorDe(err)
		}
	}
	cfg.Search = l.Q
	if l.Sort != "" {
		if cfg.Sort, err = ordenDe(l.Sort); err != nil {
			return nil, nil, errorDe(err)
		}
	}
	m, err := r.mundo(ctx)
	if err != nil {
		return nil, nil, err
	}
	var regs []dominio.Registro
	completos := map[string]api.BasesRecord{}
	for _, rec := range m.registros {
		if rec.TableID != tabla {
			continue
		}
		c := m.completo(rec)
		completos[c.ID] = c
		regs = append(regs, registroDe(c))
	}
	ordenados := dominio.FiltrarYOrdenar(m.campos[tabla], regs, cfg)
	out := make([]api.BasesRecord, len(ordenados))
	for i, x := range ordenados {
		out[i] = completos[x.ID]
	}
	if campos == nil {
		campos = []api.BasesField{}
	}
	return out, campos, nil
}

func (r *Repository) GetRecord(ctx context.Context, registro string) (api.BasesRecord, error) {
	rec, err := leerUno[api.BasesRecord](ctx, r, "record", registro)
	if err != nil {
		return api.BasesRecord{}, err
	}
	m, err := r.mundo(ctx)
	if err != nil {
		return api.BasesRecord{}, err
	}
	return m.completo(rec), nil
}

// validarEnlaces comprueba los enlaces de un registro: a registros que existen en la
// tabla de destino del campo.
func validarEnlaces(campos []dominio.Campo, m mundo, enlaces map[string][]string) (map[string][]string, error) {
	out := map[string][]string{}
	for campo, ids := range enlaces {
		i := slices.IndexFunc(campos, func(c dominio.Campo) bool { return c.ID == campo })
		if i < 0 || campos[i].Kind != dominio.TipoRelacion {
			return nil, dominio.ErrInvalido
		}
		limpios, err := dominio.ValidarEnlaces(ids)
		if err != nil {
			return nil, err
		}
		for _, id := range limpios {
			if destino, ok := m.registros[id]; !ok || destino.TableID != campos[i].LinkTableID {
				return nil, dominio.ErrInvalido
			}
		}
		if len(limpios) > 0 {
			out[campo] = limpios
		}
	}
	return out, nil
}

func (r *Repository) CreateRecord(ctx context.Context, tabla string, datos map[string]any, enlaces map[string][]string) (api.BasesRecord, error) {
	if _, err := leerUno[api.BasesTable](ctx, r, "table", tabla); err != nil {
		return api.BasesRecord{}, err
	}
	m, err := r.mundo(ctx)
	if err != nil {
		return api.BasesRecord{}, err
	}
	campos := m.campos[tabla]
	entrada, err := crudos(datos)
	if err != nil {
		return api.BasesRecord{}, errorDe(err)
	}
	limpios, err := dominio.ValidarDatos(campos, entrada)
	if err == nil {
		err = dominio.TamanoDatos(limpios)
	}
	if err != nil {
		return api.BasesRecord{}, errorDe(err)
	}
	links, err := validarEnlaces(campos, m, enlaces)
	if err != nil {
		return api.BasesRecord{}, errorDe(err)
	}
	// La operación es el alta tal como se pide, ya limpia: repetirla tras un corte da el
	// MISMO registro (el que quedó pendiente), no otro.
	alta := struct {
		Tabla   string                     `json:"table"`
		Datos   map[string]json.RawMessage `json:"data"`
		Enlaces map[string][]string        `json:"links"`
	}{tabla, limpios, links}
	rec, err := crear(ctx, r, "record-create", "record", alta, func(context.Context) (string, api.BasesRecord, error) {
		now := time.Now().UTC()
		rec := api.BasesRecord{
			ID: uuid.NewString(), TableID: tabla, Data: valores(limpios), Links: links, Rev: 1,
			CreatedBy: r.scope.Subject, UpdatedBy: r.scope.Subject, CreatedAt: now, UpdatedAt: now,
		}
		return rec.ID, rec, nil
	})
	if err != nil {
		return api.BasesRecord{}, err
	}
	m.registros[rec.ID] = rec
	return m.completo(rec), nil
}

// WriteCell escribe UNA celda validada como en el servicio. Conflict dice que lo que
// había no era lo que quien escribe veía (gana el último).
func (r *Repository) WriteCell(ctx context.Context, registro, campo string, celda api.BasesCell, op string) (api.BasesCellResult, error) {
	m, err := r.mundo(ctx)
	if err != nil {
		return api.BasesCellResult{}, err
	}
	actual, ok := m.registros[registro]
	if !ok {
		return api.BasesCellResult{}, errorDe(dominio.ErrNoExiste)
	}
	campos := m.campos[actual.TableID]
	var valor json.RawMessage
	var links []string
	esEnlace := celda.Links != nil
	if esEnlace {
		limpios, err := validarEnlaces(campos, m, map[string][]string{campo: celda.Links})
		if err != nil {
			return api.BasesCellResult{}, errorDe(err)
		}
		links = limpios[campo]
	} else {
		raw, err := crudos(map[string]any{campo: celda.Value})
		if err != nil {
			return api.BasesCellResult{}, errorDe(err)
		}
		previos, _ := json.Marshal(actual.Data)
		limpio, err := dominio.ValidarParcial(dominio.ConRetiradas(campos, previos), raw)
		if err != nil {
			return api.BasesCellResult{}, errorDe(err)
		}
		valor = limpio[campo]
	}
	if op == "" {
		op = uuid.NewString()
	}
	// Repetir la misma op tras un corte no escribe otra vez: devuelve el registro que
	// escribió la primera, con Replayed y sin Conflict, como el servicio.
	var res api.BasesCellResult
	res.Record, res.Replayed, err = cambiarFila(ctx, r, "record-cell", "record", registro, map[string]string{"record": registro, "field": campo, "op": op}, func(rec *api.BasesRecord) error {
		if rec.Data == nil {
			rec.Data = map[string]any{}
		}
		if rec.Links == nil {
			rec.Links = map[string][]string{}
		}
		antes := rec.Data[campo]
		switch {
		case esEnlace:
			antes = rec.Links[campo]
			if len(links) == 0 {
				delete(rec.Links, campo)
			} else {
				rec.Links[campo] = links
			}
		case valor == nil:
			delete(rec.Data, campo)
		default:
			var v any
			_ = json.Unmarshal(valor, &v)
			rec.Data[campo] = v
		}
		res.Conflict = !mismoValor(antes, celda.Expected)
		data, _ := crudos(rec.Data)
		if err := dominio.TamanoDatos(data); err != nil {
			return errorDe(err)
		}
		rec.Rev++
		rec.UpdatedBy = r.scope.Subject
		rec.UpdatedAt = time.Now().UTC()
		return nil
	})
	if err != nil {
		return api.BasesCellResult{}, err
	}
	m.registros[registro] = res.Record
	res.Record = m.completo(res.Record)
	return res, nil
}

// mismoValor compara dos valores del cable por su JSON (nil, vacío y lista vacía son
// lo mismo: «no había nada»).
func mismoValor(a, b any) bool {
	norm := func(v any) []byte {
		raw, _ := json.Marshal(v)
		if s := string(raw); s == "null" || s == "[]" || s == `""` {
			return nil
		}
		return raw
	}
	return bytes.Equal(norm(a), norm(b))
}

// DeleteRecord borra un registro. Si otros lo enlazan, sin `forzar` es un conflicto
// («referenced»); con él, se quitan antes esos enlaces, como en el servicio.
func (r *Repository) DeleteRecord(ctx context.Context, registro string, forzar bool) error {
	m, err := r.mundo(ctx)
	if err != nil {
		return err
	}
	if _, ok := m.registros[registro]; !ok {
		return errorDe(dominio.ErrNoExiste)
	}
	var enlazan []string
	for id, otro := range m.registros {
		for _, ids := range otro.Links {
			if id != registro && slices.Contains(ids, registro) {
				enlazan = append(enlazan, id)
				break
			}
		}
	}
	if len(enlazan) > 0 && !forzar {
		return errorDe(dominio.ErrReferenciado)
	}
	slices.Sort(enlazan)
	for _, id := range enlazan {
		if _, err := cambiar(ctx, r, "record-unlink", "record", id, []string{id, registro}, func(rec *api.BasesRecord) error {
			for campo, ids := range rec.Links {
				rec.Links[campo] = slices.DeleteFunc(slices.Clone(ids), func(x string) bool { return x == registro })
				if len(rec.Links[campo]) == 0 {
					delete(rec.Links, campo)
				}
			}
			rec.Rev++
			rec.UpdatedAt = time.Now().UTC()
			return nil
		}); err != nil {
			return err
		}
	}
	return r.trashRow(ctx, "record-delete", "record", registro)
}
