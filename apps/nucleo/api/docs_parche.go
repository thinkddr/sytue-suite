// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
)

// Guardado por parches de hojas y presentaciones: en vez del modelo entero se manda lo que
// cambió (una celda, una caja) como op `patch`, que el servicio aplica sobre lo último que
// tiene, en orden de llegada y sin 409 (services/docs/internal/domain/parche.go). Es el
// mismo `diferencias` de editor/state/fusion.ts.

// Tramo de la ruta de un cambio: clave de objeto (string) o elemento de lista por id.
type parcheTramo = any

// Cambio es domain.Cambio.
type Cambio struct {
	Path  []parcheTramo `json:"path"`
	Value any           `json:"value,omitempty"`
	Del   bool          `json:"del,omitempty"`
}

const (
	parcheMaxCambios = 2000 // los topes del servicio (domain.parcheMaxCambios, parcheMaxRuta, maxClaveLen)
	parcheMaxRuta    = 10
	parcheMaxClave   = 256
)

var errNoExpresable = errors.New("api: el cambio no se deja decir como parche")

func parcheDecodificar(s string) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader([]byte(s)))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil || m == nil {
		return nil, errNoExpresable
	}
	return m, nil
}

// Diferencias son los cambios que llevan `base` hasta `mio` (dos modelos JSON), o
// errNoExpresable si no se dejan decir así —listas sin id, elementos nuevos que no van al
// final, listas reordenadas, demasiados cambios— y hay que mandar el registro entero.
func Diferencias(base, mio string) ([]Cambio, error) {
	b, err := parcheDecodificar(base)
	if err != nil {
		return nil, err
	}
	m, err := parcheDecodificar(mio)
	if err != nil {
		return nil, err
	}
	var out []Cambio
	if err := difObjetos(nil, b, m, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func ponerCambio(out *[]Cambio, c Cambio) error {
	if len(c.Path) > parcheMaxRuta || len(*out) >= parcheMaxCambios {
		return errNoExpresable
	}
	*out = append(*out, c)
	return nil
}

func rutaMas(r []parcheTramo, t parcheTramo) []parcheTramo {
	return append(append([]parcheTramo(nil), r...), t)
}

func difObjetos(r []parcheTramo, base, mio map[string]any, out *[]Cambio) error {
	claves := map[string]struct{}{}
	for k := range base {
		claves[k] = struct{}{}
	}
	for k := range mio {
		claves[k] = struct{}{}
	}
	for k := range claves {
		if k == "" || len(k) > parcheMaxClave {
			return errNoExpresable
		}
		vb, okb := base[k]
		vm, okm := mio[k]
		if err := difValor(rutaMas(r, k), vb, okb, vm, okm, out); err != nil {
			return err
		}
	}
	return nil
}

func difValor(r []parcheTramo, base any, okb bool, mio any, okm bool, out *[]Cambio) error {
	if okb == okm && reflect.DeepEqual(base, mio) {
		return nil
	}
	if !okm {
		return ponerCambio(out, Cambio{Path: r, Del: true})
	}
	if ob, ok1 := base.(map[string]any); ok1 && okb {
		if om, ok2 := mio.(map[string]any); ok2 {
			return difObjetos(r, ob, om, out)
		}
	}
	if lb, ok1 := base.([]any); ok1 && okb {
		if lm, ok2 := mio.([]any); ok2 {
			kb, km := parcheClavesPor(lb, "id"), parcheClavesPor(lm, "id")
			if kb != nil && km != nil {
				return difLista(r, lb, lm, kb, km, out)
			}
			if parcheClavesPor(lb, "name") != nil && parcheClavesPor(lm, "name") != nil {
				return errNoExpresable // la fusión la trata por nombre
			}
		}
	}
	return ponerCambio(out, Cambio{Path: r, Value: mio})
}

// parcheClavesPor son los `campo` de una lista, o nil si algún elemento no tiene uno único.
func parcheClavesPor(lista []any, campo string) []string {
	out := make([]string, 0, len(lista))
	vistos := map[string]struct{}{}
	for _, e := range lista {
		m, ok := e.(map[string]any)
		if !ok {
			return nil
		}
		k, ok := m[campo].(string)
		if _, dup := vistos[k]; !ok || dup {
			return nil
		}
		vistos[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

func difLista(r []parcheTramo, base, mio []any, kb, km []string, out *[]Cambio) error {
	enM := map[string]int{}
	for i, k := range km {
		enM[k] = i
	}
	enB := map[string]int{}
	for i, k := range kb {
		enB[k] = i
	}
	// El servicio añade al final: el orden de mío tiene que ser el de base sin lo quitado,
	// y lo nuevo detrás.
	var quedan []string
	for _, k := range kb {
		if _, ok := enM[k]; ok {
			quedan = append(quedan, k)
		}
	}
	if len(km) < len(quedan) || !reflect.DeepEqual(km[:len(quedan)], quedan) {
		return errNoExpresable
	}
	for _, k := range kb {
		if _, ok := enM[k]; !ok {
			if err := ponerCambio(out, Cambio{Path: rutaMas(r, map[string]string{"id": k}), Del: true}); err != nil {
				return err
			}
		}
	}
	for i, k := range km {
		tr := rutaMas(r, map[string]string{"id": k})
		j, estaba := enB[k]
		if !estaba {
			if err := ponerCambio(out, Cambio{Path: tr, Value: mio[i]}); err != nil {
				return err
			}
			continue
		}
		bm, ok1 := base[j].(map[string]any)
		mm, ok2 := mio[i].(map[string]any)
		switch {
		case ok1 && ok2:
			if err := difObjetos(tr, bm, mm, out); err != nil {
				return err
			}
		case !reflect.DeepEqual(base[j], mio[i]):
			return errNoExpresable
		}
	}
	return nil
}

// parcheGuardado es lo que contesta el servicio a un parche: el registro que resultó de
// ponerlo sobre lo último que había (lo de los demás incluido).
type parcheGuardado struct {
	Valor   string
	Dot     DocDot
	Lamport uint64
	Seq     int64
	// SinEfecto: el servicio no materializó nada (ya estaba aplicado, o no cambiaba nada).
	SinEfecto bool
}

// mandarParche envía los cambios de `base` a `valor` sobre `clave` y devuelve lo que
// resultó. errNoExpresable (o un 400 del servicio, que no entiende parches) piden al
// llamante caer al guardado del registro entero.
func mandarParche(ctx context.Context, c *Client, id, clave, base, valor, opID string, baseSeq int64) (parcheGuardado, error) {
	cambios, err := Diferencias(base, valor)
	if err != nil {
		return parcheGuardado{}, err
	}
	if len(cambios) == 0 {
		return parcheGuardado{SinEfecto: true, Valor: valor, Seq: baseSeq}, nil
	}
	var cuerpo bytes.Buffer
	enc := json.NewEncoder(&cuerpo)
	enc.SetEscapeHTML(false) // cada «<» del texto de una caja serían seis bytes contra el tope del cuerpo
	if err := enc.Encode(map[string]any{"ops": []any{map[string]any{
		"kind": "patch", "key": clave, "op_id": opID, "base": baseSeq, "cambios": cambios,
	}}}); err != nil {
		return parcheGuardado{}, err
	}
	r, err := JSON[struct {
		Seq int64 `json:"seq"`
		Ops []struct {
			Op struct {
				OpID    string          `json:"op_id"`
				Value   json.RawMessage `json:"value"`
				Dot     DocDot          `json:"dot"`
				Lamport uint64          `json:"lamport"`
			} `json:"op"`
		} `json:"ops"`
	}](ctx, c, Request{
		Method: http.MethodPost, Path: "/documents/" + url.PathEscape(id) + "/ops",
		Header: http.Header{"Content-Type": {"application/json"}}, Body: cuerpo.Bytes(),
	})
	if err != nil {
		return parcheGuardado{}, err
	}
	for _, o := range r.Ops {
		var v string
		if o.Op.OpID == opID && json.Unmarshal(o.Op.Value, &v) == nil {
			return parcheGuardado{Valor: v, Dot: o.Op.Dot, Lamport: o.Op.Lamport, Seq: r.Seq}, nil
		}
	}
	return parcheGuardado{SinEfecto: true, Valor: valor, Seq: r.Seq}, nil
}

// sinParches dice si el error pide caer al registro entero.
func sinParches(err error) bool {
	var e *Error
	return errors.Is(err, errNoExpresable) || (errors.As(err, &e) && e.Status == http.StatusBadRequest)
}
