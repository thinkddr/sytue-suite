// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"context"
	"errors"
)

// # Touch ID y sus equivalentes
//
// Lo que la suite hace con ellos lo decide `apps/fyne/sesion` (bloquear la sesión
// guardada: si está puesto, abrir una app pide al dueño que diga quién es). Aquí solo se
// pregunta si el sistema sabe comprobarlo y se le pide que lo haga, con su diálogo:
//
//   - macOS e iOS: LocalAuthentication, con la política que acepta la huella o la cara
//     y, si no, la contraseña del equipo (o el código del teléfono). Es la misma que usa
//     el propio sistema para desbloquear ajustes. En iOS, Face ID aborta la app si el
//     Info.plist no lleva `NSFaceIDUsageDescription`: lo pone `empaquetar.sh` en las
//     trece, con el texto del catálogo común en cada idioma. ponytail: hoy ninguna
//     pantalla de iOS enciende el bloqueo (la casilla va en la bandeja del Hub de
//     escritorio).
//   - Windows: Windows Hello (`UserConsentVerifier`), que también acepta el PIN.
//   - Android: BiometricPrompt, con la clase propia `eu.sytue.nativo.Biometria` de la
//     APK (apps/fyne/android/java), que acepta la huella o la cara fuertes y, si no, el
//     bloqueo del teléfono. Pide USE_BIOMETRIC, que declara `empaquetar.sh`.
//   - Linux: no hay un servicio común (fprintd va por PAM); no se ofrece.

// ErrNoVerificado es que la persona ha cancelado o el sistema no la ha reconocido.
var ErrNoVerificado = errors.New("escritorio: no se ha verificado a la persona")

// Biometria es cómo se llama aquí la verificación del dueño —«Touch ID», «Face ID»,
// «Optic ID», «Windows Hello»— o "" si el sistema no tiene. Con `soloContrasena` el
// equipo sabe verificar, pero con la contraseña y no con el cuerpo (un Mac sin Touch ID).
func Biometria() (nombre string, soloContrasena bool) { return biometria() }

// Verificar le pide al sistema que compruebe que quien está delante es el dueño, con
// `motivo` en su diálogo. BLOQUEA mientras el diálogo está abierto. Devuelve nil si lo
// es, ErrNoVerificado si no, y ErrNoDisponible si este sistema no sabe comprobarlo.
func Verificar(ctx context.Context, motivo string) error { return verificar(ctx, motivo) }

// respuestaBiometricPrompt traduce lo que devuelve `Biometria.verificar` de Android: un
// fallo ANTES de pedir el diálogo (sin la clase, sin nada con que verificar) es
// ErrNoDisponible; 0 es verificado, y cualquier otro código —cancelar, demasiados
// intentos, -1 si Java falló con el diálogo pedido— es ErrNoVerificado. Aquí y no en el
// fichero de Android para que su guardián corra en cualquier sistema.
func respuestaBiometricPrompt(antes error, codigo int) error {
	switch {
	case antes != nil:
		return ErrNoDisponible
	case codigo == 0:
		return nil
	}
	return ErrNoVerificado
}
