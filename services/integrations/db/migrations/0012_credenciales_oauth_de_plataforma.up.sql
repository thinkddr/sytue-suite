-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Credenciales con las que SYTUE se presenta ante cada proveedor de terceros.
--
-- QUÉ RESUELVE. Con una app registrada por Sytue ante un proveedor, conectar es
-- UN CLIC: el admin del cliente solo aprueba la pantalla de permisos. Sin ella
-- todo queda en modo BYO, y cada organización tiene que darse de alta como
-- desarrollador ante Google o Microsoft, declarar el redirect_uri y pegar aquí
-- su client_id y su secret. Sigue siendo mejor que un token estático, pero no es
-- un clic, y para migrar el correo de una empresa es una barrera enorme.
--
-- POR QUÉ EN LA BASE Y NO EN EL ENTORNO. Hasta ahora esto vivía SOLO en
-- SYTUE_INTEGRATIONS_OAUTH_CLIENTS, un JSON en el chart: cambiar una credencial
-- exigía editar el despliegue y reiniciar los pods, y por eso en producción
-- estaba sencillamente vacío. En una tabla se edita desde la superficie de
-- plataforma, queda constancia de quién la tocó, y rotar un secret es un
-- formulario en vez de una release.
--
-- EL SECRET VA CIFRADO con la misma KEK que el resto de secretos del servicio
-- (keycrypto, SYTUE_INTEGRATIONS_KEY_ENCRYPTION_KEY). El client_id NO: viaja en
-- la URL de autorización a la vista de cualquiera, y tratarlo como secreto solo
-- serviría para no poder enseñarlo en la interfaz, que es justo donde hace falta
-- para comprobar que coincide con lo registrado ante el proveedor.
--
-- SIN RLS y sin `tenant`, a propósito: esto NO es de una organización, es de la
-- plataforma. Solo lo toca la superficie de operador, que va autenticada aparte
-- del JWT de la suite. Que no tenga columna de tenant es lo que impide que una
-- consulta de negocio la alcance por descuido.

CREATE TABLE platform_oauth_clients (
    -- Clave de la plantilla del catálogo ("google-workspace", "microsoft365"…).
    -- Sin FK a app_templates: una credencial puede registrarse ANTES de que la
    -- plantilla se siembre, y perderla porque el catálogo aún no ha arrancado
    -- sería exactamente el problema que esta tabla viene a quitar de en medio.
    template_key      text PRIMARY KEY CHECK (length(btrim(template_key)) > 0),
    client_id         text NOT NULL CHECK (length(btrim(client_id)) > 0),
    -- Cifrado con keycrypto. NULL es VÁLIDO: hay proveedores que registran la app
    -- como cliente público y se apoyan solo en PKCE.
    client_secret_enc text,
    -- Quién la dejó así. Es un correo y no un uuid porque el usuario vive en la
    -- base de Sytue ID, no en esta, y una FK entre bases no existe; para auditar
    -- una credencial, saber a quién preguntar vale más que un identificador que
    -- aquí no resuelve nada.
    updated_by        text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- `updated_at` lo pone el UPDATE, no un trigger: este servicio no tiene la
-- función `set_updated_at` que sí tienen otros, y añadirla solo para una tabla
-- sería traerse una convención ajena a media migración.

-- El rol de aplicación NO recibe permisos sobre esta tabla: se lee y escribe con
-- el rol del servicio, fuera de las transacciones por-org. Una credencial de
-- plataforma no debe ser alcanzable desde una petición de un cliente ni aunque
-- una política de RLS se escriba mal algún día.
