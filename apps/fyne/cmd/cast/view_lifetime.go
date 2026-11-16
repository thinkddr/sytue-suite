// SPDX-License-Identifier: Apache-2.0
package main

import (
	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
)

// These methods run on the Fyne UI thread. A distinct mount is used for every
// adoption, even when the same account is opened again.
type castPresenter interface {
	AlFondo()
	AlVolver()
	Repintar()
	SucesoPiP(bool)
}

type castViews struct{ current *castView }

type castView struct {
	owner     *castViews
	presenter castPresenter
}

// registerLifecycle is called once for the Suite, not once per private mount.
func (v *castViews) registerLifecycle(s *sesion.Suite) {
	s.AlFondo(v.background)
	s.AlVolver(v.foreground)
}

func (v *castViews) mount(p castPresenter) *castView {
	view := &castView{owner: v, presenter: p}
	v.current = view
	return view
}

func (v *castViews) background() {
	if v.current != nil && v.current.presenter != nil {
		v.current.presenter.AlFondo()
	}
}

func (v *castViews) foreground() {
	if v.current != nil && v.current.presenter != nil {
		v.current.presenter.AlVolver()
	}
}

// poster returns a worker callback. The check is deliberately inside the
// dispatched closure: a callback queued by an old mount must not repaint it or
// be redirected to a newer mount.
func (v *castView) poster(dispatch func(func())) func(string) {
	return func(string) {
		dispatch(func() {
			if v.owner.current == v && v.presenter != nil {
				v.presenter.Repintar()
			}
		})
	}
}

func (v *castView) pip(closed bool) {
	if v.owner.current == v && v.presenter != nil {
		v.presenter.SucesoPiP(closed)
	}
}

// detach invalidates queued callbacks before cancelling the mount's cache.
// It returns whether this view was current, so a stale detach cannot clear a
// newer presenter.
func (v *castView) detach(cache *photos.Fototeca) bool {
	current := v.owner.current == v
	if current {
		v.owner.current = nil
	}
	v.presenter = nil
	cache.Vaciar()
	return current
}
