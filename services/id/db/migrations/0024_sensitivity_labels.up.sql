-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Etiquetas de SENSIBILIDAD (paridad Microsoft Purview sensitivity labels / Google
-- Drive labels). La TAXONOMÍA vive aquí, en el plano de control de la organización:
-- es una definición de gobierno (como los roles o las políticas de acceso), no un
-- dato de un producto concreto. Cada producto que etiqueta contenido (Drive, hoy)
-- guarda un SNAPSHOT denormalizado de la etiqueta sobre el propio contenido, así:
--
--   * decide sin llamar a este servicio en el camino caliente (compartir/descargar);
--   * la etiqueta sigue siendo legible aunque la taxonomía cambie o se borre;
--   * un cambio posterior de la taxonomía NO reescribe el histórico (el admin puede
--     re-etiquetar si lo quiere; nada lo hace por él a sus espaldas).
--
-- MODELO MÍNIMO Y HONESTO: nombre, orden de sensibilidad (rank), color, descripción
-- en lenguaje llano y UNA política real: block_external_share. Sin cifrado por
-- etiqueta ni clasificación automática (no están implementados; no se insinúan aquí).
--
-- rank ORDENA las etiquetas de menor a mayor sensibilidad (público < interno <
-- confidencial < restringido); lo define el ADMIN, no hay escala fija. No es único:
-- dos etiquetas pueden empatar (el desempate de listado es el nombre).
--
-- AISLAMIENTO POR TENANT (igual que mig 0018/0019/0020, a NIVEL DE APLICACIÓN):
-- FK org_id + scoping por org en CADA query, DENY-BY-DEFAULT. Una etiqueta de la
-- org A jamás se lista, resuelve ni edita desde la org B.

CREATE TABLE sensitivity_labels (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name        text NOT NULL,
    -- Orden de sensibilidad (mayor = más sensible). Lo fija el admin al crearla.
    rank        smallint NOT NULL DEFAULT 0,
    -- Color de presentación (#rrggbb). Vacío ⇒ la UI usa el color neutro.
    color       text NOT NULL DEFAULT '',
    -- Para qué es, en lenguaje llano (se muestra a quien etiqueta, no solo al admin).
    description text NOT NULL DEFAULT '',
    -- ÚNICA política de la etiqueta: si está activa, el contenido etiquetado con ella
    -- NO admite enlaces públicos/externos. El enforcement vive en el producto que
    -- guarda el snapshot (Drive: creación de shares kind=link).
    block_external_share boolean NOT NULL DEFAULT false,
    created_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Nombre único por organización (case-insensitive): dos etiquetas homónimas serían
-- indistinguibles para quien etiqueta.
CREATE UNIQUE INDEX sensitivity_labels_org_name_idx
    ON sensitivity_labels (org_id, lower(name));

-- Listado por org en orden de sensibilidad (el de la UI y el de la API de lectura).
CREATE INDEX sensitivity_labels_org_rank_idx
    ON sensitivity_labels (org_id, rank, id);

CREATE TRIGGER sensitivity_labels_set_updated_at BEFORE UPDATE ON sensitivity_labels
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
