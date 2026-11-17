// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// MaxMatrixRecoveryWrappers acota los envoltorios de una cuenta: una contraseña y
// unas pocas passkeys. Más es otra cosa que una persona con sus dispositivos.
const MaxMatrixRecoveryWrappers = 16

// MatrixRecoveryAccount dice de qué cuenta de Matrix es un envoltorio. La vacía
// es `@sytue_<sub>`, la de Rooms y del almacenamiento heredado, una sola en todas
// las organizaciones. Una cuenta aislada de almacenamiento es de una organización
// y un producto, y tiene su propia clave de recuperación (ADR 0082).
type MatrixRecoveryAccount struct {
	Tenant  string
	Product string
}

func (a MatrixRecoveryAccount) valid() bool { return (a.Tenant == "") == (a.Product == "") }

// Rooms dice si es la cuenta de Rooms, la única que pudo tener custodia con KEK.
func (a MatrixRecoveryAccount) Rooms() bool { return a.Tenant == "" && a.Product == "" }

// MatrixRecoveryWrapperRow es un envoltorio tal como se guardó: un JSON opaco
// que el servicio validó al recibirlo y no sabe abrir.
type MatrixRecoveryWrapperRow struct {
	ID        string
	Wrapper   []byte
	CreatedAt time.Time
}

// MatrixRecoveryWrappers devuelve los envoltorios de esa cuenta y si aún queda la
// custodia con KEK de ADR 0081 (solo la cuenta de Rooms la tuvo). La tabla solo
// se ve en mantenimiento: quien llama ya comprobó que `subject` es quien pide.
func (s *Store) MatrixRecoveryWrappers(ctx context.Context, subject string, account MatrixRecoveryAccount) ([]MatrixRecoveryWrapperRow, bool, error) {
	if !account.valid() {
		return nil, false, domain.ErrInvalidInput
	}
	var rows []MatrixRecoveryWrapperRow
	var legacy bool
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		result, err := tx.Query(ctx, `SELECT id::text, wrapper::text, created_at FROM matrix_recovery_wrappers
			WHERE subject=$1 AND organizacion=$2 AND product=$3 ORDER BY created_at, id`, subject, account.Tenant, account.Product)
		if err != nil {
			return err
		}
		for result.Next() {
			var row MatrixRecoveryWrapperRow
			var raw string
			if err := result.Scan(&row.ID, &raw, &row.CreatedAt); err != nil {
				result.Close()
				return err
			}
			row.Wrapper = []byte(raw)
			rows = append(rows, row)
		}
		result.Close()
		if err := result.Err(); err != nil {
			return err
		}
		if account.Rooms() {
			return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_recovery_keys WHERE subject=$1)`, subject).Scan(&legacy)
		}
		return nil
	})
	return rows, legacy, err
}

// PutMatrixRecoveryWrapper guarda (o sustituye) el envoltorio de ese tipo y esa
// passkey. En la misma transacción borra los envoltorios de otra clave de
// almacenamiento (la clave cambió: ya no abren nada) y, en la cuenta de Rooms, la
// copia con KEK de ADR 0081: desde aquí, nadie más que la persona puede leerla.
func (s *Store) PutMatrixRecoveryWrapper(ctx context.Context, subject string, account MatrixRecoveryAccount, kind, credentialID, keyID string, wrapper []byte) (string, error) {
	if !account.valid() || subject == "" || keyID == "" || len(wrapper) == 0 || (kind == "passkey") != (credentialID != "") {
		return "", domain.ErrInvalidInput
	}
	var id string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		// Serializa las escrituras de una misma cuenta: el límite y la rotación
		// se deciden sobre lo que hay, no sobre lo que había.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('matrix_recovery_wrappers:'||$1||chr(31)||$2||chr(31)||$3, 0))`, subject, account.Tenant, account.Product); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM matrix_recovery_wrappers WHERE subject=$1 AND organizacion=$2 AND product=$3 AND key_id<>$4`, subject, account.Tenant, account.Product, keyID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO matrix_recovery_wrappers(subject,organizacion,product,kind,credential_id,key_id,wrapper) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)
			ON CONFLICT(subject,organizacion,product,kind,credential_id) DO UPDATE SET key_id=EXCLUDED.key_id, wrapper=EXCLUDED.wrapper, created_at=now()
			RETURNING id::text`, subject, account.Tenant, account.Product, kind, credentialID, keyID, string(wrapper)).Scan(&id); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_recovery_wrappers WHERE subject=$1 AND organizacion=$2 AND product=$3`, subject, account.Tenant, account.Product).Scan(&count); err != nil {
			return err
		}
		if count > MaxMatrixRecoveryWrappers {
			return domain.ErrConflict
		}
		if account.Rooms() {
			_, err := tx.Exec(ctx, `DELETE FROM matrix_recovery_keys WHERE subject=$1`, subject)
			return err
		}
		return nil
	})
	return id, err
}
