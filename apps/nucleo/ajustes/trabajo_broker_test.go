// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func brokerFixture() (pedidoTrabajoBroker, AccesoSesion) {
	access := AccesoSesion{Version: 1, Identidad: strings.Repeat("a", 64), Revision: strings.Repeat("1", 32), Activo: true}
	scope := storage.Scope{Tenant: "acme", Subject: "ana", Product: "notes"}
	return pedidoTrabajoBroker{Version: 1, Cuenta: "sesion-" + strings.Repeat("2", 32), Acceso: &access, Ambito: &scope}, access
}

func brokerResponse(t *testing.T, accion string, p pedidoTrabajoBroker, access AccesoSesion, work *registroTrabajoBroker) string {
	t.Helper()
	raw, err := json.Marshal(access)
	if err != nil {
		t.Fatal(err)
	}
	cabecera := ""
	if p.Ambito != nil {
		var err error
		cabecera, err = storage.EncodeScopeHeader(*p.Ambito)
		if err != nil {
			t.Fatal(err)
		}
	}
	body, err := json.Marshal(respuestaTrabajoBroker{Version: 1, Cuenta: p.Cuenta, Accion: accion, Acceso: raw, Ambito: p.Ambito, Cabecera: cabecera, Operacion: strings.Repeat("3", 32), Permiso: strings.Repeat("4", 32), Trabajo: work})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func readyWorkBroker(reply func(context.Context, string, string) (string, error)) clienteTrabajoBroker {
	return clienteTrabajoBroker{disponible: func() (bool, error) { return true, nil }, pedir: reply}
}

func TestWorkBrokerDistinguishesKnownOldFromNewFailureWithoutFallback(t *testing.T) {
	failure := errors.New("broker unavailable")
	for _, tc := range []struct {
		name      string
		available bool
		failure   error
		native    bool
	}{{"known-old", false, nil, false}, {"new-failed", false, failure, true}, {"new-available", true, nil, true}} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := brokerFixture()
			p.Acceso = nil
			p.Ambito = nil
			calls := 0
			client := clienteTrabajoBroker{disponible: func() (bool, error) { return tc.available, tc.failure }, pedir: func(context.Context, string, string) (string, error) { calls++; return "{}", nil }}
			_, native, err := client.aplicar(t.Context(), "leer", p)
			if native != tc.native || (!tc.available && calls != 0) || (tc.available && calls != 1) {
				t.Fatal("incorrect broker/file routing", native, calls, err)
			}
			if tc.failure != nil && !errors.Is(err, failure) {
				t.Fatal("new broker failure became an old-version fallback", err)
			}
			if tc.available && !errors.Is(err, ErrProtocoloTrabajoSesion) {
				t.Fatal("malformed new response became legacy fallback", err)
			}
		})
	}
}

func TestWorkBrokerValidatesBeforeCapabilityOrTransportIO(t *testing.T) {
	for _, change := range []func(*pedidoTrabajoBroker){func(p *pedidoTrabajoBroker) { p.Cuenta = "../another-account" }, func(p *pedidoTrabajoBroker) { p.Version = 2 }, func(p *pedidoTrabajoBroker) { p.Ambito.Tenant = "" }, func(p *pedidoTrabajoBroker) { p.Ambito = nil }, func(p *pedidoTrabajoBroker) { p.Acceso.Revision = "bad" }} {
		p, _ := brokerFixture()
		change(&p)
		calls := 0
		client := clienteTrabajoBroker{disponible: func() (bool, error) { calls++; return true, nil }, pedir: func(context.Context, string, string) (string, error) { calls++; return "", nil }}
		if _, native, err := client.aplicar(t.Context(), "iniciar", p); !native || !errors.Is(err, ErrProtocoloTrabajoSesion) || calls != 0 {
			t.Fatal("invalid work routing performed native IO", calls, err)
		}
	}
}

func TestWorkBrokerRejectsWrongAccountScopeNonceOrVersion(t *testing.T) {
	for _, kind := range []string{"account", "scope", "scope-header", "nonce", "version", "action", "trailing", "unknown-field"} {
		t.Run(kind, func(t *testing.T) {
			p, access := brokerFixture()
			body := brokerResponse(t, "iniciar", p, access, &registroTrabajoBroker{Incierto: true})
			switch kind {
			case "account":
				body = strings.Replace(body, p.Cuenta, "sesion-"+strings.Repeat("9", 32), 1)
			case "scope":
				body = strings.Replace(body, `"tenant":"acme"`, `"tenant":"another"`, 1)
			case "scope-header":
				header, err := storage.EncodeScopeHeader(*p.Ambito)
				if err != nil {
					t.Fatal(err)
				}
				other := *p.Ambito
				other.Tenant = "another"
				foreign, err := storage.EncodeScopeHeader(other)
				if err != nil {
					t.Fatal(err)
				}
				body = strings.Replace(body, header, foreign, 1)
			case "nonce":
				body = strings.Replace(body, access.Revision, strings.Repeat("8", 32), 1)
			case "version":
				body = strings.Replace(body, `"v":1`, `"v":2`, 1)
			case "action":
				body = strings.Replace(body, `"action":"iniciar"`, `"action":"leer"`, 1)
			case "trailing":
				body += " {}"
			case "unknown-field":
				body = strings.TrimSuffix(body, "}") + `,"foreign":true}`
			}
			client := readyWorkBroker(func(context.Context, string, string) (string, error) { return body, nil })
			if _, native, err := client.aplicar(t.Context(), "iniciar", p); !native || err == nil {
				t.Fatal("misrouted response authorized WAL", kind, err)
			}
		})
	}
}

