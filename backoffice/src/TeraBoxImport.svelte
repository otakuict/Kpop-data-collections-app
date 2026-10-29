<script>
 import { onMount } from 'svelte';
 import { X, RefreshCw, Check, LoaderCircle } from '@lucide/svelte';
 import ExampleFinder from './ExampleFinder.svelte';
 import TeraBoxNames from './TeraBoxNames.svelte';
 let { api, sources=[], onclose, onsaved } = $props();
 let accounts=$state([]), groups=$state([]), accountID=$state(1), lookbackDays=$state(5), working=$state(false);
 let error=$state(''), notice=$state(''), preview=$state(null), results=$state(null);
 let sheetEnabled=$state(false), jevConfigured=$state(false), pane=$state('accounts');
 let namesOpen=$state(false);
 let memberGroup=$state(''), memberName=$state(''), memberAliases=$state(''), memberSource=$state('');
 const selectedAccount=$derived(accounts.find(a=>a.id===accountID));
 const selectedRows=$derived(preview?.rows.filter(r=>r.selected)||[]);
 const memberCount=$derived(groups.reduce((sum,g)=>sum+g.members.length,0));
 const validLookback=$derived(Number.isInteger(lookbackDays)&&lookbackDays>=1&&lookbackDays<=10);
 const duplicateTimers=new Map();
 function clearDuplicateChecks(){for(const timer of duplicateTimers.values())clearTimeout(timer);duplicateTimers.clear();}
 function checkFinalTitle(row){
  clearTimeout(duplicateTimers.get(row.fileId));
  row.checkVersion=(row.checkVersion||0)+1;
  row.duplicate=false;row.checkError='';row.checking=Boolean(row.title.trim()&&row.group&&row.setDate);
  if(!row.checking)return;
  const version=row.checkVersion;
  duplicateTimers.set(row.fileId,setTimeout(async()=>{
   duplicateTimers.delete(row.fileId);
   try{
    const params=new URLSearchParams({title:row.title,group:row.group,setDate:row.setDate});
    const result=await api(`/admin/terabox/title-check?${params}`);
    if(row.checkVersion===version&&preview?.rows.includes(row))row.duplicate=result.duplicate;
   }catch(e){
    if(row.checkVersion===version&&preview?.rows.includes(row))row.checkError=e.message;
   }finally{
    if(row.checkVersion===version&&preview?.rows.includes(row))row.checking=false;
   }
  },300));
 }
 function modal(node){node.showModal();return {destroy(){node.close();}};}
 async function load(){
  const [a,g]=await Promise.all([api('/admin/terabox/accounts'),api('/admin/artists')]);
  accounts=a.accounts;groups=g.groups;sheetEnabled=a.sheetSyncEnabled;jevConfigured=a.jevConfigured;
 }
 onMount(()=>{working=true;load().then(changeAccount).catch(e=>error=e.message).finally(()=>working=false);return clearDuplicateChecks;});
 function changeAccount(){
  clearDuplicateChecks();
  preview=null;results=null;notice='';error='';
 }
 async function scan(){
  clearDuplicateChecks();
  working=true;error='';notice='';results=null;preview=null;
  try{
   const p=await api('/admin/terabox/preview',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({accountId:accountID,days:lookbackDays})});
   preview={...p,rows:p.rows.map(row=>({...row,selected:row.status==='new',seller:selectedAccount?.source||'',example:'',notes:'',duplicate:false,checking:false,checkError:''}))};pane='preview';
  }catch(e){error=e.message;}finally{working=false;}
 }
 async function confirm(){
  working=true;error='';notice='';
  try{
   const rows=selectedRows.map(({fileId,title,group,setDate,seller,example,notes})=>({fileId,title,group,setDate,source:seller.trim(),example,notes}));
   const response=await api('/admin/terabox/commit',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({previewId:preview.id,rows})});
   results=response.results;
   for(const result of results){const row=preview.rows.find(r=>r.fileId===result.fileId);if(row){row.selected=false;row.status=result.status==='saved'?'duplicate':result.status;row.warning=result.error||'';}}
   notice=`Saved ${results.filter(r=>r.status==='saved').length} sets to Google Sheet and DB. ${results.filter(r=>r.status==='duplicate').length} already existed.`;
   await onsaved?.();
  }catch(e){error=e.message;}finally{working=false;}
 }
 async function saveMember(event){
  event.preventDefault();working=true;error='';notice='';
  try{
   await api('/admin/artists',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({group:memberGroup,name:memberName.trim(),aliases:memberAliases.split(',').map(a=>a.trim()).filter(Boolean),source:memberSource.trim()})});
   await load();notice='Artist aliases saved. Scan again to use the updated catalog.';memberName='';memberAliases='';memberSource='';
  }catch(e){error=e.message;}finally{working=false;}
 }
 async function recover(){
  working=true;error='';notice='';
  try{
   const response=await api('/admin/terabox/recover',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({accountId:accountID})});
   const unresolved=response.results.filter(r=>r.status==='reconcile');
   notice=`Sheet checked: ${response.results.filter(r=>r.status==='saved').length} reconciled, ${response.results.filter(r=>r.status==='retry').length} ready to scan and review again.${unresolved.length?' Some matching rows still need manual Sheet review.':''}`;
   if(!unresolved.length){preview=null;pane='accounts';}await onsaved?.();
  }catch(e){error=e.message;}finally{working=false;}
 }
 function pickArtist(row,id){
  const member=groups.flatMap(g=>g.members).find(m=>m.id===Number(id));if(!member)return;
  row.memberId=member.id;row.group=member.group;
  const candidates=[member.name,...member.aliases];
  const matched=candidates.sort((a,b)=>b.length-a.length).find(a=>row.rawName.toLowerCase().startsWith(a.toLowerCase()));
  const suffix=matched?row.rawName.slice(matched.length):row.rawName.match(/(\s*(?:\d+|premium)(?:\s+.*)?)$/i)?.[0]||'';
  row.title=(row.setDate?row.setDate.replaceAll('-','').slice(2)+' ':'')+member.name+suffix;
  row.method='manual';row.warning='';
  checkFinalTitle(row);
 }
 function editMember(group,member){memberGroup=group;memberName=member.name;memberAliases=member.aliases.join(', ');memberSource=member.source;}
