// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// PlatformOAuthClient es la credencial con la que SYTUE se presenta ante un
// proveedor. Es de la plataforma, no de una organización: la misma app registrada
// sirve a todos los clientes, y es la diferencia entre conectar en un clic o
// pedirle a cada cliente que se dé de alta como desarrollador ante Google o
// Microsoft.
type PlatformOAuthClient struct {
	TemplateKey string `json:"template_key"`
	ClientID    string `json:"client_id"`
	// HasSecret y no el secret: la API es write-only. Un secreto que se puede leer
	// de vuelta acaba en un log, en una captura de pantalla o en el historial del
	// navegador de quien administra.
	HasSecret bool   `json:"has_secret"`
	UpdatedBy string `json:"updated_by"`
	UpdatedAt string `json:"updated_at"`
}

// ListPlatformOAuthClients devuelve las credenciales registradas, SIN secretos.
func (s *Store) ListPlatformOAuthClients(ctx context.Context) ([]PlatformOAuthClient, error) {
	// Sin tenantTx: esto no es de ninguna organización y no debe pasar por el rol
	// de aplicación, que solo tiene permisos sobre las tablas por-tenant.
	rows, err := s.pool.Query(ctx, `
		SELECT template_key, client_id, (client_secret_enc IS NOT NULL),
		       updated_by, to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM platform_oauth_clients ORDER BY template_key`)
	if err != nil {
		return nil, fmt.Errorf("store: listar credenciales de plataforma: %w", err)
	}
	defer rows.Close()
	var out []PlatformOAuthClient
	for rows.Next() {
		var c PlatformOAuthClient
		if err := rows.Scan(&c.TemplateKey, &c.ClientID, &c.HasSecret, &c.UpdatedBy, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: leer credencial de plataforma: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PutPlatformOAuthClient registra o reemplaza la credencial de un proveedor.
//
// secret vacío BORRA el que hubiera: hay proveedores que registran la app como
// cliente público y se apoyan solo en PKCE, así que «sin secret» es un estado
// legítimo y tiene que ser expresable. Para cambiar solo el client_id sin tocar
// el secret está keepSecret.
func (s *Store) PutPlatformOAuthClient(ctx context.Context, key, clientID, secret, updatedBy string, keepSecret bool) error {
	if !s.cipher.Enabled() && secret != "" {
		return ErrNoCipher
	}
	enc := ""
	if secret != "" {
		sealed, err := s.cipher.Encrypt(secret)
		if err != nil {
			return fmt.Errorf("store: sellar el secret del proveedor: %w", err)
		}
		enc = sealed
	}
	var sql string
	var args []any
	if keepSecret {
		// El UPDATE no menciona client_secret_enc, así que lo guardado se queda como
		// está. Permite corregir un client_id mal pegado sin volver a pedirle el
		// secret al proveedor, que en varios solo se enseña una vez y luego hay que
		// rotarlo para verlo otra vez.
		sql = `INSERT INTO platform_oauth_clients (template_key, client_id, client_secret_enc, updated_by)
		       VALUES ($1, $2, NULL, $3)
		       ON CONFLICT (template_key) DO UPDATE SET
		         client_id = excluded.client_id, updated_by = excluded.updated_by, updated_at = now()`
		args = []any{key, clientID, updatedBy}
	} else {
		sql = `INSERT INTO platform_oauth_clients (template_key, client_id, client_secret_enc, updated_by)
		       VALUES ($1, $2, nullif($3,''), $4)
		       ON CONFLICT (template_key) DO UPDATE SET
		         client_id = excluded.client_id, client_secret_enc = excluded.client_secret_enc,
		         updated_by = excluded.updated_by, updated_at = now()`
		args = []any{key, clientID, enc, updatedBy}
	}
	if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("store: guardar credencial de plataforma: %w", err)
	}
	return nil
}

// DeletePlatformOAuthClient retira la credencial de un proveedor. Las conexiones
// ya establecidas siguen funcionando con el refresh token que guardaron; lo que
// deja de haber es camino de un clic para conexiones NUEVAS.
func (s *Store) DeletePlatformOAuthClient(ctx context.Context, key string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM platform_oauth_clients WHERE template_key = $1`, key)
	if err != nil {
		return fmt.Errorf("store: borrar credencial de plataforma: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// LoadPlatformOAuthSecrets devuelve las credenciales CON el secret descifrado,
// para que el servicio se presente ante el proveedor. Solo lo usa el resolutor de
// credenciales; no hay ninguna ruta HTTP que devuelva esto.
//
// Una credencial cuyo secret no se puede descifrar se OMITE en vez de romper la
// carga entera: si alguien rota la KEK dejando material viejo detrás, es mejor
// perder ese proveedor —que vuelve a modo BYO, con su aviso— que quedarse sin
// ninguno.
func (s *Store) LoadPlatformOAuthSecrets(ctx context.Context) (map[string]struct{ ClientID, ClientSecret string }, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT template_key, client_id, coalesce(client_secret_enc,'') FROM platform_oauth_clients`)
	if err != nil {
		return nil, fmt.Errorf("store: cargar credenciales de plataforma: %w", err)
	}
	defer rows.Close()
	out := make(map[string]struct{ ClientID, ClientSecret string })
	for rows.Next() {
		var key, id, enc string
		if err := rows.Scan(&key, &id, &enc); err != nil {
			return nil, fmt.Errorf("store: leer credencial de plataforma: %w", err)
		}
		secret := ""
		if enc != "" {
			plain, derr := s.cipher.Decrypt(enc)
			if derr != nil {
				continue
			}
			secret = plain
		}
		out[key] = struct{ ClientID, ClientSecret string }{ClientID: id, ClientSecret: secret}
	}
	return out, rows.Err()
}
