-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve a un solo dominio de correo por organización.
--
-- Las tres columnas de `org_profiles` siguen ahí —la subida no las borró, para no
-- tumbar a la versión anterior durante el rollout—, así que aquí solo hay que
-- devolverles el dominio PRINCIPAL: lo que se haya conectado desde entonces vive
-- en la tabla que este fichero borra, y sin esta copia la organización volvería
-- al subdominio de la suite sin que nadie lo hubiera pedido.
--
-- Los dominios NO principales se pierden: no hay dónde ponerlos, que es
-- exactamente el motivo por el que existía la tabla. Las direcciones ya asignadas
-- en ellos no se tocan (viven en org_mailboxes), pero Sytue deja de afirmar que
-- sirve esos dominios, así que conviene mirarlas antes de revertir.
UPDATE org_profiles p SET
    domain             = d.domain,
    domain_token       = d.token,
    domain_verified_at = d.verified_at
FROM org_mail_domains d
WHERE d.org_id = p.org_id AND d.is_primary;

DROP TABLE IF EXISTS org_mail_domains;
