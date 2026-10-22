// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package avisos escribe por correo a quien lleva las obligaciones de una
// organización cuando faltan diez días para su fecha.
//
// Corre dentro del servicio, sin la sesión de nadie: cada media hora mira qué
// organizaciones tienen perfil y, a partir de las ocho de la mañana de Madrid,
// manda los avisos del día. Que no se mande dos veces lo decide la base (una fila
// por obligación, periodo y tipo), no el reloj de una réplica.
package avisos

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // la imagen es distroless: sin esto no hay Europe/Madrid

	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// DiasAntes es con cuánta antelación a «tu fecha» se escribe.
const DiasAntes = 10

// tipoAviso distingue este aviso de otros que vengan después en avisos_enviados.
const tipoAviso = "10d"

// Deps es lo que necesita la pasada.
type Deps struct {
	Store        *store.Store
	Obligaciones *obligaciones.Catalogo
	Correo       correo.Config
	// Enlace es la página de obligaciones del ERP, para el correo.
	Enlace string
	Log    *slog.Logger
}

// Ejecutar hace una pasada cada media hora hasta que se cancele el contexto.
func Ejecutar(ctx context.Context, d Deps) {
	// Sin credencial no se intenta: tanto el MSA de Sytue Mail como el relé exigen
	// autenticarse, y un intento cada media hora solo llenaría el log de rechazos.
	if !d.Correo.Activo() || d.Correo.User == "" || d.Correo.Password == "" {
		d.Log.Info("avisos de obligaciones por correo desactivados: falta SMTP_FROM o la credencial (smtp-user, smtp-password)")
		return
	}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		d.Log.Error("avisos: sin zona horaria de Madrid", "error", err)
		return
	}
	tic := time.NewTicker(30 * time.Minute)
	defer tic.Stop()
	for {
		if ahora := time.Now().In(madrid); ahora.Hour() >= 8 {
			if n, err := Pasada(ctx, d, ahora); err != nil {
				d.Log.Warn("avisos: pasada con errores", "enviados", n, "error", err)
			} else if n > 0 {
				d.Log.Info("avisos: enviados", "correos", n)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tic.C:
		}
	}
}

// Pasada manda los avisos que tocan hoy y devuelve cuántos correos salieron.
func Pasada(ctx context.Context, d Deps, ahora time.Time) (int, error) {
	tenants, err := d.Store.TenantsConPerfil(ctx)
	if err != nil {
		return 0, err
	}
	hoy := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, time.UTC)
	enviados := 0
	var fallos []string
	for _, t := range tenants {
		n, err := pasadaDe(tenancy.Into(ctx, tenancy.Tenant{Slug: t}), d, hoy)
		enviados += n
		if err != nil {
			fallos = append(fallos, t+": "+err.Error())
		}
	}
	if len(fallos) > 0 {
		return enviados, fmt.Errorf("%s", strings.Join(fallos, "; "))
	}
	return enviados, nil
}

func pasadaDe(ctx context.Context, d Deps, hoy time.Time) (int, error) {
	org, err := d.Store.Organizacion(ctx)
	if err != nil {
		return 0, err
	}
	perfil := d.Obligaciones.PerfilDe(org.Pais, org.Forma, org.CP, org.Condiciones)
	var tocan []obligaciones.Instancia
	for _, anio := range []int{hoy.Year(), hoy.Year() + 1} {
		inst, err := d.Obligaciones.Calendario(perfil, anio)
		if err != nil {
			return 0, err
		}
		tocan = append(tocan, Tocan(inst, hoy)...)
	}
	if len(tocan) == 0 {
		return 0, nil
	}
	para, err := d.Store.DestinatariosDe(ctx, permisos.Obligaciones, permisos.PorDefecto(permisos.Obligaciones) != permisos.Ninguno)
	if err != nil || len(para) == 0 {
		return 0, err
	}
	enviados := 0
	for _, i := range tocan {
		enviados += avisar(ctx, d, org.Nombre, org.Pais, i, para)
	}
	return enviados, nil
}

