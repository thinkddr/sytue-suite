<a id="destino-react-vite--typescript"></a>
# Target: React (Vite + TypeScript)

Technical and normative specification for exporting sites to React SPA applications with Vite and TypeScript.

<a id="1-versiones-y-herramientas"></a>
## 1. Versions and Tools

- React: `{{v.react_version}}`
- React DOM: `{{v.react_dom_version}}`
- Vite: `{{v.vite_version}}`
- React plugin for Vite: `{{v.vite_plugin_react_version}}`
- TypeScript: `{{v.typescript_version}}`
- React types: `{{v.types_react_version}}`
- React DOM types: `{{v.types_react_dom_version}}`
- Node types: `{{v.types_node_version}}`

<a id="2-árbol-de-directorios-permitido"></a>
## 2. Allowed Directory Tree

```text
├── index.html
├── package.json
├── tsconfig.json
├── tsconfig.node.json
├── vite.config.ts
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── index.css
│   ├── api/
│   │   └── pages-api.ts
│   └── components/
│       ├── Formulario.tsx
│       ├── BotonComprar.tsx
│       └── pages-<kind>.tsx
└── README.md
```

<a id="3-restricciones-y-reglas-de-exportación"></a>
## 3. Export Restrictions and Rules

- **Connection to the Public API**: All dynamic operations (catalog, availability, purchase reservations, contact forms and whistleblower reports) are performed exclusively through `src/api/pages-api.ts`.
- **Exact dependencies**: `package.json` declares fixed, exact versions according to the normative dependency table, with no wildcards or range prefixes.
- **No CDNs or disallowed external resources**: Linking external scripts or styles from CDNs in `index.html` is forbidden.
- **Client-side ALTCHA resolution**: The `Formulario.tsx` component and the purchase flow solve ALTCHA challenges in the background via `resolverAltcha` before sending mutating requests.
