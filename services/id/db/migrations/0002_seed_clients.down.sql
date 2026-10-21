-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DELETE FROM oauth_clients WHERE id IN (
    'sytue-one', 'sytue-console', 'sytue-cloud', 'sytue-cloud-api',
    'sytue-mail', 'sytue-drive', 'sytue-calendar', 'sytue-contacts',
    'sytue-docs', 'sytue-chat', 'sytue-meet', 'sytue-hub'
);
