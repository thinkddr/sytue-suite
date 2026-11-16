// SPDX-License-Identifier: Apache-2.0
package ingress

// ValidateProof binds an authenticated Rooms response to the actual fresh
// signature. A read may use a resolve signature; the retained fence still
// commits to the canonical begin intent, including the original B token.
func (f DeviceFence) ValidateProof(proof ControlProof) error {
	if err := proof.Validate(proof.Request.Scope, false); err != nil {
		return err
	}
	if proof.Request.Kind != "task.handoff" || proof.Request.Handoff == nil {
		return ErrContract
	}
	if f.Authority.Generation != proof.Proof.Generation || f.Authority.RootKey != proof.Proof.RootKey {
		return ErrRevoked
	}
	return f.Validate(proof.Request, f.Authority)
}
