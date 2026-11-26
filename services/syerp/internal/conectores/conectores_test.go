// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conectores

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// firmar reproduce lo que hace Stripe al enviar el webhook.
func firmar(cuerpo []byte, secreto string, t time.Time) string {
	marca := fmt.Sprintf("%d", t.Unix())
	mac := hmac.New(sha256.New, []byte(secreto))
	mac.Write([]byte(marca + "."))
	mac.Write(cuerpo)
	return "t=" + marca + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// TestFirmaStripe cubre la única autenticación que tiene el webhook. Si esto se
// rompe, cualquiera puede dar facturas por cobradas.
func TestFirmaStripe(t *testing.T) {
	cuerpo := []byte(`{"id":"evt_1","type":"invoice.paid"}`)
	const secreto = "webhook_secret_for_tests"
	ahora := time.Now()

	if err := VerificarFirmaStripe(cuerpo, firmar(cuerpo, secreto, ahora), secreto, ahora); err != nil {
		t.Fatalf("firma legítima rechazada: %v", err)
	}

	casos := map[string]struct {
		cuerpo   []byte
		cabecera string
		secreto  string
		ahora    time.Time
		quiero   error
	}{
		"cuerpo alterado": {
			[]byte(`{"id":"evt_1","type":"invoice.paid","amount_paid":999999}`),
			firmar(cuerpo, secreto, ahora), secreto, ahora, ErrFirmaInvalida,
		},
		"secreto equivocado":   {cuerpo, firmar(cuerpo, secreto, ahora), "webhook_secret_other", ahora, ErrFirmaInvalida},
		"sin cabecera":         {cuerpo, "", secreto, ahora, ErrFirmaInvalida},
		"sin v1":               {cuerpo, "t=123", secreto, ahora, ErrFirmaInvalida},
		"firma no hexadecimal": {cuerpo, "t=" + fmt.Sprint(ahora.Unix()) + ",v1=zzzz", secreto, ahora, ErrFirmaInvalida},
		"repetición de ayer": {
			cuerpo, firmar(cuerpo, secreto, ahora.Add(-24*time.Hour)), secreto, ahora, ErrFirmaCaducada,
		},
		"marca en el futuro": {
			cuerpo, firmar(cuerpo, secreto, ahora.Add(time.Hour)), secreto, ahora, ErrFirmaCaducada,
		},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			err := VerificarFirmaStripe(c.cuerpo, c.cabecera, c.secreto, c.ahora)
			if !errors.Is(err, c.quiero) {
				t.Fatalf("error = %v, se esperaba %v", err, c.quiero)
			}
		})
	}
}

// TestFirmaAdmiteVariasDuranteLaRotacion: Stripe manda dos v1 mientras se rota el
// secreto. Aceptar solo el primero deja de cobrar justo durante la rotación.
func TestFirmaAdmiteVariasDuranteLaRotacion(t *testing.T) {
	cuerpo := []byte(`{"id":"evt_2"}`)
	ahora := time.Now()
	buena := firmar(cuerpo, "webhook_secret_new", ahora)
	// La cabecera lleva primero una firma de otro secreto y luego la buena.
	cabecera := buena[:len("t=1234567890")] + ",v1=" +
		hex.EncodeToString([]byte("firma que no cuadra en absoluto..")) + "," + buena[len("t=1234567890")+1:]
	if err := VerificarFirmaStripe(cuerpo, cabecera, "webhook_secret_new", ahora); err != nil {
		t.Fatalf("con dos firmas y una buena debe aceptar: %v", err)
	}
}

func TestLeerEventoStripe(t *testing.T) {
	cuerpo := []byte(`{
		"id":"evt_3","type":"invoice.paid",
		"data":{"object":{"id":"in_123","amount_paid":121000,"currency":"eur",
		                  "metadata":{"syerp_ref":"FV-2026-00001"}}}}`)
	c, err := LeerEventoStripe(cuerpo)
	if err != nil {
		t.Fatalf("LeerEventoStripe: %v", err)
	}
	if c.Centimos != 121000 || c.Moneda != "EUR" || c.RefDocumento != "FV-2026-00001" || c.ExternoID != "in_123" {
		t.Fatalf("cobro = %+v", c)
	}
}

func TestLeerEventoIgnoraLosQueNoCobran(t *testing.T) {
	_, err := LeerEventoStripe([]byte(`{"id":"evt_4","type":"customer.created"}`))
	if !errors.Is(err, ErrEventoIgnorado) {
		t.Fatalf("error = %v, se esperaba ErrEventoIgnorado", err)
	}
}

