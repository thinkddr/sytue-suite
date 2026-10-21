// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Conector de SOLO LECTURA (pieza D del ADR 0068). El propietario de una base concede a
// un consumidor (hoy, Calcus) la lectura de UNA tabla o de UNA vista con un token que
// caduca y que puede revocar: el token se resuelve contra la base en cada lectura, así
// que revocar corta de inmediato. El conector no escribe NUNCA: no hay ruta que lo haga.

const (
	// PrefijoTokenConector distingue un token de conector de cualquier otra credencial.
	PrefijoTokenConector = "sbc_"
	// MaxConexionesPorBase acota las conexiones vivas de una base.
	MaxConexionesPorBase = 50
	// MaxFilasConector es el tope de filas de una lectura (el de los orígenes de Calcus).
	MaxFilasConector = 10000
	// DiasConexionPorDefecto y MaxDiasConexion acotan la caducidad.
	DiasConexionPorDefecto = 90
	MaxDiasConexion        = 365
	// ConsumidorCalcus es el consumidor por defecto: el único que había antes de 0020.
	ConsumidorCalcus = "calcus"
)

// consumidores son los productos que pueden leer por una conexión. `vista` dice si la
// conexión ha de acotarse a una VISTA: los nuevos sí (lo que sale de Bases hacia un
// fichero, un informe o un sitio público es lo que el propietario dejó en una vista, no la
// tabla entera); Calcus conserva la tabla entera, que ya tenía.
var consumidores = map[string]struct{ vista bool }{
	ConsumidorCalcus: {},
	"drive":          {vista: true},
	"erp":            {vista: true},
	"web":            {vista: true},
}

