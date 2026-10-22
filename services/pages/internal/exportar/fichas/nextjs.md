# Destino: Next.js (App Router + TypeScript)

Ficha técnica y normativa para la exportación de sitios a proyectos Next.js con App Router y TypeScript.

## 1. Versiones y Herramientas

- Next.js: `{{v.nextjs_version}}`
- React: `{{v.react_version}}`
- React DOM: `{{v.react_dom_version}}`
- TypeScript: `{{v.typescript_version}}`
- Tipos de React: `{{v.types_react_version}}`
- Tipos de React DOM: `{{v.types_react_dom_version}}`
- Tipos de Node: `{{v.types_node_version}}`

## 2. Árbol de Directorios Permitido

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

## 3. Restricciones y Reglas de Exportación

- **Conexión a la API Pública**: La comunicación con la tienda y los formularios públicos delegados utiliza el cliente `lib/pages-api.ts`.
- **Estructura App Router**: Emplea el enrutador estándar App Router de Next.js.
- **Dependencias exactas**: `package.json` declara versiones estrictas y coincidentes con la tabla normativa de versiones.
- **Validación de orígenes y seguridad**: Los formularios integran el resolvedor de ALTCHA del lado del navegador para validar envíos.
