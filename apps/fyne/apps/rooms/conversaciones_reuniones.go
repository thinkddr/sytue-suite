// SPDX-License-Identifier: Apache-2.0
package rooms

import (
	"context"
	"errors"
	"slices"

	"fyne.io/fyne/v2"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func (c *conversacionExterna) vigenteMeet() bool { return !c.cerrado && c.ambitoMeet()() }
func (c *conversacionExterna) ambitoMeet() func() bool {
	ctx, generation, current := c.ctx, c.generacionMeet, c.sa.GeneracionSesion
	return func() bool {
		return (ctx == nil || ctx.Err() == nil) && (current == nil || generation != "" && current() == generation)
	}
}

func (c *conversacionExterna) fuenteMeet() string {
	if c.proveedorMeet == "microsoft-teams" {
		return c.proveedorMeet
	}
	return "google-meet"
}

func (c *conversacionExterna) falloMeet() string {
	if c.fuenteMeet() == "microsoft-teams" {
		return "appConnection.failed"
	}
	return "external.meet.failed"
}

func (c *conversacionExterna) nombreMeet() string {
	if c.fuenteMeet() == "microsoft-teams" {
		return "Microsoft Teams"
	}
	return "Google Meet"
}

func (c *conversacionExterna) cambiarProveedorMeet(p string) {
	if p != c.fuenteMeet() {
		c.proveedorMeet = p
		c.cuentaMeet, c.enlaceMeet, c.envioMeet, c.errorMeet, c.esperadaMeet, c.pendienteMeet = "", "", "", "", "", ""
		c.compartidoMeet = false
		c.reconciliarMeet()
	}
}

// Both meeting providers reuse personal consent and the exact account vault.
func (c *conversacionExterna) bloqueMeet() []fyne.CanvasObject {
	if len(c.cuentas.Meetings) == 0 {
		return nil
	}
	t := c.sa.P.T
	hijos := []fyne.CanvasObject{ui.NuevoRotulo(t.T("external.meet.title", nil), tema.TextoBase, ui.SemiNegrita, "text")}
	for _, p := range []struct{ id, name string }{{"google-meet", "Google Meet"}, {"microsoft-teams", "Microsoft Teams"}} {
		tarjeta := ui.NuevaTarjetaEleccion(p.name, "", func() { c.cambiarProveedorMeet(p.id); c.pintar() })
		tarjeta.Elegida, tarjeta.Inerte = c.fuenteMeet() == p.id, c.ocupado || c.cargando
		hijos = append(hijos, tarjeta)
	}
	cuerpo, crearClave := "external.meet.body", "external.meet.create"
	if c.fuenteMeet() == "microsoft-teams" {
		cuerpo, crearClave = "external.teamsMeeting.body", "external.teamsMeeting.create"
	}
	hijos = append(hijos, ui.NuevoRotulo(t.T("link.joinMeeting", i18n.Vars{"provider": c.nombreMeet()}), tema.TextoBase, ui.SemiNegrita, "text"), c.texto(t.T(cuerpo, nil)))
	tarjetas, registrado, hayCuentas := c.tarjetasMeet()
	hijos = append(hijos, tarjetas...)
	hijos = append(hijos, c.accionesMeet(registrado, hayCuentas, crearClave)...)
	if !registrado {
		hijos = append(hijos, c.texto(t.T("external.unavailable", i18n.Vars{"provider": c.nombreMeet()})))
	}
	if c.pendienteMeet != "" {
		hijos = append(hijos, c.boton("action.retry", func() { c.conectarMeet(c.pendienteMeet) }))
	} else if c.esperadaMeet != "" {
		hijos = append(hijos, c.boton("action.retry", func() { c.cargarCuentas("") }))
	}
	if c.enlaceMeet != "" {
		hijos = append(hijos, c.enlaceProtegido()...)
	}
	if c.errorMeet != "" {
		hijos = append(hijos, c.texto(t.T(c.errorMeet, nil)))
	}
	return hijos
}

func (c *conversacionExterna) ampliaChatMeet(account *api.ExternalMeeting) bool {
	return account != nil && !account.Connected && c.fuenteMeet() == "microsoft-teams" && slices.ContainsFunc(c.cuentas.Items, func(a api.ExternalChatAccount) bool { return a.InstanceID == account.InstanceID && a.Connected })
}

func (c *conversacionExterna) accionesMeet(registrado, hayCuentas bool, crearClave string) []fyne.CanvasObject {
	t := c.sa.P.T
	hijos := []fyne.CanvasObject{}
	elegida := c.meetElegida()
	if hayCuentas {
		if c.ampliaChatMeet(elegida) {
			hijos = append(hijos, c.texto(t.T("external.teamsMeeting.extend", nil)))
		}
		crear := c.boton(crearClave, c.crearMeet)
		crear.Inerte = crear.Inerte || elegida == nil || !elegida.Connected || elegida.DisabledByOrg || c.inciertasMeet[c.cuentaMeet] || c.esperadaMeet != ""
		hijos = append(hijos, crear)
		if elegida != nil && !elegida.Connected {
			autorizar := c.boton("appConnection.authorize", func() { c.conectarMeet(c.cuentaMeet) })
			autorizar.Inerte = autorizar.Inerte || !registrado || elegida.DisabledByOrg
			hijos = append(hijos, autorizar)
		}
	}
	clave := "external.meet.connect"
	if c.fuenteMeet() == "microsoft-teams" {
		clave = "appConnection.authorize"
	}
	if hayCuentas {
		clave = "external.addAccount"
	}
	conectar := c.boton(clave, func() { c.pendienteMeet, c.esperadaMeet = "", ""; c.conectarMeet("") })
	conectar.Inerte = conectar.Inerte || !registrado || (c.sa.AutorizarReunion == nil && (c.fuenteMeet() != "google-meet" || c.sa.AutorizarConversaciones == nil))
	hijos = append(hijos, conectar)
	return hijos
}

func (c *conversacionExterna) tarjetasMeet() ([]fyne.CanvasObject, bool, bool) {
	registrado, hay := false, false
	var hijos []fyne.CanvasObject
	for _, meet := range c.cuentas.Meetings {
		if meet.Provider != c.fuenteMeet() {
			continue
		}
		registrado = registrado || meet.Available
		if meet.InstanceID == "" || (!meet.Connected && c.fuenteMeet() != "microsoft-teams") {
			continue
		}
		hay = true
		nombre := meet.Name
		if nombre == "" {
			nombre = c.nombreMeet()
		}
		tarjeta := ui.NuevaTarjetaEleccion(nombre, "", func() { c.elegirMeet(meet.InstanceID); c.pintar() })
		tarjeta.Elegida, tarjeta.Inerte, tarjeta.Ancho = c.cuentaMeet == meet.InstanceID, c.ocupado || c.cargando, max(180, min(520, c.lienzo.Size().Width-48))
		hijos = append(hijos, tarjeta)
	}
	return hijos, registrado, hay
}

func (c *conversacionExterna) meetElegida() *api.ExternalMeeting {
	for idx := range c.cuentas.Meetings {
		meet := &c.cuentas.Meetings[idx]
		if meet.Provider == c.fuenteMeet() && (meet.Connected || c.fuenteMeet() == "microsoft-teams") && meet.InstanceID != "" && meet.InstanceID == c.cuentaMeet {
			return meet
		}
	}
	return nil
}

func (c *conversacionExterna) elegirMeet(id string) {
	anterior := c.cuentaMeet
	c.cuentaMeet = id
	if c.meetElegida() == nil {
		c.cuentaMeet = anterior
		return
	}
	if anterior != id {
		c.enlaceMeet, c.envioMeet = "", ""
		c.compartidoMeet = false
	}
	c.esperadaMeet, c.pendienteMeet, c.errorMeet = "", "", ""
}

func (c *conversacionExterna) reconciliarMeet() {
	if c.esperadaMeet != "" {
		id := c.esperadaMeet
		for _, meet := range c.cuentas.Meetings {
			if meet.InstanceID == id && meet.Provider == c.fuenteMeet() && meet.Connected {
				c.elegirMeet(id)
				c.esperadaMeet, c.errorMeet = "", ""
				return
			}
		}
		c.errorMeet = c.falloMeet()
		return
	}
	if c.cuentaMeet != "" {
		if c.meetElegida() == nil {
			c.cuentaMeet, c.enlaceMeet, c.envioMeet = "", "", ""
		}
		return
	}
	c.enlaceMeet, c.envioMeet = "", ""
	activos, unica := 0, ""
	for _, meet := range c.cuentas.Meetings {
		if meet.Provider == c.fuenteMeet() && meet.Connected && meet.InstanceID != "" {
			activos++
			unica = meet.InstanceID
		}
	}
	if activos == 1 {
		c.cuentaMeet = unica
	}
}

func (c *conversacionExterna) crearMeet() {
	elegida := c.meetElegida()
	if !c.vigenteMeet() || c.ocupado || elegida == nil || !elegida.Connected || c.esperadaMeet != "" || c.inciertasMeet[c.cuentaMeet] {
		return
	}
	c.ocupado, c.errorMeet = true, ""
	c.enlaceMeet, c.envioMeet = "", ""
	c.compartidoMeet = false
	instancia, cuenta, proveedor := c.cuentaMeet, elegida.AccountID, c.fuenteMeet()
	guard := c.ambitoMeet()
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (string, error) {
		if !guard() {
			return "", context.Canceled
		}
		if proveedor == "microsoft-teams" {
			r, err := c.sa.CuentasApps.ProtectedTeamsMeeting(ctx, instancia, cuenta, "")
			return r.URL, err
		}
		return c.sa.CuentasApps.ExternalMeetingCreate(ctx, instancia)
	}, func(u string, err error) {
		if !c.vigenteMeet() {
			return
		}
		c.ocupado = false
		if c.cuentaMeet != instancia || c.fuenteMeet() != proveedor {
			return
		}
		if err != nil {
			c.errorMeet = c.falloMeet()
			if proveedor == "microsoft-teams" {
				c.errorMeet = c.errorReunionTeams(err, instancia)
			}
		} else {
			c.enlaceMeet = u
		}
		c.pintar()
	})
}

