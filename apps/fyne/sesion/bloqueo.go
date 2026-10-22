// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// # Bloquear la sesión con Touch ID
//
// Es una preferencia compartida (`ajustes.Bloqueo`) como el tema: se pone una vez y vale
// para las trece apps. Con ella puesta, cada app pide al abrirse que el dueño diga quién
// es con lo que tenga el sistema —Touch ID, Face ID, Windows Hello, la contraseña del
// equipo— antes de leer la sesión del llavero; si no lo dice, no abre.
//
// Protege de quien se sienta delante de un equipo desbloqueado, que es de lo que protege
// en cualquier app que lo ofrezca. No de un programa del propio usuario: ese ya puede
// leer el llavero.
//
// ponytail: cada app pregunta al abrirse, aunque otra lo haya preguntado hace un
// minuto; un plazo de gracia compartido se añade si abrir varias seguidas molesta.

// desbloqueo es lo que salió de pedir la verificación al abrir.
type desbloqueo int

const (
	verificado     desbloqueo = iota // el dueño ha dicho quién es
	rechazado                        // no lo ha dicho: la app no abre
	sinVerificador                   // el equipo no sabe preguntarlo
)

// desbloquear pide la verificación al abrir.
//
// Un equipo que ya no sabe verificar (Touch ID y contraseña retirados, o un sistema
// sin nada) ni deja al dueño fuera ni abre sin más: devuelve sinVerificador y quien
// llama pide la contraseña de Sytue (decisión del dueño, 2026-10-15). Antes abría la
// sesión guardada sin preguntar nada (NAT-09).
//
// Sin traducciones se pregunta igual, con el motivo en inglés: antes, un fallo al
// cargarlas abría la sesión sin preguntar (NAT-09).
func desbloquear(e ajustes.Estado) desbloqueo {
	motivo := "Unlock Sytue"
	if t, err := i18n.New(i18n.Negotiate(sistema.Idioma(), e.Idioma), i18n.Common()); err == nil {
		motivo = t.T("shell.lock.reason", nil)
	}
	switch err := escritorio.Verificar(context.Background(), motivo); {
	case err == nil:
		return verificado
	case errors.Is(err, escritorio.ErrNoDisponible):
		return sinVerificador
	default:
		return rechazado
	}
}

// pedirSytueIDAlAbrir aplica el bloqueo al abrir: sin bloqueo o verificado, sigue
// (false); sin forma de verificar en este equipo, hay que entrar con la cuenta de
// Sytue (true); si el dueño no se ha identificado, la app no abre.
func pedirSytueIDAlAbrir(e ajustes.Estado) bool {
	if !bloqueoPuesto(e) {
		return false
	}
	switch desbloquear(e) {
	case verificado:
		return false
	case sinVerificador:
		return true
	case rechazado:
	}
	os.Exit(0) //nolint:revive // deep-exit deliberado: entrar por el navegador sería saltarse el bloqueo
	return false
}

// prepararReidentificacion hace que el login de esta app pida la contraseña de Sytue
// aunque el navegador tenga sesión, y lo dice con un aviso del sistema.
func (s *Suite) prepararReidentificacion(e ajustes.Estado) {
	s.oidc.MaxAge = reidentificacionBloqueo
	if t, err := i18n.New(i18n.Negotiate(sistema.Idioma(), e.Idioma), i18n.Common()); err == nil {
		s.avisarSistema(t.T("shell.lock.signInTitle", nil), t.T("shell.lock.signInBody", nil), nil)
	}
}

// reidentificacionBloqueo es el `max_age` (segundos) del login cuando el bloqueo no
// se puede comprobar con el sistema: la sesión que el navegador tenga con Sytue ID no
// basta, hay que volver a escribir la contraseña. No 0: Sytue ID compara con «>», y a
// la vuelta del login la sesión nueva ya tendría unos milisegundos.
const reidentificacionBloqueo = 60

// cuentaBloqueo es la marca del bloqueo en el almacén seguro (NAT-09). `ajustes.json`
// es un fichero que cualquiera con el equipo abierto edita; la marca del llavero no se
// quita sin el propio llavero, así que borrar `"bloqueo": true` ya no basta.
const cuentaBloqueo = auth.CuentaBloqueo

// bloqueoPuesto dice si hay que pedir la verificación: lo dicen los ajustes O el
// almacén seguro.
func bloqueoPuesto(e ajustes.Estado) bool {
	if e.Bloqueo {
		return true
	}
	marca, err := auth.LeerSecreto(cuentaBloqueo)
	return err == nil && len(marca) > 0
}

// marcarBloqueo pone o quita la marca del almacén seguro, junto con el ajuste.
func marcarBloqueo(puesto bool) error {
	if puesto {
		return auth.GuardarSecreto(cuentaBloqueo, []byte("1"))
	}
	return auth.BorrarSecreto(cuentaBloqueo)
}

// ElementoBloqueo es «Pedir Touch ID al abrir» para el menú de la bandeja, o nil si este
// sistema no sabe verificar al dueño. Ponerlo y quitarlo también pide la verificación: si
// no, cualquiera que pasara por delante lo quitaría.
func (s *Suite) ElementoBloqueo(t *i18n.Translator, menu *fyne.Menu) *fyne.MenuItem {
	nombre, soloContrasena := escritorio.Biometria()
	if nombre == "" && !soloContrasena {
		return nil
	}
	if nombre == "" {
		nombre = t.T("shell.lock.devicePassword", nil)
	}
	item := fyne.NewMenuItem(t.T("shell.lock.toggle", i18n.Vars{"metodo": nombre}), nil)
	item.Checked = ajustes.Leer().Bloqueo
	item.Action = func() {
		quiere := !item.Checked
		go func() {
			if err := escritorio.Verificar(context.Background(), t.T("shell.lock.reason", nil)); err != nil {
				slog.Info("sesion: el bloqueo no se cambia sin verificar al dueño", "error", err)
				return
			}
			if err := marcarBloqueo(quiere); err != nil {
				slog.Warn("sesion: no se ha podido guardar el bloqueo en el almacén seguro", "error", err)
				return
			}
			if err := s.Poner(func(a *ajustes.Ajustes) { a.Bloqueo = quiere }); err != nil {
				slog.Warn("sesion: no se ha podido guardar el bloqueo", "error", err)
				return
			}
			fyne.Do(func() {
				item.Checked = quiere
				menu.Refresh()
			})
		}()
	}
	return item
}
