// SPDX-License-Identifier: Apache-2.0
package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

func TestEncryptedBackupRejectedBeforeNativeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"notebook":{"title":"book"},"notas":[],"descartadas":0}`))
	}))
	defer server.Close()
	client := NewNotes(server.URL, nil, nil)
	const unsafe = `{"formato":"sytue.notepads","version":1,"notebook":{"title":"book","encrypted":true,"crypto":{"v":1,"kdf":"PBKDF2-SHA256","salt":"abcdefghijklmnop","iterations":600000,"check":"encv1:test"}},"notas":[{"kind":"text","content_cipher":"encv1:AAAA","links":[{"target_kind":"note","target_id":"x","label":"secret"}]}]}`
	for _, action := range []func() error{
		func() error { _, err := client.PreviewNotebook(t.Context(), []byte(unsafe), ""); return err },
		func() error { _, err := client.ImportNotebook(t.Context(), []byte(unsafe), ""); return err },
	} {
		if err := action(); !errors.Is(err, model.ErrEncryptedBackupReview) {
			t.Fatalf("unsafe backup: %v", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("private legacy link reached network")
	}
	clean := strings.Replace(unsafe, `,"links":[{"target_kind":"note","target_id":"x","label":"secret"}]`, "", 1)
	if _, err := client.PreviewNotebook(t.Context(), []byte(clean), ""); !errors.Is(err, model.ErrEncryptedBackupReview) {
		t.Fatalf("encrypted backup sent: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("encrypted metadata reached network")
	}
	const plain = `{"formato":"sytue.notepads","version":1,"notebook":{"title":"plain"},"notas":[]}`
	if _, err := client.PreviewNotebook(t.Context(), []byte(plain), ""); err != nil {
		t.Fatal("clear backup failed", err)
	}
	if calls.Load() != 1 {
		t.Fatal("clear backup was not forwarded")
	}
}
