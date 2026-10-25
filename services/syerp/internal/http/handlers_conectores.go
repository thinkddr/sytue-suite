// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/internal/conectores"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

var errConectorDesconocido = httpx.NewError(http.StatusNotFound, "conector_desconocido",
	"That connector is not available.")

// --- Configuración de conexiones (solo administración) ----------------------

type cuerpoConexion struct {
	Instancia string         `json:"instancia"`
	Secreto   string         `json:"secreto"`
	Config    map[string]any `json:"config"`
	Activo    bool           `json:"activo"`
}

// proveedoresConexion son los que admiten una conexión guardada (la misma lista
// que la restricción de la tabla `conexiones`, mig 0007).
var proveedoresConexion = map[string]bool{
	"stripe": true, "salesforce": true, "odoo": true, "business-central": true, "holded": true, proveedorBases: true,
}

// GuardarConexion configura un proveedor de pago o de importación para la organización.
func (h *Handlers) GuardarConexion(w http.ResponseWriter, r *http.Request) {
	proveedor := chi.URLParam(r, "proveedor")
	if !proveedoresConexion[proveedor] {
		httpx.Fail(w, r, errConectorDesconocido)
		return
	}
	var cuerpo cuerpoConexion
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if proveedor == proveedorBases {
		h.guardarConexionBases(w, r, cuerpo)
		return
	}
	err := h.store.GuardarConexion(r.Context(), store.Conexion{
		Proveedor: proveedor, Instancia: cuerpo.Instancia,
		Config: cuerpo.Config, Activo: cuerpo.Activo, Secreto: cuerpo.Secreto,
	}, h.cifra, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.NoContent(w)
}

// ListarConexiones devuelve las conexiones configuradas, sin secretos.
func (h *Handlers) ListarConexiones(w http.ResponseWriter, r *http.Request) {
	cx, err := h.store.ListarConexiones(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"conexiones": cx})
}

// --- Importar desde Salesforce ----------------------------------------------

type cuerpoImportar struct {
	// Objetos de Salesforce a traer. Vacío = los tres del mapeo.
	Objetos []string `json:"objetos"`
	// Limite por objeto. La pasarela también tiene el suyo.
	Limite int `json:"limite"`
}

type resultadoObjeto struct {
	Objeto       string   `json:"objeto"`
	Modelo       string   `json:"modelo"`
	Creados      int      `json:"creados"`
	Actualizados int      `json:"actualizados"`
	Fallos       []string `json:"fallos,omitempty"`
}

// ImportarSalesforce trae cuentas, contactos y oportunidades de Salesforce.
//
// Sale por el Integration Gateway reenviando la autorización de quien importa:
// SyERP no guarda credenciales del tercero ni tiene una cuenta de servicio con
// acceso a todas las organizaciones.
//
// Es idempotente: cada fila se identifica por su id de origen (`externo_id`),
// así que importar dos veces actualiza en vez de duplicar.
func (h *Handlers) ImportarSalesforce(w http.ResponseWriter, r *http.Request) {
	h.importarDesde(w, r, "salesforce", conectores.MapeosSalesforce, func(m conectores.Mapeo, limite int) map[string]any {
		return map[string]any{"object": m.Objeto, "fields": m.Columnas(), "limit": limite}
	})
}

// ImportarOdoo trae contactos, productos, facturas emitidas y oportunidades de
// Odoo. Mismo camino que Salesforce: el driver JSON-RPC vive en la pasarela.
func (h *Handlers) ImportarOdoo(w http.ResponseWriter, r *http.Request) {
	h.importarDesde(w, r, "odoo", conectores.MapeosOdoo, func(m conectores.Mapeo, limite int) map[string]any {
		spec := map[string]any{"model": m.Objeto, "fields": m.Columnas(), "limit": limite}
		if m.Filtro != nil {
			spec["domain"] = m.Filtro
		}
		return spec
	})
}

