// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package cifrado

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/contactos"
	"git.sytue.eu/sytue/one/apps/fyne/internal/nativestoragetest"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateContacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	nativeIngress "git.sytue.eu/sytue/one/apps/nucleo/ingress"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const applicationPassword = "artificial-disposable-native-application-password"

type applicationSDKDevice struct {
	client       *mx.Client
	recovery     string
	room         id.RoomID
	config       mx.Config
	dropWrite    atomic.Bool
	lostWrites   atomic.Int32
	lostAccepted atomic.Pointer[context.CancelFunc]
}

func openApplicationSDKDevice(ctx context.Context, t *testing.T, origin string, scope storage.Scope, user, controller *mautrix.Client, room id.RoomID, recovery string) *applicationSDKDevice {
	t.Helper()
	device := &applicationSDKDevice{}
	actual, err := nativestoragetest.Open(ctx, t, nativestoragetest.Config{Scope: scope, Origin: origin, User: user, Controller: controller, Room: room, Password: applicationPassword, Recovery: recovery, Intercept: func(w http.ResponseWriter, r *http.Request, forward *httputil.ReverseProxy) bool {
		if !device.dropWrite.Load() || r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/send/m.room.encrypted/") {
			return false
		}
		recorded := httptest.NewRecorder()
		forward.ServeHTTP(recorded, r)
		if recorded.Code >= 200 && recorded.Code < 300 {
			device.lostWrites.Add(1)
			if cancel := device.lostAccepted.Load(); cancel != nil {
				(*cancel)()
			}
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		return true
	}})
	if err != nil {
		t.Fatal("public SDK device", err)
	}
	device.client, device.recovery, device.room, device.config = actual.Client, actual.Recovery, actual.Room, actual.Config
	return device
}

type applicationRetainedSource struct {
	mu        sync.Mutex
	state     records.SnapshotState
	row       records.SnapshotRow
	body      []byte
	opens     int
	retired   bool
	refreshes int
}

func (s *applicationRetainedSource) State(ctx context.Context, scope storage.Scope) (records.SnapshotState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scope != s.state.Scope {
		return records.SnapshotState{}, records.ErrSource
	}
	return s.state, ctx.Err()
}

func (s *applicationRetainedSource) RefreshState(ctx context.Context, scope storage.Scope) (records.SnapshotState, error) {
	s.mu.Lock()
	s.refreshes++
	s.mu.Unlock()
	return s.State(ctx, scope)
}

func (s *applicationRetainedSource) Begin(ctx context.Context, scope storage.Scope, epoch string) (records.SnapshotState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scope != s.state.Scope || s.retired {
		return records.SnapshotState{}, records.ErrSource
	}
	if s.state.Mode == "legacy" {
		now := time.Now().UTC()
		s.state.Mode = "frozen"
		s.state.Epoch = epoch
		s.state.StartedAt = &now
		s.state.Rows = 1
	}
	return s.state, ctx.Err()
}

func (s *applicationRetainedSource) Page(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (records.SnapshotPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := records.SnapshotPage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
	if s.retired || scope != s.state.Scope || epoch != s.state.Epoch {
		return page, records.ErrSource
	}
	if after.Kind == "" {
		page.Rows = append(page.Rows, s.row)
	}
	return page, ctx.Err()
}

func (s *applicationRetainedSource) Open(ctx context.Context, scope storage.Scope, epoch string, row records.SnapshotRow) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retired || scope != s.state.Scope || epoch != s.state.Epoch || row.Kind != s.row.Kind || row.ID != s.row.ID {
		return nil, records.ErrSource
	}
	s.opens++
	return io.NopCloser(bytes.NewReader(s.body)), ctx.Err()
}

type applicationRouting = nativestoragetest.Cutover

