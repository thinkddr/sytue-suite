-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- RETENCIÓN LEGAL PARA NOTAS Y CUADERNOS: la contrapartida de la 0005.
--
-- La 0005 puso a caducar la papelera, y con eso Sytue Notepads pasó a DESTRUIR
-- contenido solo. Eso está bien mientras nadie tenga la obligación de conservar,
-- y deja de estarlo el día que llega un litigio, una inspección o un plazo
-- sectorial: entonces hay que poder decir «esto no se borra» sin apagar la purga
-- de toda la instalación, que es lo único que hoy se podía hacer
-- (`trashRetention: 0`).
--
-- Espeja la 0012 de Drive y la 0009 de Mail. Misma tabla, mismas columnas y
-- mismos índices, a propósito: quien ya sabe leer los holds de Drive sabe leer
-- estos, y la Auditoría unificada no tiene que aprender un formato por producto.
--
-- PRESERVACIÓN DINÁMICA, no sello. No se marca cada nota: se evalúa contra los
-- holds ACTIVOS en el punto de borrado (NOT EXISTS). Así, poner un hold protege
-- también lo que ya estaba en la papelera, y liberarlo devuelve todo al ciclo
-- normal sin tener que recorrer nada. El precio es un JOIN en la purga, que
-- corre una vez por hora.
--
-- QUÉ SIGUE PASANDO CON UN HOLD ACTIVO: la papelera sigue ocultando la nota
-- —quien la borró la ve borrada— y la persona puede seguir usando su cuenta con
-- normalidad. Lo único que cambia es que el recolector no la destruye. Un hold
-- no es un castigo ni se anuncia: es una obligación de la organización.

CREATE TABLE legal_holds (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant      text        NOT NULL,
    subject     text,                                     -- `sub` de Sytue ID (owner_sub); NULL = toda la organización
    reason      text        NOT NULL DEFAULT '',
    case_ref    text        NOT NULL DEFAULT '',          -- identificador del caso o litigio
    created_by  text        NOT NULL,                     -- sub del admin que lo creó
    created_at  timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,                              -- NULL = activo
    released_by text        NOT NULL DEFAULT ''
);

-- Holds ACTIVOS que cubren a un sujeto (o a toda la organización): es la consulta
-- que hace el recolector en CADA pasada, así que es la que tiene que ser barata.
CREATE INDEX legal_holds_active_idx ON legal_holds (tenant, subject) WHERE released_at IS NULL;
CREATE INDEX legal_holds_tenant_idx ON legal_holds (tenant, created_at DESC);

ALTER TABLE legal_holds ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_holds FORCE ROW LEVEL SECURITY;

CREATE POLICY legal_holds_tenant_isolation ON legal_holds
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- MANTENIMIENTO, Y ESTO ES LO QUE EVITA PERDER DATOS. El recolector de la 0005 es
-- CROSS-TENANT: corre en `pg.InMaintenanceTx`, que fija `app.maintenance` y NO
-- `app.tenant`. Bajo FORCE ROW LEVEL SECURITY, sin esta política vería CERO holds
-- —la de aislamiento no casa, porque no hay tenant fijado— y concluiría que no
-- hay nada preservado. Es decir: destruiría precisamente las notas que un juzgado
-- mandó conservar, y en silencio. El aislamiento no se pierde por esto: cada
-- consulta de purga sigue llevando su `h.tenant = <tenant de la fila>`.
--
-- MÁS ESTRECHA QUE LA DE DRIVE, que es equivalente pero sin acotar el comando. El
-- recolector solo LEE holds: no los crea, no los libera y no los borra. `FOR
-- SELECT` deja eso escrito en la base en vez de en un comentario, y si algún día
-- alguien intenta escribir holds desde una transacción de mantenimiento, falla en
-- vez de colar.
CREATE POLICY legal_holds_maintenance ON legal_holds
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

-- FUSIÓN DE ORGANIZACIONES, por el mismo motivo que se le dio a `audit_log` en la
-- 0004: al fusionar se reescribe el `tenant` de cada fila, y bajo FORCE ROW LEVEL
-- SECURITY la política de aislamiento no ve las del origen, así que el UPDATE
-- afectaría a cero filas y la fusión se declararía completada dejando los holds
-- atrás. Perder un hold en una fusión es perder la obligación de conservar sin
-- que nadie se entere.
CREATE POLICY legal_holds_platform_merge ON legal_holds
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
