// Spec e2e de Sytue DevX (cloud.sytue.localhost). Sesión admin vía storageState
// del contrato (dependencies: ['setup']); no crea datos, así que es re-ejecutable.
import { test, expect } from '@playwright/test';
import { gotoApp, expectNoConsoleErrors, sidebarNav } from './helpers';

test.describe('Cloud — shell y proyectos', () => {
  test('renderiza el shell con navegación global, región UE y sin errores de consola', async ({
    page,
  }) => {
    // La primera compilación de la ruta en el dev-server Astro puede superar
    // los 45 s del timeout base; slow() lo triplica (convención de la suite,
    // ver console.spec.ts). En caliente no añade espera alguna.
    test.slow();
    const sinErrores = expectNoConsoleErrors(page); // antes de navegar
    await gotoApp(page, 'cloud');

    // Navegación global del sidebar (nav de marca de @sytue/ui).
    const sidebar = sidebarNav(page);
    await expect(sidebar).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'Proyectos', exact: true })).toBeVisible();
    // «Uso» y «Facturación» son dos entradas separadas del nav; antes eran una
    // sola («Uso y facturación», que hoy es solo el TÍTULO de la página /usage).
    await expect(sidebar.getByRole('link', { name: 'Uso', exact: true })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: /Facturaci[oó]n/ })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'Ajustes', exact: true })).toBeVisible();

    // Región UE visible (pie del sidebar).
    await expect(sidebar.getByText(/Regi[oó]n fr-par · 100 % UE/)).toBeVisible();

    sinErrores();
  });

  test('la vista Proyectos muestra su estado (vacío o lista)', async ({ page }) => {
    test.slow(); // arranque en frío del dev-server (ver arriba)
    await gotoApp(page, 'cloud');

    // exact: el nombre de rol se compara por subcadena por defecto y el título
    // del EmptyState («Aún no hay proyectos», un h3) también contiene
    // «proyectos» → violación de modo estricto en el estado vacío.
    await expect(page.getByRole('heading', { name: 'Proyectos', exact: true })).toBeVisible();

    // Estado válido: tarjetas de proyecto (enlaces a /projects/<slug>) o el
    // estado vacío de ProjectsList — el seed degrada a 3 proyectos sin backend.
    // Ambos localizados dentro de <main> (excluye la navegación del sidebar).
    const main = page.locator('main');
    const tarjetas = main.locator('a[href^="/projects/"]');
    const vacio = main.getByText('Aún no hay proyectos');
    await expect(tarjetas.first().or(vacio)).toBeVisible();
  });
});
