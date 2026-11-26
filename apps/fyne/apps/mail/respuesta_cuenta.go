// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La respuesta automática por cuenta (Sytue Office, M-10)
//
// Con un servicio que cumple api.MailRespuestas (el correo local), la autorespuesta no
// es una del servicio sino una por cuenta, guardada en el servidor de cada cuenta
// (ManageSieve). Solo existe donde el proveedor la ofrece, así que:
//
//   - al abrir los Ajustes se pregunta a cada cuenta que podría tenerla (con un plazo
//     corto: esa pregunta es también la prueba de si la tiene), y la fila «Vacaciones»
//     no aparece hasta que alguna contesta como servidor Sieve; si ninguna, no aparece;
//   - el diálogo es el de siempre con lo que la cuenta añade: qué cuenta, cada cuántos
//     días a la misma persona, las fechas solo si el servidor las admite, y qué pasa con
//     las reglas que la persona ya tenga en el servidor (siguen detrás, o bloquean);
//   - lo que se enseña se lee siempre del servidor: no hay copia que se desincronice.
//
// Con *api.Mail (la edición conectada) nada de esto corre: la página y el diálogo son
// los de siempre (las ramas están en ajustes.go y vacaciones.go).

// autorespuesta es lo que contestó el servidor de una cuenta.
type autorespuesta struct {
	cuenta api.MailAlias
	r      api.MailRespuesta
	err    error
}

// tieneSieve: el servidor contestó como servidor Sieve (aunque luego fallara algo, como
// la contraseña, que el diálogo dice).
func (a autorespuesta) tieneSieve() bool { return !errors.Is(a.err, api.ErrMailSinRespuesta) }

// leerAutorespuestas pregunta a cada cuenta que podría tenerla. Mientras tanto se queda lo
// que se supiera de la vez anterior (la fila no parpadea al volver a los Ajustes).
func (b *Buzon) leerAutorespuestas(rs api.MailRespuestas) {
	b.vueltaAutorespuestas++
	vuelta := b.vueltaAutorespuestas
	cuentas := rs.CuentasRespuesta()
	vivas := map[string]bool{}
	for _, c := range cuentas {
		vivas[c.ID] = true
	}
	// Las cuentas que ya no están (se quitaron) dejan de contar.
	maps.DeleteFunc(b.autorespuestas, func(id string, _ autorespuesta) bool { return !vivas[id] })
	b.pintarAutorespuestas()
	for _, c := range cuentas {
		pedir(b, func(ctx context.Context) (api.MailRespuesta, error) { return rs.LeerRespuesta(ctx, c.ID) },
			func(r api.MailRespuesta, err error) {
				if vuelta != b.vueltaAutorespuestas {
					return
				}
				if b.autorespuestas == nil {
					b.autorespuestas = map[string]autorespuesta{}
				}
				b.autorespuestas[c.ID] = autorespuesta{cuenta: c, r: r, err: err}
				b.pintarAutorespuestas()
			})
	}
}

// cuentasConAutorespuesta son las cuentas cuyo servidor contestó como servidor Sieve, en
// el orden de las cuentas.
func (b *Buzon) cuentasConAutorespuesta() []api.MailAlias {
	rs, ok := b.S.(api.MailRespuestas)
	if !ok {
		return nil
	}
	var out []api.MailAlias
	for _, c := range rs.CuentasRespuesta() {
		if a, ok := b.autorespuestas[c.ID]; ok && a.tieneSieve() {
			out = append(out, c)
		}
	}
	return out
}

// ajustesOcultos son las filas de la página que no se ofrecen: las de AjustesOcultos y,
// con la respuesta por cuenta, «Vacaciones» mientras ninguna cuenta la tenga.
func (b *Buzon) ajustesOcultos() map[string]bool {
	if _, ok := b.S.(api.MailRespuestas); !ok || len(b.cuentasConAutorespuesta()) > 0 {
		return b.AjustesOcultos
	}
	m := maps.Clone(b.AjustesOcultos)
	if m == nil {
		m = map[string]bool{}
	}
	m["vacation"] = true
	return m
}

