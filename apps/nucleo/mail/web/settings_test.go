// SPDX-License-Identifier: Apache-2.0
package web

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func TestBrowserSettingsWireRetainsTheNativeCASRevisionAndEditorFields(t *testing.T) {
	revision := strings.Repeat("a", 64)
	original := api.MailFilter{ID: "11111111-1111-4111-8111-111111111111", Name: "artificial", Enabled: true, Priority: 7, MatchSubject: "artificial subject", PrivateRevision: revision}
	native, _ := json.Marshal(original)
	if strings.Contains(string(native), revision) {
		t.Fatal("test no longer exercises the native-only revision contract")
	}
	raw, err := json.Marshal(editFilter(original))
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := parameters[filterEditor](raw)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := incoming.value()
	if err != nil || !reflect.DeepEqual(original, restored) {
		t.Fatal("browser filter editor lost its compare-and-swap revision", err)
	}
	until := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	vacation := api.MailVacation{Enabled: true, Subject: "artificial subject", Body: "artificial private body", EndsAt: &until, PrivateRevision: revision}
	raw, err = json.Marshal(editVacation(vacation))
	if err != nil {
		t.Fatal(err)
	}
	edit, err := parameters[vacationEditor](raw)
	if err != nil {
		t.Fatal(err)
	}
	restoredVacation, err := edit.value()
	if err != nil || !reflect.DeepEqual(vacation, restoredVacation) {
		t.Fatal("browser vacation editor lost its compare-and-swap revision", err)
	}
}

func TestBrowserSettingsDoNotTreatMalformedRevisionsOrUnknownControlsAsAuthorization(t *testing.T) {
	for _, revision := range []string{"foreign", "https://other.invalid", strings.Repeat("a", 63), strings.Repeat("G", 64)} {
		if _, err := (filterEditor{Revision: revision}).value(); !errors.Is(err, webport.ErrRejected) {
			t.Fatal("unbounded filter revision admitted")
		}
		if _, err := (vacationEditor{Revision: revision}).value(); !errors.Is(err, webport.ErrRejected) {
			t.Fatal("unbounded vacation revision admitted")
		}
	}
	for _, raw := range []string{`{"revision":"","url":"https://other.invalid"}`, `{"private_revision":"forged"}`, `{"revision":""}{}`} {
		if _, err := parameters[filterEditor]([]byte(raw)); !errors.Is(err, webport.ErrRejected) {
			t.Fatal("unknown browser filter control admitted")
		}
		if _, err := parameters[vacationEditor]([]byte(raw)); !errors.Is(err, webport.ErrRejected) {
			t.Fatal("unknown browser vacation control admitted")
		}
	}
}
