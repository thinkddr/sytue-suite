// SPDX-License-Identifier: Apache-2.0
import test from 'node:test';
import assert from 'node:assert/strict';
import { generateKeyPairSync, privateDecrypt, createDecipheriv, constants } from 'node:crypto';
import { accountInventory, encryptInventory, projectAlert } from './security-account-inventory.mjs';
const env = { GITHUB_REPOSITORY:'thinkddr/sytue-suite', SECURITY_ALERTS_TOKEN:'fixture-token', EXPECTED_REPOSITORIES:'1' };
const repository = {id:1,full_name:'thinkddr/library',owner:{login:'thinkddr'},default_branch:'main'};
function fixture(replies={}) {
 const calls=[];
 return {calls,fetchImpl:async(url,options)=>{
  calls.push({url,options}); assert.equal(new URL(url).origin,'https://api.github.com'); assert.equal(options.method,'GET'); assert.equal(options.redirect,'error');
  const path=new URL(url).pathname;
  if(replies[path])return replies[path](url,options);
  if(path==='/user/repos')return Response.json([repository]);
  return Response.json([]);
 }};
}
test('inventories owned repositories, uses default branch and omits the source and other owners',async()=>{
 const api=fixture({'/user/repos':()=>Response.json([repository,{...repository,id:2,full_name:env.GITHUB_REPOSITORY},{...repository,id:3,full_name:'another/library',owner:{login:'another'}}])});
 const r=await accountInventory({env,...api}); assert.equal(r.complete,true); assert.equal(r.repositories.length,1);assert.equal(api.calls.length,4);
 const code=api.calls.find(c=>c.url.includes('/code-scanning/'));assert.equal(new URL(code.url).searchParams.get('ref'),'refs/heads/main');
 assert.ok(!JSON.stringify(r).includes(env.SECURITY_ALERTS_TOKEN));
});
test('permission failures are coverage gaps, not zero-alert findings',async()=>{
 const api=fixture({'/repos/thinkddr/library/dependabot/alerts':()=>Response.json({message:'Resource not accessible'}, {status:403})});
 const r=await accountInventory({env,...api});assert.equal(r.complete,false);assert.equal(r.repositories[0].sources.dependabot.status,'access_denied');assert.ok(!('alerts' in r.repositories[0].sources.dependabot));
});
test('not-enabled features remain explicitly distinct from empty enabled inventories',async()=>{
 const api=fixture({'/repos/thinkddr/library/code-scanning/alerts':()=>Response.json({message:'Code scanning is not enabled for this repository'}, {status:403})});
 const r=await accountInventory({env,...api});assert.equal(r.complete,true);assert.equal(r.repositories[0].sources['code-scanning'].status,'not_enabled');
});
test('incomplete repository visibility fails coverage even if visible sources have no alerts',async()=>{
 const api=fixture();const r=await accountInventory({env:{...env,EXPECTED_REPOSITORIES:'2'},...api});assert.equal(r.complete,false);
});
test('secret values and arbitrary response fields are never projected',()=>{
 const a=projectAlert('secret-scanning','thinkddr/library',{number:1,state:'open',secret:'DO-NOT-RETAIN-THIS-SECRET',secret_type:'github_personal_access_token',validity:'active',html_url:'https://attacker.invalid'});
 assert.equal(a.url,'https://github.com/thinkddr/library/security/secret-scanning/1');assert.ok(!JSON.stringify(a).includes('DO-NOT-RETAIN'));assert.ok(!('secret' in a));
 assert.throws(()=>projectAlert('unsupported','thinkddr/library',{number:1}));
});
test('cursor pagination stays on the exact Dependabot endpoint',async()=>{
 let page=0;const api=fixture({'/repos/thinkddr/library/dependabot/alerts':url=>{page++;return page===1?Response.json([{number:1,state:'open'}],{headers:{link:'<https://api.github.com/repos/thinkddr/library/dependabot/alerts?state=open&per_page=100&after=next>; rel="next"'}}):Response.json([{number:2,state:'open'}]);}});
 const r=await accountInventory({env,...api});assert.equal(r.complete,true);assert.equal(r.repositories[0].sources.dependabot.alerts.length,2);assert.ok(api.calls.some(c=>new URL(c.url).searchParams.get('after')==='next'));
});
test('untrusted pagination cannot redirect credentials',async()=>{
 const api=fixture({'/repos/thinkddr/library/dependabot/alerts':()=>Response.json([],{headers:{link:'<https://attacker.invalid/steal?after=next>; rel="next"'}})});
 const r=await accountInventory({env,...api});assert.equal(r.complete,false);assert.equal(r.repositories[0].sources.dependabot.status,'invalid_pagination');assert.ok(api.calls.every(c=>new URL(c.url).origin==='https://api.github.com'));
});
test('duplicates and rate limits prevent a complete inventory',async()=>{
 const api=fixture({'/repos/thinkddr/library/code-scanning/alerts':()=>Response.json([{number:1,state:'open'},{number:1,state:'open'}])});
 assert.equal((await accountInventory({env,...api})).complete,false);
 const limited=fixture({'/repos/thinkddr/library/code-scanning/alerts':()=>Response.json({message:'rate limit'}, {status:403,headers:{'x-ratelimit-remaining':'0'}})});
 const r=await accountInventory({env,...limited});assert.equal(r.complete,false);assert.equal(limited.calls.length,2);
});
test('invalid runtime configuration performs no requests',async()=>{
 for(const override of [{GITHUB_REPOSITORY:'owner/..'},{EXPECTED_REPOSITORIES:'0'},{SECURITY_ALERTS_TOKEN:'x\nheader'}])await assert.rejects(accountInventory({env:{...env,...override},fetchImpl:()=>assert.fail()}));
});
test('private metadata is encrypted for the recipient and authenticated against tampering',()=>{
 const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
 const report={repositories:[{repository:'private-owner/confidential',alerts:[{path:'sensitive/path.go'}]}]};const encrypted=encryptInventory(report,publicKey.export({type:'spki',format:'pem'}));
 assert.ok(!JSON.stringify(encrypted).includes('confidential'));assert.ok(!JSON.stringify(encrypted).includes('sensitive'));
 const key=privateDecrypt({key:privateKey,padding:constants.RSA_PKCS1_OAEP_PADDING,oaepHash:'sha256'},Buffer.from(encrypted.wrappedKey,'base64'));
 function decrypt(ciphertext){const d=createDecipheriv('aes-256-gcm',key,Buffer.from(encrypted.iv,'base64'));d.setAuthTag(Buffer.from(encrypted.tag,'base64'));return Buffer.concat([d.update(ciphertext),d.final()]);}
 assert.deepEqual(JSON.parse(decrypt(Buffer.from(encrypted.ciphertext,'base64'))),report);
 const tampered=Buffer.from(encrypted.ciphertext,'base64');tampered[0]^=1;assert.throws(()=>decrypt(tampered));
 const ec=generateKeyPairSync('ec',{namedCurve:'prime256v1'});assert.throws(()=>encryptInventory(report,ec.publicKey.export({type:'spki',format:'pem'})),/RSA/);
});
