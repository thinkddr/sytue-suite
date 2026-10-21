// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Formularios públicos (piezas C del ADR 0068). El permiso aquí NO es un rol: es el
// TOKEN del enlace, que solo existe en manos de quien el propietario se lo dio y que se
// puede revocar al instante. Un token válido permite una sola cosa: añadir registros a
// la tabla del formulario con los campos de su vista, hasta el cupo del enlace.

// MaxEnlacesPorFormulario acota los enlaces vivos de una vista.
const MaxEnlacesPorFormulario = 10

// TokenFormulario es la forma de un token: 32 bytes en base64 sin relleno. Se comprueba
// ANTES de ir a la base: una cadena de otra forma no puede ser un token.
var tokenFormulario = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// EnlaceFormulario es un enlace público a un formulario. Nunca lleva el token.
type EnlaceFormulario struct {
	ID           string     `json:"id"`
	ViewID       string     `json:"view_id"`
	MaxResponses int        `json:"max_responses"`
	Responses    int        `json:"responses"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
}

// FormularioPublico es lo que ve quien abre el enlace: el nombre del formulario y los
// campos que puede rellenar. Nada de la base, la tabla, otras vistas ni otras personas.
type FormularioPublico struct {
	Name   string         `json:"name"`
	Fields []domain.Campo `json:"fields"`
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// camposDelFormulario son los campos que admite una vista de formulario: los de su
// configuración, en su orden, o todos los que no son enlaces. Un enlace entre tablas
// nunca se rellena desde fuera: apuntaría a registros que quien envía no puede ver.
func camposDelFormulario(todos []domain.Campo, cfg domain.ConfigVista) []domain.Campo {
	validos := make([]domain.Campo, 0, len(todos))
	for _, c := range todos {
		if c.Kind != domain.TipoRelacion && c.Kind != domain.TipoFormula && c.Kind != domain.TipoAdjunto {
			validos = append(validos, c)
		}
	}
	if len(cfg.Fields) == 0 {
		return validos
	}
	out := make([]domain.Campo, 0, len(cfg.Fields))
	for _, id := range cfg.Fields {
		if i := slices.IndexFunc(validos, func(c domain.Campo) bool { return c.ID == id }); i >= 0 {
			out = append(out, validos[i])
		}
	}
	return out
}

// CrearEnlaceFormulario crea un enlace público para una vista de tipo formulario
// (propietario) y devuelve el token, que no se podrá volver a leer.
func (s *Store) CrearEnlaceFormulario(ctx context.Context, a domain.Actor, vista string, maxEnvios int, caduca *time.Time) (EnlaceFormulario, string, error) {
	if maxEnvios == 0 {
		maxEnvios = 500
	}
	if maxEnvios < 1 || maxEnvios > 10000 {
		return EnlaceFormulario{}, "", fmt.Errorf("%w: cupo de envíos", domain.ErrInvalido)
	}
	if caduca != nil && (caduca.Before(time.Now()) || caduca.After(time.Now().AddDate(1, 0, 1))) {
		return EnlaceFormulario{}, "", fmt.Errorf("%w: caducidad", domain.ErrInvalido)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return EnlaceFormulario{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	e := EnlaceFormulario{ViewID: vista, MaxResponses: maxEnvios, ExpiresAt: caduca, CreatedBy: a.Sub}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		var tabla, base, tipo string
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text, kind FROM base_views WHERE id = $1 AND tenant = $2`, vista, a.Tenant).Scan(&tabla, &base, &tipo)); err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccFormulario); err != nil {
			return err
		}
		if tipo != string(domain.VistaFormulario) {
			return fmt.Errorf("%w: la vista no es un formulario", domain.ErrInvalido)
		}
		var vivos int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_form_links WHERE tenant = $1 AND view_id = $2 AND revoked_at IS NULL`, a.Tenant, vista).Scan(&vivos); err != nil {
			return err
		}
		if vivos >= MaxEnlacesPorFormulario {
			return domain.ErrLimite
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_form_links (tenant, base_id, table_id, view_id, token_hash, max_responses, expires_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id::text, created_at`,
			a.Tenant, base, tabla, vista, hashToken(token), maxEnvios, caduca, a.Sub).Scan(&e.ID, &e.CreatedAt); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "form.link.create", nil, jsonDe(map[string]any{"link": e.ID, "max": maxEnvios}))
	})
	if err != nil {
		return EnlaceFormulario{}, "", err
	}
	return e, token, nil
}