// ImportarBusinessCentral trae clientes, proveedores, artículos y facturas de
// Microsoft Dynamics 365 Business Central por el driver OData de la pasarela.
func (h *Handlers) ImportarBusinessCentral(w http.ResponseWriter, r *http.Request) {
	h.importarDesde(w, r, "business-central", conectores.MapeosBusinessCentral, func(m conectores.Mapeo, limite int) map[string]any {
		spec := map[string]any{"entity_set": m.Objeto, "select": m.Columnas(), "max_rows": limite}
		if filtro, ok := m.Filtro.(string); ok && filtro != "" {
			spec["filter"] = filtro
		}
		return spec
	})
}

// ImportarHolded trae contactos, productos y facturas emitidas y recibidas de
// Holded por su driver de la pasarela.
func (h *Handlers) ImportarHolded(w http.ResponseWriter, r *http.Request) {
	h.importarDesde(w, r, "holded", conectores.MapeosHolded, func(m conectores.Mapeo, limite int) map[string]any {
		return map[string]any{"resource": m.Objeto, "fields": m.Columnas(), "limit": limite}
	})
}

func (h *Handlers) importarDesde(w http.ResponseWriter, r *http.Request, proveedor string,
	mapeos []conectores.Mapeo, spec func(conectores.Mapeo, int) map[string]any,
) {
	if h.pasarela == nil {
		httpx.Fail(w, r, errConectorDesconocido)
		return
	}
	var cuerpo cuerpoImportar
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cx, err := h.store.Conexion(r.Context(), proveedor, h.cifra)
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	if !cx.Activo || cx.Instancia == "" {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage(
			proveedor+" is not connected: set the gateway instance first."))
		return
	}

	objetos := cuerpo.Objetos
	if len(objetos) == 0 {
		for _, m := range mapeos {
			objetos = append(objetos, m.Clave())
		}
	}
	autorizacion := r.Header.Get("Authorization")
	var out []resultadoObjeto

	for _, objeto := range objetos {
		mapeo, ok := conectores.MapeoDe(mapeos, objeto)
		if !ok {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("unknown "+proveedor+" object: "+objeto))
			return
		}
		modelo, ok := h.cat.Modelo(mapeo.Modelo)
		if !ok {
			httpx.Fail(w, r, errModeloDesconocido)
			return
		}
		tabla, err := h.pasarela.Consultar(r.Context(), cx.Instancia, autorizacion, spec(mapeo, cuerpo.Limite))
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnprocessable.Wrap(err).WithMessage(err.Error()))
			return
		}

		filas, fallos := mapeo.Traducir(tabla)
		res := resultadoObjeto{Objeto: mapeo.Clave(), Modelo: mapeo.Modelo}
		for _, f := range fallos {
			res.Fallos = append(res.Fallos, f.Error())
		}
		for _, fila := range filas {
			etiqueta := ""
			if s, ok := fila.Data["nombre"].(string); ok {
				etiqueta = s
			}
			creado, err := h.store.UpsertPorExterno(r.Context(), modelo, fila.ExternoID,
				fila.Data, etiqueta, fila.Estado, sub(r))
			if err != nil {
				// Una fila que no entra no aborta la importación: se cuenta y se
				// sigue. Abortar en la fila 4.999 de 5.000 deja el trabajo a
				// medias y sin forma de saber por dónde iba.
				res.Fallos = append(res.Fallos, fila.ExternoID+": "+err.Error())
				continue
			}
			if creado {
				res.Creados++
			} else {
				res.Actualizados++
			}
		}
		out = append(out, res)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"importado": out})
}

// --- Webhook de Stripe ------------------------------------------------------

