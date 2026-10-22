// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
)

// MessagingNotifier es el puente de la videollamada hacia la mensajería de Sytue Rooms:
// avisa de que una llamada ligada a un canal terminó —para que publique la tarjeta de
// sistema «Llamada finalizada · duración»— y le pregunta si un sujeto puede leer un
// canal (verja de las salas con origen en un canal).
//
// La implementación real es InProcessNotifier: mensajería y vídeo son el MISMO servicio,
// así que es una llamada directa. Antes viajaba por HTTP entre dos procesos con un
// secreto compartido: si el secreto faltaba, el puente quedaba a nil y la verja del
// canal fallaba CERRADA — abrir una llamada desde un canal devolvía 403 sin explicación.
// Ahora no hay secreto que olvidar.
type MessagingNotifier interface {
	CallEnded(ctx context.Context, in CallEndedInput) error
	// ChannelAccess indica si `subject` puede LEER el canal indicado. La mensajería es
	// la única fuente de verdad del roster, así que se pregunta en CADA entrada a la
	// sala: salir del canal revoca el acceso a su llamada al instante, sin ninguna ACL
	// replicada en el lado del vídeo.
	ChannelAccess(ctx context.Context, tenant, channelID, subject string) (bool, error)
}

// CallEndedInput son los datos de la tarjeta «llamada finalizada» que se publicará
// en el canal de origen.
type CallEndedInput struct {
	Tenant      string `json:"tenant"`
	ChannelID   string `json:"channel_id"`
	AuthorSub   string `json:"author_sub"`
	DurationSec int64  `json:"duration_sec"`
}
