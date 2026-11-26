// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	cuentas "git.sytue.eu/sytue/one/apps/fyne/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	localerp "git.sytue.eu/sytue/one/apps/nucleo/local/erp"
)

// abridorDoble apunta lo que se le da al sistema, o falla.
type abridorDoble struct {
	abiertos []*url.URL
	fallo    error
}

func (a *abridorDoble) OpenURL(u *url.URL) error {
	if a.fallo != nil {
		return a.fallo
	}
	a.abiertos = append(a.abiertos, u)
	return nil
}

// De punta a punta, «Abrir en el correo» del ERP: el PDF de una factura del ERP local se
// deja en «para-enviar», Mail recibe el enlace y su redactor lo tiene adjunto, igual,
// con su nombre; la carpeta queda vacía y el mismo enlace no vuelve a adjuntar nada.
// Sin Mail, sin carpeta compartida (Android) o si no se abre, no queda nada dejado y la
// ficha hace lo de siempre.
func TestAbrirEnElCorreoLlevaElPDFAMail(t *testing.T) {
	ctx := context.Background()
	cat, obl, err := localerp.Cargar()
	if err != nil {
		t.Fatal(err)
	}
	alm, err := localerp.Abrir(t.TempDir(), cat)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = alm.Cerrar() })
	e := api.NewERP(localerp.Base, nil, localerp.NuevoServidor(cat, obl, alm).Cliente(), "es")
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Acme", NIF: "B12345678", Pais: "ES", Forma: "sl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CambiarModulo(ctx, "facturacion", true); err != nil {
		t.Fatal(err)
	}
	cli, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Cliente", "tipo": "persona", "correo": "pagos@cliente.es"})
	if err != nil {
		t.Fatal(err)
	}
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-05-04", "base": 1000, "iva": 210, "total": 1210})
	if err != nil {
		t.Fatal(err)
	}
	// Lo que hace la ficha (redactarFuera): el PDF del servicio y el mensaje del diálogo.
	cuerpo, nombre, err := e.PDFFactura(ctx, "factura_venta", fac.ID)
	if err != nil {
		t.Fatal(err)
	}
	pdf, _ := io.ReadAll(cuerpo)
	_ = cuerpo.Close()
	envio, err := e.EnvioFactura(ctx, "factura_venta", fac.ID)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) || nombre != "FV-2026-00001.pdf" {
		t.Fatalf("factura: %q %q %v", nombre, pdf[:min(len(pdf), 16)], err)
	}

	salida := correo.ParaEnviar{Dir: filepath.Join(t.TempDir(), "para-enviar")}
	a := &abridorDoble{}
	if err := redactarEnMail(a, true, salida, envio.Para, envio.Asunto, envio.Mensaje, nombre, pdf); err != nil {
		t.Fatal(err)
	}
	if len(a.abiertos) != 1 || a.abiertos[0].Scheme != "sytueoffice-mail" {
		t.Fatalf("abierto: %v", a.abiertos)
	}
	// Lo que llega a Mail.
	r, ok := cuentas.LeerEnlace(a.abiertos[0], salida)
	if !ok || r.Para != "pagos@cliente.es" || r.Asunto != envio.Asunto || r.Asunto == "" || r.Cuerpo != envio.Mensaje || r.SinAdjuntar != 0 {
		t.Fatalf("redactar: %+v", r)
	}
	if len(r.Adjuntos) != 1 || r.Adjuntos[0].Nombre != "FV-2026-00001.pdf" || r.Adjuntos[0].Tipo != "application/pdf" || !bytes.Equal(r.Adjuntos[0].Datos, pdf) {
		t.Fatalf("adjuntos: %d", len(r.Adjuntos))
	}
	if es, _ := os.ReadDir(salida.Dir); len(es) != 0 {
		t.Errorf("quedó en la carpeta: %v", es)
	}
	if r, _ := cuentas.LeerEnlace(a.abiertos[0], salida); len(r.Adjuntos) != 0 || r.SinAdjuntar != 1 {
		t.Errorf("el enlace volvió a adjuntar: %+v", r)
	}

	// Sin Mail, sin carpeta, o el sistema no lo abre: error y nada dejado.
	for nombreCaso, caso := range map[string]struct {
		hay    bool
		salida correo.ParaEnviar
		a      *abridorDoble
	}{
		"sin Mail":    {false, salida, &abridorDoble{}},
		"sin carpeta": {true, correo.ParaEnviar{}, &abridorDoble{}},
		"no se abre":  {true, salida, &abridorDoble{fallo: errors.New("no")}},
	} {
		if err := redactarEnMail(caso.a, caso.hay, caso.salida, "a@b.eu", "x", "", nombre, pdf); err == nil || len(caso.a.abiertos) != 0 {
			t.Errorf("%s: %v %v", nombreCaso, err, caso.a.abiertos)
		}
		if es, _ := os.ReadDir(salida.Dir); len(es) != 0 {
			t.Errorf("%s: quedó en la carpeta: %v", nombreCaso, es)
		}
	}
}
