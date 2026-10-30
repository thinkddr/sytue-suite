// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package matrix is the native Rooms client for maintained Matrix E2EE. It is kept
// separate from api so that the crypto engine is linked only by products using it.
package matrix

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var (
	ErrLoginRequired = errors.New("matrix: sign in is required")
	ErrIdentity      = errors.New("matrix: the account does not match this Rooms session")
	ErrSecureStore   = errors.New("matrix: secure key storage is unavailable or inconsistent")
	ErrNotEncrypted  = errors.New("matrix: this Rooms channel does not use supported E2EE")
	ErrNotReady      = errors.New("matrix: encrypted sync is not ready")
	ErrUnverified    = errors.New("matrix: sender device is not verified")
	ErrClosed        = errors.New("matrix: client is closed")
)

// RoomBinding and Bootstrap reuse the shared native service contract.
type (
	RoomBinding = api.RoomsMatrixRoomBinding
	Bootstrap   = api.RoomsMatrixBootstrap
)

// Secrets is the existing OS keychain contract, injectable for remote tests.
type Secrets interface {
	Read(string) ([]byte, error)
	Write(string, []byte) error
	Delete(string) error
}
type systemSecrets struct{}

func (systemSecrets) Read(key string) ([]byte, error)      { return auth.LeerSecreto(key) }
func (systemSecrets) Write(key string, value []byte) error { return auth.GuardarSecreto(key, value) }
func (systemSecrets) Delete(key string) error              { return auth.BorrarSecreto(key) }

// VerificationCallbacks is the maintained SAS verification UI contract. Callbacks
// run off the UI thread. A UI must explicitly accept/compare/confirm each request.
type VerificationCallbacks interface {
	verificationhelper.RequiredCallbacks
	verificationhelper.ShowSASCallbacks
}

// Config binds Matrix identity and the local store to the active Sytue actor/tenant.
type Config struct {
	Bootstrap             Bootstrap
	SytueApex             string
	Subject               string
	Tenant                string
	StoreDir              string
	DeviceName            string
	Secrets               Secrets
	HTTPClient            *http.Client
	VerificationCallbacks VerificationCallbacks
}

type credentials struct {
	Homeserver  string      `json:"homeserver"`
	UserID      id.UserID   `json:"user_id"`
	DeviceID    id.DeviceID `json:"device_id"`
	AccessToken string      `json:"access_token"`
	Subject     string      `json:"subject"`
	Tenant      string      `json:"tenant"`
}

// EventRef is safe for the Rooms metadata API: it contains no message plaintext.
type EventRef = api.RoomsMatrixEventRef

// DecryptedEvent is delivered only after Matrix authenticated decryption and trust
// checks. Content stays in the calling client, never the Rooms metadata API.
type DecryptedEvent struct {
	ChannelID    string
	Reference    EventRef
	SenderDevice string
	Sender       string
	Timestamp    int64
	Type         string
	Content      json.RawMessage
	Trust        id.TrustState
	Recovered    bool
}

// Client owns one person's Matrix device and encrypted SQLite session store.
type Client struct {
	cfg          Config
	account      string
	directory    string
	session      credentials
	cli          *mautrix.Client
	crypto       *cryptohelper.CryptoHelper
	verify       *verificationhelper.VerificationHelper
	master       []byte
	mu           sync.Mutex
	callKeys     map[*CallKeys]struct{}
	backupMu     sync.Mutex
	stateMu      sync.RWMutex
	done         chan struct{}
	cancel       context.CancelFunc
	syncing      atomic.Bool
	ready        atomic.Bool
	firstSync    chan struct{}
	firstOnce    sync.Once
	closed       atomic.Bool
	delivery     chan deliveryResult
	deliveryCtx  context.Context
	backupSignal chan struct{}
}

func newClient(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.Bootstrap.HomeserverURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		!validIdentityConfig(cfg) ||
		!strings.HasPrefix(cfg.Bootstrap.MatrixUserID, "@") || !strings.Contains(cfg.Bootstrap.MatrixUserID, ":") ||
		cfg.Bootstrap.Encryption != string(id.AlgorithmMegolmV1) {
		return nil, ErrIdentity
	}
	namespace := ""
	if cfg.Bootstrap.StorageProduct != "" {
		if !storage.Product(cfg.Bootstrap.StorageProduct) || !validStorageBindings(cfg.Bootstrap) {
			return nil, ErrIdentity
		}
		namespace = "storage:" + cfg.Bootstrap.StorageProduct
	} else if cfg.Bootstrap.StorageRoomID != "" || len(cfg.Bootstrap.StorageRoomIDs) != 0 {
		return nil, ErrIdentity
	}
	return newAccountClient(cfg, u, namespace)
}

