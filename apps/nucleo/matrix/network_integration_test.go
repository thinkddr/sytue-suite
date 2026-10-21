// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Network relay is opt-in, artificial-account-only remote verification. Credentials
// arrive over stdin and are never emitted. Product clients retain HTTPS-only SSO.
func TestMatrixSynapseRelay(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_NETWORK_RELAY") != "1" {
		t.Skip("remote artificial-account harness only")
	}
	relay := &networkRelay{t: t}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	defer func() {
		if relay.client != nil {
			_ = relay.client.Close()
		}
	}()
	scan := bufio.NewScanner(os.Stdin)
	scan.Buffer(make([]byte, 4096), 2<<20)
	for scan.Scan() {
		var request networkRequest
		if json.Unmarshal(scan.Bytes(), &request) != nil {
			relay.emit(map[string]any{"type": "error", "stage": "input"})
			continue
		}
		if request.Action == "close" {
			return
		}
		if err := relay.handle(ctx, request); err != nil {
			relay.emit(map[string]any{"type": "error", "stage": relay.stage, "error_type": fmt.Sprintf("%T", err)})
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal("network relay input failed")
	}
}

type networkRequest struct { //nolint:tagliatelle // Harness uses the same canonical Matrix field names.
	Action      string       `json:"action"`
	Homeserver  string       `json:"homeserver_url"`
	User        string       `json:"user_id"`
	AccessToken string       `json:"access_token"`
	Device      string       `json:"device_id"`
	Room        string       `json:"room_id"`
	Controller  string       `json:"controller_user_id"`
	Password    string       `json:"password"`
	Text        string       `json:"text"`
	Transaction string       `json:"transaction_id"`
	Event       *event.Event `json:"event"`
}

type networkRelay struct {
	t      *testing.T
	mu     sync.Mutex
	client *Client
	stage  string
}

func (r *networkRelay) emit(value map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = json.NewEncoder(os.Stdout).Encode(value)
}

func (r *networkRelay) handle(ctx context.Context, req networkRequest) error {
	r.stage = req.Action
	if req.Action == "init" {
		return r.initialize(ctx, req)
	}
	if r.client == nil {
		return ErrNotReady
	}
	switch req.Action {
	case "send":
		ref, err := r.client.SendText(ctx, req.Room, req.Text)
		if err == nil {
			r.emit(map[string]any{"type": "sent", "event_id": ref.EventID})
		}
		return err
	case "verify":
		txn, err := r.client.StartVerificationWith(ctx, req.User)
		if err == nil {
			r.emit(map[string]any{"type": "verification-started", "transaction_id": txn})
		}
		return err
	case "confirm":
		return r.client.ConfirmSAS(ctx, id.VerificationTransactionID(req.Transaction))
	case "decrypt":
		if req.Event == nil || req.Event.Type != event.EventEncrypted {
			return ErrNotEncrypted
		}
		req.Event.RoomID = id.RoomID(req.Room)
		if err := req.Event.Content.ParseRaw(req.Event.Type); err != nil {
			return err
		}
		decrypted, err := r.client.crypto.Decrypt(ctx, req.Event)
		if err != nil {
			r.emit(map[string]any{"type": "undecryptable"})
			return nil //nolint:nilerr // Expected negative result is a typed fixture response, not a relay failure.
		}
		value, err := r.client.decryptedEvent(ctx, decrypted)
		if err != nil {
			r.emit(map[string]any{"type": "undecryptable"})
			return nil //nolint:nilerr // Expected negative result is a typed fixture response, not a relay failure.
		}
		content, err := DecodeMessage(value)
		if err == nil {
			r.emit(map[string]any{"type": "decrypted", "text": content.Body})
		}
		return err
	default:
		return ErrUnsupported
	}
}

