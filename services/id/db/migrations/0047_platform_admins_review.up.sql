-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Que alguien vuelva a mirar quién puede administrar la plataforma.
--
-- `platform_admins` (0031) se concede y ya está: no hay nada que obligue a
-- revisarla nunca. El PIM (0043) hizo que el alcance no valga sin activarlo,
-- pero la ELEGIBILIDAD sigue siendo indefinida — quien entró para una migración
-- de hace dos años sigue pudiendo elevarse hoy, y nadie tiene un momento en el
-- que se le pregunte si eso sigue teniendo sentido.
--
-- Se guarda cuándo se revisó por última vez y quién lo hizo. Con eso, el panel
-- puede decir «este acceso lleva sin revisarse ocho meses», que es la pregunta.
--
-- NO se retira nada por no revisarlo. Un acceso que caduca solo suena bien hasta
-- el día en que caduca el de todo el mundo a la vez y no queda nadie que pueda
-- devolverlo: se avisa, y retirar sigue siendo un acto deliberado.
--
-- `reviewed_at` es NOT NULL y se rellena con `created_at`: conceder el acceso ES
-- haberlo mirado una vez, y así ninguna consulta necesita un caso especial para
-- «nunca revisado».
ALTER TABLE platform_admins
    ADD COLUMN reviewed_at timestamptz NOT NULL DEFAULT now(),
    -- Quién firmó la última revisión. NULL = ninguna todavía (vale el alta).
    -- No es FK, por lo mismo que `granted_by`: borrar a un admin no debe
    -- reescribir quién revisó a los demás.
    ADD COLUMN reviewed_by uuid;

UPDATE platform_admins SET reviewed_at = created_at;
