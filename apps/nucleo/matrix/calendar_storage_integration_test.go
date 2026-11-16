// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	matrixid "maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privatecalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type calendarStagedSource struct {
	Staged       bool                  `json:"staged_fixture"`
	Snapshot     records.SnapshotState `json:"state"`
	Rows         []model.Row           `json:"rows"`
	retired      bool
	pages, opens int
}

func (s *calendarStagedSource) State(_ context.Context, scope storage.Scope) (records.SnapshotState, error) {
	if !s.Staged || s.Snapshot.Validate(scope) != nil || s.Snapshot.PrivateIngressReady {
		return records.SnapshotState{}, records.ErrSource
	}
	return s.Snapshot, nil
}

func (*calendarStagedSource) Begin(context.Context, storage.Scope, string) (records.SnapshotState, error) {
	return records.SnapshotState{}, privatecalendar.ErrPrivateIngress
}

func (s *calendarStagedSource) Page(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (records.SnapshotPage, error) {
	s.pages++
	page := records.SnapshotPage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
	if s.retired || scope != s.Snapshot.Scope || epoch != s.Snapshot.Epoch {
		return page, records.ErrSource
	}
	for _, row := range s.Rows {
		if row.Kind < after.Kind || row.Kind == after.Kind && row.ID <= after.ID {
			continue
		}
		if len(page.Rows) == 2 {
			last := page.Rows[1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		sum := sha256.Sum256(row.JSON)
		page.Rows = append(page.Rows, storage.StreamSourceRow{Kind: row.Kind, ID: row.ID, Size: int64(len(row.JSON)), SHA256: hex.EncodeToString(sum[:])})
	}
	return page, page.Validate(scope, epoch, after)
}

func (s *calendarStagedSource) Open(_ context.Context, scope storage.Scope, epoch string, item records.SnapshotRow) (io.ReadCloser, error) {
	s.opens++
	if s.retired || scope != s.Snapshot.Scope || epoch != s.Snapshot.Epoch {
		return nil, records.ErrSource
	}
	for _, row := range s.Rows {
		if row.Kind == item.Kind && row.ID == item.ID {
			return io.NopCloser(bytes.NewReader(row.JSON)), nil
		}
	}
	return nil, records.ErrSource
}

func TestPrivateCalendarAgainstSynapseResumesStagedPostgresAndRecoversCRUD(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_CALENDAR") != "1" {
		t.Skip("requires disposable remote Synapse and synthetic staged PostgreSQL export")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	path := os.Getenv("SYTUE_CALENDAR_STAGED_VECTOR")
	if !strings.HasPrefix(path, "/tmp/sytue-calendar-private-staged-vector-") || strings.Contains(strings.TrimPrefix(path, "/tmp/"), "/") {
		t.Fatal("fixture vector must stay in its dedicated temporary path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var source calendarStagedSource
	if json.Unmarshal(raw, &source) != nil || !source.Staged || len(source.Rows) != 6 || source.Snapshot.Scope.Product != "calendar" || source.Snapshot.PrivateIngressReady {
		t.Fatal("invalid staged Calendar fixture")
	}
	slices.SortFunc(source.Rows, func(a, b model.Row) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	address := os.Getenv("SYTUE_CALENDAR_SYNAPSE_URL")
	target, err := url.Parse(address)
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.Port() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		t.Fatal("fixture Synapse must be explicit remote loopback")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), source.Snapshot.Scope, storage.IsolatedIdentity)
	client := device.Client
	defer func() { _ = client.Close() }()
	files, gateway, _, _ := fileFixture(t)
	gateway.product = "calendar"
	faults := &lostStorageIndexACK{Client: client, drop: true}
	open := func(journal *offline.Store, crypto records.Crypto) *privatecalendar.Repository {
		r, err := privatecalendar.New(privatecalendar.Config{Context: ctx, Scope: source.Snapshot.Scope, Crypto: crypto, Gateway: files.gateway, Journal: journal, Source: &source})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	repository := open(files.journal, faults)
	if err := repository.Initialize(ctx); !errors.Is(err, records.ErrNotReady) {
		t.Fatal("staged source opened before import", err)
	}
	if source.pages != 0 || source.opens != 0 {
		t.Fatal("initialize imported retained plaintext")
	}
	if err := repository.ImportLegacySnapshot(ctx); err == nil || faults.accepted == (storage.Reference{}) {
		t.Fatal("accepted source index ACK was not interrupted", err)
	}
	if snapshot, err := repository.Snapshot(ctx); err == nil || snapshot != nil {
		t.Fatal("partial import became visible")
	}
	repository.Close()
	repository = open(files.journal, faults)
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("staged source did not resume", err)
	}
	checkCalendarOriginals(ctx, t, repository, source.Rows)
	transport := &calendarNoLegacyTransport{}
	original := api.NewCalendars("https://calendar-legacy.invalid", nil, &http.Client{Transport: transport})
	private, err := original.WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	retainedID, reminderID := "", ""
	for _, row := range source.Rows {
		if row.Kind == "event" {
			retainedID = row.ID
		}
		if row.Kind == "reminder" {
			reminderID = row.ID
		}
	}
	retained, err := private.GetEvent(ctx, retainedID)
	if err != nil || retained.Reminders == nil || len(*retained.Reminders) != 1 {
		t.Fatal("original event/alarm lost", err)
	}
	input := retained.Input()
	input.Summary = "Calendar encrypted edit"
	input.Description += " / private post-import write"
	faults.mu.Lock()
	faults.drop = true
	faults.mu.Unlock()
	if _, err := private.UpdateEvent(ctx, retainedID, input); err == nil {
		t.Fatal("accepted mutation ACK was not interrupted")
	}
	repository.Close()
	repository = open(files.journal, faults)
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal("published mutation was not recoverable", err)
	}
	private, err = original.WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	if updated, err := private.UpdateEvent(ctx, retainedID, input); err != nil || updated.ID != retainedID || updated.Summary != input.Summary {
		t.Fatal("private mutation did not reconcile exact retry", err)
	}
	if err := private.DismissReminder(ctx, reminderID); err != nil {
		t.Fatal(err)
	}
	survivor := exercisePrivateCalendarCRUD(ctx, t, private)
	checkCalendarHistory(ctx, t, repository, source.Rows)
	if transport.calls.Load() != 0 {
		t.Fatal("private Calendar reached legacy transport")
	}
	if pending, err := repository.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("completed private work remained pending", pending.Count, err)
	}
	if err := repository.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.List(ctx); err == nil {
		t.Fatal("drained repository accepted work")
	}
	if err := repository.Resume(); err != nil {
		t.Fatal(err)
	}
	checkCalendarOpaqueProvider(ctx, t, client, device, gateway)
	postWrite, err := repository.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expectedRows := postWrite.Rows()
	defer func() {
		for _, row := range expectedRows {
			clear(row.JSON)
		}
	}()
	if err := client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	repository.Close()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	login, err := device.Matrix.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: device.Matrix.UserID.String()}, Password: device.Password, DeviceID: "CALENDAR_FRESH_DEVICE"})
	if err != nil {
		t.Fatal(err)
	}
	config := device.Config
	config.StoreDir = t.TempDir()
	config.Secrets = &memorySecrets{values: map[string][]byte{}}
	second := storageSynapseSession(ctx, t, config, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: config.Subject, Tenant: config.Tenant})
	defer func() { _ = second.Close() }()
	go func() { _ = second.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := second.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	fresh := open(journal, second)
	defer fresh.Close()
	if err := fresh.Initialize(ctx); err == nil {
		t.Fatal("token-only fresh device opened private Calendar")
	}
	if err := second.Recover(ctx, device.Recovery); err != nil {
		t.Fatal(err)
	}
	pages, opens := source.pages, source.opens
	source.retired = true
	if err := fresh.Initialize(ctx); err != nil {
		t.Fatal("empty-journal device did not recover encrypted Calendar", err)
	}
	freshAPI, err := original.WithPrivateData(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if actual, err := freshAPI.GetEvent(ctx, retainedID); err != nil || actual.Summary != input.Summary || actual.Description != input.Description {
		t.Fatal("fresh device lost edited retained event", err)
	}
	if actual, err := freshAPI.GetEvent(ctx, survivor.ID); err != nil || actual.Summary != survivor.Summary {
		t.Fatal("fresh device lost new event", err)
	}
	checkCalendarHistory(ctx, t, fresh, source.Rows)
	checkCalendarOriginals(ctx, t, fresh, expectedRows)
	if source.pages != pages || source.opens != opens {
		t.Fatal("fresh device reimported retained plaintext")
	}
	checkCalendarConflict(ctx, t, second, device.Room.String(), fresh)
	t.Log("CALENDAR_STAGED_PG_SDK_FRESH_PASS rows=6 Ready=false ingress=false personal CRUD, lost import/mutation ACK, exact history, opaque provider, fresh recovery and conflict verified; shared ingress/ACL/resources/subscriptions remain blocked")
}

type calendarNoLegacyTransport struct{ calls atomic.Int32 }

func (t *calendarNoLegacyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("private Calendar attempted legacy HTTP")
}

func checkCalendarOriginals(ctx context.Context, t *testing.T, r *privatecalendar.Repository, want []model.Row) {
	t.Helper()
	s, err := r.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows()) != len(want) {
		t.Fatal("retained row count changed")
	}
	for _, row := range want {
		got, ok := s.Row(row.Kind, row.ID)
		if !ok || !bytes.Equal(row.JSON, got.JSON) {
			t.Fatal("original SQL row changed", row.Kind)
		}
	}
}

