# Destino: React (Vite + TypeScript)

Ficha técnica y normativa para la exportación de sitios a aplicaciones SPA en React con Vite y TypeScript.

## 1. Versiones y Herramientas

- React: `{{v.react_version}}`
- React DOM: `{{v.react_dom_version}}`
- Vite: `{{v.vite_version}}`
- Plugin React para Vite: `{{v.vite_plugin_react_version}}`
- TypeScript: `{{v.typescript_version}}`
- Tipos de React: `{{v.types_react_version}}`
- Tipos de React DOM: `{{v.types_react_dom_version}}`
- Tipos de Node: `{{v.types_node_version}}`

## 2. Árbol de Directorios Permitido

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

## 3. Restricciones y Reglas de Exportación

- **Conexión a la API Pública**: Todas las operaciones dinámicas (catálogo, disponibilidad, reservas de compra, formularios de contacto y denuncias) se realizan exclusivamente a través de `src/api/pages-api.ts`.
- **Dependencias exactas**: `package.json` declara versiones fijas y exactas de acuerdo con la tabla normativa de dependencias, sin comodines ni prefijos de rango.
- **Sin CDN ni recursos externos no permitidos**: Prohibido enlazar scripts externos o estilos desde CDNs en `index.html`.
- **Resolución de ALTCHA en cliente**: El componente `Formulario.tsx` y el flujo de compra resuelven retos ALTCHA en segundo plano mediante `resolverAltcha` antes de enviar peticiones mutables.
