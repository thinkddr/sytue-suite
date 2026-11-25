<a id="destino-prestashop-tema-child-de-hummingbird"></a>
# Destination: PrestaShop (Hummingbird Child Theme)

Technical and normative sheet for exporting sites to modern PrestaShop themes based on Hummingbird.

<a id="1-versiones-y-herramientas"></a>
## 1. Versions and Tools

- Theme version: `{{v.prestashop_theme_version}}`
- Compatible PrestaShop version: `{{v.prestashop_version}}`
- Hummingbird parent theme version: `{{v.prestashop_hummingbird_version}}`

<a id="2-árbol-de-directorios-permitido"></a>
## 2. Allowed Directory Tree

```text
├── config/
│   └── theme.yml
├── preview.png
├── templates/
│   ├── index.tpl
│   ├── layout.tpl
│   └── catalog/
├── assets/
│   └── css/
│       └── custom.css
├── products_export_prestashop.csv
└── README.md
```

<a id="3-restricciones-y-reglas-de-exportación"></a>
## 3. Export Restrictions and Rules

- **Hummingbird child theme**: `config/theme.yml` must declare `parent: hummingbird` to guarantee compatibility with the modern PrestaShop standard.
- **Mandatory preview image**: `preview.png` is generated deterministically with `image/png`, reflecting the site's primary color.
- **Product catalog**: CSV export ready to be imported into the PrestaShop catalog.
- **Smarty templates**: The templates in `templates/` inherit from Hummingbird's main blocks with no arbitrary external scripts.
