// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

type repositorioPrivado interface {
	api.DriveData
	Initialize(context.Context) error
	Refresh(context.Context) error
	ResumeUploads(context.Context) ([]api.File, error)
	Close()
}

// One immutable presenter and repository per identity. UI callbacks always retain
// their original screen; they never acquire the next account's data service.
type drivePrivado struct {
	panel         *cifrado.Panel
	identidad     func() (auth.IdentitySnapshot, bool)
	iniciar       func(context.Context, func(context.Context, repositorioPrivado, func(context.Context) error)) error
	cerrarPaneles func(func())
	mostrar       func()
	reintentar    func()
	montar        func(context.Context, repositorioPrivado) (*drive.Pantalla, error)
	bloqueado     func(bool)
	despachar     func(func())
	avisar        func(error)
	cuenta        auth.IdentitySnapshot
	dentro        bool
	ciclo         *cicloDrive
	pantalla      *drive.Pantalla
}

type cicloDrive struct {
	ctx              context.Context
	cancel           context.CancelFunc
	validar          func(context.Context) error
	mu               sync.Mutex
	repo             repositorioPrivado
	jobs             sync.WaitGroup
	cerrado, ocupado bool // UI thread only.
	pendientes       []func(*drive.Pantalla)
	refresco         func()
	repetir          bool
	timer            *time.Timer
}

func (c *cicloDrive) repositorio() repositorioPrivado {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.repo
}

func (d *drivePrivado) instalar() {
	if d.despachar == nil {
		d.despachar = fyne.Do
	}
	if d.panel == nil {
		return
	}
	c := d.ciclo
	d.panel.AlEvento = func(_ mx.DecryptedEvent, err error) {
		if err != nil || !d.vigente(c) || d.pantalla == nil || c.timer != nil {
			return
		}
		c.timer = time.AfterFunc(150*time.Millisecond, func() {
			d.despachar(func() {
				c.timer = nil
				if d.vigente(c) && d.pantalla != nil {
					d.pantalla.Recargar()
				}
			})
		})
	}
}

func (d *drivePrivado) vigente(c *cicloDrive) bool {
	owner, dentro := d.identidad()
	return c != nil && d.ciclo == c && !c.cerrado && c.ctx.Err() == nil && dentro && owner == d.cuenta && (c.validar == nil || c.validar(c.ctx) == nil)
}

func (d *drivePrivado) cambiar() {
	owner, dentro := d.identidad()
	if d.cuenta == owner && d.dentro == dentro {
		return
	}
	d.cerrar()
	d.cuenta, d.dentro = owner, dentro
	d.bloqueado(dentro)
	if dentro {
		if d.iniciar == nil {
			d.avisar(api.ErrPrivateDriveOperation)
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		c := &cicloDrive{ctx: ctx, cancel: cancel}
		d.ciclo = c
		err := d.iniciar(ctx, func(bound context.Context, repo repositorioPrivado, check func(context.Context) error) {
			if !d.vigente(c) {
				return
			}
			c.ctx, c.validar = bound, check
			c.mu.Lock()
			c.repo = repo
			c.mu.Unlock()
			d.preparar(bound)
		})
		if err != nil {
			d.avisar(err)
		}
	}
}

// reabrir cierra lo de esta cuenta y vuelve a elegir dónde vive su Drive: tras
// pasarlo a la bóveda, o tras volver a la copia del servidor.
func (d *drivePrivado) reabrir() {
	d.cerrar()
	d.cuenta, d.dentro = auth.IdentitySnapshot{}, false
	d.cambiar()
}

func (d *drivePrivado) cerrar() {
	drain := d.retirar()
	if d.cerrarPaneles != nil {
		d.cerrarPaneles(drain)
		d.cerrarPaneles = nil
	} else if drain != nil {
		go drain()
	}
}

func (d *drivePrivado) retirar() func() {
	c := d.ciclo
	d.ciclo = nil
	if d.pantalla != nil {
		d.pantalla.Cerrar()
		d.pantalla = nil
	}
	if c == nil {
		return nil
	}
	c.cerrado = true
	if c.cancel != nil {
		c.cancel()
	}
	c.pendientes, c.refresco = nil, nil
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			c.jobs.Wait()
			if repo := c.repositorio(); repo != nil {
				repo.Close()
			}
		})
	}
}

