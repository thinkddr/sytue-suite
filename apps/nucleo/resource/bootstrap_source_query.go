// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BootstrapSourceQuery requests only ORIGINAL public migration metadata. It
// binds the actual SDK actor and consumed product session to one resource and
// room. It is neither an owner-vault capability nor a key/recipient grant.
type BootstrapSourceQuery struct {
	Version              int                       `json:"v"`
	Domain               string                    `json:"domain"`
	Resource             Resource                  `json:"resource"`
	OwnerScope           storage.Scope             `json:"owner_scope"`
	Target               storage.PublicationTarget `json:"target"`
	Actor                Actor                     `json:"actor"`
	SessionBindingSHA256 string                    `json:"session_binding_sha256"`
}

func (q BootstrapSourceQuery) Valid() bool {
	return q.Version == 1 && q.Domain == "org.sytue.resource.bootstrap.source.v1" && q.Resource.Valid() && q.OwnerScope.Valid() && q.Resource.Tenant == q.OwnerScope.Tenant && q.Resource.Product == q.OwnerScope.Product && matrixID(q.Target.RoomID, '!') && q.Target.DeviceID == q.Actor.DeviceID && q.Actor.Valid() && digest(q.SessionBindingSHA256)
}

type SignedBootstrapSourceQuery struct {
	BootstrapSourceQuery
	Signatures signatures.Signatures `json:"signatures"`
}
type VerifiedBootstrapSourceQuery struct {
	value   BootstrapSourceQuery
	session VerifiedSession
	raw     []byte
}

func PrepareBootstrapSourceQuery(ctx context.Context, current VerifiedSession, key Resource, room string) (BootstrapSourceQuery, error) {
	if !current.valid() {
		return BootstrapSourceQuery{}, ErrInvalid
	}
	hash, err := matrixproof.RequestDigest(current.Binding())
	if err != nil {
		return BootstrapSourceQuery{}, err
	}
	q := BootstrapSourceQuery{Version: 1, Domain: "org.sytue.resource.bootstrap.source.v1", Resource: key, OwnerScope: current.Scope(), Target: storage.PublicationTarget{RoomID: room, DeviceID: current.Actor().DeviceID}, Actor: current.Actor(), SessionBindingSHA256: hash}
	if !q.Valid() {
		return BootstrapSourceQuery{}, ErrInvalid
	}
	if err := current.Revalidate(ctx); err != nil {
		return BootstrapSourceQuery{}, err
	}
	return q, nil
}

func VerifyBootstrapSourceQuery(ctx context.Context, signed SignedBootstrapSourceQuery, current VerifiedSession) (VerifiedBootstrapSourceQuery, error) {
	q := signed.BootstrapSourceQuery
	if !q.Valid() || !current.valid() || q.OwnerScope != current.Scope() || q.Actor != current.Actor() {
		return VerifiedBootstrapSourceQuery{}, ErrInvalid
	}
	hash, err := matrixproof.RequestDigest(current.Binding())
	if err != nil || hash != q.SessionBindingSHA256 {
		return VerifiedBootstrapSourceQuery{}, ErrInvalid
	}
	if err := current.Revalidate(ctx); err != nil {
		return VerifiedBootstrapSourceQuery{}, err
	}
	if err := current.Device().VerifyJSON(ctx, q.OwnerScope, q.Target, signed); err != nil {
		return VerifiedBootstrapSourceQuery{}, err
	}
	raw, err := matrixproof.CanonicalBytes(signed)
	if err != nil {
		return VerifiedBootstrapSourceQuery{}, err
	}
	out := VerifiedBootstrapSourceQuery{value: q, session: current, raw: raw}
	if err := out.Revalidate(ctx); err != nil {
		return VerifiedBootstrapSourceQuery{}, err
	}
	return out, nil
}
func (q VerifiedBootstrapSourceQuery) Value() BootstrapSourceQuery    { return q.value }
func (q VerifiedBootstrapSourceQuery) PublicQueryBytes() []byte       { return bytes.Clone(q.raw) }
func (q VerifiedBootstrapSourceQuery) SessionBinding() SessionBinding { return q.session.Binding() }
func (q VerifiedBootstrapSourceQuery) Revalidate(ctx context.Context) error {
	if !q.value.Valid() || !q.session.valid() || len(q.raw) == 0 || q.value.OwnerScope != q.session.Scope() || q.value.Actor != q.session.Actor() {
		return ErrRevoked
	}
	return q.session.Revalidate(ctx)
}
