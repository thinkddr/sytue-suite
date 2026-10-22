// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de buscar y reemplazar. Lo que hay que comprobar es que el cambio
// LLEGUE AL DOCUMENTO y que se avise cuando el texto nuevo no cabe: un PDF no
// tiene reflujo, así que un reemplazo largo se aprieta hasta que se nota, y
// ofrecerlo sin decirlo estropea el documento de quien lo usa.
package domain

import (
	"strings"
	"testing"
)

func TestReemplazarCambiaTodasLasPaginas(t *testing.T) {
	d := docDePrueba(t, "contrato con Acme", "anexo de Acme", "sin nada")

	coincidencias, err := d.BuscarParaReemplazar(BusquedaReemplazo{Buscar: "Acme", Reemplazar: "Sytue"})
	if err != nil {
		t.Fatalf("BuscarParaReemplazar: %v", err)
	}
	if len(coincidencias) != 2 {
		t.Fatalf("coincidencias = %+v, quería 2 (páginas 1 y 2)", coincidencias)
	}

	salida, n, err := ReemplazarEnDocumentoPDF(d, BusquedaReemplazo{Buscar: "Acme", Reemplazar: "Sytue"}, nil)
	if err != nil {
		t.Fatalf("ReemplazarEnDocumentoPDF: %v", err)
	}
	if n != 2 {
		t.Errorf("cambiados = %d, quería 2", n)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el documento reemplazado no abre: %v", err)
	}
	for i := 0; i < 2; i++ {
		texto, _ := d2.TextoPlano(i)
		if strings.Contains(texto, "Acme") {
			t.Errorf("página %d sigue diciendo Acme: %q", i+1, texto)
		}
		if !strings.Contains(texto, "Sytue") {
			t.Errorf("página %d no dice Sytue: %q", i+1, texto)
		}
	}
}

// TestBuscarAvisaDeLoQueNoCabe es el check que separa esto de un procesador de
// textos: sin reflujo, un reemplazo largo aprieta la línea hasta que se nota.
func TestBuscarAvisaDeLoQueNoCabe(t *testing.T) {
	d := docDePrueba(t, "Acme")

	// Mismo número de caracteres: cabe sin discusión. El caso anterior era
	// «Acme»→«Sytue», que parece inofensivo pero crece un 25 % —en una palabra de
	// cuatro letras, una más es mucho— y el motor lo marcaba como apretado con
	// razón: el umbral es del 12 % (encogidoQueSeNota). La expectativa del test
	// era la equivocada, no el cálculo.
	corto, err := d.BuscarParaReemplazar(BusquedaReemplazo{Buscar: "Acme", Reemplazar: "Nova"})
	if err != nil {
		t.Fatalf("BuscarParaReemplazar: %v", err)
	}
	if len(corto) != 1 || !corto[0].Cabe {
		t.Errorf("«Acme»→«Nova» ocupa lo mismo y tiene que caber: %+v", corto)
	}

	largo, err := d.BuscarParaReemplazar(BusquedaReemplazo{
		Buscar: "Acme", Reemplazar: "Corporación Industrial Acme y Asociados",
	})
	if err != nil {
		t.Fatalf("BuscarParaReemplazar: %v", err)
	}
	if len(largo) != 1 {
		t.Fatalf("coincidencias = %+v", largo)
	}
	if largo[0].Cabe {
		t.Error("un reemplazo diez veces más largo NO cabe: hay que avisarlo antes de aplicarlo")
	}
}

// TestBuscarNoDistingueMayusculasPorDefecto: quien busca el nombre de una
// empresa lo encuentra escrito de tres formas en el mismo pliego.
func TestBuscarNoDistingueMayusculasPorDefecto(t *testing.T) {
	d := docDePrueba(t, "ACME y acme y Acme")

	c, err := d.BuscarParaReemplazar(BusquedaReemplazo{Buscar: "acme", Reemplazar: "Sytue"})
	if err != nil {
		t.Fatalf("BuscarParaReemplazar: %v", err)
	}
	if len(c) != 1 {
		t.Fatalf("coincidencias = %+v", c)
	}
	if strings.Contains(strings.ToLower(c[0].Despues), "acme") {
		t.Errorf("no se sustituyeron las tres formas: %q", c[0].Despues)
	}

	// Y con la opción activada, solo la que casa exactamente.
	c2, err := d.BuscarParaReemplazar(BusquedaReemplazo{
		Buscar: "acme", Reemplazar: "Sytue", DistinguirMayusculas: true,
	})
	if err != nil {
		t.Fatalf("BuscarParaReemplazar: %v", err)
	}
	if len(c2) != 1 || !strings.Contains(c2[0].Despues, "ACME") {
		t.Errorf("distinguiendo mayúsculas solo cambia «acme»: %+v", c2)
	}
}

// TestSustituirConservaLoQueHayAlrededor: el fallo fácil es pasar el fragmento
// a minúsculas para comparar y devolverlo así, perdiendo el resto de la línea.
func TestSustituirConservaLoQueHayAlrededor(t *testing.T) {
	got, hay := sustituir("El contrato de ACME S.L. con fecha", "acme", "Sytue", false)
	if !hay {
		t.Fatal("debería casar sin distinguir mayúsculas")
	}
	if got != "El contrato de Sytue S.L. con fecha" {
		t.Errorf("resultado = %q", got)
	}
}

// TestReemplazarSoloLoElegido: la lista se enseña para poder descartar las que
// no interesan; si aplicara todas daría igual enseñarla.
func TestReemplazarSoloLoElegido(t *testing.T) {
	d := docDePrueba(t, "Acme uno", "Acme dos")

	todas, err := d.BuscarParaReemplazar(BusquedaReemplazo{Buscar: "Acme", Reemplazar: "Sytue"})
	if err != nil || len(todas) != 2 {
		t.Fatalf("preparación: %v %+v", err, todas)
	}
	salida, n, err := ReemplazarEnDocumentoPDF(d,
		BusquedaReemplazo{Buscar: "Acme", Reemplazar: "Sytue"}, todas[:1])
	if err != nil {
		t.Fatalf("ReemplazarEnDocumentoPDF: %v", err)
	}
	if n != 1 {
		t.Fatalf("cambiados = %d, quería solo 1", n)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	segunda, _ := d2.TextoPlano(1)
	if !strings.Contains(segunda, "Acme") {
		t.Errorf("la página que NO se eligió también cambió: %q", segunda)
	}
}

func TestBuscarRechazaLoImposible(t *testing.T) {
	d := docDePrueba(t, "texto")
	if _, err := d.BuscarParaReemplazar(BusquedaReemplazo{Buscar: "  "}); err == nil {
		t.Error("sin texto que buscar debería fallar")
	}
	if _, err := d.BuscarParaReemplazar(BusquedaReemplazo{Buscar: "x", SoloPaginas: []int{9}}); err == nil {
		t.Error("una página que no existe debería fallar")
	}
	if _, _, err := ReemplazarEnDocumentoPDF(d,
		BusquedaReemplazo{Buscar: "no-esta-en-el-documento", Reemplazar: "x"}, nil); err == nil {
		t.Error("reemplazar algo que no está debería decirlo")
	}
}
