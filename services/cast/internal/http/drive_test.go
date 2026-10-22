// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// Lo que contesta `/videos:from-drive` es contrato con la web y con la app nativa:
// cada error tiene su código HTTP y su `code`, y el de «Drive no contesta» NO se
// confunde con el de la cuota aunque envuelva los mismos errores del cliente.
func TestTraducirLosErroresDeSubirDesdeDrive(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		status int
		code   string
	}{
		{"no existe", fmt.Errorf("%w: el fichero de Drive", domain.ErrNoExiste), http.StatusNotFound, "not_found"},
		{"prohibido", fmt.Errorf("%w: el fichero de Drive", domain.ErrProhibido), http.StatusForbidden, "forbidden"},
		{"no es vídeo", fmt.Errorf("%w (application/pdf)", service.ErrNoEsVideo), http.StatusUnsupportedMediaType, "not_video"},
		{"demasiado grande", domain.ErrDemasiadoGrande, http.StatusRequestEntityTooLarge, "too_large"},
		{"sin cuota", &domain.SinCuota{Disponibles: 7}, http.StatusInsufficientStorage, "quota_exceeded"},
		{"Drive no contesta", fmt.Errorf("%w: %w", service.ErrDriveNoContesta, cuota.ErrNoDisponible), http.StatusServiceUnavailable, "unavailable"},
		{"sin Drive", service.ErrSinDrive, http.StatusServiceUnavailable, "unavailable"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var e *httpx.Error
			if !errors.As(traducir(c.err), &e) {
				t.Fatalf("traducir(%v) no es un httpx.Error", c.err)
			}
			if e.Status != c.status || e.Code != c.code {
				t.Fatalf("traducir(%v) = %d %q; quiero %d %q", c.err, e.Status, e.Code, c.status, c.code)
			}
		})
	}
	var e *httpx.Error
	_ = errors.As(traducir(fmt.Errorf("%w: %w", service.ErrDriveNoContesta, cuota.ErrNoDisponible)), &e)
	if e.Message != "Sytue Drive did not answer. Try again in a moment." {
		t.Fatalf("mensaje = %q: se confunde con el de la cuota", e.Message)
	}
}
