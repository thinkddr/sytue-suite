// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build cgo

package matrix

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// resetHomeserver es un homeserver mínimo con varias versiones de copia de
// seguridad, borrado de versiones y UIA en la subida de claves de firma cruzada.
type resetHomeserver struct {
	mu           sync.Mutex
	accountData  map[string]json.RawMessage
	deviceKeys   map[string]json.RawMessage
	crossSigning map[string]json.RawMessage
	versions     []string
	bodies       map[string]json.RawMessage
	deleted      []string
	uiaDone      bool
	next         int
}

func (h *resetHomeserver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	notFound := func() {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errcode":"M_NOT_FOUND"}`)
	}
	switch {
	case strings.HasSuffix(path, "/account/whoami"):
		_, _ = io.WriteString(w, `{"user_id":"@sytue_person:matrix.test","device_id":"DEVICE"}`)
	case strings.Contains(path, "/account_data/"):
		key := strings.SplitN(path, "/account_data/", 2)[1]
		switch {
		case r.Method == http.MethodPut:
			h.accountData[key], _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{}`)
		case h.accountData[key] != nil:
			_, _ = w.Write(h.accountData[key])
		default:
			notFound()
		}
	case strings.HasSuffix(path, "/keys/device_signing/upload"):
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if auth := body["auth"]; len(auth) == 0 || string(auth) == "null" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"session":"uia","flows":[{"stages":["m.login.dummy"]}],"params":{}}`)
			return
		}
		h.uiaDone = true
		delete(body, "auth")
		h.crossSigning = body
		_, _ = io.WriteString(w, `{}`)
	case strings.HasSuffix(path, "/keys/signatures/upload"):
		_, _ = io.WriteString(w, `{"failures":{}}`)
	case strings.HasSuffix(path, "/room_keys/version") && r.Method == http.MethodPost:
		h.next++
		version := strconv.Itoa(h.next)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["version"], body["count"], body["etag"] = version, 0, "0"
		h.bodies[version], _ = json.Marshal(body)
		h.versions = append(h.versions, version)
		_, _ = io.WriteString(w, `{"version":"`+version+`"}`)
	case strings.HasSuffix(path, "/room_keys/version"):
		if len(h.versions) == 0 {
			notFound()
			return
		}
		_, _ = w.Write(h.bodies[h.versions[len(h.versions)-1]])
	case strings.Contains(path, "/room_keys/version/"):
		version := path[strings.LastIndex(path, "/")+1:]
		if r.Method == http.MethodDelete {
			for i, v := range h.versions {
				if v == version {
					h.versions = append(h.versions[:i], h.versions[i+1:]...)
					h.deleted = append(h.deleted, version)
					_, _ = io.WriteString(w, `{}`)
					return
				}
			}
		}
		if body, ok := h.bodies[version]; ok && r.Method == http.MethodGet {
			_, _ = w.Write(body)
			return
		}
		notFound()
	case strings.HasSuffix(path, "/room_keys/keys"):
		_, _ = io.WriteString(w, `{"count":0,"etag":"0"}`)
	case strings.HasSuffix(path, "/keys/query"):
		response := map[string]any{"device_keys": map[string]any{"@sytue_person:matrix.test": h.deviceKeys}}
		for name, keys := range h.crossSigning {
			response[name+"s"] = map[string]any{"@sytue_person:matrix.test": keys}
		}
		_ = json.NewEncoder(w).Encode(response)
	case strings.HasSuffix(path, "/keys/upload"):
		var body struct {
			DeviceKeys json.RawMessage `json:"device_keys"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.DeviceKeys) > 0 {
			h.deviceKeys["DEVICE"] = body.DeviceKeys
		}
		_, _ = io.WriteString(w, `{"one_time_key_counts":{"signed_curve25519":50}}`)
	default:
		_, _ = io.WriteString(w, `{}`)
	}
}

func newResetClient(t *testing.T, server *httptest.Server, product string) *Client {
	t.Helper()
	cfg := configFor(t, server)
	if product != "" {
		cfg = storageConfig(t, server, product)
	}
	cfg.Bootstrap.MatrixUserID = "@sytue_person:matrix.test"
	cfg.Subject = "person"
	c, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.session = credentials{Homeserver: server.URL, UserID: "@sytue_person:matrix.test", DeviceID: "DEVICE", AccessToken: "artificial-token", Subject: cfg.Subject, Tenant: cfg.Tenant}
	if err := c.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	if err := c.initialize(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	return c
}

func dummyUIA(*mautrix.RespUserInteractive) any { return map[string]any{"type": "m.login.dummy"} }

// «He olvidado la clave de recuperación» (D-ROOM-02): la cuenta tiene identidad y
// copia de seguridad y nadie recuerda la clave; Recover y BootstrapRecovery no
// tenían salida. El reinicio pasa por UIA, deja una identidad, una clave y una
// copia nuevas, y borra las copias selladas con la clave perdida.
func TestResetRecoveryReplacesTheForgottenIdentity(t *testing.T) {
	h := &resetHomeserver{accountData: map[string]json.RawMessage{}, deviceKeys: map[string]json.RawMessage{}, bodies: map[string]json.RawMessage{}}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	c := newResetClient(t, server, "")
	ctx := t.Context()

	forgotten, err := c.BootstrapRecovery(ctx, dummyUIA)
	if err != nil || forgotten == "" {
		t.Fatalf("bootstrap: %v", err)
	}
	// Hasta que la persona confirma que guardó la clave, repetir devuelve la
	// misma; después ya hay identidad y no se sustituye sin pasar por el reinicio.
	if err := c.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	h.mu.Lock()
	oldMaster := string(h.crossSigning["master_key"])
	h.uiaDone = false
	h.mu.Unlock()
	if _, err := c.BootstrapRecovery(ctx, dummyUIA); !errors.Is(err, ErrRecoveryExists) {
		t.Fatalf("bootstrap replaced an identity: %v", err)
	}

	// Sin UIA no se borra nada: el homeserver decide antes que la limpieza.
	if _, err := c.ResetRecovery(ctx, func(*mautrix.RespUserInteractive) any { return nil }); err == nil {
		t.Fatal("reset without UIA succeeded")
	}
	h.mu.Lock()
	if len(h.deleted) != 0 || len(h.versions) != 1 {
		t.Fatalf("a failed UIA deleted backups: %v", h.deleted)
	}
	h.mu.Unlock()
	// …y la clave vieja sigue abriendo la identidad vieja, por si aparece.
	if err := c.Recover(ctx, forgotten); err != nil {
		t.Fatalf("a failed UIA orphaned the old identity: %v", err)
	}

	fresh, err := c.ResetRecovery(ctx, dummyUIA)
	if err != nil || fresh == "" || fresh == forgotten {
		t.Fatalf("reset: key changed=%v err=%v", fresh != forgotten, err)
	}
	h.mu.Lock()
	newMaster := string(h.crossSigning["master_key"])
	deleted, versions := append([]string(nil), h.deleted...), append([]string(nil), h.versions...)
	uia := h.uiaDone
	h.mu.Unlock()
	if !uia || newMaster == "" || newMaster == oldMaster {
		t.Fatal("reset did not publish a new cross-signing identity through UIA")
	}
	if len(deleted) != 1 || deleted[0] != "1" || len(versions) != 1 || versions[0] == "1" {
		t.Fatalf("old backups not replaced: deleted=%v versions=%v", deleted, versions)
	}
	status, err := c.VerificationStatus(ctx)
	if err != nil || !status.Verified {
		t.Fatalf("device not verified after reset: %+v %v", status, err)
	}
	// La copia nueva abre con la clave nueva y la vieja ya no abre nada.
	if _, version, err := c.readBackup(ctx); err != nil || version != id.KeyBackupVersion(versions[0]) {
		t.Fatalf("new backup unusable: %v", err)
	}
	if _, err := c.recoveryStorageKey(ctx, forgotten); err == nil {
		t.Fatal("forgotten key still opens secret storage")
	}
	if err := c.Recover(ctx, fresh); err != nil {
		t.Fatalf("new key does not recover: %v", err)
	}
}

func TestResetRecoveryRefusesPrivateStorage(t *testing.T) {
	h := &resetHomeserver{accountData: map[string]json.RawMessage{}, deviceKeys: map[string]json.RawMessage{}, bodies: map[string]json.RawMessage{}}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	c := newResetClient(t, server, "drive")
	if _, err := c.ResetRecovery(t.Context(), dummyUIA); !errors.Is(err, ErrResetUnsupported) {
		t.Fatalf("private storage reset: %v", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.uiaDone || len(h.accountData) != 0 {
		t.Fatal("private storage reset touched the account")
	}
}