func checkCalendarHistory(ctx context.Context, t *testing.T, r *privatecalendar.Repository, originals []model.Row) {
	t.Helper()
	s, err := r.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range originals {
		actual, ok := s.Row(row.Kind, row.ID)
		if !ok {
			t.Fatal("retained history omitted", row.Kind)
		}
		switch row.Kind {
		case "event":
			if !bytes.Contains(actual.JSON, []byte("9007199254740993")) {
				t.Fatal("future integer lost")
			}
		case "calendar", "reminder", "outbox":
			if !bytes.Equal(actual.JSON, row.JSON) {
				t.Fatal("untouched history changed", row.Kind)
			}
		}
	}
}

func checkCalendarOpaqueProvider(ctx context.Context, t *testing.T, client *Client, device storageSynapseDevice, gateway *fileGatewayFixture) {
	t.Helper()
	history := artificialCatalogHistory(ctx, t, client, device.Room.String())
	for _, entry := range history {
		stored, err := device.Matrix.GetEvent(ctx, device.Room, matrixid.EventID(entry.Reference.EventID))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(stored)
		if err != nil || stored.Type != event.EventEncrypted || bytes.Contains(raw, []byte("private post-import write")) || bytes.Contains(raw, []byte("Exact original")) {
			t.Fatal("Calendar plaintext reached Synapse", err)
		}
	}
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.data) == 0 {
		t.Fatal("large original ICS did not use encrypted file path")
	}
	for _, raw := range gateway.data {
		if bytes.Contains(raw, []byte("Exact original")) || bytes.Contains(raw, []byte("private post-import write")) {
			t.Fatal("Calendar plaintext reached object provider")
		}
	}
}

