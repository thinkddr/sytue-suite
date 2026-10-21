// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

type asistenciaFactura struct {
	Data         map[string]any    `json:"data"`
	Etiquetas    map[string]string `json:"etiquetas"`
	Contacto     map[string]any    `json:"contacto,omitempty"`
	NumeroOrigen string            `json:"numero_origen"`
	Existente    *store.Registro   `json:"existente,omitempty"`
}

// LeerAsistenciaFactura — POST /{modelo}/asistencia/leer, XML crudo.
// Solo prepara una ficha de compra para revisar. No llama a terceroDe (crea
// contactos), importarFacturas, Asentar ni a VERI*FACTU. Un XML de venta ya
// emitido no es una autorización para volver a emitirlo desde este ERP.
func (h *Handlers) LeerAsistenciaFactura(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Clave != "factura_compra" || !m.RecibeFacturas() {
		httpx.Fail(w, r, errFacturaIlegible.WithMessage("Electronic invoices can only prepare a purchase invoice. Already issued sales invoices must not be reissued."))
		return
	}
	if !h.puede(r.Context(), m.Modulo, permisos.Usuario) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	if !h.puedeVerbo(r.Context(), m.Modulo, permisos.Crear) {
		httpx.Fail(w, r, errSinVerbo(permisos.Crear))
		return
	}
	contacto, ok := h.cat.Modelo("contacto")
	if !ok {
		httpx.Fail(w, r, errModeloDesconocido)
		return
	}
	// Comprobar ANTES de buscar: la lectura de obligaciones/facturación nunca
	// permite conocer un contacto oculto ni su identificador.
	if !h.puede(r.Context(), contacto.Modulo, permisos.Lectura) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportBytes))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Send one electronic invoice of up to 8 MiB."))
		return
	}
	cuerpo = bytes.TrimPrefix(cuerpo, []byte("\xef\xbb\xbf"))
	if !esXML(cuerpo) {
		httpx.Fail(w, r, errFacturaIlegible.WithMessage("Send one UBL, CII or Facturae XML invoice. CSV is not supported by this assistant."))
		return
	}
	facturas, err := leerFacturas(cuerpo)
	if err != nil {
		httpx.Fail(w, r, errFacturaIlegible.WithMessage(err.Error()))
		return
	}
	if len(facturas) != 1 {
		httpx.Fail(w, r, errFacturaIlegible.WithMessage("This assistant reads one invoice at a time. Split the XML batch first."))
		return
	}
	org, err := h.store.Organizacion(r.Context())
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := facturas[0]
	if err := validarAsistenciaFactura(f, org); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.prepararAsistenciaFactura(r.Context(), m, contacto, f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	externo, _ := out.Data["externo_id"].(string)
	existente, err := h.store.BuscarPorExterno(r.Context(), m.Clave, externo)
	if err != nil && !errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, err)
		return
	}
	if err == nil {
		out.Existente = &existente
		out.Contacto = nil
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func validarAsistenciaFactura(f facturaRecibida, org store.Organizacion) error {
	if org.NIF == "" || f.Receptor.NIF == "" || !domain.MismoNIF(org.NIF, f.Receptor.NIF) {
		return errFacturaIlegible.WithMessage("The invoice must identify your organisation as its recipient. Check both tax identification numbers.")
	}
	if !strings.EqualFold(f.Moneda, domain.MonedaPorDefecto) {
		return errFacturaIlegible.WithMessage("The invoice must explicitly use EUR. Currency conversion is not performed.")
	}
	if err := f.comprobar(org, true); err != nil {
		return errFacturaIlegible.WithMessage(err.Error())
	}
	if (f.IVA > 0 && f.Base > math.MaxInt64-f.IVA) || (f.IVA < 0 && f.Base < math.MinInt64-f.IVA) {
		return errFacturaIlegible.WithMessage("The invoice total exceeds the supported amount.")
	}
	return nil
}

func (h *Handlers) prepararAsistenciaFactura(ctx context.Context, m, contacto *catalogo.Modelo, f facturaRecibida) (asistenciaFactura, error) {
	out := asistenciaFactura{Etiquetas: map[string]string{}, NumeroOrigen: validation.SanitizeText(f.Numero)}
	var err error
	datos := map[string]any{"fecha": f.Fecha, "base": f.Base, "iva": f.IVA, "total": f.Total}
	emisor := f.Emisor.Nombre
	if variantes := domain.VariantesNIF(f.Emisor.NIF); len(variantes) > 0 {
		emisor = variantes[len(variantes)-1]
		id, err := h.store.BuscarPorNIF(ctx, contacto.Clave, f.Emisor.NIF)
		if err != nil && !errors.Is(err, store.ErrNoEncontrado) {
			return out, err
		}
		if err == nil {
			reg, err := h.store.Obtener(ctx, contacto.Clave, id)
			if err != nil {
				return out, err
			}
			datos[m.Contable.CampoTercero] = reg.ID
			out.Etiquetas[reg.ID] = cmp.Or(reg.Etiqueta, reg.Ref)
		}
	}
	// La misma identidad de origen que la importación XML: proveedor/número.
	datos["externo_id"] = emisor + "/" + f.Numero
	if f.Vencimiento != "" {
		datos["vencimiento"] = f.Vencimiento
	}
	if f.Nota != "" {
		datos["notas"] = f.Nota
	}
	if _, hay := datos[m.Contable.CampoTercero]; !hay {
		out.Contacto, err = contactoAsistenciaFactura(contacto, f.Emisor)
		if err != nil {
			return out, errFacturaIlegible.WithMessage(err.Error())
		}
	}
	// Solo la relación puede estar aún pendiente: el resto conserva toda la
	// validación del modelo (fechas reales, céntimos enteros, límites de texto).
	preview := *m
	preview.Campos = append([]catalogo.Campo(nil), m.Campos...)
	for i := range preview.Campos {
		if preview.Campos[i].Clave == m.Contable.CampoTercero {
			preview.Campos[i].Requerido = false
		}
	}
	crudo, err := json.Marshal(datos)
	if err == nil {
		out.Data, _, err = domain.Validar(&preview, crudo)
	}
	if err != nil {
		return out, errFacturaIlegible.WithMessage(err.Error())
	}
	return out, nil
}

func contactoAsistenciaFactura(m *catalogo.Modelo, p parteFactura) (map[string]any, error) {
	tipo := "organizacion"
	if p.Persona {
		tipo = "persona"
	}
	datos := map[string]any{"nombre": primero(p.Nombre, p.NIF), "tipo": tipo}
	for clave, valor := range map[string]string{
		"nif": p.NIF, "direccion": p.Direccion, "ciudad": p.Ciudad, "cp": p.CP, "pais": p.Pais,
	} {
		if valor != "" {
			datos[clave] = valor
		}
	}
	if validation.IsEmail(p.Correo) {
		datos["correo"] = p.Correo
	}
	crudo, err := json.Marshal(datos)
	if err != nil {
		return nil, err
	}
	data, _, err := domain.Validar(m, crudo)
	return data, err
}