func (r *networkRelay) initialize(ctx context.Context, req networkRequest) error {
	if r.client != nil {
		return ErrIdentity
	}
	r.stage = "init-origin"
	actual, err := url.Parse(req.Homeserver)
	// HTTP is accepted only by this remote fixture on the loopback Synapse port.
	if err != nil || actual.Scheme != "http" || actual.Host != "127.0.0.1:18584" || actual.Path != "" || actual.RawQuery != "" || actual.Fragment != "" {
		return ErrIdentity
	}
	r.stage = "init-client"
	cfg := Config{Bootstrap: Bootstrap{Actor: "artificial-go", Tenant: "artificial-network", HomeserverURL: "https://matrix.sytue.test", MatrixUserID: req.User, ControllerUserID: req.Controller, Encryption: string(id.AlgorithmMegolmV1), Channels: []RoomBinding{{ChannelID: "artificial-channel", RoomID: req.Room}}}, Subject: "artificial-go", Tenant: "artificial-network", StoreDir: r.t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: &http.Client{Timeout: 45 * time.Second}, VerificationCallbacks: r}
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	r.client = client
	client.cli.HomeserverURL = actual
	client.session = credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: id.UserID(req.User), DeviceID: id.DeviceID(req.Device), AccessToken: req.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant}
	if err := client.loadMaster(true); err != nil {
		return err
	}
	client.cli.UserID, client.cli.DeviceID, client.cli.AccessToken = client.session.UserID, client.session.DeviceID, client.session.AccessToken
	r.stage = "init-join"
	if _, err = client.cli.JoinRoom(ctx, req.Room, nil); err != nil {
		return err
	}
	r.stage = "init-store"
	if err := client.initialize(ctx, true); err != nil {
		return err
	}
	r.stage = "init-recovery"
	_, err = client.BootstrapRecovery(ctx, func(challenge *mautrix.RespUserInteractive) any {
		return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": req.User}, "password": req.Password, "session": challenge.Session}
	})
	if err != nil {
		return err
	}
	// Recovery material is confined to this process and discarded, never stdout.
	go func() {
		_ = client.Sync(ctx, func(value DecryptedEvent, err error) {
			if err != nil {
				return
			}
			content, decodeErr := DecodeMessage(value)
			if decodeErr == nil {
				r.emit(map[string]any{"type": "message", "event_id": value.Reference.EventID, "sender": value.Sender, "text": content.Body})
			}
		})
	}()
	r.stage = "init-sync"
	if err := client.WaitReady(ctx); err != nil {
		return err
	}
	r.emit(map[string]any{"type": "ready", "device_id": client.DeviceID()})
	return nil
}

func (r *networkRelay) VerificationRequested(ctx context.Context, txn id.VerificationTransactionID, _ id.UserID, _ id.DeviceID) {
	go func() {
		if err := r.client.AcceptVerification(ctx, txn); err != nil {
			r.emit(map[string]any{"type": "error", "stage": "accept"})
		}
	}()
}

func (r *networkRelay) VerificationReady(ctx context.Context, txn id.VerificationTransactionID, _ id.DeviceID, sas, _ bool, _ *verificationhelper.QRCode) {
	if sas {
		go func() {
			if err := r.client.StartSAS(ctx, txn); err != nil {
				r.emit(map[string]any{"type": "error", "stage": "start-sas"})
			}
		}()
	}
}

func (r *networkRelay) ShowSAS(_ context.Context, txn id.VerificationTransactionID, _ []rune, _ []string, decimal []int) {
	r.emit(map[string]any{"type": "sas", "transaction_id": txn, "decimal": decimal})
}

func (r *networkRelay) VerificationDone(_ context.Context, txn id.VerificationTransactionID, _ event.VerificationMethod) {
	r.emit(map[string]any{"type": "verified", "transaction_id": txn})
}

func (r *networkRelay) VerificationCancelled(_ context.Context, txn id.VerificationTransactionID, _ event.VerificationCancelCode, _ string) {
	r.emit(map[string]any{"type": "cancelled", "transaction_id": txn})
}
