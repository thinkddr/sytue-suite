-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El dispositivo DESDE EL QUE se abrió cada sesión, para el claim `device_trust`.
--
-- La sesión sabía cómo se identificó la persona (`amr`) pero no desde dónde, y el
-- dispositivo (0022) se registraba aparte, sin lazo con la sesión: no había forma de
-- decir en un token «esta sesión viene de un dispositivo de confianza». Con el lazo,
-- cada token que emite la sesión —el del código y los de cada refresco— evalúa el
-- dispositivo EN ESE MOMENTO: quitarle la confianza o revocarlo baja el claim en el
-- siguiente refresco, sin esperar a que caduque la sesión.
--
-- ON DELETE SET NULL: revocar el dispositivo (borrarlo) deja la sesión viva pero sin
-- dispositivo, que es exactamente «sin confianza». Nullable porque las sesiones sin
-- organización no registran dispositivo y porque las anteriores a esto no lo tienen.
ALTER TABLE sessions
    ADD COLUMN device_id uuid REFERENCES devices(id) ON DELETE SET NULL;

-- Para que revocar un dispositivo no recorra todas las sesiones buscando las suyas.
CREATE INDEX sessions_device_idx ON sessions (device_id) WHERE device_id IS NOT NULL;