// pintarAutorespuestas pone la fila con lo que se sabe: si sale y su insignia (la de la
// primera cuenta que responde ahora o, si ninguna, apagada).
func (b *Buzon) pintarAutorespuestas() {
	p := b.P.Ajustes
	if p == nil {
		return
	}
	p.Ocultas = b.ajustesOcultos()
	p.Vacaciones = nil
	var leida *api.MailRespuesta
	for _, c := range b.cuentasConAutorespuesta() {
		a := b.autorespuestas[c.ID]
		if a.err != nil {
			continue
		}
		if leida == nil || (a.r.Activa && !leida.Activa) {
			leida = &a.r
		}
	}
	if leida != nil {
		tono, rotulo, detalle := EstadoVacaciones(b.P.T, b.P.Locale, leida.Activa, diaLocal(leida.Desde), diaLocal(leida.Hasta), time.Now())
		p.Vacaciones = &EstadoAjuste{Tono: tono, Rotulo: rotulo, Detalle: detalle}
	}
	b.P.Refrescar()
}

// ---- El diálogo, por cuenta ----

// Días entre respuestas a la misma persona: los de `cuentas/sieve`.
const (
	diasRespuestaMinimo   = 1
	diasRespuestaMaximo   = 60
	diasRespuestaDeSerie  = 7
	anchoDiasRespuesta    = 96
	separadorAvisoCuentas = " "
)

// montarCuenta arma el formulario con lo que añade la cuenta: los avisos de sus reglas y
// sus fechas, y cada cuántos días a la misma persona. Encima, fuera del cuerpo, qué cuenta.
func (g *dialogoVacaciones) montarCuenta(campo func(string, fyne.CanvasObject, string, float32) *fyne.Container,
	estado, fechas, previo fyne.CanvasObject,
) {
	t := g.b.P.T
	g.nota = container.New(ui.Columna{Hueco: tema.Espacio(3)})
	g.dias = ui.NuevaEntrada(strconv.Itoa(diasRespuestaDeSerie))
	g.dias.Ancho = anchoDiasRespuesta
	g.dias.Rotulo = g.texto("respuestaDias", "")
	g.dias.AlCambiar(func(string) { g.pintar() })
	// Un número de dos cifras: el campo va a su ancho, no a lo ancho del diálogo.
	g.grupoDias = campo(g.texto("respuestaDias", ""), container.New(ui.Fila{}, g.dias), g.texto("respuestaDiasPista", ""), g.ancho)
	g.cajaFechas, _ = fechas.(*fyne.Container)
	g.form = container.New(ui.Columna{Hueco: tema.Espacio(5)},
		ui.NuevaAyudaEnLinea(t.T("vacation.explainBody", nil)+separadorAvisoCuentas+g.texto("respuestaServidor", "")).
			ConTono(ui.AyudaInfo).ConTitulo(t.T("vacation.explainTitle", nil)),
		estado,
		g.nota,
		campo(t.T("common.subject", nil), g.asunto, t.T("vacation.subjectHint", nil), g.ancho),
		campo(t.T("common.message", nil), g.cuerpo, t.T("vacation.bodyHint", nil), g.ancho),
		g.grupoDias,
		fechas,
		previo)

	// Qué cuenta: un selector si hay varias, y si no, su dirección.
	var control fyne.CanvasObject
	if len(g.cuentas) > 1 {
		nombres := make([]string, len(g.cuentas))
		for i, c := range g.cuentas {
			nombres[i] = c.Address
		}
		g.selCuenta = ui.NuevoSelector(nombres, func(i int) {
			if i >= 0 && i < len(g.cuentas) && g.cuentas[i].ID != g.cuenta {
				g.cuenta = g.cuentas[i].ID
				g.cargar()
			}
		})
		g.selCuenta.Elegida = 0
		g.selCuenta.Rotulo = t.T("externalMail.account", nil)
		control = g.selCuenta
	} else {
		control = ui.NuevoRotulo(g.cuentas[0].Address, tema.TextoSM, ui.Normal, "text")
	}
	g.grupoCuenta = ui.GrupoCampo(t.T("externalMail.account", nil), false, control, "", "")
}