// newAccountClient only constructs the shared local account/HTTP engine. Each
// public constructor validates its own identity and authorization policy first.
func newAccountClient(cfg Config, u *url.URL, namespace string) (*Client, error) {
	cfg.Bootstrap.Channels = slices.Clone(cfg.Bootstrap.Channels)
	cfg.Bootstrap.DraftRoomIDs = slices.Clone(cfg.Bootstrap.DraftRoomIDs)
	cfg.Bootstrap.StorageRoomIDs = slices.Clone(cfg.Bootstrap.StorageRoomIDs)
	cfg.Bootstrap.HomeserverURL = strings.TrimRight(u.String(), "/")
	if cfg.Secrets == nil {
		cfg.Secrets = systemSecrets{}
	}
	accountScope := cfg.Bootstrap.HomeserverURL + "\x00" + cfg.Bootstrap.MatrixUserID + "\x00" + cfg.Subject + "\x00" + cfg.Tenant
	if namespace != "" {
		accountScope = namespace + "\x00" + accountScope
	}
	h := sha256.Sum256([]byte(accountScope))
	account := hex.EncodeToString(h[:])
	cli, err := mautrix.NewClient(cfg.Bootstrap.HomeserverURL, id.UserID(cfg.Bootstrap.MatrixUserID), "")
	if err != nil {
		return nil, err
	}
	httpc := http.Client{Transport: api.Transporte}
	if cfg.HTTPClient != nil {
		httpc = *cfg.HTTPClient
	}
	// Matrix credentials must never follow a redirect to another authority. Media
	// is requested through the same homeserver, including remote mxc content.
	httpc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != u.Scheme || req.URL.Host != u.Host {
			return errors.New("matrix: cross-origin redirect rejected")
		}
		return nil
	}
	cli.Client = &httpc
	cli.Log = zerolog.Nop() // no plaintext, login tokens, recovery keys or HTTP bodies in logs
	cli.UserAgent = "Sytue Rooms"
	return &Client{cfg: cfg, account: account, directory: filepath.Join(cfg.StoreDir, account), cli: cli, backupSignal: make(chan struct{}, 1), firstSync: make(chan struct{})}, nil
}

// Open restores only a keychain-bound device. Missing/lost private storage never
// causes automatic replacement of device keys or a fallback to plaintext.
func Open(ctx context.Context, cfg Config) (*Client, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	saved, err := c.readSession()
	if err != nil {
		return nil, err
	}
	c.session = saved
	if err = c.initialize(ctx, false); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) readSession() (credentials, error) {
	data, err := c.cfg.Secrets.Read(c.sessionKey())
	if err != nil {
		return credentials{}, fmt.Errorf("%w: session", ErrSecureStore)
	}
	defer clear(data)
	if len(data) == 0 {
		return credentials{}, ErrLoginRequired
	}
	var saved credentials
	if json.Unmarshal(data, &saved) != nil || saved.Homeserver != c.cfg.Bootstrap.HomeserverURL ||
		saved.UserID != id.UserID(c.cfg.Bootstrap.MatrixUserID) || saved.Subject != c.cfg.Subject || saved.Tenant != c.cfg.Tenant ||
		saved.DeviceID == "" || saved.AccessToken == "" {
		return credentials{}, ErrIdentity
	}
	return saved, nil
}
func (c *Client) sessionKey() string { return "rooms-matrix-session-" + c.account }
func (c *Client) masterKey() string  { return "rooms-matrix-master-" + c.account }
func (c *Client) loadMaster(create bool) error {
	key, err := c.cfg.Secrets.Read(c.masterKey())
	if err != nil {
		return fmt.Errorf("%w: master key", ErrSecureStore)
	}
	if len(key) == 0 && create {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		if err = c.cfg.Secrets.Write(c.masterKey(), key); err != nil {
			clear(key)
			return fmt.Errorf("%w: master key", ErrSecureStore)
		}
	}
	if len(key) != 32 {
		return ErrSecureStore
	}
	clear(c.master)
	c.master = append([]byte(nil), key...)
	clear(key)
	return nil
}

