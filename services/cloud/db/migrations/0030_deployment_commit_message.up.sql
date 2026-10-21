-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- El asunto del commit que se despliega. El dashboard ya tenía el hueco —la fila
-- de un deployment lo enseña truncado, con tooltip— pero el control plane nunca
-- lo guardó, así que el frontend se lo inventaba repitiendo el SHA («Despliegue
-- abc1234»), que ya sale en su propia columna. Los webhooks SÍ lo traen desde el
-- principio (head_commit.message en GitHub/Gitea, commits[] en GitLab, y el
-- título del PR/MR para las previews): solo había que dejar de tirarlo.
--
-- El texto lo escribe QUIEN HACE EL PUSH, no la plataforma: entra como dato
-- hostil. El CHECK repite el tope de domain.MaxCommitMessage a propósito —el
-- saneado vive en el dominio (primera línea, sin caracteres de control) y esta
-- es la segunda mitad—, de modo que ni un camino nuevo que se salte el dominio
-- puede meter un mensaje de un megabyte en la tabla que más se lista.
--
-- Vacío para todo lo que no nace de un push: redeploy manual, rollback,
-- plantilla e imagen BYO. Sin backfill posible (el dato no se guardó nunca);
-- los deployments anteriores se quedan sin mensaje y la UI enseña su etiqueta.

ALTER TABLE deployments
    ADD COLUMN IF NOT EXISTS commit_message text NOT NULL DEFAULT ''
        CHECK (char_length(commit_message) <= 200);
