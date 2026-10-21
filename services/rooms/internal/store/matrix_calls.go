// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	mediadomain "git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

const callEpochTTL = 10 * time.Minute

// CallMatrixGate provisions authorization rooms, never media key material.
type CallMatrixGate interface {
	EnsureCallRoom(context.Context, string, uuid.UUID, int64, []string) (string, error)
	Reconcile(context.Context, string, []string, bool, bool) error
}

// CallMediaTransport enforces revocation in the SFU before acknowledging an ACL
// change. Both implementations live in this process; no internal HTTP loop.
type CallMediaTransport interface {
	PauseCall(context.Context, string, []string, []string) error
	EnableCall(context.Context, string, string, string, string, bool) error
	RotateCallTransport(context.Context, string, string, string, uuid.UUID, int64, int) error
}

func (s *Store) SetCallMatrixGate(g CallMatrixGate)         { s.callMatrix = g }
func (s *Store) SetCallMediaTransport(g CallMediaTransport) { s.callMedia = g }

type callMember struct {
	mediadomain.CallMember
	JoinID         uuid.UUID
	Role           mediadomain.Role
	PublishAllowed bool
	Joined         bool
	SID            string
	Removed        bool
	Blocked        bool
	PendingUntil   time.Time
	DocumentUntil  *time.Time
}
type callState struct {
	ID                   uuid.UUID
	Tenant, Name, Status string
	Document             bool
	DocumentID           uuid.UUID
	DocumentVersion      int64
	IdentityVersion      int64
	MaxParticipants      int
	ChannelID            uuid.UUID
	Members              []callMember
}

// Identity/document or channel locks precede the call row, matching revocation order.
func lockCallTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (callState, error) {
	st := callState{ID: id, Tenant: tenant}
	var metadata []byte
	if err := tx.QueryRow(ctx, `SELECT metadata FROM rooms WHERE tenant=$1 AND id=$2`, tenant, id).Scan(&metadata); err != nil {
		return st, callError(err)
	}
	var meta struct {
		Source *mediadomain.RoomSource `json:"source"`
	}
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return st, err
	}
	if meta.Source != nil && meta.Source.Kind == mediadomain.SourceDocument {
		st.Document = true
		docID, err := uuid.Parse(meta.Source.RefID)
		if err != nil {
			return st, mediadomain.ErrPermission
		}
		st.DocumentID = docID
		if proof, ok := mediadomain.DocumentCallAccessFrom(ctx); ok {
			st.IdentityVersion, err = lockDocumentIdentityFenceTx(ctx, tx, tenant, proof.Actor)
			if err != nil {
				return st, err
			}
		}
		st.DocumentVersion, err = lockDocumentCallFenceTx(ctx, tx, docID)
		if err != nil {
			return st, err
		}
	}
	if meta.Source != nil && meta.Source.Kind == mediadomain.SourceChat {
		channel, err := uuid.Parse(meta.Source.RefID)
		if err != nil {
			return st, mediadomain.ErrPermission
		}
		st.ChannelID = channel
		if err := lockMatrixChannelTx(ctx, tx, tenant, channel); err != nil {
			return st, err
		}
	}
	if err := tx.QueryRow(ctx, `SELECT name,status,max_participants FROM rooms WHERE tenant=$1 AND id=$2 FOR UPDATE`, tenant, id).Scan(&st.Name, &st.Status, &st.MaxParticipants); err != nil {
		return st, callError(err)
	}
	return st, nil
}

func callError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return mediadomain.ErrNotFound
	}
	return err
}

