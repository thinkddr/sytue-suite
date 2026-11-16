// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package cifrado

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	mailUI "git.sytue.eu/sytue/one/apps/fyne/apps/mail"
	"git.sytue.eu/sytue/one/apps/fyne/internal/nativestoragetest"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

type nativeMailHTTPBarrier struct {
	entered, canceled, release chan struct{}
	once                       sync.Once
}

func (b *nativeMailHTTPBarrier) finish() { b.once.Do(func() { close(b.release) }) }

type nativeMailFixtureTransport struct {
	base                             http.RoundTripper
	registered, earlyIO, sourceReads atomic.Int32
	barrier                          atomic.Pointer[nativeMailHTTPBarrier]
	loseSubmit, lostSubmit           atomic.Bool
	submitted                        atomic.Int32
}

func (t *nativeMailFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.HasPrefix(request.URL.Path, "/api/v1/private-storage/migration") {
		if t.registered.Load() != 1 {
			t.earlyIO.Add(1)
			return nil, errors.New("source IO preceded the unique owner registration")
		}
		if strings.Contains(request.URL.Path, "/source/") {
			t.sourceReads.Add(1)
		}
	}
	if strings.HasSuffix(request.URL.Path, "/private-storage/ingress/pending") {
		if b := t.barrier.Load(); b != nil {
			close(b.entered)
			<-request.Context().Done()
			close(b.canceled)
			<-b.release
			return nil, request.Context().Err()
		}
	}
	response, err := t.base.RoundTrip(request)
	if request.Method == http.MethodPost && request.URL.Path == "/api/v1/private-storage/submissions" {
		t.submitted.Add(1)
		if err == nil && t.loseSubmit.CompareAndSwap(true, false) {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			t.lostSubmit.Store(true)
			return nil, errors.New("synthetic lost HTTP ACK after actual private SMTP")
		}
	}
	return response, err
}

// SQL source rows are supplied by the explicitly labelled service fixture. This
// exercises actual PG authority/queue, routing fence, native Factory/holder,
// Mail repository/executor, SDK files/SSSS, presenter and final ACK together.
func TestNativeMailApplicationAgainstAuthorityQueueAndSynapse(t *testing.T) {
	runNativeMailApplicationCase(t, false)
}

// BrowserFull reuses the exact SQL18/3 import and an independently recovered
// native SDK. NativeFull separately keeps every original UI/ingress/SMTP case.
func TestNativeMailBrowserFullSourceAgainstAuthorityAndSynapse(t *testing.T) {
	runNativeMailApplicationCase(t, true)
}

