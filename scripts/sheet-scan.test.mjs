import test from 'node:test';
import assert from 'node:assert/strict';
import { previewSheet, commitSheet, extractSets } from '../backoffice/src/sheet-scan.js';

test('confirm imports the reviewed CSV before fetching only new/changed sets, targeting five images', async () => {
 const calls=[];
 const api=async (path, options) => {
  calls.push([path, options]);
  if(path==='/admin/import')return {created:2,updated:165,scanCandidates:[166,167],candidates:[1,166,167]};
  return {id:Number(path.split('/')[3]),title:'New',images:[{},{},{}],imageState:'ready',imageError:''};
 };
 const result=await commitSheet(api,{sheetUrl:'https://docs.google.com/spreadsheets/d/example',csv:'Date,Name,GROUP\n0,New,IVE\n'},{});
 assert.deepEqual(calls.map(c=>c[0]),['/admin/import','/admin/sets/166/extract?target=5','/admin/sets/167/extract?target=5']);
 assert.equal(calls[0][1].body.get('sheetUrl'),'https://docs.google.com/spreadsheets/d/example');assert.equal(await calls[0][1].body.get('csv').text(),'Date,Name,GROUP\n0,New,IVE\n');
 assert.equal(result.batch.ready,2);assert.deepEqual(result.batch.remaining,[]);
});

test('unchanged sheet makes no image requests', async () => {
 let calls=0;
 const result=await commitSheet(async () => {calls++;return {created:0,scanCandidates:[],candidates:[1]};},{sheetUrl:'sheet',csv:'reviewed'},{});
 assert.equal(calls,1);assert.equal(result.batch.total,0);
});

test('provider pause preserves the imported report and unfinished queue, including current set', async () => {
 const seen=[];
 const api=async path=>{
  seen.push(path);
  if(path==='/admin/import')return {created:3,scanCandidates:[2,3,4]};
  if(path.includes('/3/'))throw Object.assign(new Error('source HTTP 429'),{pausedUntil:'2099-01-01T00:00:00Z'});
  return {id:2,title:'Stored',images:[{},{}],imageState:'ready'};
 };
 const result=await commitSheet(api,{sheetUrl:'sheet',csv:'reviewed'},{});
 assert.equal(result.report.created,3);assert.deepEqual(result.batch.remaining,[3,4]);
 assert.equal(result.batch.ready,1);assert.equal(result.batch.pause.pausedUntil,'2099-01-01T00:00:00Z');assert.equal(seen.length,3);
});

test('stop retains unprocessed sets and unavailable images do not prevent later sets', async () => {
 let stop=false;
 const batch=await extractSets(async ()=>({id:1,title:'Missing',images:[],imageError:'No public images',imageState:'failed'}),[1,2],{target:5,shouldStop:()=>stop,onProgress:p=>{if(p.done===1)stop=true;}});
 assert.equal(batch.failed,1);assert.deepEqual(batch.remaining,[2]);assert.equal(batch.stopped,true);
});

test('import failure does not start extraction', async () => {
 let calls=0;
 await assert.rejects(commitSheet(async ()=>{calls++;throw new Error('Sheet unavailable');},{sheetUrl:'sheet',csv:'reviewed'},{} ),/Sheet unavailable/);
 assert.equal(calls,1);
});

test('preview only requests the dry-run endpoint and never imports or extracts',async()=>{const paths=[];const preview=await previewSheet(async path=>{paths.push(path);return {report:{created:1},csv:'reviewed',sheetUrl:'sheet'};},'sheet');assert.deepEqual(paths,['/admin/import/preview']);assert.equal(preview.csv,'reviewed');});
