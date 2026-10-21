// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// Los avisos de la tienda salen del outbox (§10.5): hasta el 2026-10-16 se escribían y
// nadie los despachaba, así que no llegaba ni la confirmación de un pedido. Despachar los
// entrega con el Dispatcher de platform/pkg/events, el de los demás servicios: el correo
// por el relé (platform/pkg/correo, con el nombre de la tienda y respondiendo a ella),
// los avisos de la organización por el Hub, y los pedidos pagados a SyERP (contabilidad
// y facturación con Veri*Factu, §11).
var tiposAviso = []string{
	"order.paid.email_buyer", "order.paid.email_store", "order.paid.hub",
	"pages.order.oversold", "pages.payment.duplicated", "pages.order.withdrawal",
	"pages.form.submitted", "order.paid.erp",
}

// corteAvisos es la edad a partir de la cual un aviso ya no sale: se da por entregado
// sin mandarlo. Lo que se acumuló mientras no había despachador (y lo que se quede
// atascado si el relé cae) no debe llegar de golpe días después: una confirmación de
// pedido o un «nuevo mensaje del formulario» de hace una semana confunde más que
// informa, y la tienda ya ve sus pedidos en la app. Dos días cubren una caída de un fin
// de semana del relé.
//
// El desistimiento no tiene corte: el acuse al comprador es una obligación (art. 106.5
// LGDCU, en soporte duradero) y la tienda tiene 14 días para devolver el dinero, así que
// sale mientras el aviso exista (la purga lo quita a los 30 días).
const corteAvisos = 48 * time.Hour

// Despachar entrega los avisos del outbox hasta que se cancele ctx. Sin relé (sin
// SMTP_FROM) los correos fallan y se reintentan; los del Hub salen igual.
func (s *Servicio) Despachar(ctx context.Context, c correo.Config) error {
	if s.enviarCorreo == nil {
		s.enviarCorreo = func(ctx context.Context, op correo.Opciones, para, asunto, cuerpo string) error {
			return correo.EnviarCon(ctx, c, op, para, asunto, cuerpo)
		}
	}
	return events.NewDispatcher(s.store.DB(), s.entregarAviso, events.DispatcherConfig{
		Channels: []events.Channel{"pages"},
		Types:    tiposAviso,
	}).Run(ctx)
}

// avisoTienda es la carga de cualquiera de los avisos: solo identificadores e importes
// (WEB-10). Lo demás se lee ahora, con la RLS de la organización.
type avisoTienda struct {
	OrderID      uuid.UUID `json:"order_id"`
	SiteID       uuid.UUID `json:"site_id"`
	FormID       string    `json:"form_id"`
	SubmissionID uuid.UUID `json:"submission_id"`
	WithdrawalID uuid.UUID `json:"withdrawal_id"`
}

// entregarAviso es el Handler. Un error hace que el Dispatcher lo reintente; uno que no
// se arregla reintentando (el pedido ya no existe) se da por entregado.
//
// ponytail: un aviso a varias personas se reintenta entero, así que si falla el correo de
// la segunda la primera lo recibe dos veces. Un registro por destinatario si molesta.
func (s *Servicio) entregarAviso(ctx context.Context, ev events.Event) error {
	if ev.Type != "pages.order.withdrawal" && ev.Type != "order.paid.erp" && time.Since(ev.CreatedAt) > corteAvisos {
		log.From(ctx).Info("pages: aviso fuera de plazo, no se manda", "tipo", ev.Type, "id", ev.ID, "creado", ev.CreatedAt)
		return nil
	}
	var a avisoTienda
	if err := json.Unmarshal(ev.Payload, &a); err != nil {
		return nil //nolint:nilerr // una carga que no se lee no se arregla reintentando.
	}
	err := s.avisar(ctx, ev, a)
	if errors.Is(err, domain.ErrPedidoNoExiste) || errors.Is(err, domain.ErrNoExiste) || errors.Is(err, domain.ErrFormularioNoExiste) {
		return nil
	}
	return err
}

