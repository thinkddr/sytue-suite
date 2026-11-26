// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// # Enviar la factura por correo con el PDF adjunto
//
// Las dos rutas de services/syerp (`internal/http/enviar_factura.go`) con la misma forma
// y los mismos errores, pero el correo no sale de un buzón del servicio: sale por una
// CUENTA DE CORREO de Sytue Mail de este equipo (`nucleo/local/correo`, el Remitente),
// con su SMTP y por el portero de red, y la persona elige cuál. Sin ninguna cuenta,
// `disponible` es falso y enviar contesta 503 `correo_no_configurado`, como un
// despliegue sin servidor de correo: la pantalla ofrece entonces el PDF y el programa
// de correo del sistema. Lo que sale queda apuntado en el historial de la factura con
// el mismo texto que el servicio (`historial.enviada_por_correo`).
//
//	GET  /factura_venta/{id}/enviar   para, enviable, disponible y, solo aquí, las cuentas
//	                                  y el asunto y el texto propuestos (número, importe y
//	                                  vencimiento)
//	POST /factura_venta/{id}/enviar   {para, asunto, mensaje, cuenta} → {enviada: true}

// Correo es con qué sale la factura: las cuentas de correo de este equipo y su envío.
// Lo cumple `*correo.Remitente`; las pruebas ponen uno propio.
type Correo interface {
	Cuentas() ([]correo.Cuenta, error)
	Enviar(ctx context.Context, cuenta string, para []string, crudo []byte) error
}

// PonerCorreo enchufa el envío por correo; nil lo quita (enviar contesta 503).
func (s *Servidor) PonerCorreo(c Correo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.envios = c
}

func (s *Servidor) correo() Correo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.envios
}

// Topes del correo que escribe la persona, los del servicio.
const (
	maxAsuntoEnvio  = 300
	maxMensajeEnvio = 10_000
)

// Los códigos que solo contesta el ERP local (la pantalla los dice con su texto,
// apps/fyne/apps/erp.ErroresLocales): por qué no salió el correo, cuando se sabe.
const (
	CodigoCuentaCorreo   = "cuenta_correo_desconocida"
	CodigoCorreoAcceso   = "correo_acceso"
	CodigoCorreoSinRed   = "correo_sin_red"
	CodigoCorreoIncierto = "correo_incierto"
)

// facturaEnviable: el borrador aún no es una factura y la anulada ya no lo es.
func facturaEnviable(reg api.ERPRegistro) bool {
	return reg.Estado != "borrador" && reg.Estado != "anulado"
}

// cuentasDeEnvio son las cuentas de este equipo; sin Correo, ninguna.
func (s *Servidor) cuentasDeEnvio() ([]correo.Cuenta, error) {
	c := s.correo()
	if c == nil {
		return nil, nil
	}
	return c.Cuentas()
}

// correoDelCliente es la dirección de facturación del cliente, o su correo general.
func (s *Servidor) correoDelCliente(ctx context.Context, reg api.ERPRegistro) string {
	id, _ := reg.Data["cliente"].(string)
	if id == "" {
		return ""
	}
	c, err := s.alm.Obtener(ctx, "contacto", id)
	if err != nil {
		return ""
	}
	texto := func(k string) string { v, _ := c.Data[k].(string); return strings.TrimSpace(v) }
	return cmp.Or(texto("email_facturas"), texto("correo"))
}

