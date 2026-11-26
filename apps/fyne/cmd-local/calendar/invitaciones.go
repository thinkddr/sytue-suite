// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Las invitaciones por correo (iMIP, A-11): en un calendario de una cuenta CalDAV cuyo
// servidor no las manda él, lo que la persona organiza (y su respuesta a lo que organiza
// otro) sale por la cuenta de Sytue Mail de este equipo con esa dirección, por su SMTP,
// sin abrir el buzón de Mail —el mismo camino que la factura del ERP— y solo hacia sus
// servidores, por el portero. Qué y a quién lo decide `agenda.Cartero`; aquí se le da
// el correo y se avisa de lo que no salió.

// correoDeMail son las cuentas de Mail de este equipo y su envío (`correo.Remitente`).
type correoDeMail struct{ r *correo.Remitente }

func nuevoCorreo(l *local.App) correoDeMail {
	return correoDeMail{correo.NuevoRemitente(filepath.Join(l.Espacio.Rutas.Datos, "Correo"), l.Secretos,
		correo.Red{Marcar: l.Red.Marcador(red.Correo), HTTP: l.Red.Cliente(red.OAuth)})}
}

func (c correoDeMail) Buzones() ([]agenda.Buzon, error) {
	cs, err := c.r.Cuentas()
	out := make([]agenda.Buzon, 0, len(cs))
	for _, x := range cs {
		out = append(out, agenda.Buzon{ID: x.ID, Direccion: x.Direccion, Nombre: x.Nombre})
	}
	return out, err
}

func (c correoDeMail) Enviar(ctx context.Context, buzon string, para []string, crudo []byte) error {
	return c.r.Enviar(ctx, buzon, para, crudo)
}

// textosIMIP son los del correo de las invitaciones, en el idioma de la app.
func textosIMIP(l *local.App) agenda.TextosIMIP {
	return agenda.TextosIMIP{
		Invitacion: texto(l, "imip.invitacion"), Actualizada: texto(l, "imip.actualizada"), Cancelada: texto(l, "imip.cancelada"),
		Cuando: texto(l, "imip.cuando"), Donde: texto(l, "imip.donde"), Organiza: texto(l, "imip.organiza"),
	}
}

// avisosIMIP son las frases de lo que no salió, una por evento: sin cuenta de Mail con
// la dirección, o el envío que falló. Lo demás va al registro.
func avisosIMIP(l *local.App, errs []error) []string {
	var out []string
	for _, err := range errs {
		var sin agenda.SinRemitente
		var fallo agenda.FalloEnvio
		var frase string
		switch {
		case errors.As(err, &sin):
			frase = fmt.Sprintf(texto(l, "imip.sinremitente"), sin.Resumen, sin.Direccion)
		case errors.As(err, &fallo):
			slog.Warn("calendar: no salieron las invitaciones por correo", "error", err)
			frase = fmt.Sprintf(texto(l, "imip.fallo"), fallo.Resumen, fallo.Direccion)
		default:
			slog.Warn("calendar: invitaciones por correo", "error", err)
			continue
		}
		if !strings.Contains(strings.Join(out, "\n"), frase) {
			out = append(out, frase)
		}
	}
	return out
}
