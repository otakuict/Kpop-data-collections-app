import test from 'node:test';
import assert from 'node:assert/strict';
import { splitFinderClue, finderSearchPreview, finderSearchDate } from '../backoffice/src/finder-query.js';

test('date-prefixed names produce the same separated fields as finder paste',()=>{
 assert.deepEqual(finderSearchPreview('261018 sullyoon','2026-10-24'),{date:'261018',name:'sullyoon'});
 assert.deepEqual(splitFinderClue('  261018\n  sullyoon  '),{date:'261018',name:'sullyoon'});
 assert.deepEqual(splitFinderClue('261018 sullyoon nol festival'),{date:'261018',name:'sullyoon nol festival'});
});

test('plain names use the selected set date while non-clues remain intact',()=>{
 assert.deepEqual(finderSearchPreview(' Sana ','2026-10-24'),{date:'261024',name:'Sana'});
 assert.deepEqual(finderSearchPreview('Sana',''),{date:'',name:'Sana'});
 assert.equal(splitFinderClue('261018sullyoon'),null);
 assert.equal(splitFinderClue('261018'),null);
});

test('editable finder dates become ISO dates for the gallery API',()=>{
 assert.equal(finderSearchDate('261018'),'2026-10-18');
 assert.equal(finderSearchDate(''),'');
});
