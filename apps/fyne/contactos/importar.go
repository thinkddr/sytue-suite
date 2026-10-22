// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"context"
	"log/slog"
	"regexp"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Importar un `.vcf` o un CSV a la libreta activa: `VcardToolbar` de ContactsApp.tsx.

// reTarjeta separa las tarjetas de un `.vcf` (`/BEGIN:VCARD[\s\S]*?END:VCARD/gi`).
var reTarjeta = regexp.MustCompile(`(?is)BEGIN:VCARD.*?END:VCARD`)

// desdoblar une las líneas plegadas (RFC 6350 §3.2: una línea que empieza por espacio
// o tabulador continúa la anterior).
var desdoblar = regexp.MustCompile(`\r?\n[ \t]`)

// reNombre es una línea FN con algo escrito, con o sin grupo (`item1.FN`).
var reNombre = regexp.MustCompile(`(?im)^(?:[a-z0-9-]+\.)?FN(?:;[^:\r\n]*)?:[ \t]*\S`)

// Tarjetas es `parseVcf`: cada tarjeta del fichero TAL CUAL, sin las que no traen nombre
// (FN). Aquí no se interpreta nada más: la tarjeta entera va al servicio
// (`api.Contacts.ImportCard`), que es quien conserva la foto y los X-* de otros
// clientes, igual que el BFF de la web con su `baseVcard`.
func Tarjetas(texto string) []string {
	var fuera []string
	for _, t := range reTarjeta.FindAllString(texto, -1) {
		if reNombre.MatchString(desdoblar.ReplaceAllString(t, "")) {
			fuera = append(fuera, t)
		}
	}
	return fuera
}

// ---- En la App ----

// importarVCard crea, UNA a una y en serie para no ametrallar el servicio, las tarjetas
// del fichero, con el avance real en el propio botón («Importando 3 de 20»): el total se
// sabe antes de la primera escritura. Bloquea: en segundo plano.
func (a *App) importarVCard(datos []byte) {
	var libreta string
	fyne.DoAndWait(func() {
		if a.Pantalla.Activa != nil {
			libreta = a.Pantalla.Activa.ID
		}
	})
	if libreta == "" {
		return
	}
	entradas := Tarjetas(string(datos))
	hechas, fallos := 0, 0
	avance := func() {
		fyne.Do(func() {
			p := a.Pantalla
			p.ImportOcupado = true
			p.Importando = p.t("io.importProgress", i18n.Vars{"done": hechas + fallos, "total": len(entradas)})
			a.repintar()
		})
	}
	avance()
	for _, tarjeta := range entradas {
		ctx, fin := a.ctx()
		_, err := a.svc.ImportCard(ctx, libreta, tarjeta)
		fin()
		if err != nil {
			fallos++
		} else {
			hechas++
		}
		avance()
	}
	fyne.Do(func() { a.informarImportacion(hechas, fallos, "io.noneImportedVcard") })
}

// importarCSV manda el CSV entero en UNA petición: el servicio detecta el dialecto
// (Google u Outlook), se salta las filas malas y no repite correos.
func (a *App) importarCSV(datos []byte) {
	var libreta string
	fyne.DoAndWait(func() {
		if p := a.Pantalla; p.Activa != nil {
			libreta = p.Activa.ID
			p.ImportOcupado, p.Importando = true, ""
			a.repintar()
		}
	})
	if libreta == "" {
		return
	}
	ctx, fin := context.WithTimeout(context.Background(), 4*a.Plazo)
	n, omitidos, err := a.svc.ImportCSV(ctx, libreta, datos)
	fin()
	fyne.Do(func() {
		if err != nil {
			slog.Warn("contactos: no se pudo importar el CSV", "error", err)
			a.Pantalla.ImportOcupado = false
			a.avisar(ui.AvisoPeligro, "io.csvFailed", "io.csvFailedHint")
			a.repintar()
			return
		}
		a.informarImportacion(n, omitidos, "io.noneImportedCsv")
	})
}

// informarImportacion es `reportImport`: cuántos entraron (y cuántos se omitieron), o
// que no entró ninguno y por qué. En el hilo de la interfaz.
func (a *App) informarImportacion(hechas, omitidas int, sinNinguno string) {
	p := a.Pantalla
	p.ImportOcupado, p.Importando = false, ""
	if hechas == 0 {
		a.avisar(ui.AvisoPeligro, "io.noneImported", sinNinguno)
		a.repintar()
		return
	}
	d := ""
	if omitidas > 0 {
		d = p.t("io.skipped", i18n.Vars{"count": omitidas})
	}
	a.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.t("io.imported", i18n.Vars{"count": hechas}), d))
	if p.Activa != nil {
		a.recargar(*p.Activa)
	}
}