// envioFactura — GET /{modelo}/{id}/enviar — con qué arranca el diálogo.
func (s *Servidor) envioFactura(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if m.Clave != "factura_venta" {
		noContable(w, "Only customer invoices can be sent.")
		return
	}
	ctx := r.Context()
	reg, err := s.alm.Obtener(ctx, m.Clave, r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	cs, err := s.cuentasDeEnvio()
	if err != nil {
		falloDe(w, err)
		return
	}
	out := api.ERPEnvioFactura{
		Para: s.correoDelCliente(ctx, reg), Enviable: facturaEnviable(reg), Disponible: len(cs) > 0,
		PorCuenta: true, Cuentas: []api.ERPCuentaCorreo{},
	}
	for _, c := range cs {
		out.Cuentas = append(out.Cuentas, api.ERPCuentaCorreo{ID: c.ID, Direccion: c.Direccion, Nombre: cmp.Or(c.Rotulo, c.Nombre)})
	}
	emisor := ""
	if o, err := s.alm.Organizacion(ctx); err == nil {
		emisor = o.Nombre
	}
	out.Asunto, out.Mensaje = PlantillaEnvio(idioma(r), DatosEnvio{
		Numero: reg.Ref, Importe: efactura.Euros(totalDe(reg)),
		Vence: fechaImpresa(reg.Data["vencimiento"]), Emisor: emisor,
	})
	responder(w, http.StatusOK, out)
}

// totalDe es el total de la factura: el suyo o, sin él, la base más el IVA.
func totalDe(reg api.ERPRegistro) int64 {
	if t, err := dominio.Centimos(reg.Data, "total"); err == nil && t != 0 {
		return t
	}
	base, _ := dominio.Centimos(reg.Data, "base")
	iva, _ := dominio.Centimos(reg.Data, "iva")
	return base + iva
}

// fechaImpresa es la fecha como la escribe el impreso (dd/mm/aaaa); vacía si no hay.
func fechaImpresa(v any) string {
	s, _ := v.(string)
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return t.Format("02/01/2006")
}

// enviarFactura — POST /{modelo}/{id}/enviar — manda la factura en PDF a UNA dirección
// por la cuenta elegida y lo apunta en el historial del documento.
func (s *Servidor) enviarFactura(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if m.Clave != "factura_venta" {
		noContable(w, "Only customer invoices can be sent.")
		return
	}
	var c api.ERPCorreoFactura
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	para, asunto := strings.TrimSpace(c.Para), strings.TrimSpace(c.Asunto)
	switch {
	case !validation.IsEmail(para):
		fallo(w, http.StatusUnprocessableEntity, "destinatario_invalido", "The recipient is not a valid email address.", nil)
		return
	case asunto == "" || utf8.RuneCountInString(asunto) > maxAsuntoEnvio:
		fallo(w, http.StatusBadRequest, "bad_request", "The subject is empty or too long.", nil)
		return
	case utf8.RuneCountInString(c.Mensaje) > maxMensajeEnvio:
		fallo(w, http.StatusBadRequest, "bad_request", "The message is too long.", nil)
		return
	}
	envios := s.correo()
	cs, err := s.cuentasDeEnvio()
	if err != nil {
		falloDe(w, err)
		return
	}
	if envios == nil || len(cs) == 0 {
		fallo(w, http.StatusServiceUnavailable, "correo_no_configurado", "There is no email account in Sytue Mail on this device.", nil)
		return
	}
	cuenta := cs[0]
	if c.Cuenta != "" {
		i := slices.IndexFunc(cs, func(x correo.Cuenta) bool { return x.ID == c.Cuenta })
		if i < 0 {
			fallo(w, http.StatusUnprocessableEntity, CodigoCuentaCorreo, "That email account is no longer in Sytue Mail.", nil)
			return
		}
		cuenta = cs[i]
	}
	f, reg, ok := s.impresoDe(w, r, m)
	if !ok {
		return
	}
	if !facturaEnviable(reg) {
		fallo(w, http.StatusUnprocessableEntity, "factura_no_enviable", "Only issued invoices can be sent: drafts and cancelled invoices cannot.", nil)
		return
	}
	pdf, err := f.PDF()
	if err != nil {
		falloDe(w, err)
		return
	}
	crudo, err := eml.Componer(eml.Salida{
		De: cuenta.Direccion, DeNombre: cmp.Or(strings.TrimSpace(f.Emisor.Nombre), cuenta.Nombre),
		Para: []string{para}, Asunto: asunto, Texto: c.Mensaje,
		Adjuntos: []eml.Adjunto{{Nombre: f.NombrePDF(), Tipo: "application/pdf", Datos: pdf}},
	}, time.Now())
	if err != nil {
		falloDe(w, err)
		return
	}
	ctx := r.Context()
	if err := envios.Enviar(ctx, cuenta.ID, []string{para}, crudo); err != nil {
		fallo(w, http.StatusBadGateway, codigoDeEnvio(err), err.Error(), nil)
		return
	}
	// En el historial, en el idioma de quien la manda, como el servicio. El correo YA
	// salió: si no se apunta, decir que falló invitaría a mandarlo otra vez.
	nota := strings.ReplaceAll(s.cat.Rotulo("historial.enviada_por_correo", idioma(r)), "{para}", para)
	_ = s.alm.Anotar(ctx, m.Clave, reg.ID, nota)
	responder(w, http.StatusOK, map[string]any{"enviada": true})
}

// codigoDeEnvio dice por qué no salió: la cuenta no deja entrar, no hay red (o el
// interruptor «Sin conexión»), la conexión se cortó a mitad (puede haber salido), o el
// servidor lo rechazó (el `correo_no_enviado` del servicio).
func codigoDeEnvio(err error) string {
	switch {
	case errors.Is(err, correo.ErrSinCuenta):
		return CodigoCuentaCorreo
	case errors.Is(err, correo.ErrAcceso):
		return CodigoCorreoAcceso
	case errors.Is(err, correo.ErrSinRed):
		return CodigoCorreoSinRed
	case errors.Is(err, correo.ErrEnvioIncierto):
		return CodigoCorreoIncierto
	}
	return "correo_no_enviado"
}
