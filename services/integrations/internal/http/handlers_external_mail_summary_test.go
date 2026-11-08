// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/mailbox"
)

// El resumen lista las cuentas que Mail abre: sin borradores, sin las conectadas
// solo para Calendario y de la más antigua a la más nueva, como `accounts`.
//
// VISTO EN ROJO (2026-11-06): sin el filtro de `mailServesMail`, la cuenta solo de
// Calendario salía en el resumen de Mail.
func TestElResumenDeCorreoListaSoloBuzones(t *testing.T) {
	now := time.Now()
	instances := []domain.AppInstance{
		borradorDeCorreo("borrador", "gmail-mail", now),
		cuentaDeCorreo("calendario", "gmail-mail", "c@example.eu", []string{"calendar"}, now.Add(-time.Minute)),
		cuentaDeCorreo("segunda", "mail-imap", "b@example.eu", nil, now.Add(-time.Hour)),
		cuentaDeCorreo("primera", "gmail-mail", "a@example.eu", []string{"mail", "calendar"}, now.Add(-2*time.Hour)),
	}
	accounts := mailSummaryAccounts(instances)
	got := make([]string, 0, len(accounts))
	for _, inst := range accounts {
		got = append(got, inst.ID)
	}
	if !reflect.DeepEqual(got, []string{"primera", "segunda"}) {
		t.Fatalf("cuentas del resumen = %v; quiero los buzones, de la más antigua a la más nueva", got)
	}
}

// Una cuenta que pide volver a iniciar sesión y otra cuyo proveedor no contesta no
// se llevan por delante a las demás: cada una dice su estado y el orden se queda.
//
// VISTO EN ROJO (2026-11-06): con `mailSummaryState` devolviendo siempre
// «unavailable», la cuenta caducada no pedía volver a iniciar sesión; sin el plazo
// de `mailFoldersWithin`, la cuenta colgada retenía el resumen entero.
func TestElResumenDeCorreoAguantaCuentasCaidas(t *testing.T) {
	now := time.Now()
	desconectada := cuentaDeCorreo("desconectada", "gmail-mail", "d@example.eu", nil, now)
	desconectada.Status = domain.StatusError
	accounts := []domain.AppInstance{
		cuentaDeCorreo("buena", "gmail-mail", "a@example.eu", nil, now),
		cuentaDeCorreo("caducada", "microsoft-mail", "b@example.eu", nil, now),
		cuentaDeCorreo("reconectar", "gmail-mail", "r@example.eu", nil, now),
		cuentaDeCorreo("colgada", "mail-imap", "c@example.eu", nil, now),
		cuentaDeCorreo("caida", "mail-imap", "e@example.eu", nil, now),
		desconectada,
	}
	colgada := make(chan struct{})
	defer close(colgada)
	var preguntadas atomic.Int32
	folders := func(ctx context.Context, inst domain.AppInstance) ([]mailbox.Folder, error) {
		preguntadas.Add(1)
		switch inst.ID {
		case "buena":
			return []mailbox.Folder{{ID: "INBOX", Name: "INBOX", Role: "inbox", Unread: 3}}, nil
		case "caducada":
			return nil, mailbox.ErrAuth
		case "reconectar":
			return nil, httpx.ErrConflict.WithCode("external_reconnect_required")
		case "colgada":
			<-colgada // no mira el contexto, como un cliente atascado en una lectura
			return nil, nil
		default:
			return nil, errors.New("proveedor caído: respuesta cruda con un token-secreto")
		}
	}
	start := time.Now()
	items := summarizeMail(context.Background(), accounts, 2, 50*time.Millisecond, folders)
	if time.Since(start) > 2*time.Second {
		t.Fatalf("el resumen esperó a la cuenta colgada: %s", time.Since(start))
	}
	want := map[string]string{"buena": "ok", "caducada": "reconnect", "reconectar": "reconnect", "colgada": "unavailable", "caida": "unavailable", "desconectada": "reconnect"}
	for i, item := range items {
		id := item["instance_id"].(string)
		if id != accounts[i].ID {
			t.Fatalf("puesto %d es %q; el resumen debe conservar el orden", i, id)
		}
		if item["state"] != want[id] {
			t.Errorf("%s: estado %v; quiero %s", id, item["state"], want[id])
		}
	}
	if got := items[0]["folders"].([]mailbox.Folder); len(got) != 1 || got[0].Unread != 3 {
		t.Fatalf("carpetas de la buena = %v", got)
	}
	if got := items[1]["folders"].([]mailbox.Folder); got == nil || len(got) != 0 {
		t.Fatalf("una cuenta caída lleva carpetas vacías, no nulas: %v", got)
	}
	if n := preguntadas.Load(); n != 5 {
		t.Fatalf("se preguntó a %d proveedores; la desconectada no se pregunta", n)
	}
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), "token-secreto") {
		t.Fatalf("el resumen deja salir la respuesta del proveedor: %s", raw)
	}
}

// No se pregunta a más proveedores a la vez de los que dice `parallel`.
//
// VISTO EN ROJO (2026-11-06): sin el semáforo `slots`, las seis a la vez.
func TestElResumenDeCorreoNoPreguntaATodosALaVez(t *testing.T) {
	now := time.Now()
	ids := []string{"a", "b", "c", "d", "e", "f"}
	accounts := make([]domain.AppInstance, 0, len(ids))
	for _, id := range ids {
		accounts = append(accounts, cuentaDeCorreo(id, "mail-imap", id+"@example.eu", nil, now))
	}
	var dentro, maximo atomic.Int32
	summarizeMail(context.Background(), accounts, 2, time.Second, func(context.Context, domain.AppInstance) ([]mailbox.Folder, error) {
		n := dentro.Add(1)
		for {
			m := maximo.Load()
			if n <= m || maximo.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		dentro.Add(-1)
		return nil, nil
	})
	if maximo.Load() > 2 {
		t.Fatalf("%d proveedores a la vez; el tope es 2", maximo.Load())
	}
}