func (c *Client) initialize(ctx context.Context, create bool) error {
	if err := c.loadMaster(false); err != nil {
		return err
	}
	c.cli.UserID, c.cli.DeviceID, c.cli.AccessToken = c.session.UserID, c.session.DeviceID, c.session.AccessToken
	who, err := c.cli.Whoami(ctx)
	if err != nil {
		return err
	}
	if who.UserID != c.session.UserID || who.DeviceID != c.session.DeviceID {
		return ErrIdentity
	}
	if err := os.MkdirAll(c.directory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(c.directory, 0o700); err != nil {
		return err
	}
	// The filename is a hash, not an untrusted device ID or provider URL.
	h := sha256.Sum256([]byte(c.session.DeviceID))
	dbPath := filepath.Join(c.directory, hex.EncodeToString(h[:])+".sqlite")
	if !create {
		if info, err := os.Stat(dbPath); err != nil || info.Size() == 0 {
			return fmt.Errorf("%w: device database", ErrSecureStore)
		}
	}
	dbFile, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err = dbFile.Chmod(0o600); err != nil {
		_ = dbFile.Close()
		return err
	}
	if err := dbFile.Close(); err != nil {
		return err
	}
	c.cli.Syncer = &guardedSyncer{DefaultSyncer: mautrix.NewDefaultSyncer(), owner: c}
	helper, err := cryptohelper.NewCryptoHelper(c.cli, c.master, dbPath)
	if err != nil {
		return err
	}
	c.crypto = helper
	helper.DBAccountID = c.account
	if err := helper.Init(ctx); err != nil {
		return err
	}
	c.cli.Crypto = helper
	machine := helper.Machine()
	machine.CryptoStore = c.devicePolicy(machine.CryptoStore)
	machine.AllowKeyShare = func(context.Context, *id.Device, event.RequestedKeyInfo) *mxcrypto.KeyShareRejection {
		return &mxcrypto.KeyShareRejectNoResponse
	}
	machine.SendKeysMinTrust = id.TrustStateCrossSignedVerified
	machine.ShareKeysMinTrust = id.TrustStateCrossSignedVerified
	if err := c.rotateRestoredOutbound(ctx); err != nil {
		return err
	}
	machine.SessionReceived = func(context.Context, id.RoomID, id.SessionID, uint32) { c.scheduleBackup() }
	if callbacks := c.cfg.VerificationCallbacks; callbacks != nil {
		c.verify = verificationhelper.NewVerificationHelper(c.cli, machine, nil, callbacks, false, false, true)
		if err := c.verify.Init(ctx); err != nil {
			return err
		}
		c.cli.Verification = c.verify
	}
	helper.DecryptErrorCallback = func(_ *event.Event, err error) { c.deliver(DecryptedEvent{}, err) }
	syncer, ok := c.cli.Syncer.(mautrix.ExtensibleSyncer)
	if !ok {
		return errors.New("matrix: unsupported syncer")
	}
	receive := func(ctx context.Context, evt *event.Event) {
		if !c.allowedRoom(evt.RoomID.String()) {
			return
		}
		value, err := c.decryptedEvent(ctx, evt)
		c.deliver(value, err)
	}
	syncer.OnEventType(event.EventMessage, receive)
	syncer.OnEventType(controlType(CallKeyEvent), receive)
	syncer.OnEventType(controlType(CallKeyRequestEvent), receive)
	return nil
}

type deliveryResult struct {
	value DecryptedEvent
	err   error
}

func (c *Client) deliver(value DecryptedEvent, err error) {
	if c.closed.Load() {
		return
	}
	// Olm controls have no channel ID. Consume them before the UI's timeline
	// routing, including while a call is waiting for its first media key.
	if err == nil && isControl(value.Type) {
		c.mu.Lock()
		keys := make([]*CallKeys, 0, len(c.callKeys))
		for key := range c.callKeys {
			keys = append(keys, key)
		}
		c.mu.Unlock()
		for _, key := range keys {
			key.Receive(value)
		}
		return
	}
	c.mu.Lock()
	queue, ctx := c.delivery, c.deliveryCtx
	c.mu.Unlock()
	if queue != nil && ctx != nil {
		select {
		case queue <- deliveryResult{value, err}:
		case <-ctx.Done():
		}
	}
}

// Sync blocks until canceled. CryptoHelper consumes the complete maintained sync
// response, including device changes, membership, to-device and one-time-key counts.
func (c *Client) Sync(ctx context.Context, handler func(DecryptedEvent, error)) error {
	if c.closed.Load() {
		return ErrClosed
	}
	if !c.syncing.CompareAndSwap(false, true) {
		return errors.New("matrix: sync is already running")
	}
	defer c.syncing.Store(false)
	syncCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.mu.Lock()
	if c.closed.Load() {
		c.mu.Unlock()
		return ErrClosed
	}
	c.cancel, c.done = cancel, make(chan struct{})
	c.delivery, c.deliveryCtx = make(chan deliveryResult, 32), syncCtx
	queue := c.delivery
	done := c.done
	c.mu.Unlock()
	go c.backupLoop(syncCtx)
	// UI delivery runs separately from sync/state mutation. A callback may close
	// its client without deadlocking the syncer or closing an in-use crypto store.
	go func() {
		for {
			select {
			case result := <-queue:
				if handler != nil && !c.closed.Load() {
					handler(result.value, result.err)
				}
			case <-syncCtx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		c.ready.Store(false)
		c.mu.Lock()
		c.cancel, c.delivery, c.deliveryCtx = nil, nil, nil
		c.mu.Unlock()
		close(done)
	}()
	return c.cli.SyncWithContext(syncCtx)
}

// Verification exposes only the official interactive flow; it never auto-confirms
// a new device. Nil means the calling UI did not install verification callbacks.
func (c *Client) Verification() *verificationhelper.VerificationHelper { return c.verify }

// Close stops sync and closes the encrypted store while keeping resumable secrets
// in the OS keychain. Logout below explicitly revokes and forgets them.
func (c *Client) Close() error {
	if c == nil || !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.ready.Store(false)
	c.firstOnce.Do(func() {
		if c.firstSync != nil {
			close(c.firstSync)
		}
	})
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.delivery, c.deliveryCtx = nil, nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	defer clear(c.master)
	if c.crypto != nil {
		return c.crypto.Close()
	}
	return nil
}

// Logout revokes this Matrix session and removes only this account's local keys.
func (c *Client) Logout(ctx context.Context) error {
	if c == nil {
		return nil
	}
	_, remoteErr := c.cli.Logout(ctx)
	localErr := c.Close()
	forgetErr := errors.Join(c.cfg.Secrets.Delete(c.sessionKey()), c.cfg.Secrets.Delete(c.masterKey()), c.cfg.Secrets.Delete(c.backupSecretKey()))
	// The directory was constructed exclusively from the authenticated account hash.
	removeErr := os.RemoveAll(c.directory)
	return errors.Join(remoteErr, localErr, forgetErr, removeErr)
}

func (c *Client) requireEncrypted(ctx context.Context, roomID string) error {
	if c.closed.Load() {
		return ErrClosed
	}
	if !c.ready.Load() {
		return ErrNotReady
	}
	if !c.allowedRoom(roomID) {
		return ErrIdentity
	}
	var state event.EncryptionEventContent
	if err := c.cli.StateEvent(ctx, id.RoomID(roomID), event.StateEncryption, "", &state); err != nil {
		return fmt.Errorf("%w: state unavailable", ErrNotEncrypted)
	}
	if state.Algorithm != id.AlgorithmMegolmV1 {
		return ErrNotEncrypted
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Verified {
		return ErrUnverified
	}
	return nil
}

// SendText encrypts explicitly and sends only m.room.encrypted, even if the SDK's
// automatic encryption policy would otherwise permit an unencrypted event.
func (c *Client) SendText(ctx context.Context, roomID, text string) (EventRef, error) {
	return c.send(ctx, roomID, event.MessageEventContent{MsgType: event.MsgText, Body: text})
}

func (c *Client) send(ctx context.Context, roomID string, content event.MessageEventContent) (EventRef, error) {
	return c.sendEvent(ctx, roomID, event.EventMessage, content)
}

func (c *Client) sendEvent(ctx context.Context, roomID string, kind event.Type, content any) (EventRef, error) {
	return c.sendEventWithTxn(ctx, roomID, kind, content, "")
}

func (c *Client) sendEventWithTxn(ctx context.Context, roomID string, kind event.Type, content any, transaction string) (EventRef, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if err := c.requireEncrypted(ctx, roomID); err != nil {
		return EventRef{}, err
	}
	if err := c.checkRoomIdentities(ctx, id.RoomID(roomID)); err != nil {
		return EventRef{}, err
	}
	encrypted, err := c.crypto.Encrypt(ctx, id.RoomID(roomID), kind, content)
	if err != nil {
		return EventRef{}, err
	}
	response, err := c.cli.SendMessageEvent(ctx, id.RoomID(roomID), event.EventEncrypted, encrypted, mautrix.ReqSendEvent{TransactionID: transaction})
	if err != nil {
		return EventRef{}, err
	}
	if response == nil || response.EventID == "" {
		return EventRef{}, errors.New("matrix: missing event receipt")
	}
	c.scheduleBackup()
	return EventRef{RoomID: roomID, EventID: response.EventID.String()}, nil
}

// Compile-time assertion: key sharing is provided by the maintained crypto store.
var _ mxcrypto.Store = (*mxcrypto.SQLCryptoStore)(nil)

func (c *Client) allowedRoom(roomID string) bool {
	if roomID == "" {
		return false
	}
	if c.cfg.Bootstrap.StorageProduct != "" {
		return roomID == c.cfg.Bootstrap.StorageRoomID || slices.Contains(c.cfg.Bootstrap.StorageRoomIDs, roomID)
	}
	if c.cfg.Bootstrap.DraftRoomID == roomID {
		return true
	}
	for _, draft := range c.cfg.Bootstrap.DraftRoomIDs {
		if draft == roomID {
			return true
		}
	}
	for _, binding := range c.cfg.Bootstrap.Channels {
		if binding.RoomID == roomID {
			return true
		}
	}
	return false
}

// guardedSyncer keeps sends closed while the maintained syncer updates membership
// and invalidates outbound sessions, and on any failed sync/reconnect.
type guardedSyncer struct {
	*mautrix.DefaultSyncer
	owner *Client
}

func (s *guardedSyncer) ProcessResponse(ctx context.Context, response *mautrix.RespSync, since string) error {
	s.owner.stateMu.Lock()
	defer s.owner.stateMu.Unlock()
	// Existing readers finish against their locked state; new readers wait for
	// the membership/key update instead of observing a spurious disconnection.
	s.owner.ready.Store(false)
	err := s.DefaultSyncer.ProcessResponse(ctx, s.owner.routeDeviceControls(ctx, response, since), since)
	if err == nil && ctx.Err() == nil && !s.owner.closed.Load() {
		s.owner.ready.Store(true)
		s.owner.firstOnce.Do(func() {
			if s.owner.firstSync != nil {
				close(s.owner.firstSync)
			}
		})
	}
	return err
}

func (s *guardedSyncer) OnFailedSync(response *mautrix.RespSync, err error) (time.Duration, error) {
	s.owner.ready.Store(false)
	return s.DefaultSyncer.OnFailedSync(response, err)
}

// checkRoomIdentities requires an explicitly verified human cross-signing
// identity. The maintained machine separately excludes unsigned devices, and the
// fixed controller is never a recipient, regardless of future signing state.
func (c *Client) checkRoomIdentities(ctx context.Context, roomID id.RoomID) error {
	members, err := c.cli.StateStore.GetRoomJoinedOrInvitedMembers(ctx, roomID)
	if err != nil {
		return err
	}
	for _, userID := range members {
		if userID == c.cli.UserID || userID == id.UserID(c.cfg.Bootstrap.ControllerUserID) {
			continue
		}
		// Private application data/drafts must never be shared with another human,
		// even one already cross-signed and trusted for ordinary Rooms channels.
		if c.privateRoom(roomID.String()) {
			return ErrIdentity
		}
		machine := c.crypto.Machine()
		keys, err := machine.CryptoStore.GetCrossSigningKeys(ctx, userID)
		if err != nil {
			return err
		}
		_, ok := keys[id.XSUsageMaster]
		if !ok {
			if _, err = machine.FetchKeys(ctx, []id.UserID{userID}, true); err != nil {
				return err
			}
			keys, err = machine.CryptoStore.GetCrossSigningKeys(ctx, userID)
			if err != nil {
				return err
			}
			_, ok = keys[id.XSUsageMaster]
		}
		if !ok {
			return ErrUnverified
		}
		approved, err := machine.IsUserTrusted(ctx, userID)
		if err != nil {
			return err
		}
		if !approved {
			return ErrUnverified
		}
	}
	return nil
}

func validIdentityConfig(cfg Config) bool {
	return cfg.Subject != "" && cfg.Tenant != "" && cfg.StoreDir != "" && cfg.Bootstrap.Actor == cfg.Subject && cfg.Bootstrap.Tenant == cfg.Tenant && validController(cfg.Bootstrap)
}