// The actual app host, Factory, domain constructors, historical cutover and
// WithPrivateData run together. Only disposable SSO/routing/source ports are
// fixtures; no Verified, key, Ready or private SDK state is manufactured.
func TestPrivateContactsApplicationAgainstSynapseRequiresRecoveryAndSurvivesRestart(t *testing.T) {
	if os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	origin, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION_URL"))
	if err != nil {
		t.Fatal(err)
	}
	application := fynetest.NewApp()
	defer application.Quit()
	factoryBase, _, session, _ := factoryFixture(t)
	scope := storage.Scope{Tenant: "acme", Subject: session.Subject(), Product: "contacts"}
	register := func(username string) *mautrix.Client {
		t.Helper()
		client, err := mautrix.NewClient(origin.String(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		matrixprooftest.Register(ctx, t, client, username, applicationPassword, "REGISTRATION")
		return client
	}
	controller := register("rooms_controller")
	legacyUser := register("sytue_" + scope.Subject)
	isolatedUser := register(strings.TrimSuffix(strings.TrimPrefix(storage.IsolatedMatrixUserPrefix(scope), "@"), ":"))
	sourceRoom := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, legacyUser.UserID)
	targetRoom := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, isolatedUser.UserID)
	legacy := openApplicationSDKDevice(ctx, t, origin.String(), scope, legacyUser, controller, sourceRoom, "")
	target := openApplicationSDKDevice(ctx, t, origin.String(), scope, isolatedUser, controller, targetRoom, "")
	if err := legacy.client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	var journalOpens atomic.Int32
	factory := Factory[*mx.Client]{Product: scope.Product, CaptureWork: factoryBase.CaptureWork, RegisterWork: factoryBase.RegisterWork, CurrentIdentity: factoryBase.CurrentIdentity, BindHTTP: factoryBase.BindHTTP, OpenJournal: func(ctx context.Context) (*offline.Store, error) { journalOpens.Add(1); return journal, ctx.Err() }}
	book := uuid.NewString()
	raw, _ := json.Marshal(map[string]any{"id": book, "tenant": scope.Tenant, "owner_sub": scope.Subject, "name": "personal", "display_name": "Retained private book", "is_default": true, "ctag": "original", "sync_token": 1, "future_integer": json.Number("900719925474099312345")})
	hash := sha256.Sum256(raw)
	source := &applicationRetainedSource{state: records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, row: records.SnapshotRow{Kind: "book", ID: book, Size: int64(len(raw)), SHA256: hex.EncodeToString(hash[:])}, body: raw}
	routing := nativestoragetest.NewCutover(scope, controller, sourceRoom)
	var plaintext atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plaintext.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	gateway := api.NewDrive(server.URL, nil, server.Client())
	translator, err := i18n.New("es", i18n.Common(), catalogos.Contacts())
	if err != nil {
		t.Fatal(err)
	}
	devices := Devices[*mx.Client]{Legacy: legacy.client, Isolated: target.client, RequireLegacy: true}
	queue := make(chan func(), 64)
	state := ApplicationSignedOut
	var facade *api.Contacts
	var screen *contactos.App
	mounts := 0
	host := &privateApplication[*mx.Client, *privateContacts.Repository]{}
	host.cfg = privateApplicationConfig[*mx.Client, *privateContacts.Repository]{
		identity: factory.CurrentIdentity, capture: session.WithAuthorizationIntent, dispatch: func(fn func()) { queue <- fn }, state: func(value ApplicationState, _ error) { state = value }, replace: func(fyne.CanvasObject) {},
		setup: func(_ context.Context, ready func(context.Context, *Resources[*mx.Client]), failed func(error)) (applicationPanels, func(context.Context) error, error) {
			panels := &applicationSDKPanels{dispatch: func(fn func()) { queue <- fn }, open: func(ctx context.Context) error {
				resources, err := factory.Open(ctx, devices)
				if err != nil {
					return err
				}
				queue <- func() { ready(resources.Context(), resources) }
				return nil
			}, failed: failed}
			return panels, func(context.Context) error {
				if devices.Legacy != nil {
					return errors.Join(devices.Legacy.Close(), devices.Isolated.Close())
				}
				return devices.Isolated.Close()
			}, nil
		},
		build: func(ctx context.Context, resources *Resources[*mx.Client]) (ApplicationPlan[*privateContacts.Repository], error) {
			return NewRecordApplicationPlan(ctx, resources, RecordApplicationConfig[*privateContacts.Repository]{Gateway: gateway, Source: source, Service: routing, New: privateContacts.New, Historical: privateContacts.NewHistoricalSource})
		},
		mount: func(_ context.Context, _ *Resources[*mx.Client], repository *privateContacts.Repository) (ApplicationView, error) {
			var err error
			facade, err = api.NewContacts(server.URL, nil, server.Client()).WithPrivateData(repository)
			if err != nil {
				return ApplicationView{}, err
			}
			screen = contactos.NuevaApp(translator, facade)
			mounts++
			return ApplicationView{Object: screen.Objeto(), Detach: screen.Cerrar}, nil
		},
	}
	pump := func(done func() bool) {
		t.Helper()
		deadline, stop := context.WithTimeout(ctx, 45*time.Second)
		defer stop()
		for !done() {
			select {
			case fn := <-queue:
				fn()
			case <-deadline.Done():
				t.Fatal("native host did not reach its verified lifecycle boundary", state)
			}
		}
	}
	host.adopt()
	pump(func() bool { return state == ApplicationFailed })
	if mounts != 0 || source.opens != 0 || journalOpens.Load() != 0 {
		t.Fatal("unacknowledged recovery opened app/source/journal")
	}
	if err := target.client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	host.retry()
	pump(func() bool { return state == ApplicationReady })
	if mounts != 1 || source.opens != 1 || journalOpens.Load() != 1 {
		t.Fatal("verified app failed complete source import", mounts, source.opens, journalOpens.Load())
	}
	retireConsent := exerciseApplicationStorageConsent(ctx, t, application, target, host.active.resources, translator)
	contact, err := facade.Create(ctx, book, api.ContactInput{FormattedName: "Private app contact", Emails: []api.ContactEmail{{Email: "private@example.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := target.client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	retained := host.active.plan.Repository
	if count, err := retained.Pending(ctx); err != nil || count.Count != 0 {
		t.Fatal("accepted app mutation remained pending", count, err)
	}
	host.close()
	pump(func() bool {
		for _, life := range host.retirements {
			select {
			case <-life.retiredDone:
			default:
				return false
			}
		}
		return true
	})
	if err := retireConsent(ctx); err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	source.retired = true
	opens := source.opens
	source.mu.Unlock()
	fresh := openApplicationSDKDevice(ctx, t, origin.String(), scope, isolatedUser, controller, targetRoom, target.recovery)
	if err := fresh.client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	devices = Devices[*mx.Client]{Isolated: fresh.client}
	next := &privateApplication[*mx.Client, *privateContacts.Repository]{cfg: host.cfg}
	host = next
	host.adopt()
	pump(func() bool { return state == ApplicationReady && mounts == 2 })
	got, err := facade.Contact(ctx, contact.ID)
	if err != nil || got.FormattedName != "Private app contact" {
		t.Fatal("fresh app lost private edit", err)
	}
	source.mu.Lock()
	unchanged := source.opens == opens
	source.mu.Unlock()
	if !unchanged || plaintext.Load() != 0 {
		t.Fatal("fresh app read plaintext legacy content", plaintext.Load())
	}
	exerciseApplicationContactsRefresh(ctx, t, application, origin.String(), scope, isolatedUser, controller, targetRoom, target.recovery, factory, session.WithAuthorizationIntent, routing, source, gateway, host.active.resources, fresh, host.active.plan.Repository, facade, screen, contact, book)
	if plaintext.Load() != 0 {
		t.Fatal("explicit private refresh reached legacy HTTP", plaintext.Load())
	}
	host.close()
	pump(func() bool {
		for _, life := range host.retirements {
			select {
			case <-life.retiredDone:
			default:
				return false
			}
		}
		return true
	})
}

type applicationSDKPanels struct {
	ctx      context.Context
	open     func(context.Context) error
	failed   func(error)
	dispatch func(func())
}

func (p *applicationSDKPanels) Start(ctx context.Context) error { p.ctx = ctx; p.Retry(); return nil }

func (p *applicationSDKPanels) Retry() {
	go func() {
		err := p.open(p.ctx)
		if err != nil {
			p.dispatch(func() { p.failed(err) })
		}
	}()
}
func (*applicationSDKPanels) Show()       {}
func (*applicationSDKPanels) Invalidate() {}

// The metadata service commits before dropping its first reply. The SDK and
// panel remain real; Ensure must reconcile the exact active root without asking
// again or signing/enrolling a second root.
type applicationConsentService struct {
	authority     *matrixproof.Authority
	request       matrixproof.RootEnrollment
	confirmations int
	drop          bool
}

func (s *applicationConsentService) MatrixStorageAuthority(ctx context.Context, _ storage.Scope) (matrixproof.Authority, error) {
	if s.authority == nil {
		return matrixproof.Authority{}, &api.Error{Status: http.StatusConflict, Body: `{"error":{"code":"storage_root_enrollment_required"}}`}
	}
	return *s.authority, ctx.Err()
}

func (s *applicationConsentService) MatrixStorageRootChallenge(_ context.Context, scope storage.Scope, epoch, kind string, request matrixproof.RootEnrollment) (matrixproof.RootChallenge, error) {
	s.request = request
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return matrixproof.RootChallenge{}, err
	}
	now := time.Now()
	return matrixproof.RootChallenge{Request: request, Action: matrixproof.Action{Version: 1, Scope: scope, Epoch: epoch, Kind: kind, Target: request.Target, Generation: request.Generation, RootKey: request.RootKey, RequestSHA256: digest, Nonce: rand.Text(), IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}}, nil
}

func (s *applicationConsentService) ConfirmMatrixStorageRoot(_ context.Context, _ storage.Scope, proof matrixproof.SignedAction) (matrixproof.Authority, error) {
	s.confirmations++
	s.authority = &matrixproof.Authority{Version: 1, Scope: proof.Scope, UserID: s.request.UserID, RootKey: proof.RootKey, Generation: proof.Generation, Status: "active"}
	if s.drop {
		s.drop = false
		return matrixproof.Authority{}, errors.New("synthetic lost accepted root enrollment ACK")
	}
	return *s.authority, nil
}

type applicationConsentDevice struct {
	*mx.Client
	trace func(string)
}

func (d applicationConsentDevice) StorageRootConsentRequest(ctx context.Context, lease offline.WorkLease, scope storage.Scope, target storage.PublicationTarget, kind string, previous *matrixproof.Authority) (matrixproof.RootEnrollment, error) {
	d.trace("request.begin")
	request, err := d.Client.StorageRootConsentRequest(ctx, lease, scope, target, kind, previous)
	d.trace("request.end")
	return request, err
}

func (d applicationConsentDevice) SignIngressAction(ctx context.Context, lease offline.WorkLease, intent matrixproof.Intent, action matrixproof.Action) (matrixproof.SignedAction, error) {
	d.trace("sign.begin")
	proof, err := d.Client.SignIngressAction(ctx, lease, intent, action)
	d.trace("sign.end")
	return proof, err
}

func logApplicationConsentStacks(t *testing.T) {
	t.Helper()
	buffer := make([]byte, 1<<20)
	n := runtime.Stack(buffer, true)
	for _, stack := range strings.Split(string(buffer[:n]), "\n\n") {
		if strings.Contains(stack, "RootConsent") || strings.Contains(stack, "StorageApproval") || strings.Contains(stack, "ConfirmarAlmacen") || strings.Contains(stack, "SignIngressAction") {
			t.Log(stack)
		}
	}
}

func exerciseApplicationStorageConsent(ctx context.Context, t *testing.T, application fyne.App, device *applicationSDKDevice, resources *Resources[*mx.Client], translator *i18n.Translator) func(context.Context) error {
	t.Helper()
	window := application.NewWindow("Native storage consent")
	window.Resize(fyne.NewSize(640, 480))
	window.SetContent(&fyne.Container{})
	config := device.config
	config.StoreDir = t.TempDir()
	config.Secrets = &authtest.Secrets{}
	var logins atomic.Int32
	panel := &Panel{Base: config, Lienzo: window.Canvas, Traducir: func(key string) string { return translator.T(key, nil) }, Bootstrap: func(ctx context.Context) (mx.Bootstrap, error) { return config.Bootstrap, ctx.Err() }, Login: func(ctx context.Context, _ mx.Config) (*mx.Client, error) {
		logins.Add(1)
		return device.client, ctx.Err()
	}}
	ready := make(chan struct{}, 1)
	panel.AlLista = func(context.Context) {
		select {
		case ready <- struct{}{}:
		default:
		}
	}
	fyne.DoAndWait(func() {
		panel.Iniciar(ctx, resources.PrivateStorageScope().Subject, resources.PrivateStorageScope().Tenant)
	})
	deadline, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	for {
		idle := false
		fyne.DoAndWait(func() { idle = !panel.busy })
		if idle {
			break
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-deadline.Done():
			t.Fatal("real panel resume did not settle")
		}
	}
	fyne.DoAndWait(panel.conectar)
	verified := false
	for !verified {
		fyne.DoAndWait(func() {})
		select {
		case <-ready:
			verified = true
		case <-time.After(5 * time.Millisecond):
		case <-deadline.Done():
			var busy, hasClient bool
			fyne.DoAndWait(func() { busy = panel.busy; hasClient = panel.client != nil })
			t.Fatal("real SDK panel did not verify", logins.Load(), busy, hasClient)
		}
	}
	approve := nativeStorageApproval(resources, panel)
	target, err := device.client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	user, _ := device.client.Identity()
	request := matrixproof.RootEnrollment{Scope: resources.PrivateStorageScope(), Target: target, UserID: user}
	wrong := request
	wrong.Scope.Tenant = "other"
	if err := approve(ctx, wrong); err == nil {
		t.Fatal("wrong tenant opened consent")
	}
	wrong = request
	wrong.Target.DeviceID = "OTHER"
	if err := approve(ctx, wrong); err == nil {
		t.Fatal("wrong device opened consent")
	}
	service := &applicationConsentService{drop: true}
	trace := func(phase string) { t.Log("root consent", phase) }
	prompts := 0
	flow := nativeIngress.RootConsent{Scope: resources.PrivateStorageScope(), Target: target, Device: applicationConsentDevice{Client: device.client, trace: trace}, Service: service, Lease: resources.WorkLease(), Authorize: func(ctx context.Context) error {
		trace("authorize.begin")
		err := resources.Revalidate(ctx)
		trace("authorize.end")
		return err
	}, Approve: func(ctx context.Context, request matrixproof.RootEnrollment) error {
		prompts++
		trace("approve.begin")
		err := approve(ctx, request)
		trace("approve.end")
		return err
	}}
	result := make(chan error, 1)
	go func() { _, err := flow.Ensure(ctx); result <- err }()
	accepted := false
	for !accepted {
		fyne.DoAndWait(func() {
			for _, overlay := range window.Canvas().Overlays().List() {
				widget, ok := overlay.(fyne.Widget)
				if !ok {
					continue
				}
				for _, object := range fynetest.WidgetRenderer(widget).Objects() {
					dialog, ok := object.(*ui.Dialogo)
					if !ok || dialog.Titulo != translator.T("matrix.allowStorageTitle", nil) {
						continue
					}
					buttons := dialog.Pie.(*fyne.Container).Objects
					buttons[len(buttons)-1].(*ui.Boton).AlPulsar()
					accepted = true
				}
			}
		})
		if accepted {
			break
		}
		select {
		case err := <-result:
			t.Fatal("real panel rejected its verified device before prompt", err)
		case <-time.After(5 * time.Millisecond):
		case <-deadline.Done():
			t.Fatal("real consent prompt was not shown")
		}
	}
	returned := false
	for !returned {
		fyne.DoAndWait(func() {})
		select {
		case err := <-result:
			if err == nil {
				t.Fatal("lost accepted root ACK was hidden")
			}
			returned = true
		case <-time.After(5 * time.Millisecond):
		case <-deadline.Done():
			logApplicationConsentStacks(t)
			t.Fatal("accepted root consent did not return", service.confirmations)
		}
	}
	authority, err := flow.Ensure(ctx)
	if err != nil || authority.Status != "active" || prompts != 1 || service.confirmations != 1 {
		t.Fatal("lost ACK retry repeated consent or replaced root", err, prompts, service.confirmations)
	}
	return func(ctx context.Context) error {
		fyne.DoAndWait(panel.Cerrar)
		err := panel.Retire(ctx)
		window.Close()
		return err
	}
}

func exerciseApplicationContactsRefresh(ctx context.Context, t *testing.T, _ fyne.App, origin string, scope storage.Scope, user, controller *mautrix.Client, room id.RoomID, recovery string, factory Factory[*mx.Client], capture func(context.Context) context.Context, routing *applicationRouting, source *applicationRetainedSource, gateway *api.Drive, readerResources *Resources[*mx.Client], readerDevice *applicationSDKDevice, reader *privateContacts.Repository, readerFacade *api.Contacts, screen *contactos.App, original api.Contact, book string) {
	t.Helper()
	writerDevice := openApplicationSDKDevice(ctx, t, origin, scope, user, controller, room, recovery)
	if err := writerDevice.client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	writerJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	writerFactory := factory
	writerFactory.OpenJournal = func(ctx context.Context) (*offline.Store, error) { return writerJournal, ctx.Err() }
	resources, err := writerFactory.Open(capture(readerResources.Context()), Devices[*mx.Client]{Isolated: writerDevice.client})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewRecordApplicationPlan(ctx, resources, RecordApplicationConfig[*privateContacts.Repository]{Gateway: gateway, Source: source, Service: routing, New: privateContacts.New, Historical: privateContacts.NewHistoricalSource})
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := NewRepositoryCycle(resources, RepositoryConfig[*privateContacts.Repository]{Create: func(context.Context) (*privateContacts.Repository, error) { return plan.Repository, nil }, Prepare: plan.Prepare})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = cycle.RetirePreservingWork(ctx); _ = plan.Retire(ctx) }()
	if err := cycle.Open(ctx); err != nil {
		t.Fatal("second SDK application could not open", err)
	}
	writer, err := api.NewContacts("https://private.invalid", nil, nil).WithPrivateData(plan.Repository)
	if err != nil {
		t.Fatal(err)
	}
	refresh := func() {
		t.Helper()
		done := make(chan struct{})
		go func() { screen.Cargar(); close(done) }()
		deadline, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		for {
			fyne.DoAndWait(func() {})
			select {
			case <-done:
				fyne.DoAndWait(func() {})
				return
			case <-time.After(5 * time.Millisecond):
			case <-deadline.Done():
				t.Fatal("actual Contacts refresh did not settle")
			}
		}
	}
	visible := func() string {
		var name string
		fyne.DoAndWait(func() {
			for _, person := range screen.Pantalla.Contactos {
				if person.ID == original.ID {
					name = person.FormattedName
				}
			}
		})
		return name
	}
	refresh()
	source.mu.Lock()
	initialRefreshes := source.refreshes
	source.mu.Unlock()
	if initialRefreshes != 0 {
		t.Fatal("initial mount reloaded the already authenticated private catalog", initialRefreshes)
	}
	if name := visible(); name != original.FormattedName {
		t.Fatal("private screen did not show original projection", name)
	}
	lostCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	readerDevice.lostAccepted.Store(&stop)
	readerDevice.dropWrite.Store(true)
	_, lostErr := readerFacade.Create(lostCtx, book, api.ContactInput{FormattedName: "Retained pending contact"})
	stop()
	readerDevice.lostAccepted.Store(nil)
	readerDevice.dropWrite.Store(false)
	pending, err := reader.Pending(ctx)
	if lostErr == nil || readerDevice.lostWrites.Load() == 0 || err != nil || pending.Count == 0 {
		t.Fatal("actual accepted SDK mutation did not retain its lost ACK", lostErr, err, pending.Count, readerDevice.lostWrites.Load())
	}
	current, err := writer.Contact(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := writer.Update(ctx, original.ID, api.ContactInput{FormattedName: "Changed on another SDK device"}, current.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if stale, err := readerFacade.Contact(ctx, original.ID); err != nil || stale.FormattedName != original.FormattedName {
		t.Fatal("ordinary read implicitly refreshed the library", err)
	}
	source.mu.Lock()
	epoch := source.state.Epoch
	source.state.Epoch = uuid.NewString()
	source.mu.Unlock()
	refresh()
	if name := visible(); name != original.FormattedName {
		t.Fatal("invalid receipt replaced the old visible projection", name)
	}
	if old, err := readerFacade.Contact(ctx, original.ID); err != nil || old.FormattedName != original.FormattedName {
		t.Fatal("failed candidate replaced private heads", err)
	}
	after, err := reader.Pending(ctx)
	if err != nil || after != pending {
		t.Fatal("failed refresh changed pending WAL", after, pending, err)
	}
	source.mu.Lock()
	source.state.Epoch = epoch
	source.mu.Unlock()
	refresh()
	if name := visible(); name != changed.FormattedName {
		t.Fatal("explicit UI refresh did not publish other SDK edit", name)
	}
	after, err = reader.Pending(ctx)
	if err != nil || after != pending {
		t.Fatal("successful refresh reconciled or deleted pending WAL", after, pending, err)
	}
	source.mu.Lock()
	refreshes := source.refreshes
	source.mu.Unlock()
	if refreshes != 2 {
		t.Fatal("refresh did not use read-only source metadata", refreshes)
	}
}