func (s *Servicio) avisar(ctx context.Context, ev events.Event, a avisoTienda) error {
	tenant := ev.Tenant
	sitio, err := s.store.ObtenerSitioPublico(ctx, tenant, a.SiteID)
	if err != nil {
		return err
	}
	ajustes, err := s.store.ObtenerAjustesTienda(ctx, tenant, a.SiteID)
	if err != nil {
		return err
	}
	op := correo.Opciones{NombreRemitente: sitio.Name + " (vía Sytue Web)", ResponderA: ajustes.ReplyTo}
	hub := func(tipo, titulo string, params map[string]string, ruta string) error {
		params["site"] = sitio.Name
		var errs []error
		for _, p := range ajustes.Notify {
			if p.Sub == "" {
				continue
			}
			errs = append(errs, s.hub.Send(ctx, hubnotify.Notification{
				Tenant: tenant, Subject: p.Sub, Source: "pages", Type: tipo, Title: titulo, Params: params,
				URL: hubnotify.EnlacePages(s.publicURL, tenant, ruta),
			}))
		}
		return errors.Join(errs...)
	}

	switch ev.Type {
	case "pages.order.withdrawal":
		return s.avisarDesistimiento(ctx, tenant, a, op, hub)
	case "pages.form.submitted":
		return s.avisarFormulario(ctx, tenant, a, sitio.Name, op)
	}

	pedido, err := s.store.ObtenerPedidoPorID(ctx, tenant, a.SiteID, a.OrderID)
	if err != nil {
		return err
	}
	datos := map[string]string{"number": pedido.Number, "total": importe(pedido.TotalCents, pedido.Currency, pedido.Language)}
	numero := map[string]string{"number": pedido.Number}
	pedidos := "site/" + a.SiteID.String() + "?tab=orders"
	switch ev.Type {
	case "order.paid.email_buyer":
		if pedido.BuyerEmailSentAt != nil {
			return nil
		}
		l := pedido.Language
		if err := s.enviarCorreo(ctx, op, pedido.BuyerEmail, s.textos.Texto(l, "order_paid_buyer_subject", datos), s.textos.Texto(l, "order_paid_buyer_body", datos)); err != nil {
			return err
		}
		return s.store.MarcarCorreoComprador(ctx, tenant, pedido.ID)
	case "order.paid.email_store":
		// ponytail: en el idioma en que se compró, que suele ser el de la web; el de cada
		// persona de la tienda no lo sabe el servicio.
		l := pedido.Language
		var errs []error
		for _, p := range ajustes.Notify {
			if p.Email != "" {
				errs = append(errs, s.enviarCorreo(ctx, op, p.Email, s.textos.Texto(l, "order_paid_store_subject", datos), s.textos.Texto(l, "order_paid_store_body", datos)))
			}
		}
		return errors.Join(errs...)
	case "order.paid.hub":
		return hub("pages.order.paid", "Nuevo pedido "+pedido.Number, numero, pedidos)
	case "pages.order.oversold":
		return hub("pages.order.oversold", "Pedido "+pedido.Number+" pagado sin existencias", numero, pedidos)
	case "pages.payment.duplicated":
		return hub("pages.payment.duplicated", "Cobro duplicado en el pedido "+pedido.Number, numero, pedidos)
	case "order.paid.erp":
		return s.avisarERP(ctx, tenant, pedido)
	}
	return nil
}

// importe es `29,10 EUR` (`29.10 EUR` en inglés).
func importe(centimos int, divisa, idioma string) string {
	sep := ","
	if idioma == "en" {
		sep = "."
	}
	return fmt.Sprintf("%d%s%02d %s", centimos/100, sep, centimos%100, divisa)
}

// campana avisa a la gente de la tienda (los `notify` con sub) por el Hub.
type campana func(tipo, titulo string, params map[string]string, ruta string) error

// avisarDesistimiento manda el acuse al comprador (una vez) y avisa a la tienda.
func (s *Servicio) avisarDesistimiento(ctx context.Context, tenant string, a avisoTienda, op correo.Opciones, hub campana) error {
	w, idioma, err := s.store.ObtenerDesistimiento(ctx, tenant, a.WithdrawalID)
	if err != nil {
		return err
	}
	if w.AckSentAt == nil {
		datos := map[string]string{"number": w.OrderNumber}
		if err := s.enviarCorreo(ctx, op, w.Email, s.textos.Texto(idioma, "withdrawal_ack_subject", datos), s.textos.Texto(idioma, "withdrawal_ack_body", datos)); err != nil {
			return err
		}
		if err := s.store.MarcarAcuseDesistimiento(ctx, tenant, w.ID); err != nil {
			return err
		}
	}
	return hub("pages.withdrawal.requested", "Desistimiento solicitado: "+w.OrderNumber,
		map[string]string{"number": w.OrderNumber}, "site/"+a.SiteID.String()+"?tab=orders")
}

// avisarFormulario avisa de un envío a quien diga form_settings.notify.
func (s *Servicio) avisarFormulario(ctx context.Context, tenant string, a avisoTienda, sitio string, op correo.Opciones) error {
	fs, err := s.store.ObtenerDefinicionFormulario(ctx, tenant, a.SiteID, a.FormID)
	if err != nil {
		return err
	}
	// A quién, lo dice form_settings.notify; sin nadie, el envío queda en la bandeja.
	// El tope de 200 avisos al día por sitio ya lo aplicó quien escribió el aviso.
	datos := map[string]string{"form_id": a.FormID}
	l := s.store.IdiomaDeEnvio(ctx, tenant, a.SubmissionID)
	var errs []error
	for _, p := range fs.Notify {
		if p.Email != "" {
			errs = append(errs, s.enviarCorreo(ctx, op, p.Email, s.textos.Texto(l, "form_received_subject", datos), s.textos.Texto(l, "form_received_body", datos)))
		}
		if p.Sub != "" {
			errs = append(errs, s.hub.Send(ctx, hubnotify.Notification{
				Tenant: tenant, Subject: p.Sub, Source: "pages", Type: "pages.form.received",
				Title: "Nuevo mensaje del formulario de " + sitio, Params: map[string]string{"site": sitio},
				URL: hubnotify.EnlacePages(s.publicURL, tenant, "site/"+a.SiteID.String()+"?tab=forms"),
			}))
		}
	}
	return errors.Join(errs...)
}

