// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"
	"encoding/json"
	"slices"
	"sort"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// RecipientEnrollment is a product-scoped public hybrid key, approved by an
// independently verified SDK device. This signature proves who enrolled a key;
// it NEVER adds the recipient to a resource or grants it an epoch by itself.
type RecipientEnrollment struct {
	Version   int           `json:"v"`
	Domain    string        `json:"domain"`
	Scope     storage.Scope `json:"scope"`
	UserID    string        `json:"user_id"`
	Recipient string        `json:"recipient"`
	KeyID     string        `json:"key_id"`
	Issuer    Actor         `json:"issuer"`
}
type SignedRecipient struct {
	RecipientEnrollment
	Signatures signatures.Signatures `json:"signatures"`
}
type VerifiedRecipient struct {
	proof      json.RawMessage
	device     publicEvidenceVerifier
	enrollment RecipientEnrollment
	hash       string
}

func (r RecipientEnrollment) Valid() bool {
	recipient, err := pqc.ParseRecipient(pqc.HybridProfile, r.Recipient)
	if err != nil {
		return false
	}
	keyID, err := pqc.RecipientID(recipient)
	return r.Version == 1 && r.Domain == "org.sytue.resource.recipient.v1" && r.Scope.Valid() && matrixID(r.UserID, '@') && r.Issuer.Valid() && r.Issuer.UserID == r.UserID && err == nil && r.KeyID == keyID
}

func actorMatches(device matrixproof.Device, actor Actor, scope storage.Scope) bool {
	return identityMatches(device.Identity(), actor, scope)
}

func VerifyRecipient(ctx context.Context, value SignedRecipient, device matrixproof.Device, target storage.PublicationTarget) (VerifiedRecipient, error) {
	if !target.Valid() || target.DeviceID != value.Issuer.DeviceID {
		return VerifiedRecipient{}, ErrInvalid
	}
	return VerifyAccountRecipient(ctx, value, device)
}

// VerifyAccountRecipient is the same SDK enrollment verifier before the first
// resource room. It proves neither key possession nor consent: the retained
// hybrid challenge and current SessionProvider must establish both separately.
func VerifyAccountRecipient(ctx context.Context, value SignedRecipient, device matrixproof.Device) (VerifiedRecipient, error) {
	if !value.Valid() || !actorMatches(device, value.Issuer, value.Scope) {
		return VerifiedRecipient{}, ErrInvalid
	}
	if err := device.VerifyAccountJSON(ctx, value.Scope, value); err != nil {
		return VerifiedRecipient{}, err
	}
	hash, err := matrixproof.RequestDigest(value.RecipientEnrollment)
	if err != nil {
		return VerifiedRecipient{}, err
	}
	proof, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return VerifiedRecipient{}, err
	}
	return VerifiedRecipient{enrollment: value.RecipientEnrollment, hash: hash, proof: proof, device: device}, nil
}
func (r VerifiedRecipient) SHA256() string             { return r.hash }
func (r VerifiedRecipient) Value() RecipientEnrollment { return r.enrollment }
func (r VerifiedRecipient) valid() bool                { return digest(r.hash) && r.enrollment.Valid() }

type Member struct {
	Scope            storage.Scope `json:"scope"`
	UserID           string        `json:"user_id"`
	RootKey          string        `json:"root_key"`
	Recipient        string        `json:"recipient"`
	ProductKeyID     string        `json:"product_key_id"`
	EnrollmentSHA256 string        `json:"enrollment_sha256"`
	Role             string        `json:"role"`
	Actions          []Action      `json:"actions"`
	From             Position      `json:"from"`
}

func (m Member) valid(resource Resource) bool {
	recipient, err := pqc.ParseRecipient(pqc.HybridProfile, m.Recipient)
	if err != nil {
		return false
	}
	keyID, err := pqc.RecipientID(recipient)
	return resource.Valid() && m.Scope.Valid() && m.Scope.Tenant == resource.Tenant && m.Scope.Product == resource.Product && matrixID(m.UserID, '@') && key(m.RootKey) && err == nil && keyID == m.ProductKeyID && digest(m.EnrollmentSHA256) && storage.Identifier(m.Role) && actionsValid(m.Actions) && m.From.Valid()
}
func cloneMember(m Member) Member { m.Actions = slices.Clone(m.Actions); return m }
func memberOrder(a, b Member) bool {
	if a.Scope.Subject != b.Scope.Subject {
		return a.Scope.Subject < b.Scope.Subject
	}
	if a.UserID != b.UserID {
		return a.UserID < b.UserID
	}
	return a.ProductKeyID < b.ProductKeyID
}

