// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Posponer y silenciar (M-18), EN LOCAL: es lo que la pantalla hace con
// `mail.Posponedor`, sin servidor que lo guarde.
//
//   - Posponer saca de Recibidos los mensajes de la conversación hasta una fecha
//     (`mensajes.pospuesto`); en el servidor siguen en INBOX. Al vencer, con la app
//     abierta o en el siguiente arranque, vuelven solos, sin leer y arriba de la lista
//     (`vuelto`). Si llega una respuesta antes, la conversación vuelve con ella.
//   - Silenciar archiva lo que la conversación tiene en Recibidos y manda al Archivo lo
//     que llegue después (`silenciados`). Mover sí se sube al servidor: es un MOVE.
//
// Los dos se quedan en este equipo: otro cliente de la misma cuenta no se entera.

// MailPospone: en Sytue Office se pospone y se silencia siempre (en local).
func (b *Buzon) MailPospone() bool { return true }

// Snoozes son las conversaciones pospuestas, la que vuelve antes primero.
func (b *Buzon) Snoozes(ctx context.Context) ([]api.MailSnooze, error) {
	filas, err := b.x.db.QueryContext(ctx, "SELECT hilo, MAX(pospuesto) AS h FROM mensajes WHERE pospuesto > 0 GROUP BY hilo ORDER BY h")
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []api.MailSnooze
	for filas.Next() {
		var s api.MailSnooze
		var h int64
		if err := filas.Scan(&s.ThreadID, &h); err != nil {
			return nil, err
		}
		s.Until = deUnix(h)
		out = append(out, s)
	}
	return out, filas.Err()
}

// Los límites de posponer, los del servicio.
const (
	minimoPosponer = time.Minute
	maximoPosponer = 366 * 24 * time.Hour
)

