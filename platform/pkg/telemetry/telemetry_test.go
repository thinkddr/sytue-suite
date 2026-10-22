// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package telemetry

import (
	"context"
	"errors"
	"testing"
)

func TestProviderStub(t *testing.T) {
	p, err := NewProvider(context.Background(), Config{Service: "sytue-id", Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Shutdown(context.Background())

	tr := p.Tracer("test")
	ctx, span := tr.Start(context.Background(), "op", String("k", "v"))
	if ctx == nil || span == nil {
		t.Fatal("Start devolvió nil")
	}
	span.SetAttributes(Int("n", 1))
	span.RecordError(errors.New("x"))
	span.End()
	if !span.ended {
		t.Fatal("span no marcado como finalizado")
	}

	m := p.Meter("test")
	m.Counter("reqs").Add(context.Background(), 1, String("route", "/x"))
	m.Histogram("latency").Record(context.Background(), 12.5)
}

func TestNilSpanSafe(t *testing.T) {
	var s *Span
	s.SetAttributes(Int("n", 1))
	s.RecordError(errors.New("x"))
	s.End() // no debe entrar en pánico
}