// texto es un texto propio de la respuesta por cuenta en el idioma de la pantalla, con
// {nombre} sustituido.
func (g *dialogoVacaciones) texto(clave, nombre string) string {
	s := i18n.Propios{Base: TextosRespuesta, Traducciones: traduccionesRespuesta}.En(g.b.P.Locale, clave)
	return strings.ReplaceAll(s, "{nombre}", nombre)
}

// cargarCuenta lee del servidor de la cuenta elegida y pone el formulario.
func (g *dialogoVacaciones) cargarCuenta() {
	cuenta := g.cuenta
	pedir(g.b, func(ctx context.Context) (api.MailRespuesta, error) { return g.porCuenta.LeerRespuesta(ctx, cuenta) },
		func(r api.MailRespuesta, err error) {
			if cuenta != g.cuenta {
				return // se eligió otra mientras tanto
			}
			if err != nil {
				g.estado = Error
				g.pintar()
				return
			}
			g.actual = r
			g.conmutador.Encendido = r.Activa
			// Bloqueada y apagada no se puede encender: las reglas de la persona mandan.
			if r.Bloqueada != "" && !r.Activa {
				g.conmutador.Disable()
			} else {
				g.conmutador.Enable()
			}
			g.asunto.PonerTexto(r.Asunto)
			g.cuerpo.PonerTexto(r.Mensaje)
			g.desde.PonerTexto("")
			g.hasta.PonerTexto("")
			if r.Desde != nil {
				g.desde.PonerTexto(ui.EscribirFecha(r.Desde.Local()))
			}
			if r.Hasta != nil {
				g.hasta.PonerTexto(ui.EscribirFecha(r.Hasta.Local()))
			}
			dias := r.Dias
			if dias <= 0 {
				dias = diasRespuestaDeSerie
			}
			g.dias.PonerTexto(strconv.Itoa(dias))
			g.estado = Listo
			g.pintar()
		})
}

// diasValidos lee los días: vacío, los de serie; fuera de margen o sin ser un número, no.
func (g *dialogoVacaciones) diasValidos() (int, bool) {
	if g.dias == nil {
		return 0, false
	}
	s := strings.TrimSpace(g.dias.Texto())
	if s == "" {
		return diasRespuestaDeSerie, true
	}
	n, err := strconv.Atoi(s)
	return n, err == nil && n >= diasRespuestaMinimo && n <= diasRespuestaMaximo
}

// noSeGuarda: con la respuesta por cuenta, unos días que no valen, o encenderla con las
// reglas de la persona bloqueándola. Sin ella, nunca.
func (g *dialogoVacaciones) noSeGuarda() bool {
	if g.porCuenta == nil {
		return false
	}
	if _, ok := g.diasValidos(); !ok {
		return true
	}
	return g.actual.Bloqueada != "" && g.conmutador.Encendido && !g.actual.Activa
}

// repintarCuenta pone los avisos de la cuenta, la pista de los días y las fechas.
func (g *dialogoVacaciones) repintarCuenta() {
	var notas []fyne.CanvasObject
	switch {
	case g.actual.Bloqueada != "" && !g.actual.Activa:
		notas = append(notas, ui.NuevaAyudaEnLinea(g.texto("respuestaBloqueada", g.actual.Bloqueada)).ConTono(ui.AyudaAviso))
	case g.actual.Reglas != "":
		notas = append(notas, ui.NuevaAyudaEnLinea(g.texto("respuestaReglas", g.actual.Reglas)).ConTono(ui.AyudaNeutra))
	}
	if !g.actual.ConFechas {
		notas = append(notas, ui.NuevaAyudaEnLinea(g.texto("respuestaSinFechas", "")).ConTono(ui.AyudaNeutra))
	}
	g.nota.Objects = notas
	g.nota.Refresh()
	if len(notas) == 0 {
		g.nota.Hide()
	} else {
		g.nota.Show()
	}
	if g.cajaFechas != nil {
		if g.actual.ConFechas {
			g.cajaFechas.Show()
		} else {
			g.cajaFechas.Hide()
		}
	}
	_, ok := g.diasValidos()
	tono := ui.AyudaNeutra
	if !ok {
		tono = ui.AyudaPeligro
	}
	ponerPista(g.grupoDias, g.texto("respuestaDiasPista", ""), tono, g.ancho)
	if g.dias.Invalida != !ok {
		g.dias.Invalida = !ok
		g.dias.Refresh()
	}
}

