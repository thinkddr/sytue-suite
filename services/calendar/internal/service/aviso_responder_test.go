// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// apuntaAtributos guarda el valor en texto de cada atributo registrado, tal cual
// llega al manejador (antes de que este lo entrecomille o lo escape).
type apuntaAtributos struct {
	mu      sync.Mutex
	valores map[string]string
}

func (a *apuntaAtributos) Enabled(context.Context, slog.Level) bool { return true }
func (a *apuntaAtributos) WithAttrs([]slog.Attr) slog.Handler       { return a }
func (a *apuntaAtributos) WithGroup(string) slog.Handler            { return a }
func (a *apuntaAtributos) Handle(_ context.Context, r slog.Record) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	r.Attrs(func(at slog.Attr) bool {
		a.valores[at.Key] = at.Value.String()
		return true
	})
	return nil
}

// Un UID con CR/LF (lo escribe el cliente en el .ics) no puede llegar crudo al log:
// en un sumidero de texto fabricaría una línea falsa.
func TestAvisoAlResponderEscapaSaltosDeLinea(t *testing.T) {
	h := &apuntaAtributos{valores: map[string]string{}}
	ctx := log.Into(context.Background(), slog.New(h))
	uid := "evento-1\r\nlevel=ERROR msg=\"falso\" uid=otro"
	avisoAlResponder(ctx, "calendar: prueba", uid, errors.New("no existe "+uid))

	for _, clave := range []string{"uid", "error"} {
		v, ok := h.valores[clave]
		if !ok {
			t.Fatalf("falta el atributo %q", clave)
		}
		if strings.ContainsAny(v, "\r\n") {
			t.Errorf("%s llega con saltos de línea crudos: %q", clave, v)
		}
		if !strings.Contains(v, `evento-1\r\nlevel=ERROR`) {
			t.Errorf("%s debe conservar el valor escapado y legible: %q", clave, v)
		}
	}
}
