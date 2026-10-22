// SPDX-License-Identifier: Apache-2.0
// The production shared panel; all provider responses and accounts are artificial.
import {chromium} from 'playwright';
import {expect as baseExpect} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {assertWizardAccessibility,assertWizardLayout,openWizardFixture} from './lib/wizard-checks.mjs';
const expect=baseExpect.configure({timeout:15000});
const BASE=process.env.LINKED_URL??'http://127.0.0.1:4343/calendar/prueba/dev/servicios-vinculados';
const OUT=process.env.LINKED_OUT??'/tmp/servicios-vinculados';
mkdirSync(OUT,{recursive:true});
const browser=await chromium.launch();const report=[];
const id='11111111-1111-4111-8111-111111111111';
const account={instance_id:id,provider:'microsoft',address:'fixture@example.eu',name:'Fixture',connected:true,services:['mail','calendar','contacts','notes'],capabilities:{read:true,send:true}};
const providers=['gmail','microsoft','yahoo'].map(provider=>({provider,available:true,authorization:'oauth2',services:['mail','calendar','contacts','notes'].map(service=>({service,available:service!=='notes'||provider==='microsoft',...(service==='notes'&&provider==='gmail'?{reason:'workspace_admin_required'}:{})}))}));
async function fixture(viewport,service,mode='connected'){
 const context=await browser.newContext({viewport,...(mode==='allDay'?{timezoneId:'America/Los_Angeles'}:{})});const page=await context.newPage();const writes=[];const starts=[];let failWrite=true;
 const item=service==='calendar'?{id:'record',collection_id:'collection',summary:'Before',description:'Keep description',location:'Keep location',start:'2026-10-20T09:00:00Z',end:'2026-10-20T10:00:00Z',all_day:mode==='allDay',etag:'"v1"',read_only:false}:service==='contacts'?{id:'record',collection_id:'collection',name:'Before',organization:'Keep company',emails:['fixture@example.eu'],phones:['+34000000000'],note:'Keep note',etag:'"v1"',read_only:false}:{id:'record',collection_id:'collection',title:'Before',text:'Keep rich content',can_append:true,read_only:true};
 await context.route('**/api/external-mail/**',async route=>{
  if(route.request().method()==='GET'){await route.fulfill({json:{items:mode==='empty'?[]:[{...account,connected:mode!=='denied',...(mode==='reconnect'?{services:['mail','calendar']}:{})}],providers:mode==='unavailable'?providers.map(value=>({...value,available:false})):providers}});return;}
  const data=route.request().postDataJSON();starts.push(data);
  await route.fulfill({json:{instance_id:id,authorize_url:'https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=synthetic'}});
 });
 await context.route('https://login.microsoftonline.com/**',route=>route.fulfill({contentType:'text/html',body:'<html><body>Artificial consent</body></html>'}));
 await context.route('**/api/linked-services/**',async route=>{
  const request=route.request(),url=new URL(request.url());
  if(request.method()==='POST'){
   const data=request.postDataJSON();writes.push(data);expect(data.expected_address).toBe(account.address);expect(data.collection).toBe('collection');
   if(service==='calendar'||service==='contacts'){expect(data.etag).toBe('"v1"');expect(data.patch).toEqual(mode==='allDay'?{start:'2026-10-22T00:00:00Z',end:'2026-10-23T00:00:00Z',all_day:true,time_zone:'UTC'}:service==='calendar'?{summary:'Updated'}:{name:'Updated'});}
   else{expect(data.id).toBe('record');expect(data.text).toBe('Append safely');expect(data.title).toBeUndefined();}
   if(mode==='network'){await route.abort('connectionreset');return;}
   if(failWrite){await route.fulfill({status:service==='notes'?502:409,json:{error:{code:service==='notes'?'external_write_unknown':'external_resource_changed'}}});}else await route.fulfill(service==='notes'?{status:204}:{json:{...item,...data.patch}});
   return;
  }
  expect(url.searchParams.get('expected_address')).toBe(account.address);
  if(url.pathname.endsWith('/events')){const from=new Date(url.searchParams.get('from'));const to=new Date(url.searchParams.get('to'));expect(from.getUTCDate()).toBe(1);expect(to.getUTCDate()).toBe(1);expect(to-from).toBeLessThanOrEqual(31*86400000);}
  if(url.pathname.endsWith('/collections'))await route.fulfill({json:{items:[{id:'collection',name:'Remote collection',read_only:false}]}});
  else if(url.pathname.endsWith('/event')||url.pathname.endsWith('/item'))await route.fulfill({json:item});
  else await route.fulfill({json:{items:[item]}});
 });
 const url=new URL(BASE);url.searchParams.set('service',service);if(mode==='denied'||mode==='missingReturn'){url.searchParams.set('connect-linked','1');url.searchParams.set('instance',mode==='missingReturn'?'not-in-inventory':id);if(mode==='denied')url.searchParams.set('oauth_error','denied');}
 await openWizardFixture(page,url.href);
 if(mode!=='denied'&&mode!=='missingReturn')await page.getByRole('button',{name:'Connected services',exact:true}).click();
 const dialog=page.getByRole('dialog');await expect(dialog).toBeVisible();
 return{page,context,dialog,writes,starts,setSuccess(){failWrite=false;}};
}
try{
 for(const viewport of [{width:390,height:844},{width:1280,height:900}]){
  for(const service of ['calendar','contacts','notes']){
   const test=await fixture(viewport,service);const{page,dialog,writes}=test;
   if(service==='calendar'){await dialog.getByRole('button',{name:'Next month',exact:true}).click();await expect(dialog.getByText('Before',{exact:true})).toBeVisible();await dialog.getByRole('button',{name:'Previous month',exact:true}).click();}
   await dialog.getByText('Before',{exact:true}).click();
   await expect(dialog.getByLabel('Title',{exact:true})).toHaveValue('Before');
   await assertWizardLayout({page,dialog});await assertWizardAccessibility({page});
   await page.screenshot({path:`${OUT}/${service}-${viewport.width}.png`});
   if(service==='notes'){
    await expect(dialog.getByText('Keep rich content',{exact:true})).toBeVisible();
    await dialog.getByLabel('Add',{exact:true}).fill('Append safely');
   }else await dialog.getByLabel('Title',{exact:true}).fill('Updated');
   const save=dialog.getByRole('button',{name:service==='notes'?'Add':'Save',exact:true});await save.click();
   await expect(dialog.getByRole('alert')).toContainText(service==='notes'?'Refresh and check':'changed');await expect(save).toBeDisabled();expect(writes).toHaveLength(1);
   // Explicit read required before another write; merely waiting never replays it.
   await dialog.getByRole('button',{name:'Back to the list',exact:true}).click();
   await dialog.getByRole('button',{name:'Retry',exact:true}).click();
   await dialog.getByText('Before',{exact:true}).click();test.setSuccess();
   if(service==='notes')await dialog.getByLabel('Add',{exact:true}).fill('Append safely');else await dialog.getByLabel('Title',{exact:true}).fill('Updated');
   await dialog.getByRole('button',{name:service==='notes'?'Add':'Save',exact:true}).click();
   await expect(dialog.getByText('Before',{exact:true})).toBeVisible();expect(writes).toHaveLength(2);
   report.push({viewport:viewport.width,service,partialWrite:true,versionChecked:service!=='notes',appendOnly:service==='notes',noReplay:true});await test.context.close();
  }
  const reconnect=await fixture(viewport,'contacts','reconnect');await reconnect.dialog.getByLabel('Account',{exact:true}).selectOption(id);await expect(reconnect.dialog.getByRole('checkbox',{name:'Calendar',exact:true})).toBeChecked();await reconnect.dialog.getByRole('button',{name:'Sign in',exact:true}).last().click();await expect(reconnect.page).toHaveURL(/^https:\/\/login\.microsoftonline\.com(?:\/|$)/);expect(reconnect.starts[0]).toMatchObject({instance_id:id,services:['mail','calendar','contacts']});await reconnect.context.close();
  const network=await fixture(viewport,'notes','network');await network.dialog.getByText('Before',{exact:true}).click();await network.dialog.getByLabel('Add',{exact:true}).fill('Append safely');await network.dialog.getByRole('button',{name:'Add',exact:true}).click();await expect(network.dialog.getByRole('alert')).toContainText('Refresh and check');await expect(network.dialog.getByRole('button',{name:'Add',exact:true})).toBeDisabled();expect(network.writes).toHaveLength(1);await network.context.close();
  const day=await fixture(viewport,'calendar','allDay');await day.dialog.getByText('Before',{exact:true}).click();await day.dialog.getByLabel('Ends',{exact:true}).fill('2026-10-23');await day.dialog.getByLabel('Starts',{exact:true}).fill('2026-10-22');day.setSuccess();await day.dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(day.dialog.getByText('Before',{exact:true})).toBeVisible();expect(day.writes).toHaveLength(1);await day.context.close();
  const fresh=await fixture(viewport,'calendar','empty');await fresh.dialog.getByText('Microsoft Outlook',{exact:true}).click();
  await fresh.dialog.getByRole('button',{name:'Sign in',exact:true}).last().click();await expect(fresh.page).toHaveURL(/^https:\/\/login\.microsoftonline\.com(?:\/|$)/);
  expect(fresh.starts).toEqual([{provider:'microsoft',return_to:'external-calendar',services:['calendar']}]);await fresh.context.close();
  const denied=await fixture(viewport,'calendar','denied');await expect(denied.dialog.getByRole('alert')).toContainText('not granted');
  await denied.dialog.getByRole('button',{name:'Sign in',exact:true}).last().click();await expect(denied.page).toHaveURL(/^https:\/\/login\.microsoftonline\.com(?:\/|$)/);expect(denied.starts[0].instance_id).toBe(id);await denied.context.close();
  const missing=await fixture(viewport,'calendar','missingReturn');await expect(missing.dialog.getByRole('alert')).toContainText('not granted');await expect(missing.dialog.getByText('Before',{exact:true})).toHaveCount(0);expect(missing.starts).toHaveLength(0);await missing.context.close();
  const unavailable=await fixture(viewport,'notes','unavailable');await unavailable.dialog.getByRole('button',{name:'Sign in',exact:true}).first().click();await unavailable.dialog.getByText('Microsoft Outlook',{exact:true}).click();await expect(unavailable.dialog.getByText('Connection to this provider is not available for your account.',{exact:true})).toBeVisible();await expect(unavailable.dialog.getByRole('button',{name:'Sign in',exact:true}).last()).toBeDisabled();await unavailable.context.close();
  report.push({viewport:viewport.width,emptyOAuth:true,deniedExactRetry:true,unavailableDisabled:true});
 }
 writeFileSync(`${OUT}/report.json`,JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
}finally{await browser.close();}
