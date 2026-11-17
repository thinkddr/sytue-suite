// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"net/http"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conTarea registra una tarea de mentira para «fotos» y la quita al acabar.
func conTarea(t *testing.T, tarea TareaEnFondo) {
	t.Helper()
	RegistrarTareaEnFondo("fotos", tarea)
	t.Cleanup(func() {
		tareasMu.Lock()
		delete(tareas, "fotos")
		tareasMu.Unlock()
	})
}

var fotosSinVentana = SinVentana{Bundle: "eu.sytue.photos", Producto: "fotos", Zona: time.UTC}

// TestTareaSinVentanaConLaSesionGuardada: despertada por TareaFondo, sin `main` ni Suite,
// la tarea registrada del producto corre con la sesión guardada —que habla con los
// servicios con el token de esa sesión— y con la misma clave de cuenta que usa la app
// abierta. Sin tarea registrada, o con un paquete que no es de la suite, no corre nada.
func TestTareaSinVentanaConLaSesionGuardada(t *testing.T) {
	b := nuevoBancoSinVentana(t)
	guardarMovil(t, "ana")
	var vista EnFondo
	conTarea(t, func(ctx context.Context, f EnFondo) Resultado {
		vista = f
		resp, err := api.New(f.URL("photos"), f.Sesion, nil).Do(ctx, api.Request{Method: http.MethodGet, Path: "/x"})
		if err != nil {
			t.Errorf("la tarea no ha podido hablar con el servicio: %v", err)
			return Fallo
		}
		_ = resp.Body.Close()
		return Nuevos
	})
	if r := TareaSinVentana(t.Context(), fotosSinVentana); r != Nuevos {
		t.Fatalf("resultado %v", r)
	}
	if want := IdentidadDeCuenta(urlSinVentana("id"), "ana", ""); vista.Cuenta != want {
		t.Errorf("cuenta %q, quiero %q", vista.Cuenta, want)
	}
	b.mu.Lock()
	tokens := append([]string(nil), b.tokens...)
	b.mu.Unlock()
	if len(tokens) != 1 || tokens[0] != "Bearer acceso-ana" {
		t.Errorf("tokens enviados %q", tokens)
	}
	if r := TareaSinVentana(t.Context(), SinVentana{Bundle: "eu.sytue.photos", Producto: "otra"}); r != SinCambios {
		t.Errorf("sin tarea registrada: %v", r)
	}
	if r := TareaSinVentana(t.Context(), SinVentana{Bundle: "com.ajena.app", Producto: "fotos"}); r != SinCambios {
		t.Errorf("con un paquete ajeno: %v", r)
	}
}

// TestTareaSinVentanaSinSesion: sin nadie que haya entrado, la tarea no corre.
func TestTareaSinVentanaSinSesion(t *testing.T) {
	nuevoBancoSinVentana(t)
	corrio := false
	conTarea(t, func(context.Context, EnFondo) Resultado { corrio = true; return Nuevos })
	if r := TareaSinVentana(t.Context(), fotosSinVentana); r != SinCambios || corrio {
		t.Fatalf("sin sesión: %v, corrió %v", r, corrio)
	}
}