func canonicalMembers(resource Resource, members []Member) ([]Member, error) {
	if !resource.Valid() || len(members) == 0 {
		return nil, ErrInvalid
	}
	out := make([]Member, len(members))
	for i, m := range members {
		if !m.valid(resource) {
			return nil, ErrInvalid
		}
		out[i] = cloneMember(m)
	}
	sort.Slice(out, func(i, j int) bool { return memberOrder(out[i], out[j]) })
	for i := 1; i < len(out); i++ {
		if !memberOrder(out[i-1], out[i]) {
			return nil, ErrInvalid
		}
	}
	return out, nil
}

func MembershipDigest(resource Resource, members []Member) (string, error) {
	ordered, err := canonicalMembers(resource, members)
	if err != nil {
		return "", err
	}
	return matrixproof.RequestDigest(struct {
		Domain   string   `json:"domain"`
		Resource Resource `json:"resource"`
		Members  []Member `json:"members"`
	}{Domain: "org.sytue.resource.membership.v1", Resource: resource, Members: ordered})
}

type VerifiedRoster struct {
	record      json.RawMessage
	recordHash  string
	resource    Resource
	members     []Member
	enrollments map[string]RecipientEnrollment
	recipients  map[string]VerifiedRecipient
	hash        string
}

func VerifyRoster(resource Resource, members []Member, recipients []VerifiedRecipient) (VerifiedRoster, error) {
	ordered, err := canonicalMembers(resource, members)
	if err != nil {
		return VerifiedRoster{}, err
	}
	enrollments := map[string]RecipientEnrollment{}
	recipientProofs := map[string]VerifiedRecipient{}
	for _, verified := range recipients {
		if !verified.valid() {
			return VerifiedRoster{}, ErrInvalid
		}
		enrollments[verified.hash] = verified.enrollment
		recipientProofs[verified.hash] = verified
	}
	for _, member := range ordered {
		enrolled, ok := enrollments[member.EnrollmentSHA256]
		if !ok || !memberMatchesEnrollment(member, enrolled) {
			return VerifiedRoster{}, ErrInvalid
		}
	}
	hash, err := MembershipDigest(resource, ordered)
	if err != nil {
		return VerifiedRoster{}, err
	}
	record, err := matrixproof.CanonicalBytes(ordered)
	if err != nil {
		return VerifiedRoster{}, err
	}
	recordHash, err := CipherJSONDigest(record)
	if err != nil {
		return VerifiedRoster{}, err
	}
	return VerifiedRoster{resource: resource, members: ordered, enrollments: enrollments, recipients: recipientProofs, hash: hash, record: record, recordHash: recordHash}, nil
}
func (r VerifiedRoster) SHA256() string { return r.hash }
func (r VerifiedRoster) Members() []Member {
	out := make([]Member, len(r.members))
	for i, m := range r.members {
		out[i] = cloneMember(m)
	}
	return out
}

func (r VerifiedRoster) valid() bool {
	return r.resource.Valid() && digest(r.hash) && len(r.members) > 0
}

func (r VerifiedRoster) issuer(scope storage.Scope, actor Actor) bool {
	for _, member := range r.members {
		if member.Scope == scope && member.UserID == actor.UserID && member.RootKey == actor.RootKey && r.enrollments[member.EnrollmentSHA256].Issuer == actor && slices.Contains(member.Actions, Manage) {
			return true
		}
	}
	return false
}

// Anchor is retained by the trusted client/broker after an explicit resource
// creation or verified migration. It must never be reconstructed from a new
// server response. Genesis permits the exact verified legacy baseline sequence.
type Anchor struct {
	Resource     Resource
	PolicySHA256 string
	OwnerScope   storage.Scope
	Owner        Actor
	Baseline     Position
}
type VerifiedEpoch struct {
	proof  json.RawMessage
	device publicEvidenceVerifier
	parent *VerifiedEpoch
	// Restored metadata always retains CURRENT B and the ORIGINAL source fence.
	// It must never be usable through the legacy device-A Authorize path.
	restored *restoredBootstrapAuthority
	value    Epoch
	roster   VerifiedRoster
	hash     string
}

func (e VerifiedEpoch) valid() bool {
	return e.value.Valid() && e.roster.valid() && e.value.Resource == e.roster.resource && e.value.MembershipSHA256 == e.roster.hash && digest(e.hash)
}
func (e VerifiedEpoch) Value() Epoch     { return e.value }
func (e VerifiedEpoch) SHA256() string   { return e.hash }
func (e VerifiedEpoch) Roster() []Member { return e.roster.Members() }
func EpochDigest(epoch Epoch) (string, error) {
	if !epoch.Valid() {
		return "", ErrInvalid
	}
	return matrixproof.RequestDigest(epoch)
}