// salvarCuenta guarda en el servidor de la cuenta. Sin asunto, el que promete la pista.
func (g *dialogoVacaciones) salvarCuenta() {
	if g.estado != Listo || g.alReves() || g.noSeGuarda() || g.guardar.Cargando {
		return
	}
	desde, hasta, ok := g.fechas()
	if !ok {
		g.b.avisar(ui.AvisoPeligro, "vacation.saveFailed", nil)
		return
	}
	dias, _ := g.diasValidos()
	asunto := strings.TrimSpace(g.asunto.Texto())
	if asunto == "" {
		asunto = g.b.P.T.T("vacation.subjectPlaceholder", nil)
	}
	r := api.MailRespuesta{Cuenta: g.cuenta, Activa: g.conmutador.Encendido, Asunto: asunto, Mensaje: g.cuerpo.Texto(), Dias: dias}
	if g.actual.ConFechas {
		r.Desde, r.Hasta = PeriodoVacaciones(desde, hasta)
	}
	cuenta := g.cuenta
	g.guardar.Cargando, g.cancelar.Inerte = true, true
	g.guardar.Refresh()
	g.cancelar.Refresh()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.porCuenta.GuardarRespuesta(ctx, cuenta, r)
	}, func(_ struct{}, err error) {
		g.guardar.Cargando, g.cancelar.Inerte = false, false
		g.guardar.Refresh()
		g.cancelar.Refresh()
		if err != nil {
			g.b.avisar(ui.AvisoPeligro, "vacation.saveFailed", nil)
			return
		}
		g.b.avisar(ui.AvisoExito, "vacation.saved", nil)
		g.d.Cerrar()
		g.b.leerEstadoAjustes()
	})
}

// Los textos propios de la respuesta por cuenta (§5.14): español e inglés aquí, los
// otros veinticuatro idiomas en respuesta_cuenta_idiomas.go. {nombre} es el script de la
// persona en su servidor.
var TextosRespuesta = map[string][2]string{ // clave: {es, en}
	"respuestaDias":      {"Días entre respuestas a la misma persona", "Days between replies to the same person"},
	"respuestaDiasPista": {"De 1 a 60. A quien te escriba varias veces se le responde una sola vez en ese plazo.", "From 1 to 60. Someone who writes to you several times gets the reply only once in that period."},
	"respuestaSinFechas": {"Tu servidor de correo no admite fechas en la respuesta automática: responde mientras esté activada.", "Your mail server does not support dates in the auto-reply: it replies for as long as it is on."},
	"respuestaBloqueada": {"Tu servidor de correo ya tiene activas otras reglas («{nombre}») que no se pueden combinar con una respuesta automática sin cambiarlas. Para no romperlas, no se activa desde aquí: usa la web de tu proveedor.", "Your mail server already has other rules turned on (“{nombre}”) that cannot be combined with an auto-reply without changing them. To avoid breaking them, it is not turned on from here: use your provider’s website."},
	"respuestaReglas":    {"Tus reglas del servidor («{nombre}») siguen funcionando detrás de la respuesta y, al desactivarla, quedan exactamente como estaban.", "Your server rules (“{nombre}”) keep working after the reply, and when you turn it off they are left exactly as they were."},
	"respuestaServidor":  {"Se guarda en tu servidor de correo: responde aunque este equipo esté apagado.", "It is saved on your mail server, so it replies even when this device is off."},
}
