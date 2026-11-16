// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package miniatura

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"
)

func TestRenderContextoCancelaAntesDeCargarYAlEsperarTurno(t *testing.T) {
	release := Turno()
	defer release()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	called := false
	_, err := RenderConContexto(ctx, func(context.Context) ([]byte, error) { called = true; return nil, nil }, 512, "image/png")
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal("cancelled source loaded", err, called)
	}
	// Waiting for the shared memory turn must also release on cancellation.
	ctx, cancel = context.WithCancel(t.Context())
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, err := RenderConContexto(ctx, func(context.Context) ([]byte, error) { return nil, errors.New("unexpected loader") }, 512, "image/png")
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled renderer remains blocked on the shared turn")
	}
}

func TestRenderContextoBorraOrigenInclusoAlFallarOCancelar(t *testing.T) {
	for _, mode := range []string{"success", "error", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 8, 4))); err != nil {
				t.Fatal(err)
			}
			raw := bytes.Clone(encoded.Bytes())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("source failed")
			out, err := RenderConContexto(ctx, func(context.Context) ([]byte, error) {
				if mode == "error" {
					return raw, failure
				}
				if mode == "cancel" {
					cancel()
				}
				return raw, nil
			}, 4, "image/png")
			if !bytes.Equal(raw, make([]byte, len(raw))) {
				t.Fatal("source retained in plaintext")
			}
			switch mode {
			case "success":
				cfg, _, readErr := image.DecodeConfig(bytes.NewReader(out))
				if err != nil || readErr != nil || cfg.Width != 4 || cfg.Height != 2 {
					t.Fatal(cfg, err, readErr)
				}
			case "error":
				if !errors.Is(err, failure) || out != nil {
					t.Fatal(err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) || out != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
