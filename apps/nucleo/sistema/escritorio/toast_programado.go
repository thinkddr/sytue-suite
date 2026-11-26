// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"strings"
	"time"
)

// El aviso PROGRAMADO de Windows (`ScheduledToastNotification`): lo entrega el sistema a
// su hora aunque la app esté cerrada. Es lo que usan los recordatorios de Sytue Office
// (`local/avisos`, PLAN-EDICION-LOCAL §5.12).
//
// No se usa el `ScheduleNotification` de Fyne por la misma razón que su `SendNotification`
// (ver `sesion.AvisoDeRespaldo`): interpola título y cuerpo con `%q` en un `.ps1` que
// PowerShell evalúa, y el título de un evento puede venir de un `.ics` ajeno. Aquí todo
// va escapado como en guionToast y por `-EncodedCommand`.

// guionToastProgramado programa el toast para `cuando`, con la etiqueta `etiqueta` (para
// poder quitarlo después) y el enlace que abre al pulsarlo.
func guionToastProgramado(appID, enlace, titulo, cuerpo, etiqueta string, cuando time.Time) string {
	lanzar := ""
	if enlace != "" {
		lanzar = ` activationType="protocol" launch="` + textoXML(enlace) + `"`
	}
	toast := `<toast` + lanzar + `>` +
		`<visual><binding template="ToastGeneric"><text>` + textoXML(titulo) + `</text><text>` +
		textoXML(cuerpo) + `</text></binding></visual></toast>`
	return "[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null\n" +
		"[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null\n" +
		"$xml = New-Object Windows.Data.Xml.Dom.XmlDocument\n" +
		"$xml.LoadXml('" + toast + "')\n" +
		"$cuando = [DateTimeOffset]::Parse('" + cuando.UTC().Format(time.RFC3339) + "')\n" +
		"$s = [Windows.UI.Notifications.ScheduledToastNotification]::new($xml, $cuando)\n" +
		"$s.Tag = '" + etiquetaSegura(etiqueta) + "'\n" +
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('" + comillasLiteralPS.Replace(appID) + "').AddToSchedule($s)\n"
}

// guionCancelarToast quita los toasts programados con esa etiqueta.
func guionCancelarToast(appID, etiqueta string) string {
	return "[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null\n" +
		"$n = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('" + comillasLiteralPS.Replace(appID) + "')\n" +
		"foreach ($s in $n.GetScheduledToastNotifications()) { if ($s.Tag -eq '" + etiquetaSegura(etiqueta) + "') { $n.RemoveFromSchedule($s) } }\n"
}

// etiquetaSegura deja letras, cifras y guiones (Windows admite hasta 64 caracteres; las
// de `local/avisos` son 16 hexadecimales).
func etiquetaSegura(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// ProgramarToast deja a Windows un aviso para `cuando`. Fuera de Windows, ErrNoDisponible.
func ProgramarToast(appID, enlace, titulo, cuerpo, etiqueta string, cuando time.Time) error {
	return ejecutarToast(guionToastProgramado(appID, enlace, titulo, cuerpo, etiqueta, cuando))
}

// CancelarToast quita el aviso programado con esa etiqueta.
func CancelarToast(appID, etiqueta string) error {
	return ejecutarToast(guionCancelarToast(appID, etiqueta))
}