func callMembersTx(ctx context.Context, tx pgx.Tx, st *callState) error {
	rows, err := tx.Query(ctx, `SELECT subject,device_id,join_id,role,publish_allowed,joined,livekit_sid,removed,blocked,pending_until,document_access_until FROM matrix_call_members WHERE tenant=$1 AND call_id=$2 ORDER BY subject`, st.Tenant, st.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	st.Members = nil
	for rows.Next() {
		var m callMember
		if err := rows.Scan(&m.Subject, &m.DeviceID, &m.JoinID, &m.Role, &m.PublishAllowed, &m.Joined, &m.SID, &m.Removed, &m.Blocked, &m.PendingUntil, &m.DocumentUntil); err != nil {
			return err
		}
		st.Members = append(st.Members, m)
	}
	return rows.Err()
}

func callMemberActive(m callMember, now time.Time) bool {
	return !m.Removed && !m.Blocked && (m.Joined || m.PendingUntil.After(now)) && (m.DocumentUntil == nil || m.DocumentUntil.After(now))
}

func callRoster(st callState, now time.Time) []mediadomain.CallMember {
	roster := []mediadomain.CallMember{}
	for _, m := range st.Members {
		if callMemberActive(m, now) {
			roster = append(roster, m.CallMember)
		}
	}
	return roster
}

func callSubjects(roster []mediadomain.CallMember) []string {
	out := make([]string, 0, len(roster))
	for _, m := range roster {
		out = append(out, m.Subject)
	}
	return out
}

func (s *Store) pruneCallTx(ctx context.Context, tx pgx.Tx, st *callState) error {
	if st.Status == string(mediadomain.RoomEnded) {
		return mediadomain.ErrRoomClosed
	}
	var allowed []string
	if st.ChannelID != uuid.Nil {
		state, err := matrixChannelTx(ctx, tx, st.Tenant, st.ChannelID)
		if err != nil {
			return err
		}
		if !state.ReadOnly {
			allowed = state.Subjects
		}
	}
	denied, err := deniedCallSubjectsTx(ctx, tx, st.Tenant, st.ID)
	if err != nil {
		return err
	}

	if err := callMembersTx(ctx, tx, st); err != nil {
		return err
	}
	now := time.Now()
	for _, m := range st.Members {
		reject := rejectCallMember(m, st.ChannelID != uuid.Nil, allowed, denied, now) || st.Document && !s.documentCallsEnabled
		if reject && !m.Removed {
			if _, err := tx.Exec(ctx, `UPDATE matrix_call_members SET removed=true,ready_generation=0 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, st.Tenant, st.ID, m.Subject); err != nil {
				return err
			}
		}
	}
	return callMembersTx(ctx, tx, st)
}

// rotateCallTx pauses senders before retiring the previous room. Every epoch
// receives a different Matrix room, while Olm keys target only roster devices.
func (s *Store) rotateCallTx(ctx context.Context, tx pgx.Tx, st callState, force bool) (mediadomain.CallEncryption, error) {
	result := mediadomain.CallEncryption{CallID: st.ID, Members: callRoster(st, time.Now())}
	before, expires, found, err := loadCallEpochTx(ctx, tx, st, &result)
	if err != nil {
		return result, err
	}
	if !force && found && slices.Equal(before, result.Members) && expires.After(time.Now().Add(callRefreshWindow(st, expires))) {
		result.ExpiresAtMS = expires.UnixMilli()
		setCallLeader(&result)
		return result, nil
	}
	if s.callMatrix == nil || s.callMedia == nil {
		return result, mediadomain.ErrEncryptionUnavailable
	}
	if err := s.pauseCallEpoch(ctx, st, result, before); err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `UPDATE matrix_call_members SET ready_generation=0,joined=false,livekit_sid='',pending_until=now()+interval '2 minutes' WHERE tenant=$1 AND call_id=$2`, st.Tenant, st.ID); err != nil {
		return result, err
	}
	if len(result.Members) == 0 {
		if err := s.callMedia.RotateCallTransport(ctx, callTransportName(st, result), "", st.Tenant, st.ID, result.Generation, st.MaxParticipants); err != nil {
			return result, err
		}
		_, err := tx.Exec(ctx, `UPDATE matrix_call_epochs SET expires_at=LEAST(expires_at,now()) WHERE tenant=$1 AND call_id=$2`, st.Tenant, st.ID)
		return result, err
	}
	return s.createCallEpochTx(ctx, tx, st, result)
}

func loadCallEpochTx(ctx context.Context, tx pgx.Tx, st callState, result *mediadomain.CallEncryption) ([]mediadomain.CallMember, time.Time, bool, error) {
	var raw []byte
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT generation,room_id,livekit_room,members,expires_at FROM matrix_call_epochs WHERE tenant=$1 AND call_id=$2 ORDER BY generation DESC LIMIT 1`, st.Tenant, st.ID).Scan(&result.Generation, &result.MatrixRoomID, &result.LiveKitRoom, &raw, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, expires, false, nil
	}
	if err != nil {
		return nil, expires, false, err
	}
	var before []mediadomain.CallMember
	if json.Unmarshal(raw, &before) != nil {
		return nil, expires, false, mediadomain.ErrEncryptionUnavailable
	}
	return before, expires, true, nil
}

func (s *Store) pauseCallEpoch(ctx context.Context, st callState, result mediadomain.CallEncryption, before []mediadomain.CallMember) error {
	paused := callSubjects(before)
	for _, m := range result.Members {
		if !slices.Contains(paused, m.Subject) {
			paused = append(paused, m.Subject)
		}
	}
	current := callSubjects(result.Members)
	removed := []string{}
	for _, subject := range paused {
		if !slices.Contains(current, subject) {
			removed = append(removed, subject)
		}
	}
	if err := s.callMedia.PauseCall(ctx, callTransportName(st, result), paused, removed); err != nil {
		return err
	}
	// Previous epochs were already retired when the current one committed.
	// Keep every reference without replaying historical rooms on every refresh.
	if result.MatrixRoomID != "" {
		return s.callMatrix.Reconcile(ctx, result.MatrixRoomID, nil, true, false)
	}
	return nil
}

func (s *Store) createCallEpochTx(ctx context.Context, tx pgx.Tx, st callState, result mediadomain.CallEncryption) (mediadomain.CallEncryption, error) {
	oldTransport := callTransportName(st, result)
	result.Generation++
	result.LiveKitRoom = fmt.Sprintf("%s-e%d", st.Name, result.Generation)
	if err := s.callMedia.RotateCallTransport(ctx, oldTransport, result.LiveKitRoom, st.Tenant, st.ID, result.Generation, st.MaxParticipants); err != nil {
		return result, err
	}
	room, err := s.callMatrix.EnsureCallRoom(matrixContext(ctx, tx), st.Tenant, st.ID, result.Generation, callSubjects(result.Members))
	if err != nil {
		return result, err
	}
	result.MatrixRoomID = room
	expires := callEpochExpiry(st, time.Now())
	result.ExpiresAtMS = expires.UnixMilli()
	setCallLeader(&result)
	members, err := json.Marshal(result.Members)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_call_epochs(tenant,call_id,generation,room_id,livekit_room,members,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, st.Tenant, st.ID, result.Generation, room, result.LiveKitRoom, members, expires)
	return result, err
}

func setCallLeader(binding *mediadomain.CallEncryption) {
	if len(binding.Members) > 0 {
		binding.LeaderSubject = binding.Members[0].Subject
		binding.LeaderDeviceID = binding.Members[0].DeviceID
	}
}

//nolint:gocognit,gocyclo // transactional state machine for encrypted call preparation requires managing locks, epochs, membership, and media bridges in a single unit
func (s *Store) PrepareCallEncryption(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID, role mediadomain.Role) (mediadomain.CallEncryption, error) {
	tenant := tenancy.MustFrom(ctx).Slug
	var result mediadomain.CallEncryption
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		st, err := lockCallTx(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		if err := s.requireDocumentCallProof(ctx, st, actor); err != nil {
			return err
		}
		if err := s.pruneCallTx(ctx, tx, &st); err != nil {
			return err
		}
		if st.ChannelID != uuid.Nil {
			state, err := matrixChannelTx(ctx, tx, tenant, st.ChannelID)
			if err != nil {
				return err
			}
			if state.ReadOnly || !slices.Contains(state.Subjects, actor) {
				return mediadomain.ErrPermission
			}
		}
		replacing := false
		for _, m := range st.Members {
			if m.Subject == actor && m.Blocked {
				return mediadomain.ErrPermission
			}
			if m.Subject == actor && callMemberActive(m, time.Now()) {
				replacing = true
			}
		}
		if !replacing && st.MaxParticipants > 0 && len(callRoster(st, time.Now())) >= st.MaxParticipants {
			return mediadomain.ErrRoomFull
		}

		_, err = tx.Exec(ctx, `INSERT INTO matrix_call_members(tenant,call_id,subject,device_id,join_id,role,pending_until) VALUES($1,$2,$3,$4,$5,$6,now()+interval '2 minutes')
 ON CONFLICT(tenant,call_id,subject) DO UPDATE SET device_id=EXCLUDED.device_id,join_id=EXCLUDED.join_id,role=EXCLUDED.role,pending_until=EXCLUDED.pending_until,joined=false,livekit_sid='',removed=false,ready_generation=0`, tenant, id, actor, device, join, role)
		if err != nil {
			return err
		}
		if st.DocumentID != uuid.Nil {
			proof, _ := mediadomain.DocumentCallAccessFrom(ctx)
			var until *time.Time
			if proof.ExpiresAtMS > 0 {
				expiry := time.UnixMilli(proof.ExpiresAtMS)
				until = &expiry
			}
			if _, err := tx.Exec(ctx, `UPDATE matrix_call_members SET document_access_until=$4,identity_version=$5 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, tenant, id, actor, until, proof.IdentityVersion); err != nil {
				return err
			}
		}
		if err := callMembersTx(ctx, tx, &st); err != nil {
			return err
		}
		result, err = s.rotateCallTx(ctx, tx, st, true)
		return err
	})
	return result, err
}

func findCallMember(st callState, actor, device string, join uuid.UUID) (callMember, error) {
	for _, m := range st.Members {
		if m.Subject == actor && m.DeviceID == device && m.JoinID == join && callMemberActive(m, time.Now()) {
			return m, nil
		}
	}
	return callMember{}, mediadomain.ErrPermission
}

func (s *Store) CallEncryption(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID) (mediadomain.CallEncryption, error) {
	tenant := tenancy.MustFrom(ctx).Slug
	var result mediadomain.CallEncryption
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		st, err := lockCallTx(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		if err := s.requireDocumentCallProof(ctx, st, actor); err != nil {
			return err
		}
		if err := s.pruneCallTx(ctx, tx, &st); err != nil {
			return err
		}
		// Commit a changed roster even if this now-removed caller cannot obtain it.
		result, err = s.rotateCallTx(ctx, tx, st, false)
		return err
	})
	if err != nil {
		return result, err
	}
	// Re-read the nonce after reconciliation; a replaced device never obtains a
	// fresh epoch merely because its actor still belongs to the channel.
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		st := callState{ID: id, Tenant: tenant}
		if err := callMembersTx(ctx, tx, &st); err != nil {
			return err
		}
		_, err := findCallMember(st, actor, device, join)
		return err
	})
	return result, err
}

func (s *Store) ReadyCallEncryption(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID, generation int64) error {
	tenant := tenancy.MustFrom(ctx).Slug
	var denied error
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		st, err := lockCallTx(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		if err := s.requireDocumentCallProof(ctx, st, actor); err != nil {
			return err
		}
		if err := s.pruneCallTx(ctx, tx, &st); err != nil {
			return err
		}
		binding, err := s.rotateCallTx(ctx, tx, st, false)
		if err != nil {
			return err
		}
		m, err := findCallMember(st, actor, device, join)
		if err != nil {
			denied = err
			return nil //nolint:nilerr // denied captures member lookup denial without aborting transaction
		}
		if generation < 1 || binding.Generation != generation || binding.ExpiresAtMS <= time.Now().UnixMilli() {
			denied = mediadomain.ErrEncryptionGeneration
			return nil
		}
		if s.callMedia == nil {
			return mediadomain.ErrEncryptionUnavailable
		}
		if err := s.callMedia.EnableCall(ctx, binding.LiveKitRoom, actor, device, join.String(), m.Role.CanPublish() && m.PublishAllowed); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE matrix_call_members SET ready_generation=$4 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, tenant, id, actor, generation)
		return err
	})
	if err != nil {
		return err
	}
	return denied
}

// CallPresence ignores delayed webhooks from a previous connection or device.
func (s *Store) CallPresence(ctx context.Context, id uuid.UUID, actor string, join uuid.UUID, sid, transport string, joined bool) (bool, error) {
	if join == uuid.Nil || sid == "" || transport == "" {
		return false, nil
	}
	tenant := tenancy.MustFrom(ctx).Slug
	accepted := false
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		st, err := lockCallTx(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		var currentTransport string
		if err := tx.QueryRow(ctx, `SELECT livekit_room FROM matrix_call_epochs WHERE tenant=$1 AND call_id=$2 ORDER BY generation DESC LIMIT 1`, tenant, id).Scan(&currentTransport); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if currentTransport != transport {
			return nil
		}
		if err := callMembersTx(ctx, tx, &st); err != nil {
			return err
		}
		for _, m := range st.Members {
			if m.Subject != actor || m.JoinID != join || m.Removed || m.Blocked {
				continue
			}
			accepted, err = s.recordCallPresenceTx(ctx, tx, st, m, sid, joined)
			return err
		}
		return nil
	})
	return accepted, err
}

func (s *Store) RevokeCallMember(ctx context.Context, id uuid.UUID, subject string, blocked bool) error {
	return s.changeCall(ctx, id, func(ctx context.Context, tx pgx.Tx, st callState) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_call_members SET removed=true,blocked=$4,ready_generation=0 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, st.Tenant, id, subject, blocked)
		return err
	})
}

func (s *Store) CloseCallEncryption(ctx context.Context, id uuid.UUID) error {
	return s.changeCall(ctx, id, func(ctx context.Context, tx pgx.Tx, st callState) error {
		if _, err := tx.Exec(ctx, `UPDATE rooms SET status='ended',ended_at=COALESCE(ended_at,now()),updated_at=now() WHERE tenant=$1 AND id=$2`, st.Tenant, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE matrix_call_members SET removed=true,ready_generation=0 WHERE tenant=$1 AND call_id=$2`, st.Tenant, id)
		return err
	})
}

func (s *Store) changeCall(ctx context.Context, id uuid.UUID, change func(context.Context, pgx.Tx, callState) error) error {
	tenant := tenancy.MustFrom(ctx).Slug
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		st, err := lockCallTx(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		if claims, ok := authn.FromContext(ctx); ok {
			if err := s.requireDocumentCallProof(ctx, st, claims.Subject); err != nil {
				return err
			}
		}
		if err := change(ctx, tx, st); err != nil {
			return err
		}
		if err := callMembersTx(ctx, tx, &st); err != nil {
			return err
		}
		_, err = s.rotateCallTx(ctx, tx, st, true)
		return err
	})
}

func (s *Store) ReadmitCallMember(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_call_members SET blocked=false WHERE tenant=$1 AND call_id=$2 AND subject=$3`, tenancy.MustFrom(ctx).Slug, id, actor)
		return err
	})
}

func (s *Store) SetCallPublication(ctx context.Context, id uuid.UUID, actor string, allowed bool) error {
	return s.changeCall(ctx, id, func(ctx context.Context, tx pgx.Tx, st callState) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_call_members SET publish_allowed=$4,ready_generation=0 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, st.Tenant, id, actor, allowed)
		return err
	})
}

func (s *Store) revokeChannelCallsTx(ctx context.Context, tx pgx.Tx, tenant string, channel uuid.UUID) error {
	t, err := tenancy.New(tenant)
	if err != nil {
		return err
	}
	ctx = tenancy.Into(ctx, t)
	rows, err := tx.Query(ctx, `SELECT id FROM rooms WHERE tenant=$1 AND status<>'ended' AND metadata->'source'->>'kind'='chat' AND metadata->'source'->>'ref_id'=$2 ORDER BY id FOR UPDATE`, tenant, channel.String())
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		st, err := lockCallTx(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		if err := s.pruneCallTx(ctx, tx, &st); err != nil {
			return err
		}
		if len(st.Members) == 0 {
			continue
		}
		if _, err := s.rotateCallTx(ctx, tx, st, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) revokeSubjectCalls(ctx context.Context, subject, tenant string) error {
	type call struct {
		tenant string
		id     uuid.UUID
	}
	var calls []call
	if err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant,call_id FROM matrix_call_members WHERE subject=$1 AND NOT removed AND ($2='' OR tenant=$2) ORDER BY tenant,call_id`, subject, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c call
			if err := rows.Scan(&c.tenant, &c.id); err != nil {
				return err
			}
			calls = append(calls, c)
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	for _, c := range calls {
		t, err := tenancy.New(c.tenant)
		if err != nil {
			return err
		}
		if err := s.RevokeCallMember(tenancy.Into(ctx, t), c.id, subject, false); err != nil && !errors.Is(err, mediadomain.ErrNotFound) {
			return err
		}
	}
	return nil
}

// ExpireCallEncryptions bounds abandoned pending joins and idle epochs without
// relying on a browser poll. A failed remote revocation is retried next tick.
func (s *Store) ExpireCallEncryptions(ctx context.Context) error {
	type call struct {
		tenant string
		id     uuid.UUID
	}
	var due []call
	if err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT r.tenant,r.id FROM rooms r JOIN LATERAL
 (SELECT expires_at FROM matrix_call_epochs e WHERE e.tenant=r.tenant AND e.call_id=r.id ORDER BY generation DESC LIMIT 1) e ON true
 WHERE r.status<>'ended' AND (e.expires_at<=now()+interval '2 seconds' OR EXISTS(SELECT 1 FROM matrix_call_members m WHERE m.tenant=r.tenant AND m.call_id=r.id AND NOT m.removed AND NOT m.joined AND m.pending_until<=now()) OR EXISTS(SELECT 1 FROM matrix_call_members m WHERE m.tenant=r.tenant AND m.call_id=r.id AND NOT m.removed AND m.document_access_until<=now()) OR ($1 AND r.metadata->'source'->>'kind'='document'))
 AND EXISTS(SELECT 1 FROM matrix_call_members m WHERE m.tenant=r.tenant AND m.call_id=r.id AND NOT m.removed)
 ORDER BY r.tenant,r.id LIMIT 100`, !s.documentCallsEnabled)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c call
			if err := rows.Scan(&c.tenant, &c.id); err != nil {
				return err
			}
			due = append(due, c)
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	for _, c := range due {
		t, err := tenancy.New(c.tenant)
		if err != nil {
			return err
		}
		callCtx := tenancy.Into(ctx, t)
		if err := s.enMantenimientoDe(callCtx, func(tx pgx.Tx, _ string) error {
			st, err := lockCallTx(callCtx, tx, c.tenant, c.id)
			if err != nil {
				return err
			}
			if err := s.pruneCallTx(callCtx, tx, &st); err != nil {
				return err
			}
			_, err = s.rotateCallTx(callCtx, tx, st, false)
			return err
		}); err != nil && !errors.Is(err, mediadomain.ErrRoomClosed) {
			return err
		}
	}
	return nil
}

func callTransportName(st callState, binding mediadomain.CallEncryption) string {
	if binding.LiveKitRoom != "" {
		return binding.LiveKitRoom
	}
	return st.Name
}

func (s *Store) CallTransportName(ctx context.Context, id uuid.UUID) (string, error) {
	var name string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT livekit_room FROM matrix_call_epochs WHERE tenant=$1 AND call_id=$2 ORDER BY generation DESC LIMIT 1`, tenancy.MustFrom(ctx).Slug, id).Scan(&name)
	})
	return name, callError(err)
}