func runNativeMailApplicationCase(t *testing.T, browserSource bool) {
	mode := os.Getenv("SYTUE_INGRESS_HANDOFF_CASE")
	if os.Getenv("SYTUE_INGRESS_DEVICE_PROOF") != "1" || (mode != "native-mail" && mode != "native-mail-sql") {
		t.Skip("requires disposable authority/queue/Synapse")
	}
	proofLimit := 5 * time.Minute
	if os.Getenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF") == "1" {
		proofLimit = 9 * time.Minute
	}
	webComposite := os.Getenv("SYTUE_MAIL_WEB_PROOF") == "1"
	if browserSource && !webComposite {
		t.Fatal("BrowserFull requires its exact real web fixture")
	}
	parentName := "TestPrivateIngressLiveNativeMailSharedOutbound"
	if browserSource {
		parentName = "TestPrivateIngressLiveNativeMailBrowserFullSource"
	}
	if webComposite {
		if os.Getenv("SYTUE_MAIL_WEB_PROOF_PARENT") != parentName || mode != "native-mail-sql" || os.Getenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF") != "1" {
			t.Fatal("web composite requires its exact native Mail parent")
		}
		proofLimit = 14 * time.Minute
	}
	if debug := os.Getenv("SYTUE_MAIL_WEB_DEBUG"); debug != "" && (debug != "1" || !webComposite) {
		t.Fatal("browser debug hold requires the exact web composite")
	}
	if strict := os.Getenv("SYTUE_MAIL_NATIVE_STRICT_PROOF"); strict != "" && (strict != "1" || !webComposite) {
		t.Fatal("native strict proof requires the exact web composite")
	}
	ctx, cancel := context.WithTimeout(t.Context(), proofLimit)
	defer cancel()
	stopProfile := func() {}
	if webComposite {
		profile, err := os.CreateTemp("", "sytue-mail-native-cpu-*.pprof")
		if err != nil {
			t.Fatal(err)
		}
		if err = pprof.StartCPUProfile(profile); err != nil {
			_ = profile.Close()
			t.Fatal(err)
		}
		var stopped sync.Once
		stopProfile = func() { stopped.Do(func() { pprof.StopCPUProfile(); _ = profile.Close() }) }
		t.Cleanup(stopProfile)
		t.Log("MAIL_NATIVE_CPU_PROFILE", profile.Name())
	}
	var vector matrixprooftest.IngressDeviceVector
	if err := json.Unmarshal([]byte(os.Getenv("SYTUE_INGRESS_DEVICE_VECTOR")), &vector); err != nil {
		t.Fatal(err)
	}
	apiOrigin, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_INGRESS_DEVICE_API_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := matrixprooftest.DisposableHTTPOrigin(vector.SynapseURL); err != nil {
		t.Fatal(err)
	}
	wantSourceReads := 1
	var expected *storagetest.MailSnapshot
	if mode == "native-mail-sql" {
		raw, err := os.ReadFile(os.Getenv("SYTUE_MAIL_PRIVATE_VECTOR"))
		if err != nil {
			t.Fatal(err)
		}
		value, err := storagetest.DecodeMailSnapshot(raw)
		clear(raw)
		if err != nil || value.State.Scope != vector.Scope || value.State.Epoch != vector.Epoch {
			t.Fatal("SQL export differs from the authority scope/epoch", err)
		}
		expected = &value
		wantSourceReads = len(value.Rows)
	}
	application := fynetest.NewApp()
	defer application.Quit()
	base, _, session, _ := factoryFixture(t)
	witness, err := base.CaptureWork(ctx, vector.Scope)
	if err != nil {
		t.Fatal(err)
	}
	tokens := factoryTokens(vector.Scope.Subject)
	tokens.Organizacion = vector.Scope.Tenant
	tokens.Access = "synthetic-live-device-session"
	factoryAdopt(t, witness, session, tokens)
	controllerID := "@rooms_controller:" + strings.TrimPrefix(vector.UserID, storage.IsolatedMatrixUserPrefix(vector.Scope))
	controller, err := mautrix.NewClient(vector.SynapseURL, id.UserID(controllerID), "")
	if err != nil {
		t.Fatal(err)
	}
	openDevice := func(userID, room, recovery string, intercept func(http.ResponseWriter, *http.Request, *httputil.ReverseProxy) bool) *nativestoragetest.Device {
		user, err := mautrix.NewClient(vector.SynapseURL, id.UserID(userID), "")
		if err != nil {
			t.Fatal(err)
		}
		device, err := nativestoragetest.Open(ctx, t, nativestoragetest.Config{Scope: vector.Scope, Origin: vector.SynapseURL, User: user, Controller: controller, Room: id.RoomID(room), Password: "artificial-disposable-ingress-password", Recovery: recovery, Intercept: intercept})
		if err != nil {
			t.Fatal(err)
		}
		return device
	}
	legacy := openDevice(vector.LegacyUserID, vector.LegacyRoomID, "", nil)
	target := openDevice(vector.UserID, vector.RoomID, "", nil)
	if err := legacy.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	devices := Devices[*mx.Client]{Legacy: legacy.Client, Isolated: target.Client, RequireLegacy: true}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	transport := &nativeMailFixtureTransport{base: http.DefaultTransport}
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second}
	opaque := &storagetest.CipherGateway{Product: "mail"}
	objects := httptest.NewServer(opaque)
	defer objects.Close()
	draftTransport := &nativeDraftBlobACK{base: http.DefaultTransport}
	objectsClient := objects.Client()
	objectsClient.Transport = draftTransport
	factory := Factory[*mx.Client]{Product: "mail", CaptureWork: base.CaptureWork, CurrentIdentity: base.CurrentIdentity, BindHTTP: base.BindHTTP, OpenJournal: func(ctx context.Context) (*offline.Store, error) { return journal, ctx.Err() }, RegisterWork: func(work offline.PrivateWork, _ auth.TestigoTrabajo) (func(), error) {
		if work.PrivateStorageScope() != vector.Scope || transport.registered.Add(1) != 1 {
			return nil, ErrPrivateResources
		}
		return func() { transport.registered.Add(-1) }, nil
	}}
	queue := make(chan func(), 256)
	state := ApplicationSignedOut
	var failure error
	stateChanges := 0
	mounts, detaches, prompts := 0, 0, 0
	var facade *api.Mail
	var browserRooms *api.Rooms
	var holder *MailApplicationRepository
	var repository *privateMail.Repository
	var presenter *mailUI.Buzon
	var mailWindow fyne.Window
	consent := NewIngressConsent(apiOrigin.String())
	host := &privateApplication[*mx.Client, *MailApplicationRepository]{}
	host.cfg = privateApplicationConfig[*mx.Client, *MailApplicationRepository]{
		identity: func() (auth.IdentitySnapshot, bool) { return session.SnapshotIdentity(), true }, capture: session.WithAuthorizationIntent, dispatch: func(fn func()) { queue <- fn }, replace: func(fyne.CanvasObject) {}, state: func(next ApplicationState, err error) { state = next; failure = err; stateChanges++ },
		setup: func(_ context.Context, ready func(context.Context, *Resources[*mx.Client]), failed func(error)) (applicationPanels, func(context.Context) error, error) {
			panels := &applicationSDKPanels{dispatch: func(fn func()) { queue <- fn }, failed: failed, open: func(ctx context.Context) error {
				resources, err := factory.Open(ctx, devices)
				if err != nil {
					return err
				}
				queue <- func() { ready(resources.Context(), resources) }
				return nil
			}}
			closing := devices
			return panels, func(context.Context) error {
				if closing.Legacy != nil {
					return errors.Join(closing.Legacy.Close(), closing.Isolated.Close())
				}
				return closing.Isolated.Close()
			}, nil
		},
		beforeOpen: func(ctx context.Context, resources *Resources[*mx.Client]) error {
			// RootConsent itself uses the bound HTTP client. The fixture approval
			// is the test user's explicit choice; the real Panel has its own proof.
			return consent.BeforeOpen(ctx, resources, func(ctx context.Context, request matrixproof.RootEnrollment) error {
				prompts++
				if request.Scope != vector.Scope || request.Target.DeviceID != resources.Devices.Isolated.DeviceID() {
					return ErrPrivateResources
				}
				return ctx.Err()
			})
		},
		build: func(ctx context.Context, resources *Resources[*mx.Client]) (ApplicationPlan[*MailApplicationRepository], error) {
			authority, err := consent.Authority(ctx, resources)
			if err != nil {
				return ApplicationPlan[*MailApplicationRepository]{}, err
			}
			return NewMailApplicationPlan(ctx, resources, MailApplicationConfig{Gateway: api.NewDrive(objects.URL, resources.HTTP(), objectsClient), Source: api.NewMail(apiOrigin.String(), resources.HTTP(), client), Rooms: api.NewRooms(apiOrigin.String(), resources.HTTP(), client), Authority: authority})
		},
		mount: func(ctx context.Context, resources *Resources[*mx.Client], value *MailApplicationRepository) (ApplicationView, error) {
			core, err := value.ReadyRepository(ctx)
			if err != nil {
				return ApplicationView{}, err
			}
			browserRooms = api.NewRooms(apiOrigin.String(), resources.HTTP(), client)
			facade, err = api.NewMail(apiOrigin.String(), resources.HTTP(), client).WithPrivateData(core)
			if err != nil {
				return ApplicationView{}, err
			}
			translator, err := mailUI.Traductor("en")
			if err != nil {
				return ApplicationView{}, err
			}
			screen := mailUI.NuevaPantalla(translator, "en")
			box := mailUI.NuevoBuzon(screen, facade)
			box.Lanzar = func(fn func()) { queue <- fn }
			box.Fondo = func(func()) {}
			box.Esperar = func(time.Duration, func()) func() { return func() {} }
			presenter = box
			if mailWindow == nil {
				mailWindow = application.NewWindow("Synthetic private Mail")
				t.Cleanup(mailWindow.Close)
			}
			mailWindow.SetContent(screen.Objeto())
			mailWindow.Resize(fyne.NewSize(1280, 800))
			box.CambiarSesion(ctx, vector.Scope.Tenant+"\x00"+vector.Scope.Subject)
			holder, repository = value, core
			mounts++
			return ApplicationView{Object: screen.Objeto(), Detach: func() { detaches++; box.Parar() }}, nil
		},
	}
	pump := func(done func() bool) {
		t.Helper()
		limit := 3 * time.Minute
		measuredOrigin := webComposite
		if measuredOrigin {
			limit = 14 * time.Minute
		}
		deadline, stop := context.WithTimeout(ctx, limit)
		defer stop()
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		started := time.Now()
		captured := false
		lastProgress := started
		var previous [7]int64
		for !done() {
			select {
			case fn := <-queue:
				fn()
			case <-tick.C:
				stats, available := opaque.TryCounters()
				t.Log("MAIL_NATIVE_PROGRESS", int(time.Since(started).Seconds()), state, transport.sourceReads.Load(), stats, available, mounts)
				if available {
					current := [7]int64{int64(state), int64(transport.sourceReads.Load()), int64(stats.Puts), int64(stats.Gets), int64(stats.Blobs), int64(mounts), int64(transport.registered.Load())}
					if current != previous {
						lastProgress = time.Now()
						previous = current
					}
				}
				if !captured && time.Since(started) > 40*time.Second {
					captured = true
					file, err := os.CreateTemp("", "sytue-mail-native-stacks-*.txt")
					if err == nil {
						_ = pprof.Lookup("goroutine").WriteTo(file, 2)
						_ = file.Close()
						t.Log("MAIL_NATIVE_STACKS", file.Name())
					}
				}
				if measuredOrigin && time.Since(lastProgress) > 45*time.Second {
					t.Fatal("native Mail origin proof stopped progressing", state, transport.sourceReads.Load(), stats, available, mounts)
				}
			case <-deadline.Done():
				stats, available := opaque.TryCounters()
				t.Fatal("native Mail host did not reach its retained boundary", state, transport.sourceReads.Load(), transport.registered.Load(), stats, available, mounts)
			}
		}
		t.Log("MAIL_NATIVE_BOUNDARY_SECONDS", time.Since(started).Seconds(), state, mounts)
	}
	post := func(path string) {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiOrigin.String()+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer synthetic-live-device-session")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatal("fixture control failed", path, response.StatusCode)
		}
	}
	host.adopt()
	pump(func() bool { return state == ApplicationFailed })
	if mounts != 0 || prompts != 0 || transport.sourceReads.Load() != 0 || transport.registered.Load() != 0 {
		t.Fatal("unacknowledged recovery mounted or read a source")
	}
	if err := target.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	previousState := stateChanges
	host.retry()
	pump(func() bool {
		return stateChanges > previousState && (state == ApplicationReady || state == ApplicationFailed)
	})
	if state != ApplicationReady || mounts != 1 || prompts != 1 || transport.registered.Load() != 1 || transport.sourceReads.Load() != int32(wantSourceReads) || transport.earlyIO.Load() != 0 {
		t.Fatal("verified Mail did not mount only after complete source receipt", state, mounts, prompts, transport.registered.Load(), transport.sourceReads.Load(), transport.earlyIO.Load(), failure)
	}
	assertImported := func() {
		t.Helper()
		if expected == nil {
			return
		}
		original, ok := expected.Body("mime_message", expected.LiveID)
		if !ok {
			t.Fatal("SQL export omitted original MIME")
		}
		body, err := facade.Raw(ctx, expected.LiveID)
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(body)
		closeErr := body.Close()
		defer clear(got)
		if readErr != nil || closeErr != nil || !bytes.Equal(got, original.Body) {
			t.Fatal("private native inbox changed original SQL MIME", readErr, closeErr)
		}
		signature, err := facade.Signature(ctx)
		if err != nil || signature != expected.Signature {
			t.Fatal("private native inbox lost its original large signature", err)
		}
		if body, err := facade.Raw(ctx, expected.DeletedID); err == nil {
			_ = body.Close()
			t.Fatal("deleted historical MIME became a visible message")
		}
	}
	assertImported()
	t.Log("MAIL_NATIVE_SQL_IMPORTED")
	recovered := target
	recoverNativeScope := func() {
		expectedMounts := mounts + 1
		post("/fixture/retire-source")
		var keyWrites atomic.Int32
		fresh := openDevice(vector.UserID, vector.RoomID, target.Recovery, func(_ http.ResponseWriter, r *http.Request, _ *httputil.ReverseProxy) bool {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/account_data/org.sytue.ingress.key.v1.") {
				keyWrites.Add(1)
			}
			return false
		})
		if err := fresh.Client.AcknowledgeRecoverySetup(ctx); err != nil {
			t.Fatal(err)
		}
		journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
		if err != nil {
			t.Fatal(err)
		}
		if count, err := journal.CountOwner(vector.Scope.Tenant+"\x00"+vector.Scope.Subject+"\x00mail", storage.PrivateJournalNamespaces("mail")...); err != nil || count != 0 {
			t.Fatal("fresh device did not start with an empty journal", count, err)
		}
		devices = Devices[*mx.Client]{Isolated: fresh.Client}
		previousReads := transport.sourceReads.Load()
		previousPrompts := prompts
		host = &privateApplication[*mx.Client, *MailApplicationRepository]{cfg: host.cfg}
		host.adopt()
		pump(func() bool { return mounts == expectedMounts || state == ApplicationFailed })
		if state != ApplicationReady || transport.sourceReads.Load() != previousReads || prompts != previousPrompts || keyWrites.Load() != 0 {
			t.Fatal("fresh native device replaced enrollment or recaptured SQL", state, keyWrites.Load(), prompts, transport.sourceReads.Load())
		}
		recovered = fresh
	}

	if browserSource {
		if expected == nil || len(expected.Rows) != 18 || expected.State.Blobs != 3 {
			t.Fatal("BrowserFull requires the unchanged original SQL18/3 source")
		}
		host.close()
		pump(func() bool {
			select {
			case <-host.previous:
				return true
			default:
				return false
			}
		})
		if transport.registered.Load() != 0 {
			t.Fatal("BrowserFull initial SDK did not drain")
		}
		recoverNativeScope()
		assertImported()
		stopProfile()
		verifyMailBrowserSession(ctx, t, vector, recovered.Config, recovered.Recovery, apiOrigin.String(), objects.URL, expected.LiveID, facade, browserRooms, holder)
		host.close()
		pump(func() bool {
			select {
			case <-host.previous:
				return true
			default:
				return false
			}
		})
		if transport.earlyIO.Load() != 0 || transport.registered.Load() != 0 || detaches != 2 {
			t.Fatal("BrowserFull escaped original source/session cleanup")
		}
		t.Log("NATIVE_MAIL_BROWSER_FULL_SOURCE_MIME_RELOAD_RETAINED_ACK_PASS")
		return
	}

	var draftID string
	var draftMIME []byte
	if expected != nil {
		primary, _, err := facade.Aliases(ctx)
		if err != nil || primary == "" {
			t.Fatal("native composer did not recover its authenticated sender", err)
		}
		draftMIME, err = eml.Componer(eml.Salida{De: primary, Asunto: "PRIVATE-NATIVE-DRAFT-MARKER", Texto: "Original private native draft", Cco: []string{"hidden@example.test"}, Borrador: true, Adjuntos: []eml.Adjunto{{Nombre: "private.txt", Tipo: "text/plain", Datos: []byte("PRIVATE-NATIVE-DRAFT-ATTACHMENT")}}}, time.Date(2026, 10, 30, 10, 20, 30, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		defer clear(draftMIME)
		beforeDraft := opaque.Snapshot()
		editor, private := facade.PrivateDraftRevisions()
		if !private || editor == nil {
			t.Fatal("native composer has no admitted revision port")
		}
		attempt, err := editor.PrepareDraftRevision(ctx, api.MailDraftRevision{Editor: "22222222-2222-4222-8222-222222222222", Revision: "33333333-3333-4333-8333-333333333333", From: primary, MIME: draftMIME})
		if err != nil {
			t.Fatal("native composer did not capture its immutable revision", err)
		}
		if prepared := opaque.Snapshot(); prepared.Puts != beforeDraft.Puts {
			t.Fatal("native draft was uploaded before its original was admitted")
		}
		if pending, err := holder.Pending(ctx); err != nil || pending.Count != 1 {
			t.Fatal("native draft capture lost retained SDK ownership", pending, err)
		}
		draft, err := editor.CommitDraftRevision(ctx, attempt)
		if err != nil {
			t.Fatal("native composer did not retain its private draft", err)
		}
		draftID = draft.ID
		again, err := editor.CommitDraftRevision(ctx, attempt)
		afterDraft := opaque.Snapshot()
		if err != nil || again.ID != draftID || afterDraft.Puts != beforeDraft.Puts+1 || len(afterDraft.Data) != len(beforeDraft.Data)+1 {
			t.Fatal("native draft retry duplicated exact MIME", err)
		}
	}
	assertDraft := func() {
		t.Helper()
		if draftID == "" {
			return
		}
		body, err := facade.Raw(ctx, draftID)
		if err != nil {
			t.Fatal(err)
		}
		actual, readErr := io.ReadAll(body)
		closeErr := body.Close()
		defer clear(actual)
		if readErr != nil || closeErr != nil || !bytes.Equal(actual, draftMIME) {
			t.Fatal("fresh private native composer lost its exact draft", readErr, closeErr)
		}
		parsed, err := eml.Leer(bytes.NewReader(actual))
		defer func() {
			for _, part := range parsed.Adjuntos {
				clear(part.Datos)
			}
		}()
		if err != nil || len(parsed.Cco) != 1 || parsed.Cco[0] != "hidden@example.test" || len(parsed.Adjuntos) != 1 || string(parsed.Adjuntos[0].Datos) != "PRIVATE-NATIVE-DRAFT-ATTACHMENT" {
			t.Fatal("native draft recovery lost hidden recipients or attachments", err)
		}
	}
	assertDraft()
	t.Log("MAIL_NATIVE_DURABLE_REVISION_PORT_ACKNOWLEDGED")
	var uiDraftID string
	var uiDraftMIME []byte
	if expected != nil {
		uiDraftID, uiDraftMIME = exerciseNativeDraftSave(ctx, t, presenter, mailWindow, facade, holder, opaque, draftTransport, queue)
		defer clear(uiDraftMIME)
	}
	assertUIDraft := func() {
		t.Helper()
		if uiDraftID == "" {
			return
		}
		body, err := facade.Raw(ctx, uiDraftID)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(body)
		closeErr := body.Close()
		defer clear(raw)
		if err != nil || closeErr != nil || !bytes.Equal(raw, uiDraftMIME) {
			t.Fatal("fresh SSSS device lost the exact draft saved through the UI", err, closeErr)
		}
	}
	assertUIDraft()
	baselineProvider := opaque.Snapshot()
	post("/fixture/seal")
	runReceiver := func(expectFailure bool) {
		t.Helper()
		life, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		result := make(chan error, 1)
		outcome := make(chan bool, 1)
		go func() {
			result <- holder.RunReceiver(life, func(got ingress.PollResult, err error) {
				outcome <- got.Attempted == 1 && ((expectFailure && got.Completed == 0 && err != nil) || (!expectFailure && got.Completed == 1 && err == nil))
				stop()
			})
		}()
		select {
		case valid := <-outcome:
			if !valid {
				t.Fatal("receiver did not preserve original accepted ACK outcome")
			}
		case <-life.Done():
			t.Fatal("receiver did not retry the retained accepted ACK", life.Err())
		}
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatal("receiver cancellation did not join its owner", err)
		}
	}
	runReceiver(true)
	if pending, err := holder.Pending(ctx); err != nil || pending.Count == 0 {
		t.Fatal("accepted lost ACK discarded native WAL", pending, err)
	}
	runReceiver(false)
	if pending, err := holder.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("accepted ACK could not retire native WAL", pending, err)
	}
	const original = "From: synthetic@example.invalid\r\nSubject: PRIVATE-LIVE-MIME-MARKER\r\n\r\nOriginal MIME / 漢字 é\r\n"
	assertMIME := func() {
		t.Helper()
		body, err := facade.Raw(ctx, "native_received")
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(body)
		closeErr := body.Close()
		defer clear(got)
		if readErr != nil || closeErr != nil || !bytes.Equal(got, []byte(original)) {
			t.Fatal("Mail MIME changed across authorized ingress/recovery", readErr, closeErr)
		}
	}
	assertMIME()
	provider := opaque.Snapshot()
	if provider.Puts != baselineProvider.Puts+1 || len(provider.Data) != len(baselineProvider.Data)+1 {
		t.Fatal("lost ACK duplicated actual MIME ciphertext", provider.Puts-baselineProvider.Puts, len(provider.Data)-len(baselineProvider.Data))
	}
	for _, cipher := range provider.Data {
		if bytes.Contains(cipher, []byte("PRIVATE-LIVE")) || bytes.Contains(cipher, []byte("Original MIME")) || bytes.Contains(cipher, []byte("PRIVATE-NATIVE-DRAFT")) || bytes.Contains(cipher, []byte("PRIVATE-NATIVE-UI-DRAFT")) {
			t.Fatal("provider received plaintext MIME")
		}
	}
	var outboundProof *nativeSharedOutboundProof
	if os.Getenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF") == "1" {
		outboundProof = prepareNativeSharedOutbound(ctx, t, repository, facade, expected, opaque)
	}
	barrier := &nativeMailHTTPBarrier{entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(barrier.finish)
	transport.barrier.Store(barrier)
	receiverDone := make(chan error, 1)
	go func() { receiverDone <- holder.RunReceiver(host.active.resources.Context(), nil) }()
	select {
	case <-barrier.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	host.close()
	select {
	case <-barrier.canceled:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if transport.registered.Load() != 1 || detaches != 1 {
		t.Fatal("native retirement released registered SDK work before HTTP cleanup")
	}
	barrier.finish()
	if err := <-receiverDone; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	pump(func() bool {
		select {
		case <-host.previous:
			return true
		default:
			return false
		}
	})
	if transport.registered.Load() != 0 {
		t.Fatal("joined native owner remained registered")
	}
	transport.barrier.Store(nil)
	recoverNativeScope()
	assertMIME()
	assertImported()
	assertDraft()
	assertUIDraft()
	if repository == nil {
		t.Fatal("native presenter lacks its verified repository")
	}
	if outboundProof != nil {
		completeNativeSharedOutbound(ctx, t, outboundProof, repository, facade, opaque, transport, client, apiOrigin.String())
	}
	if os.Getenv("SYTUE_MAIL_WEB_PROOF") == "1" {
		stopProfile()
		verifyMailBrowserSession(ctx, t, vector, recovered.Config, recovered.Recovery, apiOrigin.String(), objects.URL, "native_received", facade, browserRooms, holder)
	}
	host.close()
	pump(func() bool {
		select {
		case <-host.previous:
			return true
		default:
			return false
		}
	})
	if transport.earlyIO.Load() != 0 || transport.registered.Load() != 0 || detaches != 2 {
		t.Fatal("native Mail escaped its source/session boundary")
	}
	t.Log("NATIVE_MAIL_AUTHORITY_QUEUE_SDK_MIME_PASS; historical HTTP source is a conformance fixture (optionally backed by actual PG snapshot); real authority/PG queue/Factory/holder/SDK/executor/MIME/ACK loss/join/fresh SSSS recovery, production readiness remains disabled")
}
