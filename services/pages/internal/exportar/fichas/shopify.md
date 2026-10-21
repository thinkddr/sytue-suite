# Destino: Shopify (Theme Store & Online Store 2.0)

Ficha técnica y normativa para la exportación de sitios a temas de Shopify Online Store 2.0.

## 1. Versiones y Herramientas

- Versión del tema: `{{v.shopify_theme_version}}`
- Herramienta CLI de desarrollo: `{{v.shopify_cli_version}}`
- Linter oficial: `{{v.shopify_theme_check_version}}`

## 2. Árbol de Directorios Permitido

El tema exportado debe generar exclusivamente los siguientes directorios y archivos:

```text
├── assets/
│   └── theme.css
├── config/
│   ├── settings_schema.json
│   └── settings_data.json
├── layout/
│   └── theme.liquid
├── locales/
│   └── en.default.json
├── sections/
│   ├── header-group.json
│   ├── footer-group.json
│   └── pages-<kind>.liquid
├── templates/
│   ├── index.json
│   ├── product.json
│   ├── collection.json
│   ├── cart.json
│   └── 404.json
└── README.md
```

## 3. Restricciones y Límites Normativos de Shopify

- **Niveles de anidamiento**: Máximo 8 niveles de profundidad en el árbol de bloques de una sección. Si una sección del IR supera 8 niveles, la instrucción a la IA y el generador aplanan el contenido como HTML directo dentro del bloque padre.
- **Tope de bloques por sección**: Máximo 50 bloques por sección.
- **Longitud de identificadores**:
  - `type` de bloques: Máximo 25 caracteres.
  - `label` o nombres descriptivos en esquemas: Máximo 50 caracteres.
- **Esquema de configuraciones (`settings`)**:
  - Parámetros `range`: Valores enteros comprendidos entre 1 y 250.
- **Puntos de anclaje obligatorios en `layout/theme.liquid`**:
  - Elemento `{{ content_for_header }}` dentro de `<head>`.
  - Elemento `{{ content_for_layout }}` dentro de `<body>`.
- **Integridad referencial**:
  - Toda sección referenciada en `templates/*.json` debe existir como archivo `.liquid` o grupo `.json`.
  - Todo bloque referenciado en la configuración de una sección debe estar declarado en el `schema.blocks` correspondiente.
  - Toda clave de traducción `t:clave` o `{{ 'clave' | t }}` debe existir en el archivo `locales/en.default.json`.
- **Información del tema**:
  - El archivo `config/settings_schema.json` debe contener una categoría `theme_info` con nombre, versión `{{v.shopify_theme_version}}`, autor y documentación.
- **Sin scripts ni fuentes externas no autorizadas**:
  - Prohibido enlazar hojas de estilo o scripts externos desde CDNs o Google Fonts. Todo el CSS y los recursos van empaquetados localmente en `assets/` o usan fuentes del sistema.

## 4. Gestión de Idiomas y Contenido

- El contenido principal del sitio se exporta en el idioma principal (`languages.default`) como configuraciones editables del tema.
- `locales/en.default.json` contiene únicamente los textos y etiquetas del sistema del tema.
- Las traducciones adicionales se gestionan a través de las capacidades nativas multi-idioma de Shopify (Shopify Translate & Adapt), como se documenta en el `README.md` del archivo zip.

## 5. Catálogo de Productos

- Los productos de la tienda se exportan como archivo independiente `products_export_shopify.csv` listo para importar en el panel de administración de Shopify.
