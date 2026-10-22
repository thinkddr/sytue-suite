// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Dispositivos de confianza (mig 0022). Aislamiento por tenant a nivel de aplicación:
// las lecturas/mutaciones self-service se acotan por user_id; las admin por org_id. El
// device_id (derivado, único global) es la llave natural del upsert por-login.

const deviceCols = `id, org_id, user_id, device_id, name, user_agent, trusted, first_seen, last_seen, last_ip`

func scanDevice(row pgx.Row) (domain.Device, error) {
	var d domain.Device
	err := row.Scan(&d.ID, &d.OrgID, &d.UserID, &d.DeviceID, &d.Name, &d.UserAgent,
		&d.Trusted, &d.FirstSeen, &d.LastSeen, &d.LastIP)
	return d, err
}

// UpsertDevice registra el dispositivo o refresca su última actividad (por device_id,
// la llave estable por usuario+navegador). En el primer registro fija first_seen y
// trusted=false; en los siguientes actualiza name/user_agent/last_seen/last_ip SIN
// tocar `trusted` (la confianza persiste entre logins). Devuelve la fila resultante.
func (s *Store) UpsertDevice(ctx context.Context, q pg.Querier, d domain.Device) (domain.Device, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO devices (org_id, user_id, device_id, name, user_agent, last_ip)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (device_id) DO UPDATE SET
			name = EXCLUDED.name, user_agent = EXCLUDED.user_agent,
			last_seen = now(), last_ip = EXCLUDED.last_ip
		RETURNING ` + deviceCols
	got, err := scanDevice(q.QueryRow(ctx, sql, d.OrgID, d.UserID, d.DeviceID, d.Name, d.UserAgent, d.LastIP))
	return got, errf("UpsertDevice", err)
}

// GetDeviceByDeviceID recupera el dispositivo (user, device_id) — la lectura que hace
// el motor de Acceso Condicional para saber si es de confianza. Acotado por user_id
// (la confianza es por usuario). ErrNotFound si no está registrado.
func (s *Store) GetDeviceByDeviceID(ctx context.Context, userID, deviceID string) (domain.Device, error) {
	const sql = `SELECT ` + deviceCols + ` FROM devices WHERE user_id = $1 AND device_id = $2`
	d, err := scanDevice(s.DB().QueryRow(ctx, sql, userID, deviceID))
	return d, errf("GetDeviceByDeviceID", err)
}

// GetDeviceForUser recupera un dispositivo por (user, id) — self-service, acotado al
// dueño (un id ajeno da ErrNotFound, sin fuga). Usado antes de marcar de confianza para
// comprobar que es el dispositivo actual.
func (s *Store) GetDeviceForUser(ctx context.Context, userID, id string) (domain.Device, error) {
	const sql = `SELECT ` + deviceCols + ` FROM devices WHERE id = $1 AND user_id = $2`
	d, err := scanDevice(s.DB().QueryRow(ctx, sql, id, userID))
	return d, errf("GetDeviceForUser", err)
}

// ListDevicesForUser lista los dispositivos del usuario (self-service), recientes
// primero. Sirve devices_user_idx.
func (s *Store) ListDevicesForUser(ctx context.Context, userID string) ([]domain.Device, error) {
	const sql = `SELECT ` + deviceCols + ` FROM devices WHERE user_id = $1 ORDER BY last_seen DESC, id`
	return s.queryDevices(ctx, sql, userID)
}

// ListDevicesForOrg lista los dispositivos de la organización (vista admin), recientes
// primero, con el email/nombre del dueño (join con users) para mostrarlo. Sirve
// devices_org_idx.
func (s *Store) ListDevicesForOrg(ctx context.Context, orgID string) ([]domain.Device, error) {
	const sql = `SELECT d.id, d.org_id, d.user_id, d.device_id, d.name, d.user_agent,
		d.trusted, d.first_seen, d.last_seen, d.last_ip, coalesce(u.email,''), coalesce(u.name,'')
		FROM devices d JOIN users u ON u.id = d.user_id
		WHERE d.org_id = $1 ORDER BY d.last_seen DESC, d.id`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListDevicesForOrg", err)
	}
	defer rows.Close()
	out := make([]domain.Device, 0, 8)
	for rows.Next() {
		var d domain.Device
		if err := rows.Scan(&d.ID, &d.OrgID, &d.UserID, &d.DeviceID, &d.Name, &d.UserAgent,
			&d.Trusted, &d.FirstSeen, &d.LastSeen, &d.LastIP, &d.UserEmail, &d.UserName); err != nil {
			return nil, errf("ListDevicesForOrg.scan", err)
		}
		out = append(out, d)
	}
	return out, errf("ListDevicesForOrg.iter", rows.Err())
}

func (s *Store) queryDevices(ctx context.Context, sql, arg string) ([]domain.Device, error) {
	rows, err := s.DB().Query(ctx, sql, arg)
	if err != nil {
		return nil, errf("queryDevices", err)
	}
	defer rows.Close()
	out := make([]domain.Device, 0, 8)
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, errf("queryDevices.scan", err)
		}
		out = append(out, d)
	}
	return out, errf("queryDevices.iter", rows.Err())
}

// SetDeviceTrustedForUser marca (o desmarca) de confianza un dispositivo del usuario,
// acotado por (user, id): un dispositivo ajeno no se toca. Devuelve la fila si existía.
func (s *Store) SetDeviceTrustedForUser(ctx context.Context, userID, id string, trusted bool) (domain.Device, error) {
	const sql = `UPDATE devices SET trusted = $3 WHERE id = $1 AND user_id = $2 RETURNING ` + deviceCols
	d, err := scanDevice(s.DB().QueryRow(ctx, sql, id, userID, trusted))
	return d, errf("SetDeviceTrustedForUser", err)
}

// DeleteDeviceForUser revoca (borra) un dispositivo del usuario, acotado por (user, id).
// Devuelve true si borró una fila (existía y era suya).
func (s *Store) DeleteDeviceForUser(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM devices WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, errf("DeleteDeviceForUser", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteDeviceForOrg revoca (borra) un dispositivo de la organización (vista admin),
// acotado por (org, id): aislamiento tenant, un dispositivo de otra org no se toca.
func (s *Store) DeleteDeviceForOrg(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM devices WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return false, errf("DeleteDeviceForOrg", err)
	}
	return tag.RowsAffected() > 0, nil
}
