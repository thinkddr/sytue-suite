// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"math"
	"sort"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BootstrapRosterInput is PUBLIC metadata. A correctly hashed stream is not a
// roster: each leaf still needs CURRENT product enrollment and consent, and the
// ORIGINAL frozen application source must still authorize its owner.
type BootstrapRosterInput struct {
	Member    Member          `json:"member"`
	Recipient SignedRecipient `json:"recipient"`
}

func (v BootstrapRosterInput) valid(key Resource) bool {
	if !v.Member.valid(key) || !v.Recipient.Valid() || len(v.Recipient.Signatures) == 0 || !memberMatchesEnrollment(v.Member, v.Recipient.RecipientEnrollment) {
		return false
	}
	h, err := matrixproof.RequestDigest(v.Recipient.RecipientEnrollment)
	return err == nil && h == v.Member.EnrollmentSHA256
}

func BootstrapRosterInputBytes(key Resource, value BootstrapRosterInput) ([]byte, error) {
	if !value.valid(key) {
		return nil, ErrInvalid
	}
	return matrixproof.CanonicalBytes(value)
}

// BootstrapRosterStream commits canonical JSON incrementally. Only individual
// transport pages are bounded; there is no total roster or history cap. Strict
// GLOBAL order rejects duplicates split across pages as well.
type BootstrapRosterStream struct {
	resource   Resource
	digest     hash.Hash
	membership hash.Hash
	count      int64
	last       *Member
	finished   bool
}

func NewBootstrapRosterStream(key Resource) (*BootstrapRosterStream, error) {
	if !key.Valid() {
		return nil, ErrInvalid
	}
	h := sha256.New()
	_, _ = h.Write([]byte(`{"domain":"org.sytue.resource.bootstrap.members.v1","inputs":[`))
	membership := sha256.New()
	_, _ = membership.Write([]byte(`{"domain":"org.sytue.resource.membership.v1","members":[`))
	return &BootstrapRosterStream{resource: key, digest: h, membership: membership}, nil
}

func (s *BootstrapRosterStream) Add(value BootstrapRosterInput) error {
	if s == nil || s.finished || s.count == math.MaxInt64 {
		return ErrInvalid
	}
	raw, err := BootstrapRosterInputBytes(s.resource, value)
	if err != nil {
		return err
	}
	if s.last != nil && !memberOrder(*s.last, value.Member) {
		return ErrInvalid
	}
	if s.count > 0 {
		_, _ = s.digest.Write([]byte(","))
	}
	_, _ = s.digest.Write(raw)
	member, err := matrixproof.CanonicalBytes(value.Member)
	if err != nil {
		return err
	}
	if s.count > 0 {
		_, _ = s.membership.Write([]byte(","))
	}
	_, _ = s.membership.Write(member)
	last := cloneMember(value.Member)
	s.last = &last
	s.count++
	return nil
}

func (s *BootstrapRosterStream) Finish() (Decimal, string, error) {
	if s == nil || s.finished || s.count == 0 {
		return "", "", ErrInvalid
	}
	s.finished = true
	key, err := matrixproof.CanonicalBytes(s.resource)
	if err != nil {
		return "", "", err
	}
	_, _ = s.digest.Write([]byte(`],"resource":`))
	_, _ = s.digest.Write(key)
	_, _ = s.digest.Write([]byte("}"))
	_, _ = s.membership.Write([]byte(`],"resource":`))
	_, _ = s.membership.Write(key)
	_, _ = s.membership.Write([]byte("}"))
	count, _ := Sequence(s.count)
	return count, hex.EncodeToString(s.digest.Sum(nil)), nil
}

// MembershipSHA256 binds the same streamed leaves to the plan's independent
// membership commitment, using the existing canonical MembershipDigest shape.
func (s *BootstrapRosterStream) MembershipSHA256() string {
	if s == nil || !s.finished || s.membership == nil {
		return ""
	}
	return hex.EncodeToString(s.membership.Sum(nil))
}

type BootstrapRosterManifest struct {
	Version             int     `json:"v"`
	Domain              string  `json:"domain"`
	PlanSHA256          string  `json:"plan_sha256"`
	MemberCount         Decimal `json:"member_count"`
	OrderedInputsSHA256 string  `json:"ordered_inputs_sha256"`
}
type SignedBootstrapRosterManifest struct {
	BootstrapRosterManifest
	Signatures signatures.Signatures `json:"signatures"`
}

