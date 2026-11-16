// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

var ErrPublicationAdmissionRequired = errors.New("matrix: durable publication admission is required")

// PublicationBinding contains no credentials or keys. It identifies the exact
// SDK producer, room, session and retry transaction captured before backup.
type PublicationBinding struct {
	Tenant         string
	Subject        string
	MatrixUserID   string
	DeviceID       string
	RoomID         string
	SessionID      string
	TransactionID  string
	NativeConsumer string
}

// PublicationAdmissionHook runs for ONE prepared operation without SDK state
// locks. Before returning its marker it must durably admit/persist the original
// wire and transaction, so retry after a lost HTTP receipt reuses both unchanged.
type PublicationAdmissionHook func(context.Context, *PreparedEncryptedPublication) (publication.Marker, error)

// PreparedEncryptedPublication keeps all SDK/session internals private. Preparing
// never sends a room event: it encrypts, ACKs recoverable backup/provenance and
// captures an immutable wire for either the normal SDK send or opaque AS relay.
type PreparedEncryptedPublication struct {
	owner                  *Client
	machine                *cryptohelper.CryptoHelper
	client                 *mautrix.Client
	session                credentials
	binding                PublicationBinding
	resourceProduct        string
	storageProduct         string
	identityVersion        storage.IdentityVersion
	intent                 *storagePublication
	resourceIntent         resourceOperation
	storageAdmissionIntent *storageAdmission
	raw                    json.RawMessage
	provenance             sessionproof.Session
	retired                atomic.Bool
	sending                chan struct{}
	markerMu               sync.Mutex
	marker                 publication.Marker
}

func (p *PreparedEncryptedPublication) Binding() PublicationBinding {
	if p == nil {
		return PublicationBinding{}
	}
	return p.binding
}

func (p *PreparedEncryptedPublication) Wire() []byte {
	if p == nil || p.retired.Load() {
		return nil
	}
	return bytes.Clone(p.raw)
}

func (p *PreparedEncryptedPublication) Provenance() sessionproof.Session {
	if p == nil || p.retired.Load() {
		return sessionproof.Session{}
	}
	return p.provenance
}

func (p *PreparedEncryptedPublication) Close() {
	if p != nil {
		p.retired.Store(true)
	}
}

func (p *PreparedEncryptedPublication) sameClient() bool {
	c := p.owner
	return !p.retired.Load() && !c.closed.Load() && c.crypto == p.machine && c.cli == p.client && c.session == p.session && c.session.DeviceID.String() == p.binding.DeviceID && c.cfg.Subject == p.binding.Subject && c.cfg.Tenant == p.binding.Tenant && c.cfg.Bootstrap.MatrixUserID == p.binding.MatrixUserID && c.cfg.NativeConsumer == p.binding.NativeConsumer && c.cfg.ResourceProduct == p.resourceProduct && c.cfg.Bootstrap.StorageProduct == p.storageProduct && c.cfg.Bootstrap.StorageIdentityVersion.Normalized() == p.identityVersion
}

func (p *PreparedEncryptedPublication) operationContext(ctx context.Context) context.Context {
	if p.storageAdmissionIntent != nil {
		ctx = context.WithValue(ctx, storageAdmissionKey{}, p.storageAdmissionIntent)
	}
	if p.resourceIntent != nil {
		ctx = p.resourceIntent.operationContext(ctx)
	}
	if p.intent != nil {
		return context.WithValue(ctx, storagePublicationContext{}, *p.intent)
	}
	return ctx
}

