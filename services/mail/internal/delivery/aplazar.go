// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

// Aplazar la entrega a una cuenta cuyo plan de entrada ya nadie puede reclamar
// (ADR 0087, entrega 5, «Queda para después»).
//
// Con la autoridad de Matrix del correo retirada en Rooms, la admisión SMTP
// seguiría cifrando el correo nuevo para el plan de entrada vigente, el de
// Matrix, que ya no reclama nadie: quedaría en la cola, ilegible. Lo mismo pasa
// tras un traspaso mientras la bóveda aún no ha rotado el plan (lo rota al abrir
// el correo). En vez de eso, el MX contesta un 4xx a ESE destinatario, en RCPT
// (450 4.2.1): el remitente reintenta con sus reglas y, si la persona no pasa a
// la bóveda a tiempo, se rinde y devuelve el aviso de no entrega a quien
// escribió (RFC 5321 §4.5.4.1, unos cinco días). Aquí no se guarda nada del
// mensaje, ni en claro ni cifrado.
//
// Solo se pregunta a Rooms si el plan vigente es de Matrix (o de antes de
// guardar su autoridad, que también lo es): con un plan de la bóveda, sin
// entrada privada o sin plan, el correo sigue su camino de siempre sin ninguna
// llamada.
//
//   - Rooms contesta `storage_matrix_retired` → se aplaza (motivo `retirada`).
//   - Rooms da la autoridad de la bóveda → el traspaso está hecho pero el plan
//     aún es el de Matrix → se aplaza hasta que la bóveda rote (`traspaso`).
//   - Rooms da la autoridad de Matrix → se entrega, como siempre.
//   - Rooms no contesta, o contesta algo que no sirve → se aplaza
//     (`sin_respuesta`): un retraso se recupera; un correo cifrado para un plan
//     perdido, no.
//
// Sin Rooms configurado (desarrollo) no se aplaza nada. Cada aplazamiento queda
// en el registro (cuenta, generación del plan y motivo, sin direcciones) y en el
// contador `mail.inbound.deferred`, con el motivo.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// ErrAplazada: el correo para ese destinatario tiene que esperar a que la
// persona pase su correo a la bóveda. Es temporal: el MX contesta 450 4.2.1 y el
// webhook, 503 con Retry-After.
var ErrAplazada = errors.New("delivery: entrega aplazada hasta que la cuenta pase su correo a la bóveda")

// Motivos de un aplazamiento, en el registro y en el contador.
const (
	MotivoRetirada     = "retirada"
	MotivoTraspaso     = "traspaso"
	MotivoSinRespuesta = "sin_respuesta"
)

// consultaAutoridad acota la pregunta a Rooms desde RCPT: sin respuesta en ese
// plazo, se aplaza.
const consultaAutoridad = 10 * time.Second

// AutoridadCorreo es la lectura de la autoridad del correo de una persona en
// Rooms (callaccess.Client, por el canal interno que ya comparten).
type AutoridadCorreo interface {
	ReadStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error)
}

// planesDeEntrada es lo que se lee de la base para decidir (store.Store).
type planesDeEntrada interface {
	PlanDeEntradaVigente(context.Context, store.ResolvedAddress) (store.PlanDeEntrada, error)
}

// Aplazar dice si el correo para `rcpt` tiene que esperar (ErrAplazada, con el
// motivo en el texto). Otro error es una lectura fallida de la base, que el MX
// trata como cualquier fallo temporal. Una dirección sin cuenta (un canal de
// Rooms) nunca se aplaza.
func (in *Inbound) Aplazar(ctx context.Context, rcpt store.ResolvedAddress) error {
	if in.autoridad == nil || in.planes == nil || rcpt.AccountID == uuid.Nil {
		return nil
	}
	t, err := tenancy.New(rcpt.Tenant)
	if err != nil {
		return fmt.Errorf("delivery: tenant inválido %q: %w", rcpt.Tenant, err)
	}
	ctx = tenancy.Into(ctx, t)
	plan, err := in.planes.PlanDeEntradaVigente(ctx, rcpt)
	if err != nil {
		return fmt.Errorf("delivery: leer el plan de entrada: %w", err)
	}
	if !plan.Privada || plan.Generacion == 0 || plan.AutoridadUsuario == vaultproof.UserID(plan.Scope) {
		return nil
	}
	consulta, cancelar := context.WithTimeout(ctx, consultaAutoridad)
	defer cancelar()
	autoridad, err := in.autoridad.ReadStorageAuthority(consulta, plan.Scope)
	var motivo string
	switch {
	case errors.Is(err, callaccess.ErrStorageMatrixRetired):
		motivo = MotivoRetirada
	case err != nil:
		motivo = MotivoSinRespuesta
	case vaultproof.IsAuthority(autoridad):
		motivo = MotivoTraspaso
	default:
		return nil
	}
	in.anotarAplazada(ctx, rcpt, plan, motivo, err)
	return fmt.Errorf("%w (%s)", ErrAplazada, motivo)
}

func (in *Inbound) anotarAplazada(ctx context.Context, rcpt store.ResolvedAddress, plan store.PlanDeEntrada, motivo string, causa error) {
	contarAplazada(ctx, motivo)
	if in.logger == nil {
		return
	}
	attrs := []any{"tenant", rcpt.Tenant, "account", rcpt.AccountID.String(), "generacion", plan.Generacion, "motivo", motivo}
	if motivo == MotivoSinRespuesta {
		// Una avería de Rooms retrasa el correo de todas las cuentas de Matrix:
		// eso sí merece un aviso.
		in.logger.WarnContext(ctx, "inbound: entrega aplazada, Rooms no dice si Matrix está retirada", append(attrs, "error", causa)...)
		return
	}
	in.logger.InfoContext(ctx, "inbound: entrega aplazada hasta el traspaso a la bóveda", attrs...)
}

var (
	contadorAplazadasUnaVez sync.Once
	contadorAplazadas       *telemetry.Counter
)

// contarAplazada suma un aplazamiento al contador `mail.inbound.deferred`. Uno
// por proceso, aunque haya dos receptores (MX y webhook); sin telemetría, nada.
func contarAplazada(ctx context.Context, motivo string) {
	contadorAplazadasUnaVez.Do(func() {
		if medidor := telemetry.DefaultMeter("mail.inbound"); medidor != nil {
			contadorAplazadas = medidor.Counter("mail.inbound.deferred")
		}
	})
	if contadorAplazadas != nil {
		contadorAplazadas.Add(ctx, 1, telemetry.String("motivo", motivo))
	}
}
