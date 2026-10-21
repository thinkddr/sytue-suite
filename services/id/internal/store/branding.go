// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// El logotipo propio de una organización (ver migración 0051).
//
// Aislamiento por columna, como el resto del servicio: las escrituras reciben el
// `org_id` de la organización activa del token —resuelto en el handler, NUNCA
// tomado de la ruta— y la lectura pública va por `slug`, que es lo que viaja en la
// URL. No hay forma de escribir en el branding de otro tenant sin ser su
// administrador, y la lectura es pública a propósito: es el logo que sale pintado
// en la barra de todos sus empleados.

// LogoPorSlug devuelve el PNG ya reencuadrado y su hash. Sin fila (o sin
// organización) devuelve nil y cadena vacía SIN error: no tener logo propio es el
// caso normal, no un fallo, y quien llama pinta entonces la marca de la casa.
func (s *Store) LogoPorSlug(ctx context.Context, q pg.Querier, slug string) ([]byte, string, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `SELECT b.logo_png, b.logo_hash
		FROM org_branding b
		JOIN organizations o ON o.id = b.org_id
		WHERE o.slug = $1 AND o.status = 'active'`
	var png []byte
	var hash string
	switch err := q.QueryRow(ctx, sql, slug).Scan(&png, &hash); {
	case err == nil:
		return png, hash, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, "", nil
	default:
		return nil, "", errf("LogoPorSlug", err)
	}
}

// HashDeLogo devuelve solo el hash, para el claim del token. Se llama en CADA
// emisión, así que no arrastra el binario: son treinta kilobytes que no hacen
// ninguna falta para componer una URL.
func (s *Store) HashDeLogo(ctx context.Context, q pg.Querier, orgID string) (string, error) {
	// `q` nulo = sin transacción, como en el resto del store. Faltaba, y el
	// emisor de tokens llama justo así (`HashDeLogo(ctx, nil, orgID)`): la
	// llamada entraba con una interfaz nil y reventaba dentro. El pánico lo
	// recogía el middleware y salía un 500 sin nada en el log, así que TODAS las
	// emisiones de token —código, DCR, exchange— devolvían «Internal server
	// error» sin decir por qué.
	if q == nil {
		q = s.DB()
	}
	const sql = `SELECT logo_hash FROM org_branding WHERE org_id = $1`
	var hash string
	switch err := q.QueryRow(ctx, sql, orgID).Scan(&hash); {
	case err == nil:
		return hash, nil
	case errors.Is(err, pgx.ErrNoRows):
		return "", nil
	default:
		return "", errf("HashDeLogo", err)
	}
}

// GuardarLogo fija el logo de la organización. Es un UPSERT: cambiar de logo es la
// operación normal, no un caso excepcional.
func (s *Store) GuardarLogo(ctx context.Context, q pg.Querier, orgID string, png []byte, hash string) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO org_branding (org_id, logo_png, logo_hash, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (org_id) DO UPDATE
		SET logo_png = EXCLUDED.logo_png, logo_hash = EXCLUDED.logo_hash, updated_at = now()`
	_, err := q.Exec(ctx, sql, orgID, png, hash)
	return errf("GuardarLogo", err)
}

// BorrarLogo devuelve la organización a la marca de la casa. Borrar lo que no
// existe no es un error: el resultado es el que se pedía.
func (s *Store) BorrarLogo(ctx context.Context, q pg.Querier, orgID string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `DELETE FROM org_branding WHERE org_id = $1`, orgID)
	return errf("BorrarLogo", err)
}

// consultaDeApariencia es el UPDATE de cada valor por defecto de apariencia de la
// organización. Una consulta escrita por columna, y no la columna concatenada: el
// nombre llega de quien llama.
var consultaDeApariencia = map[string]string{
	"color": `UPDATE organizations SET color = $2, updated_at = now() WHERE id = $1`,
	"forma": `UPDATE organizations SET forma = $2, updated_at = now() WHERE id = $1`,
}

// GuardarApariencia fija un valor por defecto de la organización: su color o su
// forma. Quien llama ya validó el valor contra el dominio (domain.OrgColors,
// domain.OrgFormas); aquí solo se escribe.
func (s *Store) GuardarApariencia(ctx context.Context, q pg.Querier, orgID, eje, valor string) error {
	consulta, ok := consultaDeApariencia[eje]
	if !ok {
		return errf("GuardarApariencia", errors.New("eje de apariencia desconocido: "+eje))
	}
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, consulta, orgID, valor)
	return errf("GuardarApariencia", err)
}