func (c *conversacionExterna) errorReunionTeams(err error, instancia string) string {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code() == "external_meeting_unprotected" {
			return "external.teamsMeeting.unprotected"
		}
		if apiErr.Code() != "external_meeting_unknown" && apiErr.Code() != "" {
			return c.falloMeet()
		}
	}
	if c.inciertasMeet == nil {
		c.inciertasMeet = map[string]bool{}
	}
	c.inciertasMeet[instancia] = true
	return "external.teamsMeeting.unknown"
}

func (c *conversacionExterna) conectarMeet(instancia string) {
	autorizar := c.sa.AutorizarReunion
	if autorizar == nil && c.fuenteMeet() == "google-meet" {
		autorizar = c.sa.AutorizarConversaciones
	}
	if !c.vigenteMeet() || c.ocupado || autorizar == nil {
		return
	}
	c.ocupado, c.errorMeet = true, ""
	proveedor := c.fuenteMeet()
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (string, error) { return autorizar(ctx, proveedor, instancia) }, func(id string, err error) {
		if !c.vigenteMeet() {
			return
		}
		c.ocupado = false
		if c.fuenteMeet() != proveedor {
			return
		}
		if err != nil || id == "" {
			if id != "" {
				c.pendienteMeet = id
			}
			c.errorMeet = c.falloMeet()
			c.pintar()
			return
		}
		c.esperadaMeet, c.pendienteMeet = id, ""
		c.cargarCuentas("")
	})
}