// avisar manda el aviso de una obligación a quien toca, una sola vez: si no le
// llega a nadie, se libera para la pasada siguiente.
func avisar(ctx context.Context, d Deps, organizacion, pais string, i obligaciones.Instancia, para []store.Destinatario) int {
	reclamado, err := d.Store.ReclamarAviso(ctx, i.Clave, i.Periodo, tipoAviso)
	if err != nil || !reclamado {
		return 0
	}
	faltan := map[string]int{}
	if desde, hasta, ok := obligaciones.RangoDePeriodo(i.Periodo); ok {
		if porMes, err := d.Store.Pendientes(ctx, desde, hasta); err == nil {
			for clave, meses := range porMes {
				for _, n := range meses {
					faltan[clave] += n
				}
			}
		}
	}
	asunto, cuerpo := Correo(organizacion, pais, i, faltan, d.Enlace)
	salieron := 0
	for _, p := range para {
		if err := correo.Enviar(ctx, d.Correo, p.Email, asunto, cuerpo); err != nil {
			d.Log.Warn("avisos: no salió un correo", "obligacion", i.Clave, "periodo", i.Periodo, "error", err)
			continue
		}
		salieron++
	}
	if salieron == 0 {
		_ = d.Store.LiberarAviso(ctx, i.Clave, i.Periodo, tipoAviso)
	}
	return salieron
}

// Tocan son las obligaciones cuya «tu fecha» cae justo dentro de DiasAntes días.
func Tocan(inst []obligaciones.Instancia, hoy time.Time) []obligaciones.Instancia {
	objetivo := hoy.AddDate(0, 0, DiasAntes)
	var out []obligaciones.Instancia
	for _, i := range inst {
		if i.Recomendada.Equal(objetivo) {
			out = append(out, i)
		}
	}
	return out
}

var meses = []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

func fechaLarga(t time.Time, pais string) string {
	if pais == "ES" {
		return fmt.Sprintf("%d de %s de %d", t.Day(), meses[t.Month()-1], t.Year())
	}
	return t.Format("2 January 2006")
}

// Correo es el asunto y el texto del aviso. En castellano para una organización
// española —las guías de sus obligaciones lo están— y en inglés para el resto.
func Correo(organizacion, pais string, i obligaciones.Instancia, faltan map[string]int, enlace string) (string, string) {
	periodo := obligaciones.EtiquetaPeriodo(i.Periodo)
	if pais != "ES" {
		asunto := fmt.Sprintf("10 days left: %s · %s", i.Nombre, periodo)
		cuerpo := fmt.Sprintf("Hello,\n\n%s · %s for %s is due on your date, %s (legal deadline %s).\n\nOpen Sytue ERP: %s\n",
			i.Nombre, periodo, organizacion, fechaLarga(i.Recomendada, pais), fechaLarga(i.Limite, pais), enlace)
		return asunto, cuerpo
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Hola:\n\nQuedan %d días para tu fecha de %s · %s (%s).\n\n", DiasAntes, i.Nombre, periodo, organizacion)
	fmt.Fprintf(&b, "Tu fecha: %s\n", fechaLarga(i.Recomendada, pais))
	if i.LimiteDomiciliacion != nil {
		fmt.Fprintf(&b, "Si domicilias el pago, hasta el %s\n", fechaLarga(*i.LimiteDomiciliacion, pais))
	}
	fmt.Fprintf(&b, "Plazo legal: %s\n", fechaLarga(i.Limite, pais))
	if g := i.Guia; g != nil {
		fmt.Fprintf(&b, "\nQué es: %s\n", g.Que)
	}
	var lineas []string
	for _, c := range []struct {
		clave  string
		una    string
		varias string
	}{
		{"banco", "movimiento del banco sin factura", "movimientos del banco sin factura"},
		{"facturas", "factura sin llevar al libro", "facturas sin llevar al libro"},
		{"nominas", "nómina sin llevar al libro", "nóminas sin llevar al libro"},
	} {
		if n := faltan[c.clave]; n > 0 && slices.Contains(i.Preparacion, c.clave) {
			texto := c.varias
			if n == 1 {
				texto = c.una
			}
			lineas = append(lineas, fmt.Sprintf("- %d %s", n, texto))
		}
	}
	if len(lineas) > 0 {
		b.WriteString("\nQué te falta en Sytue ERP:\n" + strings.Join(lineas, "\n") + "\n")
	} else if len(i.Preparacion) > 0 {
		b.WriteString("\nEn Sytue ERP está todo al día para presentarla.\n")
	}
	if g := i.Guia; g != nil && len(g.Pasos) > 0 {
		b.WriteString("\nCómo se hace:\n")
		for n, p := range g.Pasos {
			fmt.Fprintf(&b, "%d. %s\n", n+1, p)
		}
	}
	fmt.Fprintf(&b, "\nÁbrelo en Sytue ERP: %s\n\nRecibes este aviso porque tienes acceso a Obligaciones en el ERP de %s.\n", enlace, organizacion)
	return fmt.Sprintf("Quedan %d días: %s · %s", DiasAntes, i.Nombre, periodo), b.String()
}
