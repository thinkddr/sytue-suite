// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

func TestPersonalQuotaIsAuthorizedAgainForEachActorOnTheSameClient(t *testing.T) {
	var mu sync.Mutex
	var actors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := r.URL.Query().Get("actor")
		mu.Lock()
		actors = append(actors, actor)
		mu.Unlock()
		if r.URL.Path != "/internal/v1/storage/personal-capacity" || r.URL.Query().Get("tenant") != "acme" ||
			r.Header.Get("X-Sytue-Internal") != "internal-personal-fixture" {
			t.Errorf("la consulta perdió el tenant, actor o autenticación: %s %v", r.URL, r.Header)
		}
		var result personalStorageCapacity
		if actor == "ana" {
			total, used, free := int64(1000), int64(300), int64(700)
			result.Connected = true
			result.Capacity = objectstore.Capacity{TotalBytes: &total, UsedBytes: &used, AvailableBytes: &free}
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	st := &fakeStore{quota: domain.Quota{LimitBytes: 500, UsedBytes: 200}}
	s := newSvc(st, &fakeBlobs{}, nil)
	s.router = newBlobRouter(&fakeBlobs{}, &fakeExternal{}, "", newIntegrationsClient(server.URL, "internal-personal-fixture"))
	s.router.cache["acme"] = entradaExterna{expires: time.Now().Add(time.Hour)}
	for _, actor := range []string{"ana", "bea", "ana"} {
		ctx := authn.Into(tenantCtx(t), authn.Claims{Subject: actor})
		q, err := s.Quota(ctx)
		want := int64(500)
		if actor == "ana" {
			want = 1500
		}
		if err != nil || q.LimitBytes != want || q.BaseLimitBytes != 500 || q.DriveBytes() != 200 {
			t.Fatalf("la cuota personal se compartió con %q: %+v %v", actor, q, err)
		}
	}
	// Sin usuario o con token delegado no se reutiliza la cuota que Ana leyó.
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), actors...)
	}
	before := len(snapshot())
	_, _ = s.Quota(tenantCtx(t))
	_, _ = s.Quota(authn.Into(tenantCtx(t), authn.Claims{Subject: "ana", Delegado: true}))
	after := snapshot()
	if len(after) != before || len(after) != 3 || after[0] != "ana" || after[1] != "bea" || after[2] != "ana" {
		t.Fatalf("se cacheó por tenant o se abrió la vista personal a un delegado: %v", after)
	}
	if st.quota.LimitBytes != 500 || st.quota.UsedBytes != 200 {
		t.Fatal("la cuota de la cuenta no puede persistir las capacidades personales")
	}
}

func TestUnknownOrganizationStorageDoesNotHideKnownPersonalCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"connected":true,"total_bytes":1000,"used_bytes":300,"available_bytes":700}`))
	}))
	defer server.Close()
	st := &fakeStore{quota: domain.Quota{LimitBytes: 500, UsedBytes: 700}}
	s := newSvc(st, &fakeBlobs{}, nil)
	s.router = newBlobRouter(&fakeBlobs{}, &fakeExternal{}, "", newIntegrationsClient(server.URL, "fixture"))
	// S3 está conectado pero no ofrece una cuota finita estándar.
	s.router.cache["acme"] = entradaExterna{blobs: &fakeBlobs{}, expires: time.Now().Add(time.Hour)}
	q, err := s.Quota(authn.Into(tenantCtx(t), authn.Claims{Subject: "ana"}))
	if err != nil || !q.ConnectedStorage || !q.StorageCapacityPartial || q.LimitBytes != 1500 ||
		q.UsedBytes != 1000 || q.Available() != 700 || q.BaseLimitBytes != 500 || q.BaseUsedBytes != 700 {
		t.Fatalf("S3 desconocido no debe borrar Google conocido ni el exceso local comerse su libre: %+v %v", q, err)
	}
}

func TestPersonalQuotaPassesTheActiveTenantWithoutCaching(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("actor") != "ana" {
			t.Error("falta la identidad de la persona")
		}
		if r.URL.Query().Get("tenant") == "acme" {
			_, _ = w.Write([]byte(`{"connected":true,"total_bytes":1000,"used_bytes":300,"available_bytes":700}`))
		} else {
			_, _ = w.Write([]byte(`{"connected":false,"total_bytes":null,"used_bytes":null,"available_bytes":null}`))
		}
	}))
	defer server.Close()
	s := newSvc(&fakeStore{quota: domain.Quota{LimitBytes: 500, UsedBytes: 200}}, &fakeBlobs{}, nil)
	s.router = newBlobRouter(&fakeBlobs{}, &fakeExternal{}, "", newIntegrationsClient(server.URL, "fixture"))
	for _, tenant := range []string{"acme", "beta"} {
		s.router.cache[tenant] = entradaExterna{expires: time.Now().Add(time.Hour)}
		ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: tenant}), authn.Claims{Subject: "ana"})
		q, err := s.Quota(ctx)
		if err != nil || q.ConnectedStorage != (tenant == "acme") {
			t.Fatalf("el tenant %s heredó otra capacidad: %+v %v", tenant, q, err)
		}
	}
}

type cancelledCapacityBlobs struct {
	*fakeBlobs
	started chan struct{}
}

func (b cancelledCapacityBlobs) Capacity(ctx context.Context) (objectstore.Capacity, error) {
	b.started <- struct{}{}
	<-ctx.Done()
	return objectstore.Capacity{}, ctx.Err()
}

func TestOrganizationAndPersonalCapacityReadInParallelAndShareCancellation(t *testing.T) {
	personalStarted, orgStarted := make(chan struct{}, 1), make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		personalStarted <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	s := newSvc(&fakeStore{quota: domain.Quota{LimitBytes: 500, UsedBytes: 200}}, &fakeBlobs{}, nil)
	s.router = newBlobRouter(&fakeBlobs{}, &fakeExternal{}, "", newIntegrationsClient(server.URL, "fixture"))
	s.router.cache["acme"] = entradaExterna{blobs: cancelledCapacityBlobs{fakeBlobs: &fakeBlobs{}, started: orgStarted}, expires: time.Now().Add(time.Hour)}
	ctx, cancel := context.WithCancel(authn.Into(tenantCtx(t), authn.Claims{Subject: "ana"}))
	defer cancel()
	done := make(chan domain.Quota, 1)
	go func() {
		q, _ := s.Quota(ctx)
		done <- q
	}()
	for _, started := range []chan struct{}{orgStarted, personalStarted} {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("un proveedor bloqueó el comienzo del otro")
		}
	}
	cancel()
	select {
	case q := <-done:
		if q.LimitBytes != 500 || q.Available() != 300 || !q.StorageCapacityPartial {
			t.Fatalf("cancelar no acredita capacidad remota: %+v", q)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la cancelación de la petición no cerró ambas consultas")
	}
}
