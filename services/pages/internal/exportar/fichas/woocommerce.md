# Destino: WooCommerce (WordPress Block Theme)

Ficha técnica y normativa para la exportación de sitios a temas de bloques para WordPress y WooCommerce.

## 1. Versiones y Herramientas

- Versión del tema: `{{v.woocommerce_theme_version}}`
- Versión de WooCommerce compatible: `{{v.woocommerce_version}}`
- Versión mínima de WordPress: `{{v.wordpress_version}}`

## 2. Árbol de Directorios Permitido

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

## 3. Restricciones y Reglas de Exportación

- **Tema de Bloques y theme.json**: El tema se estructura como tema de bloques FSE (Full Site Editing) con especificación versión 3 de `theme.json`.
- **Comentarios de bloque equilibrados**: Toda plantilla HTML contiene comentarios de bloques WordPress equilibrados (`<!-- wp:nombre -->` y `<!-- /wp:nombre -->` o auto-cerrados `<!-- wp:nombre /-->`).
- **Soporte de WooCommerce**: `functions.php` declara `add_theme_support('woocommerce')` y compatibilidad con plantillas de bloques.
- **Imagen de vista previa**: `screenshot.png` se genera de forma determinista con `image/png` utilizando la paleta cromática principal del sitio.
- **Catálogo de productos**: Exportación en CSV listo para el importador nativo de WooCommerce.