// SnoozeThread pospone lo que la conversación tiene en Recibidos (o cambia la fecha si
// ya lo estaba).
func (b *Buzon) SnoozeThread(ctx context.Context, hilo string, hasta time.Time) (api.MailSnooze, error) {
	if d := time.Until(hasta); d < minimoPosponer-time.Second || d > maximoPosponer {
		return api.MailSnooze{}, api.ErrSnoozeInvalido
	}
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return api.MailSnooze{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ids, err := idsDe(ctx, tx, `SELECT m.id`+desde+`WHERE m.hilo = ? AND (m.pospuesto > 0 OR f.rol = 'inbox')`, hilo)
	if err != nil {
		return api.MailSnooze{}, err
	}
	if len(ids) == 0 {
		return api.MailSnooze{}, api.ErrNadaQuePosponer
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET pospuesto = ?, vuelto = 0 WHERE id = ?", hasta.Unix(), id); err != nil {
			return api.MailSnooze{}, err
		}
		if err := apuntar(ctx, tx, id); err != nil {
			return api.MailSnooze{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return api.MailSnooze{}, err
	}
	b.avisar()
	return api.MailSnooze{ThreadID: hilo, Until: deUnix(hasta.Unix())}, nil
}

// UnsnoozeThread la devuelve ya a Recibidos, con su fecha de llegada (quien la trae a
// mano ya la está mirando).
func (b *Buzon) UnsnoozeThread(ctx context.Context, hilo string) error {
	return b.devolver(ctx, "hilo = ? AND pospuesto > 0", []any{hilo}, false)
}

// devolver saca de «Pospuestos» lo que cumple donde. solo: ha vencido, y vuelve sin
// leer y arriba.
func (b *Buzon) devolver(ctx context.Context, donde string, args []any, solo bool) error {
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ids, err := idsDe(ctx, tx, "SELECT id FROM mensajes WHERE "+donde, args...)
	if err != nil || len(ids) == 0 {
		return err
	}
	vuelto := int64(0)
	if solo {
		vuelto = time.Now().Unix()
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET pospuesto = 0, vuelto = ? WHERE id = ?", vuelto, id); err != nil {
			return err
		}
		if err := apuntar(ctx, tx, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if solo {
		txt := make([]string, len(ids))
		for i, id := range ids {
			txt[i] = strconv.FormatInt(id, 10)
		}
		_ = b.SetFlagMany(ctx, txt, api.MailFlagSeen, false)
	}
	b.avisar()
	return nil
}

// despertarPospuestos devuelve lo que ya ha vencido. Lo llama el bucle de Arrancar.
func (b *Buzon) despertarPospuestos(ctx context.Context) {
	_ = b.devolver(ctx, "pospuesto > 0 AND pospuesto <= ?", []any{time.Now().Unix()}, true)
}

// MuteThread silencia la conversación (lo que tenga en Recibidos se archiva) o deja de
// silenciarla. Devuelve cuántos mensajes ha archivado.
func (b *Buzon) MuteThread(ctx context.Context, hilo string, silenciar bool) (int, error) {
	if !silenciar {
		_, err := b.x.db.ExecContext(ctx, "DELETE FROM silenciados WHERE hilo = ?", hilo)
		return 0, err
	}
	if _, err := b.x.db.ExecContext(ctx, "INSERT OR IGNORE INTO silenciados (hilo) VALUES (?)", hilo); err != nil {
		return 0, err
	}
	ids, err := idsDe(ctx, b.x.db, `SELECT m.id`+desde+`WHERE m.hilo = ? AND f.rol = 'inbox'`, hilo)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	txt := make([]string, len(ids))
	for i, id := range ids {
		txt[i] = strconv.FormatInt(id, 10)
	}
	if err := b.MoveMany(ctx, txt, idCarpeta("", "", "archive")); err != nil {
		return 0, err
	}
	b.avisar()
	return len(ids), nil
}

// ThreadState dice si la conversación está silenciada o pospuesta.
func (b *Buzon) ThreadState(ctx context.Context, hilo string) (api.MailEstadoHilo, error) {
	var e api.MailEstadoHilo
	var n int
	if err := b.x.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM silenciados WHERE hilo = ?", hilo).Scan(&n); err != nil {
		return e, err
	}
	e.Muted = n > 0
	var h int64
	_ = b.x.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(pospuesto), 0) FROM mensajes WHERE hilo = ?", hilo).Scan(&h)
	if h > 0 {
		t := deUnix(h)
		e.SnoozedUntil = &t
	}
	return e, nil
}

// silenciarLlegadas trata lo recién llegado a Recibidos: lo de una conversación
// silenciada se va al Archivo y lo de una pospuesta la hace volver. Dice si movió algo.
func (b *Buzon) silenciarLlegadas(ctx context.Context, ids []int64) bool {
	hubo := false
	for _, n := range ids {
		s, err := b.x.sobre(ctx, n)
		if err != nil || s.hilo == "" {
			continue
		}
		var silenciado int
		_ = b.x.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM silenciados WHERE hilo = ?", s.hilo).Scan(&silenciado)
		if silenciado > 0 {
			hubo = b.MoveMany(ctx, []string{strconv.FormatInt(n, 10)}, idCarpeta("", "", "archive")) == nil || hubo
			continue
		}
		_ = b.devolver(ctx, "hilo = ? AND pospuesto > 0", []any{s.hilo}, true)
	}
	return hubo
}

// idsDe son los ids que devuelve una consulta de una columna.
func idsDe(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, consulta string, args ...any,
) ([]int64, error) {
	filas, err := q.QueryContext(ctx, consulta, args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []int64
	for filas.Next() {
		var id int64
		if err := filas.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, filas.Err()
}

// ---- Cuota (QUOTA, RFC 9208) ----

// cuota es el espacio de una cuenta tal como lo contó el servidor en la última vuelta.
type cuota struct {
	Usado  int64 `json:"usado"`
	Limite int64 `json:"limite"`
}

func claveCuota(cuenta string) string { return "cuota_" + cuenta }

func (b *Buzon) ponerCuota(ctx context.Context, cuenta string, q cuota) error {
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	return b.x.ponerAjuste(ctx, claveCuota(cuenta), string(raw))
}

// Quota es el espacio de las cuentas que lo cuentan (IMAP QUOTA), sumado: el medidor
// del pie del lateral. Sin ninguna que lo cuente, cero (y el medidor no sale).
func (b *Buzon) Quota(ctx context.Context) (api.MailQuota, error) {
	var out api.MailQuota
	for _, c := range b.reg.lista() {
		v, err := b.x.ajuste(ctx, claveCuota(c.ID))
		if err != nil || strings.TrimSpace(v) == "" {
			continue
		}
		var q cuota
		if json.Unmarshal([]byte(v), &q) == nil && q.Limite > 0 {
			out.UsedBytes += q.Usado
			out.QuotaBytes += q.Limite
		}
	}
	return out, nil
}
