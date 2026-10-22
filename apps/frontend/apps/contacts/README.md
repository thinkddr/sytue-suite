# Sytue Contacts

Contactos propios de la suite **Sytue One** en **Astro 7 + Preact 10** sobre el
servicio Go propio **Sytue Contacts** (estándar **CardDAV** RFC 6352 + API REST;
estado en PostgreSQL; soberanía 100 % UE, Scaleway). Lista filtrable y ficha de
contacto, crear / editar / eliminar. Lenguaje visual **Sytue**, modo claro/oscuro y
WCAG AA del design system.

Monta la **Sytue Shell** (`@sytue/shell`) y usa exclusivamente `@sytue/ui`.

## Arquitectura

- **BFF + OIDC/PKCE** (`@sytue/auth`): `src/pages/auth/*` (client `sytue-contacts`
  contra **Sytue ID**). Cookie de sesión httpOnly `.sytue.eu`.
- **BFF CardDAV** (`src/pages/api/*`): proxy same-origin (cookie) → servicio Go
  **Sytue Contacts** con el access token Bearer de Sytue ID, vía
  `@sytue/api-clients/carddav`. Endpoints: `addressbooks`, `contacts`, `contact`
  (POST crear/editar, DELETE eliminar). El cuerpo vCard 4.0 lo genera el servidor
  (`src/lib/vcard.ts`), no el cliente.
- **Islas Preact** (`src/components/*`): `ContactsApp` (raíz), `ContactDetail`
  (ficha, con acción cross-app _Enviar correo_ → Sytue Mail), `ContactModal`
  (crear/editar/eliminar).

## Desarrollo

Puerto dev `4328` (ver `docs/CONVENTIONS.md` §6); dominio `contacts.sytue.localhost`.
Runtime/instalación/bundler con **Bun** (sustituye a Node.js + pnpm).

```bash
bun install
bun --filter @sytue/contacts dev
```

Copia `.env.example` a `.env` (sólo placeholders; secretos reales fuera de git).

## Producción

`Dockerfile` multi-stage (Bun, salida `standalone` de Astro). Desplegado por el
umbrella chart `sytue-one` en Kapsule (Scaleway, regiones UE).
