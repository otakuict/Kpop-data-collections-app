<script>
 import { ArrowUpRight, Image as ImageIcon } from '@lucide/svelte';
 let { set, number, onopen } = $props();
 let position = $state(0), pointerId = $state(null);
 let active = $derived(Math.round(position));

 function move(index) {
  position = Math.max(0, Math.min(set.images.length - 1, index));
 }
 function drag(event) {
  if (pointerId !== event.pointerId) return;
  const bounds = event.currentTarget.getBoundingClientRect();
  const fraction = (event.clientX - bounds.left - 12) / (bounds.width - 24);
  move(fraction * (set.images.length - 1));
 }
 function start(event) {
  if (!event.isPrimary || event.button !== 0 || pointerId !== null) return;
  event.preventDefault();
  event.currentTarget.focus({ preventScroll: true });
  event.currentTarget.setPointerCapture(event.pointerId);
  pointerId = event.pointerId;
  drag(event);
 }
 function finish(event) {
  if (pointerId !== event.pointerId) return;
  pointerId = null;
  move(active);
  if (event.currentTarget.hasPointerCapture(event.pointerId)) {
   event.currentTarget.releasePointerCapture(event.pointerId);
  }
 }
 function release(event) {
  drag(event);
  finish(event);
 }
 function keydown(event) {
  const targets = { ArrowLeft: active - 1, ArrowRight: active + 1, Home: 0, End: set.images.length - 1 };
  if (!Object.hasOwn(targets, event.key)) return;
  event.preventDefault();
  move(targets[event.key]);
 }
</script>

<div class="card-image" role="group" aria-label={`Images for ${set.title}`}>
 {#if set.images.length}
  <div class="carousel-track" class:dragging={pointerId !== null} style:transform={`translateX(${-position * 100}%)`}>
   {#each set.images as image, index (image.id)}
    <button class="carousel-slide" onclick={() => onopen(set, index)} aria-label={index ? `View ${set.title}, image ${index + 1}` : `View ${set.title}`} tabindex={active === index ? 0 : -1} inert={active !== index}>
     <img src={image.url} alt={`${set.title} — image ${index + 1}`} loading="lazy" draggable="false"/>
    </button>
   {/each}
  </div>
  {#if set.images.length > 1}
   <div class="carousel-bar" class:dragging={pointerId !== null} role="slider" tabindex="0" aria-label="Choose image" aria-orientation="horizontal" aria-valuemin="1" aria-valuemax={set.images.length} aria-valuenow={active + 1} aria-valuetext={`Image ${active + 1} of ${set.images.length}`} onkeydown={keydown} onpointerdown={start} onpointermove={drag} onpointerup={release} onpointercancel={finish} onlostpointercapture={finish}>
    <span class="carousel-rail">
     <span class="carousel-progress" style:width={`${position / (set.images.length - 1) * 100}%`}></span>
     <span class="carousel-thumb" style:left={`${position / (set.images.length - 1) * 100}%`}></span>
    </span>
   </div>
   <span class="carousel-position" aria-label="Image position" aria-live="polite">{active + 1} / {set.images.length}</span>
  {/if}
 {:else}
  <button class="carousel-empty" onclick={() => onopen(set, 0)} aria-label={`View ${set.title}`}><div class="no-image"><ImageIcon size={28} strokeWidth={1}/><span>PREVIEW UNAVAILABLE</span><small>Metadata archived</small></div></button>
 {/if}
 <span class="card-number">{number}</span>
 <button class="card-arrow" aria-label={`Open ${set.title}`} onclick={() => onopen(set, active)}><ArrowUpRight size={17}/></button>
 <span class="image-count"><ImageIcon size={11}/>{set.images.length ? `${set.images.length} IMAGE${set.images.length > 1 ? 'S' : ''}` : 'NO PREVIEW'}</span>
</div>
