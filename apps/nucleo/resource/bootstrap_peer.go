// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// BootstrapPeerQuery names no pin, root, origin URL or caller-selected owner.
// The current authenticated creator must verify both signed statements against
// the ORIGINAL frozen source before any peer registry lookup is authorized.
type BootstrapPeerQuery struct {
	Plan     SignedBootstrapPlan           `json:"plan"`
	Manifest SignedBootstrapRosterManifest `json:"manifest"`
	Scope    storage.Scope                 `json:"scope"`
	DeviceID string                        `json:"device_id"`
}

func (q BootstrapPeerQuery) Valid() bool {
	return q.Plan.Valid() && q.Manifest.ValidFor(q.Plan.BootstrapPlan) && q.Scope.Valid() && q.Scope.Tenant == q.Plan.Source.Resource.Tenant && q.Scope.Product == q.Plan.Source.Resource.Product && ValidDeviceID(q.DeviceID) && len(q.Plan.Signatures) != 0 && len(q.Manifest.Signatures) != 0
}

// BootstrapRosterPeer validates the ENTIRE immutable signed stream before
// returning one public leaf. An arbitrary page or SQL received_count is not an
// inclusion proof. This operation grants no room, epoch key or publication.
func BootstrapRosterPeer(ctx context.Context, pending VerifiedBootstrapRosterManifest, pages BootstrapRosterPageSource, scope storage.Scope, device string) (BootstrapRosterInput, error) {
	if err := pending.Revalidate(ctx); err != nil {
		return BootstrapRosterInput{}, err
	}
	selected, err := selectBootstrapRosterPeer(ctx, pending.Plan(), pending.Value(), pages, scope, device)
	if err != nil {
		return BootstrapRosterInput{}, err
	}
	if err := pending.Revalidate(ctx); err != nil {
		return BootstrapRosterInput{}, err
	}
	return selected, nil
}

func fetchBootstrapRosterPage(ctx context.Context, key Resource, pages BootstrapRosterPageSource, offset, count int64) (BootstrapRosterPage, error) {
	if err := ctx.Err(); err != nil {
		return BootstrapRosterPage{}, err
	}
	position, _ := Sequence(offset)
	page, err := pages.BootstrapRosterPage(ctx, position)
	if err != nil {
		return BootstrapRosterPage{}, err
	}
	if page.Offset != position || int64(len(page.Inputs)) > count-offset {
		return BootstrapRosterPage{}, ErrInvalid
	}
	if _, err := BootstrapRosterPageBytes(key, page); err != nil {
		return BootstrapRosterPage{}, err
	}
	return page, nil
}

func selectBootstrapRosterPeer(ctx context.Context, plan BootstrapPlan, manifest BootstrapRosterManifest, pages BootstrapRosterPageSource, scope storage.Scope, device string) (BootstrapRosterInput, error) {
	var selected BootstrapRosterInput
	if pages == nil || !scope.Valid() || !ValidDeviceID(device) || scope.Tenant != plan.Source.Resource.Tenant || scope.Product != plan.Source.Resource.Product {
		return selected, ErrInvalid
	}
	stream, err := NewBootstrapRosterStream(plan.Source.Resource)
	if err != nil {
		return selected, err
	}
	count, _ := manifest.MemberCount.Int64()
	var offset int64
	found := false
	for offset < count {
		page, err := fetchBootstrapRosterPage(ctx, plan.Source.Resource, pages, offset, count)
		if err != nil {
			return BootstrapRosterInput{}, err
		}
		for _, input := range page.Inputs {
			if err := stream.Add(input); err != nil {
				return BootstrapRosterInput{}, err
			}
			if input.Member.Scope == scope && input.Recipient.Issuer.DeviceID == device {
				if found {
					return BootstrapRosterInput{}, ErrInvalid
				}
				selected, found = input, true
			}
			offset++
		}
	}
	n, sha, err := stream.Finish()
	if err != nil || n != manifest.MemberCount || sha != manifest.OrderedInputsSHA256 || stream.MembershipSHA256() != plan.MembershipSHA256 {
		return BootstrapRosterInput{}, ErrInvalid
	}
	if !found {
		return BootstrapRosterInput{}, ErrRevoked
	}
	return selected, nil
}

// CheckBootstrapPeerSession binds every leaf field to an independently resolved
// CURRENT enrollment. No caller-provided root/pin can create a VerifiedSession.
func checkBootstrapPeerSnapshot(input BootstrapRosterInput, current VerifiedSession) error {
	if !current.valid() || input.Member.Scope != current.Scope() || input.Recipient.Issuer != current.Actor() || input.Member.EnrollmentSHA256 != current.Recipient().SHA256() || input.Member.EnrollmentSHA256 != current.Binding().EnrollmentSHA256 || !memberMatchesEnrollment(input.Member, current.Recipient().Value()) {
		return ErrRevoked
	}
	return nil
}

func CheckBootstrapPeerSession(ctx context.Context, input BootstrapRosterInput, current VerifiedSession) error {
	if err := checkBootstrapPeerSnapshot(input, current); err != nil {
		return err
	}
	return current.Revalidate(ctx)
}