func (m BootstrapRosterManifest) Valid() bool {
	n, err := m.MemberCount.Int64()
	return m.Version == 1 && m.Domain == "org.sytue.resource.bootstrap.roster-manifest.v1" && digest(m.PlanSHA256) && err == nil && n > 0 && digest(m.OrderedInputsSHA256)
}

func (m BootstrapRosterManifest) ValidFor(plan BootstrapPlan) bool {
	h, err := BootstrapPlanSHA256(plan)
	return m.Valid() && err == nil && h == m.PlanSHA256
}

func NewBootstrapRosterManifest(plan BootstrapPlan, count Decimal, streamSHA string) (BootstrapRosterManifest, error) {
	h, err := BootstrapPlanSHA256(plan)
	if err != nil {
		return BootstrapRosterManifest{}, err
	}
	m := BootstrapRosterManifest{Version: 1, Domain: "org.sytue.resource.bootstrap.roster-manifest.v1", PlanSHA256: h, MemberCount: count, OrderedInputsSHA256: streamSHA}
	if !m.Valid() {
		return BootstrapRosterManifest{}, ErrInvalid
	}
	return m, nil
}

// PrepareBootstrapRosterManifest is the in-memory convenience API; callers with
// larger rosters can use BootstrapRosterStream without buffering all inputs.
func PrepareBootstrapRosterManifest(plan BootstrapPlan, inputs []BootstrapRosterInput) (BootstrapRosterManifest, []BootstrapRosterInput, error) {
	ordered := append([]BootstrapRosterInput(nil), inputs...)
	sort.Slice(ordered, func(i, j int) bool { return memberOrder(ordered[i].Member, ordered[j].Member) })
	stream, err := NewBootstrapRosterStream(plan.Source.Resource)
	if err != nil {
		return BootstrapRosterManifest{}, nil, err
	}
	for _, v := range ordered {
		if err := stream.Add(v); err != nil {
			return BootstrapRosterManifest{}, nil, err
		}
	}
	count, sha, err := stream.Finish()
	if err != nil {
		return BootstrapRosterManifest{}, nil, err
	}
	manifest, err := NewBootstrapRosterManifest(plan, count, sha)
	if err != nil {
		return BootstrapRosterManifest{}, nil, err
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		return BootstrapRosterManifest{}, nil, err
	}
	if err := json.Unmarshal(raw, &ordered); err != nil {
		return BootstrapRosterManifest{}, nil, err
	}
	return manifest, ordered, nil
}

const BootstrapRosterPageMaxBytes = 128 * 1024

// Page boundaries are transport metadata; they do not change the signed stream.
type BootstrapRosterPage struct {
	Offset Decimal                `json:"offset"`
	Inputs []BootstrapRosterInput `json:"inputs"`
}

func BootstrapRosterPageBytes(key Resource, page BootstrapRosterPage) ([]byte, error) {
	offset, err := page.Offset.Int64()
	if err != nil || offset < 0 || len(page.Inputs) == 0 {
		return nil, ErrInvalid
	}
	for i, input := range page.Inputs {
		if !input.valid(key) || i > 0 && !memberOrder(page.Inputs[i-1].Member, input.Member) {
			return nil, ErrInvalid
		}
	}
	raw, err := matrixproof.CanonicalBytes(page)
	if err != nil || len(raw) > BootstrapRosterPageMaxBytes {
		return nil, ErrInvalid
	}
	return raw, nil
}

// BootstrapRosterPageOrderKeys derives the byte ordering used by the SAME Go
// member comparator. SQL persists these public bounds to fence append races.
func BootstrapRosterPageOrderKeys(key Resource, page BootstrapRosterPage) ([]byte, []byte, error) {
	if _, err := BootstrapRosterPageBytes(key, page); err != nil {
		return nil, nil, err
	}
	order := func(m Member) []byte { return []byte(m.Scope.Subject + "\x00" + m.UserID + "\x00" + m.ProductKeyID) }
	return order(page.Inputs[0].Member), order(page.Inputs[len(page.Inputs)-1].Member), nil
}

