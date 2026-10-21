// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

// El callback de OAuth es la ÚNICA ruta del servicio que llega sin sesión: la
// invoca el navegador siguiendo un redirect del proveedor, así que no trae token
// ni organización. El tenant sale del estado sellado, y si ahí no viene, el canje
// tiene que pararse ANTES de tocar el store.
//
// Sin eso —que es como estaba— el contexto llegaba vacío al store y todo terminaba
// en «tenancy: no hay tenant en el contexto»: el consentimiento fallaba SIEMPRE,
// el navegador solo veía `oauth_error=exchange` y el motivo real se quedaba en el
// log del servicio.
//
// Ojo con el slug: un UUID de organización cumple la sintaxis entera —minúsculas,
// dígitos y guiones interiores—, así que `tenancy.New` lo aceptaría sin rechistar y
// la RLS no encontraría ni una fila. Lo que sostiene el flujo es el contrato del
// esquema: `org_id` es el SLUG (`0001_core.up.sql`) y las políticas lo comparan con
// `current_setting('app.tenant')`.
//
// El `Handlers` va vacío a propósito: si la comprobación del tenant se moviera
// después de la primera consulta, este test dejaría de pasar por nil pointer en vez
// de por el error esperado, que es exactamente la regresión que hay que impedir.
func TestElCanjeSeParaSiElEstadoSelladoNoTraeTenant(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/callback?code=x&state=y", http.NoBody)

	err := h.redeemAndStore(r, consent.Redeem{
		Tenant:   "", // lo que provoca el fallo
		Instance: "1f41ab34-9c25-4810-81af-ed884347af73",
		Code:     "codigo-de-autorizacion",
	})
	if err == nil {
		t.Fatal("sin tenant en el estado sellado, el canje debe fallar y no falló")
	}
	if !strings.Contains(err.Error(), "tenant") {
		t.Fatalf("el error debe nombrar el tenant para que se pueda diagnosticar; fue: %v", err)
	}
}
