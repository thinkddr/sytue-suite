// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func TestNativeCryptoMessageLabelsDoNotImplyOldOrRecoveredContentIsVerified(t *testing.T) {
	sa, _ := salaDePrueba(t, &servicioFalso{})
	sa.Cifrado = &Cifrado{}
	legacy := sa.mensajeDe(api.RoomsMessage{Body: "old message", Encryption: "legacy"})
	if legacy.Cuerpo == "old message" {
		t.Fatal("legacy content was presented without encryption status")
	}
	recovered := sa.mensajeDe(api.RoomsMessage{Body: "recovered history", MatrixEvent: &api.RoomsMatrixEventRef{RoomID: "!room", EventID: "$event"}})
	if !strings.HasPrefix(recovered.Cuerpo, sa.P.T.T("matrix.senderUnverified", nil)) {
		t.Fatal("recovered history was presented as authenticated")
	}
}

func TestNativeLogoutDropsDecryptedViewsAndDrafts(t *testing.T) {
	sa, _ := salaDePrueba(t, &servicioFalso{})
	sa.mensajes = []api.RoomsMessage{{Body: "private content"}}
	sa.borradores = map[ambito]string{{canal: "channel"}: "private draft"}
	sa.P.Canal.Historial.Mensajes = []Mensaje{{Cuerpo: "private content"}}
	sa.P.Abrir(Canal{ID: "channel", Nombre: "Private"})
	sa.Olvidar()
	if len(sa.mensajes) != 0 || len(sa.borradores) != 0 || len(sa.P.Canal.Historial.Mensajes) != 0 || !sa.P.Canal.sinCanal {
		t.Fatal("logout retained decrypted message/draft state")
	}
}

func TestNativeChannelsLoadWithoutAwaitingMatrixVerification(t *testing.T) {
	srv := &servicioFalso{
		canales: []api.RoomsChannel{{ID: "c1", Name: "general", Kind: "public"}},
	}
	sa, _, _ := salaEn(t, srv, 800)
	// Matrix está configurado pero aún no verificado
	c := NuevoCifrado(sa, &api.Rooms{}, mx.Config{}, nil)
	sa.Cifrado = c
	sa.Arrancar(t.Context())

	if sa.P.Lateral.EstadoCanales != Listo {
		t.Fatalf("EstadoCanales = %v; se esperaba Listo", sa.P.Lateral.EstadoCanales)
	}
	if len(sa.P.Lateral.Canales) == 0 {
		t.Fatal("los canales no cargaron al arrancar la sala")
	}
	if sa.P.Lateral.Canales[0].Nombre != "general" {
		t.Fatalf("canal inesperado: %v", sa.P.Lateral.Canales[0].Nombre)
	}
}
