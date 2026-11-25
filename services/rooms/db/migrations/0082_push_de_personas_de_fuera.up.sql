-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- EL PUSH A LA PERSONA DE OTRA ORGANIZACIÓN (ADR 0040). Quien viene de fuera se
-- suscribe al Web Push desde SU organización, con su token, y ahí quedan su
-- suscripción y su «Pausar avisos». El enviador las buscaba en la organización del
-- canal y no encontraba nada: a la persona de fuera nunca le llegaba un aviso.
--
-- Leerlas es una lectura entre organizaciones y va como las demás de estas
-- conversaciones (mig 0036): por la política de mantenimiento, de solo lectura, y
-- SOLO de quien está como externo en alguna conversación. Una transacción de
-- mantenimiento no ve las suscripciones ni las pausas de nadie más.
--
-- Expand puro: dos políticas nuevas, ninguna fila cambia.

CREATE POLICY push_subscriptions_externos_maintenance ON push_subscriptions FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on'
           AND EXISTS (SELECT 1 FROM channel_members cm
                       WHERE cm.subject = push_subscriptions.subject AND cm.externo));

CREATE POLICY user_notify_prefs_externos_maintenance ON user_notify_prefs FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on'
           AND EXISTS (SELECT 1 FROM channel_members cm
                       WHERE cm.subject = user_notify_prefs.subject AND cm.externo));
