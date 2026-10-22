-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- La otra mitad de la 0041: los destinos de CIERRE de sesión.
--
-- Aquella retiró `chat`, `meet` y `console` de `redirect_uris` y dejó intacto
-- `post_logout_redirect_uris`. No es lo mismo pero se le parece bastante: un
-- destino de cierre registrado es un sitio al que Sytue ID acepta MANDAR a la
-- persona al terminar la sesión, con la confianza de venir de nuestro dominio.
-- No lleva código de autorización —por eso no es tan grave como un callback—,
-- pero sigue siendo una redirección que nosotros firmamos hacia un nombre que
-- desde el 2026-09-11 ya no es nuestro.
--
-- MISMA GUARDA QUE LA 0041: nunca dejar a un cliente sin ningún destino de
-- cierre. Quedarse con la lista vacía convierte un `end_session` normal en un
-- error, y ninguno de estos hosts es el único de nadie.
UPDATE oauth_clients
SET post_logout_redirect_uris = ARRAY(
        SELECT u FROM unnest(post_logout_redirect_uris) AS u
        WHERE u !~ '^https://(chat|meet|console)\.sytue\.eu(/|$)'
    ),
    updated_at = now()
WHERE managed_by = 'static'
  AND EXISTS (
        SELECT 1 FROM unnest(post_logout_redirect_uris) AS u
        WHERE u !~ '^https://(chat|meet|console)\.sytue\.eu(/|$)'
      )
  AND EXISTS (
        SELECT 1 FROM unnest(post_logout_redirect_uris) AS u
        WHERE u ~ '^https://(chat|meet|console)\.sytue\.eu(/|$)'
      );
