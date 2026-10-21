// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"strings"
	"unicode/utf16"
)

// El toast de Windows se escribe aquí, sin etiqueta de sistema, para que su guardián
// corra en Linux: lo que se decide —el XML, las comillas, el enlace— es texto, y lo
// único que pide Windows es ejecutarlo (escritorio_windows.go).

// guionToast es el PowerShell que enseña el toast con la plantilla genérica de Windows
// 10 y 11 y `activationType="protocol"`, que al pulsarlo abre `enlace`. El
// `SendNotification` de Fyne usa la plantilla vieja `ToastImageAndText02`, que no admite
// ni `launch` ni el tipo de activación: por eso este va aparte.
func guionToast(appID, enlace, titulo, cuerpo string) string {
	toast := `<toast activationType="protocol" launch="` + textoXML(enlace) + `">` +
		`<visual><binding template="ToastGeneric"><text>` + textoXML(titulo) + `</text><text>` +
		textoXML(cuerpo) + `</text></binding></visual></toast>`
	return "[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null\n" +
		"[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null\n" +
		"$xml = New-Object Windows.Data.Xml.Dom.XmlDocument\n" +
		"$xml.LoadXml('" + toast + "')\n" +
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('" + comillasLiteralPS.Replace(appID) + "')" +
		".Show([Windows.UI.Notifications.ToastNotification]::new($xml))\n"
}

// comillasPS son las que PowerShell toma por comilla simple: la recta y las cuatro
// tipográficas. Una sola ’ en «l’application» cerraría la cadena y lo que sigue se
// ejecutaría como órdenes.
var comillasPS = strings.NewReplacer("'", "&#39;", "‘", "&#x2018;", "’", "&#x2019;",
	"‚", "&#x201A;", "‛", "&#x201B;")

// comillasLiteralPS escapa las cinco comillas simples que PowerShell reconoce como
// delimitador de cadena literal ('…') duplicándolas, y quita saltos de línea para
// argumentos que no son XML (como el AUMID de CreateToastNotifier).
var comillasLiteralPS = strings.NewReplacer(
	"'", "''",
	"‘", "‘‘",
	"’", "’’",
	"‚", "‚‚",
	"‛", "‛‛",
	"\r", " ",
	"\n", " ",
)

// textoXML escapa para el XML del toast y deja fuera cualquier comilla simple, así que
// cabe entre las comillas de PowerShell tal cual.
func textoXML(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return comillasPS.Replace(b.String())
}

// codificado es el guion como lo pide `powershell -EncodedCommand`: UTF-16LE en base64.
// Por argumento y no por fichero —lo que hace Fyne— para no dejar un .ps1 por aviso en
// la carpeta temporal.
func codificado(guion string) string {
	u := utf16.Encode([]rune(guion))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}
