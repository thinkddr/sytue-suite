// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/importar"
)

// La factura que sale (Facturae, UBL, el impreso y su PDF) y la que entra (UBL, CII o
// Facturae en XML), con apps/nucleo/erp/efactura: la misma generación y la misma
// lectura que services/syerp. Sin VERI*FACTU (D-29): sin QR tributario.

// facturaDe traduce un registro del almacén a efactura.Factura.
func facturaDe(reg api.ERPRegistro, campoBase, campoIVA string, lineas []dominio.LineaFactura) efactura.Factura {
	texto := func(k string) string { s, _ := reg.Data[k].(string); return s }
	base, _ := dominio.Centimos(reg.Data, campoBase)
	iva, _ := dominio.Centimos(reg.Data, campoIVA)
	return efactura.Factura{
		ID: reg.ID, Numero: reg.Ref, Fecha: texto("fecha"), Vencimiento: texto("vencimiento"), Notas: texto("notas"),
		Estado: reg.Estado, Tipo: texto("tipo_factura"), RegimenIVA: texto("regimen_iva"),
		Base: base, IVA: iva, Lineas: lineas, Rectificativa: texto("rectifica") != "",
	}
}

// emisor es el perfil de la organización como emisor.
func (s *Servidor) emisor(ctx context.Context, o api.ERPOrganizacion) efactura.Parte {
	return efactura.Parte{
		Nombre: o.Nombre, NIF: o.NIF, Direccion: o.Direccion, CP: o.CP, Ciudad: o.Ciudad, Pais: o.Pais,
		Fisica: s.esPersonaFisica(ctx),
	}
}

// receptorDe es el contacto como receptor de una factura electrónica (por su etiqueta)
// o, con `impreso`, como lo nombra el impreso (por su nombre).
func receptorDe(c api.ERPRegistro, impreso bool) *efactura.Parte {
	texto := func(k string) string { s, _ := c.Data[k].(string); return strings.TrimSpace(s) }
	nombre := c.Etiqueta
	if impreso {
		nombre = efactura.Primero(texto("nombre"), c.Etiqueta)
	}
	return &efactura.Parte{
		Nombre: nombre, NIF: texto("nif"), Direccion: texto("direccion"), CP: texto("cp"), Ciudad: texto("ciudad"),
		Pais: texto("pais"), Correo: texto("correo"), Fisica: texto("tipo") == "persona",
		DIR3: [3]string{texto("dir3_contable"), texto("dir3_gestor"), texto("dir3_tramitadora")},
	}
}

// factura reúne la factura, sus líneas, el perfil y el cliente. Si devuelve false, ya
// ha respondido.
type facturaCompleta struct {
	reg     api.ERPRegistro
	org     api.ERPOrganizacion
	lineas  []dominio.LineaFactura
	cliente *api.ERPRegistro
}

func (s *Servidor) facturaCompleta(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo, campoCliente string) (facturaCompleta, bool) {
	ctx := r.Context()
	var out facturaCompleta
	var err error
	if out.reg, err = s.alm.Obtener(ctx, m.Clave, r.PathValue("id")); err != nil {
		falloDe(w, err)
		return out, false
	}
	if out.org, err = s.alm.Organizacion(ctx); errors.Is(err, ErrNoEncontrado) {
		fallo(w, http.StatusConflict, "organizacion_no_configurada", "The organisation profile has not been set up yet.", nil)
		return out, false
	} else if err != nil {
		falloDe(w, err)
		return out, false
	}
	if out.lineas, err = s.alm.lineasDeFactura(ctx, s.alm.db, out.reg.ID); err != nil {
		falloDe(w, err)
		return out, false
	}
	if id, _ := out.reg.Data[campoCliente].(string); id != "" {
		c, err := s.alm.Obtener(ctx, "contacto", id)
		if err != nil && !errors.Is(err, ErrNoEncontrado) {
			falloDe(w, err)
			return out, false
		}
		if err == nil {
			out.cliente = &c
		}
	}
	return out, true
}

func noContable(w http.ResponseWriter, mensaje string) {
	fallo(w, http.StatusBadRequest, "no_contable", mensaje, nil)
}

func irrealizable(w http.ResponseWriter, err error) {
	fallo(w, http.StatusUnprocessableEntity, "unprocessable", err.Error(), nil)
}

func adjuntar(w http.ResponseWriter, tipo, nombre string, cuerpo []byte) {
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": nombre}))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cuerpo)
}