func (c *conversacionExterna) enlaceProtegido() []fyne.CanvasObject {
	t := c.sa.P.T
	enlace := c.enlaceMeet
	hijos := []fyne.CanvasObject{c.texto(enlace)}
	if c.fuenteMeet() == "microsoft-teams" {
		hijos = append(hijos, c.texto(t.T("external.teamsMeeting.verified", nil)))
	}
	hijos = append(hijos, c.boton("action.copy", func() { fyne.CurrentApp().Clipboard().SetContent(enlace) }), c.boton("action.open", func() {
		var err error
		if c.fuenteMeet() == "microsoft-teams" {
			err = api.AbrirReunionTeams(fyne.CurrentApp(), enlace)
		} else {
			err = api.AbrirFuera(fyne.CurrentApp(), enlace)
		}
		if err != nil {
			c.errorMeet = c.falloMeet()
			c.pintar()
		}
	}))
	if _, ok := c.sa.S.(emisorReunionCifrada); ok {
		hijos = append(hijos, c.compartirMeetControles()...)
	}
	return hijos
}

func (c *conversacionExterna) compartirMeetControles() []fyne.CanvasObject {
	t := c.sa.P.T
	ids := []string{}
	nombres := []string{t.T("appConnection.chooseChannel", nil)}
	for _, canal := range c.sa.canales {
		if canal.Kind != "direct" && !canal.Archived {
			ids = append(ids, canal.ID)
			nombres = append(nombres, "#"+canal.Name)
		}
	}
	sel := ui.NuevoSelector(nombres, func(i int) {
		c.canalMeet = ""
		if i > 0 && i <= len(ids) {
			c.canalMeet = ids[i-1]
		}
		c.envioMeet = ""
		c.compartidoMeet = false
		c.pintar()
	})
	sel.Rotulo = t.T("appConnection.roomsChannel", nil)
	for i, id := range ids {
		if id == c.canalMeet {
			sel.Elegida = i + 1
		}
	}
	clave := "share.title"
	if c.compartidoMeet {
		clave = "action.done"
	}
	enviar := c.boton(clave, c.compartirMeet)
	enviar.Inerte = enviar.Inerte || c.canalMeet == "" || c.compartidoMeet
	return []fyne.CanvasObject{sel, enviar}
}

func (c *conversacionExterna) compartirMeet() {
	emisor, ok := c.sa.S.(emisorReunionCifrada)
	if !c.vigenteMeet() || !ok || c.ocupado || c.enlaceMeet == "" || c.canalMeet == "" || c.compartidoMeet {
		return
	}
	if c.envioMeet == "" {
		c.envioMeet = uuid.NewString()
	}
	guard := c.ambitoMeet()
	canal, enlace, txn := c.canalMeet, c.enlaceMeet, c.envioMeet
	c.ocupado, c.errorMeet = true, ""
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (api.RoomsMessage, error) {
		if !guard() {
			return api.RoomsMessage{}, context.Canceled
		}
		return emisor.PostWithTxn(ctx, canal, enlace, txn)
	}, func(_ api.RoomsMessage, err error) {
		if !c.vigenteMeet() {
			return
		}
		c.ocupado = false
		if c.envioMeet == txn {
			if err != nil {
				c.errorMeet = "share.failed"
			} else {
				c.compartidoMeet = true
			}
		}
		c.pintar()
	})
}

// emisorReunionCifrada publica el enlace de la reunión por el camino cifrado de
// Matrix, con una transacción que hace idempotente el reintento.
type emisorReunionCifrada interface {
	PostWithTxn(context.Context, string, string, string) (api.RoomsMessage, error)
}
