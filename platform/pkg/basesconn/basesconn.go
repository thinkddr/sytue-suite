// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package basesconn es el CLIENTE del conector de solo lectura de Sytue Bases (ADR 0068):
// lo importan los consumidores (Sytue Drive, SyERP, Sytue Web) en vez de copiar cada uno su
// petición, su tope de bytes y su lectura de la procedencia.
//
// La autorización es un TOKEN DE CONEXIÓN (`sbc_…`) que concedió el propietario de la base,
// acotado a una vista y a un consumidor. Se manda en `Authorization: Bearer` a una raíz FIJA
// (config de servicio del consumidor, nunca del usuario). Sytue Bases lo comprueba en CADA
// lectura: aquí no se cachea nada, así que revocar corta en el acto.
//
// ponytail: Sytue Docs (Calcus) conserva su copia anterior en
// services/docs/internal/connectors/bases.go, atada a su `Result` y a sus límites; pasarla
// aquí es quitar esa función y adaptar su test.
package basesconn

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Consumidores que este cliente conoce (los que exigen vista y organización).
const (
	Drive = "drive"
	ERP   = "erp"
	Web   = "web"
)

var (
	// ErrToken: lo que se da no tiene forma de token de conexión. No sale nada hacia Bases.
	ErrToken = errors.New("basesconn: no es un token de conexión de Sytue Bases")
	// ErrRevocada: Bases contesta 401. Revocada, caducada, de otro consumidor o de otra
	// organización: Bases no distingue y este cliente tampoco.
	ErrRevocada = errors.New("basesconn: la conexión se ha revocado, ha caducado o no es para este producto")
	// ErrNoConfigurado: el despliegue no tiene la raíz de Sytue Bases.
	ErrNoConfigurado = errors.New("basesconn: Sytue Bases no está configurado en este despliegue")
	// ErrRemoto: Bases no contesta o contesta algo que no es una tabla.
	ErrRemoto = errors.New("basesconn: Sytue Bases no ha contestado una tabla")
)

// formaToken es la de store.PrefijoTokenConector + 32 bytes en base64url.
var formaToken = regexp.MustCompile(`^sbc_[A-Za-z0-9_-]{43}$`)

// TokenValido dice si `t` tiene forma de token de conexión.
func TokenValido(t string) bool { return formaToken.MatchString(t) }

// Columna de una lectura. Type: text | number | boolean.
type Columna struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Procedencia dice de dónde salen los datos: lo que el consumidor guarda y enseña.
type Procedencia struct {
	Source       string    `json:"source"`
	Consumer     string    `json:"consumer"`
	ConnectionID string    `json:"connection_id"`
	Version      int       `json:"version"`
	BaseID       string    `json:"base_id"`
	BaseName     string    `json:"base_name"`
	TableID      string    `json:"table_id"`
	TableName    string    `json:"table_name"`
	ViewID       string    `json:"view_id,omitempty"`
	ViewName     string    `json:"view_name,omitempty"`
	GrantedBy    string    `json:"granted_by"`
	GrantedAt    time.Time `json:"granted_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	ReadAt       time.Time `json:"read_at"`
}

// Lectura es lo que devuelve el conector.
type Lectura struct {
	Columns    []Columna   `json:"columns"`
	Rows       [][]any     `json:"rows"`
	Provenance Procedencia `json:"provenance"`
}

// Peticion es una lectura: a qué raíz, con qué token, quién lee y para qué organización.
type Peticion struct {
	Raiz       string // config de SERVICIO (p. ej. http://bases:8102), nunca del usuario
	Token      string
	Consumidor string // Drive, ERP o Web
	Tenant     string // la organización de quien pide: Bases exige que sea la de la base
	MaxBytes   int64  // tope del cuerpo de la respuesta
	MaxFilas   int    // por encima, falla (no recorta en silencio)
	MaxCols    int
}

// Leer hace la lectura. Un cliente sin `Timeout` es responsabilidad de quien llama: pásese
// uno con plazo.
func Leer(ctx context.Context, cliente *http.Client, p Peticion) (*Lectura, error) {
	if strings.TrimSpace(p.Raiz) == "" {
		return nil, ErrNoConfigurado
	}
	if !TokenValido(p.Token) {
		return nil, ErrToken
	}
	if p.Consumidor == "" || p.Tenant == "" {
		return nil, fmt.Errorf("%w: falta el consumidor o la organización", ErrToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.Raiz, "/")+"/_bases/v1/connector/read", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoConfigurado, err)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Sytue-Consumer", p.Consumidor)
	req.Header.Set("X-Sytue-Tenant", p.Tenant)
	req.Header.Set("User-Agent", "Sytue-"+p.Consumidor+"-Connector/1")
	resp, err := cliente.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRemoto, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrRevocada
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrRemoto, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, p.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRemoto, err)
	}
	if int64(len(raw)) > p.MaxBytes {
		return nil, fmt.Errorf("%w: respuesta de más de %d bytes", ErrRemoto, p.MaxBytes)
	}
	var l Lectura
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRemoto, err)
	}
	if len(l.Columns) > p.MaxCols || len(l.Rows) > p.MaxFilas {
		return nil, fmt.Errorf("%w: la vista tiene %d columnas y %d filas (tope %d y %d)", ErrRemoto, len(l.Columns), len(l.Rows), p.MaxCols, p.MaxFilas)
	}
	// La procedencia ha de decir que es para ESTE consumidor: una respuesta que no lo diga
	// no viene del conector que se espera.
	if l.Provenance.Source != "sytue-bases" || l.Provenance.Consumer != p.Consumidor || l.Provenance.ConnectionID == "" {
		return nil, fmt.Errorf("%w: procedencia inesperada", ErrRemoto)
	}
	for _, f := range l.Rows {
		if len(f) != len(l.Columns) {
			return nil, fmt.Errorf("%w: fila desalineada", ErrRemoto)
		}
	}
	return &l, nil
}

// Texto es una celda como texto: lo que se pinta en un informe, un CSV o una página.
// Los números no llevan notación científica ni ceros de más.
func Texto(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// CSV escribe la lectura como CSV (UTF-8, cabecera con los nombres de columna).
//
// Defensa de la «inyección de fórmulas», como el exportador de Bases
// (services/bases/internal/domain/csv.go): a un TEXTO que empieza por `= + - @`, TAB o CR se
// le antepone `'`, para que una hoja de cálculo no lo ejecute al abrir el fichero. Los
// números no se tocan: un importe negativo sigue siendo un número.
func CSV(l *Lectura) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	fila := make([]string, len(l.Columns))
	for i, c := range l.Columns {
		fila[i] = textoSeguro(c.Name)
	}
	if err := w.Write(fila); err != nil {
		return nil, err
	}
	for _, r := range l.Rows {
		for i, v := range r {
			if s, ok := v.(string); ok {
				fila[i] = textoSeguro(s)
			} else {
				fila[i] = Texto(v)
			}
		}
		if err := w.Write(fila); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func textoSeguro(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
