// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func marcarConfirmando(t *testing.T, id Identidad) Estado {
	t.Helper()
	MarcarSesion()
	if err := MarcarIdentidad(id); err != nil {
		t.Fatal(err)
	}
	e := Leer()
	if e.Epoca == "" || e.EpocaConfirmada != e.Sesion {
		t.Fatalf("la época no queda confirmada con la marca: %+v", e)
	}
	return e
}

// TestLaEpocaSoloRotaSiCambiaQuien: renovar (otra marca, la misma identidad) no la
// rota; otro sujeto, otra organización o el cierre, sí. Una organización vacía con el
// mismo sujeto es un vacío transitorio y la conserva, también para el cambio siguiente.
func TestLaEpocaSoloRotaSiCambiaQuien(t *testing.T) {
	aparte(t)
	ana := Identidad{Emisor: "https://id.sytue.eu", Cliente: "sytue-desktop", Sujeto: "ana", Organizacion: "acme"}
	primera := marcarConfirmando(t, ana)
	renovada := marcarConfirmando(t, ana)
	if renovada.Epoca != primera.Epoca {
		t.Fatal("renovar la sesión ha rotado la época")
	}
	if renovada.Sesion == primera.Sesion {
		t.Fatal("la prueba necesita una marca de sesión nueva")
	}
	sinOrg := ana
	sinOrg.Organizacion = ""
	if e := marcarConfirmando(t, sinOrg); e.Epoca != primera.Epoca {
		t.Fatal("una organización vacía con el mismo sujeto ha rotado la época")
	}
	if e := marcarConfirmando(t, ana); e.Epoca != primera.Epoca {
		t.Fatal("volver a la misma organización tras el vacío ha rotado la época")
	}
	otraOrg := ana
	otraOrg.Organizacion = "globex"
	segunda := marcarConfirmando(t, otraOrg)
	if segunda.Epoca == primera.Epoca {
		t.Fatal("cambiar de organización no ha rotado la época")
	}
	otro := otraOrg
	otro.Sujeto = "bea"
	tercera := marcarConfirmando(t, otro)
	if tercera.Epoca == segunda.Epoca {
		t.Fatal("cambiar de persona no ha rotado la época")
	}
	otroCliente := otro
	otroCliente.Cliente = "sytue-mobile"
	if e := marcarConfirmando(t, otroCliente); e.Epoca == tercera.Epoca {
		t.Fatal("cambiar de cliente OIDC no ha rotado la época")
	}
	cierre := marcarConfirmando(t, Identidad{})
	otroCierre := marcarConfirmando(t, Identidad{})
	if cierre.Epoca == otroCierre.Epoca {
		t.Fatal("el cierre no rota la época siempre")
	}
}

// TestLaEpocaNoGuardaElSujetoEnClaro: la huella lleva la época como sal.
func TestLaEpocaNoGuardaElSujetoEnClaro(t *testing.T) {
	dir := aparte(t)
	marcarConfirmando(t, Identidad{Emisor: "https://id.example", Cliente: "c", Sujeto: "persona-secreta-42", Organizacion: "org-secreta"})
	crudo, err := os.ReadFile(filepath.Join(dir, ficheroEpoca))
	if err != nil {
		t.Fatal(err)
	}
	for _, secreto := range []string{"persona-secreta-42", "org-secreta", "id.example"} {
		if strings.Contains(string(crudo), secreto) {
			t.Fatalf("sesion.epoca deja leer %q", secreto)
		}
	}
}

// TestUnaMarcaNuevaSinEpocaQuedaSinConfirmar: una compilación que solo conoce la marca
// (la de antes de la época) cambia la sesión y la época no la sigue.
func TestUnaMarcaNuevaSinEpocaQuedaSinConfirmar(t *testing.T) {
	aparte(t)
	marcarConfirmando(t, Identidad{Sujeto: "ana"})
	MarcarSesion()
	if e := Leer(); e.EpocaConfirmada == e.Sesion {
		t.Fatal("una marca escrita sin época aparece confirmada")
	}
}

// TestMarcarIdentidadNoSeBloqueaConElCerrojoTomado: auth la llama dentro de
// CerrojoSesion. Si la función volviera a tomarlo, el proceso se esperaría a sí mismo
// hasta que venciera el plazo de quien llama.
func TestMarcarIdentidadNoSeBloqueaConElCerrojoTomado(t *testing.T) {
	aparte(t)
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Second)
	defer parar()
	soltar, err := CerrojoSesion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	hecho := make(chan error, 1)
	go func() { hecho <- MarcarIdentidad(Identidad{Sujeto: "ana"}) }()
	select {
	case err := <-hecho:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("MarcarIdentidad espera al cerrojo que ya tiene quien la llama")
	}
}
