<script>
 import { untrack } from 'svelte';
 import { ArrowRight, ChevronLeft, LayoutGrid, TableProperties, SlidersHorizontal, Search, X, MousePointer2, Image as ImageIcon } from '@lucide/svelte';
 let { hidden = false, onclose } = $props();
 let step = $state(0), dontShow = $state(untrack(()=>hidden));
 const steps = [
  { title:'Choose your view', copy:'Use the view button beside the collection title to switch between cards and a spreadsheet-style table. Your filters stay selected. Scroll the table sideways to see more columns.' },
  { title:'Find your favorites', copy:'Search by name, choose an artist, or set a date range to narrow down the collection. On mobile, tap Filters to open the filter panel.' },
  { title:'Explore each set', copy:'Select an image or set title to open its details. In card view, drag the bar below the image to browse photos. Scroll down to load more sets automatically.' },
 ];
 function dismiss(){onclose(dontShow);}
 function modal(node){node.showModal();return {destroy(){node.close();}};}
</script>

<dialog class="quick-guide" use:modal aria-labelledby="guide-title" aria-describedby="guide-copy" oncancel={event=>{event.preventDefault();dismiss();}}>
 <button class="modal-close" aria-label="Close short guide" onclick={dismiss}><X size={19}/></button>
 <div class="eyebrow">A QUICK TOUR · BIAS</div>
 <h2 id="guide-title">A little guide<span>.</span></h2>
 <p class="guide-intro">Choose a view, find a favorite, and take a closer look.</p>
 <div class="guide-demo" aria-hidden="true">
  <div class="demo-top"><span class="demo-brand">BIAS</span><span>EXAMPLE / {step+1}</span></div>
  {#key step}
   {#if step===1}
    <div class="demo-filter"><div class="demo-sidebar"><div class="demo-search"><Search size={14}/> Search…</div><span class="demo-label">ARTISTS</span><span>All artists</span><span class="demo-choice">aespa <SlidersHorizontal size={13}/></span><span>IVE</span><span class="demo-label">DATE OF SET</span><span class="demo-date">From → To</span></div><div class="demo-cards"><div></div><div></div><div></div><div></div></div><MousePointer2 class="demo-cursor filter-cursor" size={23}/></div>
   {:else if step===0}
    <div class="demo-view"><div class="demo-heading"><span>The collection.</span><span class="demo-toggle"><LayoutGrid size={17}/><TableProperties size={17}/></span></div><div class="demo-view-cards"><div></div><div></div><div></div></div><div class="demo-view-table"><div>SET_ID <span>Image set</span><span>Artist</span></div>{#each [1,2,3] as id}<div>{id}<span>Image set {id}</span><span>aespa</span></div>{/each}</div><MousePointer2 class="demo-cursor view-cursor" size={23}/></div>
   {:else}
    <div class="demo-browse"><div class="demo-photo"><ImageIcon size={38}/><span>IMAGE <b class="demo-image-one">1</b><b class="demo-image-two">2</b> / 3</span><div class="demo-rail"><i></i></div></div><div class="demo-detail"><span class="demo-label">aespa</span><strong>Open your favorite set.</strong><span>Images · Source · Date</span><span class="demo-load">↓ Scroll to load more</span></div><MousePointer2 class="demo-cursor browse-cursor" size={23}/></div>
   {/if}
  {/key}
 </div>
 <div class="guide-step-copy" aria-live="polite"><span class="guide-step-number">0{step+1} / 03</span><h3>{steps[step].title}</h3><p id="guide-copy">{steps[step].copy}</p></div>
 <div class="guide-dots" aria-label="Guide steps">{#each steps as item,index}<button class:active={step===index} aria-label={`Guide step ${index+1}: ${item.title}`} aria-current={step===index?'step':undefined} onclick={()=>step=index}></button>{/each}</div>
 <label class="guide-preference"><input type="checkbox" bind:checked={dontShow}/> Don't show it again</label>
 <div class="guide-actions"><button class="guide-back" disabled={step===0} onclick={()=>step--}><ChevronLeft size={16}/> Back</button>{#if step<2}<button class="guide-next" onclick={()=>step++}>Next <ArrowRight size={16}/></button>{:else}<button class="guide-next" onclick={dismiss}>Get started <ArrowRight size={16}/></button>{/if}</div>
</dialog>
