// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestHostnameDeFuncionCabeEnUnComodin es la razón de ser del aplanado: si
// vuelve a haber más de un segmento bajo `fn.`, ningún certificado comodín lo
// cubre —ACME no emite comodines de varios niveles— y cada función vuelve a
// pedir el suyo, publicando su nombre en los registros públicos de Certificate
// Transparency.
func TestHostnameDeFuncionCabeEnUnComodin(t *testing.T) {
	const dominio = "apps.sytue.eu"
	h := HostnameDeFuncion(uuid.New(), "mi-funcion", dominio)

	sufijo := ".fn." + dominio
	if !strings.HasSuffix(h, sufijo) {
		t.Fatalf("hostname %q no termina en %q", h, sufijo)
	}
	etiqueta := strings.TrimSuffix(h, sufijo)
	if strings.Contains(etiqueta, ".") {
		t.Fatalf("bajo `fn.` tiene que haber UN solo segmento para que *%s lo cubra; hay %q",
			sufijo, etiqueta)
	}
}

// TestHostnameDeFuncionNoColisiona: dos pares (proyecto, función) distintos no
// pueden dar el mismo hostname. Si lo dieran, una petición acabaría en la
// función de otro cliente, que es un fallo de aislamiento y no una molestia.
//
// El caso interesante es el que motiva poner el UUID delante: con la función
// primero, `a-b` + `<uuid>` y `a` + `b-<uuid>` producirían la misma cadena.
func TestHostnameDeFuncionNoColisiona(t *testing.T) {
	p1 := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	p2 := uuid.MustParse("11111111-2222-3333-4444-555555555556")
	const dom = "apps.sytue.eu"

	vistos := map[string]string{}
	for _, c := range []struct {
		proyecto uuid.UUID
		funcion  string
	}{
		{p1, "api"},
		{p1, "api-v2"},
		{p2, "api"},
		{p2, "api-v2"},
		{p1, "a-b"},
		{p1, "a"},
	} {
		h := HostnameDeFuncion(c.proyecto, c.funcion, dom)
		if antes, dup := vistos[h]; dup {
			t.Fatalf("colisión: %q lo generan tanto %s como %s/%s", h, antes, c.proyecto, c.funcion)
		}
		vistos[h] = c.proyecto.String() + "/" + c.funcion
	}
}

// TestHostnameDeFuncionEsUnaEtiquetaDNSValida: 63 caracteres es el máximo de una
// etiqueta DNS. El UUID ya gasta 36 más el guion, así que el margen para el
// nombre de la función es de 26: conviene que falle aquí y no al crear el
// Ingress.
func TestHostnameDeFuncionEsUnaEtiquetaDNSValida(t *testing.T) {
	h := HostnameDeFuncion(uuid.New(), strings.Repeat("x", 26), "apps.sytue.eu")
	etiqueta := strings.SplitN(h, ".", 2)[0]
	if len(etiqueta) > 63 {
		t.Fatalf("la etiqueta mide %d caracteres, el máximo DNS es 63: %q", len(etiqueta), etiqueta)
	}
}