// ListarEnlacesFormulario devuelve los enlaces de una vista (propietario), sin tokens.
func (s *Store) ListarEnlacesFormulario(ctx context.Context, a domain.Actor, vista string) ([]EnlaceFormulario, error) {
	out := []EnlaceFormulario{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		var tabla, base string
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_views WHERE id = $1 AND tenant = $2`, vista, a.Tenant).Scan(&tabla, &base)); err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccFormulario); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, view_id::text, max_responses, responses, expires_at, revoked_at, created_by, created_at
			  FROM base_form_links WHERE tenant = $1 AND view_id = $2 ORDER BY created_at DESC, id LIMIT $3`,
			a.Tenant, vista, 50)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e EnlaceFormulario
			if err := rows.Scan(&e.ID, &e.ViewID, &e.MaxResponses, &e.Responses, &e.ExpiresAt, &e.RevokedAt, &e.CreatedBy, &e.CreatedAt); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

// RevocarEnlaceFormulario revoca un enlace (propietario). Surte efecto en el siguiente
// envío o lectura: el token se resuelve contra la base en cada petición.
func (s *Store) RevocarEnlaceFormulario(ctx context.Context, a domain.Actor, enlace string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		var tabla, base string
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_form_links WHERE id = $1 AND tenant = $2`, enlace, a.Tenant).Scan(&tabla, &base)); err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccFormulario); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE base_form_links SET revoked_at = coalesce(revoked_at, now()) WHERE id = $1 AND tenant = $2`, enlace, a.Tenant); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "form.link.revoke", nil, jsonDe(map[string]string{"link": enlace}))
	})
}

// enlaceVivo es lo que resuelve un token.
type enlaceVivo struct {
	id, tenant, tabla, base, vista string
}

// resolverToken busca un token sin saber de qué organización es: la ÚNICA lectura sin
// tenant del servicio. Un token mal formado, desconocido, revocado, caducado o sin cupo
// es lo mismo para quien llama: ErrNoExiste.
func (s *Store) resolverToken(ctx context.Context, token string) (enlaceVivo, error) {
	if s.pool == nil || !tokenFormulario.MatchString(token) {
		return enlaceVivo{}, domain.ErrNoExiste
	}
	var e enlaceVivo
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.bases_publico', 'on', true)`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT id::text, tenant, table_id::text, base_id::text, view_id::text
			  FROM base_form_links
			 WHERE token_hash = $1 AND revoked_at IS NULL
			   AND (expires_at IS NULL OR expires_at > now()) AND responses < max_responses`,
			hashToken(token)).Scan(&e.id, &e.tenant, &e.tabla, &e.base, &e.vista)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return enlaceVivo{}, domain.ErrNoExiste
	}
	return e, err
}

// AmbitoFormulario es el ámbito del reto ALTCHA de un enlace: un reto emitido para un
// formulario no vale en otro. Deriva del hash del token, que no es secreto del enlace.
func AmbitoFormulario(token string) string { return "bases-form:" + hashToken(token)[:16] }

// FormularioPorToken devuelve lo que se enseña al abrir un enlace.
func (s *Store) FormularioPorToken(ctx context.Context, token string) (FormularioPublico, error) {
	e, err := s.resolverToken(ctx, token)
	if err != nil {
		return FormularioPublico{}, err
	}
	actor := domain.Actor{Sub: "form:" + e.id, Tenant: e.tenant}
	var out FormularioPublico
	err = s.en(ctx, actor, func(tx pgx.Tx) error {
		var cfgRaw []byte
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT name, config FROM base_views WHERE id = $1 AND tenant = $2 AND kind = 'form'`, e.vista, e.tenant).Scan(&out.Name, &cfgRaw)); err != nil {
			return err
		}
		var cfg domain.ConfigVista
		_ = json.Unmarshal(cfgRaw, &cfg)
		todos, err := camposDe(ctx, tx, actor, e.tabla)
		if err != nil {
			return err
		}
		out.Fields = camposDelFormulario(todos, cfg)
		return nil
	})
	return out, err
}

