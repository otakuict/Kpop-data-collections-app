export function floatingCard(node) {
 const card=node.firstElementChild,view=node.ownerDocument.defaultView;
 const reduced=view.matchMedia('(prefers-reduced-motion: reduce)');
 const fine=view.matchMedia('(hover: hover) and (pointer: fine)');
 const rest={x:0,y:0,lift:0,scale:1};
 let pose={...rest},target={...rest},frame=0,lastTime=0,pressed=false;
 function paint() {
  for (const [name,value] of [['--tilt-x',`${pose.x}deg`],['--tilt-y',`${pose.y}deg`],['--card-lift',`${pose.lift}px`],['--card-scale',String(pose.scale)]]) card.style.setProperty(name,value);
  const resting=pose.x===0&&pose.y===0&&pose.lift===0&&pose.scale===1;
  card.style.setProperty('--card-transform',resting?'none':`perspective(1100px) translateY(${pose.lift}px) rotateX(${pose.x}deg) rotateY(${pose.y}deg) scale(${pose.scale})`);
 }
 function animate(time) {
  const delta=lastTime?Math.min((time-lastTime)/1000,.05):1/60;
  lastTime=time;
  const blend=1-Math.exp(-16*delta);
  let moving=false;
  for (const key of Object.keys(rest)) {
   pose[key]+=(target[key]-pose[key])*blend;
   if (Math.abs(target[key]-pose[key])<.001) pose[key]=target[key];
   else moving=true;
  }
  paint();frame=moving?view.requestAnimationFrame(animate):0;
  if (!moving) lastTime=0;
 }
 function schedule() {if (!frame) frame=view.requestAnimationFrame(animate);}
 function reset(immediate=false) {
  card.classList.remove('is-floating');target={...rest};
  if (!immediate) {schedule();return;}
  view.cancelAnimationFrame(frame);frame=0;lastTime=0;pose={...rest};paint();
 }
 function move(event) {
  if (event.pointerType!=='mouse'||!fine.matches||reduced.matches||pressed||event.buttons) return;
  const bounds=node.getBoundingClientRect();
  const x=Math.max(0,Math.min(1,(event.clientX-bounds.left)/bounds.width));
  const y=Math.max(0,Math.min(1,(event.clientY-bounds.top)/bounds.height));
  target={x:(y-.5)*16,y:(.5-x)*16,lift:-6,scale:1.025};
  card.style.setProperty('--sheen-x',`${x*100}%`);card.style.setProperty('--sheen-y',`${y*100}%`);
  card.classList.add('is-floating');schedule();
 }
 const leave=()=>{if (frame || pose.lift) reset();};
 const down=()=>{pressed=true;reset(true);};
 const up=event=>{pressed=false;if (event.type==='pointerup') move(event);};
 const stop=()=>reset(true);
 node.addEventListener('pointerenter',move);node.addEventListener('pointermove',move);
 node.addEventListener('pointerleave',leave);node.addEventListener('pointerdown',down,true);
 node.addEventListener('pointerup',up,true);node.addEventListener('pointercancel',up,true);
 view.addEventListener('blur',stop);reduced.addEventListener('change',stop);fine.addEventListener('change',stop);
 return {destroy(){
  reset(true);
  node.removeEventListener('pointerenter',move);node.removeEventListener('pointermove',move);
  node.removeEventListener('pointerleave',leave);node.removeEventListener('pointerdown',down,true);
  node.removeEventListener('pointerup',up,true);node.removeEventListener('pointercancel',up,true);
  view.removeEventListener('blur',stop);reduced.removeEventListener('change',stop);fine.removeEventListener('change',stop);
 }};
}
