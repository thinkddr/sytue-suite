//go:build integration

// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Disposable loopback-only fixture secret; never a product credential.
const mailBrowserPublicationSecret = "1111111111111111111111111111111111111111111111111111111111111111"

// The real module is active in both phases. The dedicated strict run requires
// signed admission for SDK2 from the start; historical SDK1 retains its own
// source policy. Browser writes always use the strict origin path.
type mailBrowserOrigin struct {
	native, strict *service.Service
	strictPhase    atomic.Bool
	source         *nativeMailSQLFixture
	store          *store.Store
	mu             sync.Mutex
	routes         map[string]mailPublicationRoute
	auditError     error
}

func newMailBrowserOrigin(source *nativeMailSQLFixture, st *store.Store) *mailBrowserOrigin {
	cfg := source.serviceConfig
	cfg.MatrixPublicationRequireMarker = true
	return &mailBrowserOrigin{native: source.nativeService, strict: service.New(cfg, st, nil, nil), source: source, store: st}
}

func (m *mailBrowserOrigin) current() *service.Service {
	if m.strictPhase.Load() {
		return m.strict
	}
	return m.native
}

func (m *mailBrowserOrigin) BeforeMatrixPublicationOrigin(ctx context.Context, event publication.OriginEvent) (publication.OriginDecision, error) {
	return m.current().BeforeMatrixPublicationOrigin(ctx, event)
}

func (m *mailBrowserOrigin) AcceptedMatrixPublicationOrigin(ctx context.Context, event publication.OriginEvent) (publication.OriginDecision, error) {
	decision, err := m.current().AcceptedMatrixPublicationOrigin(ctx, event)
	if err == nil && decision.Managed && decision.Allow {
		m.observeAccepted(event)
	}
	return decision, err
}

func (m *mailBrowserOrigin) mount(root chi.Router) {
	mountPublicationOrigin(root, mailBrowserPublicationSecret, m)
	root.Post("/fixture/browser-phase", func(w http.ResponseWriter, r *http.Request) {
		state, err := m.store.MatrixStorageCutover(r.Context(), m.source.state.Scope.Subject, "mail")
		if err != nil || state.Phase != "isolated" {
			httpx.Fail(w, r, httpx.ErrConflict)
			return
		}
		m.strictPhase.Store(true)
		httpx.NoContent(w)
	})
}

func (m *mailBrowserOrigin) mountPublications(api chi.Router) {
	bootstrap := &Handlers{svc: m.strict}
	api.Get("/matrix/storage/bootstrap", bootstrap.MatrixStorageBootstrap)
	handlers := storagePublicationHandlers{svc: mailObservedPublicationService{Service: m.strict, origin: m}}
	api.Post("/matrix/storage/{product}/publications/reserve", handlers.reserve)
	api.Post("/matrix/storage/{product}/publications/cancel", handlers.cancel)
	api.Get("/matrix/storage/{product}/publications", handlers.get)
}

// This observer retains only public routing and commitments after the original
// service accepted a reservation. It never creates authority or stores content.
type mailPublicationRoute struct {
	Room, TransactionID, RequestSHA256, CipherSHA256 string
	Reference                                        *storage.Reference
}
type mailObservedPublicationService struct {
	*service.Service
	origin *mailBrowserOrigin
}

func (s mailObservedPublicationService) ReserveMatrixStoragePublication(ctx context.Context, actor, product string, signed publication.SignedRequest) (publication.Receipt, error) {
	receipt, err := s.Service.ReserveMatrixStoragePublication(ctx, actor, product, signed)
	if err == nil && receipt.Valid() {
		s.origin.observeReserved(signed.Request, receipt)
	}
	return receipt, err
}

