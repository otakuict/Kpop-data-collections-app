import test from 'node:test';
import assert from 'node:assert/strict';
import { sourceHue } from '../frontend/src/source-colors.js';
import { floatingCard } from '../frontend/src/floating-card.js';

const shops=['yoichi69','kdatastudio','idollove','idolxdata','datacoffeeshop','loveshakedata','SAKURADATA','Baemonfan','KeqingData','KUMDATA','KPFTIME'];
test('shops get stable distinct colors, including source aliases',()=>{
 assert.equal(new Set(shops.map(sourceHue)).size,shops.length);
 assert.equal(sourceHue('  SAKURADATA  '),sourceHue('sakuradata'));
 assert.equal(sourceHue('datacoffeeshop (?)'),sourceHue('datacoffeeshop'));
 assert.equal(sourceHue('new shop'),sourceHue('new shop'));
 assert.equal(sourceHue(''),null);
});
function fixture({reduced=false,fine=true}={}){
 const frameQueue=new Map();let nextFrame=0,time=0;
 const media=new Map();const view=new EventTarget();
 view.matchMedia=query=>{if(!media.has(query))media.set(query,Object.assign(new EventTarget(),{matches:query.includes('reduced')?reduced:fine}));return media.get(query);};
 view.requestAnimationFrame=callback=>{frameQueue.set(++nextFrame,callback);return nextFrame;};
 view.cancelAnimationFrame=id=>frameQueue.delete(id);
 const values=new Map(),classes=new Set();
 const card={style:{setProperty:(name,value)=>values.set(name,value)},classList:{add:name=>classes.add(name),remove:name=>classes.delete(name)}};
 const node=Object.assign(new EventTarget(),{ownerDocument:{defaultView:view},firstElementChild:card,getBoundingClientRect:()=>({left:20,top:30,width:200,height:300})});
 const action=floatingCard(node);
 const pointer=(type,properties={})=>{const event=new Event(type);Object.assign(event,{pointerType:'mouse',clientX:215,clientY:45,buttons:0,...properties});node.dispatchEvent(event);};
 const advance=()=>{for(let i=0;i<90;i++){time+=16;const callbacks=[...frameQueue.values()];frameQueue.clear();callbacks.forEach(cb=>cb(time));}};
 return {node,view,values,classes,media,action,pointer,advance,frameQueue};
}
test('hover tilts and lifts the card, leaving returns it to rest and stops frames',()=>{
 const f=fixture();f.pointer('pointermove');f.advance();
 assert.ok(f.classes.has('is-floating'));assert.notEqual(parseFloat(f.values.get('--tilt-x')),0);assert.ok(parseFloat(f.values.get('--card-lift'))<0);
 f.pointer('pointerleave');f.advance();
 assert.equal(parseFloat(f.values.get('--tilt-x')),0);assert.equal(parseFloat(f.values.get('--card-lift')),0);assert.equal(f.frameQueue.size,0);f.action.destroy();
});
test('touch, coarse pointers and reduced-motion preference leave the card still',()=>{
 for(const options of [{reduced:true},{fine:false},{}]){
  const f=fixture(options);f.pointer('pointermove',options.reduced||options.fine===false?{}:{pointerType:'touch'});f.advance();
  assert.equal(f.classes.size,0);assert.equal(f.frameQueue.size,0);f.action.destroy();
 }
});
test('carousel pointer-down immediately neutralizes the card and cleanup removes listeners',()=>{
 const f=fixture();f.pointer('pointermove');f.advance();f.pointer('pointerdown',{buttons:1});
 assert.equal(parseFloat(f.values.get('--tilt-x')),0);assert.equal(f.classes.size,0);
 assert.equal(f.values.get('--card-transform'),'none');
 f.pointer('pointermove',{buttons:1});assert.equal(f.frameQueue.size,0);
 f.pointer('pointerup');f.pointer('pointermove');f.advance();assert.equal(f.classes.size,1);
 f.action.destroy();f.pointer('pointermove');assert.equal(f.frameQueue.size,0);assert.equal(f.classes.size,0);
});
test('turning on reduced motion while hovered immediately resets the card',()=>{
 const f=fixture();f.pointer('pointermove');f.advance();
 const preference=f.media.get('(prefers-reduced-motion: reduce)');preference.matches=true;preference.dispatchEvent(new Event('change'));
 assert.equal(parseFloat(f.values.get('--tilt-x')),0);assert.equal(f.frameQueue.size,0);f.action.destroy();
});
