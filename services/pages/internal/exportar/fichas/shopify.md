<a id="destino-shopify-theme-store--online-store-20"></a>
# Target: Shopify (Theme Store & Online Store 2.0)

Technical and normative specification for exporting sites to Shopify Online Store 2.0 themes.

<a id="1-versiones-y-herramientas"></a>
## 1. Versions and Tools

- Theme version: `{{v.shopify_theme_version}}`
- Development CLI tool: `{{v.shopify_cli_version}}`
- Official linter: `{{v.shopify_theme_check_version}}`

<a id="2-árbol-de-directorios-permitido"></a>
## 2. Allowed Directory Tree

The exported theme must generate only the following directories and files:

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

<a id="3-restricciones-y-límites-normativos-de-shopify"></a>
## 3. Shopify Restrictions and Normative Limits

- **Nesting levels**: Maximum of 8 levels of depth in a section's block tree. If an IR section exceeds 8 levels, the AI instruction and the generator flatten the content as direct HTML inside the parent block.
- **Block cap per section**: Maximum of 50 blocks per section.
- **Identifier length**:
  - Block `type`: Maximum of 25 characters.
  - `label` or descriptive names in schemas: Maximum of 50 characters.
- **Settings schema (`settings`)**:
  - `range` parameters: Integer values between 1 and 250.
- **Mandatory anchor points in `layout/theme.liquid`**:
  - The `{{ content_for_header }}` element inside `<head>`.
  - The `{{ content_for_layout }}` element inside `<body>`.
- **Referential integrity**:
  - Every section referenced in `templates/*.json` must exist as a `.liquid` file or a `.json` group.
  - Every block referenced in a section's settings must be declared in the corresponding `schema.blocks`.
  - Every translation key `t:clave` or `{{ 'clave' | t }}` must exist in the `locales/en.default.json` file.
- **Theme information**:
  - The `config/settings_schema.json` file must contain a `theme_info` category with name, version `{{v.shopify_theme_version}}`, author and documentation.
- **No unauthorized external scripts or fonts**:
  - Linking external stylesheets or scripts from CDNs or Google Fonts is forbidden. All CSS and assets are packaged locally in `assets/` or use system fonts.

<a id="4-gestión-de-idiomas-y-contenido"></a>
## 4. Language and Content Management

- The site's main content is exported in the primary language (`languages.default`) as editable theme settings.
- `locales/en.default.json` contains only the theme's system texts and labels.
- Additional translations are managed through Shopify's native multi-language capabilities (Shopify Translate & Adapt), as documented in the `README.md` of the zip file.

<a id="5-catálogo-de-productos"></a>
## 5. Product Catalog

- The store's products are exported as a standalone `products_export_shopify.csv` file ready to import into the Shopify admin panel.
