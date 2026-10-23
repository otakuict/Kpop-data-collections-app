import test from 'node:test';
import assert from 'node:assert/strict';
import { consentKey, consentLifetime, readAnalyticsConsent, saveAnalyticsConsent } from '../frontend/src/analytics-consent.js';

const now=Date.UTC(2026,9,23);
function browser(){const entries=new Map();return ()=>({getItem:key=>entries.get(key)??null,setItem:(key,value)=>entries.set(key,value)});}

test('analytics is off until an explicit, unexpired acceptance is saved',()=>{
 const storage=browser();
 assert.equal(readAnalyticsConsent(storage,now),null);
 saveAnalyticsConsent('accepted',storage,now);
 assert.equal(readAnalyticsConsent(storage,now+1),'accepted');
 assert.equal(readAnalyticsConsent(storage,now+consentLifetime),null);
});

test('rejection is remembered and can replace earlier acceptance',()=>{
 const storage=browser();
 saveAnalyticsConsent('accepted',storage,now);
 saveAnalyticsConsent('rejected',storage,now);
 assert.equal(readAnalyticsConsent(storage,now),'rejected');
});

test('invalid, tampered and inaccessible preferences never enable analytics',()=>{
 const storage=browser();
 for(const value of ['yes','{',JSON.stringify({choice:'accepted'}),JSON.stringify({choice:'unknown',expiresAt:now+1000}),JSON.stringify({choice:'accepted',expiresAt:now+2*consentLifetime})]){
  storage().setItem(consentKey,value);
  assert.equal(readAnalyticsConsent(storage,now),null);
 }
 const blocked=()=>{throw new Error('Storage blocked');};
 assert.equal(readAnalyticsConsent(blocked,now),null);
 assert.doesNotThrow(()=>saveAnalyticsConsent('rejected',blocked,now));
});
