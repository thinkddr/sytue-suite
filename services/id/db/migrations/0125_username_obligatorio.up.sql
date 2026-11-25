-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El @usuario pasa a ser obligatorio: el paso de contract que la 0065 dejó para la
-- versión siguiente (ADR 0040).
--
-- La 0065 lo creó anulable porque durante aquel despliegue convivían pods que daban
-- de alta sin mandarlo, y el barrido de Sytue ID (AsignarUsernamesPendientes) se lo
-- fue dando a todas las cuentas de antes. Desde entonces store.CreateUser lo pone en
-- las seis vías de alta, así que el binario anterior a esta migración ya no inserta
-- ninguna fila sin él: la restricción no rechaza nada de lo que escribe.
--
-- NO ES UN `SET NOT NULL`, y a propósito: «Borrar mi cuenta» (mig 0098, ADR 0054)
-- anonimiza la ficha purgada y le QUITA el @usuario para que otra persona pueda
-- elegirlo. Esa es la única fila viva sin él, y la restricción lo dice tal cual.
--
-- Relleno seguro antes de validar. En producción el barrido ya no encuentra a nadie,
-- pero si quedara alguna cuenta viva sin @usuario la validación tumbaría el arranque
-- del IdP entero. A esas se les da uno que no puede pisar a nadie: `u` y los 32
-- dígitos de su id —único por construcción—, con un sufijo si alguien lo hubiera
-- elegido ya o fuera la parte local de un buzón. Es un @usuario válido (letras y
-- dígitos, 33 caracteres) y la persona lo puede cambiar desde Mi cuenta. No se copia
-- aquí la regla de nombre.apellidos del alta: vive en Go, y una segunda versión en
-- SQL divergiría.
DO $$
DECLARE
    r    record;
    cand text;
    n    int;
BEGIN
    FOR r IN SELECT id FROM users
             WHERE username IS NULL AND purged_at IS NULL
             ORDER BY created_at, id
    LOOP
        n := 0;
        LOOP
            cand := 'u' || replace(r.id::text, '-', '') || CASE WHEN n = 0 THEN '' ELSE n::text END;
            EXIT WHEN NOT EXISTS (SELECT 1 FROM users WHERE lower(username) = cand)
                  AND NOT EXISTS (SELECT 1 FROM org_mailboxes
                                  WHERE lower(split_part(address, '@', 1)) = cand);
            n := n + 1;
        END LOOP;
        UPDATE users SET username = cand WHERE id = r.id;
    END LOOP;
END $$;

-- NOT VALID primero y VALIDATE después: el ADD solo toma el cerrojo un instante, y la
-- comprobación de las filas existentes no bloquea las escrituras.
ALTER TABLE users ADD CONSTRAINT users_username_obligatorio
    CHECK (username IS NOT NULL OR purged_at IS NOT NULL) NOT VALID;
ALTER TABLE users VALIDATE CONSTRAINT users_username_obligatorio;

-- La cola del barrido ya no tiene a nadie que esperar: solo guardaría las purgadas.
DROP INDEX IF EXISTS users_username_pendiente_idx;
