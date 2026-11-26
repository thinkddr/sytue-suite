// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build edicionlocal

package ui

import (
	"context"
	"errors"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/anfitrion"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Sytue Office: una sesión que nunca entra
//
// Con la etiqueta `edicionlocal` (PLAN-EDICION-LOCAL §5.3) `ui` no importa `sesion`: los
// binarios de Sytue Office no inician sesión, y por eso no enlazan `sesion` ni su login
// (C-04; lo vigila `apps/fyne/local/guardian_test.go`). Su barra es la de `local`
// (`local.App.Montar`), no [BarraSuite]; esto solo deja el MISMO API de `ui` para que
// las pantallas compartidas que reciben una barra (`*BarraSuite`, siempre nil aquí)
// compilen igual.
//
// Es el espejo de `suite.go`: cada nombre de allí tiene aquí su versión vacía. Nadie la
// construye; si algo llegara a llamarla, se comporta como una suite sin sesión.

type suiteSesion struct {
	Sesion *sesionNula
	Hub    *api.Hub
}

// estadoSesion es [sesion.Estado] con lo que lee la barra.
type estadoSesion struct {
	Sujeto, GeneracionSesion, EpocaSesion string
	Tema, Color, Forma                    string
	ColorPropio, FormaPropia              string
	ConOrganizacion                       bool
	ColorOrg, Idioma                      string
	Presencia                             api.EstadoPresencia
	Mensaje                               string
}

// Dentro nunca: no hay sesión.
func (e estadoSesion) Dentro() bool { return e.Sujeto != "" }

var errSinSesion = errors.New("ui: Sytue Office no tiene sesión")

type autorizacionSalida struct{}

func (autorizacionSalida) Vigente() error                     { return errSinSesion }
func (autorizacionSalida) Cerrar(context.Context, bool) error { return errSinSesion }

// sesionNula cumple api.Session sin token: toda petición sale sin credencial.
type sesionNula struct{}

func (*sesionNula) AccessToken(context.Context) (string, error) { return "", errSinSesion }
func (*sesionNula) Invalidate(string)                           {}
func (*sesionNula) Organizacion() string                        { return "" }

func (*suiteSesion) Estado() estadoSesion                                         { return estadoSesion{} }
func (*suiteSesion) AlCambiar(func(estadoSesion))                                 {}
func (*suiteSesion) AlBuscar(func())                                              {}
func (*suiteSesion) AlConfirmarSubidaLocal(func(fyne.Window, string, func(bool))) {}
func (*suiteSesion) AlFondo(func())                                               {}
func (*suiteSesion) AlVolver(func())                                              {}
func (*suiteSesion) AlNoEncontrar(func())                                         {}
func (*suiteSesion) CuandoHayaEscena(func())                                      {}
func (*suiteSesion) PausaEnFondo() bool                                           { return false }
func (*suiteSesion) AvisoDelHub(string)                                           {}
func (*suiteSesion) Apex() string                                                 { return "" }
func (*suiteSesion) URL(string) string                                            { return "" }
func (*suiteSesion) Entrar(context.Context) error                                 { return errSinSesion }

func (*suiteSesion) Poner(func(*ajustes.Ajustes)) error { return errSinSesion }

func (*suiteSesion) PonerPresencia(context.Context, api.EstadoPresencia, string) error {
	return errSinSesion
}

func (*suiteSesion) AutorizarSalida() (autorizacionSalida, error) {
	return autorizacionSalida{}, errSinSesion
}
func (*suiteSesion) PendientesSalida() (int, error) { return 0, errSinSesion }
func (*suiteSesion) SubirNotepadsAutorizado(context.Context, autorizacionSalida) int {
	return 0
}
func (*suiteSesion) DespertarDisponible() bool { return false }
func (*suiteSesion) DespertarEncendido() bool  { return false }
func (*suiteSesion) PonerDespertar(bool) error { return errSinSesion }

// avisoDeRespaldo es el de siempre, sin `sesion` (vive en `anfitrion`).
func avisoDeRespaldo(a fyne.App, titulo, cuerpo string) { anfitrion.AvisoDeRespaldo(a, titulo, cuerpo) }

// appAbierta: sin enchufes de la suite, ninguna.
func appAbierta(string) bool { return false }
