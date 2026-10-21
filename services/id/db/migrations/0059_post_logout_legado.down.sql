-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Repone el destino de cierre de cada cliente que lo tenía. Por cliente y no a
-- lo bruto: añadir `chat` al de Admin autorizaría un destino que nunca existió.
UPDATE oauth_clients SET post_logout_redirect_uris = post_logout_redirect_uris || ARRAY['https://chat.sytue.eu/'], updated_at = now()
WHERE id = 'sytue-chat' AND NOT (post_logout_redirect_uris && ARRAY['https://chat.sytue.eu/']);

UPDATE oauth_clients SET post_logout_redirect_uris = post_logout_redirect_uris || ARRAY['https://meet.sytue.eu/'], updated_at = now()
WHERE id = 'sytue-meet' AND NOT (post_logout_redirect_uris && ARRAY['https://meet.sytue.eu/']);

UPDATE oauth_clients SET post_logout_redirect_uris = post_logout_redirect_uris || ARRAY['https://console.sytue.eu/'], updated_at = now()
WHERE id = 'sytue-console' AND NOT (post_logout_redirect_uris && ARRAY['https://console.sytue.eu/']);
