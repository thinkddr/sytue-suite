// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
)

// La rotación de la clave de entrada de una época ya dada de alta (ADR 0087).
//
// Quien traspasa su correo de Matrix a la bóveda (o vacía su bóveda) ya no tiene
// la clave de entrada de su época: el correo nuevo seguiría cifrado para una clave
// perdida. La salida es un plan de generación siguiente, y solo entra si:
//
//   - lo autoriza una autoridad de BÓVEDA (Rooms; nunca Matrix ni calendario), y
//   - esa autoridad es posterior a la que autorizó el plan vigente: otra
//     identidad (de Matrix a la bóveda) o una generación mayor (un vaciado; la
//     generación de la bóveda se cuenta encima de la de Matrix y nunca baja). Un
//     plan de antes de guardar su autoridad solo lo sucede una de bóveda;
//   - el plan vigente está activo y el nuevo es exactamente el siguiente.
//
// El plan anterior no se revoca ni se borra: las entregas ya cifradas para él
// siguen en la cola, y la admisión cifra lo nuevo para el plan más alto activo.

// keyPlanAuthority es la autoridad que autorizó un plan (vacía si es de antes).
type keyPlanAuthority struct {
	generation int64
	revoked    bool
	user       *string
	authority  *int64
}

func latestKeyPlanAuthority(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig) (keyPlanAuthority, bool, error) {
	var out keyPlanAuthority
	var revoked *time.Time
	err := tx.QueryRow(ctx, `SELECT key_generation,revoked_at,authority_user,authority_generation FROM private_ingress_key_plans WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 ORDER BY key_generation DESC LIMIT 1 FOR UPDATE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch).Scan(&out.generation, &revoked, &out.user, &out.authority)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	out.revoked = revoked != nil
	return out, true, nil
}

// rotationAllowed es la regla de arriba, sin base de datos.
func rotationAllowed(scope storage.Scope, current keyPlanAuthority, next int64, authority matrixproof.Authority) bool {
	if current.revoked || next != current.generation+1 || authority.RequireActive(scope, authority.Generation) != nil || !vaultproof.IsAuthority(authority) {
		return false
	}
	if current.user == nil || current.authority == nil {
		return true
	}
	return *current.user != authority.UserID || *current.authority < authority.Generation
}

// EnrollAuthorizedPrivateIngress es el alta de una clave de entrada autorizada
// por Rooms: guarda con el plan la autoridad que lo autorizó, y un plan de
// generación siguiente solo entra por la regla de rotación. Lo demás es
// EnrollPrivateIngress tal cual (mismo plan otra vez, o la primera generación).
func EnrollAuthorizedPrivateIngress(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, plan ingress.KeyPlan, target storage.PublicationTarget, authorization matrixproof.Authorization) error {
	authority := authorization.Authority
	if plan.Validate(cfg.Scope, cfg.Epoch.String()) != nil || agecipher.ValidateKeyPlan(plan) != nil || authority.RequireActive(cfg.Scope, authority.Generation) != nil {
		return ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return err
	}
	current, found, err := latestKeyPlanAuthority(ctx, tx, cfg)
	if err != nil {
		return err
	}
	if found && plan.Generation > current.generation && !rotationAllowed(cfg.Scope, current, plan.Generation, authority) {
		return ingress.ErrRevoked
	}
	if !found || plan.Generation > current.generation {
		if _, err := tx.Exec(ctx, `INSERT INTO private_ingress_key_plans(tenant,owner_sub,product,epoch,key_generation,key_id,profile,public_recipient,authority_user,authority_generation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(tenant,owner_sub,product,epoch,key_generation) DO NOTHING`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, plan.Generation, plan.KeyID, plan.Profile, plan.Recipient, authority.UserID, authority.Generation); err != nil {
			return privateIngressEnrollmentError(err)
		}
	}
	return EnrollPrivateIngress(ctx, tx, cfg, plan, target, authorization.SigningKey)
}
