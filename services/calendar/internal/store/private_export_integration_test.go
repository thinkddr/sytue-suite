// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Export is opt-in, synthetic and staged. Public Begin is still rejected in the
// calling test. This artifact is for cross-module SDK QA, never account readiness.
func exportCalendarStagedFixture(ctx context.Context, t *testing.T, st *Store, epoch uuid.UUID) {
	t.Helper()
	path := os.Getenv("SYTUE_CALENDAR_STAGED_VECTOR")
	if path == "" {
		return
	}
	if filepath.Dir(path) != "/tmp" || !strings.HasPrefix(filepath.Base(path), "sytue-calendar-private-staged-vector-") {
		t.Fatal("staged fixture export must use its dedicated temporary path")
	}
	state, err := st.PrivateStorageState(ctx, "alice")
	if err != nil || state.Mode != "frozen" || state.PrivateIngressReady || state.Rows != 6 {
		t.Fatal("staged fixture readiness changed", err)
	}
	rows := []model.Row{}
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := st.PrivateStorageSourcePage(ctx, "alice", epoch, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Rows {
			_, body, err := st.OpenPrivateStorageSource(ctx, "alice", epoch, item.Kind, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(body)
			closeErr := body.Close()
			if err != nil || closeErr != nil {
				t.Fatal("staged source stream failed", err, closeErr)
			}
			rows = append(rows, model.Row{Kind: item.Kind, ID: item.ID, JSON: raw})
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	defer func() {
		for _, row := range rows {
			clear(row.JSON)
		}
	}()
	if _, err := model.NewSnapshot(state.Scope, rows); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(struct {
		Staged bool                      `json:"staged_fixture"`
		State  storage.AppMigrationState `json:"state"`
		Rows   []model.Row               `json:"rows"`
	}{true, state, rows})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Log("CALENDAR_STAGED_VECTOR_EXPORTED rows=6 ingress=false")
}