// Conexion es una concesión de lectura. Nunca lleva el token.
type Conexion struct {
	ID         string     `json:"id"`
	BaseID     string     `json:"base_id"`
	TableID    string     `json:"table_id"`
	ViewID     string     `json:"view_id,omitempty"`
	Name       string     `json:"name"`
	Consumer   string     `json:"consumer"`
	Version    int        `json:"version"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	Reads      int64      `json:"reads"`
	LastReadAt *time.Time `json:"last_read_at,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Procedencia dice de dónde salen los datos de una lectura: lo que el consumidor enseña
// junto a la tabla para que quien la mira sepa qué es y quién se lo dio.
type Procedencia struct {
	Source       string    `json:"source"` // siempre "sytue-bases"
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

// ColumnaConector y LecturaConector son el contrato tabular de los orígenes de Calcus
// ({columns, rows}) más la procedencia.
type ColumnaConector struct {
	Name string `json:"name"`
	Type string `json:"type"` // text | number | boolean
}

// LecturaConector es lo que devuelve una lectura.
type LecturaConector struct {
	Columns    []ColumnaConector `json:"columns"`
	Rows       [][]any           `json:"rows"`
	Provenance Procedencia       `json:"provenance"`
}

func nuevoTokenConector() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return PrefijoTokenConector + base64.RawURLEncoding.EncodeToString(raw), nil
}

// CrearConexion concede una lectura (propietario) y devuelve el token, que no se podrá
// volver a leer. `vista` vacío = la tabla entera (solo Calcus). `consumidor` vacío = Calcus.
func (s *Store) CrearConexion(ctx context.Context, a domain.Actor, tabla, vista, nombre, consumidor string, dias int) (Conexion, string, error) {
	nombre, err := domain.LimpiarNombre(nombre)
	if err != nil {
		return Conexion{}, "", err
	}
	if consumidor == "" {
		consumidor = ConsumidorCalcus
	}
	regla, ok := consumidores[consumidor]
	if !ok {
		return Conexion{}, "", fmt.Errorf("%w: consumidor desconocido", domain.ErrInvalido)
	}
	if regla.vista && vista == "" {
		return Conexion{}, "", fmt.Errorf("%w: esta conexión ha de acotarse a una vista", domain.ErrInvalido)
	}
	if dias == 0 {
		dias = DiasConexionPorDefecto
	}
	if dias < 1 || dias > MaxDiasConexion {
		return Conexion{}, "", fmt.Errorf("%w: caducidad entre 1 y %d días", domain.ErrInvalido, MaxDiasConexion)
	}
	token, err := nuevoTokenConector()
	if err != nil {
		return Conexion{}, "", err
	}
	c := Conexion{TableID: tabla, ViewID: vista, Name: nombre, Consumer: consumidor, Version: 1, CreatedBy: a.Sub}
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		base, err := autorizarTabla(ctx, tx, a, tabla, domain.AccConector)
		if err != nil {
			return err
		}
		c.BaseID = base
		if vista != "" {
			// La vista ha de ser de ESTA tabla: un id de vista de otra tabla no acota nada.
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_views WHERE id = $1 AND table_id = $2 AND tenant = $3`, vista, tabla, a.Tenant).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return domain.ErrNoExiste
			}
		}
		var vivas int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_connections WHERE tenant = $1 AND base_id = $2 AND revoked_at IS NULL AND expires_at > now()`, a.Tenant, base).Scan(&vivas); err != nil {
			return err
		}
		if vivas >= MaxConexionesPorBase {
			return domain.ErrLimite
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_connections (tenant, base_id, table_id, view_id, name, token_hash, expires_at, created_by, consumer)
			VALUES ($1, $2, $3, $4::uuid, $5, $6, now() + make_interval(days => $7), $8, $9)
			RETURNING id::text, expires_at, created_at`,
			a.Tenant, base, tabla, ptr(vista), nombre, hashToken(token), dias, a.Sub, consumidor).Scan(&c.ID, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "connector.grant", nil,
			jsonDe(map[string]any{"connection": c.ID, "view": vista, "days": dias, "consumer": c.Consumer}))
	})
	if err != nil {
		return Conexion{}, "", err
	}
	return c, token, nil
}

// ListarConexiones devuelve las conexiones de una base (propietario), sin tokens.
func (s *Store) ListarConexiones(ctx context.Context, a domain.Actor, base string) ([]Conexion, error) {
	out := []Conexion{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, base, "", domain.AccConector); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, base_id::text, table_id::text, coalesce(view_id::text, ''), name, consumer, version,
			       expires_at, revoked_at, reads, last_read_at, created_by, created_at
			  FROM base_connections WHERE tenant = $1 AND base_id = $2 ORDER BY created_at DESC, id LIMIT 200`, a.Tenant, base)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Conexion
			if err := rows.Scan(&c.ID, &c.BaseID, &c.TableID, &c.ViewID, &c.Name, &c.Consumer, &c.Version,
				&c.ExpiresAt, &c.RevokedAt, &c.Reads, &c.LastReadAt, &c.CreatedBy, &c.CreatedAt); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// conexionDe resuelve la base y la tabla de una conexión y exige ser propietario.
func conexionDe(ctx context.Context, tx pgx.Tx, a domain.Actor, conexion string) (base, tabla string, err error) {
	if e := errNoFilas(tx.QueryRow(ctx, `SELECT base_id::text, table_id::text FROM base_connections WHERE id = $1 AND tenant = $2`, conexion, a.Tenant).Scan(&base, &tabla)); e != nil {
		return "", "", e
	}
	// A nivel de BASE: quien revoca es el propietario de la base, no el de una tabla.
	return base, tabla, autorizar(ctx, tx, a, base, "", domain.AccConector)
}

// RevocarConexion revoca una conexión (propietario). La siguiente lectura del consumidor
// ya es un 401: el token se comprueba contra esta fila cada vez.
func (s *Store) RevocarConexion(ctx context.Context, a domain.Actor, conexion string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		base, tabla, err := conexionDe(ctx, tx, a, conexion)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE base_connections SET revoked_at = coalesce(revoked_at, now()), revoked_by = coalesce(revoked_by, $3) WHERE id = $1 AND tenant = $2`, conexion, a.Tenant, a.Sub); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "connector.revoke", nil, jsonDe(map[string]string{"connection": conexion}))
	})
}

// RotarConexion cambia el token de una conexión viva (propietario): el anterior deja de
// valer en el acto y `version` sube, de modo que la procedencia delata el cambio.
func (s *Store) RotarConexion(ctx context.Context, a domain.Actor, conexion string) (int, string, error) {
	token, err := nuevoTokenConector()
	if err != nil {
		return 0, "", err
	}
	var version int
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		base, tabla, err := conexionDe(ctx, tx, a, conexion)
		if err != nil {
			return err
		}
		if err := errNoFilas(tx.QueryRow(ctx, `
			UPDATE base_connections SET token_hash = $3, version = version + 1
			 WHERE id = $1 AND tenant = $2 AND revoked_at IS NULL AND expires_at > now() RETURNING version`,
			conexion, a.Tenant, hashToken(token)).Scan(&version)); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "connector.rotate", nil, jsonDe(map[string]any{"connection": conexion, "version": version}))
	})
	if err != nil {
		return 0, "", err
	}
	return version, token, nil
}

// conexionAjena dice si quien presenta el token NO es a quien se concedió: otro
// consumidor, otra organización, o un consumidor que debe decir su organización y no la dice.
func conexionAjena(concedidoA, consumidor, tenant, tenantLector string) bool {
	if concedidoA != consumidor {
		return true
	}
	if tenantLector == "" {
		return consumidor != ConsumidorCalcus
	}
	return tenantLector != tenant
}

// LeerPorConector es la ÚNICA operación del consumidor: leer lo que la conexión abarca.
// Un token mal formado, desconocido, revocado o caducado es lo mismo: ErrNoExiste (401
// en HTTP), sin distinguir el motivo. Cada lectura se cuenta y se audita.
//
// `consumidor` es quien dice ser el que llama (vacío = Calcus) y ha de ser el de la
// conexión: el token que se concedió a SyERP no publica un sitio. `tenant` es la
// organización para la que lee: si viene, ha de ser la de la base, y un consumidor que no
// sea Calcus está obligado a mandarla (Bases no se comparte entre organizaciones; con Calcus
// se tolera su ausencia porque sus conexiones son anteriores a esta regla). Un token válido
// presentado por quien no toca también es ErrNoExiste, y queda auditado (`connector.denied`).
func (s *Store) LeerPorConector(ctx context.Context, token, consumidor, tenantLector, remoto string) (LecturaConector, error) {
	if consumidor == "" {
		consumidor = ConsumidorCalcus
	}
	if s.pool == nil || !strings.HasPrefix(token, PrefijoTokenConector) || !tokenFormulario.MatchString(strings.TrimPrefix(token, PrefijoTokenConector)) {
		return LecturaConector{}, domain.ErrNoExiste
	}
	var id, tenant, base, tabla, concedidoA string
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.bases_publico', 'on', true)`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text, tenant, base_id::text, table_id::text, consumer FROM base_connections WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
			hashToken(token)).Scan(&id, &tenant, &base, &tabla, &concedidoA)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LecturaConector{}, domain.ErrNoExiste
	}
	if err != nil {
		return LecturaConector{}, err
	}
	actor := domain.Actor{Sub: "conn:" + id, Tenant: tenant}
	if conexionAjena(concedidoA, consumidor, tenant, tenantLector) {
		// No se lee nada. Se anota quién lo intentó: un token en el consumidor equivocado
		// o en otra organización es lo que el propietario querría saber.
		_ = s.en(ctx, actor, func(tx pgx.Tx) error {
			return auditar(ctx, tx, actor, base, &tabla, nil, nil, "connector.denied", nil,
				jsonDe(map[string]any{"connection": id, "consumer": consumidor, "granted_to": concedidoA, "other_tenant": tenantLector != "" && tenantLector != tenant, "remote": remoto}))
		})
		return LecturaConector{}, domain.ErrNoExiste
	}
	var out LecturaConector
	err = s.en(ctx, actor, func(tx pgx.Tx) error {
		p := Procedencia{Source: "sytue-bases", Consumer: consumidor, ConnectionID: id}
		// Se vuelve a comprobar dentro de la transacción del tenant (revocación entre medias)
		// y se cuenta la lectura en la misma sentencia.
		if err := errNoFilas(tx.QueryRow(ctx, `
			UPDATE base_connections SET reads = reads + 1, last_read_at = now()
			 WHERE id = $1 AND tenant = $2 AND token_hash = $3 AND consumer = $4 AND revoked_at IS NULL AND expires_at > now()
			RETURNING base_id::text, table_id::text, coalesce(view_id::text, ''), version, created_by, created_at, expires_at, now()`,
			id, tenant, hashToken(token), consumidor).Scan(&p.BaseID, &p.TableID, &p.ViewID, &p.Version, &p.GrantedBy, &p.GrantedAt, &p.ExpiresAt, &p.ReadAt)); err != nil {
			return err
		}
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT b.name, t.name FROM bases b JOIN base_tables t ON t.base_id = b.id AND t.tenant = b.tenant WHERE t.id = $1 AND b.tenant = $2`, p.TableID, tenant).Scan(&p.BaseName, &p.TableName)); err != nil {
			return err
		}
		campos, err := camposDe(ctx, tx, actor, p.TableID)
		if err != nil {
			return err
		}
		var cfg domain.ConfigVista
		if p.ViewID != "" {
			var raw []byte
			if err := errNoFilas(tx.QueryRow(ctx, `SELECT name, config FROM base_views WHERE id = $1 AND table_id = $2 AND tenant = $3`, p.ViewID, p.TableID, tenant).Scan(&p.ViewName, &raw)); err != nil {
				return err
			}
			_ = json.Unmarshal(raw, &cfg)
		}
		regs, _, err := leerRegistros(ctx, tx, actor, p.TableID, campos, cfg, MaxFilasConector, "")
		if err != nil {
			return err
		}
		etiquetas, err := etiquetasDeEnlaces(ctx, tx, actor, campos, regs)
		if err != nil {
			return err
		}
		// Una vista con lista de campos (un formulario) acota también las COLUMNAS: la
		// conexión no enseña lo que la vista no enseña.
		out = tablaDeConector(camposDeLaVista(campos, cfg), regs, etiquetas)
		out.Provenance = p
		tabla := p.TableID
		return auditar(ctx, tx, actor, p.BaseID, &tabla, nil, nil, "connector.read", nil,
			jsonDe(map[string]any{"connection": id, "version": p.Version, "rows": len(regs), "remote": remoto, "consumer": consumidor}))
	})
	return out, err
}

// tablaDeConector pasa registros al contrato {columns, rows}. Un enlace sale como el
// texto de los registros enlazados; una selección múltiple, como sus opciones unidas.
// camposDeLaVista son los campos que una conexión enseña: los de la lista de la vista, en
// su orden, o todos si la vista no tiene lista (rejilla, tablero o tabla entera).
func camposDeLaVista(campos []domain.Campo, cfg domain.ConfigVista) []domain.Campo {
	if len(cfg.Fields) == 0 {
		return campos
	}
	out := make([]domain.Campo, 0, len(cfg.Fields))
	for _, id := range cfg.Fields {
		if i := slices.IndexFunc(campos, func(c domain.Campo) bool { return c.ID == id }); i >= 0 {
			out = append(out, campos[i])
		}
	}
	return out
}

func tablaDeConector(campos []domain.Campo, regs []domain.Registro, etiquetas map[string]string) LecturaConector {
	out := LecturaConector{Columns: make([]ColumnaConector, len(campos)), Rows: make([][]any, 0, len(regs))}
	for i, c := range campos {
		tipo := "text"
		switch c.Efectivo().Kind { //nolint:exhaustive // el resto viaja como texto
		case domain.TipoNumero:
			tipo = "number"
		case domain.TipoCasilla:
			tipo = "boolean"
		}
		out.Columns[i] = ColumnaConector{Name: c.Name, Type: tipo}
	}
	for _, r := range regs {
		fila := make([]any, len(campos))
		for i, c := range campos {
			switch c.Kind { //nolint:exhaustive // el resto se decodifica tal cual
			case domain.TipoRelacion:
				nombres := make([]string, 0, len(r.Links[c.ID]))
				for _, id := range r.Links[c.ID] {
					nombres = append(nombres, etiquetas[id])
				}
				if len(nombres) > 0 {
					fila[i] = strings.Join(nombres, ", ")
				}
			case domain.TipoMultiple:
				var l []string
				if json.Unmarshal(r.Data[c.ID], &l) == nil && len(l) > 0 {
					fila[i] = strings.Join(l, ", ")
				}
			default:
				var v any
				if raw := r.Data[c.ID]; len(raw) > 0 && json.Unmarshal(raw, &v) == nil {
					fila[i] = v
				}
			}
		}
		out.Rows = append(out.Rows, fila)
	}
	return out
}

// etiquetasDeEnlaces da nombre a los registros enlazados desde `regs`: el primer campo
// con valor de cada uno (como en la interfaz). Una consulta por tabla destino.
func etiquetasDeEnlaces(ctx context.Context, tx pgx.Tx, a domain.Actor, campos []domain.Campo, regs []domain.Registro) (map[string]string, error) {
	out := map[string]string{}
	for _, c := range campos {
		if c.Kind != domain.TipoRelacion {
			continue
		}
		var ids []string
		for _, r := range regs {
			ids = append(ids, r.Links[c.ID]...)
		}
		if len(ids) == 0 {
			continue
		}
		destino, err := camposDe(ctx, tx, a, c.LinkTableID)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `SELECT id::text, data FROM base_records WHERE tenant = $1 AND table_id = $2 AND id = ANY($3::uuid[]) LIMIT $4`,
			a.Tenant, c.LinkTableID, ids, domain.MaxRegistrosPorTabla)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var raw []byte
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			data := map[string]json.RawMessage{}
			_ = json.Unmarshal(raw, &data)
			out[id] = etiquetaDe(destino, data, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// etiquetaDe es el texto con el que se nombra un registro: su primer campo de texto,
// número, fecha o selección con valor; si no tiene, el principio de su id.
func etiquetaDe(campos []domain.Campo, data map[string]json.RawMessage, id string) string {
	for _, c := range campos {
		if c.Kind == domain.TipoRelacion || c.Kind == domain.TipoCasilla || c.Kind == domain.TipoMultiple {
			continue
		}
		var v any
		if raw := data[c.ID]; len(raw) > 0 && json.Unmarshal(raw, &v) == nil && v != nil {
			return fmt.Sprint(v)
		}
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
