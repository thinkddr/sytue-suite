// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

var ambitoAplazado = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "mail"}

// planesFijos es la base: el plan vigente de la cuenta, o un fallo al leerlo.
type planesFijos struct {
	plan    store.PlanDeEntrada
	err     error
	lectura int
}

func (p *planesFijos) PlanDeEntradaVigente(ctx context.Context, rcpt store.ResolvedAddress) (store.PlanDeEntrada, error) {
	p.lectura++
	if t, err := tenancy.From(ctx); err != nil || t.Slug != rcpt.Tenant {
		return store.PlanDeEntrada{}, errors.New("la lectura no lleva el tenant del destinatario")
	}
	return p.plan, p.err
}

// roomsFijo es Rooms contestando siempre lo mismo a la autoridad del correo.
type roomsFijo struct {
	autoridad matrixproof.Authority
	err       error
	preguntas []storage.Scope
	conPlazo  bool
}

func (r *roomsFijo) ReadStorageAuthority(ctx context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	r.preguntas = append(r.preguntas, scope)
	_, r.conPlazo = ctx.Deadline()
	return r.autoridad, r.err
}

func autoridadDe(user string) matrixproof.Authority {
	return matrixproof.Authority{Version: 1, Scope: ambitoAplazado, UserID: user, RootKey: strings.Repeat("A", 43), Generation: 6, Status: "active"}
}

var (
	usuarioMatrix = storage.IsolatedMatrixUserPrefix(ambitoAplazado) + "matrix.sytue.eu"
	usuarioBoveda = vaultproof.UserID(ambitoAplazado)
	cuenta        = store.ResolvedAddress{Tenant: "acme", AccountID: uuid.MustParse("6f1c1c56-0a43-4ad4-8a3d-3e0a5c39a3a1"), Subject: "persona-1", Address: "ana@acme.test"}
)

func planDe(user string) store.PlanDeEntrada {
	return store.PlanDeEntrada{Scope: ambitoAplazado, Privada: true, Generacion: 1, AutoridadUsuario: user}
}

// El correo para un plan de entrada de Matrix que ya nadie reclama espera; el de
// un plan de la bóveda, el de Matrix aún honrada y el que no va por la entrada
// privada entran como siempre. A Rooms solo se le pregunta por un plan de Matrix.
func TestAplazarSoloLoQueQuedariaIlegible(t *testing.T) {
	retirada := callaccess.ErrStorageMatrixRetired
	for name, c := range map[string]struct {
		plan      store.PlanDeEntrada
		autoridad matrixproof.Authority
		err       error
		motivo    string // vacío: no se aplaza
		pregunta  bool
	}{
		"Matrix retirada":                     {plan: planDe(usuarioMatrix), err: retirada, motivo: MotivoRetirada, pregunta: true},
		"plan de antes de guardar autoridad":  {plan: planDe(""), err: retirada, motivo: MotivoRetirada, pregunta: true},
		"traspaso hecho y plan aún de Matrix": {plan: planDe(usuarioMatrix), autoridad: autoridadDe(usuarioBoveda), motivo: MotivoTraspaso, pregunta: true},
		"Rooms no contesta":                   {plan: planDe(usuarioMatrix), err: callaccess.ErrUnavailable, motivo: MotivoSinRespuesta, pregunta: true},
		"Matrix aún honrada":                  {plan: planDe(usuarioMatrix), autoridad: autoridadDe(usuarioMatrix), pregunta: true},
		"plan de la bóveda":                   {plan: planDe(usuarioBoveda), err: retirada},
		"cuenta sin entrada privada":          {plan: store.PlanDeEntrada{Scope: ambitoAplazado}, err: retirada},
		"época sin plan todavía":              {plan: store.PlanDeEntrada{Scope: ambitoAplazado, Privada: true}, err: retirada},
	} {
		t.Run(name, func(t *testing.T) {
			var registro bytes.Buffer
			rooms := &roomsFijo{autoridad: c.autoridad, err: c.err}
			in := &Inbound{autoridad: rooms, planes: &planesFijos{plan: c.plan}, logger: slog.New(slog.NewTextHandler(&registro, nil))}
			err := in.Aplazar(t.Context(), cuenta)
			if c.motivo == "" {
				if err != nil {
					t.Fatalf("aplazado sin motivo: %v", err)
				}
			} else if !errors.Is(err, ErrAplazada) || !strings.Contains(err.Error(), c.motivo) {
				t.Fatalf("quiero aplazar por %q: %v", c.motivo, err)
			}
			if got := len(rooms.preguntas) == 1; got != c.pregunta {
				t.Fatalf("preguntas a Rooms: %v", rooms.preguntas)
			}
			if c.pregunta && (rooms.preguntas[0] != ambitoAplazado || !rooms.conPlazo) {
				t.Fatalf("pregunta a Rooms: %+v (con plazo: %v)", rooms.preguntas[0], rooms.conPlazo)
			}
			// Observable: el motivo y la cuenta, nunca la dirección.
			if c.motivo != "" && (!strings.Contains(registro.String(), "motivo="+c.motivo) || !strings.Contains(registro.String(), cuenta.AccountID.String())) {
				t.Fatalf("el aplazamiento no quedó en el registro: %s", registro.String())
			}
			if strings.Contains(registro.String(), cuenta.Address) {
				t.Fatal("la dirección quedó en el registro")
			}
		})
	}
}

// Sin Rooms configurado (desarrollo) o para un canal de Rooms, ni se lee la base:
// todo como antes. Un fallo al leer la base no es un aplazamiento: el MX lo trata
// como cualquier fallo temporal.
func TestAplazarSinRoomsNiCuentaYConLaBaseCaida(t *testing.T) {
	planes := &planesFijos{plan: planDe(usuarioMatrix)}
	if err := (&Inbound{planes: planes}).Aplazar(t.Context(), cuenta); err != nil || planes.lectura != 0 {
		t.Fatalf("sin Rooms: %v (%d lecturas)", err, planes.lectura)
	}
	rooms := &roomsFijo{err: callaccess.ErrStorageMatrixRetired}
	canal := store.ResolvedAddress{Address: "canal@rooms.acme.test"}
	if err := (&Inbound{autoridad: rooms, planes: planes}).Aplazar(t.Context(), canal); err != nil || planes.lectura != 0 {
		t.Fatalf("canal de Rooms: %v (%d lecturas)", err, planes.lectura)
	}
	caida := errors.New("base caída")
	err := (&Inbound{autoridad: rooms, planes: &planesFijos{err: caida}}).Aplazar(t.Context(), cuenta)
	if !errors.Is(err, caida) || errors.Is(err, ErrAplazada) || len(rooms.preguntas) != 0 {
		t.Fatalf("base caída: %v", err)
	}
	if err := (&Inbound{autoridad: rooms, planes: planes}).Aplazar(t.Context(), store.ResolvedAddress{Tenant: "ACME no vale", AccountID: cuenta.AccountID}); err == nil || errors.Is(err, ErrAplazada) {
		t.Fatalf("tenant inválido: %v", err)
	}
}
