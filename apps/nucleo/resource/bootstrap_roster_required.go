// SPDX-License-Identifier: Apache-2.0
package resource

import "context"

// RequireCurrentRosterCapture is the activation boundary for a complete current
// capture. The historical/owner-only VerifyBootstrapPlan API remains available
// for proof restoration, but cannot supply a live room/key/publication binding.
// Every member must match the actual private VerifiedSession retained by the
// shared completion verifier; client JSON and public binding counts are useless.
func (p VerifiedBootstrapPlan) RequireCurrentRosterCapture(ctx context.Context) error {
	if !p.valid() || len(p.currentRecipients) == 0 || len(p.currentRecipients) != len(p.roster.members) {
		return ErrRevoked
	}
	for i, member := range p.roster.members {
		current := p.currentRecipients[i]
		enrolled, ok := p.roster.enrollments[member.EnrollmentSHA256]
		if !ok || !current.valid() || current.Scope() != member.Scope || current.Actor() != enrolled.Issuer || current.Recipient().SHA256() != member.EnrollmentSHA256 || current.Binding().EnrollmentSHA256 != member.EnrollmentSHA256 || !memberMatchesEnrollment(member, current.Recipient().Value()) {
			return ErrRevoked
		}
	}
	return p.Revalidate(ctx)
}