func verifyEpochSignature(ctx context.Context, value SignedEpoch, roster VerifiedRoster, device matrixproof.Device) error {
	scope := device.Identity().Scope
	if !value.Valid() || !roster.valid() || value.Resource != roster.resource || value.MembershipSHA256 != roster.hash || scope.Tenant != value.Resource.Tenant || scope.Product != value.Resource.Product || !actorMatches(device, value.Issuer, scope) {
		return ErrInvalid
	}
	return device.VerifyJSON(ctx, scope, storage.PublicationTarget{RoomID: value.RoomID, DeviceID: value.Issuer.DeviceID}, value)
}

func VerifyInitialEpoch(ctx context.Context, value SignedEpoch, roster VerifiedRoster, device matrixproof.Device, anchor Anchor) (VerifiedEpoch, error) {
	if err := verifyEpochSignature(ctx, value, roster, device); err != nil {
		return VerifiedEpoch{}, err
	}
	if err := ValidateInitialBaseline(value.Epoch, anchor); err != nil {
		return VerifiedEpoch{}, err
	}
	if device.Identity().Scope != anchor.OwnerScope || !roster.issuer(anchor.OwnerScope, anchor.Owner) {
		return VerifiedEpoch{}, ErrInvalid
	}
	hash := anchor.PolicySHA256
	proof, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return VerifiedEpoch{}, err
	}
	return VerifiedEpoch{value: value.Epoch, roster: roster, hash: hash, proof: proof, device: device}, nil
}

func VerifyNextEpoch(ctx context.Context, value SignedEpoch, roster VerifiedRoster, device matrixproof.Device, previous VerifiedEpoch, manager VerifiedCapability, cutover Position) (VerifiedEpoch, error) {
	return verifyNextEpoch(ctx, value, roster, device, previous, manager, cutover, nil)
}

func validNextEpochLink(value SignedEpoch, previous VerifiedEpoch, manager VerifiedCapability, cutover Position) bool {
	if !previous.valid() || !manager.valid() || manager.epochHash != previous.hash || !manager.value.Allows(Manage) || !cutover.Valid() || value.PreviousEpoch != previous.value.ID || value.PreviousPolicySHA256 != previous.hash || value.Resource != previous.value.Resource || value.RoomID != previous.value.RoomID || value.Cutover != cutover {
		return false
	}
	generation, err := previous.value.Generation.Next(1)
	if err != nil || value.Generation != generation {
		return false
	}
	oldSeq, _ := previous.value.Cutover.Seq.Int64()
	newSeq, _ := cutover.Seq.Int64()
	return newSeq >= oldSeq
}

func verifyNextEpoch(ctx context.Context, value SignedEpoch, roster VerifiedRoster, device matrixproof.Device, previous VerifiedEpoch, manager VerifiedCapability, cutover Position, current *VerifiedSession) (VerifiedEpoch, error) {
	if previous.restored != nil && current == nil {
		return VerifiedEpoch{}, ErrRevoked
	}
	if !validNextEpochLink(value, previous, manager, cutover) {
		return VerifiedEpoch{}, ErrInvalid
	}
	if err := ValidateBaselineTransition(previous.value, value.Epoch); err != nil {
		return VerifiedEpoch{}, err
	}
	if err := ValidateCheckpointContext(value.Epoch, device.Identity().Scope, value.Issuer); err != nil {
		return VerifiedEpoch{}, err
	}
	scope := device.Identity().Scope
	actor := manager.enrollment.Issuer
	if current != nil {
		access, err := AuthorizeSession(ctx, previous, manager, *current, Manage)
		if err != nil {
			return VerifiedEpoch{}, err
		}
		actor = access.Actor()
	}
	if scope != manager.member.Scope || value.Issuer != actor {
		return VerifiedEpoch{}, ErrInvalid
	}
	if err := verifyEpochSignature(ctx, value, roster, device); err != nil {
		return VerifiedEpoch{}, err
	}
	hash, err := EpochDigest(value.Epoch)
	if err != nil {
		return VerifiedEpoch{}, err
	}
	proof, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return VerifiedEpoch{}, err
	}
	return VerifiedEpoch{value: value.Epoch, roster: roster, hash: hash, parent: &previous, proof: proof, device: device}, nil
}

type VerifiedCapability struct {
	proof      json.RawMessage
	device     publicEvidenceVerifier
	value      Capability
	member     Member
	enrollment RecipientEnrollment
	epochHash  string
	hash       string
}

func (c VerifiedCapability) valid() bool {
	return c.value.Valid() && c.member.valid(c.value.Resource) && digest(c.epochHash) && digest(c.hash)
}

