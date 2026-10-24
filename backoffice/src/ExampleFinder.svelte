<script>
 import { onDestroy } from 'svelte';
 import { splitFinderClue, finderSearchPreview, finderSearchDate } from './finder-query.js';
 import { Search, LoaderCircle } from '@lucide/svelte';
 let { source, setDate, name, group, selectedURL='', disabled=false, api, onchoose }=$props();
 let searchDate=$state(''), searchName=$state('');
 let seller=$state(''), count=$state(''), searching=$state(false), result=$state(null), error=$state('');
 let controller;
 let generation=0;
 let previousSource='';
 const searchKey=$derived(JSON.stringify({seller,setDate,name,group,count,searchDate,searchName}));
 $effect(()=>{
  const preview=finderSearchPreview(name,setDate);
  searchDate=preview.date;searchName=preview.name;
 });
 $effect(()=>{
  if(source!==previousSource){previousSource=source;seller=source||'';}
 });
 $effect(()=>{
  searchKey;
  generation++;
  controller?.abort();
  searching=false;result=null;error='';
 });
 onDestroy(()=>{generation++;controller?.abort();});
 function pasteClue(event){
  const clue=splitFinderClue(event.clipboardData?.getData('text')||'');
  if(!clue)return;
  event.preventDefault();searchDate=clue.date;searchName=clue.name;
 }
 async function findExamples(){
  const current=++generation;
  controller?.abort();controller=new AbortController();
  searching=true;error='';result=null;
  try{
   const found=await api('/admin/example-candidates',{method:'POST',signal:controller.signal,headers:{'Content-Type':'application/json'},body:JSON.stringify({seller:seller.trim(),setDate:finderSearchDate(searchDate.trim()),name:searchName.trim(),group,count:Number(count)||0})});
   if(current===generation)result=found;
  }catch(e){if(current===generation&&e.name!=='AbortError')error=e.message;}
  finally{if(current===generation)searching=false;}
 }
</script>

<fieldset class="example-finder" disabled={disabled}>
 <legend>Find an example link</legend>
 <p class="form-help">Search Gank using the editable preview below. A YYMMDD date at the start of Name is split out automatically, just like Finder. These fields control lookup; Save uses the set details above. TypeSafe suggests a match; choose a link to fill Example, then save as usual.</p>
 <div class="form-row">
  <label>Gank seller nickname<input bind:value={seller} maxlength="80" placeholder="loveshakedata"/><small>Starts from Source. Use the seller’s Gank nickname.</small></label>
  <label>File count (optional)<input type="number" bind:value={count} min="1" max="9999999" step="1" placeholder="2075"/></label>
 </div>
 <div class="form-row">
  <label>Search date (YYMMDD)<input type="text" inputmode="numeric" pattern={'[0-9]{6}'} maxlength="6" bind:value={searchDate} onpaste={pasteClue} placeholder="261018"/></label>
  <label>Search name preview<input type="text" bind:value={searchName} onpaste={pasteClue} maxlength="100" placeholder="sullyoon"/></label>
 </div>
 <button type="button" class="primary secondary" disabled={disabled||searching||!seller.trim()||(!searchDate&&!searchName.trim()&&!count)} onclick={findExamples}>
  {#if searching}<LoaderCircle size={16} class="spin"/>{:else}<Search size={16}/>{/if}{searching?'Finding links and asking TypeSafe…':'Find example links'}
 </button>
 {#if error}<p class="alert error" role="alert">{error}</p>{/if}
 {#if result}
  <div aria-live="polite" class="finder-results">
   <p>{result.candidates.length?`${result.candidates.length} links found. Choose one below.`:'No matching links found. Check the seller, date, and name, or enter an Example URL manually.'}</p>
   {#if result.truncated}<p class="form-help">Showing the first 60 links. Add a more specific name or date to narrow the search.</p>{/if}
   {#if result.warning}<p class="form-help" role="status">{result.warning}</p>{/if}
   {#each result.candidates as candidate (candidate.url)}
    <article class:recommended={candidate.url===result.recommendedUrl}>
     {#if candidate.url===result.recommendedUrl}<strong class="recommendation">TypeSafe recommendation · confidence {Math.round(result.confidence*100)}%</strong>{/if}
     <strong>{candidate.title||'Gank result'}</strong>
     <small>{candidate.source} · {candidate.status} · {candidate.matchExplanation}</small>
     <p>{candidate.excerpt}</p>
     <a href={candidate.url} target="_blank" rel="noopener noreferrer">{candidate.url}</a>
     <button type="button" class="primary secondary" aria-pressed={selectedURL===candidate.url} onclick={()=>onchoose(candidate.url)}>{selectedURL===candidate.url?'Selected link':'Use this link'}</button>
    </article>
   {/each}
  </div>
 {/if}
</fieldset>

<style>
 .example-finder{border:1px solid var(--line,#d7dce2);border-radius:12px;padding:16px;margin:16px 0;min-width:0}
 legend{padding:0 6px;font-weight:600}
 .finder-results{display:grid;gap:12px;margin-top:16px;max-height:420px;overflow:auto}
 article{display:grid;gap:8px;border:1px solid var(--line,#d7dce2);border-radius:10px;padding:14px}
 article p{margin:0;font-size:13px}
 article a{overflow-wrap:anywhere;font-size:12px;color:inherit}
 article button{justify-self:start}
 .recommended{border-color:var(--highlight,#75d7f2);background:#75d7f20a}
 .recommendation{color:var(--highlight,#75d7f2);font-size:12px}
 small{font-size:12px;opacity:.75}
</style>