// A room_finished webhook for a retired epoch must not close the active call.
func (s *Store) FinishCallTransport(ctx context.Context, id uuid.UUID, transport string) (bool, error) {
	tenant := tenancy.MustFrom(ctx).Slug
	finished := false
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		st, err := lockCallTx(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		var current string
		if err := tx.QueryRow(ctx, `SELECT livekit_room FROM matrix_call_epochs WHERE tenant=$1 AND call_id=$2 ORDER BY generation DESC LIMIT 1`, tenant, id).Scan(&current); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if current != transport || st.Status == string(mediadomain.RoomEnded) {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE matrix_call_members SET removed=true,ready_generation=0 WHERE tenant=$1 AND call_id=$2`, tenant, id); err != nil {
			return err
		}
		if err := callMembersTx(ctx, tx, &st); err != nil {
			return err
		}
		if _, err := s.rotateCallTx(ctx, tx, st, true); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE rooms SET status='ended',ended_at=COALESCE(ended_at,now()),updated_at=now() WHERE tenant=$1 AND id=$2`, tenant, id)
		finished = err == nil
		return err
	})
	return finished, err
}

func (s *Store) CallRole(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID) (mediadomain.Role, error) {
	var role mediadomain.Role
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT role FROM matrix_call_members WHERE tenant=$1 AND call_id=$2 AND subject=$3 AND device_id=$4 AND join_id=$5 AND NOT removed AND NOT blocked AND (joined OR pending_until>now())`, tenancy.MustFrom(ctx).Slug, id, actor, device, join).Scan(&role)
	})
	return role, callError(err)
}

func deniedCallSubjectsTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT subject FROM matrix_call_members m WHERE tenant=$1 AND call_id=$2 AND (
 EXISTS(SELECT 1 FROM matrix_identity_access a WHERE a.subject=m.subject AND a.status<>'active') OR
 EXISTS(SELECT 1 FROM matrix_tenant_access a WHERE a.tenant=m.tenant AND a.subject=m.subject AND a.status<>'active'))`, tenant, id)
	if err != nil {
		return nil, err
	}
	var denied []string
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			rows.Close()
			return nil, err
		}
		denied = append(denied, subject)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return denied, nil
}

