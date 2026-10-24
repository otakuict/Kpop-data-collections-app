import test from 'node:test';
import assert from 'node:assert/strict';
import { readAPIResponse } from '../backoffice/src/api-response.js';

test('HTML gateway errors produce a useful status message without parsing or displaying the HTML',async()=>{
 const response=new Response('<!DOCTYPE html><html>private gateway diagnostic</html>',{status:502,headers:{'Content-Type':'text/html'}});
 const result=await readAPIResponse(response);
 assert.match(result.error,/502/);
 assert.match(result.error,/gateway/i);
 assert.doesNotMatch(result.error,/Unexpected token|DOCTYPE|private gateway diagnostic/);
});
test('upstream JSON errors and cooldown metadata are preserved',async()=>{
 const error={error:'TeraBox session expired',pausedUntil:'2026-10-25T01:00:00Z'};
 assert.deepEqual(await readAPIResponse(new Response(JSON.stringify(error),{status:429})),error);
});
test('a successful non-JSON response is rejected instead of being treated as API data',async()=>{
 await assert.rejects(()=>readAPIResponse(new Response('<html>login</html>',{status:200})),/invalid response/i);
});