func checkCalendarConflict(ctx context.Context, t *testing.T, client *Client, room string, r *privatecalendar.Repository) {
	t.Helper()
	history := artificialCatalogHistory(ctx, t, client, room)
	heads, err := storage.CatalogHeads(r.PrivateStorageScope(), history)
	if err != nil {
		t.Fatal(err)
	}
	for _, head := range heads {
		var entity struct {
			Entity string `json:"entity"`
		}
		if json.Unmarshal(head.Commit.Metadata, &entity) != nil || entity.Entity != "object_state" {
			continue
		}
		target, err := client.StoragePublicationTarget()
		if err != nil {
			t.Fatal(err)
		}
		for _, side := range []string{"left", "right"} {
			commit := head.Commit
			commit.Parents = []storage.Reference{head.Reference}
			if _, err := client.PutStorageObjectAt(ctx, target, "calendar_fork_"+side, head.ObjectID, "calendar_fork_"+side, commit); err != nil {
				t.Fatal(err)
			}
		}
		if err := r.Initialize(ctx); !errors.Is(err, records.ErrConflict) {
			t.Fatal("conflicting private heads were silently selected", err)
		}
		if _, err := r.List(ctx); err == nil {
			t.Fatal("conflicted repository remained usable")
		}
		return
	}
	t.Fatal("no edited Calendar entity available for conflict test")
}