func rejectCallMember(m callMember, inChannel bool, allowed, denied []string, now time.Time) bool {
	return slices.Contains(denied, m.Subject) || inChannel && !slices.Contains(allowed, m.Subject) || !m.Joined && !m.PendingUntil.After(now) || m.DocumentUntil != nil && !m.DocumentUntil.After(now.Add(2*time.Second))
}

func (s *Store) recordCallPresenceTx(ctx context.Context, tx pgx.Tx, st callState, m callMember, sid string, joined bool) (bool, error) {
	var err error
	if joined {
		if !m.Joined && !m.PendingUntil.After(time.Now()) {
			return false, nil
		}
		_, err = tx.Exec(ctx, `UPDATE matrix_call_members SET joined=true,livekit_sid=$4 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, st.Tenant, st.ID, m.Subject, sid)
	} else {
		if m.SID != sid || !m.Joined {
			return false, nil
		}
		_, err = tx.Exec(ctx, `UPDATE matrix_call_members SET joined=false,removed=true,ready_generation=0 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, st.Tenant, st.ID, m.Subject)
	}
	if err != nil {
		return false, err
	}
	if !joined {
		if err := callMembersTx(ctx, tx, &st); err != nil {
			return false, err
		}
		_, err = s.rotateCallTx(ctx, tx, st, true)
	}
	return err == nil, err
}

func callEpochExpiry(st callState, now time.Time) time.Time {
	expires := now.Add(callEpochTTL)
	for _, m := range st.Members {
		if callMemberActive(m, now) && m.DocumentUntil != nil && m.DocumentUntil.Before(expires) {
			expires = *m.DocumentUntil
		}
	}
	return expires
}

func callRefreshWindow(st callState, expires time.Time) time.Duration {
	for _, m := range st.Members {
		if !m.Removed && m.DocumentUntil != nil && !m.DocumentUntil.After(expires.Add(time.Millisecond)) {
			return 2 * time.Second
		}
	}
	return 30 * time.Second
}