// facturae — GET /{modelo}/{id}/facturae — Facturae 3.2.2 sin firmar.
func (s *Servidor) facturae(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if m.Contable == nil || !m.Contable.Cobro {
		noContable(w, "Only issued invoices can be exported as Facturae.")
		return
	}
	fc, ok := s.facturaCompleta(w, r, m, m.Contable.CampoTercero)
	if !ok {
		return
	}
	f := facturaDe(fc.reg, m.Contable.CampoBase, m.Contable.CampoIVA, fc.lineas)
	if f.Simplificada() {
		irrealizable(w, efactura.ErrSimplificada)
		return
	}
	var receptor *efactura.Parte
	if fc.cliente != nil {
		receptor = receptorDe(*fc.cliente, false)
	}
	doc, err := efactura.Facturae(s.emisor(r.Context(), fc.org), receptor, f)
	if err != nil {
		irrealizable(w, err)
		return
	}
	cuerpo, err := doc.Bytes()
	if err != nil {
		falloDe(w, err)
		return
	}
	adjuntar(w, "application/xml; charset=utf-8", doc.Nombre(), cuerpo)
}

// ubl — GET /{modelo}/{id}/ubl — UBL 2.1.
func (s *Servidor) ubl(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if m.Contable == nil || !m.Contable.Cobro {
		noContable(w, "Only issued invoices can be exported as UBL.")
		return
	}
	fc, ok := s.facturaCompleta(w, r, m, m.Contable.CampoTercero)
	if !ok {
		return
	}
	f := facturaDe(fc.reg, m.Contable.CampoBase, m.Contable.CampoIVA, fc.lineas)
	if err := efactura.Admitida(f); err != nil {
		irrealizable(w, err)
		return
	}
	var receptor *efactura.Parte
	if fc.cliente != nil {
		receptor = receptorDe(*fc.cliente, false)
	}
	f.Lineas = nil
	cuerpo, nombre, err := efactura.UBL(s.emisor(r.Context(), fc.org), receptor, f)
	if err != nil {
		falloDe(w, err)
		return
	}
	adjuntar(w, "application/xml; charset=utf-8", nombre, cuerpo)
}

// impreso arma la factura para el cliente; si devuelve false, ya ha respondido.
func (s *Servidor) impreso(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) (efactura.Imprimible, bool) {
	f, _, ok := s.impresoDe(w, r, m)
	return f, ok
}

// impresoDe es impreso con el registro de la factura (su estado decide si se envía).
func (s *Servidor) impresoDe(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) (efactura.Imprimible, api.ERPRegistro, bool) {
	if m.Clave != "factura_venta" {
		noContable(w, "Only customer invoices can be printed.")
		return efactura.Imprimible{}, api.ERPRegistro{}, false
	}
	fc, ok := s.facturaCompleta(w, r, m, "cliente")
	if !ok {
		return efactura.Imprimible{}, api.ERPRegistro{}, false
	}
	f := facturaDe(fc.reg, "base", "iva", fc.lineas)
	if id, _ := fc.reg.Data["rectifica"].(string); id != "" {
		if orig, err := s.alm.Obtener(r.Context(), "factura_venta", id); err == nil {
			f.Rectifica = orig.Ref
		}
	}
	var cliente *efactura.Parte
	if fc.cliente != nil {
		cliente = receptorDe(*fc.cliente, true)
	}
	l := idioma(r)
	return efactura.Imprimir(f, s.emisor(r.Context(), fc.org), cliente, l, func(clave string) string { return s.cat.Rotulo(clave, l) }), fc.reg, true
}

// imprimir — GET /{modelo}/{id}/imprimir — la factura como página para imprimir.
func (s *Servidor) imprimir(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	f, ok := s.impreso(w, r, m)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := f.HTML(w); err != nil {
		falloDe(w, err)
	}
}

// pdfFactura — GET /{modelo}/{id}/pdf — la misma factura en PDF (apps/nucleo/pdfdoc).
func (s *Servidor) pdfFactura(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	f, ok := s.impreso(w, r, m)
	if !ok {
		return
	}
	pdf, err := f.PDF()
	if err != nil {
		falloDe(w, err)
		return
	}
	adjuntar(w, "application/pdf", f.NombrePDF(), pdf)
}

// --- Facturas recibidas ----------------------------------------------------------------

func facturaIlegible(w http.ResponseWriter, mensaje string) {
	fallo(w, http.StatusUnprocessableEntity, "factura_ilegible", mensaje, nil)
}

