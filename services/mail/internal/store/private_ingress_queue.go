// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"io"
	"reflect"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func (s *Store) PrivateIngressQueue(authority callaccess.PrivateIngressAuthority, objects objectstore.PrivateIngressObjects) pg.PrivateIngressQueue {
	return pg.PrivateIngressQueue{Transaction: func(ctx context.Context, scope storage.Scope, epoch, kind string, operation func(pgx.Tx, pg.AuthorizedPrivateIngressConfig) error) error {
		if authority == nil {
			return pg.ErrPrivateIngressMissing
		}
		if !mailIngressOwner(ctx, scope) || !storage.MigrationEpoch(epoch) {
			return pg.ErrPrivateStorageScope
		}
		parsed, err := uuid.Parse(epoch)
		if err != nil {
			return err
		}
		return s.privateTx(ctx, scope.Subject, func(tx pgx.Tx) error {
			if err := s.mailIngressEpoch(ctx, tx, scope, parsed, kind); err != nil {
				return err
			}
			cfg := privateInboundConfig(scope, parsed)
			cfg.VerifyReceipt = func(ctx context.Context, claim ingress.Claim, receipt ingress.Receipt) error {
				return authority.VerifyStorageReceipt(ctx, scope, claim.Target, receipt.Commit)
			}
			return operation(tx, pg.AuthorizedPrivateIngressConfig{PrivateIngressConfig: cfg, Authority: authority})
		})
	}, Body: func(ctx context.Context, scope storage.Scope, id string) (io.ReadCloser, error) {
		if objects == nil || !mailIngressOwner(ctx, scope) {
			return nil, pg.ErrPrivateIngressMissing
		}
		tenant, err := tenancy.From(ctx)
		if err != nil {
			return nil, err
		}
		key, err := objectstore.PrivateIngressBlobKey(scope, id)
		if err != nil {
			return nil, err
		}
		return objects.Get(ctx, tenant, key)
	}}
}

// EntradaPrivada dice si la entrada privada del correo está montada para un ámbito
// y la prepara (jmap.PrivateIngress). Es la misma cola que PrivateIngressQueue: la
// autoridad de Rooms que firma reclamaciones y recibos y el almacén de objetos de lo
// cifrado. Sin las dos, o para un ámbito que no es de quien pregunta, no está lista.
//
// Faltaba en producción: el servicio contestaba `private_ingress_ready: false` a
// todas las cuentas, el núcleo de cliente rechazaba `mail.capabilities` y Mail se
// quedaba en `prepare/error.Error` aun vaciando los datos.
type EntradaPrivada struct {
	autoridad callaccess.PrivateIngressAuthority
	objetos   objectstore.PrivateIngressObjects
}

// NuevaEntradaPrivada. Una autoridad o un almacén nil (también un puntero nil
// dentro de la interfaz) dejan la entrada sin montar.
func NuevaEntradaPrivada(autoridad callaccess.PrivateIngressAuthority, objetos objectstore.PrivateIngressObjects) EntradaPrivada {
	return EntradaPrivada{autoridad: autoridad, objetos: objetos}
}

func (e EntradaPrivada) PrivateIngressReady(ctx context.Context, scope storage.Scope) (bool, error) {
	return !nulo(e.autoridad) && !nulo(e.objetos) && mailIngressOwner(ctx, scope), nil
}

func (e EntradaPrivada) PreparePrivateIngress(ctx context.Context, scope storage.Scope) error {
	if lista, _ := e.PrivateIngressReady(ctx, scope); !lista {
		return pg.ErrPrivateIngressMissing
	}
	return nil
}

// nulo: nil, o un puntero nil envuelto en la interfaz (`callaccess.New` devuelve
// *Client nil si falta la URL o el secreto de Rooms).
func nulo(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	return r.Kind() == reflect.Pointer && r.IsNil()
}

func mailIngressOwner(ctx context.Context, scope storage.Scope) bool {
	claims, ok := authn.FromContext(ctx)
	tenant, err := tenancy.From(ctx)
	return ok && err == nil && !claims.Delegado && scope.Valid() && scope.Product == "mail" && scope.Tenant == tenant.Slug && claims.Tenant == scope.Tenant && claims.Subject == scope.Subject
}

func (s *Store) mailIngressEpoch(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, kind string) error {
	var account uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE tenant=$1 AND subject=$2 AND kind='user'`, scope.Tenant, scope.Subject).Scan(&account); err != nil {
		return err
	}
	if err := pg.EnsurePrivateStorageState(ctx, tx, "mail_private_storage_states", scope.Tenant, scope.Subject, &account); err != nil {
		return err
	}
	var mode string
	var current *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT mode,epoch FROM mail_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND account_id=$3 FOR SHARE`, scope.Tenant, scope.Subject, account).Scan(&mode, &current); err != nil {
		return err
	}
	if (kind == "key.enroll" || kind == "enrollment") && mode == "legacy" {
		return nil
	}
	if (mode != "frozen" && mode != "private") || current == nil || *current != epoch {
		return ErrPrivateStorageEpoch
	}
	return nil
}