func (m *mailBrowserOrigin) observeReserved(request publication.Request, receipt publication.Receipt) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.routes == nil {
		m.routes = map[string]mailPublicationRoute{}
	}
	route := mailPublicationRoute{Room: request.Target.RoomID, TransactionID: request.TransactionID, RequestSHA256: receipt.RequestSHA256, CipherSHA256: request.CipherSHA256}
	if previous, ok := m.routes[receipt.Marker.ID]; ok {
		route.Reference = previous.Reference
		if route.Room != previous.Room || route.TransactionID != previous.TransactionID || route.RequestSHA256 != previous.RequestSHA256 || route.CipherSHA256 != previous.CipherSHA256 {
			m.auditError = errors.New("public reservation routing changed")
		}
	}
	m.routes[receipt.Marker.ID] = route
}

func (m *mailBrowserOrigin) observeAccepted(event publication.OriginEvent) {
	hash, marker, err := publication.WireDigest(event.Content, true)
	if err != nil {
		if m.strictPhase.Load() {
			m.mu.Lock()
			m.auditError = errors.New("strict origin accepted an event without its reservation marker")
			m.mu.Unlock()
		}
		return
	} // Historical SDK1 has no SDK2 reservation marker.
	m.mu.Lock()
	defer m.mu.Unlock()
	route, ok := m.routes[marker.ID]
	if !ok || route.Room != event.Reference.RoomID || route.CipherSHA256 != hash {
		m.auditError = errors.New("accepted origin did not match its public reservation")
		return
	}
	if route.Reference != nil && *route.Reference != event.Reference {
		m.auditError = errors.New("accepted public reservation changed reference")
		return
	}
	ref := event.Reference
	route.Reference = &ref
	m.routes[marker.ID] = route
}

func (m *mailBrowserOrigin) auditBrowserPublications(ctx context.Context, scope storage.Scope) error {
	if m == nil || m.store == nil || m.source == nil || !m.strictPhase.Load() {
		return errors.New("strict browser origin was not exercised")
	}
	state, err := m.store.MatrixStorageCutover(ctx, scope.Subject, scope.Product)
	if err != nil {
		return err
	}
	if state.Validate(scope) != nil || state.Phase != "isolated" || len(state.SourceRooms) == 0 {
		return errors.New("browser source cutover was not retained")
	}
	m.source.mu.Lock()
	retired := m.source.retired
	snapshot := m.source.snapshot
	m.source.mu.Unlock()
	if !retired || snapshot.State.Scope != scope || len(snapshot.Rows) != 18 || snapshot.State.Blobs != 3 {
		return errors.New("browser source changed its original frozen SQL18/3 capture")
	}
	m.mu.Lock()
	auditErr := m.auditError
	routes := make([]mailPublicationRoute, 0, len(m.routes))
	for _, route := range m.routes {
		if route.Reference != nil {
			ref := *route.Reference
			route.Reference = &ref
			routes = append(routes, route)
		}
	}
	m.mu.Unlock()
	if auditErr != nil {
		return auditErr
	}
	if len(routes) == 0 {
		return errors.New("no accepted browser publication reached the original store")
	}
	receiptSeen := false
	for _, route := range routes {
		receipt, err := m.store.ReadMatrixStoragePublication(ctx, scope.Subject, scope.Product, route.Room, route.TransactionID)
		if err != nil {
			return err
		}
		if !receipt.Valid() || receipt.Status != "accepted" || receipt.RequestSHA256 != route.RequestSHA256 || receipt.Reference == nil || *receipt.Reference != *route.Reference {
			return errors.New("public PG admission lost its accepted ciphertext reference")
		}
		if state.Receipt != nil && *state.Receipt == *route.Reference {
			receiptSeen = true
		}
	}
	if !receiptSeen {
		return errors.New("the retained cutover receipt was not admitted by the original PG ledger")
	}
	return nil
}

// Only the explicitly named BrowserFull proof omits the separate worker proof.
// Unknown names keep the complete original worker postconditions.
func runLiveMailPostAudit(caseName string, browser, worker func() error) error {
	if caseName == "TestPrivateIngressLiveNativeMailBrowserFullSource" {
		return browser()
	}
	return worker()
}
