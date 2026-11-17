<a id="destino-nextjs-app-router--typescript"></a>
# Target: Next.js (App Router + TypeScript)

Technical and normative spec sheet for exporting sites to Next.js projects with App Router and TypeScript.

<a id="1-versiones-y-herramientas"></a>
## 1. Versions and Tools

- Next.js: `{{v.nextjs_version}}`
- React: `{{v.react_version}}`
- React DOM: `{{v.react_dom_version}}`
- TypeScript: `{{v.typescript_version}}`
- React types: `{{v.types_react_version}}`
- React DOM types: `{{v.types_react_dom_version}}`
- Node types: `{{v.types_node_version}}`

<a id="2-árbol-de-directorios-permitido"></a>
## 2. Allowed Directory Tree

```text
├── package.json
├── tsconfig.json
├── next.config.ts
├── app/
│   ├── layout.tsx
│   ├── page.tsx
│   └── globals.css
├── lib/
│   └── pages-api.ts
├── components/
│   ├── Formulario.tsx
│   ├── BotonComprar.tsx
│   └── pages-<kind>.tsx
└── README.md
```

<a id="3-restricciones-y-reglas-de-exportación"></a>
## 3. Export Constraints and Rules

- **Public API connection**: Communication with the store and the delegated public forms uses the `lib/pages-api.ts` client.
- **App Router structure**: Uses the standard Next.js App Router.
- **Exact dependencies**: `package.json` declares strict versions that match the normative version table.
- **Origin validation and security**: Forms embed the browser-side ALTCHA solver to validate submissions.
