// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	roomsdomain "git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Messaging es la parte de la mensajería de Sytue Rooms que la videollamada necesita.
// La satisface el servicio de mensajería del MISMO proceso.
type Messaging interface {
	// PostCallEnded publica la tarjeta de sistema «llamada finalizada» en el canal.
	PostCallEnded(ctx context.Context, channelID uuid.UUID, authorSub string, durationSec int64) (roomsdomain.Message, error)
	// GetChannel resuelve un canal aplicando la verja de lectura completa (público ⇒
	// miembro del equipo; privado ⇒ miembro del canal; directo ⇒ participante). Un
	// error significa «no puede leerlo», sin distinguir el motivo.
	GetChannel(ctx context.Context, id uuid.UUID, subject string) (roomsdomain.Channel, error)
}

// InProcessNotifier conecta la videollamada con la mensajería SIN salir del proceso.
//
// Antes esto viajaba por HTTP entre dos servicios (`/internal/call-ended` y
// `/internal/channel-access`) con un secreto compartido, y ese cableado era frágil de
// una forma peligrosa: si faltaba el secreto, el notificador quedaba a nil y la verja
// que exige poder LEER un canal antes de abrir su sala fallaba CERRADA — abrir una
// llamada desde un canal devolvía 403 y nadie sabía por qué. Al ser un solo servicio, la
// llamada es directa: no hay secreto que olvidar, ni red que falle, ni 403 fantasma.
type InProcessNotifier struct{ msg Messaging }

// NewInProcessNotifier construye el puente. `msg` es el servicio de mensajería.
func NewInProcessNotifier(msg Messaging) *InProcessNotifier {
	if msg == nil {
		return nil
	}
	return &InProcessNotifier{msg: msg}
}

// CallEnded publica la tarjeta de «llamada finalizada» en el canal de origen.
func (n *InProcessNotifier) CallEnded(ctx context.Context, in CallEndedInput) error {
	t, err := tenancy.New(in.Tenant)
	if err != nil {
		return err
	}
	channelID, err := uuid.Parse(in.ChannelID)
	if err != nil {
		return err
	}
	_, err = n.msg.PostCallEnded(tenancy.Into(ctx, t), channelID, in.AuthorSub, in.DurationSec)
	return err
}

// ChannelAccess indica si el sujeto puede leer el canal. La mensajería sigue siendo la
// ÚNICA fuente de verdad del roster: salir del canal revoca el acceso a su llamada al
// instante, sin ninguna lista replicada aquí.
func (n *InProcessNotifier) ChannelAccess(ctx context.Context, tenant, channelID, subject string) (bool, error) {
	t, err := tenancy.New(tenant)
	if err != nil {
		return false, err
	}
	id, err := uuid.Parse(channelID)
	if err != nil {
		return false, err
	}
	if _, err := n.msg.GetChannel(tenancy.Into(ctx, t), id, subject); err != nil {
		// Cualquier negativa (no existe, no es miembro) es lo mismo de cara al llamante:
		// no puede. No es un fallo del puente, así que no se propaga como error.
		//nolint:nilerr // deliberado: «no puede leerlo» no es una avería.
		return false, nil
	}
	return true, nil
}
