<script>
 import { onDestroy, untrack } from 'svelte';
 import { X, List, LoaderCircle, Copy } from '@lucide/svelte';
 let {api,accounts,initialAccountID,initialDays,onclose}=$props();
 let accountID=$state(untrack(()=>initialAccountID)),days=$state(untrack(()=>initialDays)),allDates=$state(false);
 let loading=$state(false),error=$state(''),result=$state(null),copying=$state(false),copyNotice=$state(''),copyError=$state('');
 let controller;
 const account=$derived(accounts.find(a=>a.id===accountID));
 const validDays=$derived(allDates||(Number.isInteger(days)&&days>=1&&days<=10));
 function modal(node){node.showModal();return {destroy(){node.close();}};}
 function clearResults(){result=null;error='';copyNotice='';copyError='';}
 onDestroy(()=>controller?.abort());
 async function copyJSON(){
  const current=result;
  copying=true;copyNotice='';copyError='';
  try{
   const json=JSON.stringify(current.entries.map(entry=>({name:entry.name,modify_at:entry.modifiedAt||''})),null,2);
   await navigator.clipboard.writeText(json);
   if(result===current)copyNotice='Copied JSON.';
  }catch{
   if(result===current)copyError='Could not copy JSON. Allow clipboard access and try again.';
  }finally{copying=false;}
 }
 async function getNames(event){
  event.preventDefault();loading=true;clearResults();controller=new AbortController();
  try{
   result=await api('/admin/terabox/names',{method:'POST',headers:{'Content-Type':'application/json'},signal:controller.signal,body:JSON.stringify({accountId:accountID,days:allDates?0:days})});
  }catch(e){if(e.name!=='AbortError')error=e.message;}finally{loading=false;}
 }
</script>

<dialog class="tera-names-dialog" use:modal aria-labelledby="tera-names-title" oncancel={event=>{event.preventDefault();onclose();}}>
 <div class="editor-heading"><h2 id="tera-names-title">TeraBox names</h2><button type="button" class="icon-btn" aria-label="Close names" onclick={onclose}><X size={20}/></button></div>
 <p class="review-note">View original folder and file names directly inside the saved root folder, including ZIP files. Subfolder contents are not read. Nothing is imported.</p>
 <form onsubmit={getNames}>
  <div class="names-controls">
   <label>Names account<select aria-label="Names account" bind:value={accountID} onchange={clearResults} disabled={loading}><option value={1}>{accounts.find(a=>a.id===1)?.label||'Account 1 — not connected'}</option><option value={2}>{accounts.find(a=>a.id===2)?.label||'Account 2 — not connected'}</option></select></label>
   <label>Look back (days)<input type="number" min="1" max="10" step="1" bind:value={days} oninput={clearResults} disabled={loading||allDates} required={!allDates}/></label>
   <button class="primary" disabled={loading||!account||account.configured===false||!validDays}>{#if loading}<LoaderCircle size={16} class="spin"/>{:else}<List size={16}/>{/if}{loading?'Getting names…':'Get names'}</button>
  </div>
  <label class="all-dates"><input type="checkbox" bind:checked={allDates} onchange={clearResults} disabled={loading}/>All dates</label>
  <p class="root-path">Saved root: <code>{account?.root||'Connect this account first'}</code></p>
 </form>
 {#if error}<p class="alert error" role="alert">{error}</p>{/if}
 {#if loading}<p class="table-status" role="status">Reading names from TeraBox…</p>{/if}
 {#if result}
  <div class="names-result-heading">
  <p class="result-summary" role="status">{result.entries.length} names · {result.scanned} entries scanned · {result.complete?'Complete':'Partial results'} · {result.days?`past ${result.days} days`:'all dates'}</p>
  <button type="button" class="primary secondary" disabled={copying} onclick={copyJSON}><Copy size={16}/>Copy JSON</button>
  </div>
  {#if copyNotice}<p class="form-help" role="status">{copyNotice}</p>{/if}
  {#if copyError}<p class="alert error" role="alert">{copyError}</p>{/if}
  {#each result.warnings as warning}<p class="alert error" role="alert">{warning}</p>{/each}
  {#if !result.entries.length}<p class="table-status">No names found for these settings.</p>{:else}
   <ol class="names-list">
    {#each result.entries as entry (entry.fileId)}<li><span class="entry-kind">{entry.isDir?'Folder':'File'}</span><div><strong>{entry.name}</strong><small>{entry.path}</small><small>Modified (Bangkok): {#if entry.modifiedAt}<time datetime={entry.modifiedAt}>{new Date(entry.modifiedAt).toLocaleString('en-GB',{timeZone:'Asia/Bangkok',hour12:false})}</time>{:else}Not available{/if}</small></div></li>{/each}
   </ol>
  {/if}
 {/if}
</dialog>

<style>
 .tera-names-dialog{width:min(900px,94vw);max-height:90vh;padding:24px;border:1px solid var(--line,#314d6b);border-radius:16px;background:var(--surface,#101b2b);color:#edf4fb;overflow:auto}
 .tera-names-dialog::backdrop{background:#15151bcc;backdrop-filter:blur(4px)}
 .review-note,.root-path,.result-summary{font-size:12px;line-height:1.7;color:#cedbe9}.root-path{overflow-wrap:anywhere}
 .names-controls{display:grid;grid-template-columns:minmax(0,1fr) 140px auto;align-items:end;gap:14px}
 label{display:flex;flex-direction:column;gap:6px;font-size:13px;font-weight:600;margin:0}
 input,select{width:100%;min-width:0;height:42px;padding:10px;border:1px solid #314d6b;border-radius:7px;background:#0a1423;color:#edf4fb}
 .names-controls button{min-height:42px;white-space:nowrap}
 .all-dates{flex-direction:row;align-items:center;margin-top:14px}.all-dates input{width:16px;height:16px}
 .names-result-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.names-result-heading button{white-space:nowrap}
 .names-list{list-style:none;padding:0;margin:16px 0 0;border-top:1px solid #314d6b}
 .names-list li{display:flex;align-items:flex-start;gap:14px;padding:12px 0;border-bottom:1px solid #25384c}.names-list li>div{min-width:0}
 .names-list strong{font-size:13px;font-weight:600;overflow-wrap:anywhere}.names-list small{display:block;margin-top:5px;font-size:11px;color:#9aaec3;overflow-wrap:anywhere}
 .entry-kind{flex:0 0 45px;color:#75d7f2;font-size:11px;padding-top:2px}
 @media(max-width:600px){.tera-names-dialog{padding:16px}.names-controls{grid-template-columns:minmax(0,1fr) 115px}.names-controls button{grid-column:1/-1;width:100%}}
</style>
