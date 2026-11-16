// SPDX-License-Identifier: Apache-2.0
package matrixproof

import (
	"context"
	"encoding/json"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// DeviceEvidence contains only the public SDK chain captured after live device
// verification. Its JSON representation is evidence, never current authority.
// A historical consumer must obtain its independent pin from a trusted hold or
// previously verified cutover, not from this evidence or the caller's request.
type DeviceEvidence struct {
	Version    int                      `json:"v"`
	Scope      storage.Scope            `json:"scope"`
	ServerName string                   `json:"server_name"`
	UserID     id.UserID                `json:"user_id"`
	DeviceID   id.DeviceID              `json:"device_id"`
	Master     mautrix.CrossSigningKeys `json:"master"`
	Self       mautrix.CrossSigningKeys `json:"self"`
	Device     mautrix.DeviceKeys       `json:"device"`
}

func cloneEvidence(e DeviceEvidence) (DeviceEvidence, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return DeviceEvidence{}, ErrUnverified
	}
	var copied DeviceEvidence
	if err = json.Unmarshal(raw, &copied); err != nil {
		return DeviceEvidence{}, ErrUnverified
	}
	return copied, nil
}

func evidenceFromOrigin(origin Origin) (DeviceEvidence, error) {
	master, masterOK := origin.Keys.MasterKeys[origin.UserID]
	self, selfOK := origin.Keys.SelfSigningKeys[origin.UserID]
	device, deviceOK := origin.Keys.DeviceKeys[origin.UserID][origin.DeviceID]
	if !masterOK || !selfOK || !deviceOK {
		return DeviceEvidence{}, ErrUnverified
	}
	return cloneEvidence(DeviceEvidence{Version: 1, Scope: origin.Scope, ServerName: origin.ServerName, UserID: origin.UserID, DeviceID: origin.DeviceID, Master: master, Self: self, Device: device})
}

// PublicEvidence returns an independent copy of the chain verified while this
// device was active. Later mutations of a query response cannot change it.
func (d Device) PublicEvidence() (DeviceEvidence, error) {
	if d.evidence == nil {
		return DeviceEvidence{}, ErrUnverified
	}
	return cloneEvidence(*d.evidence)
}

// verifyDeviceEvidence is the single maintained SDK chain verifier for both live
// and historical devices. Liveness is checked only by the live entry point.
func verifyDeviceEvidence(ctx context.Context, e DeviceEvidence, pinnedRoot id.Ed25519) (Identity, error) {
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	localpart, server, err := e.UserID.Parse()
	if e.Version != 1 || err != nil || localpart == "" || server != e.ServerName || !e.Scope.Valid() || e.DeviceID == "" || strings.ContainsAny(e.ServerName, "/@?#\\ \x00\r\n\t") || !validKey(pinnedRoot.String()) {
		return Identity{}, ErrUnverified
	}
	root, err := crossKey(e.Master, e.UserID, id.XSUsageMaster)
	if err != nil || root != pinnedRoot {
		return Identity{}, ErrUnverified
	}
	self, err := crossKey(e.Self, e.UserID, id.XSUsageSelfSigning)
	if err != nil {
		return Identity{}, err
	}
	if ok, err := signatures.VerifySignatureJSON(e.Self, e.UserID, root.String(), root); err != nil || !ok {
		return Identity{}, ErrUnverified
	}
	signing, err := deviceSigningKey(e.Device, e.UserID, e.DeviceID, self)
	if err != nil {
		return Identity{}, err
	}
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	return Identity{Scope: e.Scope, UserID: e.UserID, DeviceID: e.DeviceID, RootKey: root, SigningKey: signing, EncryptionKey: e.Device.Keys.GetCurve25519(e.DeviceID)}, nil
}

// HistoricalDevice verifies only an already accepted historical transcript. It
// deliberately has no SigningKey, VerifyAction or current publication methods.
type HistoricalDevice struct {
	identity Identity
	evidence *DeviceEvidence
}

// VerifyHistoricalDevice verifies the public chain against all six independently
// retained pin fields. It never restores account/device activity or creates a
// live Device, and cannot authorize a new publication.
func VerifyHistoricalDevice(ctx context.Context, evidence DeviceEvidence, pin Identity) (HistoricalDevice, error) {
	if err := ctx.Err(); err != nil {
		return HistoricalDevice{}, err
	}
	if pin.RootKey == "" {
		return HistoricalDevice{}, ErrRootEnrollment
	}
	snapshot, err := cloneEvidence(evidence)
	if err != nil {
		return HistoricalDevice{}, err
	}
	identity, err := verifyDeviceEvidence(ctx, snapshot, pin.RootKey)
	if err != nil {
		return HistoricalDevice{}, err
	}
	if identity != pin {
		return HistoricalDevice{}, ErrUnverified
	}
	return HistoricalDevice{identity: identity, evidence: &snapshot}, nil
}

// PublicEvidence returns only an independent copy of the verified public chain.
// It never recreates account/device activity, a signing key or a live Device.
func (d HistoricalDevice) PublicEvidence() (DeviceEvidence, error) {
	if d.evidence == nil {
		return DeviceEvidence{}, ErrUnverified
	}
	return cloneEvidence(*d.evidence)
}

func (d HistoricalDevice) Identity() Identity { return d.identity }
func (d HistoricalDevice) VerifyJSON(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, value any) error {
	return verifyDeviceJSON(ctx, d.identity, scope, target, value)
}

// VerifyAccountJSON verifies only a retained historical account transcript. It
// cannot turn an old enrollment into a current product/device session.
func (d HistoricalDevice) VerifyAccountJSON(ctx context.Context, scope storage.Scope, value any) error {
	return verifyAccountJSON(ctx, d.identity, scope, value)
}
