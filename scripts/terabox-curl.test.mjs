import test from 'node:test';
import assert from 'node:assert/strict';
import { parseTeraBoxCurl } from '../backoffice/src/teraboxCurl.js';

test('extracts Cookie and jsToken from the copied Bash cURL format',()=>{
 const command=String.raw`curl --url 'https\://dm.terabox.com/api/list?app_id=250528&jsToken=test%2Btoken&dir=%2F' \
  -H 'Accept: application/json' \
  -b 'ndus=test-cookie; BAIDUID=other' \
  -H 'Referer: https\://dm.terabox.com/chinese/main'`;
 assert.deepEqual(parseTeraBoxCurl(command),{
  cookie:'ndus=test-cookie; BAIDUID=other',jsToken:'test+token',host:'dm.terabox.com',bdstoken:'',
 });
});

test('accepts a Cookie header and optional bdstoken from the same request',()=>{
 const command=`curl 'https://www.terabox.com/api/list?jsToken=xyz&bdstoken=bds%2F1' -H 'Cookie: ndus=other-cookie'`;
 assert.deepEqual(parseTeraBoxCurl(command),{
  cookie:'ndus=other-cookie',jsToken:'xyz',host:'www.terabox.com',bdstoken:'bds/1',
 });
});

test('accepts HTML space indentation copied with the example without changing Cookie text',()=>{
 const command="curl --url 'https://dm.terabox.com/api/list?jsToken=abc' \\\n&#x20; -b 'ndus=value&#x20;kept'";
 assert.equal(parseTeraBoxCurl(command).cookie,'ndus=value&#x20;kept');
});

test('rejects missing values and unrelated hosts without exposing pasted secrets',()=>{
 for(const command of [
  `curl --url 'https://evil.example/api/list?jsToken=secret' -b 'ndus=secret'`,
  `curl --url 'https://dm.terabox.com/api/list?jsToken=secret'`,
  `curl --url 'https://dm.terabox.com/api/list' -b 'ndus=secret'`,
  `curl --url 'https://dm.terabox.com/api/list?jsToken=secret' -b 'unterminated`,
 ]){
  assert.throws(()=>parseTeraBoxCurl(command),error=>!error.message.includes('secret'));
 }
});
