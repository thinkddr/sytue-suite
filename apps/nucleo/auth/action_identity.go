// SPDX-License-Identifier: Apache-2.0
package auth

// ActionIdentity is a non-secret UI snapshot. Its incarnation survives ordinary
// refresh and changes on interactive adoption or a different identity. It cannot
// authorize an HTTP request; the persistent native witness remains authoritative.
type ActionIdentity struct {
	Identity    IdentitySnapshot
	Incarnation uint64
}

func (s *Session) cacheActionIdentity() {
	snapshot := &ActionIdentity{Identity: s.identityLocked(), Incarnation: s.actionVersion}
	s.actionIdentity.Store(snapshot)
}

// SnapshotActionIdentity does not wait for an OIDC renewal or credential write.
// User actions can capture an identity before dispatching background work.
func (s *Session) SnapshotActionIdentity() ActionIdentity {
	if snapshot := s.actionIdentity.Load(); snapshot != nil {
		return *snapshot
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheActionIdentity()
	return *s.actionIdentity.Load()
}
