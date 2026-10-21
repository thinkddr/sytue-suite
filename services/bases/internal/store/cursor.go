// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Paginación por cursor (keyset). El cursor es OPACO para el cliente y dice dónde se
// quedó la página anterior en el orden de la vista: el valor de cada clave del orden en
// el último registro entregado, más su `created_at` y su id, que desempatan siempre.
//
// No va firmado y no hace falta: no concede nada. Quien lo falsifica solo elige desde
// dónde sigue leyendo una tabla que ya puede leer (la autorización es la de cada
// petición, no la del cursor), y sus valores viajan como PARÁMETROS, nunca en el SQL.
//
// Estable con el orden de la vista: lleva la firma de ese orden (`o`), y si la vista
// cambió de orden entre dos páginas el cursor se rechaza (422) en vez de saltarse o
// repetir registros en silencio; el cliente vuelve a empezar.
type cursorPagina struct {
	O string    `json:"o"`
	K []*string `json:"k"` // un valor por clave del orden; null = sin valor (van al final)
	T string    `json:"t"` // created_at del último registro, con microsegundos
	I string    `json:"i"` // su id
}

func (q *consulta) firmaOrden() string {
	var b strings.Builder
	for _, k := range q.claves {
		fmt.Fprintf(&b, "%s|%s|%t;", k.campo, k.tipo, k.desc)
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:8])
}

// cursorDe es el cursor que continúa después del registro r.
func (q *consulta) cursorDe(r domain.Registro) string {
	c := cursorPagina{O: q.firmaOrden(), K: make([]*string, len(q.claves)), T: r.CreatedAt.UTC().Format(time.RFC3339Nano), I: r.ID}
	for i, k := range q.claves {
		raw := r.Data[k.campo]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		v := string(raw) // número y casilla: su literal JSON es el de Postgres
		if k.tipo != domain.TipoNumero && k.tipo != domain.TipoCasilla {
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
			if rs := []rune(v); len(rs) > domain.MaxClaveOrden {
				v = string(rs[:domain.MaxClaveOrden])
			}
		}
		c.K[i] = &v
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// despuesDe añade a la consulta «lo que va después del cursor». Todo lo que trae el
// cursor se valida ANTES de llegar a Postgres (un `::numeric` sobre basura sería un 500)
// y entra como parámetro. Con cada clave: ASC/DESC … NULLS LAST, así que detrás de un
// valor van los mayores (o menores), luego los vacíos; y detrás de un vacío, solo vacíos.
func (q *consulta) despuesDe(cur string) error {
	malo := fmt.Errorf("%w: cursor", domain.ErrInvalido)
	if len(cur) > domain.MaxCursor {
		return malo
	}
	raw, err := base64.RawURLEncoding.DecodeString(cur)
	if err != nil {
		return malo
	}
	var c cursorPagina
	if json.Unmarshal(raw, &c) != nil || c.O != q.firmaOrden() || len(c.K) != len(q.claves) {
		return malo
	}
	if _, err := time.Parse(time.RFC3339Nano, c.T); err != nil {
		return malo
	}
	if _, err := uuid.Parse(c.I); err != nil {
		return malo
	}
	pred := "(r.created_at, r.id) > (" + q.add(c.T) + "::timestamptz, " + q.add(c.I) + "::uuid)"
	for i := len(q.claves) - 1; i >= 0; i-- {
		k := q.claves[i]
		if c.K[i] == nil {
			pred = "(" + k.expr + " IS NULL AND " + pred + ")"
			continue
		}
		v := *c.K[i]
		conv, ok := conversionDeClave(k.tipo, v)
		if !ok {
			return malo
		}
		p := q.add(v) + conv
		op := " > "
		if k.desc {
			op = " < "
		}
		pred = "(" + k.expr + op + p + " OR " + k.expr + " IS NULL OR (" + k.expr + " = " + p + " AND " + pred + "))"
	}
	q.where += " AND " + pred
	return nil
}

// conversionDeClave valida el valor que el cursor trae para una clave del orden y dice a
// qué tipo se convierte el parámetro ("" = texto).
func conversionDeClave(tipo domain.Tipo, v string) (string, bool) {
	switch tipo { //nolint:exhaustive // el resto se compara como texto
	case domain.TipoNumero:
		n, err := strconv.ParseFloat(v, 64)
		return "::numeric", err == nil && !math.IsInf(n, 0) && !math.IsNaN(n)
	case domain.TipoCasilla:
		return "::boolean", v == "true" || v == "false"
	}
	return "", utf8.ValidString(v) && !strings.ContainsRune(v, 0) && utf8.RuneCountInString(v) <= domain.MaxClaveOrden
}
