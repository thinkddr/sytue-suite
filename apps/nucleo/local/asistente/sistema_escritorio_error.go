// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package asistente

// AtajoIA es el atajo de la app Atajos que hace de puente con Apple Intelligence.
//
// Apple no da a las apps de fuera de Swift una API de su modelo (Foundation Models es
// solo Swift). Lo que sí da, desde macOS 26, es la acción «Usar modelo» de Atajos con el
// modelo del dispositivo o Private Cloud Compute, y la orden `shortcuts` para ejecutar un
// atajo con un fichero de entrada. La persona crea una vez un atajo con este nombre que
// recibe texto, lo pasa a «Usar modelo» y devuelve la respuesta (README de la edición).
const AtajoIA = "Sytue Office IA"

// ErrorEscritorio es lo que falla de la IA del sistema en el escritorio (el atajo de
// Atajos en macOS, Foundry Local en Windows), con un código para que la pantalla lo diga
// en el idioma de la app; Error() es el español de los registros. Desenvuelve a
// ErrSinIANativa.
type ErrorEscritorio struct {
	Codigo string
	// Detalle es lo que dijo el sistema (la salida del atajo que falló), si lo hay.
	Detalle string
}

// Códigos de ErrorEscritorio.
const (
	EscritorioFaltaAtajo    = "falta-atajo"    // macOS: no está el atajo AtajoIA
	EscritorioFalloAtajo    = "fallo-atajo"    // macOS: el atajo falló (Detalle)
	EscritorioSinTexto      = "sin-texto"      // macOS: Apple Intelligence no devolvió texto
	EscritorioFaltaFoundry  = "falta-foundry"  // Windows: Foundry Local no está instalado
	EscritorioFoundryParado = "foundry-parado" // Windows: Foundry Local no está en marcha
)

func (e *ErrorEscritorio) Error() string {
	switch e.Codigo {
	case EscritorioFaltaAtajo:
		return "asistente: crea en Atajos el atajo «" + AtajoIA + "» con la acción «Usar modelo»"
	case EscritorioFalloAtajo:
		return "asistente: el atajo falló: " + e.Detalle
	case EscritorioSinTexto:
		return "asistente: Apple Intelligence no devolvió texto"
	case EscritorioFaltaFoundry:
		return "asistente: instala Foundry Local («winget install Microsoft.FoundryLocal») y arranca un modelo"
	case EscritorioFoundryParado:
		return "asistente: Foundry Local no está en marcha («foundry service start»)"
	}
	return ErrSinIANativa.Error()
}

// Unwrap es ErrSinIANativa.
func (e *ErrorEscritorio) Unwrap() error { return ErrSinIANativa }
