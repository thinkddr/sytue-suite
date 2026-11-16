// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type policyEntry struct {
	port     Port
	identity Identity
	target   storage.PublicationTarget
	policy   storage.PublicationPolicy
	original context.Context
	gate     *offline.WorkGate
}

// Policies retains original Go authorization callbacks as transient scoped
// capabilities. HTTP admission/acknowledgment remains durable at the origin;
// releasing a callback is never evidence that an earlier send was not accepted.
type Policies struct {
	mu      sync.Mutex
	entries map[string]*policyEntry
}

func NewPolicies() *Policies { return &Policies{entries: make(map[string]*policyEntry)} }
func (p *Policies) Add(ctx context.Context, port Port, identity Identity, target storage.PublicationTarget, policy storage.PublicationPolicy) (string, func(), error) { //nolint:contextcheck // the policy gate is deliberately bound to the registration ctx; its release func takes none
	if port == nil || identity.Validate() != nil || !target.Valid() || target.DeviceID != identity.Binding.DeviceID || policy == nil {
		return "", nil, ErrRejected
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	id, err := newCapabilityID()
	if err != nil {
		return "", nil, err
	}
	entry := &policyEntry{port: port, identity: identity, target: target, policy: policy, original: ctx, gate: offline.NewWorkGate(ctx)}
	p.mu.Lock()
	if len(p.entries) >= 1024 {
		p.mu.Unlock()
		entry.gate.Close() //nolint:contextcheck // gate teardown takes no ctx by design: closing must not be cancellable
		return "", nil, ErrRejected
	}
	if _, found := p.entries[id]; found {
		p.mu.Unlock()
		entry.gate.Close() //nolint:contextcheck // gate teardown takes no ctx by design: closing must not be cancellable
		return "", nil, ErrRejected
	}
	p.entries[id] = entry
	p.mu.Unlock()
	var once sync.Once
	return id, func() { once.Do(func() { p.mu.Lock(); delete(p.entries, id); p.mu.Unlock(); entry.gate.Close() }) }, nil //nolint:contextcheck // gate teardown takes no ctx by design: closing must not be cancellable
}

func (p *Policies) Authorize(ctx context.Context, port Port, identity Identity, id string, target storage.PublicationTarget, transaction string) error { //nolint:contextcheck // work runs under the gate ctx derived from the registration ctx, cancelled by the caller ctx via AfterFunc
	if !ValidID(id) || !target.Valid() || len(transaction) < 1 || len(transaction) > 512 || strings.ContainsAny(transaction, "\x00\r\n") {
		return ErrRejected
	}
	p.mu.Lock()
	entry := p.entries[id]
	p.mu.Unlock()
	if entry == nil || port == nil || entry.port.ID() != port.ID() || entry.identity != identity || entry.target != target {
		return ErrRejected
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	original, cancel := context.WithCancel(entry.original)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	if ctx.Err() != nil {
		cancel()
	}
	active, done, err := entry.gate.Start(original)
	if err != nil {
		return err
	}
	defer done()
	if err := RevalidateIdentity(active, port, identity); err != nil { //nolint:contextcheck // active derives from the registration lifetime and is cancelled by the caller ctx via AfterFunc
		return err
	}
	if err := entry.policy(active); err != nil { //nolint:contextcheck // active derives from the registration lifetime and is cancelled by the caller ctx via AfterFunc
		return err
	}
	if err := active.Err(); err != nil {
		return err
	}
	if err := RevalidateIdentity(active, port, identity); err != nil { //nolint:contextcheck // active derives from the registration lifetime and is cancelled by the caller ctx via AfterFunc
		return err
	}
	return nil
}

func (p *Policies) RevokePort(ctx context.Context, portID string) error { //nolint:contextcheck // gate teardown takes no ctx by design: closing must not be cancellable
	if !ValidID(portID) {
		return ErrRejected
	}
	p.mu.Lock()
	entries := []*policyEntry{}
	for id, entry := range p.entries {
		if entry.port.ID() == portID {
			delete(p.entries, id)
			entries = append(entries, entry)
		}
	}
	p.mu.Unlock()
	for _, entry := range entries {
		entry.gate.Close() //nolint:contextcheck // gate teardown takes no ctx by design: closing must not be cancellable
	}
	return ctx.Err()
}