func DecodeBootstrapRosterPage(key Resource, raw []byte) (BootstrapRosterPage, error) {
	var out BootstrapRosterPage
	if len(raw) > BootstrapRosterPageMaxBytes {
		return out, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil {
		return BootstrapRosterPage{}, ErrInvalid
	}
	canonical, err := BootstrapRosterPageBytes(key, out)
	if err != nil || !bytes.Equal(canonical, raw) {
		return BootstrapRosterPage{}, ErrInvalid
	}
	return out, nil
}

// VerifiedBootstrapRosterManifest authenticates only a PENDING public capture.
// It has no epoch, grant, registry-routing permit or VerifiedPlan constructor.
type VerifiedBootstrapRosterManifest struct {
	plan                     SignedBootstrapPlan
	manifest                 SignedBootstrapRosterManifest
	planProof, manifestProof []byte
	hash                     string
	current                  VerifiedSession
	source                   OriginalBootstrapSource
}

func verifyBootstrapOwnerMetadata(ctx context.Context, signed SignedBootstrapPlan, current VerifiedSession) (string, json.RawMessage, error) {
	value := signed.BootstrapPlan
	if !value.Valid() || !current.valid() || value.CreatorScope != current.Scope() || value.Creator != current.Actor() {
		return "", nil, ErrInvalid
	}
	binding, err := matrixproof.RequestDigest(current.Binding())
	if err != nil || binding != value.SessionBindingSHA256 {
		return "", nil, ErrInvalid
	}
	if err := current.Device().VerifyJSON(ctx, value.CreatorScope, storage.PublicationTarget{RoomID: value.RoomID, DeviceID: value.Creator.DeviceID}, signed); err != nil {
		return "", nil, err
	}
	sha, err := BootstrapPlanSHA256(value)
	if err != nil {
		return "", nil, err
	}
	proof, err := matrixproof.CanonicalBytes(signed)
	return sha, proof, err
}

func verifyBootstrapOwnerIntent(ctx context.Context, signed SignedBootstrapPlan, current VerifiedSession, source OriginalBootstrapSource) (string, json.RawMessage, error) {
	if source == nil {
		return "", nil, ErrInvalid
	}
	sha, proof, err := verifyBootstrapOwnerMetadata(ctx, signed, current)
	if err != nil {
		return "", nil, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return "", nil, err
	}
	retained, err := source.BootstrapSource(ctx, signed.Source.Resource)
	if err != nil {
		return "", nil, err
	}
	if retained != signed.Source {
		return "", nil, ErrRevoked
	}
	return sha, proof, nil
}

func revalidateBootstrapOwnerIntent(ctx context.Context, value BootstrapPlan, current VerifiedSession, source OriginalBootstrapSource) error {
	if source == nil || !value.Valid() || !current.valid() {
		return ErrRevoked
	}
	if err := current.Revalidate(ctx); err != nil {
		return err
	}
	if err := source.CheckBootstrapSource(ctx, value.Source); err != nil {
		return err
	}
	return current.Revalidate(ctx)
}

func VerifyBootstrapRosterManifest(ctx context.Context, plan SignedBootstrapPlan, manifest SignedBootstrapRosterManifest, current VerifiedSession, source OriginalBootstrapSource) (VerifiedBootstrapRosterManifest, error) {
	if source == nil {
		return VerifiedBootstrapRosterManifest{}, ErrInvalid
	}
	metadata, err := verifyBootstrapPeerMetadata(ctx, plan, manifest, current)
	if err != nil {
		return VerifiedBootstrapRosterManifest{}, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return VerifiedBootstrapRosterManifest{}, err
	}
	retained, err := source.BootstrapSource(ctx, plan.Source.Resource)
	if err != nil {
		return VerifiedBootstrapRosterManifest{}, err
	}
	if retained != plan.Source {
		return VerifiedBootstrapRosterManifest{}, ErrRevoked
	}
	out := VerifiedBootstrapRosterManifest{plan: metadata.plan, manifest: metadata.manifest, planProof: metadata.planProof, manifestProof: metadata.manifestProof, hash: metadata.hash, current: current, source: source}
	if err := out.Revalidate(ctx); err != nil {
		return VerifiedBootstrapRosterManifest{}, err
	}
	return out, nil
}

func (m VerifiedBootstrapRosterManifest) Value() BootstrapRosterManifest {
	return m.manifest.BootstrapRosterManifest
}
func (m VerifiedBootstrapRosterManifest) Plan() BootstrapPlan     { return m.plan.BootstrapPlan }
func (m VerifiedBootstrapRosterManifest) SHA256() string          { return m.hash }
func (m VerifiedBootstrapRosterManifest) PublicPlanBytes() []byte { return bytes.Clone(m.planProof) }

func (m VerifiedBootstrapRosterManifest) PublicManifestBytes() []byte {
	return bytes.Clone(m.manifestProof)
}

func (m VerifiedBootstrapRosterManifest) SessionBinding() SessionBinding { return m.current.Binding() }

func (m VerifiedBootstrapRosterManifest) Revalidate(ctx context.Context) error {
	if !digest(m.hash) || !m.manifest.ValidFor(m.plan.BootstrapPlan) || len(m.planProof) == 0 || len(m.manifestProof) == 0 {
		return ErrRevoked
	}
	return revalidateBootstrapOwnerIntent(ctx, m.plan.BootstrapPlan, m.current, m.source)
}

type BootstrapRosterPageSource interface {
	BootstrapRosterPage(context.Context, Decimal) (BootstrapRosterPage, error)
}

func resolveBootstrapRosterInput(ctx context.Context, input BootstrapRosterInput, provider *SessionProvider) (VerifiedRecipient, VerifiedSession, error) {
	current, err := provider.Resolve(ctx, input.Member.Scope, input.Recipient.Issuer.DeviceID)
	if err != nil {
		return VerifiedRecipient{}, VerifiedSession{}, err
	}
	verified, err := VerifyAccountRecipient(ctx, input.Recipient, current.Device())
	if err != nil {
		return VerifiedRecipient{}, VerifiedSession{}, err
	}
	if current.Actor() != input.Recipient.Issuer || current.Recipient().SHA256() != verified.SHA256() || current.Binding().EnrollmentSHA256 != input.Member.EnrollmentSHA256 || !memberMatchesEnrollment(input.Member, current.Recipient().Value()) {
		return VerifiedRecipient{}, VerifiedSession{}, ErrRevoked
	}
	return verified, current, nil
}

// CompleteBootstrapRosterCapture uses ALL pages and the SAME existing roster and
// plan verifiers. Every recipient resolves independently through CURRENT product
// consent and enrollment; saved page/status flags never become a verified leaf.
func CompleteBootstrapRosterCapture(ctx context.Context, pending VerifiedBootstrapRosterManifest, pages BootstrapRosterPageSource, provider *SessionProvider) (VerifiedBootstrapPlan, error) {
	if pages == nil || provider == nil {
		return VerifiedBootstrapPlan{}, ErrInvalid
	}
	if err := pending.Revalidate(ctx); err != nil {
		return VerifiedBootstrapPlan{}, err
	}
	count, _ := pending.Value().MemberCount.Int64()
	stream, err := NewBootstrapRosterStream(pending.Plan().Source.Resource)
	if err != nil {
		return VerifiedBootstrapPlan{}, err
	}
	var members []Member
	var recipients []VerifiedRecipient
	var sessions []VerifiedSession
	var offset int64
	for offset < count {
		page, err := fetchBootstrapRosterPage(ctx, pending.Plan().Source.Resource, pages, offset, count)
		if err != nil {
			return VerifiedBootstrapPlan{}, err
		}
		for _, input := range page.Inputs {
			if err := stream.Add(input); err != nil {
				return VerifiedBootstrapPlan{}, err
			}
			verified, current, err := resolveBootstrapRosterInput(ctx, input, provider)
			if err != nil {
				return VerifiedBootstrapPlan{}, err
			}
			members = append(members, cloneMember(input.Member))
			recipients = append(recipients, verified)
			sessions = append(sessions, current)
			offset++
		}
	}
	n, sha, err := stream.Finish()
	if err != nil || n != pending.Value().MemberCount || sha != pending.Value().OrderedInputsSHA256 {
		return VerifiedBootstrapPlan{}, ErrInvalid
	}
	roster, err := VerifyRoster(pending.Plan().Source.Resource, members, recipients)
	if err != nil {
		return VerifiedBootstrapPlan{}, err
	}
	if roster.SHA256() != pending.Plan().MembershipSHA256 {
		return VerifiedBootstrapPlan{}, ErrInvalid
	}
	for _, current := range sessions {
		if err := current.Revalidate(ctx); err != nil {
			return VerifiedBootstrapPlan{}, err
		}
	}
	plan, err := VerifyBootstrapPlan(ctx, pending.plan, roster, pending.current, pending.source)
	if err != nil {
		return VerifiedBootstrapPlan{}, err
	}
	plan.currentRecipients = sessions
	if err := plan.Revalidate(ctx); err != nil {
		return VerifiedBootstrapPlan{}, err
	}
	return plan, nil
}

// CurrentRosterBindings returns public fencing tuples, never permissions/keys.
// Empty means this plan was not completed by the current paginated capture.
func (p VerifiedBootstrapPlan) CurrentRosterBindings() []SessionBinding {
	out := make([]SessionBinding, len(p.currentRecipients))
	for i, current := range p.currentRecipients {
		out[i] = current.Binding()
	}
	return out
}
