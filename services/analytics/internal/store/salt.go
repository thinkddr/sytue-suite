// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// SaltLen son los bytes de la sal diaria (256 bits, la longitud de bloque de
// SHA-256: más no aporta a HMAC).
const SaltLen = 32

// Salts custodia la SAL DIARIA, que es la pieza sobre la que se sostiene todo el
// ADR 0026. Una sola al día para toda la plataforma (el site_id va dentro del
// HMAC, y es eso lo que separa a dos sitios), cifrada en reposo, cacheada en
// proceso y DESTRUIDA al cambiar el día:
//
//   - en PostgreSQL se borra la fila del día anterior;
//   - en memoria se sobrescribe con ceros el búfer de la sal vieja.
//
// Lo que compra no es una política sino una imposibilidad: pasada la medianoche,
// NADIE —tampoco Sytue con acceso root— puede recalcular el identificador de un
// visitante de ayer, porque el material para hacerlo ya no existe. Su precio,
// aceptado y documentado: no hay «visitantes únicos del mes», ni retorno, ni
// cohortes, y una visita que cruza medianoche cuenta como dos.
type Salts struct {
	// load lee (o crea) la sal del día indicado. Es un campo y no una llamada
	// directa al pool para que el test pueda ejercitar el cambio de día —la
	// propiedad crítica— sin levantar PostgreSQL.
	load func(ctx context.Context, day time.Time) ([]byte, error)
	now  func() time.Time

	mu   sync.Mutex
	day  string
	salt []byte
}

// NewSalts construye el custodio sobre PostgreSQL. `flag` es la variable de
// sesión con la que este proceso abre la tabla: IngestSettingKey en el colector
// y pg.MaintenanceSettingKey en la API/cron. cipher puede ser nil (sin KEK): la
// sal se guardaría EN CLARO, que es modo de desarrollo, no de producción.
func NewSalts(pool *pg.Pool, cipher *keycrypto.Cipher, flag string) *Salts {
	return newSalts(func(ctx context.Context, day time.Time) ([]byte, error) {
		return loadOrCreateSalt(ctx, pool, cipher, flag, day)
	}, time.Now)
}

func newSalts(load func(context.Context, time.Time) ([]byte, error), now func() time.Time) *Salts {
	return &Salts{load: load, now: now}
}

// Today devuelve la sal del día EN UTC. Al detectar cambio de día pide la nueva
// y borra la anterior de la memoria del proceso. Devuelve una COPIA: así el
// borrado no puede vaciarle la clave bajo los pies a un evento en vuelo.
func (s *Salts) Today(ctx context.Context) ([]byte, error) {
	day := s.now().UTC()
	key := day.Format(time.DateOnly)

	s.mu.Lock()
	defer s.mu.Unlock()
	if key == s.day && len(s.salt) == SaltLen {
		return append([]byte(nil), s.salt...), nil
	}

	fresh, err := s.load(ctx, day)
	if err != nil {
		return nil, err
	}
	if len(fresh) != SaltLen {
		return nil, fmt.Errorf("analytics: la sal del día %s mide %d bytes, se esperaban %d", key, len(fresh), SaltLen)
	}
	// Ceros sobre la sal de ayer ANTES de soltar la referencia: que un volcado de
	// memoria del proceso no la contenga.
	for i := range s.salt {
		s.salt[i] = 0
	}
	s.day, s.salt = key, fresh
	return append([]byte(nil), fresh...), nil
}

// loadOrCreateSalt devuelve la sal del día, creándola si no existe, y BORRA las
// de cualquier otro día en la misma transacción. Es idempotente entre réplicas:
// la primera que llega inserta y el resto lee la misma fila.
func loadOrCreateSalt(ctx context.Context, pool *pg.Pool, cipher *keycrypto.Cipher, flag string, day time.Time) ([]byte, error) {
	fresh := make([]byte, SaltLen)
	if _, err := rand.Read(fresh); err != nil {
		return nil, fmt.Errorf("analytics: generar sal: %w", err)
	}
	sealed, err := cipher.Encrypt(base64.StdEncoding.EncodeToString(fresh))
	if err != nil {
		return nil, fmt.Errorf("analytics: cifrar sal: %w", err)
	}

	var stored string
	err = pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config($1, 'on', true)", flag); err != nil {
			return fmt.Errorf("analytics: set_config %s: %w", flag, err)
		}
		// El borrado va PRIMERO y en la misma transacción que la lectura: no hay
		// instante en el que convivan dos días de sal.
		if _, err := tx.Exec(ctx, `DELETE FROM analytics_salts WHERE day <> $1::date`, day); err != nil {
			return fmt.Errorf("analytics: purgar sales antiguas: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO analytics_salts (day, salt) VALUES ($1::date, $2)
			 ON CONFLICT (day) DO NOTHING`, day, sealed); err != nil {
			return fmt.Errorf("analytics: guardar sal: %w", err)
		}
		return tx.QueryRow(ctx, `SELECT salt FROM analytics_salts WHERE day = $1::date`, day).Scan(&stored)
	})
	if err != nil {
		return nil, err
	}

	plain, err := cipher.Decrypt(stored)
	if err != nil {
		return nil, fmt.Errorf("analytics: descifrar sal: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(plain)
	if err != nil {
		return nil, fmt.Errorf("analytics: sal corrupta: %w", err)
	}
	return salt, nil
}
