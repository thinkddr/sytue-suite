// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/apps/erp"
	localerp "git.sytue.eu/sytue/one/apps/nucleo/local/erp"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// «Comprobar en VIES» de Sytue Office contra un VIES de mentira: lo contestado llega a la
// pantalla tal cual, y cada fallo, con el que ella sabe decir.
func TestComprobarEnVIESDesdeLaEdicion(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch string(cuerpo) {
		case `{"countryCode":"FR","vatNumber":"12345678901"}`:
			_, _ = io.WriteString(w, `{"countryCode":"FR","vatNumber":"12345678901","valid":true,"name":"EXEMPLE SARL","address":"1 RUE DE L’EXEMPLE\n75002 PARIS"}`)
		case `{"countryCode":"DE","vatNumber":"123456789"}`:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"actionSucceed":false,"errorWrappers":[{"error":"MS_UNAVAILABLE"}]}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"actionSucceed":false,"errorWrappers":[{"error":"INVALID_INPUT"}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	falso := comprobarVIESCon(func() localerp.VIES { return localerp.VIES{HTTP: srv.Client(), URL: srv.URL} })

	r, err := falso(ctx, "FR 12345678901", "")
	if err != nil || r != (erp.ComprobacionVIES{Pais: "FR", Numero: "12345678901", Valido: true, Nombre: "EXEMPLE SARL", Direccion: "1 RUE DE L’EXEMPLE, 75002 PARIS"}) {
		t.Fatalf("válido: %+v %v", r, err)
	}
	for nif, quiero := range map[string]erp.FalloVIES{
		"DE123456789": erp.VIESNoResponde,
		"PT999999999": erp.VIESNoEsNIFIVA,
		"B12345678":   erp.VIESNoEsNIFIVA, // sin prefijo ni país: ni se pregunta
	} {
		if _, err := falso(ctx, nif, ""); !errors.Is(err, quiero) {
			t.Errorf("%s: %v; quiero %v", nif, err, quiero)
		}
	}

	// Por el portero, como en la app: el permiso puntual no deja ir a este equipo, y con
	// «Sin conexión» no sale; la pantalla dice que no se llegó.
	p := red.Abrir("")
	portero := comprobarVIESCon(func() localerp.VIES { return localerp.VIES{HTTP: p.Puntual(red.VIES, ""), URL: srv.URL} })
	if _, err := portero(ctx, "FR12345678901", ""); !errors.Is(err, erp.VIESSinRed) {
		t.Errorf("a este equipo por el portero: %v; quiero VIESSinRed", err)
	}
	if err := p.PonerSinConexion(true); err != nil {
		t.Fatal(err)
	}
	if _, err := portero(ctx, "FR12345678901", ""); !errors.Is(err, erp.VIESSinRed) {
		t.Errorf("con «Sin conexión»: %v; quiero VIESSinRed", err)
	}
	if ds := p.Destinos(); len(ds) != 0 {
		t.Errorf("acabada la consulta, el portero sigue abierto: %+v", ds)
	}

	for err, quiero := range map[error]erp.FalloVIES{
		&net.OpError{Op: "dial", Err: errors.New("connection refused")}: erp.VIESSinRed,
		&net.DNSError{Err: "no such host", Name: "vies.example"}:        erp.VIESSinRed,
		context.DeadlineExceeded:                                        erp.VIESNoResponde,
		errors.New("otra cosa"):                                         erp.VIESNoResponde,
	} {
		if got := falloVIES(err); !errors.Is(got, quiero) {
			t.Errorf("%v: %v; quiero %v", err, got, quiero)
		}
	}
}
