// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"
)

// leerTodos decodifica las filas vivas de un tipo.
func leerTodos[T any](ctx context.Context, r *Repository, kind string) ([]T, error) {
	raws, err := r.readByKind(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raws))
	for _, raw := range raws {
		var v T
		err := json.Unmarshal(raw, &v)
		clear(raw)
		if err != nil {
			return nil, ErrBases
		}
		out = append(out, v)
	}
	return out, nil
}

// leerUno decodifica una fila viva.
func leerUno[T any](ctx context.Context, r *Repository, kind, id string) (T, error) {
	var v T
	raw, err := r.core.ReadRaw(ctx, kind, id)
	if err != nil {
		return v, errorDe(dominio.ErrNoExiste)
	}
	defer clear(raw)
	// Una fila en la papelera (trashRow) se sigue leyendo en el motor, pero ya no existe.
	var estado struct {
		DeletedAt *json.RawMessage `json:"deleted_at"`
	}
	if json.Unmarshal(raw, &estado) != nil || json.Unmarshal(raw, &v) != nil {
		return v, ErrBases
	}
	if estado.DeletedAt != nil && string(*estado.DeletedAt) != "null" {
		return v, errorDe(dominio.ErrNoExiste)
	}
	return v, nil
}

// # Repetir una operación
//
// El motor reconoce una mutación por su operación y su entrada (`op` e `input`): si
// la misma quedó pendiente en su diario (se escribió, pero un corte se llevó la
// respuesta), repetirla la retoma de ahí —sin volver a preparar nada— y devuelve lo que
// ya escribió. Por eso la entrada es lo que pide quien llama, nunca lo que se inventa
// al escribir (el id de una fila nueva, la hora), y lo que se devuelve es siempre lo que
// devuelve el motor.

// cambiar lee una fila, la cambia y la escribe en UNA mutación del motor, y la devuelve
// como quedó escrita.
func cambiar[T any](ctx context.Context, r *Repository, op, kind, id string, input any, cambio func(*T) error) (T, error) {
	v, _, err := cambiarFila(ctx, r, op, kind, id, input, cambio)
	return v, err
}

// cambiarFila es cambiar y dice si era una operación repetida: la que un corte dejó
// pendiente, que el motor retoma sin volver a llamar a cambio.
func cambiarFila[T any](ctx context.Context, r *Repository, op, kind, id string, input any, cambio func(*T) error) (T, bool, error) {
	preparada := false
	raw, err := r.mutar(ctx, op, input, func(ctx context.Context) (string, string, string, []byte, error) {
		preparada = true
		existing, err := r.core.ReadRaw(ctx, kind, id)
		if err != nil {
			return "", "", "", nil, errorDe(dominio.ErrNoExiste)
		}
		defer clear(existing)
		var v T
		if json.Unmarshal(existing, &v) != nil {
			return "", "", "", nil, ErrBases
		}
		if err := cambio(&v); err != nil {
			return "", "", "", nil, err
		}
		val, err := json.Marshal(v)
		return kind, id, "live", val, err
	})
	v, err := escrita[T](raw, err)
	return v, err == nil && !preparada, err
}

// crear escribe una fila nueva y la devuelve como quedó escrita. input es la operación
// tal como la pide quien llama (sin id ni hora): repetirla tras un corte da la MISMA
// fila, con su id, en vez de crear otra. nueva hace la fila con un id nuevo; solo se
// llama si no hay una así pendiente, y en ella van las comprobaciones que dependen de
// lo que ya hay (los topes, la posición).
func crear[T any](ctx context.Context, r *Repository, op, kind string, input any, nueva func(context.Context) (string, T, error)) (T, error) {
	raw, err := r.mutar(ctx, op, input, prepararNueva(kind, nueva))
	return escrita[T](raw, err)
}