func exercisePrivateCalendarCRUD(ctx context.Context, t *testing.T, c *api.Calendars) api.Event {
	listed, err := c.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatal("trash calendar leaked into list", err)
	}
	book, err := c.CreateCalendar(ctx, api.CalendarInput{Name: "device-calendar", DisplayName: "Device Calendar", Timezone: "Europe/Madrid"})
	if err != nil {
		t.Fatal(err)
	}
	book, err = c.UpdateCalendar(ctx, book.ID, api.CalendarInput{DisplayName: "Renamed private calendar", Color: "#123456"})
	if err != nil || book.DisplayName != "Renamed private calendar" || book.Color != "#123456" {
		t.Fatal("calendar metadata edit failed", err)
	}
	start := time.Date(2027, 3, 22, 8, 0, 0, 0, time.UTC)
	minutes := []int{10}
	series, err := c.CreateEvent(ctx, book.ID, api.EventInput{UID: "calendar-private-series", Summary: "private search series", Description: "private post-import write", Start: start, End: start.Add(time.Hour), TZID: "Europe/Madrid", RRule: "FREQ=WEEKLY;COUNT=3", Reminders: &minutes})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := c.Instances(ctx, book.ID, start.Add(-time.Hour), start.AddDate(0, 0, 20))
	if err != nil || len(instances) != 3 || instances[0].Start.Hour() != 8 || instances[1].Start.Hour() != 7 {
		t.Fatal("recurrence/DST changed", len(instances), err)
	}
	if err := c.AmendRecurrence(ctx, book.ID, series.UID, instances[1].Start, api.RecorteExcluir); err != nil {
		t.Fatal(err)
	}
	instances, err = c.Instances(ctx, book.ID, start.Add(-time.Hour), start.AddDate(0, 0, 20))
	if err != nil || len(instances) != 2 {
		t.Fatal("recurrence edit did not preserve exclusion", len(instances), err)
	}
	single, err := c.CreateEvent(ctx, book.ID, api.EventInput{Summary: "private search single", Start: start.Add(2 * time.Hour), End: start.Add(3 * time.Hour), TZID: "Europe/Madrid"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.SearchEventsPage(ctx, "private search", 1, "")
	if err != nil || len(first.Events) != 1 || first.Siguiente == "" {
		t.Fatal("search first page invalid", err)
	}
	second, err := c.SearchEventsPage(ctx, "private search", 1, first.Siguiente)
	if err != nil || len(second.Events) != 1 || second.Events[0].ID == first.Events[0].ID || second.Siguiente != "" {
		t.Fatal("search lost or duplicated page", err)
	}
	if _, err := c.SearchEventsPage(ctx, "different search", 1, first.Siguiente); err == nil {
		t.Fatal("search cursor escaped its query")
	}
	if all, err := c.SearchEvents(ctx, "private search", 10); err != nil || len(all) != 2 {
		t.Fatal("search convenience lost events", err)
	}
	slots, err := c.SuggestSlots(ctx, start, start.Add(8*time.Hour), 30, "Europe/Madrid", "09:00", "18:00", false)
	if err != nil || len(slots) == 0 {
		t.Fatal("free/busy suggestion failed", err)
	}
	for _, slot := range slots {
		if slot.Start.Before(start.Add(time.Hour)) && slot.End.After(start) {
			t.Fatal("free/busy offered occupied time")
		}
	}
	ics, err := c.ExportICS(ctx, book.ID)
	if err != nil || !bytes.Contains(ics, []byte("UID:calendar-private-series")) || !bytes.Contains(ics, []byte("EXDATE")) || !bytes.Contains(ics, []byte("VALARM")) {
		t.Fatal("ICS export lost series/alarms", err)
	}
	if err := c.DeleteEvent(ctx, single.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetEvent(ctx, single.ID); err == nil {
		t.Fatal("deleted event remained visible")
	}
	trash, err := c.CreateCalendar(ctx, api.CalendarInput{Name: "device-trash"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := c.CreateEvent(ctx, trash.ID, api.EventInput{Summary: "hidden descendant", Start: start, End: start.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteCalendar(ctx, trash.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetEvent(ctx, hidden.ID); err == nil {
		t.Fatal("calendar deletion left visible descendants")
	}
	if invitations, err := c.Invitations(ctx, true); err != nil || len(invitations) != 0 {
		t.Fatal("staged fixture invented invitations", err)
	}
	if shares, err := c.Shares(ctx, book.ID); err != nil || len(shares) != 0 {
		t.Fatal("staged fixture invented shares", err)
	}
	if reminders, err := c.Reminders(ctx); err != nil || len(reminders) != 0 {
		t.Fatal("dismissed/future reminder became due", err)
	}
	exerciseCalendarDueReminder(ctx, t, c, book.ID)
	checkCalendarMissingCapabilities(ctx, t, c, book.ID, series.ID)
	return series
}

func exerciseCalendarDueReminder(ctx context.Context, t *testing.T, c *api.Calendars, calendar string) {
	t.Helper()
	start := time.Now().UTC().Add(4 * time.Second).Truncate(time.Second)
	minutes := []int{0}
	ev, err := c.CreateEvent(ctx, calendar, api.EventInput{Summary: "private due alarm", Start: start, End: start.Add(time.Hour), Reminders: &minutes})
	if err != nil {
		t.Fatal("private alarm creation failed", err)
	}
	timer := time.NewTimer(max(time.Until(start)+100*time.Millisecond, 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
	due, err := c.Reminders(ctx)
	if err != nil || len(due) != 1 || due[0].EventID != ev.ID {
		t.Fatal("device did not derive due reminder", len(due), err)
	}
	if err := c.DismissReminder(ctx, due[0].ID); err != nil {
		t.Fatal(err)
	}
	if due, err := c.Reminders(ctx); err != nil || len(due) != 0 {
		t.Fatal("dismissed device reminder reappeared", err)
	}
	if err := c.DeleteEvent(ctx, ev.ID); err != nil {
		t.Fatal(err)
	}
}

func checkCalendarMissingCapabilities(ctx context.Context, t *testing.T, c *api.Calendars, calendar, eventID string) {
	t.Helper()
	for _, check := range []struct {
		name string
		run  func() error
		want error
	}{
		{"invitation-reply", func() error { _, err := c.RespondInvitation(ctx, "invitation", api.InvitacionAceptada); return err }, privatecalendar.ErrPrivateIngress},
		{"partstat", func() error { return c.SetPartStat(ctx, eventID, api.AsistenciaSi) }, privatecalendar.ErrPrivateIngress},
		{"shared-inbox", func() error { _, err := c.SharedWithMe(ctx); return err }, records.ErrShared},
		{"share", func() error { return c.Share(ctx, calendar, "recipient@example.invalid", api.PermisoLeer) }, records.ErrShared},
		{"revoke", func() error { return c.RevokeShare(ctx, calendar, "share") }, records.ErrShared},
		{"subscribe", func() error {
			_, err := c.Subscribe(ctx, api.SubscriptionInput{URL: "https://feed.example.invalid/private"})
			return err
		}, privatecalendar.ErrPrivateSubscriptions},
		{"refresh", func() error { return c.RefreshSubscription(ctx, calendar) }, privatecalendar.ErrPrivateSubscriptions},
		{"resources", func() error { _, err := c.Resources(ctx, time.Time{}, time.Time{}); return err }, records.ErrShared},
		{"resource-create", func() error { _, err := c.CreateResource(ctx, api.ResourceInput{Name: "private room"}); return err }, records.ErrShared},
		{"resource-update", func() error {
			_, err := c.UpdateResource(ctx, "room", api.ResourceInput{Name: "private room"})
			return err
		}, records.ErrShared},
		{"resource-delete", func() error { return c.DeleteResource(ctx, "room") }, records.ErrShared},
		{"resource-book", func() error { _, err := c.BookResource(ctx, "room", api.Booking{UID: "private"}); return err }, records.ErrShared},
	} {
		if err := check.run(); !errors.Is(err, check.want) {
			t.Fatal("missing capability became success", check.name, err)
		}
	}
}
