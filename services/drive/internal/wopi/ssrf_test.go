// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package wopi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// La dirección del editor que escribe una ORGANIZACIÓN es entrada de fuera: sin
// TLS, o hacia una IP que no es pública, el catálogo no se descarga. Los
// servidores de este test son locales (loopback) y responden un catálogo válido:
// si se llega a pedir, el test lo ve.
func TestElEditorDeUnaOrganizacionNoSaleHaciaDentro(t *testing.T) {
	pedido := false
	contestar := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pedido = true
		_, _ = w.Write([]byte(`<wopi-discovery><net-zone><app><action name="edit" ext="docx" urlsrc="https://editor.example/x?"/></app></net-zone></wopi-discovery>`))
	})
	enClaro := httptest.NewServer(contestar)
	defer enClaro.Close()
	conTLS := httptest.NewTLSServer(contestar)
	defer conTLS.Close()

	for nombre, dir := range map[string]string{
		"sin TLS":                enClaro.URL,
		"https a una IP local":   conTLS.URL,
		"https a la red interna": "https://10.0.0.5/hosting/discovery",
		"otro esquema":           "file:///etc/hostname",
	} {
		if exts, err := Probe(context.Background(), dir, time.Minute); !errors.Is(err, ErrDireccionDelEditor) {
			t.Errorf("%s (%s): %v %v, esperaba ErrDireccionDelEditor", nombre, dir, exts, err)
		}
	}
	if pedido {
		t.Error("se llegó a pedir el catálogo a un servidor local")
	}
	// La del despliegue (WOPI_DISCOVERY_URL) sí puede ser interna y sin TLS, y el
	// registro solo trata así a ESA dirección.
	if exts := NewDiscoveryDeConfianza(enClaro.URL, time.Minute).Extensions(context.Background()); len(exts) != 1 {
		t.Errorf("el catálogo del despliegue no se leyó: %v", exts)
	}
	reg := NewRegistry(func(_ context.Context, tenant string) (string, bool) {
		if tenant == "propia" {
			return conTLS.URL, true
		}
		return enClaro.URL, true
	}, time.Minute)
	reg.Confiar(enClaro.URL)
	if exts := reg.For(context.Background(), "cualquiera").Extensions(context.Background()); len(exts) != 1 {
		t.Errorf("el registro no lee el catálogo del despliegue: %v", exts)
	}
	if exts := reg.For(context.Background(), "propia").Extensions(context.Background()); len(exts) != 0 {
		t.Errorf("el registro leyó el catálogo local de una organización: %v", exts)
	}
}
