// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type foreignFileScope struct{ storage.BoundFiles }

func (f foreignFileScope) FileScope() storage.Scope {
	scope := f.BoundFiles.FileScope()
	scope.Subject = "other"
	return scope
}

type stoppedFileScope struct {
	storage.BoundFiles
	err error
}

func (f stoppedFileScope) Prepare(context.Context) error { return f.err }
func TestPrivateMailUsesExplicitBoundFileCapabilityWithoutManufacturingHTTPGateway(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	cfg := f.repo.cfg
	cfg.Gateway = nil
	cloned, err := New(cfg)
	if err != nil {
		t.Fatal("real SDK file capability incorrectly required a dummy HTTP client", err)
	}
	defer cloned.Close()
	if err := cloned.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	cfg.Files = foreignFileScope{cfg.Files}
	if _, err := New(cfg); err == nil {
		t.Fatal("foreign scoped SDK files were accepted")
	}
}

func TestPrivateMailFileCapabilityCannotClaimReadyAfterSDKAccessStopped(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	cfg := f.repo.cfg
	unavailable := errors.New("synthetic stopped file SDK")
	cfg.Files = stoppedFileScope{BoundFiles: cfg.Files, err: unavailable}
	cfg.Gateway = nil
	cloned, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cloned.Close()
	if err := cloned.Initialize(t.Context()); !errors.Is(err, unavailable) {
		t.Fatal("unavailable file capability was treated as ready", err)
	}
}
