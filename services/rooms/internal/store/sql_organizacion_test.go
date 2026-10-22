// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/rooms/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia de Sytue Rooms que lee o modifica una
// tabla con `tenant` lo nombra en su predicado. La política RLS no basta —un rol que
// se la salta lee la fila de cualquiera con un `WHERE id = $1`—, así que el filtro
// tiene que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{".", "../media/store", "../../cmd/rooms"},
		Excepciones: map[string]string{
			"matrix_document_calls.go:AdvanceDocumentCallAccess": "un documento conserva su UUID al cambiar de organización; cierra solo sus llamadas documentales en todas sus organizaciones anteriores, fijando cada tenant antes de modificar la sala",
			"externos.go:tenantsAnfitriones":                     "la única consulta entre organizaciones de las conversaciones con gente de fuera (ADR 0040): en cuáles está esta persona como externa; solo filas `externo` de ese sujeto",
			"externos.go:AnfitrionDeReunionDeEvento":             "en qué organización vive la reunión de un evento al que viene alguien de fuera (decisión del 2026-10-16); solo la organización, solo salas de un evento",
			"externos.go:AnfitrionDeCanal":                       "ADR 0040: en qué organización vive el canal donde esta persona es externa; solo filas `externo` de ese sujeto y ese canal",
			"invitaciones.go:invitacionesDe":                     "la persona invitada desde otra organización pregunta por SUS invitaciones (mig 0041): solo las filas de su sujeto, por la política de mantenimiento, de solo lectura",
			"invitaciones.go:organizacionDeInvitacion":           "en qué organización vive el canal al que está invitada esta persona; solo su fila de ese canal",
			"integrations.go:LookupIncomingWebhookByTokenHash":   "el webhook entrante llega sin sesión: su token (por hash) ES lo que dice de qué organización y canal es",
			"integrations.go:LookupBotByTokenHash":               "el bot se autentica con su token (por hash), que es lo que dice de qué organización es",
			"whatsapp.go:LookupWhatsAppAccountByTokenHash":       "el aviso de Meta llega sin organización: el token de la URL (por hash) dice de qué cuenta es",
			"scheduled.go:ClaimDueScheduled":                     "el trabajador de mensajes programados reclama los vencidos de todas las organizaciones y fija la de cada fila antes de publicar",
			"main.go:runOutboxPurge":                             "retención del outbox de eventos: purga los de más de 48 h de todas las organizaciones",
			"moderacion.go:bloqueoEnDirectoTx":                   "un bloqueo es de la PERSONA, sea cual sea la organización donde lo puso; el canal sí va filtrado por la suya",
			"moderacion.go:quienesBloquean":                      "qué destinatarios de un aviso han bloqueado al autor, en cualquier organización; solo devuelve sus sujetos",
			"moderacion.go:ListBlocks":                           "a quién tiene bloqueado UNA persona, en todas sus organizaciones",
			"moderacion.go:Unblock":                              "quitar un bloqueo de UNA persona vale en todas sus organizaciones",
			"operador.go:ListReportsOperador":                    "la cola del operador de la plataforma es de todas las organizaciones (ADR 0073); quién es operador lo decide Sytue ID",
			"operador.go:OrgDeDenuncia":                          "el operador averigua de qué organización es una denuncia antes de abrirla en la suya (ADR 0073)",
			"operador.go:PurgarDenuncias":                        "retención: purga las denuncias resueltas de todas las organizaciones, en segundo plano",
		},
	})
}
