-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LA IP DE ACCESO DEJA DE GUARDARSE EN CLARO.
--
-- Hasta aquí, `sessions.ip` era una columna `inet` legible por cualquiera con
-- acceso a la base de datos: el equipo, una copia de seguridad, o quien se
-- llevara un volcado. Para un producto que se vende como privado por defecto,
-- eso era exactamente lo contrario de lo prometido.
--
-- Ahora se guardan cifradas con una clave DERIVADA DEL TOKEN DE SESIÓN. Y el
-- token no está en esta tabla: solo su SHA-256 (columna token_hash). El token
-- vive únicamente en la cookie del navegador de su dueño, así que estos dos
-- campos son ruido para cualquiera que mire la base de datos sin esa cookie.
--
-- SE BORRA LO YA GUARDADO. La columna `ip` se elimina con sus datos en lugar de
-- migrarlos: cifrar lo que ya está en claro no lo vuelve privado —la copia de
-- seguridad de ayer lo sigue teniendo— y conservarlo «por si acaso» sería
-- guardar justo lo que se dice no guardar. Se pierde el historial de IP
-- anterior, que es el precio correcto de la promesa.
BEGIN;

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS ip_cifrada    bytea,
    ADD COLUMN IF NOT EXISTS lugar_cifrado bytea;

-- La columna `ip` NO se borra aquí, y no es un olvido: durante un despliegue
-- conviven pods de la versión anterior, que todavía hacen
-- `INSERT INTO sessions (..., ip, ...)`. Borrarla en el mismo paso que se añaden
-- las cifradas rompe cada login contra un pod aún no reemplazado, y deja un
-- rollback de Helm sin vuelta. Es expand ahora y contract después: la retirada
-- va en una migración posterior, cuando la versión que dejó de escribirla lleve
-- desplegada al menos una release. El código de ESTA ya no la escribe, así que
-- lo que queda en la columna se congela en lo que hubiera.

COMMENT ON COLUMN sessions.ip_cifrada IS
    'IP de acceso cifrada (AES-256-GCM) con clave derivada del token de sesión. '
    'NO descifrable sin la cookie del usuario: el token no se almacena, solo su hash.';
COMMENT ON COLUMN sessions.lugar_cifrado IS
    'Ubicación aproximada, cifrada igual que ip_cifrada. Se resuelve al crear la '
    'sesión y se cifra en el acto; la IP no vuelve a tocar el disco en claro.';

COMMIT;