func (p *PreparedEncryptedPublication) lockReady(ctx context.Context) (func(), error) {
	if p == nil || p.owner == nil || p.retired.Load() {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := p.owner
	c.stateMu.RLock()
	if !p.sameClient() {
		c.stateMu.RUnlock()
		return nil, ErrIdentity
	}
	if err := c.verifyPublicationSessionBackup(ctx, p.binding.RoomID, id.SessionID(p.binding.SessionID)); err != nil {
		c.stateMu.RUnlock()
		return nil, err
	}
	return c.stateMu.RUnlock, nil
}

// Revalidate repeats the captured operation policy OUTSIDE state locks and then
// checks this exact current SDK identity/outbound session and its backup ACK.
func (p *PreparedEncryptedPublication) Revalidate(ctx context.Context) error {
	if p == nil {
		return ErrIdentity
	}
	ctx = p.operationContext(ctx)
	if err := publicationOperationPolicy(ctx); err != nil {
		return err
	}
	release, err := p.lockReady(ctx)
	if err != nil {
		return err
	}
	release()
	return ctx.Err()
}

// PrepareEncryptedPublication is also used by the scoped Resource adapter. The
// caller supplies the existing intent context; later callbacks cannot replace
// its captured authorization with a new account, room, device or storage policy.
func (c *Client) PrepareEncryptedPublication(ctx context.Context, room string, kind event.Type, content any, transaction string) (*PreparedEncryptedPublication, error) {
	return c.prepareEncryptedPublication(ctx, room, kind, content, transaction)
}

func (c *Client) prepareEncryptedPublication(ctx context.Context, room string, kind event.Type, content any, transaction string) (*PreparedEncryptedPublication, error) {
	if err := publicationOperationPolicy(ctx); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	locked := true
	defer func() {
		if locked {
			c.stateMu.RUnlock()
		}
	}()
	if _, bound := ctx.Value(storagePublicationContext{}).(storagePublication); bound {
		if err := c.validateStoragePublication(ctx, room); err != nil {
			return nil, err
		}
	}
	if err := c.requireEncrypted(ctx, room); err != nil {
		return nil, err
	}
	if err := c.checkRoomIdentities(ctx, id.RoomID(room)); err != nil {
		return nil, err
	}
	if transaction == "" {
		transaction = c.cli.TxnID()
	}
	encrypted, err := c.crypto.Encrypt(ctx, id.RoomID(room), kind, content)
	if err != nil {
		return nil, err
	}
	p := c.newPreparedEncryptedPublication(ctx, room, transaction, encrypted)
	// Keep backupMu -> stateMu ordering: release the encryption read lease before
	// backup IO, then recheck the exact identity/session before exposing any wire.
	c.stateMu.RUnlock()
	locked = false
	if err := publicationOperationPolicy(ctx); err != nil {
		return nil, err
	}
	if err := c.backupEncryptedPublication(ctx, room, p.sameClient); err != nil {
		return nil, err
	}
	if err := publicationOperationPolicy(ctx); err != nil {
		return nil, err
	}
	release, err := p.lockReady(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := c.attachProvenanceAndCanonicalWire(p, room, encrypted); err != nil {
		return nil, err
	}
	return p, nil
}

func (c *Client) newPreparedEncryptedPublication(ctx context.Context, room, transaction string, encrypted *event.EncryptedEventContent) *PreparedEncryptedPublication {
	p := &PreparedEncryptedPublication{owner: c, machine: c.crypto, client: c.cli, session: c.session, binding: PublicationBinding{Tenant: c.cfg.Tenant, Subject: c.cfg.Subject, MatrixUserID: c.cfg.Bootstrap.MatrixUserID, DeviceID: c.session.DeviceID.String(), RoomID: room, SessionID: encrypted.SessionID.String(), TransactionID: transaction, NativeConsumer: c.cfg.NativeConsumer}, resourceProduct: c.cfg.ResourceProduct, storageProduct: c.cfg.Bootstrap.StorageProduct, identityVersion: c.cfg.Bootstrap.StorageIdentityVersion.Normalized(), sending: make(chan struct{}, 1)}
	if intent, bound := ctx.Value(storagePublicationContext{}).(storagePublication); bound {
		cloned := intent
		p.intent = &cloned
	}
	if intent, ok := ctx.Value(resourceRoomKey{}).(resourceOperation); ok {
		p.resourceIntent = intent
	}
	if intent, ok := ctx.Value(storageAdmissionKey{}).(*storageAdmission); ok {
		p.storageAdmissionIntent = intent
	}
	return p
}

func (c *Client) attachProvenanceAndCanonicalWire(p *PreparedEncryptedPublication, room string, encrypted *event.EncryptedEventContent) error {
	raw, err := json.Marshal(encrypted)
	if err != nil {
		return err
	}
	canonical, _, err := publication.CanonicalWire(raw, false)
	clear(raw)
	if err != nil {
		return err
	}
	own := c.crypto.Machine().OwnIdentity()
	p.provenance = sessionproof.Session{RoomID: room, SessionID: encrypted.SessionID.String(), SenderKey: own.IdentityKey.String(), SenderUser: c.session.UserID.String(), SigningKey: own.SigningKey.String()}
	if !p.provenance.Valid() || encrypted.SenderKey != "" && encrypted.SenderKey.String() != p.provenance.SenderKey {
		clear(canonical)
		return ErrIdentity
	}
	p.raw = canonical
	return nil
}

func (p *PreparedEncryptedPublication) pinMarker(marker publication.Marker) error {
	if !marker.Valid() {
		return ErrPublicationAdmissionRequired
	}
	p.markerMu.Lock()
	defer p.markerMu.Unlock()
	if p.marker.Valid() && p.marker != marker {
		return ErrIdentity
	}
	p.marker = marker
	return nil
}

func (p *PreparedEncryptedPublication) markerValue() publication.Marker {
	p.markerMu.Lock()
	defer p.markerMu.Unlock()
	return p.marker
}

// Send performs the exact prepared SDK write. Opaque AS transport can instead
// use Wire/Binding after obtaining its own durable sending permit. Neither path
// can reconstruct plaintext or re-encrypt a retry under the same transaction.
func (p *PreparedEncryptedPublication) Send(ctx context.Context, hook PublicationAdmissionHook) (EventRef, error) {
	if p == nil || p.owner == nil {
		return EventRef{}, ErrIdentity
	}
	if p.resourceIntent != nil {
		return EventRef{}, ErrPublicationAdmissionRequired
	}
	select {
	case p.sending <- struct{}{}:
		defer func() { <-p.sending }()
	case <-ctx.Done():
		return EventRef{}, ctx.Err()
	}
	ctx = p.operationContext(ctx)
	if err := p.Revalidate(ctx); err != nil {
		return EventRef{}, err
	}
	if err := p.resolveSendMarker(ctx, hook); err != nil {
		return EventRef{}, err
	}
	// A policy callback may use the SDK or HTTP; never run it under stateMu.
	if err := publicationOperationPolicy(ctx); err != nil {
		return EventRef{}, err
	}
	release, err := p.lockReady(ctx)
	if err != nil {
		return EventRef{}, err
	}
	defer release()
	wire, err := p.buildMarkedSendWire()
	if err != nil {
		return EventRef{}, err
	}
	defer clear(wire)
	response, err := p.client.SendMessageEvent(ctx, id.RoomID(p.binding.RoomID), event.EventEncrypted, json.RawMessage(wire), mautrix.ReqSendEvent{TransactionID: p.binding.TransactionID})
	if err != nil {
		return EventRef{}, err
	}
	if response == nil || response.EventID == "" {
		return EventRef{}, errors.New("matrix: missing event receipt")
	}
	p.owner.scheduleBackup()
	return EventRef{RoomID: p.binding.RoomID, EventID: response.EventID.String()}, nil
}

func (p *PreparedEncryptedPublication) resolveSendMarker(ctx context.Context, hook PublicationAdmissionHook) error {
	p.owner.stateMu.RLock()
	required := p.owner.cfg.RequirePublicationAdmission
	p.owner.stateMu.RUnlock()
	if required && hook == nil && !p.markerValue().Valid() {
		return ErrPublicationAdmissionRequired
	}
	if hook != nil {
		marker, err := hook(ctx, p)
		if err != nil {
			return err
		}
		return p.pinMarker(marker)
	}
	return nil
}

func (p *PreparedEncryptedPublication) buildMarkedSendWire() ([]byte, error) {
	wire := bytes.Clone(p.raw)
	marker := p.markerValue()
	if marker.Valid() {
		marked, err := publication.AttachMarker(wire, marker)
		clear(wire)
		if err != nil {
			return nil, err
		}
		return marked, nil
	}
	if p.owner.cfg.RequirePublicationAdmission {
		clear(wire)
		return nil, ErrPublicationAdmissionRequired
	}
	return wire, nil
}

// resumePreparedPublication rebuilds only the local SDK holder around retained
// ciphertext. It grants no room/record policy and does not re-encrypt a retry.
// New sends still require the same current producer and outbound backup ACK.
func (c *Client) resumePreparedPublication(ctx context.Context, binding PublicationBinding, wire []byte, provenance sessionproof.Session) *PreparedEncryptedPublication {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	p := &PreparedEncryptedPublication{owner: c, machine: c.crypto, client: c.cli, session: c.session, binding: binding, resourceProduct: c.cfg.ResourceProduct, storageProduct: c.cfg.Bootstrap.StorageProduct, identityVersion: c.cfg.Bootstrap.StorageIdentityVersion.Normalized(), raw: bytes.Clone(wire), provenance: provenance, sending: make(chan struct{}, 1)}
	if intent, ok := ctx.Value(storagePublicationContext{}).(storagePublication); ok {
		cloned := intent
		p.intent = &cloned
	}
	if intent, ok := ctx.Value(resourceRoomKey{}).(resourceOperation); ok {
		p.resourceIntent = intent
	}
	if intent, ok := ctx.Value(storageAdmissionKey{}).(*storageAdmission); ok {
		p.storageAdmissionIntent = intent
	}
	return p
}
