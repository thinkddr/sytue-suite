# Destino: PrestaShop (Tema Child de Hummingbird)

Ficha técnica y normativa para la exportación de sitios a temas modernos de PrestaShop basados en Hummingbird.

## 1. Versiones y Herramientas

- Versión del tema: `{{v.prestashop_theme_version}}`
- Versión de PrestaShop compatible: `{{v.prestashop_version}}`
- Versión de tema padre Hummingbird: `{{v.prestashop_hummingbird_version}}`

## 2. Árbol de Directorios Permitido

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

## 3. Restricciones y Reglas de Exportación

- **Tema hijo de Hummingbird**: `config/theme.yml` declara obligatoriamente `parent: hummingbird` para garantizar compatibilidad con el estándar moderno de PrestaShop.
- **Imagen de vista previa obligatoria**: `preview.png` se genera de forma determinista mediante `image/png` reflejando el color primario del sitio.
- **Catálogo de productos**: Exportación en CSV listo para importación en el catálogo de PrestaShop.
- **Plantillas Smarty**: Las plantillas en `templates/` heredan de los bloques principales de Hummingbird sin scripts externos arbitrarios.