// TestCentimosDeDecimal es el motivo por el que esa función existe: con float64,
// 1234.56 se convierte en 123455.
func TestCentimosDeDecimal(t *testing.T) {
	casos := map[string]int64{
		"":           0,
		"0":          0,
		"1234.56":    123456,
		"1234.5":     123450,
		"1234":       123400,
		"-99.99":     -9999,
		"0.005":      1,      // redondeo comercial: medio céntimo sube
		"0.004":      0,      //
		"1234.565":   123457, // idem con arrastre
		"8.1":        810,
		"29.99":      2999,
		"1000000.00": 100000000,
	}
	for entrada, quiero := range casos {
		got, err := CentimosDeDecimal(entrada)
		if err != nil {
			t.Fatalf("%q: %v", entrada, err)
		}
		if got != quiero {
			t.Errorf("%q = %d, se esperaba %d", entrada, got, quiero)
		}
	}
	for _, malo := range []string{"12,34", "abc", "1.2.3", "12 34"} {
		if _, err := CentimosDeDecimal(malo); err == nil {
			t.Errorf("%q debería fallar", malo)
		}
	}
}

func TestTraducirSalesforce(t *testing.T) {
	mapeo, ok := MapeoDe(MapeosSalesforce, "Opportunity")
	if !ok {
		t.Fatal("falta el mapeo de Opportunity")
	}
	var tabla Tabla
	crudo := `{
		"columns":[{"name":"Id"},{"name":"Name"},{"name":"Amount"},
		           {"name":"Probability"},{"name":"CloseDate"},{"name":"StageName"}],
		"rows":[["006abc","Renovación Acme",12345.67,80,"2026-12-01","Closed Won"],
		        ["006def","Sin importe",null,10,"2026-11-01","Prospecting"]]}`
	if err := json.Unmarshal([]byte(crudo), &tabla); err != nil {
		t.Fatalf("tabla de prueba: %v", err)
	}
	filas, fallos := mapeo.Traducir(tabla)
	if len(fallos) != 0 {
		t.Fatalf("fallos inesperados: %v", fallos)
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d", len(filas))
	}
	if filas[0].Data["importe"] != int64(1234567) {
		t.Errorf("importe = %#v, se esperaba 1234567 céntimos", filas[0].Data["importe"])
	}
	if filas[0].Estado != "ganada" {
		t.Errorf("estado = %q, se esperaba ganada", filas[0].Estado)
	}
	if filas[0].ExternoID != "006abc" {
		t.Errorf("externo = %q", filas[0].ExternoID)
	}
	// Un importe nulo no inventa un cero: el campo simplemente no viaja.
	if _, hay := filas[1].Data["importe"]; hay {
		t.Error("un Amount nulo no debe guardarse como importe")
	}
}

// Una fila de Business Central y otra de Holded, tal como las entrega la
// pasarela, tienen que salir con importes en céntimos exactos y el estado
// traducido: es la diferencia entre importar el libro o descuadrarlo.
func TestTraducirBusinessCentralYHolded(t *testing.T) {
	casos := []struct {
		mapeos []Mapeo
		clave  string
		crudo  string
		estado string
		total  int64
	}{
		{MapeosBusinessCentral, "salesInvoices", `{
			"columns":[{"name":"id"},{"name":"invoiceDate"},{"name":"totalAmountIncludingTax"},{"name":"status"}],
			"rows":[["bc-1","2026-03-01",1493.82,"Paid"]]}`, "pagado", 149382},
		// Holded: el driver entrega el estado como texto; 2 es «pagada en parte».
		{MapeosHolded, "invoices", `{
			"columns":[{"name":"id"},{"name":"date"},{"name":"total"},{"name":"status"}],
			"rows":[["h-1","2026-01-01",12.1,"2"]]}`, "confirmado", 1210},
	}
	for _, c := range casos {
		mapeo, ok := MapeoDe(c.mapeos, c.clave)
		if !ok {
			t.Fatalf("falta el mapeo %s", c.clave)
		}
		var tabla Tabla
		if err := json.Unmarshal([]byte(c.crudo), &tabla); err != nil {
			t.Fatalf("tabla de prueba: %v", err)
		}
		filas, fallos := mapeo.Traducir(tabla)
		if len(fallos) != 0 || len(filas) != 1 {
			t.Fatalf("%s: filas=%d fallos=%v", c.clave, len(filas), fallos)
		}
		if f := filas[0]; f.Estado != c.estado || f.Data["total"] != c.total || f.Data["fecha"] == nil {
			t.Errorf("%s: %+v", c.clave, f)
		}
	}
	// Una misma entidad de BC alimenta dos cosas: el filtro va como $filter.
	for _, clave := range []string{"customers:empresas", "customers:personas", "items:bienes", "items:servicios"} {
		m, _ := MapeoDe(MapeosBusinessCentral, clave)
		if f, _ := m.Filtro.(string); f == "" {
			t.Errorf("%s sin filtro OData", clave)
		}
	}
}
