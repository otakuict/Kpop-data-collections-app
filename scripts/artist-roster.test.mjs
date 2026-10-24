import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';

test('profile extraction separates members without a Stage Name instead of attaching their aliases to the previous artist',()=>{
 const python=`
import importlib.util
spec=importlib.util.spec_from_file_location('roster','scripts/refresh-artist-roster.py')
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
text='Stage Name: Rei (레이)\\nBirth Name: Naoi Rei (나오이 레이)\\n'+'x'*600+'\\nBirth Name: Jang Won-young (장원영)\\n'+'x'*600+'\\nStage Name: Liz (리즈)\\nBirth Name: Kim Ji-won (김지원)\\n'
members=module.parse_profile_names(text,'https://example.com/profile')
assert len(members)==3, members
assert members[1]['name']=='Jang Won-young', members
assert '장원영' not in members[0]['aliases'], members
assert '김지원' in members[2]['aliases'], members
`;
 const result=spawnSync('python3',['-B','-c',python],{encoding:'utf8'});
 assert.equal(result.status,0,result.stderr);
});

test('bundled roster preserves all gallery categories, source references and known Korean artist identities',()=>{
 const seed=JSON.parse(readFileSync(new URL('../backend/internal/gallery/assets/artist-roster.json',import.meta.url),'utf8'));
 assert.equal(Object.keys(seed.groups).length,31);
 assert.equal(seed.groups.TripleS.length,24);
 assert.equal(seed.groups.SOLO.length,4);
 for(const [group,members] of Object.entries(seed.groups)){
  assert.ok(members.length>0,group);
  assert.equal(new Set(members.map(m=>m.name)).size,members.length,`${group} duplicate memberships`);
  for(const member of members){
   assert.ok(member.name.length<150);
   assert.ok(member.source.startsWith('https://'));
   assert.ok(Array.isArray(member.aliases));
  }
 }
 const anna=seed.groups.MEOVV.find(m=>m.name==='Anna');
 assert.ok(anna.aliases.includes('안나'));
 assert.ok(!seed.groups.MEOVV.find(m=>m.name==='Gawon').aliases.includes('안나'));
 assert.ok(seed.groups.TripleS.find(m=>m.name==='Kim Nakyoung').aliases.includes('김나경'));
 assert.ok(seed.groups.SOLO.find(m=>m.name==='Yena').aliases.includes('예나'));
});
