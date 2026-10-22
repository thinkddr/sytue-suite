// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !darwin && !windows

package sesion

import (
	"context"
	"errors"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// TestElBloqueoSinVerificadorPideLaCuentaDeSytue: con el bloqueo puesto en un equipo
// que no sabe verificar al dueño (aquí, Linux), la app ni se queda cerrada ni abre la
// sesión guardada: abre sin sesión y su login pide la contraseña de Sytue (`max_age`)
// aunque el navegador ya tenga sesión con Sytue ID. Decisión del dueño del 2026-10-15.
// VISTO EN ROJO con el código anterior, que abría la sesión guardada sin preguntar
// (NAT-09). Solo fuera de macOS y Windows: allí el test enseñaría el diálogo de verdad.
// Y la bandeja no ofrece un bloqueo que este equipo no sabe cumplir.
func TestElBloqueoSinVerificadorPideLaCuentaDeSytue(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	primera := Abrir(a, "sytue.eu")
	primera.Sesion.Adoptar(auth.Tokens{Access: "a", Refresh: "r", Subject: "ada", Expiry: time.Now().Add(time.Hour)})
	if err := ajustes.Cambiar(func(x *ajustes.Ajustes) { x.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
	if !ajustes.Leer().Bloqueo {
		t.Fatal("el bloqueo no se ha guardado")
	}

	otra := Abrir(a, "sytue.eu") // con el bloqueo puesto
	if e := otra.Estado(); e.Dentro() {
		t.Fatalf("sin forma de verificar, la sesión guardada no se abre: %+v", e)
	}
	if otra.oidc.MaxAge <= 0 {
		t.Fatal("el login no pide volver a escribir la contraseña de Sytue")
	}
	if guardados, err := otra.almacen.Cargar(); err != nil || guardados.Refresh != "r" {
		t.Fatalf("la sesión guardada sigue ahí para cuando el dueño entre: %+v %v", guardados, err)
	}
	if otra.ElementoBloqueo(nil, fyne.NewMenu("x")) != nil {
		t.Error("la bandeja ofrece un bloqueo que este equipo no sabe cumplir")
	}
}

func TestReidentificacionNoSeSaltaConRefreshNiMarcaDeOtraApp(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	primera := Abrir(a, "sytue.eu")
	primera.Sesion.Adoptar(auth.Tokens{Refresh: "r", Subject: "ada"})
	if err := ajustes.Cambiar(func(x *ajustes.Ajustes) { x.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
	otra := Abrir(a, "sytue.eu")
	// Un refresh no puede abrir la cuenta antes de cumplir el bloqueo local.
	if _, err := otra.Sesion.AccessToken(context.Background()); !errors.Is(err, auth.ErrLoginRequired) {
		t.Fatalf("el refresh ha saltado la reidentificación: %v", err)
	}
	primera.Sesion.Adoptar(auth.Tokens{Refresh: "r2", Subject: "ada"})
	otra.aplicar(ajustes.Leer())
	if otra.Estado().Dentro() {
		t.Fatal("una marca de otra app ha saltado la reidentificación")
	}
	if guardados, _ := otra.almacen.Cargar(); guardados.Refresh != "r2" {
		t.Fatal("la espera de reidentificación ha borrado la sesión de las otras apps")
	}
}