// contactoPorNIF es el contacto vivo con ese NIF (con o sin prefijo de país).
func (a *Almacen) contactoPorNIF(ctx context.Context, nif string) (api.ERPRegistro, bool, error) {
	regs, err := a.listarTx(ctx, a.db, "contacto", false, "")
	if err != nil {
		return api.ERPRegistro{}, false, err
	}
	for i := len(regs) - 1; i >= 0; i-- {
		if n, _ := regs[i].Data["nif"].(string); n != "" && dominio.MismoNIF(n, nif) {
			return regs[i], true, nil
		}
	}
	return api.ERPRegistro{}, false, nil
}

// leerFactura — POST /factura_compra/asistencia/leer — un XML de proveedor → la ficha
// de una factura recibida para revisar. No crea nada.
func (s *Servidor) leerFactura(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if m.Clave != "factura_compra" || !m.RecibeFacturas() {
		facturaIlegible(w, "Electronic invoices can only prepare a purchase invoice. Already issued sales invoices must not be reissued.")
		return
	}
	contacto, ok := s.cat.Modelo("contacto")
	if !ok {
		fallo(w, http.StatusNotFound, "modelo_desconocido", "That model does not exist in the catalogue.", nil)
		return
	}
	f, ok := leerUnaFactura(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	org, err := s.alm.Organizacion(ctx)
	if errors.Is(err, ErrNoEncontrado) {
		fallo(w, http.StatusConflict, "organizacion_no_configurada", "The organisation profile has not been set up yet.", nil)
		return
	}
	if err != nil {
		falloDe(w, err)
		return
	}
	if err := f.ComprobarAsistencia(org.NIF); err != nil {
		facturaIlegible(w, err.Error())
		return
	}
	out := api.ERPRelleno{Etiquetas: map[string]string{}, NumeroOrigen: f.Numero}
	datos := map[string]any{"fecha": f.Fecha, "base": f.Base, "iva": f.IVA, "total": f.Total, "externo_id": f.ClaveExterna()}
	if f.Vencimiento != "" {
		datos["vencimiento"] = f.Vencimiento
	}
	if f.Nota != "" {
		datos["notas"] = f.Nota
	}
	if f.Emisor.NIF != "" {
		c, hay, err := s.alm.contactoPorNIF(ctx, f.Emisor.NIF)
		if err != nil {
			falloDe(w, err)
			return
		}
		if hay {
			datos[m.Contable.CampoTercero] = c.ID
			out.Etiquetas[c.ID] = efactura.Primero(c.Etiqueta, c.Ref)
		}
	}
	if _, hay := datos[m.Contable.CampoTercero]; !hay {
		crudo, _ := json.Marshal(f.Emisor.DatosContacto())
		if out.Contacto, _, err = dominio.Validar(contacto, crudo); err != nil {
			facturaIlegible(w, err.Error())
			return
		}
	}
	// Solo la relación puede estar aún pendiente: el resto pasa la validación del modelo.
	vista := *m
	vista.Campos = append([]catalogo.Campo(nil), m.Campos...)
	for i := range vista.Campos {
		if vista.Campos[i].Clave == m.Contable.CampoTercero {
			vista.Campos[i].Requerido = false
		}
	}
	crudo, _ := json.Marshal(datos)
	if out.Data, _, err = dominio.Validar(&vista, crudo); err != nil {
		facturaIlegible(w, err.Error())
		return
	}
	existente, hay, err := s.alm.buscarPor(ctx, s.alm.db, m.Clave, "externo_id", f.ClaveExterna())
	if err != nil {
		falloDe(w, err)
		return
	}
	if hay {
		out.Existente, out.Contacto = &existente, nil
	}
	responder(w, http.StatusOK, out)
}

// leerUnaFactura lee del cuerpo una sola factura electrónica en XML. Si no puede, ya ha
// contestado por qué y devuelve false.
func leerUnaFactura(w http.ResponseWriter, r *http.Request) (efactura.Recibida, bool) {
	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, maxImportBytes+1))
	if err != nil || len(cuerpo) > maxImportBytes {
		fallo(w, http.StatusBadRequest, "bad_request", "Send one electronic invoice of up to 8 MiB.", nil)
		return efactura.Recibida{}, false
	}
	cuerpo = trasBOM(cuerpo)
	if !efactura.EsXML(cuerpo) {
		facturaIlegible(w, "Send one UBL, CII or Facturae XML invoice. CSV is not supported by this assistant.")
		return efactura.Recibida{}, false
	}
	facturas, err := efactura.Leer(cuerpo)
	if err != nil {
		facturaIlegible(w, err.Error())
		return efactura.Recibida{}, false
	}
	if len(facturas) != 1 {
		facturaIlegible(w, "This assistant reads one invoice at a time. Split the XML batch first.")
		return efactura.Recibida{}, false
	}
	return facturas[0], true
}

