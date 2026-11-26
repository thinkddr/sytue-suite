// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"
)

// Lo que el repositorio hace como el servicio (PLAN-EDICION-LOCAL.md §6.12, B-03 y
// B-04): las MISMAS reglas, de `dominio`, aplicadas al escribir y al leer en el
// dispositivo. Este fichero traduce entre los tipos del cable (`api.Bases*`) y los del
// dominio, y los errores del dominio a los que esperan las pantallas.

// errorDe pasa un error del dominio al `*api.Error` que leen las pantallas (su estado y
// su `code`, como los manda el servicio); los demás pasan tal cual.
func errorDe(err error) error {
	var codigo string
	var estado int
	switch {
	case err == nil:
		return nil
	case errors.Is(err, dominio.ErrReferenciado):
		estado, codigo = http.StatusConflict, "referenced"
	case errors.Is(err, dominio.ErrLimite):
		estado, codigo = http.StatusConflict, "limit_reached"
	case errors.Is(err, dominio.ErrConflicto):
		estado, codigo = http.StatusConflict, "rev_conflict"
	case errors.Is(err, dominio.ErrNoExiste):
		estado, codigo = http.StatusNotFound, "not_found"
	case errors.Is(err, dominio.ErrInvalido):
		estado, codigo = http.StatusUnprocessableEntity, "invalid"
	default:
		return err
	}
	cuerpo, _ := json.Marshal(map[string]any{"error": map[string]string{"code": codigo, "message": err.Error()}})
	return &api.Error{Status: estado, URL: "local:bases", Body: string(cuerpo)}
}

func campoDe(f api.BasesField) dominio.Campo {
	return dominio.Campo{
		ID: f.ID, TableID: f.TableID, Name: f.Name, Kind: dominio.Tipo(f.Kind), Choices: slices.Clone(f.Choices),
		LinkTableID: f.LinkTableID, Formula: f.Formula, Result: dominio.Tipo(f.Result), Currency: f.Currency, Position: f.Position,
	}
}

func camposDe(fs []api.BasesField) []dominio.Campo {
	out := make([]dominio.Campo, len(fs))
	for i, f := range fs {
		out[i] = campoDe(f)
	}
	return out
}

// crudos pasa los valores del cable a JSON; nil es «vacío».
func crudos(datos map[string]any) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(datos))
	for k, v := range datos {
		if v == nil {
			out[k] = nil
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, dominio.ErrInvalido
		}
		out[k] = raw
	}
	return out, nil
}

// valores es lo contrario de crudos.
func valores(datos map[string]json.RawMessage) map[string]any {
	out := make(map[string]any, len(datos))
	for k, raw := range datos {
		if len(raw) == 0 {
			continue
		}
		var v any
		if json.Unmarshal(raw, &v) == nil && v != nil {
			out[k] = v
		}
	}
	return out
}

func registroDe(r api.BasesRecord) dominio.Registro {
	datos, _ := crudos(r.Data)
	d := dominio.Registro{
		ID: r.ID, TableID: r.TableID, Data: datos, Links: r.Links, Rev: int64(r.Rev),
		CreatedBy: r.CreatedBy, UpdatedBy: r.UpdatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if len(r.Attachments) > 0 {
		d.Attachments = map[string][]dominio.Adjunto{}
		for campo, as := range r.Attachments {
			for _, a := range as {
				d.Attachments[campo] = append(d.Attachments[campo], dominio.Adjunto{
					ID: a.ID, RecordID: a.RecordID, FieldID: a.FieldID, Name: a.Name, Mime: a.Mime,
					Size: a.Size, HasThumb: a.HasThumb, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
				})
			}
		}
	}
	return d
}

// configDe pasa la configuración de una vista del cable al dominio.
func configDe(c api.BasesViewConfig) (dominio.ConfigVista, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return dominio.ConfigVista{}, dominio.ErrInvalido
	}
	var out dominio.ConfigVista
	if json.Unmarshal(raw, &out) != nil {
		return dominio.ConfigVista{}, dominio.ErrInvalido
	}
	return out, nil
}

// configAPI es lo contrario de configDe.
func configAPI(c dominio.ConfigVista) api.BasesViewConfig {
	raw, _ := json.Marshal(c)
	var out api.BasesViewConfig
	_ = json.Unmarshal(raw, &out)
	return out
}

// ordenDe lee el orden de un momento: `campo:asc|desc` separados por comas.
func ordenDe(s string) ([]dominio.Orden, error) {
	var out []dominio.Orden
	for _, parte := range strings.Split(s, ",") {
		if parte = strings.TrimSpace(parte); parte == "" {
			continue
		}
		campo, dir, _ := strings.Cut(parte, ":")
		if dir == "" {
			dir = "asc"
		}
		if dir != "asc" && dir != "desc" {
			return nil, dominio.ErrInvalido
		}
		out = append(out, dominio.Orden{Field: campo, Dir: dir})
	}
	if len(out) > dominio.MaxOrden {
		return nil, dominio.ErrInvalido
	}
	return out, nil
}
