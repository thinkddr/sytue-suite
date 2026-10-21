-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Al revertir se pierden las credenciales registradas desde la superficie de
-- plataforma. El servicio sigue funcionando —vuelve a leer solo
-- SYTUE_INTEGRATIONS_OAUTH_CLIENTS del entorno—, pero los proveedores
-- configurados por la interfaz dejan de tener app de Sytue y sus conexiones
-- pasan a modo BYO. Es recuperable volviéndolas a introducir, y no hay forma de
-- conservarlas sin la tabla.

DROP TABLE IF EXISTS platform_oauth_clients;
