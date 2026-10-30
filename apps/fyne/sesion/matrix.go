// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/rooms/src/pages/matrix/callback.astro
// paridad-web: apps/frontend/apps/rooms/src/pages/matrix/native/start.astro
// paridad-web: apps/frontend/apps/rooms/src/pages/matrix/native/callback.astro

package sesion

import (
	"context"
	"net/url"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type matrixAuthorization struct {
	start   api.MatrixNativeAuthorization
	arrived chan api.MatrixNativeCallback
}

// MatrixBrowserReturn is the mobile Matrix PKCE broker return. It reuses the
// system browser and Suite's registered incoming-URL owner. Call off the UI thread.
// No Matrix login token or E2EE secret appears in the incoming URL.
func (s *Suite) MatrixBrowserReturn(ctx context.Context, start api.MatrixNativeAuthorization) (api.MatrixNativeCallback, error) {
	if s.app == nil || s.app.Metadata().Custom["esquema"] != start.ReturnScheme() || !start.ForProduct(s.apex, start.Product()) {
		return api.MatrixNativeCallback{}, api.ErrMatrixSSO
	}
	flow := &matrixAuthorization{start: start, arrived: make(chan api.MatrixNativeCallback, 1)}
	s.mu.Lock()
	if s.matrixConsent != nil {
		s.mu.Unlock()
		return api.MatrixNativeCallback{}, api.ErrMatrixSSO
	}
	s.matrixConsent = flow
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.matrixConsent == flow {
			s.matrixConsent = nil
		}
		s.mu.Unlock()
	}()
	opened := make(chan error, 1)
	fyne.Do(func() { opened <- api.AbrirFuera(s.app, start.AuthorizeURL) })
	select {
	case err := <-opened:
		if err != nil {
			return api.MatrixNativeCallback{}, err
		}
	case <-ctx.Done():
		return api.MatrixNativeCallback{}, ctx.Err()
	}
	select {
	case callback := <-flow.arrived:
		return callback, nil
	case <-ctx.Done():
		return api.MatrixNativeCallback{}, ctx.Err()
	}
}

func (s *Suite) recibirMatrix(u *url.URL) bool {
	if u == nil || s.app == nil || u.Scheme != s.app.Metadata().Custom["esquema"] || u.Host != "matrix" || u.Path != "/callback" {
		return false
	}
	// Consume even invalid/unsolicited callback routes. They must never be queued
	// as screen navigation or retained in Suite's deferred URL inventory.
	s.mu.Lock()
	defer s.mu.Unlock()
	flow := s.matrixConsent
	if flow == nil {
		return true
	}
	callback, ok := api.ParseMatrixNativeCallbackFor(u, flow.start.Product())
	if !ok {
		return true
	}

	if flow.start.Accepts(callback) {
		select {
		case flow.arrived <- callback:
		default:
		}
	}
	return true
}
