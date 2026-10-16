import { fromBlob } from 'image-resize-compress';

const maxSide = 1200;
const compressionThreshold = 10_000_000;
const supportedTypes = new Set(['image/jpeg', 'image/png', 'image/gif', 'image/webp']);

export async function prepareUploadImages(files, {
 compress = fromBlob,
 decode = file => createImageBitmap(file),
 createCanvas = () => document.createElement('canvas'),
 makeFile = (blob, name) => new File([blob], name, {type: 'image/jpeg'}),
} = {}) {
 const prepared = [];
 for (const file of files) {
  if (!supportedTypes.has(file.type)) throw new Error('Choose JPEG, PNG, GIF, or WebP images.');
  let bitmap;
  let canvas;
  try {
   const name = (file.name || 'image').replace(/\.[^.]+$/, '') || 'image';
   if (file.size > compressionThreshold) {
    const blob = await compress(file, {
     maxWidthOrHeight: maxSide,
     format: 'jpeg',
     quality: 90,
     backgroundColor: '#fff',
    });
    prepared.push(makeFile(blob, `${name}.jpg`));
    continue;
   }
   bitmap = await decode(file);
   if (!bitmap.width || !bitmap.height) throw new Error('Image has invalid dimensions.');
   const scale = Math.min(1, maxSide / Math.max(bitmap.width, bitmap.height));
   canvas = createCanvas();
   canvas.width = Math.max(1, Math.round(bitmap.width * scale));
   canvas.height = Math.max(1, Math.round(bitmap.height * scale));
   const context = canvas.getContext('2d');
   if (!context) throw new Error('Image resizing is unavailable in this browser.');
   context.fillStyle = '#fff';
   context.fillRect(0, 0, canvas.width, canvas.height);
   context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
   const blob = await new Promise((resolve, reject) => {
    canvas.toBlob(result => result ? resolve(result) : reject(new Error('Could not resize image.')), 'image/jpeg', 0.9);
   });
   prepared.push(makeFile(blob, `${name}.jpg`));
  } catch (error) {
   throw new Error(`Could not prepare ${file.name || 'image'}: ${error.message}`);
  } finally {
   bitmap?.close();
   if (canvas) canvas.width = canvas.height = 0;
  }
 }
 return prepared;
}
