// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"github.com/google/uuid"
	"maunium.net/go/mautrix"

	notesUI "git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/internal/nativestoragetest"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

type notesFixtureTransport struct {
	pages, opens, plaintext atomic.Int32
	registered              *atomic.Int32
}

func (t *notesFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/v1/private-storage/migration") {
		t.plaintext.Add(1)
		return nil, errors.New("private Notes fixture refused plaintext CRUD")
	}
	if strings.HasSuffix(path, "/rows") {
		t.pages.Add(1)
	}
	if strings.Contains(path, "/source/") {
		t.opens.Add(1)
	}
	if r.Method == http.MethodPost && t.registered.Load() == 0 {
		return nil, errors.New("source freeze preceded native owner registration")
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestPrivateNotepadsApplicationAgainstPostgresAndFreshDevice(t *testing.T) {
	if os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE") != "1" {
		t.Skip("requires the actual retained PostgreSQL source fixture")
	}
	if runtime.GOOS != "linux" {
		t.Skip("disposable remote Linux Factory/witness proof; native Apple/Binder probes are separate")
	}
	var scope storage.Scope
	if json.Unmarshal([]byte(os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_SCOPE")), &scope) != nil || !scope.Valid() || scope.Product != "notes" {
		t.Fatal("synthetic Notes scope required")
	}
	origin, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_SYNAPSE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	sourceOrigin, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	book, note := os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_BOOK"), os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_NOTE")
	if _, err := uuid.Parse(book); err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(note); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const password = "artificial-disposable-native-application-password"
	register := func(name string) *mautrix.Client {
		t.Helper()
		client, err := mautrix.NewClient(origin.String(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		matrixprooftest.Register(ctx, t, client, name, password, "REGISTRATION")
		return client
	}
	controller := register("rooms_controller")
	legacyUser := register("sytue_" + scope.Subject)
	isolatedUser := register(strings.TrimSuffix(strings.TrimPrefix(storage.IsolatedMatrixUserPrefix(scope), "@"), ":"))
	legacyRoom := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, legacyUser.UserID)
	targetRoom := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, isolatedUser.UserID)
	legacy, err := nativestoragetest.Open(ctx, t, nativestoragetest.Config{Scope: scope, Origin: origin.String(), User: legacyUser, Controller: controller, Room: legacyRoom, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	var encryptedPuts atomic.Int32
	targetConfig := nativestoragetest.Config{Scope: scope, Origin: origin.String(), User: isolatedUser, Controller: controller, Room: targetRoom, Password: password, Intercept: func(_ http.ResponseWriter, r *http.Request, _ *httputil.ReverseProxy) bool {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/send/m.room.encrypted/") {
			encryptedPuts.Add(1)
		}
		return false
	}}
	target, err := nativestoragetest.Open(ctx, t, targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	session := auth.NewSession(auth.Tokens{Issuer: "https://id.example.test", Cliente: "suite", Subject: scope.Subject, Organizacion: scope.Tenant, Access: "synthetic-notes-source", Expiry: time.Now().Add(time.Hour)}, nil, nil)
	var registered atomic.Int32
	ports, err := nativestoragetest.NewSessionPorts(ctx, session, "sesion-ffffffffffffffffffffffffffffffff", func(work offline.PrivateWork, _ auth.TestigoTrabajo) (func(), error) {
		if work.PrivateStorageScope() != scope {
			return nil, cifrado.ErrPrivateResources
		}
		registered.Add(1)
		return func() { registered.Add(-1) }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	factory := cifrado.Factory[*mx.Client]{Product: "notes", CaptureWork: ports.CaptureWork, RegisterWork: ports.RegisterWork, CurrentIdentity: ports.CurrentIdentity, BindHTTP: ports.BindHTTP, OpenJournal: func(ctx context.Context) (*offline.Store, error) { return journal, ctx.Err() }}
	devices := cifrado.Devices[*mx.Client]{Legacy: legacy.Client, Isolated: target.Client, RequireLegacy: true}
	transport := &notesFixtureTransport{registered: &registered}
	sourceClient := api.NewNotes(sourceOrigin.String(), session, &http.Client{Transport: transport})
	source, err := privateNotes.NewHTTPSource(sourceClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Open(session.WithAuthorizationIntent(ctx), devices); err == nil {
		t.Fatal("Factory accepted unacknowledged recovery")
	}
	state, err := source.State(ctx, scope)
	if err != nil || state.Mode != "legacy" || transport.pages.Load() != 0 || transport.opens.Load() != 0 {
		t.Fatal("failed resource verification froze or read source", err)
	}
	if err := target.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	opaque := &storagetest.CipherGateway{Product: "notes"}
	objects := httptest.NewServer(opaque)
	defer objects.Close()
	gateway := api.NewDrive(objects.URL, nil, objects.Client())
	routing := nativestoragetest.NewCutover(scope, controller, legacyRoom)
	resources, err := factory.Open(session.WithAuthorizationIntent(ctx), devices)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildNotesPlan(ctx, resources, source, gateway, routing)
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := cifrado.NewRepositoryCycle(resources, cifrado.RepositoryConfig[*notepadsRepository]{Create: func(context.Context) (*notepadsRepository, error) { return plan.Repository, nil }, Prepare: plan.Prepare})
	if err != nil {
		t.Fatal(err)
	}
	if err := cycle.Open(ctx); err != nil {
		t.Fatal("native complete-source preparation", err)
	}
	if registered.Load() < 1 || transport.pages.Load() == 0 || transport.opens.Load() == 0 {
		t.Fatal("real source or registered native owner was bypassed")
	}
	// The registered owner must preserve optional private view/export capabilities.
	facade, err := api.NewNotes(sourceOrigin.String(), session, &http.Client{Transport: transport}).WithPrivateData(plan.Repository)
	if err != nil {
		t.Fatal(err)
	}
	if tags, err := facade.ListTags(ctx); err != nil || len(tags) != 1 {
		t.Fatal("native owner hid private tag views", err, len(tags))
	}
	if reminders, err := facade.ListReminders(ctx); err != nil || len(reminders) != 1 {
		t.Fatal("native owner hid private reminders", err, len(reminders))
	}
	for _, format := range []string{"html", "md", "txt", "pdf", "onex"} {
		body, name, err := facade.ExportNotebook(ctx, book, note, format)
		if err != nil {
			t.Fatal("private export", format, err)
		}
		raw, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || len(raw) == 0 || name == "" {
			t.Fatal("empty device export", format, err)
		}
		if (format == "onex" || format == "html") && !bytes.Contains(raw, []byte("900719925474099312345")) {
			t.Fatal("private export changed retained data", format)
		}
		clear(raw)
	}
	application := fynetest.NewApp()
	defer application.Quit()
	window := application.NewWindow("Private Notepads proof")
	defer window.Close()
	translator, err := i18n.New("es", i18n.Common(), catalogos.Notes())
	if err != nil {
		t.Fatal(err)
	}
	screen, err := notesUI.NuevaPantallaPrivada(resources.Context(), plan.Repository.local, translator, "es", plan.Repository, nil, window)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fyne.DoAndWait(screen.Cerrar) })
	fyne.DoAndWait(func() {
		window.SetContent(screen.Objeto())
		window.Resize(fyne.NewSize(1280, 800))
		screen.Arrancar()
		screen.AtenderEnlace(&url.URL{Path: "/notebooks/" + book, RawQuery: "note=" + note})
	})
	const body = "Native queued E2E edit / 漢字"
	var editor *notesUI.EditorNota
	notesFixtureWait(ctx, t, func() bool {
		fyne.DoAndWait(func() {
			notesFixtureWalk(window.Content(), func(object fyne.CanvasObject) {
				if card, ok := object.(*notesUI.Tarjeta); ok && card.AlEditar != nil && editor == nil {
					card.AlEditar()
				}
			})
			notesFixtureWalk(window.Content(), func(object fyne.CanvasObject) {
				if card, ok := object.(*notesUI.Tarjeta); ok && card.Editor != nil {
					editor = card.Editor
				}
			})
		})
		return editor != nil
	})
	var enteredHTML string
	fyne.DoAndWait(func() {
		editor.PegarTexto(body)
		enteredHTML = editor.HTML()
		editor.Zona().TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	})
	if !strings.Contains(enteredHTML, body) {
		t.Fatal("real Fyne editor did not admit the requested text")
	}
	notesFixtureWait(ctx, t, func() bool {
		rows, err := plan.Repository.ListNotes(ctx, book)
		return err == nil && len(rows) == 1 && rows[0].ID == note && strings.Contains(rows[0].Content, body)
	})
	notesFixtureWait(ctx, t, func() bool {
		pending, err := plan.Repository.local.Pending(ctx)
		return err == nil && pending.Count == 0
	})
	fyne.DoAndWait(screen.Cerrar)
	if err := screen.Join(ctx); err != nil {
		t.Fatal(err)
	}
	if err := target.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cycle.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := plan.Retire(ctx); err != nil {
		t.Fatal(err)
	}
	if registered.Load() != 0 {
		t.Fatal("native owner/retained reader registration leaked", registered.Load())
	}
	// Replay exact acknowledged requests across a device change and local crashes.
	openIsolated := func(device *mx.Client) (*notepadsRepository, *cifrado.RepositoryCycle[*notepadsRepository], func(context.Context) error) {
		t.Helper()
		own, err := factory.Open(session.WithAuthorizationIntent(ctx), cifrado.Devices[*mx.Client]{Isolated: device})
		if err != nil {
			t.Fatal(err)
		}
		ownPlan, err := buildNotesPlan(ctx, own, source, gateway, routing)
		if err != nil {
			t.Fatal(err)
		}
		ownCycle, err := cifrado.NewRepositoryCycle(own, cifrado.RepositoryConfig[*notepadsRepository]{Create: func(context.Context) (*notepadsRepository, error) { return ownPlan.Repository, nil }, Prepare: ownPlan.Prepare})
		if err != nil {
			t.Fatal(err)
		}
		if err := ownCycle.Open(ctx); err != nil {
			t.Fatal(err)
		}
		return ownPlan.Repository, ownCycle, ownPlan.Retire
	}
	pendingRepo, pendingCycle, retirePending := openIsolated(target.Client)
	current, err := pendingRepo.ListNotes(ctx, book)
	if err != nil || len(current) != 1 {
		t.Fatal(err)
	}
	request := notas.EditorDraft{Version: 2, NoteID: note, NotebookID: book, Revision: uuid.NewString(), FromInput: true, Input: current[0].Input(), ExpectedUpdatedAt: current[0].UpdatedAt}
	request.Input.Content = "<p>accepted before local receipt failure</p>"
	expected := request.ExpectedUpdatedAt
	request.Input.ExpectedUpdatedAt = &expected
	draftID := "edicion:actual-ack-failure"
	if err := pendingRepo.local.Write(ctx, notas.EspacioBorradores, draftID, request); err != nil {
		t.Fatal(err)
	}
	delivery, err := pendingRepo.local.BeginEditorDelivery(ctx, draftID, request)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("synthetic local receipt IO failure")
	callbackCalls := 0
	if _, err := pendingRepo.UpdateNoteAcknowledged(ctx, note, book, request.Input, func(context.Context, api.Note) error { callbackCalls++; return injected }); !errors.Is(err, injected) || callbackCalls != 1 {
		t.Fatal("actual accepted write did not retain its failed local receipt", err, callbackCalls)
	}
	afterFailure, err := pendingRepo.ListNotes(ctx, book)
	if err != nil || len(afterFailure) != 1 || afterFailure[0].Content != request.Input.Content {
		t.Fatal("receipt failure did not occur after actual private publication", err)
	}
	if encryptedPuts.Load() == 0 {
		t.Fatal("the accepted private write did not exercise the encrypted PUT observer")
	}
	newer := request.Clone()
	newer.Revision = uuid.NewString()
	newer.Input.Content = "<p>newer edit survives receipt recovery / 漢字</p>"
	if err := pendingRepo.local.Write(ctx, notas.EspacioBorradores, draftID, newer); err != nil {
		t.Fatal(err)
	}
	if retained, err := pendingCycle.RetirePreservingWork(ctx); err != nil || retained.Count == 0 {
		t.Fatal("failed local receipt lost durable recovery work", retained, err)
	}
	if err := retirePending(ctx); err != nil {
		t.Fatal(err)
	}
	if err := target.Client.Close(); err != nil {
		t.Fatal(err)
	}
	targetConfig.Recovery = target.Recovery
	resumedDevice, err := nativestoragetest.Open(ctx, t, targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	if resumedDevice.Client.DeviceID() == target.Client.DeviceID() {
		t.Fatal("ACK recovery reused the first SDK device")
	}
	resumedRepo, resumedCycle, retireResumed := openIsolated(resumedDevice.Client)
	entries, err := resumedRepo.local.EditorDeliveries(ctx, draftID)
	if err != nil || len(entries) != 1 || !entries[0].AcceptedAt.IsZero() {
		t.Fatal("fresh device lost exact unacknowledged source", err)
	}
	delivery = entries[0]
	putsBeforeResume := encryptedPuts.Load()
	if _, found, err := resumedRepo.ResumeNoteAcknowledged(ctx, note, book, request.Input, func(ctx context.Context, accepted api.Note) error {
		return resumedRepo.local.MarkEditorDelivery(ctx, delivery, accepted)
	}); err != nil || !found {
		t.Fatal("new SDK could not verify the exact accepted request", found, err)
	}
	if encryptedPuts.Load() != putsBeforeResume {
		t.Fatal("accepted request retry published another encrypted event")
	}
	if err := resumedRepo.local.FinishEditorDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	rebased, found, err := resumedRepo.local.DurableEditor(ctx, draftID)
	if err != nil || !found || rebased.Revision != newer.Revision || rebased.EditorHTML() != newer.EditorHTML() || !rebased.ExpectedUpdatedAt.Equal(afterFailure[0].UpdatedAt) {
		t.Fatal("ACK recovery overwrote or failed to rebase newer typing", found, err)
	}
	rebasedAt := rebased.ExpectedUpdatedAt
	rebased.Input.ExpectedUpdatedAt = &rebasedAt
	finalDelivery, err := resumedRepo.local.BeginEditorDelivery(ctx, draftID, rebased)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resumedRepo.UpdateNoteAcknowledged(ctx, note, book, rebased.Input, func(ctx context.Context, accepted api.Note) error {
		return resumedRepo.local.MarkEditorDelivery(ctx, finalDelivery, accepted)
	}); err != nil {
		t.Fatal(err)
	}
	// Fail local cleanup after records returned success, then restart with only
	// the durable accepted receipt. Cancellation must never count as finalization.
	canceledCleanup, cancelCleanup := context.WithCancel(ctx)
	cancelCleanup()
	if err := resumedRepo.local.FinishEditorDelivery(canceledCleanup, finalDelivery); !errors.Is(err, context.Canceled) {
		t.Fatal("failed post-success cleanup was hidden", err)
	}
	if entries, err := resumedRepo.local.EditorDeliveries(ctx, draftID); err != nil || len(entries) != 1 || entries[0].AcceptedAt.IsZero() {
		t.Fatal("failed cleanup lost its accepted source", err)
	}
	if retained, err := resumedCycle.RetirePreservingWork(ctx); err != nil || retained.Count == 0 {
		t.Fatal("post-success crash lost its receipt", retained, err)
	}
	if err := retireResumed(ctx); err != nil {
		t.Fatal(err)
	}
	cleanupRepo, cleanupCycle, retireCleanup := openIsolated(resumedDevice.Client)
	entries, err = cleanupRepo.local.EditorDeliveries(ctx, draftID)
	if err != nil || len(entries) != 1 || entries[0].AcceptedAt.IsZero() {
		t.Fatal("restart lost the accepted local receipt", err)
	}
	putsBeforeCleanup := encryptedPuts.Load()
	if _, found, err := cleanupRepo.ResumeNoteAcknowledged(ctx, note, book, rebased.Input, func(context.Context, api.Note) error {
		t.Error("missing mutation WAL invoked an acknowledgment")
		return nil
	}); err != nil || found {
		t.Fatal("resume-only recreated a completed mutation", found, err)
	}
	if encryptedPuts.Load() != putsBeforeCleanup {
		t.Fatal("resume-only cleanup published another encrypted event")
	}
	if err := cleanupRepo.local.FinishEditorDelivery(ctx, entries[0]); err != nil {
		t.Fatal("accepted local receipt could not finish cleanup without CAS", err)
	}
	if err := cleanupCycle.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := retireCleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if registered.Load() != 0 {
		t.Fatal("ACK recovery leaked native owner registration", registered.Load())
	}
	if err := resumedDevice.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	recoveredBody := newer.Input.Content
	pages, opens := transport.pages.Load(), transport.opens.Load()
	targetConfig.Recovery = target.Recovery
	fresh, err := nativestoragetest.Open(ctx, t, targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Client.DeviceID() == target.Client.DeviceID() {
		t.Fatal("new device reused its predecessor identity")
	}
	journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	recoveredResources, err := factory.Open(session.WithAuthorizationIntent(ctx), cifrado.Devices[*mx.Client]{Isolated: fresh.Client})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := buildNotesPlan(ctx, recoveredResources, source, gateway, routing)
	if err != nil {
		t.Fatal(err)
	}
	recoveredCycle, err := cifrado.NewRepositoryCycle(recoveredResources, cifrado.RepositoryConfig[*notepadsRepository]{Create: func(context.Context) (*notepadsRepository, error) { return recovered.Repository, nil }, Prepare: recovered.Prepare})
	if err != nil {
		t.Fatal(err)
	}
	if err := recoveredCycle.Open(ctx); err != nil {
		t.Fatal("fresh SDK with empty journal", err)
	}
	rows, err := recovered.Repository.ListNotes(ctx, book)
	if err != nil || len(rows) != 1 || !strings.Contains(rows[0].Content, recoveredBody) || !bytes.Contains(rows[0].Data, []byte("900719925474099312345")) {
		t.Fatal("new device lost native edit or precise source JSON", err)
	}
	if pages != transport.pages.Load() || opens != transport.opens.Load() || transport.plaintext.Load() != 0 {
		t.Fatal("fresh private recovery depended on SQL body or plaintext CRUD")
	}
	trash, err := recovered.Repository.ListTrash(ctx, 10)
	if err != nil || len(trash) != 1 {
		t.Fatal("retained source trash disappeared", err)
	}
	for _, ciphertext := range opaque.Snapshot().Data {
		if bytes.Contains(ciphertext, []byte(body)) || bytes.Contains(ciphertext, []byte("private source sentinel")) {
			t.Fatal("opaque provider received plaintext Notes content")
		}
	}
	if err := recoveredCycle.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Retire(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("NOTEPADS_NATIVE_PG_HTTP_SDK_FRESH_PASS actual retained SQL, verified/recovery-acknowledged Factory, registered native owner, real Fyne editor queue, private mutation and empty-journal fresh SSSS; no plaintext CRUD or SQL body after recovery")
}

func notesFixtureWait(ctx context.Context, t *testing.T, ready func() bool) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("native Notepads proof did not reach its retained boundary", ctx.Err())
		}
	}
}

func notesFixtureWalk(object fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	seen := map[fyne.CanvasObject]bool{}
	var walk func(fyne.CanvasObject)
	walk = func(current fyne.CanvasObject) {
		if current == nil || seen[current] {
			return
		}
		seen[current] = true
		visit(current)
		if container, ok := current.(*fyne.Container); ok {
			for _, child := range container.Objects {
				walk(child)
			}
		} else if widget, ok := current.(fyne.Widget); ok {
			for _, child := range fynetest.WidgetRenderer(widget).Objects() {
				walk(child)
			}
		}
	}
	walk(object)
}
