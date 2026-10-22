/**
 * DeniedState — cuerpo de la página cuando el control-plane responde 401/403.
 *
 * Sustituye al contenido, no se apila encima. Antes bastaba con el banner rojo de
 * `DemoBanner` y debajo seguía pintándose la página degradada: las OCHO rutas de
 * Cloud mostraban el aviso de «no tienes permiso» y a la vez el asistente de tres
 * pasos con «Conectar repositorio», «Invitar miembro», «Crear token», «Guardar» y
 * los importes del seed (0,00 €, 19,00 €/mes, 49,00 €/mes) como si fueran del
 * cliente. Ninguna de esas acciones puede funcionar sin permiso y ninguna de esas
 * cifras es suya, así que no se pintan.
 *
 * Se monta desde `DashboardLayout` en vez del `<slot />`: el contenido slotted de
 * Astro es una función perezosa, así que ni se evalúa ni llega al DOM (las islas
 * tampoco se hidratan). Un único sitio que decide, imposible dejarlo a medias.
 *
 * Sin JS: se renderiza en el servidor (no lleva `client:*`).
 */
import { Card, CardBody, EmptyState, Icon } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';

export default function DeniedState({ locale }: { locale: Locale }) {
  const t = useT(locale);
  return (
    <div class="cloud-content">
      <Card>
        <CardBody class="p-0">
          <EmptyState
            illustration={<Icon name="lock" size={24} />}
            title={t('denied.title')}
            description={t('demo.denied')}
          />
        </CardBody>
      </Card>
    </div>
  );
}
