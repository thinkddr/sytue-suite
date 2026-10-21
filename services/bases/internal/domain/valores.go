// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// EntradaCampo es lo que envía un cliente para crear un campo.
type EntradaCampo struct {
	Name        string   `json:"name"`
	Kind        Tipo     `json:"kind"`
	Choices     []string `json:"choices"`
	LinkTableID string   `json:"link_table_id"`
	Formula     string   `json:"formula"`
	Result      Tipo     `json:"result"`
}

// ValidarEntradaCampo limpia y valida la definición de un campo. El destino de un
// enlace (que exista y sea de la misma base) lo comprueba el store dentro de la
// transacción; aquí solo que sea un UUID.
func ValidarEntradaCampo(in EntradaCampo) (EntradaCampo, error) {
	nombre, err := LimpiarNombre(in.Name)
	if err != nil || !in.Kind.Valido() {
		return in, ErrInvalido
	}
	in.Name = nombre
	switch in.Kind {
	case TipoSeleccion, TipoMultiple:
		if len(in.Choices) == 0 || len(in.Choices) > MaxOpciones {
			return in, ErrInvalido
		}
		vistas := make(map[string]struct{}, len(in.Choices))
		for i, c := range in.Choices {
			c2, err := limpiarOpcion(c)
			if err != nil {
				return in, err
			}
			if _, dup := vistas[c2]; dup {
				return in, ErrInvalido
			}
			vistas[c2] = struct{}{}
			in.Choices[i] = c2
		}
		in.LinkTableID = ""
	case TipoRelacion:
		if _, err := uuid.Parse(in.LinkTableID); err != nil {
			return in, ErrInvalido
		}
		in.Choices = nil
	case TipoFormula:
		if _, err := AnalizarFormula(in.Formula); err != nil {
			return in, err
		}
		if in.Result == "" {
			in.Result = TipoNumero
		}
		if in.Result != TipoNumero && in.Result != TipoTexto {
			return in, ErrInvalido
		}
		in.Formula = strings.TrimPrefix(strings.TrimSpace(in.Formula), "=")
		in.Choices, in.LinkTableID = nil, ""
	default:
		in.Choices, in.LinkTableID = nil, ""
	}
	if in.Kind != TipoFormula {
		in.Formula, in.Result = "", ""
	}
	return in, nil
}

func limpiarOpcion(s string) (string, error) {
	n, err := LimpiarNombre(s)
	if err != nil || utf8.RuneCountInString(n) > MaxLongOpcion {
		return "", ErrInvalido
	}
	return n, nil
}

// ValidarValor comprueba un valor contra el tipo del campo y devuelve su forma
// canónica. null (o ausencia) es «vacío» y siempre vale. Los enlaces NO pasan por
// aquí (ver ValidarEnlaces).
//
//nolint:gocognit,gocyclo // una regla por tipo de campo; separarlas esconde que el conjunto es cerrado.
func ValidarValor(c Campo, raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	malo := fmt.Errorf("%w: campo %q", ErrInvalido, c.Name)
	switch c.Kind { //nolint:exhaustive // los enlaces no son un valor: caen a «no válido»
	case TipoTexto:
		var s string
		if json.Unmarshal(raw, &s) != nil || utf8.RuneCountInString(s) > MaxLongTexto || !utf8.ValidString(s) || contieneNUL(s) {
			return nil, malo
		}
		if s == "" {
			return nil, nil
		}
		return json.Marshal(s)
	case TipoNumero:
		var f float64
		if json.Unmarshal(raw, &f) != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 1e15 {
			return nil, malo
		}
		return json.Marshal(f)
	case TipoFecha:
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, malo
		}
		t, err := time.Parse(time.DateOnly, s)
		if err != nil || t.Year() < 1 || t.Year() > 9999 {
			return nil, malo
		}
		return json.Marshal(t.Format(time.DateOnly))
	case TipoCasilla:
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return nil, malo
		}
		return json.Marshal(b)
	case TipoSeleccion:
		var s string
		if json.Unmarshal(raw, &s) != nil || !slices.Contains(c.Choices, s) {
			return nil, malo
		}
		return json.Marshal(s)
	case TipoMultiple:
		var l []string
		if json.Unmarshal(raw, &l) != nil || len(l) > MaxOpciones {
			return nil, malo
		}
		if len(l) == 0 {
			return nil, nil
		}
		slices.Sort(l)
		l = slices.Compact(l)
		for _, s := range l {
			if !slices.Contains(c.Choices, s) {
				return nil, malo
			}
		}
		return json.Marshal(l)
	}
	return nil, malo
}

func contieneNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}

// ValidarDatos valida el mapa {campo: valor} de un registro NUEVO contra los campos de
// su tabla; los vacíos se omiten del resultado.
func ValidarDatos(campos []Campo, entrada map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	parcial, err := ValidarParcial(campos, entrada)
	if err != nil {
		return nil, err
	}
	for k, v := range parcial {
		if v == nil {
			delete(parcial, k)
		}
	}
	return parcial, nil
}

// ValidarParcial valida un cambio: un id de campo desconocido, o de un enlace, se
// rechaza (no se ignora: un cliente que escribe donde no hay campo tiene un error
// que conviene ver). Un valor vacío queda como nil en el resultado, que significa
// «borra este campo».
func ValidarParcial(campos []Campo, entrada map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	porID := make(map[string]Campo, len(campos))
	for _, c := range campos {
		porID[c.ID] = c
	}
	salida := make(map[string]json.RawMessage, len(entrada))
	for id, raw := range entrada {
		c, ok := porID[id]
		// Un campo de fórmula lo calcula el servidor: quien intenta escribirlo recibe un
		// error, no se le ignora (si no, un cliente fijaría el «resultado» a mano).
		if !ok || c.Kind == TipoRelacion || c.Kind == TipoFormula || c.Kind == TipoAdjunto {
			return nil, fmt.Errorf("%w: campo desconocido", ErrInvalido)
		}
		v, err := ValidarValor(c, raw)
		if err != nil {
			return nil, err
		}
		salida[id] = v
	}
	return salida, nil
}

// TamanoDatos serializa data y comprueba que cabe en un registro.
func TamanoDatos(data map[string]json.RawMessage) error {
	n, _ := json.Marshal(data)
	if len(n) > MaxTamRegistro {
		return fmt.Errorf("%w: registro demasiado grande", ErrInvalido)
	}
	return nil
}

// ValidarEnlaces normaliza una lista de ids de registro: UUID válidos, sin
// repetidos, como máximo MaxEnlaces.
func ValidarEnlaces(ids []string) ([]string, error) {
	if len(ids) > MaxEnlaces {
		return nil, fmt.Errorf("%w: demasiados enlaces", ErrInvalido)
	}
	out := make([]string, 0, len(ids))
	for _, s := range ids {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("%w: enlace no válido", ErrInvalido)
		}
		out = append(out, u.String())
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