// avisarERP entrega el pedido pagado a SyERP para su registro contable y factura Veri*Factu (§11).
func (s *Servicio) avisarERP(ctx context.Context, tenant string, pedido *domain.Order) error {
	if s.syerpURL == "" || s.syerpSecret == "" {
		log.From(ctx).Info("pages: SyERP no configurado, omitiendo aviso erp", "tenant", tenant, "order_id", pedido.ID)
		return nil
	}
	if pedido.Test {
		log.From(ctx).Info("pages: pedido en modo prueba, no se envía a SyERP", "tenant", tenant, "order_id", pedido.ID)
		return nil
	}
	if pedido.Currency != "EUR" {
		log.From(ctx).Warn("pages: pedido en divisa no EUR, omitiendo SyERP", "tenant", tenant, "order_id", pedido.ID, "divisa", pedido.Currency)
		return nil
	}
	if pedido.ERPStatus == "done" {
		return nil
	}

	pago, err := s.store.ObtenerUltimoPagoPorOrden(ctx, tenant, pedido.ID)
	if err != nil {
		return fmt.Errorf("obtener cobro del pedido para erp: %w", err)
	}

	pt := construirPedidoTienda(tenant, pedido, pago)
	cuerpo, err := json.Marshal(pt)
	if err != nil {
		return fmt.Errorf("serializar pedido erp: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.syerpURL+"/internal/pedidos-tienda", bytes.NewReader(cuerpo))
	if err != nil {
		return fmt.Errorf("crear petición erp: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.syerpSecret)
	req.Header.Set("X-Sytue-Tenant", tenant)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("conectar con syerp: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var res struct {
			Estado     string `json:"estado"`
			PedidoRef  string `json:"pedido_ref"`
			FacturaRef string `json:"factura_ref"`
			Verifactu  string `json:"verifactu"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			log.From(ctx).Warn("pages: syerp respondió ok pero no decodificó JSON", "error", err)
		}
		return s.store.ActualizarERP(ctx, tenant, pedido.ID, "done", res.PedidoRef, res.FacturaRef)
	}

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity {
		if strings.Contains(string(respBytes), "modules_disabled") {
			log.From(ctx).Info("pages: SyERP módulos de facturación/ventas desactivados", "tenant", tenant, "order_id", pedido.ID)
			return s.store.ActualizarERP(ctx, tenant, pedido.ID, "skipped", "", "")
		}
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		log.From(ctx).Warn("pages: SyERP rechazó pedido de tienda con error no reintentable", "status", resp.StatusCode, "body", string(respBytes))
		return s.store.ActualizarERP(ctx, tenant, pedido.ID, "failed", "", "")
	}

	return fmt.Errorf("syerp /internal/pedidos-tienda devolvió código %d: %s", resp.StatusCode, string(respBytes))
}

func construirPedidoTienda(tenant string, pedido *domain.Order, pago *domain.Payment) pedidoTienda {
	var taxID, nombreFactura string
	if pedido.Invoice != nil {
		taxID, _ = pedido.Invoice["tax_id"].(string)
		nombreFactura, _ = pedido.Invoice["name"].(string)
	}
	tipoFactura := "simplificada"
	if taxID != "" && validation.NIF(taxID) {
		tipoFactura = "completa"
	}

	nombreComprador := pedido.BuyerName
	if nombreFactura != "" {
		nombreComprador = nombreFactura
	}
	if strings.TrimSpace(nombreComprador) == "" {
		nombreComprador = "Cliente Tienda Web"
	}

	pais, cp := extraerDireccionComprador(pedido.Invoice, pedido.ShippingAddress)
	if tipoFactura == "simplificada" && pais == "" {
		pais = "ES"
	}

	lineas := make([]lineaPedidoTienda, 0, len(pedido.Lines))
	for _, l := range pedido.Lines {
		nombre := l.Name
		if strings.TrimSpace(nombre) == "" {
			nombre = l.SKU
		}
		lineas = append(lineas, lineaPedidoTienda{
			SKU:                    l.SKU,
			Nombre:                 nombre,
			Cantidad:               int64(l.Quantity),
			PrecioUnitarioCentimos: int64(l.UnitPriceCents),
			TipoIVA:                int(l.VATRate),
		})
	}

	var envio *lineaPedidoTienda
	if pedido.ShippingCents > 0 {
		nombreEnvio := "Envío"
		if n, ok := pedido.ShippingMethod.Name[pedido.Language]; ok && n != "" {
			nombreEnvio = n
		} else if n, ok := pedido.ShippingMethod.Name["es"]; ok && n != "" {
			nombreEnvio = n
		}
		envio = &lineaPedidoTienda{
			Nombre:         nombreEnvio,
			PrecioCentimos: int64(pedido.ShippingCents),
			TipoIVA:        21,
		}
	}

	fecha := pedido.CreatedAt.Format("2006-01-02")
	if pedido.PaidAt != nil {
		fecha = pedido.PaidAt.Format("2006-01-02")
	}

	fechaCobro := pago.CreatedAt.UTC().Format(time.RFC3339)
	if pedido.PaidAt != nil {
		fechaCobro = pedido.PaidAt.UTC().Format(time.RFC3339)
	}

	pasarela := pago.Provider
	if pasarela == "" {
		pasarela = "web"
	}
	refCobro := pago.ProviderRef
	if refCobro == "" {
		refCobro = pedido.ID.String()
	}

	pt := pedidoTienda{
		Tenant:        tenant,
		Origen:        "pages",
		Modo:          "live",
		PedidoID:      pedido.ID.String(),
		Numero:        pedido.Number,
		Fecha:         fecha,
		Divisa:        pedido.Currency,
		Factura:       tipoFactura,
		Lineas:        lineas,
		Envio:         envio,
		TotalCentimos: int64(pedido.TotalCents),
	}
	pt.Comprador.Nombre = nombreComprador
	pt.Comprador.Correo = pedido.BuyerEmail
	pt.Comprador.NIF = taxID
	pt.Comprador.Direccion.Pais = pais
	pt.Comprador.Direccion.CP = cp

	pt.Cobro.Pasarela = pasarela
	pt.Cobro.Referencia = refCobro
	pt.Cobro.Fecha = fechaCobro
	pt.Cobro.ImporteCentimos = int64(pedido.TotalCents)
	return pt
}

var reCodigoPostal = regexp.MustCompile(`\b\d{5}\b`)

func extraerCampo(m map[string]any, claves ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range claves {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func extraerDireccionComprador(inv, ship map[string]any) (pais, cp string) {
	if inv != nil {
		if addrMap, ok := inv["address"].(map[string]any); ok {
			pais = strings.ToUpper(extraerCampo(addrMap, "country", "country_code", "pais"))
			cp = extraerCampo(addrMap, "postal_code", "postalCode", "zip", "cp")
		} else if addrStr, ok := inv["address"].(string); ok && addrStr != "" {
			cp = reCodigoPostal.FindString(addrStr)
		}
	}
	if pais == "" {
		pais = strings.ToUpper(extraerCampo(ship, "country", "country_code", "pais"))
	}
	if cp == "" {
		cp = extraerCampo(ship, "postal_code", "postalCode", "zip", "cp")
	}
	if pais == "" {
		pais = "ES"
	}
	return pais, cp
}

// pedidoTienda es el contrato privado Pages → SyERP (§11).
type pedidoTienda struct {
	Tenant    string `json:"tenant"`
	Origen    string `json:"origen"`
	Modo      string `json:"modo"`
	PedidoID  string `json:"pedido_id"`
	Numero    string `json:"numero"`
	Fecha     string `json:"fecha"`
	Divisa    string `json:"divisa"`
	Factura   string `json:"factura"`
	Comprador struct {
		Nombre    string `json:"nombre"`
		Correo    string `json:"correo"`
		NIF       string `json:"nif"`
		Direccion struct {
			Pais string `json:"pais"`
			CP   string `json:"cp"`
		} `json:"direccion"`
	} `json:"comprador"`
	Lineas        []lineaPedidoTienda `json:"lineas"`
	Envio         *lineaPedidoTienda  `json:"envio"`
	TotalCentimos int64               `json:"total_centimos"`
	Cobro         struct {
		Pasarela        string `json:"pasarela"`
		Referencia      string `json:"referencia"`
		Fecha           string `json:"fecha"`
		ImporteCentimos int64  `json:"importe_centimos"`
	} `json:"cobro"`
}

type lineaPedidoTienda struct {
	SKU                    string `json:"sku"`
	Nombre                 string `json:"nombre"`
	Cantidad               int64  `json:"cantidad"`
	PrecioUnitarioCentimos int64  `json:"precio_unitario_centimos"`
	PrecioCentimos         int64  `json:"precio_centimos"`
	TipoIVA                int    `json:"tipo_iva"`
}
