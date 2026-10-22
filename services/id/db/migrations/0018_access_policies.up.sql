-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Acceso Condicional (paridad Entra Conditional Access): políticas evaluadas en el
-- LOGIN, TRAS verificar la credencial y ANTES de emitir la sesión. Cada política de
-- una organización combina condiciones (AND) sobre el contexto del intento — a quién
-- aplica (usuarios/grupos), IP de origen (rangos CIDR, con negación para "fuera de la
-- red"), app/client_id y, opcional, franja horaria — y decide una acción:
--   allow | require_mfa | block.
--
-- ANTI-LOCKOUT (crítico): sin políticas ⇒ se permite (no rompe el login de nadie).
-- Las EXCLUSIONES (usuarios/grupos exentos) son el break-glass de administradores: un
-- usuario excluido de una política no la sufre, aunque la cumpla (evita cerrar la org
-- fuera por una regla mal configurada). El motor combina las que aplican con la acción
-- MÁS RESTRICTIVA (block > require_mfa > allow).
--
-- AISLAMIENTO POR TENANT (igual que mig 0001/0015/0017, a NIVEL DE APLICACIÓN): FK
-- org_id + scoping por org en CADA query, DENY-BY-DEFAULT. Las políticas de la org A
-- jamás se evalúan para un login de la org B. Sin RLS de Postgres (el servicio no fija
-- GUCs por-conexión en el pool compartido).

CREATE TABLE access_policies (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name           text NOT NULL,
    -- Solo las políticas habilitadas se evalúan en el login (borradores no afectan).
    enabled        boolean NOT NULL DEFAULT false,
    -- Orden de listado/evaluación (menor = antes). No cambia el resultado (gana la más
    -- restrictiva), pero da un orden estable y editable en la Console.
    priority       integer NOT NULL DEFAULT 100,
    action         text NOT NULL CHECK (action IN ('allow', 'require_mfa', 'block')),

    -- A quién aplica (include). Vacíos AMBOS ⇒ "todos los usuarios" (default Entra).
    -- Si alguno tiene valores, aplica al usuario que esté en include_users O en alguno
    -- de include_groups. UUIDs de users/groups de ESTA org (un id ajeno nunca casa).
    include_users  uuid[] NOT NULL DEFAULT '{}',
    include_groups uuid[] NOT NULL DEFAULT '{}',
    -- Exclusiones (break-glass): el usuario en exclude_users, o en algún exclude_groups,
    -- queda EXENTO de esta política aunque la cumpla.
    exclude_users  uuid[] NOT NULL DEFAULT '{}',
    exclude_groups uuid[] NOT NULL DEFAULT '{}',

    -- Condición de red: rangos CIDR. Vacío ⇒ cualquier IP. ip_negate=false ⇒ la política
    -- aplica cuando la IP ESTÁ en algún rango (p. ej. bloquear rangos conocidos);
    -- ip_negate=true ⇒ aplica cuando la IP NO está en ninguno (p. ej. exigir MFA fuera de
    -- la red de oficina). Se guardan como texto y se validan/parsean en Go (net/netip).
    ip_ranges      text[] NOT NULL DEFAULT '{}',
    ip_negate      boolean NOT NULL DEFAULT false,

    -- Condición de app: client_ids OAuth a los que aplica. Vacío ⇒ cualquier app.
    client_ids     text[] NOT NULL DEFAULT '{}',

    -- Franja horaria opcional (UTC). days_mask: bitmask de días de la semana a los que
    -- aplica (bit i = time.Weekday i, Domingo=0); 0 ⇒ todos los días. start/end_minute:
    -- minuto-del-día [start,end); -1 ⇒ sin restricción horaria. start>end ⇒ ventana que
    -- cruza medianoche. ponytail: ventana en UTC; añadir zona horaria por-política
    -- (IANA, time.LoadLocation) cuando un cliente necesite horario local de negocio.
    days_mask      smallint NOT NULL DEFAULT 0,
    start_minute   smallint NOT NULL DEFAULT -1,
    end_minute     smallint NOT NULL DEFAULT -1,

    created_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

-- Listado admin por org ordenado por prioridad, y carga de las políticas de una org en
-- el login (WHERE org_id [AND enabled]); (priority,id) da orden estable sin sort extra.
CREATE INDEX access_policies_org_idx ON access_policies (org_id, priority, id);

CREATE TRIGGER access_policies_set_updated_at BEFORE UPDATE ON access_policies
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
