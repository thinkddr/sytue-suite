-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Envío de la auditoría a un SIEM del CLIENTE (Splunk HEC, Microsoft Sentinel,
-- Elastic, Datadog…). Hasta ahora la auditoría solo se podía sacar tirando de ella
-- (CSV/JSON bajo demanda), y un SIEM no funciona así: espera RECIBIR los eventos. Sin
-- push, la correlación con el resto de la seguridad del cliente llega tarde o no llega.
--
-- Un sink por organización. El estado de la entrega vive aquí y no en memoria para que
-- reiniciar el servicio no reenvíe ni se salte nada: `last_sent_id` es la posición en el
-- bigint MONÓTONO de audit_log, que ya ordena el log sin ambigüedad.
CREATE TABLE audit_sinks (
    org_id        uuid        PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    -- endpoint del SIEM. Se valida contra SSRF al guardarlo Y en cada envío.
    url           text        NOT NULL,
    -- Cabecera de autorización del SIEM (p. ej. «Splunk <token>»). SELLADA con la KEK
    -- del servicio (keycrypto, AES-256-GCM, "encv1:…") porque es una credencial de un
    -- tercero: nunca en claro, nunca en un log, nunca de vuelta al navegador.
    auth_header   text        NOT NULL DEFAULT '',
    -- Secreto para firmar el cuerpo (HMAC-SHA256). El receptor lo verifica y así sabe
    -- que el lote viene de Sytue y no ha sido manipulado. También sellado.
    signing_secret text       NOT NULL DEFAULT '',
    enabled       boolean     NOT NULL DEFAULT true,
    -- Posición de la entrega: el último audit_log.id confirmado por el SIEM.
    last_sent_id  bigint      NOT NULL DEFAULT 0,
    -- Diagnóstico del último intento, para que el admin vea POR QUÉ no llega.
    last_error    text        NOT NULL DEFAULT '',
    last_sent_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_sinks_url_not_blank CHECK (length(url) > 0)
);