// importarFacturas — la rama XML de POST /{modelo}/importar: una «fila» por factura del
// fichero, en un lote con marcha atrás como el CSV. El emisor se busca por NIF (o por
// nombre) y, si no está, se da de alta.
func (s *Servidor) importarFacturas(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo, cuerpo []byte) {
	if !m.RecibeFacturas() {
		facturaIlegible(w, "Electronic invoices can only be imported as received (purchase) invoices.")
		return
	}
	facturas, err := efactura.Leer(cuerpo)
	if err != nil {
		facturaIlegible(w, err.Error())
		return
	}
	ctx := r.Context()
	nifOrg := ""
	if org, err := s.alm.Organizacion(ctx); err == nil {
		nifOrg = org.NIF
	} else if !errors.Is(err, ErrNoEncontrado) {
		falloDe(w, err)
		return
	}
	contacto, _ := s.cat.Modelo("contacto")
	var filas []importar.Fila
	var fallos []importar.Fallo
	for i, f := range facturas {
		n := i + 1
		if n > maxImportFilas {
			fallos = append(fallos, importar.Fallo{Fila: n, Error: fmt.Sprintf("tope de %d facturas por importación", maxImportFilas)})
			break
		}
		if err := f.Comprobar(nifOrg); err != nil {
			fallos = append(fallos, importar.Fallo{Fila: n, Error: f.Numero + ": " + err.Error()})
			continue
		}
		tercero, err := s.terceroDe(ctx, contacto, f.Emisor)
		if err != nil {
			fallos = append(fallos, importar.Fallo{Fila: n, Error: f.Numero + ": " + err.Error()})
			continue
		}
		datos := map[string]any{
			m.Contable.CampoTercero: tercero, "fecha": f.Fecha, "base": f.Base, "iva": f.IVA,
			"total": f.Total, "externo_id": f.ClaveExterna(),
		}
		if f.Vencimiento != "" {
			datos["vencimiento"] = f.Vencimiento
		}
		if f.Nota != "" {
			datos["notas"] = f.Nota
		}
		crudo, _ := json.Marshal(datos)
		data, etiqueta, err := dominio.Validar(m, crudo)
		if err != nil {
			fallos = append(fallos, importar.Fallo{Fila: n, Error: f.Numero + ": " + err.Error()})
			continue
		}
		filas = append(filas, importar.Fila{Linea: n, Data: data, Etiqueta: etiqueta, Externo: f.ClaveExterna()})
	}
	fichero := strings.TrimSpace(r.URL.Query().Get("fichero"))
	if len(fichero) > 300 {
		fichero = fichero[:300]
	}
	res, err := s.alm.Importar(ctx, m, filas, fichero, false)
	if err != nil {
		falloDe(w, err)
		return
	}
	res.Filas = len(facturas)
	res.Fallos = append(fallos, res.Fallos...)
	responder(w, http.StatusOK, struct {
		ResultadoImportacion
		Simulacion bool     `json:"simulacion"`
		Ignoradas  []string `json:"ignoradas"`
	}{res, false, []string{}})
}

// terceroDe encuentra al emisor entre los contactos —por NIF y, si la factura no lo
// trae, por nombre— o lo da de alta con lo que dice la factura.
func (s *Servidor) terceroDe(ctx context.Context, contacto *catalogo.Modelo, p efactura.ParteRecibida) (string, error) {
	if p.NIF != "" {
		c, hay, err := s.alm.contactoPorNIF(ctx, p.NIF)
		if err != nil || hay {
			return c.ID, err
		}
	} else if id, err := s.alm.BuscarPorEtiqueta(ctx, contacto.Clave, p.Nombre); !errors.Is(err, ErrNoEncontrado) {
		return id, err
	}
	crudo, err := json.Marshal(p.DatosContacto())
	if err != nil {
		return "", err
	}
	data, etiqueta, err := dominio.Validar(contacto, crudo)
	if err != nil {
		return "", fmt.Errorf("emisor %s: %w", p.Nombre, err)
	}
	reg, err := s.alm.Crear(ctx, contacto, data, etiqueta)
	return reg.ID, err
}

func trasBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf {
		return b[3:]
	}
	return b
}
