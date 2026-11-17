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
	Currency    string   `json:"currency"`
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
		if in.Choices, err = ValidarOpciones(in.Choices); err != nil {
			return in, err
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
	case TipoMoneda:
		in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
		if in.Currency == "" {
			in.Currency = MonedaPorDefecto
		}
		if !codigoMoneda(in.Currency) {
			return in, ErrInvalido
		}
		in.Choices, in.LinkTableID = nil, ""
	default:
		in.Choices, in.LinkTableID = nil, ""
	}
	if in.Kind != TipoMoneda {
		in.Currency = ""
	}
	if in.Kind != TipoFormula {
		in.Formula, in.Result = "", ""
	}
	return in, nil
}

// ValidarOpciones limpia y valida la lista de opciones de un campo de selección:
// entre 1 y MaxOpciones, sin repetidas (tras limpiarlas) y en el orden recibido.
func ValidarOpciones(opciones []string) ([]string, error) {
	if len(opciones) == 0 || len(opciones) > MaxOpciones {
		return nil, ErrInvalido
	}
	out := make([]string, 0, len(opciones))
	vistas := make(map[string]struct{}, len(opciones))
	for _, o := range opciones {
		o2, err := limpiarOpcion(o)
		if err != nil {
			return nil, err
		}
		if _, dup := vistas[o2]; dup {
			return nil, ErrInvalido
		}
		vistas[o2] = struct{}{}
		out = append(out, o2)
	}
	return out, nil
}

// ConRetiradas devuelve los campos con las opciones que el registro YA tiene aunque
// se hayan retirado del campo. Retirar una opción no borra los valores que la usan:
// quien escribe el mismo valor (o, en una múltiple, añade otra opción a las que había)
// no debe recibir un error por una opción que no ha tocado. Una opción retirada que no
// estaba en el registro sigue sin valer.
func ConRetiradas(campos []Campo, actual []byte) []Campo {
	var datos map[string]json.RawMessage
	if json.Unmarshal(actual, &datos) != nil || len(datos) == 0 {
		return campos
	}
	out := slices.Clone(campos)
	for i, c := range out {
		raw, ok := datos[c.ID]
		if !ok {
			continue
		}
		var tiene []string
		switch c.Kind { //nolint:exhaustive // solo las selecciones tienen opciones
		case TipoSeleccion:
			var s string
			if json.Unmarshal(raw, &s) == nil {
				tiene = []string{s}
			}
		case TipoMultiple:
			_ = json.Unmarshal(raw, &tiene)
		}
		for _, s := range tiene {
			if !slices.Contains(c.Choices, s) {
				out[i].Choices = append(slices.Clone(out[i].Choices), s)
			}
		}
	}
	return out
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
	case TipoNumero, TipoMoneda:
		var f float64
		if json.Unmarshal(raw, &f) != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 1e15 {
			return nil, malo
		}
		if c.Kind == TipoMoneda {
			// Céntimos: un importe con más decimales es un error de redondeo de quien
			// escribe, no un dato; guardarlo tal cual haría que dos «iguales» no lo fueran.
			f = math.Round(f*100) / 100
		}
		return json.Marshal(f)
	case TipoValoracion:
		var f float64
		if json.Unmarshal(raw, &f) != nil || f != math.Trunc(f) || f < 0 || f > MaxValoracion {
			return nil, malo
		}
		if f == 0 { // cero estrellas es «sin valorar»
			return nil, nil
		}
		return json.Marshal(int(f))
	case TipoURL, TipoCorreo, TipoTelefono:
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, malo
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v, ok := normalizarFormato(c.Kind, s)
		if !ok {
			return nil, malo
		}
		return json.Marshal(v)
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
