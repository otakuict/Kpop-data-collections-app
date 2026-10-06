import test from 'node:test';
import assert from 'node:assert/strict';
import { groupSets, displayDate, queryString } from '../frontend/src/catalog.js';
test('groups unknown dates separately without losing sets',()=>{
 const groups=groupSets([{id:1,date:'2026-09-15',group:'aespa'},{id:2,date:'',group:'IVE'}],'date');
 assert.equal(groups.length,2);assert.equal(groups[1].label,'Date unknown');assert.equal(groups.flatMap(x=>x.items).length,2);
});
test('group grouping keeps the first server order and same-group items together',()=>{
 const groups=groupSets([{group:'IVE',id:1},{group:'aespa',id:2},{group:'IVE',id:3}],'group');
 assert.deepEqual(groups.map(x=>x.items.map(i=>i.id)),[[1,3],[2]]);
});
test('query contains applied filters and dates are timezone independent',()=>{
 assert.equal(new URLSearchParams(queryString({q:'Karina',group:'aespa',page:2})).get('page'),'2');
 assert.equal(displayDate('2026-09-15'),'15 Sep 2026');assert.equal(displayDate(''),'Date unknown');
});
