/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DÓNDE SE SACAN LAS CREDENCIALES DE CADA PROVEEDOR.
 *
 * Los PASOS son los mismos en todos (crear una app web, declarar la URL de
 * vuelta, copiar el par de credenciales), así que están traducidos una sola vez
 * en el catálogo de mensajes. Lo único que cambia por proveedor es dónde vive esa
 * pantalla, y eso es lo que hay aquí.
 *
 * `donde` y `tipo` van EN INGLÉS a propósito: no son texto de Sytue sino los
 * nombres literales de los menús del proveedor, y traducirlos manda a buscar
 * «Credenciales» en una consola que dice «Credentials». Es el mismo criterio que
 * `client_id`.
 */

export interface GuiaProveedor {
  /** Enlace directo a donde se crean las credenciales, sin pasos intermedios. */
  consola: string;
  /** Migas dentro de esa consola, tal y como se llaman allí. */
  donde: string;
  /** Tipo de aplicación que hay que elegir, cuando el proveedor pregunta. */
  tipo?: string;
}

/**
 * Solo los proveedores en los que Sytue registra una app propia. El que no esté
 * cae en la guía genérica: los cuatro pasos sin el enlace directo, que es
 * bastante más útil que no decir nada.
 */
export const GUIAS: Record<string, GuiaProveedor> = {
  'google-workspace': {
    consola: 'https://console.cloud.google.com/apis/credentials',
    donde: 'APIs & Services → Credentials → Create credentials → OAuth client ID',
    tipo: 'Web application',
  },
  microsoft365: {
    consola: 'https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade',
    donde: 'App registrations → New registration → Certificates & secrets',
    tipo: 'Web',
  },
  'microsoft-teams': {
    consola: 'https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade',
    donde: 'App registrations → New registration → Certificates & secrets',
    tipo: 'Web',
  },
  azure: {
    consola: 'https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade',
    donde: 'App registrations → New registration → Certificates & secrets',
    tipo: 'Web',
  },
  github: {
    consola: 'https://github.com/settings/developers',
    donde: 'Developer settings → OAuth Apps → New OAuth App',
  },
  'jira-cloud': {
    consola: 'https://developer.atlassian.com/console/myapps/',
    donde: 'Create → OAuth 2.0 integration → Settings',
  },
  salesforce: {
    consola: 'https://login.salesforce.com/lightning/setup/NavigationMenus/home',
    donde: 'Setup → App Manager → New Connected App → Enable OAuth Settings',
  },
  servicenow: {
    consola: 'https://developer.servicenow.com/',
    donde: 'System OAuth → Application Registry → New → Third party OAuth client',
  },
  databricks: {
    consola: 'https://accounts.cloud.databricks.com/',
    donde: 'Settings → App connections → Add connection',
  },
  slack: {
    consola: 'https://api.slack.com/apps',
    donde: 'Create New App → OAuth & Permissions → Basic Information',
  },
  notion: {
    consola: 'https://www.notion.so/my-integrations',
    donde: 'New integration → Type: Public → OAuth Domain & URIs',
  },
  dropbox: {
    consola: 'https://www.dropbox.com/developers/apps',
    donde: 'Create app → Settings → OAuth 2 redirect URIs',
  },
  box: {
    consola: 'https://app.box.com/developers/console',
    donde: 'Create Platform App → Custom App → Configuration',
    tipo: 'Standard OAuth 2.0 (User Authentication)',
  },
  zoom: {
    consola: 'https://marketplace.zoom.us/develop/create',
    donde: 'Build App → General App → OAuth',
  },
  hubspot: {
    consola: 'https://developers.hubspot.com/',
    donde: 'Manage apps → Create app → Auth',
  },
  asana: {
    consola: 'https://app.asana.com/0/my-apps',
    donde: 'Create new app → OAuth → Redirect URLs',
  },
  monday: {
    consola: 'https://monday.com/developers/apps',
    donde: 'Create app → OAuth → Redirect URLs',
  },
  trello: {
    consola: 'https://trello.com/power-ups/admin',
    donde: 'New → Power-Up → API key',
  },
  zendesk: {
    consola: 'https://www.zendesk.com/login/',
    donde: 'Admin Center → Apps and integrations → OAuth clients',
  },
  okta: {
    consola: 'https://login.okta.com/',
    donde: 'Admin → Applications → Create App Integration → OIDC',
    tipo: 'Web Application',
  },
  miro: {
    consola: 'https://miro.com/app/settings/user-profile/apps',
    donde: 'Create new app → App Credentials',
  },
  confluence: {
    consola: 'https://developer.atlassian.com/console/myapps/',
    donde: 'Create → OAuth 2.0 integration → Settings',
  },
  docusign: {
    consola: 'https://apps.docusign.com/admin/apps-and-keys',
    donde: 'Apps and Keys → Add App and Integration Key → Authentication',
  },
  airtable: {
    consola: 'https://airtable.com/create/oauth',
    donde: 'Developer hub → OAuth integrations → Register new OAuth integration',
  },
  powerbi: {
    consola: 'https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade',
    donde: 'App registrations → New registration → API permissions → Power BI Service',
    tipo: 'Web',
  },
};

/** Guía del proveedor, o `undefined` si no hay una escrita para él. */
export function guiaDe(key: string): GuiaProveedor | undefined {
  return GUIAS[key];
}
