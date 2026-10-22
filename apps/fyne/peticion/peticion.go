// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package peticion es la franja de solo lectura CON su «Pedir acceso de edición», para
// cualquier app nativa: el espejo de `FranjaPeticion.tsx` de @sytue/shell. La pieza
// visual es `ui.FranjaSoloLectura`; aquí está el contrato común de
// platform/pkg/peticiones (`api.Peticiones`), que es igual en todos los servicios.
//
// Tres caras, como en la web:
//   - (b) sin permiso: «Pedir acceso de edición» y, enviada, «Petición enviada».
//   - quien puede conceder: «Ana pide acceso de edición» con «Dar acceso» y «Rechazar».
//   - (a) el bloqueo es de la persona: no hace falta esto; `ui.FranjaSoloLectura` con «Editar».
package peticion

import (
	"context"
	"sync/atomic"
	"time"
	"weak"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Plazo acota cada llamada al servicio.
var Plazo = 15 * time.Second

// Estado es lo que la franja sabe de las peticiones de UN recurso.
type Estado struct {
	// Pedir es el botón «Pedir acceso de edición»; nil = la franja solo informa.
	Pedir func()
	// Pedida cambia el botón por «Petición enviada».
	Pedida bool
	// Quien es, para quien concede, el primero que lo ha pedido. Vacío = nadie.
	Quien              string
	Conceder, Rechazar func()

	// dejarDeOir corta la escucha del flujo del Hub de la vez anterior.
	dejarDeOir func()
	// escucha es lo que la escucha del Hub alcanza, por un puntero débil: vive lo que
	// viva este Estado.
	escucha *escucha
}

type escucha struct{ o Opciones }

// Soltar deja de escuchar el Hub (al cerrar la pantalla).
func (e *Estado) Soltar() {
	if e.dejarDeOir != nil {
		e.dejarDeOir()
		e.dejarDeOir = nil
	}
}

// Opciones dice qué enseñar.
type Opciones struct {
	// SinPermiso: no puede editar y el motivo es el permiso (no un documento que no se
	// entendió, ni la falta de red): se le ofrece pedirlo.
	SinPermiso bool
	// PuedeConceder: se le enseñan las peticiones pendientes.
	PuedeConceder bool
	// YaPedida, si la da el servicio, dice si ya lo pidió (al volver a abrir).
	YaPedida func(context.Context) (bool, error)
	// Recomponer repinta la pantalla con el estado nuevo (en el hilo de la interfaz).
	Recomponer func()
	// Fallo avisa de que la petición no salió (texto ya traducido).
	Fallo func(texto string)
	T     func(string, i18n.Vars) string
	// EN VIVO, para las apps sin canal propio: al llegar por el flujo del Hub un aviso de
	// tipo Concedido (`calendar.access.granted`…) con `params.resource` == Recurso, se
	// llama a AlConceder en el hilo de la interfaz y la app vuelve a leer su permiso.
	Concedido, Recurso string
	AlConceder         func()
}

// Enganchar rellena `e` según las opciones; lo que pide al servicio va en segundo plano
// y termina con Recomponer. Llamarla en cada carga: rehace el estado anterior.
func Enganchar(e *Estado, p api.Peticiones, o Opciones) {
	e.Soltar()
	*e = Estado{}
	enSegundo := func(f func(context.Context)) {
		go func() {
			ctx, cancelar := context.WithTimeout(context.Background(), Plazo)
			defer cancelar()
			f(ctx)
		}()
	}
	if o.SinPermiso {
		e.Pedir = func() {
			enSegundo(func(ctx context.Context) {
				err := p.Pedir(ctx)
				fyne.Do(func() {
					if err != nil {
						if o.Fallo != nil {
							o.Fallo(o.T("readOnly.requestFailed", nil))
						}
						return
					}
					e.Pedida = true
					o.Recomponer()
				})
			})
		}
		if o.YaPedida != nil {
			enSegundo(func(ctx context.Context) {
				if hay, err := o.YaPedida(ctx); err == nil && hay {
					fyne.Do(func() { e.Pedida = true; o.Recomponer() })
				}
			})
		}
	}
	if o.PuedeConceder {
		mirarPendientes(e, p, o.Recomponer, enSegundo)
	}
	if o.SinPermiso && o.Concedido != "" && o.AlConceder != nil {
		oirConcesion(e, o)
	}
}

// mirarPendientes enseña la primera petición con «Dar acceso» y «Rechazar»; al
// resolverla, mira si hay otra.
func mirarPendientes(e *Estado, p api.Peticiones, recomponer func(), enSegundo func(func(context.Context))) {
	enSegundo(func(ctx context.Context) {
		ps, err := p.Pendientes(ctx)
		if err != nil {
			return
		}
		fyne.Do(func() {
			e.Quien, e.Conceder, e.Rechazar = "", nil, nil
			if len(ps) > 0 {
				sub := ps[0].Sub
				resolver := func(concede bool) func() {
					return func() {
						enSegundo(func(ctx context.Context) {
							_ = p.Resolver(ctx, sub, concede)
							mirarPendientes(e, p, recomponer, enSegundo) // la siguiente, si hay
						})
					}
				}
				e.Quien, e.Conceder, e.Rechazar = ps[0].Quien(), resolver(true), resolver(false)
			}
			recomponer()
		})
	})
}

// Franja es la franja que toca: la de solo lectura (con su botón o su nota) si
// `soloLectura`, la del que concede si hay una petición, o nil.
func Franja(e *Estado, soloLectura bool, mensaje string, t func(string, i18n.Vars) string) *ui.FranjaSoloLectura {
	switch {
	case soloLectura:
		f := ui.NuevaFranjaSoloLectura(mensaje)
		switch {
		case e.Pedida:
			f.Nota = t("readOnly.requestSent", nil)
		case e.Pedir != nil:
			f.Con(t("readOnly.requestEdit", nil), e.Pedir)
		}
		return f
	case e.Quien != "":
		return ui.NuevaFranjaSoloLectura(t("readOnly.editRequest", i18n.Vars{"name": e.Quien})).
			Con(t("readOnly.grant", nil), e.Conceder).
			ConSecundaria(t("readOnly.decline", nil), e.Rechazar)
	}
	return nil
}

// oirConcesion escucha en el flujo del Hub el aviso de acceso concedido. La escucha es de
// la app entera y no sabe cuándo se va la pantalla, así que no le alarga la vida: guarda
// la pantalla (sus Opciones) con un puntero débil y, si ya no está, se quita al siguiente
// aviso. Con un puntero normal, cada pantalla que enseñó la franja quedaba viva para
// siempre.
func oirConcesion(e *Estado, o Opciones) {
	e.escucha = &escucha{o: o}
	oyente := weak.Make(e.escucha)
	var quitar atomic.Pointer[func()]
	q := ui.AlLlegarAviso(func(n api.HubNotification) {
		x := oyente.Value()
		if x == nil {
			if f := quitar.Load(); f != nil {
				(*f)()
			}
			return
		}
		if n.Type == x.o.Concedido && (x.o.Recurso == "" || n.Params["resource"] == x.o.Recurso) {
			fyne.Do(x.o.AlConceder)
		}
	})
	quitar.Store(&q)
	e.dejarDeOir = q
}
