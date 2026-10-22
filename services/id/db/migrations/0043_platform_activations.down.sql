-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revertir devuelve el alcance de plataforma a permanente: quien esté en
-- `platform_admins` vuelve a autorizar sin activación. Se pierde el histórico de
-- elevaciones, que es justo lo que esta tabla guardaba.
DROP TABLE IF EXISTS platform_activations;
