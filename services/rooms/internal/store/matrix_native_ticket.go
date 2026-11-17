// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type matrixNativeTicket struct {
	token, product, purpose, clientID string
	approvedAt                        *time.Time
}

// consumeMatrixNativeTicketTx is the ONE PKCE/ticket exchange for legacy and
// resource handoffs. Resource purpose/client/approval are retained before SSO;
// a finish JSON field can select neither the product nor its authority.
func (s *Store) consumeMatrixNativeTicketTx(ctx context.Context, tx pgx.Tx, subject, state, ticket, challenge, product, consumer string, version storage.IdentityVersion, resourceClientID string) (matrixNativeTicket, error) {
	var out matrixNativeTicket
	var secret string
	tenant := tenancy.MustFrom(ctx).Slug
	err := tx.QueryRow(ctx, `SELECT login_secret,product,purpose,resource_client_id,resource_approved_at FROM matrix_native_consents
 WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND ticket_hash=$4 AND challenge=$5 AND identity_version=$6
 AND ($7='' OR product=$7) AND native_consumer=$8 AND expires_at>now() AND ticket_expires_at>now() AND login_secret<>''
 AND (($9='' AND purpose='') OR ($9<>'' AND purpose='resources:'||product AND resource_client_id=$9 AND resource_approved_at IS NOT NULL AND identity_version=1 AND native_consumer='')) FOR UPDATE`,
		tenant, subject, domain.HashToken(state), domain.HashToken(ticket), challenge, version, product, consumer, resourceClientID).Scan(&secret, &out.product, &out.purpose, &out.clientID, &out.approvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrForbidden
	}
	if err != nil {
		return out, err
	}
	if !s.cipher.Enabled() {
		return out, ErrNoCipher
	}
	out.token, err = s.openSecret(secret)
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `DELETE FROM matrix_native_consents WHERE tenant=$1 AND state_hash=$2`, tenant, domain.HashToken(state))
	return out, err
}
