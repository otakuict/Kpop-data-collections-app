import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const script=fileURLToPath(new URL('./strip-seed-usage.py',import.meta.url));

test('seed export removes usage history and its stored bytes while preserving gallery records',()=>{
 const directory=mkdtempSync(join(tmpdir(),'gallery-seed-usage-'));
 const path=join(directory,'seed.sqlite');
 const python=code=>{
  const result=spawnSync('python3',['-c',code,path],{encoding:'utf8'});
  assert.equal(result.status,0,result.stderr);
 };
 try{
  python("import sqlite3,sys; db=sqlite3.connect(sys.argv[1]); db.executescript(\"CREATE TABLE sets(id INTEGER PRIMARY KEY,title TEXT); INSERT INTO sets VALUES(1,'Keep this set'); CREATE TABLE usage_visits(visitor_id TEXT); INSERT INTO usage_visits VALUES('private-browser-identifier');\"); db.close()");
  const result=spawnSync('python3',[script,path],{encoding:'utf8'});
  assert.equal(result.status,0,result.stderr);
  python("import sqlite3,sys; db=sqlite3.connect(sys.argv[1]); assert db.execute('SELECT COUNT(*) FROM usage_visits').fetchone()[0]==0; assert db.execute('SELECT title FROM sets').fetchone()[0]=='Keep this set'; db.close()");
  assert.ok(!readFileSync(path).includes(Buffer.from('private-browser-identifier')));
 }finally{rmSync(directory,{recursive:true,force:true});}
});

test('seed export accepts older databases without a usage table',()=>{
 const directory=mkdtempSync(join(tmpdir(),'gallery-seed-usage-'));
 const path=join(directory,'seed.sqlite');
 try{
  const fixture=spawnSync('python3',['-c',"import sqlite3,sys; db=sqlite3.connect(sys.argv[1]); db.execute('CREATE TABLE sets(id INTEGER PRIMARY KEY)'); db.close()",path],{encoding:'utf8'});
  assert.equal(fixture.status,0,fixture.stderr);
  const result=spawnSync('python3',[script,path],{encoding:'utf8'});
  assert.equal(result.status,0,result.stderr);
 }finally{rmSync(directory,{recursive:true,force:true});}
});
