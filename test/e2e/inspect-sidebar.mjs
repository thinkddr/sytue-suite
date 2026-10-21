import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
const STORAGE = fileURLToPath(new URL('.auth/admin.json', import.meta.url));
const b = await chromium.launch();
const ctx = await b.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true, viewport:{width:1440,height:900} });
const p = await ctx.newPage();
await p.goto('https://mail.sytue.localhost/', { waitUntil:'domcontentloaded', timeout:60000 });
await p.waitForTimeout(2500);
const info = await p.evaluate(() => {
  const sb = [...document.querySelectorAll('*')].find(e => (e.getAttribute && (e.getAttribute('class')||'').split(' ').includes('bg-sidebar')));
  const root = getComputedStyle(document.documentElement);
  const wrapper = document.querySelector('.contents');
  return {
    foundSidebarEl: !!sb,
    sidebarComputedBg: sb ? getComputedStyle(sb).backgroundColor : null,
    colorSidebar_onSidebarEl: sb ? getComputedStyle(sb).getPropertyValue('--color-sidebar').trim() : null,
    colorSidebar_root: root.getPropertyValue('--color-sidebar').trim(),
    sytueSidebarBg_root: root.getPropertyValue('--sytue-sidebar-bg').trim(),
    sytueSidebarOne_root: root.getPropertyValue('--sytue-sidebar-one').trim() || '(vacio -> colors.css NO recargo)',
    wrapper_colorSidebar: wrapper ? getComputedStyle(wrapper).getPropertyValue('--color-sidebar').trim() : '(sin .contents)',
    wrapper_colorTopbar: wrapper ? getComputedStyle(wrapper).getPropertyValue('--color-topbar').trim() : null,
  };
});
console.log(JSON.stringify(info,null,2));
await b.close();
