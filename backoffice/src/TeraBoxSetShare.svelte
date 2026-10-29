<script>
 import { onMount } from 'svelte';
 import { Link2, X } from '@lucide/svelte';
 let { api, set, onclose } = $props();
 let result=$state(null), loading=$state(true), error=$state(''), notice=$state('');
 let query=$state(''), selectedKey=$state('');
 const filtered=$derived((result?.candidates||[]).filter(candidate=>`${candidate.account} ${candidate.name} ${candidate.path}`.toLowerCase().includes(query.trim().toLowerCase())));
 function key(candidate){return `${candidate.accountId}:${candidate.fileId}`;}
 function modal(node){node.showModal();return {destroy(){node.close();}};}
 function close(){if(!loading)onclose();}
 async function prepare(){
  try{result=await api(`/admin/sets/${set.id}/terabox/share`,{method:'POST'});}
  catch(e){error=e.message;}finally{loading=false;}
 }
 onMount(prepare);
 async function confirm(){
  const choice=result.candidates.find(candidate=>key(candidate)===selectedKey);
  if(!choice)return;
  loading=true;error='';
  try{result=await api(`/admin/sets/${set.id}/terabox/share-confirm`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({accountId:choice.accountId,fileId:choice.fileId})});}
  catch(e){error=e.message;}finally{loading=false;}
 }
 async function copy(){
  try{await navigator.clipboard.writeText(result.link);notice='Share link copied.';}
  catch{error='Could not copy the link. Select and copy it below.';}
 }
</script>

<dialog class="set-share-dialog" use:modal oncancel={event=>{event.preventDefault();close();}} aria-label={`Share TeraBox folder for ${set.title}`}>
 <div class="editor-heading"><div><span class="eyebrow">TERABOX SHARE · 7 DAYS</span><h2>Share folder for this set</h2></div><button class="icon-btn" aria-label="Close folder share" disabled={loading} onclick={close}><X size={20}/></button></div>
 <p class="set-context"><strong>{set.title}</strong><span>{set.group} · {set.source||'Source unlisted'} · set #{set.id}</span></p>
 {#if loading}<p role="status">{result?.status==='review'?'Creating 7-day link…':'Reading TeraBox folders and asking Jev…'}</p>{/if}
 {#if error}<p class="alert error" role="alert">{error}</p>{/if}
 {#if result?.status==='review' && !loading}
  {#if result.warning}<p class="alert error" role="status">{result.warning}</p>{/if}
  {#if result.suggested}<p class="review-note">Jev suggested <strong>{result.suggested.name}</strong> ({Math.round((result.confidence||0)*100)}% confidence). Select a folder to confirm.</p>{/if}
  <label>Find folder<input aria-label="Find TeraBox folder" bind:value={query} oninput={()=>selectedKey=''} placeholder="Search folder name or account"/></label>
  <label>Choose TeraBox folder<select aria-label="Choose TeraBox folder" bind:value={selectedKey} disabled={!filtered.length}><option value="">Select a folder</option>{#each filtered.slice(0,100) as candidate}<option value={key(candidate)}>{candidate.account} · {candidate.path}</option>{/each}</select></label>
  {#if filtered.length>100}<p class="review-note">Showing the first 100 matches. Search to narrow the list.</p>{/if}
  <div class="editor-footer"><button class="primary" onclick={confirm} disabled={!selectedKey}>Create 7-day link</button></div>
 {:else if result?.status==='shared'}
  <p class="review-note">Shared folder: <strong>{result.folder?.name}</strong> · {result.folder?.account} · expires after 7 days.</p>
  <a class="share-link" href={result.link} target="_blank" rel="noopener noreferrer">{result.link}</a>
  <button class="primary secondary" onclick={copy}><Link2 size={16}/> Copy link</button>
  {#if notice}<p class="alert success" role="status">{notice}</p>{/if}
 {/if}
</dialog>

<style>
 .set-share-dialog{width:min(680px,94vw);max-height:90vh;padding:24px;border:1px solid var(--line,#ddd);border-radius:16px;background:var(--surface,#101b2b);color:#edf4fb;overflow:auto}
 .set-share-dialog::backdrop{background:#15151bcc;backdrop-filter:blur(4px)}
 .set-context{display:flex;flex-direction:column;gap:5px;padding:12px;border:1px solid #314d6b;border-radius:8px}.set-context span,.review-note{color:#abc4da;font-size:12px}
 label{display:flex;flex-direction:column;gap:6px;margin:14px 0;font-size:13px;font-weight:600}input,select{width:100%;min-width:0;padding:10px;border:1px solid #314d6b;border-radius:7px;background:#0a1423;color:#edf4fb}
 .share-link{display:block;overflow-wrap:anywhere;color:#a8dcff;margin:12px 0}.editor-footer{display:flex;justify-content:flex-end;margin-top:16px}
 @media(max-width:600px){.set-share-dialog{padding:16px}}
</style>