</script>

<dialog class="tera-dialog" use:modal oncancel={event=>{event.preventDefault();if(!working)onclose();}} aria-labelledby="terabox-title">
 <div class="editor-heading">
  <div><span class="eyebrow">PRIVATE DRIVE IMPORT</span><h2 id="terabox-title">Update from TeraBox</h2></div>
  <button class="icon-btn" aria-label="Close TeraBox" disabled={working} onclick={onclose}><X size={20}/></button>
 </div>
 <div class="tera-toolbar">
  <label>TeraBox account<select aria-label="TeraBox account" bind:value={accountID} onchange={changeAccount} disabled={working}><option value={1}>{accounts.find(a=>a.id===1)?.label||'Account 1 — not connected'}</option><option value={2}>{accounts.find(a=>a.id===2)?.label||'Account 2 — not connected'}</option></select></label>
  <label class="lookback-days">Look back (days)<input type="number" min="1" max="10" step="1" bind:value={lookbackDays} disabled={working} required/></label>
  <button class="primary" disabled={working||!selectedAccount||!validLookback} onclick={scan}>{#if working}<LoaderCircle size={16} class="spin"/>{:else}<RefreshCw size={16}/>{/if}{working?'Working…':validLookback?`Scan last ${lookbackDays} ${lookbackDays===1?'day':'days'}`:'Scan TeraBox'}</button>
  <button class="primary secondary" disabled={working} onclick={()=>namesOpen=true}>Get TeraBox names</button>
 </div>
 <nav class="tera-tabs" aria-label="TeraBox steps">
  <button class:active={pane==='accounts'} disabled={working} onclick={()=>{pane='accounts';changeAccount();}}>Accounts</button>
  <button class:active={pane==='preview'} disabled={working||!preview} onclick={()=>pane='preview'}>Preview {preview?`(${preview.rows.length})`:''}</button>
  <button class:active={pane==='artists'} disabled={working} onclick={()=>pane='artists'}>Artist catalog ({memberCount})</button>
 </nav>
 {#if error}<p class="alert error" role="alert">{error}</p>{/if}
 {#if notice}<p class="alert success" role="status">{notice}</p>{/if}
 {#if !sheetEnabled}<p class="alert error">Google Sheet sync is disabled. You can preview, but confirmation requires enabling Sheet sync.</p>{/if}
 {#if !jevConfigured}<p class="form-help">Jev is not configured. Known aliases still resolve from the catalog; other names need manual review.</p>{/if}
 {#if pane==='accounts'}
  {#if selectedAccount}<p class="review-note">Connected account: <strong>{selectedAccount.label}</strong> · Root: <code>{selectedAccount.root}</code> · API host: {selectedAccount.host||'www.terabox.com'}.</p>{:else}<p class="review-note">This account is not connected yet.</p>{/if}
  <p class="form-help">Use the separate <strong>TeraBox auth</strong> button on the back office page to connect or update an account session.</p>
 {:else if pane==='preview'&&preview}
  <p class="review-note">{preview.scanned} entries scanned · {preview.complete?'Scan complete':'Partial scan'} · created or updated within the past {preview.days} {preview.days===1?'day':'days'}. Review the original, suggested name and group. Nothing is imported until you confirm.</p>
  {#each preview.warnings as warning}<p class="alert error">{warning}</p>{/each}
  {#if preview.rows.some(row=>row.status==='reconcile')}<p class="form-help">A previous save has an uncertain result. Check the live Sheet before making a new attempt.</p><button class="primary secondary" disabled={working||!sheetEnabled} onclick={recover}>Check Sheet & recover</button>{/if}
  {#if !preview.rows.length}<p class="table-status">No created or updated entries found within the last {preview.days} {preview.days===1?'day':'days'}.</p>{/if}
  <div class="tera-rows">
  {#each preview.rows as row (row.fileId)}
   <article class="tera-row" class:muted={row.status==='reconcile'} class:duplicate={row.status==='duplicate'||row.duplicate}>
    <div class="tera-row-heading"><label><input type="checkbox" bind:checked={row.selected} disabled={working||['duplicate','reconcile','pending','saved'].includes(row.status)} aria-label={`Import ${row.original}`}/><strong>{row.original}</strong></label><span>{row.duplicate?'duplicate':row.status} · {row.method}{row.confidence!=null?` · ${Math.round(row.confidence*100)}%`:''}</span></div>
    <small class="tera-path">{row.path} · last activity {new Date(row.updatedAt||row.createdAt).toLocaleString()}</small>
    <p class="form-help">KOROMAN: {row.romanized}</p>
    {#if row.warning}<p class="alert error">{row.warning}</p>{/if}
    {#if row.status!=='duplicate'&&row.status!=='reconcile'}
     <label>Artist suggestion<select value={row.memberId} onchange={event=>pickArtist(row,event.currentTarget.value)} disabled={working}><option value={0}>Choose from the catalog / edit manually</option>{#each groups as g}<optgroup label={g.name}>{#each g.members as member}<option value={member.id}>{member.name}</option>{/each}</optgroup>{/each}</select></label>
     <div class="form-row"><label>Final title<input bind:value={row.title} oninput={event=>{row.title=event.currentTarget.value;checkFinalTitle(row);}} maxlength="500" disabled={working}/></label><label>Group / solo<select bind:value={row.group} onchange={event=>{row.group=event.currentTarget.value;checkFinalTitle(row);}} disabled={working}><option value="">Choose a group</option>{#each groups as g}<option value={g.name}>{g.name}</option>{/each}</select></label><label>Photographed date<input type="date" bind:value={row.setDate} onchange={event=>{row.setDate=event.currentTarget.value;checkFinalTitle(row);}} disabled={working}/></label></div>
     <label class="tera-seller">Seller / Source<input bind:value={row.seller} list={`tera-sellers-${preview.id}-${row.fileId}`} maxlength="80" placeholder="Choose a source or type a seller nickname" disabled={working}/><small>Used for this set and its example link search.</small></label>
     <datalist id={`tera-sellers-${preview.id}-${row.fileId}`}>{#each sources as option}<option value={option.name}></option>{/each}</datalist>
     {#if row.duplicate}<p class="alert error" role="alert">Final title already exists in this group. Change the title before confirming.</p>{/if}
     {#if row.checkError}<p class="alert error" role="alert">Could not check Final title: {row.checkError}</p>{/if}
     <details class="tera-finder">
      <summary>Find an example link</summary>
      <ExampleFinder source={row.seller} {sources} setDate={row.setDate} name={row.title} group={row.group} selectedURL={row.example} disabled={working} {api} onchoose={url=>row.example=url} onsellerchange={value=>row.seller=value}/>
     </details>
     <div class="form-row"><label>Example URL (optional)<input bind:value={row.example} maxlength="2000" placeholder="Public example link" disabled={working}/></label><label>Notes<input bind:value={row.notes} maxlength="10000" disabled={working}/></label></div>
    {/if}
   </article>
  {/each}
  </div>
  {#if results}{#each results.filter(r=>r.error) as result}<p class="alert error" role="alert">Folder #{result.fileId}: {result.error}</p>{/each}{/if}
  <p class="form-help">Confirmation uses the existing Google Sheet → DB flow. Existing sets are skipped. Preview images can be added or fetched using the usual gallery tools after saving.</p>
  <div class="editor-footer"><span>{selectedRows.length} selected</span><button class="primary" disabled={working||!sheetEnabled||!selectedRows.length||selectedRows.some(r=>!r.title.trim()||!r.group||!r.setDate||r.duplicate||r.checking||r.checkError)} onclick={confirm}><Check size={16}/>{working?'Saving…':'Confirm to Sheet & DB'}</button></div>
 {:else if pane==='artists'}
  <p class="review-note">{groups.filter(g=>g.kind==='group').length} gallery groups · {groups.filter(g=>g.kind==='solo').length} solo categories · {memberCount} artist memberships. Includes historical members for older sets. Sources are saved per artist; ambiguous names remain separate by group.</p>
  <form onsubmit={saveMember} class="tera-account-form">
   <div class="form-row"><label>Artist group<select bind:value={memberGroup} required disabled={working}><option value="">Choose from gallery</option>{#each groups as g}<option value={g.name}>{g.name}</option>{/each}</select></label><label>English name<input bind:value={memberName} required maxlength="150" disabled={working}/></label></div>
   <label>Aliases (comma separated)<input bind:value={memberAliases} placeholder="김채원, 채원, Chaewon" disabled={working}/></label>
   <label>Source reference<input bind:value={memberSource} maxlength="1000" placeholder="Profile URL or operator reference" disabled={working}/></label>
   <button class="primary secondary" disabled={working}>Save artist / aliases</button>
  </form>
  {#each groups as g}<details class="tera-group"><summary>{g.name} · {g.members.length} members</summary>{#each g.members as member}<div class="tera-member"><button class="text-btn" onclick={()=>editMember(g.name,member)}>{member.name}</button><small>{member.aliases.join(', ')}</small>{#if member.source.startsWith('https://')}<a href={member.source} target="_blank" rel="noreferrer">Source</a>{/if}</div>{/each}</details>{/each}
 {/if}
</dialog>

{#if namesOpen}
 <TeraBoxNames {api} {accounts} initialAccountID={accountID} initialDays={validLookback?lookbackDays:5} onclose={()=>namesOpen=false}/>
{/if}

<style>
 .tera-dialog{width:min(1100px,96vw);max-height:92vh;padding:28px;border:1px solid var(--line,#ddd);border-radius:18px;background:var(--surface,#101b2b);color:#edf4fb;overflow:auto}
 .tera-dialog::backdrop{background:#15151b99;backdrop-filter:blur(4px)}
 .tera-toolbar,.tera-tabs,.tera-row-heading,.tera-member{display:flex;align-items:center;justify-content:space-between;gap:14px}
 .tera-toolbar{display:flex;flex-wrap:wrap;align-items:end;justify-content:start;margin:20px 0;gap:14px}.tera-toolbar label{min-width:0;margin:0}.tera-toolbar label:first-child{width:min(320px,100%)}.tera-toolbar .lookback-days{width:140px}.tera-toolbar input,.tera-toolbar select{height:42px}.tera-toolbar .primary{min-height:42px;white-space:nowrap}
 label{display:flex;flex-direction:column;gap:6px;font-size:13px;font-weight:600;color:#edf4fb;margin:8px 0}input,select{min-width:0;width:100%;padding:10px;border:1px solid #314d6b;border-radius:7px;background:#0a1423;color:#edf4fb}input[type=checkbox]{width:auto}
 .tera-tabs{justify-content:flex-start;flex-wrap:wrap;border-bottom:1px solid #314d6b;margin-bottom:20px}.tera-tabs button{padding:10px;border:0;background:none;border-bottom:2px solid transparent}.tera-tabs button.active{border-color:#5fd2ef;font-weight:700}
 .tera-account-form{display:grid;gap:16px;margin:20px 0}.form-row{display:flex;gap:14px;flex-wrap:wrap}.form-row>label{flex:1;min-width:180px}
 .tera-rows{display:grid;gap:16px}.tera-row{display:grid;gap:12px;border:1px solid #314d6b;border-radius:12px;padding:18px}.tera-row.muted{background:#0b1522}.tera-row.duplicate{background:linear-gradient(135deg,#432a17,#2c2330 78%);border-color:#efbd71;box-shadow:inset 4px 0 #ffcb79,0 0 0 1px #efbd7133}.tera-row.duplicate .tera-row-heading span{color:#231509;background:#ffd18a;border:1px solid #ffe4b5;border-radius:5px;padding:5px 8px;font-weight:700}.tera-row.duplicate .tera-row-heading strong{color:#ffe2ac}.tera-row-heading{align-items:flex-start;flex-wrap:wrap}.tera-row-heading label{flex-direction:row;align-items:center}.tera-row-heading span,.tera-path{font-size:12px;color:#9aaec3;overflow-wrap:anywhere}.tera-path{display:block}.tera-seller{max-width:420px}.tera-seller small{color:#abc4da;font-weight:400}.tera-group{padding:12px 0;border-top:1px solid #314d6b}.tera-member{padding:8px 0;flex-wrap:wrap}.tera-member small{flex:1;color:#9aaec3;overflow-wrap:anywhere}.form-help{font-size:11px}.review-note{color:#cedbe9;font-size:12px}summary{cursor:pointer}
 @media(max-width:900px){.tera-toolbar{grid-template-columns:minmax(0,1fr) 140px}.tera-toolbar .primary{width:100%}}
 @media(max-width:600px){.tera-dialog{padding:16px}.tera-toolbar{grid-template-columns:minmax(0,1fr) 115px}.tera-toolbar .primary{grid-column:1/-1;width:100%}.form-row>label{min-width:100%}.tera-row{padding:12px}}
</style>
