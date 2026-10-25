// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

func personalCapacityFixture(id, template string) domain.AppInstance {
	return domain.AppInstance{ID: id, TemplateKey: template, Status: domain.StatusActive, HasSecrets: true}
}

func knownPersonalCapacity() objectstore.Capacity {
	total, used, free := int64(1000), int64(300), int64(700)
	return objectstore.Capacity{TotalBytes: &total, UsedBytes: &used, AvailableBytes: &free}
}

func TestPersonalCapacityFiltersConnectionsAndDoesNotCountDuplicates(t *testing.T) {
	active := personalCapacityFixture("active", tmplGoogleDrive)
	pending := personalCapacityFixture("pending", tmplOneDrive)
	pending.Status = domain.StatusDraft
	noSecret := personalCapacityFixture("no-secret", tmplOneDrive)
	noSecret.HasSecrets = false
	instances := []domain.AppInstance{
		active, active, pending, noSecret,
		personalCapacityFixture("disabled", tmplDropbox), personalCapacityFixture("other", "slack-chat"),
	}
	var queried []string
	got := personalStorageCapacity(context.Background(), instances, map[string]bool{"dropbox": true},
		func(_ context.Context, inst domain.AppInstance) (objectstore.Capacity, error) {
			queried = append(queried, inst.ID)
			return knownPersonalCapacity(), nil
		})
	if !got.Connected || got.Partial || got.TotalBytes == nil || *got.TotalBytes != 1000 ||
		len(queried) != 1 || queried[0] != "active" {
		t.Fatalf("sólo cuenta una vez la conexión utilizable: %+v, consultas=%v", got, queried)
	}
}

func TestPersonalCapacityKeepsKnownCloudWhenAnotherCannotReport(t *testing.T) {
	got := personalStorageCapacity(context.Background(), []domain.AppInstance{
		personalCapacityFixture("google", tmplGoogleDrive), personalCapacityFixture("old-consent", tmplDropbox),
	}, nil, func(_ context.Context, inst domain.AppInstance) (objectstore.Capacity, error) {
		if inst.ID == "old-consent" {
			return objectstore.Capacity{}, errors.New("missing account_info.read")
		}
		return knownPersonalCapacity(), nil
	})
	if !got.Connected || !got.Partial || got.TotalBytes == nil || *got.TotalBytes != 1000 || *got.AvailableBytes != 700 {
		t.Fatalf("el consentimiento antiguo no borra la ampliación conocida: %+v", got)
	}
}

func TestPersonalCapacityUsesThreeWorkersAndOneCancellation(t *testing.T) {
	instances := make([]domain.AppInstance, 0, 8)
	for i := range 8 {
		instances = append(instances, personalCapacityFixture(fmt.Sprint(i), tmplGoogleDrive))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 8)
	done := make(chan personalCapacity, 1)
	var mu sync.Mutex
	active, maxActive, calls := 0, 0, 0
	go func() {
		done <- personalStorageCapacity(ctx, instances, nil, func(ctx context.Context, _ domain.AppInstance) (objectstore.Capacity, error) {
			mu.Lock()
			active++
			calls++
			maxActive = max(maxActive, active)
			mu.Unlock()
			started <- struct{}{}
			<-ctx.Done()
			mu.Lock()
			active--
			mu.Unlock()
			return objectstore.Capacity{}, ctx.Err()
		})
	}()
	for range 3 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("las tres consultas no arrancaron en paralelo")
		}
	}
	cancel()
	select {
	case got := <-done:
		if !got.Connected || !got.Partial || got.TotalBytes != nil || calls != 3 || maxActive != 3 {
			t.Fatalf("la cancelación global no inventa ceros ni arranca las ocho consultas: %+v calls=%d max=%d", got, calls, maxActive)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el sondeo no terminó al cancelar su presupuesto global")
	}
}
