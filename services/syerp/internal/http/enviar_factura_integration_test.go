// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// La factura sale por correo a la dirección de facturación del cliente, con el PDF
// adjunto, desde el buzón del ERP con el nombre de la organización y respondiendo a
// quien la manda; queda en el historial. Un borrador no sale.
func TestEnviarFacturaPorCorreo(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	hs.cfg.SMTPAddr, hs.cfg.SMTPFrom = "smtp.invalid:587", "Sytue ERP <erp@sytue.eu>"
	type envio struct {
		op           correo.Opciones
		para, asunto string
		adjunto      correo.Adjunto
	}
	var enviados []envio
	hs.enviarCorreo = func(_ context.Context, _ correo.Config, op correo.Opciones, para, asunto, _ string, a correo.Adjunto) error {
		enviados = append(enviados, envio{op, para, asunto, a})
		return nil
	}
	if w := llamar(t, "acme", http.MethodPut, "/api/v1/organizacion", "application/json",
		`{"nombre":"Acme Ibérica SL","nif":"B12345678","direccion":"Gran Vía 1","cp":"28013","ciudad":"Madrid","pais":"ES","forma":"sl"}`,
		nil, hs.GuardarOrganizacion); w.Code != http.StatusOK {
		t.Fatalf("perfil: %d %s", w.Code, w.Body.String())
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	contacto, _ := hs.cat.Modelo("contacto")
	factura, _ := hs.cat.Modelo("factura_venta")
	cliente := crear(ctx, t, st, contacto,
		`{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"B12345674","correo":"hola@cliente.es","email_facturas":"facturas@cliente.es"}`)
	id := crear(ctx, t, st, factura,
		`{"cliente":"`+cliente.ID+`","fecha":"2026-09-12","base":100000,"iva":21000,"total":121000,"notas":"Pan de septiembre"}`).ID
	params := map[string]string{"modelo": "factura_venta", "id": id}
	mandar := func() int {
		return comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/factura_venta/"+id+"/enviar",
			`{"para":"facturas@cliente.es","asunto":"Factura","mensaje":"Adjunta."}`, params, http.HandlerFunc(hs.EnviarFactura)).Code
	}

	w := llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+id+"/enviar", "", "", params, hs.EnvioFactura)
	var previa struct {
		Para                 string
		Enviable, Disponible bool
	}
	if err := json.Unmarshal(w.Body.Bytes(), &previa); err != nil || previa.Para != "facturas@cliente.es" || previa.Enviable || !previa.Disponible {
		t.Fatalf("datos del envío: %+v %v", previa, err)
	}
	if c := mandar(); c != http.StatusUnprocessableEntity || len(enviados) != 0 {
		t.Fatalf("un borrador no sale: %d", c)
	}

	if w := comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/factura_venta/"+id+"/estado",
		`{"estado":"confirmado"}`, params, http.HandlerFunc(hs.CambiarEstado)); w.Code != http.StatusOK {
		t.Fatalf("emitir: %d %s", w.Code, w.Body.String())
	}
	if c := mandar(); c != http.StatusOK || len(enviados) != 1 {
		t.Fatalf("envío: %d, %d correos", c, len(enviados))
	}
	e := enviados[0]
	if e.para != "facturas@cliente.es" || e.op.NombreRemitente != "Acme Ibérica SL" || e.op.ResponderA != "jefa@acme.es" {
		t.Fatalf("envío: %+v", e)
	}
	if e.adjunto.Tipo != "application/pdf" || !bytes.HasPrefix(e.adjunto.Datos, []byte("%PDF-")) || !strings.HasSuffix(e.adjunto.Nombre, ".pdf") {
		t.Fatalf("adjunto: %s %s", e.adjunto.Nombre, e.adjunto.Tipo)
	}
	hist, err := st.Historial(ctx, "factura_venta", id, 0)
	if err != nil {
		t.Fatal(err)
	}
	apuntado := false
	for _, h := range hist {
		apuntado = apuntado || strings.Contains(h.Texto, "Enviada por correo a facturas@cliente.es")
	}
	if !apuntado {
		t.Fatalf("no consta en el historial: %+v", hist)
	}

	w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+id+"/pdf", "", "", params, hs.PDFFactura)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("pdf: %d %v", w.Code, w.Header())
	}
}