// prepararNueva es la preparación de una mutación que escribe la fila que da nueva.
func prepararNueva[T any](kind string, nueva func(context.Context) (string, T, error)) prepararFila {
	return func(ctx context.Context) (string, string, string, []byte, error) {
		id, v, err := nueva(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		val, err := json.Marshal(v)
		return kind, id, "live", val, err
	}
}

// escrita descodifica la fila que devuelve el motor tras una mutación.
func escrita[T any](raw []byte, err error) (T, error) {
	defer clear(raw)
	var v T
	if err != nil {
		return v, err
	}
	if json.Unmarshal(raw, &v) != nil {
		return v, ErrBases
	}
	return v, nil
}

// camposDeTabla son los campos vivos de una tabla, por su posición.
func (r *Repository) camposDeTabla(ctx context.Context, tabla string) ([]api.BasesField, error) {
	todos, err := leerTodos[api.BasesField](ctx, r, "field")
	if err != nil {
		return nil, err
	}
	var out []api.BasesField
	for _, f := range todos {
		if f.TableID == tabla {
			out = append(out, f)
		}
	}
	ordenarCampos(out)
	return out, nil
}

func ordenarCampos(fs []api.BasesField) {
	slices.SortStableFunc(fs, func(a, b api.BasesField) int {
		if a.Position != b.Position {
			return a.Position - b.Position
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// porTabla agrupa los campos vivos por tabla.
func (r *Repository) porTabla(ctx context.Context) (map[string][]dominio.Campo, error) {
	todos, err := leerTodos[api.BasesField](ctx, r, "field")
	if err != nil {
		return nil, err
	}
	ordenarCampos(todos)
	out := map[string][]dominio.Campo{}
	for _, f := range todos {
		out[f.TableID] = append(out[f.TableID], campoDe(f))
	}
	return out, nil
}

func (r *Repository) CreateTable(ctx context.Context, base, nombre string) (api.BasesTable, error) {
	nombre, err := dominio.LimpiarNombre(nombre)
	if err != nil {
		return api.BasesTable{}, errorDe(err)
	}
	if _, err := r.GetBase(ctx, base); err != nil {
		return api.BasesTable{}, errorDe(dominio.ErrNoExiste)
	}
	entrada := struct {
		Base   string `json:"base"`
		Nombre string `json:"name"`
	}{base, nombre}
	return crear(ctx, r, "table-create", "table", entrada, func(ctx context.Context) (string, api.BasesTable, error) {
		tablas, err := r.ListTables(ctx, base)
		if err != nil {
			return "", api.BasesTable{}, err
		}
		if len(tablas) >= dominio.MaxTablasPorBase {
			return "", api.BasesTable{}, errorDe(dominio.ErrLimite)
		}
		t := api.BasesTable{ID: uuid.NewString(), BaseID: base, Name: nombre, Role: api.BasesOwner, Fields: []api.BasesField{}}
		for _, x := range tablas {
			t.Position = max(t.Position, x.Position+1)
		}
		return t.ID, t, nil
	})
}

// comprobarCampo valida la definición de un campo con las reglas del servicio: el
// destino de un enlace, una tabla de la misma base; las referencias de una fórmula,
// campos de la tabla o de las enlazadas.
func (r *Repository) comprobarCampo(ctx context.Context, tabla api.BasesTable, propios []api.BasesField, f api.BasesField) error {
	if f.Kind == api.BasesKindLink {
		destino, err := leerUno[api.BasesTable](ctx, r, "table", f.LinkTableID)
		if err != nil || destino.BaseID != tabla.BaseID {
			return dominio.ErrInvalido
		}
	}
	if f.Kind != api.BasesKindFormula {
		return nil
	}
	formula, err := dominio.AnalizarFormula(f.Formula)
	if err != nil {
		return err
	}
	formulas := 0
	for _, p := range propios {
		if p.Kind == api.BasesKindFormula && p.ID != f.ID {
			formulas++
		}
	}
	if formulas >= dominio.MaxFormulasPorTabla {
		return dominio.ErrLimite
	}
	todos, err := r.porTabla(ctx)
	if err != nil {
		return err
	}
	return formula.Comprobar(camposDe(propios), func(t string) []dominio.Campo { return todos[t] })
}

func (r *Repository) CreateField(ctx context.Context, tabla string, campo api.BasesNewField) (api.BasesField, error) {
	t, err := leerUno[api.BasesTable](ctx, r, "table", tabla)
	if err != nil {
		return api.BasesField{}, err
	}
	in, err := dominio.ValidarEntradaCampo(dominio.EntradaCampo{
		Name: campo.Name, Kind: dominio.Tipo(campo.Kind), Choices: campo.Choices, LinkTableID: campo.LinkTableID,
		Formula: campo.Formula, Result: dominio.Tipo(campo.Result), Currency: campo.Currency,
	})
	if err != nil {
		return api.BasesField{}, errorDe(err)
	}
	entrada := struct {
		Tabla string               `json:"table"`
		Campo dominio.EntradaCampo `json:"field"`
	}{tabla, in}
	return crear(ctx, r, "field-create", "field", entrada, func(ctx context.Context) (string, api.BasesField, error) {
		propios, err := r.camposDeTabla(ctx, tabla)
		if err != nil {
			return "", api.BasesField{}, err
		}
		if len(propios) >= dominio.MaxCamposPorTabla {
			return "", api.BasesField{}, errorDe(dominio.ErrLimite)
		}
		f := api.BasesField{
			ID: uuid.NewString(), TableID: tabla, Name: in.Name, Kind: string(in.Kind), Choices: in.Choices,
			LinkTableID: in.LinkTableID, Formula: in.Formula, Result: string(in.Result), Currency: in.Currency,
		}
		for _, p := range propios {
			f.Position = max(f.Position, p.Position+1)
		}
		if err := r.comprobarCampo(ctx, t, propios, f); err != nil {
			return "", api.BasesField{}, errorDe(err)
		}
		return f.ID, f, nil
	})
}

// SetFieldFormula cambia la expresión de un campo de fórmula. Como los valores se
// calculan al leer, toda la tabla queda recalculada al momento.
func (r *Repository) SetFieldFormula(ctx context.Context, campo, expr string) error {
	f, err := leerUno[api.BasesField](ctx, r, "field", campo)
	if err != nil {
		return err
	}
	if f.Kind != api.BasesKindFormula {
		return errorDe(dominio.ErrInvalido)
	}
	t, err := leerUno[api.BasesTable](ctx, r, "table", f.TableID)
	if err != nil {
		return err
	}
	propios, err := r.camposDeTabla(ctx, f.TableID)
	if err != nil {
		return err
	}
	f.Formula = strings.TrimPrefix(strings.TrimSpace(expr), "=")
	if err := r.comprobarCampo(ctx, t, propios, f); err != nil {
		return errorDe(err)
	}
	_, err = cambiar(ctx, r, "field-formula", "field", campo, []string{campo, f.Formula}, func(x *api.BasesField) error {
		x.Formula = f.Formula
		return nil
	})
	return err
}

// SetFieldChoices sustituye las opciones de una selección. Lo que ya está escrito con
// una opción retirada se queda (dominio.ConRetiradas).
func (r *Repository) SetFieldChoices(ctx context.Context, campo string, opciones []string) error {
	limpias, err := dominio.ValidarOpciones(opciones)
	if err != nil {
		return errorDe(err)
	}
	_, err = cambiar(ctx, r, "field-choices", "field", campo, map[string]any{"field": campo, "choices": limpias}, func(f *api.BasesField) error {
		if f.Kind != api.BasesKindSelect && f.Kind != api.BasesKindMultiselect {
			return errorDe(dominio.ErrInvalido)
		}
		f.Choices = limpias
		return nil
	})
	return err
}

// ReorderFields fija el orden de TODOS los campos de la tabla; si no son exactamente
// los suyos, es un conflicto, como en el servicio.
func (r *Repository) ReorderFields(ctx context.Context, tabla string, campos []string) error {
	actuales, err := r.camposDeTabla(ctx, tabla)
	if err != nil {
		return err
	}
	ids := make([]string, len(actuales))
	for i, f := range actuales {
		ids[i] = f.ID
	}
	pedidos := slices.Clone(campos)
	slices.Sort(ids)
	slices.Sort(pedidos)
	if !slices.Equal(ids, slices.Compact(pedidos)) || len(pedidos) != len(campos) {
		return errorDe(dominio.ErrConflicto)
	}
	for pos, id := range campos {
		if _, err := cambiar(callar(ctx), r, "field-position", "field", id, []any{id, pos}, func(f *api.BasesField) error {
			f.Position = pos
			return nil
		}); err != nil {
			return err
		}
	}
	r.resumir(ctx, "field.reorder", tabla, "")
	return nil
}

func (r *Repository) validarVista(ctx context.Context, tabla, tipo string, cfg api.BasesViewConfig) (api.BasesViewConfig, error) {
	campos, err := r.camposDeTabla(ctx, tabla)
	if err != nil {
		return cfg, err
	}
	c, err := configDe(cfg)
	if err != nil {
		return cfg, errorDe(err)
	}
	c, err = dominio.ValidarConfig(dominio.TipoVista(tipo), c, camposDe(campos))
	if err != nil {
		return cfg, errorDe(err)
	}
	return configAPI(c), nil
}

func (r *Repository) CreateView(ctx context.Context, tabla, nombre, tipo string, cfg api.BasesViewConfig) (api.BasesView, error) {
	nombre, err := dominio.LimpiarNombre(nombre)
	if err != nil {
		return api.BasesView{}, errorDe(err)
	}
	if cfg, err = r.validarVista(ctx, tabla, tipo, cfg); err != nil {
		return api.BasesView{}, err
	}
	entrada := map[string]any{"table": tabla, "name": nombre, "kind": tipo, "config": cfg}
	return crear(ctx, r, "view-create", "view", entrada, func(context.Context) (string, api.BasesView, error) {
		v := api.BasesView{ID: uuid.NewString(), TableID: tabla, Name: nombre, Kind: tipo, Config: cfg, CreatedBy: r.scope.Subject}
		return v.ID, v, nil
	})
}

// UpdateView cambia el nombre y la configuración de una vista; el tipo no cambia.
func (r *Repository) UpdateView(ctx context.Context, vista, nombre string, cfg api.BasesViewConfig) (api.BasesView, error) {
	nombre, err := dominio.LimpiarNombre(nombre)
	if err != nil {
		return api.BasesView{}, errorDe(err)
	}
	actual, err := leerUno[api.BasesView](ctx, r, "view", vista)
	if err != nil {
		return api.BasesView{}, err
	}
	if cfg, err = r.validarVista(ctx, actual.TableID, actual.Kind, cfg); err != nil {
		return api.BasesView{}, err
	}
	return cambiar(ctx, r, "view-update", "view", vista, map[string]any{"view": vista, "name": nombre, "config": cfg}, func(v *api.BasesView) error {
		v.Name, v.Config = nombre, cfg
		return nil
	})
}

var (
	_ api.BasesSchemaData = (*Repository)(nil)
	_ api.BasesQueryData  = (*Repository)(nil)
)