// WebhookStripe recibe los eventos de pago de Stripe.
//
// NO PASA POR EL MIDDLEWARE DE SESIÓN: Stripe no puede presentar un token de
// Sytue ID. Su autenticación es la FIRMA del cuerpo, y por eso se verifica antes
// de mirar nada más. La organización va en la ruta; conocer el slug no sirve de
// nada sin el secreto de firma.
//
// Siempre responde 200 a un evento verificado que no interesa: devolver error a
// algo que nunca va a cambiar hace que Stripe reintente durante días.
func (h *Handlers) WebhookStripe(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "org")
	tenant, err := tenancy.New(slug)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ctx := tenancy.Into(r.Context(), tenant)
	registro := log.From(ctx).With("proveedor", "stripe", "org", logsafe.Text(slug))

	// El cuerpo CRUDO, byte a byte: la firma se calcula sobre él y reserializar
	// el JSON la invalida.
	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, conectores.MaxCuerpoWebhook+1))
	if err != nil || len(cuerpo) > conectores.MaxCuerpoWebhook {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}

	cx, err := h.store.Conexion(ctx, "stripe", h.cifra)
	if err != nil || !cx.Activo || cx.Secreto == "" {
		// Mismo 404 que un slug inexistente: quien prueba no debe poder
		// distinguir «esa organización no existe» de «no tiene Stripe puesto».
		registro.Warn("webhook de Stripe sin conexión configurada", "error", err)
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}

	if err := conectores.VerificarFirmaStripe(cuerpo, r.Header.Get("Stripe-Signature"),
		cx.Secreto, time.Now()); err != nil {
		registro.Warn("webhook de Stripe rechazado", "error", err)
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}

	cobro, err := conectores.LeerEventoStripe(cuerpo)
	if errors.Is(err, conectores.ErrEventoIgnorado) {
		// Verificado, pero no mueve dinero: 200 y a otra cosa. Devolver error a un
		// evento que nunca va a cambiar hace que Stripe reintente durante días.
		httpx.NoContent(w)
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.Wrap(err))
		return
	}

	// Lo que nunca va a casar se anota y se acepta con 204 —devolver error haría
	// que Stripe lo reintentara durante días— y se deja rastro.
	aceptarSinCobro := func(motivo string) {
		if _, err := h.store.MarcarEvento(ctx, "stripe", cobro.EventoID, cobro.Tipo); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		registro.Warn(motivo, "evento", logsafe.Text(cobro.EventoID), "externo", logsafe.Text(cobro.ExternoID), "ref", logsafe.Text(cobro.RefDocumento))
		httpx.NoContent(w)
	}

	factura, err := h.facturaDelCobro(ctx, cobro)
	if err != nil {
		aceptarSinCobro("cobro de Stripe sin factura que casar")
		return
	}

	cuentaBanco := textoDeConfig(cx.Config, "cuenta_banco", "572000")
	cuentaCliente := textoDeConfig(cx.Config, "cuenta_cliente", "430000")
	diario := textoDeConfig(cx.Config, "diario", "banco")

	// Una sola vez: Stripe reintenta el MISMO evento por diseño. La marca va en
	// la transacción del cobro: si este falla, el reintento lo vuelve a intentar.
	asiento, err := h.store.RegistrarCobro(ctx, factura.ID, cobro.EventoID, cobro.Tipo, cuentaBanco, cuentaCliente,
		diario, "Cobro "+factura.Ref+" · Stripe", cobro.Centimos, time.Now().UTC(), "stripe")
	switch {
	case errors.Is(err, store.ErrEventoRepetido):
		registro.Info("evento de Stripe repetido, ignorado", "evento", logsafe.Text(cobro.EventoID))
		httpx.NoContent(w)
		return
	case errors.Is(err, store.ErrFacturaNoAbierta):
		aceptarSinCobro("cobro de Stripe de una factura ya pagada o anulada")
		return
	case errors.Is(err, store.ErrImporteNoCasa):
		aceptarSinCobro("cobro de Stripe por un importe que no es el de la factura")
		return
	case err != nil:
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	registro.Info("cobro registrado desde Stripe",
		"factura", factura.Ref, "asiento", asiento.Ref, "centimos", cobro.Centimos)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"asiento": asiento.Ref})
}

// facturaDelCobro localiza la factura: por la referencia que Stripe lleva en
// metadata si está, y si no por el identificador del objeto guardado en
// `externo_id`.
func (h *Handlers) facturaDelCobro(ctx context.Context, c conectores.Cobro) (store.Registro, error) {
	if c.RefDocumento != "" {
		reg, err := h.store.BuscarPorRef(ctx, "factura_venta", c.RefDocumento)
		if err == nil {
			return reg, nil
		}
		if !errors.Is(err, store.ErrNoEncontrado) {
			return store.Registro{}, err
		}
	}
	return h.store.BuscarPorExterno(ctx, "factura_venta", c.ExternoID)
}

func textoDeConfig(cfg map[string]any, clave, porDefecto string) string {
	if s, ok := cfg[clave].(string); ok && s != "" {
		return s
	}
	return porDefecto
}
