// SPDX-License-Identifier: Apache-2.0
import { expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import {
  COLOR_KEY, FORMA_KEY, STORAGE_KEY,
} from '../../../apps/frontend/packages/ui/src/theme/init.ts';

/** Preferencias reales de la suite antes de cargar cualquier isla del fixture. */
export async function prepareFixtureTheme(context, mode) {
  await context.addInitScript(
    ({ mode, keys }) => {
      // El contexto puede crear primero about:blank, sin almacenamiento por origen.
      if (!['http:', 'https:'].includes(location.protocol)) return;
      localStorage.setItem(keys.mode, mode);
      localStorage.setItem(keys.color, 'app');
      localStorage.setItem(keys.forma, 'marco');
    },
    { mode, keys: { mode: STORAGE_KEY, color: COLOR_KEY, forma: FORMA_KEY } },
  );
}

/** Verifica lo aplicado por el init/ThemeProvider real; nunca fuerza atributos. */
export async function assertFixtureTheme(page, mode) {
  const html = page.locator('html');
  await expect(html).toHaveAttribute('data-theme', mode);
  await expect(html).toHaveAttribute('data-color', 'app');
  // Marco se representa sin atributo, según applyThemeForma del design system.
  await expect.poll(() => html.getAttribute('data-forma')).toBeNull();
}

/** Astro dev may replace an optimized dependency during the first hydration. */
export async function openWizardFixture(page, url, readySelector = 'main') {
  let outdatedDependency = false;
  const observe = (response) => {
    if (response.status() === 504 && response.statusText() === 'Outdated Optimize Dep')
      outdatedDependency = true;
  };
  page.on('response', observe);
  try {
    for (let attempt = 0; ; attempt++) {
      outdatedDependency = false;
      await page.goto(url, { timeout: 180000 });
      try {
        await page.locator(readySelector).waitFor({ state: 'visible', timeout: 60000 });
        return;
      } catch (failure) {
        if (!outdatedDependency || attempt >= 2) throw failure;
      }
    }
  } finally {
    page.off('response', observe);
  }
}

/** Only synthetic fixture data: enough context to diagnose a failed remote interaction. */
export async function describeWizardState({ page }) {
  return page.evaluate(() => {
    const bounds = (element) => {
      if (!element) return null;
      const { x, y, width, height, top, right, bottom, left } = element.getBoundingClientRect();
      return { x, y, width, height, top, right, bottom, left };
    };
    const dialog = document.querySelector('[role="dialog"]');
    const footer = dialog?.lastElementChild;
    return {
      url: location.href,
      viewport: { width: innerWidth, height: innerHeight },
      visualViewport: window.visualViewport ? {
        width: window.visualViewport.width, height: window.visualViewport.height,
        offsetTop: window.visualViewport.offsetTop, offsetLeft: window.visualViewport.offsetLeft,
      } : null,
      headings: [...document.querySelectorAll('h1,h2,h3')].map((element) => element.textContent),
      dialog: bounds(dialog),
      footer: bounds(footer),
      footerButtons: [...(footer?.querySelectorAll('button') || [])].map((button) => ({
        text: button.textContent, disabled: button.disabled, bounds: bounds(button),
      })),
    };
  });
}

/** Shared checks for the actual Wizard inside a single Modal, on desktop and mobile. */
export async function assertWizardLayout({ page, dialog }) {
  await expect(page.getByRole('dialog')).toHaveCount(1);
  // Viewport updates reach CSS and the visual-viewport listener asynchronously.
  // Require the final geometry rather than reading in the same resize turn.
  await expect.poll(() => dialog.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const footer = element.lastElementChild;
    return {
      documentOverflow: document.documentElement.scrollWidth > window.innerWidth + 1,
      dialogOverflow: element.scrollWidth > element.clientWidth + 1,
      outside: rect.left < -1 || rect.right > window.innerWidth + 1 || rect.bottom > window.innerHeight + 1,
      footerOutside: [...footer.querySelectorAll('button')].some((button) => {
        const bounds = button.getBoundingClientRect();
        return bounds.left < -1 || bounds.right > window.innerWidth + 1 || bounds.bottom > window.innerHeight + 1;
      }),
    };
  })).toEqual({ documentOverflow: false, dialogOverflow: false, outside: false, footerOutside: false });
}

export async function assertWizardAccessibility({ page }, selector = '[role="dialog"]') {
  const report = await new AxeBuilder({ page }).include(selector).withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa']).analyze();
  expect(report.violations.filter((violation) => ['serious', 'critical'].includes(violation.impact)).map((violation) => ({ id: violation.id, help: violation.help, nodes: violation.nodes.map((node) => node.target) }))).toEqual([]);
}
