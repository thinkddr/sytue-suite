<a id="destino-woocommerce-wordpress-block-theme"></a>
# Target: WooCommerce (WordPress Block Theme)

Technical and normative spec sheet for exporting sites to block themes for WordPress and WooCommerce.

<a id="1-versiones-y-herramientas"></a>
## 1. Versions and Tools

- Theme version: `{{v.woocommerce_theme_version}}`
- Compatible WooCommerce version: `{{v.woocommerce_version}}`
- Minimum WordPress version: `{{v.wordpress_version}}`

<a id="2-árbol-de-directorios-permitido"></a>
## 2. Allowed Directory Tree

```text
├── style.css
├── functions.php
├── theme.json
├── screenshot.png
├── templates/
│   ├── index.html
│   ├── page.html
│   └── single-product.html
├── parts/
│   ├── header.html
│   └── footer.html
├── products_export_woocommerce.csv
└── README.md
```

<a id="3-restricciones-y-reglas-de-exportación"></a>
## 3. Export Constraints and Rules

- **Block theme and theme.json**: The theme is structured as an FSE (Full Site Editing) block theme using version 3 of the `theme.json` specification.
- **Balanced block comments**: Every HTML template contains balanced WordPress block comments (`<!-- wp:nombre -->` and `<!-- /wp:nombre -->`, or self-closing `<!-- wp:nombre /-->`).
- **WooCommerce support**: `functions.php` declares `add_theme_support('woocommerce')` and compatibility with block templates.
- **Preview image**: `screenshot.png` is generated deterministically with `image/png` using the site's main color palette.
- **Product catalog**: CSV export ready for WooCommerce's native importer.