func TestWorkBrokerRequiresDurableDirtyAndEveryCounterFlag(t *testing.T) {
	for _, action := range []string{"iniciar", "resumir"} {
		for _, key := range []string{"pending", "dirty", "discard_applied", "all_discard_requested", "clean-begin"} {
			if action == "resumir" && key == "clean-begin" {
				continue
			}
			t.Run(action+"-"+key, func(t *testing.T) {
				p, access := brokerFixture()
				body := brokerResponse(t, action, p, access, &registroTrabajoBroker{Incierto: true})
				var envelope map[string]json.RawMessage
				if err := json.Unmarshal([]byte(body), &envelope); err != nil {
					t.Fatal(err)
				}
				var work map[string]json.RawMessage
				if err := json.Unmarshal(envelope["work"], &work); err != nil {
					t.Fatal(err)
				}
				if key == "clean-begin" {
					work["dirty"] = json.RawMessage("false")
				} else {
					delete(work, key)
				}
				raw, err := json.Marshal(work)
				if err != nil {
					t.Fatal(err)
				}
				envelope["work"] = raw
				raw, err = json.Marshal(envelope)
				if err != nil {
					t.Fatal(err)
				}
				client := readyWorkBroker(func(context.Context, string, string) (string, error) { return string(raw), nil })
				if _, native, err := client.aplicar(t.Context(), action, p); !native || !errors.Is(err, ErrProtocoloTrabajoSesion) {
					t.Fatal("missing or clean state authorized WAL IO", key, err)
				}
			})
		}
	}
}

func TestWorkBrokerBindsOperationAndFailedReportCannotBecomeZero(t *testing.T) {
	p, access := brokerFixture()
	calls := 0
	client := readyWorkBroker(func(_ context.Context, action, body string) (string, error) {
		calls++
		var got pedidoTrabajoBroker
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatal(err)
		}
		if got.Cuenta != p.Cuenta || got.Ambito == nil || *got.Ambito != *p.Ambito || got.Acceso == nil || *got.Acceso != access {
			t.Fatal("work request lost its captured routing")
		}
		decoded, err := storage.DecodeScopeHeader(got.Cabecera)
		if err != nil || decoded != *p.Ambito {
			t.Fatal("work did not reuse the canonical scope header", err)
		}
		return brokerResponse(t, action, p, access, &registroTrabajoBroker{Incierto: action == "iniciar"}), nil
	})
	result, native, err := client.aplicar(t.Context(), "iniciar", p)
	if err != nil || !native || result.Operacion == "" || result.Trabajo == nil || !result.Trabajo.Incierto {
		t.Fatal("durable work was not registered before IO", err)
	}
	p.Operacion = result.Operacion
	p.Fallo = true
	if _, native, err := client.aplicar(t.Context(), "terminar", p); !native || !errors.Is(err, ErrProtocoloTrabajoSesion) || calls != 2 {
		t.Fatal("failed pending report became zero", calls, err)
	}
}

func TestWorkBrokerCancellationIsCheckedBeforeAndAfterIPC(t *testing.T) {
	p, access := brokerFixture()
	calls := 0
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := readyWorkBroker(func(context.Context, string, string) (string, error) { calls++; return "", nil })
	if _, _, err := client.aplicar(ctx, "iniciar", p); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled work performed IPC", err, calls)
	}
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	client = readyWorkBroker(func(context.Context, string, string) (string, error) {
		cancel()
		return brokerResponse(t, "iniciar", p, access, &registroTrabajoBroker{Incierto: true}), nil
	})
	if _, _, err := client.aplicar(ctx, "iniciar", p); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled response admitted work", err)
	}
}

func TestWorkBrokerExclusiveProofAndCleanupAckAreNotOptional(t *testing.T) {
	for _, action := range []string{"confirmar", "confirmar-desecho"} {
		t.Run(action, func(t *testing.T) {
			p, access := brokerFixture()
			exclusive := true
			p.Exclusiva = &exclusive
			if action == "confirmar" {
				p.Ambito = nil
			} else {
				zero := 0
				p.Pendientes = &zero
				p.Desecho = strings.Repeat("5", 32)
				p.Permiso = strings.Repeat("4", 32)
			}
			body := brokerResponse(t, action, p, access, &registroTrabajoBroker{Desecho: p.Desecho})
			if action == "confirmar" {
				body = strings.Replace(body, strings.Repeat("4", 32), "", 1)
			}
			client := readyWorkBroker(func(context.Context, string, string) (string, error) { return body, nil })
			if _, native, err := client.aplicar(t.Context(), action, p); !native || !errors.Is(err, ErrProtocoloTrabajoSesion) {
				t.Fatal("missing EX proof or unapplied tombstone acknowledged", action, err)
			}
		})
	}
}
