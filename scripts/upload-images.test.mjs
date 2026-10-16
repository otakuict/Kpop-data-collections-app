import test from 'node:test';
import assert from 'node:assert/strict';
import {prepareUploadImages} from '../backoffice/src/upload-images.js';

test('prepares five large images sequentially at gallery size', async () => {
 const input = Array.from({length: 5}, (_, index) => ({name: `photo-${index}.png`, type: 'image/png'}));
 const sizes = [];
 let active = 0;
 let peak = 0;
 const output = await prepareUploadImages(input, {
  decode: async () => {
   active++;
   peak = Math.max(peak, active);
   return {width: 6000, height: 4000, close: () => active--};
  },
  createCanvas: () => ({
   width: 0,
   height: 0,
   getContext() {
    return {fillRect() {}, drawImage(_bitmap, _x, _y, width, height) { sizes.push([width, height]); }};
   },
   toBlob(callback) { callback(new Blob(['jpeg'], {type: 'image/jpeg'})); },
  }),
  makeFile: (blob, name) => ({blob, name}),
 });
 assert.equal(output.length, 5);
 assert.equal(peak, 1);
 assert.equal(active, 0);
 assert.deepEqual(sizes, Array(5).fill([1200, 800]));
 assert.equal(output[0].name, 'photo-0.jpg');
 assert.equal(output[0].blob.type, 'image/jpeg');
});

test('uses image-resize-compress only above 10 MB', async () => {
 const calls = [];
 const output = await prepareUploadImages([
  {name: 'at-limit.png', type: 'image/png', size: 10_000_000},
  {name: 'over-limit.png', type: 'image/png', size: 10_000_001},
 ], {
  compress: async (file, options) => {
   calls.push(['compress', file.name, options]);
   return new Blob(['compressed'], {type: 'image/jpeg'});
  },
  decode: async file => {
   calls.push(['decode', file.name]);
   return {width: 2400, height: 1600, close() {}};
  },
  createCanvas: () => ({
   width: 0,
   height: 0,
   getContext: () => ({fillRect() {}, drawImage() {}}),
   toBlob: callback => callback(new Blob(['resized'], {type: 'image/jpeg'})),
  }),
  makeFile: (blob, name) => ({blob, name}),
 });
 assert.deepEqual(calls.map(([operation, name]) => [operation, name]), [
  ['decode', 'at-limit.png'],
  ['compress', 'over-limit.png'],
 ]);
 assert.deepEqual(calls[1][2], {
  maxWidthOrHeight: 1200,
  format: 'jpeg',
  quality: 90,
  backgroundColor: '#fff',
 });
 assert.deepEqual(output.map(file => file.name), ['at-limit.jpg', 'over-limit.jpg']);
});
