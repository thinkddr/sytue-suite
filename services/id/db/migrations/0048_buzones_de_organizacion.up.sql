-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Buzón de correo automático para todo el que entra en una organización.
--
-- Hasta ahora la dirección de correo de una persona era, sin más, el email con el
-- que se registró: Sytue Mail aprovisionaba la cuenta de forma perezosa la primera
-- vez que alguien abría el webmail o enviaba, tomando la dirección del claim
-- `email` del token. Eso deja dos agujeros que no se ven hasta que duelen. Quien
-- entra invitado con su correo personal acaba con un buzón en un dominio que
-- nuestro MX no sirve, y quien todavía no ha iniciado sesión NO EXISTE para el MX:
-- el correo que le mandan rebota con 550 «No such user here» hasta el día que
-- entre por primera vez. Un alta no puede depender de que la persona se conecte
-- para poder recibir.
--
-- Así que la dirección deja de ser un derivado del login y pasa a ser un recurso
-- que Sytue ID asigna en el momento del alta, aquí. Se guarda en ID —y no solo en
-- Sytue Mail— porque quien conoce el dominio de la organización, el nombre y los
-- apellidos de la persona y las tres vías de alta (invitación aceptada, SCIM y
-- consola) es ID; Mail solo recibe la dirección ya decidida.
--
-- POR QUÉ UNA SOLA TABLA PARA BUZONES Y ALIAS: un alias es otra dirección que
-- entrega en el mismo buzón, así que comparte con la dirección principal la única
-- regla que de verdad importa — una dirección entrega en un solo sitio. Teniéndolos
-- en tablas distintas esa regla habría que sostenerla a mano entre dos índices y
-- una comprobación cruzada, y el caso «doy de alta un alias que ya es el buzón de
-- otro» se colaría por la rendija de la carrera. Con una tabla es un único índice
-- único: el choque lo resuelve PostgreSQL y no hay rendija.
CREATE TABLE org_mailboxes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    address    text NOT NULL CHECK (address <> '' AND position('@' in address) > 1),
    -- La dirección principal del buzón frente a sus alias. Se modela como bandera
    -- y no como tabla aparte por lo dicho arriba: el espacio de direcciones es uno.
    primary_address boolean NOT NULL DEFAULT true,
    -- Cuándo se dejó creada la cuenta en Sytue Mail. NULL = asignada aquí pero
    -- todavía no existe allí, que es justo el estado en el que el MX rechazaría su
    -- correo. Se marca en vez de darlo por hecho porque las dos bases son
    -- distintas y no hay transacción que las abarque: sin este sello, un fallo de
    -- red al crear la cuenta dejaría a alguien con dirección asignada y sin buzón,
    -- sin nadie que lo reintentara y sin forma de saberlo. Es la cola de trabajo
    -- del reconciliador (ver mailboxProvisionLoop).
    provisioned_at timestamptz,
    -- Cuándo se pidió retirar la dirección. Quitar un alias no puede ser un DELETE
    -- a secas: la dirección también existe en Sytue Mail y allí seguiría entregando,
    -- así que quien la retirase creería haberla quitado y el correo seguiría
    -- llegando —una sorpresa silenciosa de las peores—. Se marca, el reconciliador
    -- la retira también allí y entonces desaparece la fila.
    --
    -- La fila marcada SIGUE ocupando su dirección en el índice único a propósito:
    -- si la liberase al instante, se podría reasignar a otra persona y el
    -- reconciliador borraría después en Sytue Mail el alias del nuevo dueño. Queda
    -- reservada el tiempo que tarde el barrido (segundos) y luego se libera sola.
    removed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Una dirección entrega en UN solo buzón, en toda la plataforma y sea cual sea la
-- organización: el MX resuelve por dirección, no por tenant, así que la unicidad
-- tiene que ser global o dos organizaciones podrían reclamar el mismo destino.
-- Case-insensitive porque el buzón de destino no distingue mayúsculas (RFC 5321
-- §2.4 deja la caja del local-part al destino; aquí el destino somos nosotros y
-- decidimos que no distingue: nadie espera que Ana.Perez y ana.perez sean dos).
CREATE UNIQUE INDEX org_mailboxes_address_idx ON org_mailboxes (lower(address));

-- Una sola dirección principal por persona y organización. Parcial a propósito:
-- los alias de esa misma persona son cuantos quiera. Que sea un índice y no una
-- comprobación en el código es lo que hace idempotente el alta — dos vías de alta
-- simultáneas (SCIM reintentando sobre una invitación recién aceptada) chocan aquí
-- en vez de crear dos buzones para la misma persona.
CREATE UNIQUE INDEX org_mailboxes_primary_idx ON org_mailboxes (org_id, user_id)
    WHERE primary_address;

-- Listar el buzón y los alias de una persona, y los de toda la organización, sin
-- Seq Scan: son las dos consultas que hace la consola de administración.
CREATE INDEX org_mailboxes_org_user_idx ON org_mailboxes (org_id, user_id);

-- Cola de lo que falta por crear en Sytue Mail. Parcial: en régimen normal está
-- vacía, y un índice parcial hace que el barrido periódico no toque la tabla
-- entera cada vez para descubrir que no hay nada que hacer.
CREATE INDEX org_mailboxes_pendientes_idx ON org_mailboxes (created_at)
    WHERE provisioned_at IS NULL AND removed_at IS NULL;

-- Cola de lo que falta por RETIRAR de Sytue Mail. Parcial por lo mismo.
CREATE INDEX org_mailboxes_retiradas_idx ON org_mailboxes (removed_at)
    WHERE removed_at IS NOT NULL;

CREATE TRIGGER org_mailboxes_set_updated_at BEFORE UPDATE ON org_mailboxes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Personalizar la dirección en el momento de invitar. La invitación es la vía de
-- alta manual desde la consola, y quien invita es el único que sabe si esa persona
-- debe ser `soporte@` en vez de `nombre.apellidos@`. Viaja en la fila de la
-- invitación —y no en el enlace— por lo mismo que el rol y la organización: al
-- aceptar no se confía en nada que venga del navegador. Vacío = patrón por defecto.
ALTER TABLE org_invitations ADD COLUMN mailbox_local text NOT NULL DEFAULT '';