func (c VerifiedCapability) Value() Capability {
	value := c.value
	value.Actions = slices.Clone(value.Actions)
	return value
}
func (c VerifiedCapability) SHA256() string                  { return c.hash }
func (c VerifiedCapability) Enrollment() RecipientEnrollment { return c.enrollment }
func (c VerifiedCapability) Member() Member                  { return cloneMember(c.member) }
func CapabilityDigest(capability Capability) (string, error) {
	if !capability.Valid() {
		return "", ErrInvalid
	}
	return matrixproof.RequestDigest(capability)
}

func VerifyCapability(ctx context.Context, value SignedCapability, epoch VerifiedEpoch, device matrixproof.Device) (VerifiedCapability, error) {
	if !epoch.valid() {
		return VerifiedCapability{}, ErrInvalid
	}
	selected, err := selectCapabilityMember(value.Capability, epoch.value, epoch.hash, epoch.roster.members)
	if err != nil {
		return VerifiedCapability{}, err
	}
	scope := device.Identity().Scope
	if !actorMatches(device, value.Issuer, scope) || !epoch.roster.issuer(scope, value.Issuer) {
		return VerifiedCapability{}, ErrInvalid
	}
	if err := device.VerifyJSON(ctx, scope, storage.PublicationTarget{RoomID: value.RoomID, DeviceID: value.Issuer.DeviceID}, value); err != nil {
		return VerifiedCapability{}, err
	}
	hash, err := CapabilityDigest(value.Capability)
	if err != nil {
		return VerifiedCapability{}, err
	}
	copied := value.Capability
	copied.Actions = slices.Clone(value.Actions)
	proof, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return VerifiedCapability{}, err
	}
	return VerifiedCapability{value: copied, member: cloneMember(selected), enrollment: epoch.roster.enrollments[selected.EnrollmentSHA256], epochHash: epoch.hash, hash: hash, proof: proof, device: device}, nil
}

type VerifiedAccess struct {
	session    *VerifiedSession
	epoch      VerifiedEpoch
	capability VerifiedCapability
	action     Action
}

func Authorize(epoch VerifiedEpoch, capability VerifiedCapability, subject string, action Action) (VerifiedAccess, error) {
	if epoch.restored != nil {
		return VerifiedAccess{}, ErrRevoked
	}
	return authorizeRole(epoch, capability, subject, action)
}

func authorizeRole(epoch VerifiedEpoch, capability VerifiedCapability, subject string, action Action) (VerifiedAccess, error) {
	if !epoch.valid() || !capability.valid() || capability.epochHash != epoch.hash || capability.value.Subject != subject || !capability.value.Allows(action) {
		return VerifiedAccess{}, ErrInvalid
	}
	return VerifiedAccess{epoch: epoch, capability: capability, action: action}, nil
}

func (a VerifiedAccess) valid() bool {
	return a.epoch.valid() && a.capability.valid() && a.capability.epochHash == a.epoch.hash && a.capability.value.Allows(a.action)
}
func (a VerifiedAccess) Epoch() VerifiedEpoch           { return a.epoch }
func (a VerifiedAccess) Capability() VerifiedCapability { return a.capability }
func (a VerifiedAccess) Action() Action                 { return a.action }

func isAncestor(current, ancestor VerifiedEpoch) bool {
	if !current.valid() || !ancestor.valid() {
		return false
	}
	for at := &current; at != nil; at = at.parent {
		if at.hash == ancestor.hash {
			return true
		}
	}
	return false
}

func memberMatchesEnrollment(member Member, enrolled RecipientEnrollment) bool {
	return enrolled.Scope == member.Scope && enrolled.UserID == member.UserID && enrolled.KeyID == member.ProductKeyID && enrolled.Recipient == member.Recipient && enrolled.Issuer.RootKey == member.RootKey
}

func selectCapabilityMember(value Capability, epoch Epoch, policyHash string, members []Member) (Member, error) {
	if !value.Valid() || !epoch.Valid() || value.Resource != epoch.Resource || value.RoomID != epoch.RoomID || value.Epoch != epoch.ID || value.Generation != epoch.Generation || value.PolicySHA256 != policyHash || value.MembershipSHA256 != epoch.MembershipSHA256 {
		return Member{}, ErrInvalid
	}
	for _, member := range members {
		if member.ProductKeyID == value.ProductKeyID && member.Scope.Subject == value.Subject && member.UserID == value.UserID && member.Role == value.Role && slices.Equal(member.Actions, value.Actions) && member.From == value.From && member.valid(value.Resource) {
			return cloneMember(member), nil
		}
	}
	return Member{}, ErrInvalid
}

func identityMatches(verified matrixproof.Identity, actor Actor, scope storage.Scope) bool {
	return scope.Valid() && actor.Valid() && verified.Scope == scope && verified.UserID.String() == actor.UserID && verified.DeviceID.String() == actor.DeviceID && verified.RootKey.String() == actor.RootKey
}