// EnviarFormulario añade un registro desde un enlace público. Todo en UNA transacción:
// consumir el cupo (que comprueba a la vez revocación, caducidad y límite), validar con
// los mismos tipos que la rejilla e insertar. Lo que no es un campo del formulario se
// RECHAZA: quien envía no puede escribir donde la vista no le deja.
func (s *Store) EnviarFormulario(ctx context.Context, token string, valores map[string]json.RawMessage) error {
	e, err := s.resolverToken(ctx, token)
	if err != nil {
		return err
	}
	actor := domain.Actor{Sub: "form:" + e.id, Tenant: e.tenant}
	return s.en(ctx, actor, func(tx pgx.Tx) error {
		// El cupo se consume atómicamente: dos envíos a la vez no pasan del máximo.
		tag, err := tx.Exec(ctx, `
			UPDATE base_form_links SET responses = responses + 1
			 WHERE id = $1 AND tenant = $2 AND revoked_at IS NULL
			   AND (expires_at IS NULL OR expires_at > now()) AND responses < max_responses`, e.id, e.tenant)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		var cfgRaw []byte
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT config FROM base_views WHERE id = $1 AND tenant = $2 AND kind = 'form'`, e.vista, e.tenant).Scan(&cfgRaw)); err != nil {
			return err
		}
		var cfg domain.ConfigVista
		_ = json.Unmarshal(cfgRaw, &cfg)
		todos, err := camposDe(ctx, tx, actor, e.tabla)
		if err != nil {
			return err
		}
		permitidos := camposDelFormulario(todos, cfg)
		for id := range valores {
			if !slices.ContainsFunc(permitidos, func(c domain.Campo) bool { return c.ID == id }) {
				return fmt.Errorf("%w: campo que el formulario no admite", domain.ErrInvalido)
			}
		}
		limpio, err := domain.ValidarDatos(permitidos, valores)
		if err != nil {
			return err
		}
		if len(limpio) == 0 {
			return fmt.Errorf("%w: formulario vacío", domain.ErrInvalido)
		}
		if err := domain.TamanoDatos(limpio); err != nil {
			return err
		}
		if err := hayHueco(ctx, tx, e.tenant, e.tabla); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_records (tenant, base_id, table_id, data, created_by, updated_by)
			VALUES ($1, $2, $3, $4::jsonb, $5, $5) RETURNING id::text`,
			e.tenant, e.base, e.tabla, jsonDe(limpio), actor.Sub).Scan(&id); err != nil {
			return err
		}
		if _, err := trasEscribir(ctx, tx, actor, e.base, e.tabla, []string{id}); err != nil {
			return err
		}
		return auditar(ctx, tx, actor, e.base, &e.tabla, &id, nil, "record.create.form", nil, jsonDe(limpio))
	})
}

// RegistrarAltcha anota una firma ALTCHA usada y dice si era nueva (un reto resuelto se
// acepta UNA vez). Sin tenant ni RLS: la firma es anónima.
func (s *Store) RegistrarAltcha(ctx context.Context, firma string, caduca time.Time) (bool, error) {
	if s.pool == nil {
		return true, nil
	}
	// Barrido de paso: lo caducado ya no hace falta recordarlo (Verify lo rechaza antes).
	_, _ = s.pool.Exec(ctx, `DELETE FROM base_altcha_used WHERE expires_at < now() - interval '1 hour'`)
	tag, err := s.pool.Exec(ctx, `INSERT INTO base_altcha_used (signature, expires_at) VALUES ($1, $2) ON CONFLICT (signature) DO NOTHING`, firma, caduca)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
