import test from 'node:test';
import assert from 'node:assert/strict';
import { shouldShowGuide, saveGuidePreference } from '../frontend/src/guide-preference.js';
function browser(){const saved=new Map();return ()=>({getItem:key=>saved.get(key)??null,setItem:(key,value)=>saved.set(key,value),removeItem:key=>saved.delete(key)});}
test('guide appears until the visitor explicitly chooses not to show it again',()=>{
 const storage=browser();assert.equal(shouldShowGuide(storage),true);
 saveGuidePreference(false,storage);assert.equal(shouldShowGuide(storage),true);
 saveGuidePreference(true,storage);assert.equal(shouldShowGuide(storage),false);
});
test('unchecking the preference when reopening help restores the guide on next visit',()=>{
 const storage=browser();saveGuidePreference(true,storage);saveGuidePreference(false,storage);assert.equal(shouldShowGuide(storage),true);
});
test('blocked browser storage does not prevent closing the guide or using the gallery',()=>{
 const blocked=()=>{throw new Error('Storage blocked');};assert.equal(shouldShowGuide(blocked),true);assert.doesNotThrow(()=>saveGuidePreference(true,blocked));
});