func (d *drivePrivado) preparar(ctx context.Context) {
	c := d.ciclo
	if ctx.Err() != nil || !d.vigente(c) || c.ocupado {
		return
	}
	if d.pantalla != nil {
		d.pantalla.Recargar() //nolint:contextcheck // The presenter retains c.ctx as Vida for every request.
		return
	}
	if c.repositorio() == nil {
		if d.reintentar != nil {
			d.reintentar()
		}
		return
	}
	c.ocupado = true
	c.jobs.Add(1)
	go func() { //nolint:contextcheck // Repository work uses c.ctx; dispatched UI callbacks retain their own lifecycle.
		defer c.jobs.Done()
		repo := c.repositorio()
		err := repo.Initialize(c.ctx)
		d.despachar(func() {
			if !d.vigente(c) {
				return
			}
			c.ocupado = false
			if err != nil {
				if errors.Is(err, mx.ErrRecoveryRequired) {
					d.panel.Recuperar()
				} else {
					d.avisar(err)
				}
				return
			}
			p, err := d.montar(c.ctx, repo)
			if err != nil {
				d.avisar(err)
				return
			}
			d.activar(c, repo, p)
		})
	}()
}

func (d *drivePrivado) activar(c *cicloDrive, repo repositorioPrivado, p *drive.Pantalla) {
	d.pantalla = p
	p.RefrescarDatos = func(listo func()) { d.refrescar(c, listo) }
	pendientes := c.pendientes
	c.pendientes = nil
	for _, fn := range pendientes {
		if !d.vigente(c) {
			return
		}
		fn(p)
	}
	if !d.vigente(c) {
		return
	}
	c.jobs.Add(1)
	go func() {
		defer c.jobs.Done()
		_, err := repo.ResumeUploads(c.ctx)
		d.despachar(func() {
			if !d.vigente(c) {
				return
			}
			if d.pantalla != nil {
				d.pantalla.Recargar()
			}
			if err != nil {
				d.avisar(err)
			}
		})
	}()
}

func (d *drivePrivado) refrescar(c *cicloDrive, listo func()) {
	if !d.vigente(c) {
		return
	}
	c.refresco = listo
	if c.ocupado {
		c.repetir = true
		return
	}
	c.ocupado = true
	c.jobs.Add(1)
	go func() {
		defer c.jobs.Done()
		err := c.repositorio().Refresh(c.ctx)
		d.despachar(func() {
			if !d.vigente(c) {
				return
			}
			c.ocupado = false
			callback := c.refresco
			c.refresco = nil
			if err != nil {
				c.repetir = false
				d.avisar(err)
				return
			}
			if c.repetir {
				c.repetir = false
				d.refrescar(c, callback)
				return
			}
			if callback != nil {
				callback()
			}
		})
	}()
}

// siLista es cuandoLista sin esperar ni abrir nada: con Drive aún sin montar (bloqueado
// o desbloqueándose), lo que se pide se pierde. Es la de las teclas que solo tienen
// sentido sobre lo que ya se ve —Escape, Supr—, que no deben pedir que se desbloquee.
func (d *drivePrivado) siLista(fn func(*drive.Pantalla)) {
	if d.vigente(d.ciclo) && d.pantalla != nil {
		fn(d.pantalla)
	}
}

func (d *drivePrivado) cuandoLista(fn func(*drive.Pantalla)) {
	c := d.ciclo
	if !d.vigente(c) {
		return
	}
	if d.pantalla != nil {
		fn(d.pantalla)
		return
	}
	if len(c.pendientes) < 32 {
		c.pendientes = append(c.pendientes, fn)
	}
	if d.mostrar != nil {
		d.mostrar()
	}
}

// autorizarAlmacen runs interactive root consent before constructing a repository
// or taking its work lease. The existing presenter drain joins this job before
// the panels retire their SDK keys; late UI callbacks cannot adopt another user.
func (d *drivePrivado) autorizarAlmacen(ctx context.Context, prepare func(context.Context) error, ready func()) {
	c := d.ciclo
	if ctx.Err() != nil || !d.vigente(c) || c.ocupado {
		return
	}
	c.ocupado = true
	c.jobs.Add(1)
	go func() {
		defer c.jobs.Done()
		err := prepare(ctx)
		d.despachar(func() {
			if ctx.Err() != nil || !d.vigente(c) {
				return
			}
			c.ocupado = false
			if err != nil {
				d.avisar(err)
				return
			}
			ready()
		})
	}()
}
