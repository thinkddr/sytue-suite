// SPDX-License-Identifier: Apache-2.0
// Package matrixproof verifies public Matrix device authorization with the
// maintained SDK. It never generates, retains or receives private signing keys.
package matrixproof

import (
	"context"
	"slices"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Origin is resolved independently by the server from its configured Matrix
// origin and the isolated user ID derived from Scope. It must not be decoded
// from a caller's request. Account and device activity are queried separately
// from /keys/query: an old signed key is not proof of a live device.
type Origin struct {
	Scope         storage.Scope
	ServerName    string
	UserID        id.UserID
	DeviceID      id.DeviceID
	AccountActive bool
	DeviceActive  bool
	Keys          mautrix.RespQueryKeys
}

// Device is constructible only by verification of the independently fetched
// chain against an already enrolled root. Enrollment/rotation must resolve the
// root through durable authority; Verify never trusts or pins a request's key.
type Device struct {
	scope      storage.Scope
	user       id.UserID
	device     id.DeviceID
	root       id.Ed25519
	signing    id.Ed25519
	encryption id.Curve25519
	evidence   *DeviceEvidence
}

func crossKey(value mautrix.CrossSigningKeys, user id.UserID, usage id.CrossSigningUsage) (id.Ed25519, error) {
	if value.UserID != user || len(value.Usage) != 1 || value.Usage[0] != usage || len(value.Keys) != 1 {
		return "", ErrUnverified
	}
	for name, key := range value.Keys {
		if !validKey(key.String()) || name != id.NewKeyID(id.KeyAlgorithmEd25519, key.String()) {
			return "", ErrUnverified
		}
		return key, nil
	}
	return "", ErrUnverified
}

func expectedOrigin(origin Origin) bool {
	if !origin.Scope.Valid() || origin.ServerName == "" || strings.ContainsAny(origin.ServerName, "/@?#\\ \x00\r\n\t") || origin.DeviceID == "" || !origin.AccountActive || !origin.DeviceActive || len(origin.Keys.Failures) != 0 {
		return false
	}
	return origin.UserID == id.UserID(storage.IsolatedMatrixUserPrefix(origin.Scope)+origin.ServerName)
}

// Verify requires master -> self-signing -> device, the device's own signature,
// both supported encryption algorithms and the enrolled public master. A
// successful result authorizes no domain action by itself: that action still
// needs this device's signature, a fresh one-use challenge and exact bindings.
func Verify(ctx context.Context, origin Origin, pinnedRoot id.Ed25519) (Device, error) {
	if err := ctx.Err(); err != nil {
		return Device{}, err
	}
	if pinnedRoot == "" {
		return Device{}, ErrRootEnrollment
	}
	if !expectedOrigin(origin) {
		return Device{}, ErrUnverified
	}
	return verifyUser(ctx, origin, origin.UserID, pinnedRoot)
}

// VerifyUser uses a human Matrix identity independently resolved by the trusted
// session provider. expectedUser must not come from the signed request itself.
// This does not grant access to a resource: its signed capability, generation
// and current ACL still have to be checked by the resource authority.
func VerifyUser(ctx context.Context, origin Origin, expectedUser id.UserID, pinnedRoot id.Ed25519) (Device, error) {
	return verifyUser(ctx, origin, expectedUser, pinnedRoot)
}

func verifyUser(ctx context.Context, origin Origin, expectedUser id.UserID, pinnedRoot id.Ed25519) (Device, error) {
	if err := ctx.Err(); err != nil {
		return Device{}, err
	}
	if pinnedRoot == "" {
		return Device{}, ErrRootEnrollment
	}
	localpart, server, err := expectedUser.Parse()
	if err != nil || localpart == "" || server != origin.ServerName || origin.UserID != expectedUser || !origin.Scope.Valid() || origin.DeviceID == "" || strings.ContainsAny(origin.ServerName, "/@?#\\ \x00\r\n\t") || !origin.AccountActive || !origin.DeviceActive || len(origin.Keys.Failures) != 0 || !validKey(pinnedRoot.String()) {
		return Device{}, ErrUnverified
	}
	evidence, err := evidenceFromOrigin(origin)
	if err != nil {
		return Device{}, err
	}
	identity, err := verifyDeviceEvidence(ctx, evidence, pinnedRoot)
	if err != nil {
		return Device{}, err
	}
	return Device{scope: identity.Scope, user: identity.UserID, device: identity.DeviceID, root: identity.RootKey, signing: identity.SigningKey, encryption: identity.EncryptionKey, evidence: &evidence}, nil
}

func (d Device) SigningKey(scope storage.Scope, target storage.PublicationTarget) (string, error) {
	if d.scope != scope || !scope.Valid() || d.user == "" || d.root == "" || d.signing == "" || !target.Valid() || target.DeviceID != d.device.String() {
		return "", ErrUnverified
	}
	return d.signing.String(), nil
}

func deviceSigningKey(keys mautrix.DeviceKeys, user id.UserID, device id.DeviceID, selfKey id.Ed25519) (id.Ed25519, error) {
	if keys.UserID != user || keys.DeviceID != device || keys.Dehydrated || !slices.Contains(keys.Algorithms, id.AlgorithmMegolmV1) || !slices.Contains(keys.Algorithms, id.AlgorithmOlmV1) {
		return "", ErrUnverified
	}
	signing := keys.Keys.GetEd25519(device)
	if !validKey(signing.String()) || !validKey(keys.Keys.GetCurve25519(device).String()) {
		return "", ErrUnverified
	}
	if ok, err := signatures.VerifySignatureJSON(&keys, user, selfKey.String(), selfKey); err != nil || !ok {
		return "", ErrUnverified
	}
	if ok, err := signatures.VerifySignatureJSON(&keys, user, device.String(), signing); err != nil || !ok {
		return "", ErrUnverified
	}
	return signing, nil
}

// Identity contains only independently verified public signing metadata.
// A value copied from JSON is not a Device and cannot verify any signature.
type Identity struct {
	Scope         storage.Scope
	UserID        id.UserID
	DeviceID      id.DeviceID
	RootKey       id.Ed25519
	SigningKey    id.Ed25519
	EncryptionKey id.Curve25519
}

func (d Device) Identity() Identity {
	return Identity{Scope: d.scope, UserID: d.user, DeviceID: d.device, RootKey: d.root, SigningKey: d.signing, EncryptionKey: d.encryption}
}

// VerifyJSON checks a domain transcript with the maintained Matrix SDK after
// binding the independently verified account/product and producer device. It
// deliberately grants no role, recipient, freshness or publication authority.
func (d Device) VerifyJSON(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := d.SigningKey(scope, target); err != nil {
		return err
	}
	return verifyDeviceJSON(ctx, d.Identity(), scope, target, value)
}

// VerifyAccountJSON verifies a device-bound account enrollment before any
// resource room exists. It grants no scope, enrollment, role or freshness.
func (d Device) VerifyAccountJSON(ctx context.Context, scope storage.Scope, value any) error {
	return verifyAccountJSON(ctx, d.Identity(), scope, value)
}

func verifyDeviceJSON(ctx context.Context, identity Identity, scope storage.Scope, target storage.PublicationTarget, value any) error {
	if !target.Valid() || target.DeviceID != identity.DeviceID.String() {
		return ErrUnverified
	}
	return verifyAccountJSON(ctx, identity, scope, value)
}

func verifyAccountJSON(ctx context.Context, identity Identity, scope storage.Scope, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if identity.Scope != scope || !scope.Valid() || identity.UserID == "" || identity.DeviceID == "" || identity.RootKey == "" || identity.SigningKey == "" {
		return ErrUnverified
	}
	if ok, err := signatures.VerifySignatureJSON(value, identity.UserID, identity.DeviceID.String(), identity.SigningKey); err != nil || !ok {
		return ErrUnverified
	}
	return ctx.Err()
}
