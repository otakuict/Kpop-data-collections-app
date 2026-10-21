<script>
 import { onMount } from 'svelte';
 import { previewSheet, commitSheet, extractSets, reloadSets } from './sheet-scan.js';
 import { prepareUploadImages } from './upload-images.js';
 import { Asterisk, ArrowUpRight, Plus, Search, X, Upload, RefreshCw, FileSpreadsheet, Image as ImageIcon, Check, ArrowRight, LogOut, ChevronLeft, ChevronRight, CircleAlert, Pencil, LoaderCircle } from '@lucide/svelte';
 const sessionKey='bias.admin.jwt.v1';
 let adminToken=$state(''), token=$state(''), authenticated=$state(false), authenticating=$state(false), restoring=$state(true), loginError=$state('');
 let tab=$state('catalog'), sets=$state([]), facets=$state({total:0,ready:0,groups:[],sources:[]}), q=$state(''), group=$state(''), page=$state(1), total=$state(0), loading=$state(false);
 let editImages=$state([]), originalExample=$state(''), ingestion=$state({intervalSeconds:20,pausedUntil:'',reason:''});
 let message=$state(''), error=$state(''), busy=$state(false), sheetPreviewing=$state(false), editor=$state(false), editID=$state(0), editOrigin=$state('manual'), sheetSyncEnabled=$state(false), sheetSyncConfigured=$state(false), sheetSyncRestartRequired=$state(false), credentialFile=$state(null), form=$state(blank()), files=$state([]);
 let sheetUrl=$state('https://docs.google.com/spreadsheets/d/1z45mKRtLvTZth8b-uhqkMyjzxVtHI8b7YVcySQVeEkg/edit?gid=0'), csvFile=$state(null), report=$state(null);
 let importPreview=$state(null), pendingScanIds=$state([]);
 let extracting=$state(false), progress=$state({done:0,total:0,ready:0,failed:0}), current=$state(''), stopRequested=$state(false), logs=$state([]);
 let reloadingPage=$state(false), reloadProgress=$state({done:0,total:0,reloaded:0,failed:0}), reloadCurrent=$state(''), reloadStopRequested=$state(false);
 let selectedSetIds=$state([]), reloadScope=$state('');
 const selectedCount=$derived(sets.filter(set=>selectedSetIds.includes(set.id)).length);
 let debounce, requestSequence=0;
 const galleryURL=import.meta.env.VITE_GALLERY_URL || 'http://localhost:5173';
 const missing=$derived(Math.max(0,facets.total-(facets.complete||0)));
 const gankPaused=$derived(Boolean(ingestion.pausedUntil&&Date.parse(ingestion.pausedUntil)>Date.now()));
 function isGankExample(example){try{const host=new URL(example).hostname.replace(/\.$/,'');return host==='ganknow.com'||host.endsWith('.ganknow.com');}catch{return false;}}
 function blank(){return {title:'',setDate:'',name:'',group:'',date:'',source:'',example:'',notes:'',style:{color:'',wishlist:false,new:true,noTrade:false,selective:false}};}
 const checkedRibbonNotes=$derived([form.style.wishlist&&'Wish list',form.style.new&&'New',form.style.noTrade&&'No trade',form.style.selective&&'Selective'].filter(Boolean).join('; '));
 async function api(path,options={}) {
  const response=await fetch(`/api${path}`,{...options,headers:{Authorization:`Bearer ${token}`,...options.headers}});
  const result=await response.json();if(response.status===429&&result.pausedUntil)ingestion={...ingestion,pausedUntil:result.pausedUntil,reason:result.error};if(response.status===401){authenticated=false;token='';sessionStorage.removeItem(sessionKey);throw new Error('Session expired. Sign in again.');}if(!response.ok){const failure=new Error(result.error||'Request failed');failure.pausedUntil=result.pausedUntil;throw failure;}return result;
 }
 async function login(e){
  e.preventDefault();authenticating=true;loginError='';
  try{
   const response=await fetch('/api/admin/session',{method:'POST',headers:{Authorization:`Bearer ${adminToken}`}});
   const result=await response.json();
   if(!response.ok)throw new Error(result.error||'Sign in failed');
   token=result.token;sessionStorage.setItem(sessionKey,token);adminToken='';authenticated=true;await load();
  }catch(e){loginError=e.message;}finally{authenticating=false;}
 }
 async function restoreSession(){
  token=sessionStorage.getItem(sessionKey)||'';
  try{if(token){await api('/admin/status');authenticated=true;await load();}}
  catch(e){loginError=e.message;}
  finally{restoring=false;}
 }
 async function load(){
  const seq=++requestSequence;loading=true;
  try{const query=new URLSearchParams({q,group,page:String(page),limit:'40'});const [result,f,status,adminStatus]=await Promise.all([api(`/sets?${query}`),api('/facets'),api('/admin/ingestion'),api('/admin/status')]);if(seq!==requestSequence)return;sets=result.items;selectedSetIds=[];total=result.total;facets=f;ingestion=status;sheetSyncEnabled=!!adminStatus.sheetSyncEnabled;sheetSyncConfigured=!!adminStatus.sheetSyncConfigured;sheetSyncRestartRequired=!!adminStatus.sheetSyncRestartRequired;}
  catch(e){if(seq===requestSequence)error=e.message;}finally{if(seq===requestSequence)loading=false;}
 }
 function search(){page=1;clearTimeout(debounce);debounce=setTimeout(load,250);}
 async function uploadSheetCredentials(e){
  e.preventDefault();if(!credentialFile||busy)return;
  busy=true;error='';message='';
  try{const body=new FormData();body.append('credentials',credentialFile);await api('/admin/sheet-credentials',{method:'POST',body});credentialFile=null;message='Google service account saved. Restart the backend Pod to activate Sheet sync.';await load();}
  catch(e){error=e.message;}finally{busy=false;}
 }
 onMount(()=>{restoreSession();return ()=>{clearTimeout(debounce);stopRequested=true;reloadStopRequested=true;};});
 function newSet(){form=blank();editID=0;editOrigin='manual';editImages=[];originalExample='';files=[];editor=true;message='';error='';}
 function edit(set){form={title:set.title,group:set.group,date:set.date,source:set.source,example:set.example,notes:set.notes,style:{color:set.style?.color||'',wishlist:!!set.style?.wishlist,new:!!set.style?.new,noTrade:!!set.style?.noTrade,selective:!!set.style?.selective}};editID=set.id;editOrigin=set.origin;editImages=set.images;originalExample=set.example;files=[];editor=true;message='';error='';}
 function pasteImages(e){
  if(busy)return;
  const items=Array.from(e.clipboardData?.items||[]).filter(item=>item.kind==='file'&&item.type.startsWith('image/'));
  if(!items.length)return;
  e.preventDefault();
  const pasted=items.map(item=>item.getAsFile()).filter(Boolean);
  if(pasted.some(file=>!['image/jpeg','image/png','image/gif','image/webp'].includes(file.type))){error='Paste a JPEG, PNG, GIF, or WebP image.';return;}
  const existing=form.example===originalExample?editImages.length:0;
  if(existing+files.length+pasted.length>5){error=`A set can hold at most 5 images. ${existing} stored, ${files.length} selected.`;return;}
  files=[...files,...pasted];error='';
 }
 async function rotateStoredImage(imageID){
  if(busy)return;
  busy=true;error='';
  try{const updated=await api(`/admin/sets/${editID}/images/${imageID}/rotate`,{method:'POST'});editImages=updated.images;await load();}
  catch(e){error=e.message;}finally{busy=false;}
 }
 async function save(e){
  e.preventDefault();error='';const existing=form.example===originalExample?editImages.length:0;if(files.length&&(files.length+existing<2||files.length+existing>5)){error=`Each set needs 2–5 images. ${existing} stored, ${files.length} selected.`;return;}busy=true;let saved;let sheetUpdated=false;let sheetRow=0;
  try{const uploadFiles=files.length?await prepareUploadImages(files):[];
   saved=await api(editID?`/admin/sets/${editID}`:'/admin/sets',{method:editID?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(form)});editID=saved.id;editOrigin=saved.origin;form.title=saved.title;form.date=saved.date;sheetUpdated=!!saved.sheetSynced;sheetRow=saved.sheetRow||0;
   if(uploadFiles.length){const body=new FormData();for(const file of uploadFiles)body.append('images',file);saved=await api(`/admin/sets/${saved.id}/images`,{method:'POST',body});}
   else if(form.example.startsWith('https://')&&saved.images.length<2&&!(gankPaused&&isGankExample(form.example))){saved=await api(`/admin/sets/${saved.id}/extract?target=2`,{method:'POST'});}
   editor=false;message=`Saved “${saved.title}”. ${sheetRow?`Added to Google Sheet row ${sheetRow}. `:sheetUpdated?'Google Sheet updated. ':''}${saved.images.length<2?`${saved.images.length}/5 images; needs at least 2. ${saved.imageError||'Add images or extract from the example post.'}`:`${saved.images.length}/5 images stored.`}`;await load();
  }catch(e){error=`${saved?'Metadata was saved. ':''}${e.message}`;}finally{busy=false;}
 }
 async function reloadImages(set){busy=true;error='';message='';try{const result=await api(`/admin/sets/${set.id}/reload-images`,{method:'POST'});message=result.imageError?`${set.title}: ${result.imageError}`:`${result.images.length}/5 images reloaded for “${set.title}”.`;await load();}catch(e){error=e.message;}finally{busy=false;}}
 function toggleSetSelection(id,checked){selectedSetIds=checked?[...selectedSetIds,id]:selectedSetIds.filter(selectedId=>selectedId!==id);}
 async function reloadPageImages(){
  if(busy||extracting||loading||!sets.length)return;
  clearTimeout(debounce);
  const selectedSets=sets.filter(set=>selectedSetIds.includes(set.id));
  const targetSets=(selectedSets.length?selectedSets:sets).map(set=>({id:set.id,title:set.title}));
  const pageNumber=page;
  reloadScope=selectedSets.length?`${selectedSets.length} selected set${selectedSets.length===1?'':'s'}`:`Page ${pageNumber}`;
  busy=true;reloadingPage=true;reloadStopRequested=false;reloadCurrent='';error='';message='';
  try{
   const batch=await reloadSets(api,targetSets,{
    shouldStop:()=>reloadStopRequested||!authenticated,
    onCurrent:set=>reloadCurrent=`Set #${set.id} · ${set.title}`,
    onProgress:status=>reloadProgress={done:status.done,total:status.total,reloaded:status.reloaded,failed:status.failed},
    onSet:updated=>sets=sets.map(set=>set.id===updated.id?updated:set)
   });
   message=`${reloadScope}: ${batch.reloaded} refreshed, ${batch.failed} unavailable (${batch.done}/${batch.total} attempted).${batch.stopped?' Stopped; remaining sets were not touched.':''}`;
   if(batch.pause)error=batch.pause.message;
   else if(batch.firstError)error=batch.firstError;
  }catch(e){error=e.message;}
  finally{reloadCurrent='';if(authenticated)await load();reloadingPage=false;busy=false;}
 }
 async function reviewImport(csv=null){
  busy=true;sheetPreviewing=true;message='';error='';
  try{importPreview=await previewSheet(api,sheetUrl,csv);}
  catch(e){error=e.message;}finally{sheetPreviewing=false;busy=false;}
 }
 async function reviewImagePolicy(policy){
  if(!importPreview||busy||importPreview.imagePolicy===policy)return;
  const reviewed=importPreview;
  busy=true;error='';
  try{const csv=new File([reviewed.csv],'reviewed-sheet.csv',{type:'text/csv'});importPreview=await previewSheet(api,reviewed.sheetUrl,csv,policy);}
  catch(e){error=e.message;}finally{busy=false;}
 }
 function importSheet(e){e.preventDefault();reviewImport(csvFile);}
 function batchOptions(){return {shouldStop:()=>stopRequested||!authenticated,onCurrent:id=>current=`Set #${id}`,onProgress:batch=>{progress={done:batch.done,total:batch.total,ready:batch.ready,failed:batch.failed};logs=[...batch.results].reverse().slice(0,100);}};}
 function finishBatch(batch){
  pendingScanIds=batch.remaining;
  if(batch.pause)error=batch.pause.message;
  return batch.stopped?`Image extraction paused. ${batch.ready} ready; ${batch.remaining.length} sets left. Saved metadata and images are retained.`:`Images finished: ${batch.ready} ready, ${batch.failed} unavailable.`;
 }
 async function confirmImport(){
  if(!importPreview||busy)return;
  const reviewed=importPreview;busy=true;error='';stopRequested=false;logs=[];progress={done:0,total:0,ready:0,failed:0};
  try{
   const result=await commitSheet(api,reviewed,{...batchOptions(),onImport:imported=>{report=imported;pendingScanIds=imported.scanCandidates;importPreview=null;tab='import';extracting=true;message=`Imported ${imported.total} rows: ${imported.created} new, ${imported.updated} updated.`;}});
   message=`Imported ${result.report.total} rows: ${result.report.created} new, ${result.report.updated} updated. ${finishBatch(result.batch)}`;
  }catch(e){error=e.message;}finally{busy=false;extracting=false;current='';await load();}
 }
 async function resumeImported(){
  extracting=true;stopRequested=false;error='';logs=[];
  try{const batch=await extractSets(api,pendingScanIds,{...batchOptions(),target:5});message=finishBatch(batch);}
  catch(e){error=e.message;}finally{extracting=false;current='';await load();}
 }
 async function extractMissing(){
  extracting=true;stopRequested=false;error='';logs=[];
  try{let ids=report?.candidates;if(!ids){ids=[];let p=1;for(;;){const result=await api(`/sets?limit=200&page=${p}`);ids.push(...result.items.filter(x=>x.images.length<2).map(x=>x.id));if(p*200>=result.total)break;p++;}}
   const batch=await extractSets(api,ids,{...batchOptions(),target:2});
   if(batch.pause)error=batch.pause.message;
   message=batch.stopped?'Extraction paused. Saved previews are retained.':`Extraction finished: ${batch.ready} ready, ${batch.failed} unavailable.`;report=null;await load();
  }catch(e){error=e.message;}finally{extracting=false;current='';}
 }
 function signOut(){stopRequested=true;reloadStopRequested=true;authenticated=false;token='';sessionStorage.removeItem(sessionKey);sets=[];selectedSetIds=[];message='';error='';importPreview=null;report=null;pendingScanIds=[];}
 function modal(node){node.showModal();return {destroy(){node.close();}};}
</script>

<svelte:head><title>BIAS — Back office</title></svelte:head>
<header><a class="brand" href={galleryURL}><span class="brand-mark"><Asterisk size={27} strokeWidth={2.6}/></span>Kpop Data<span>®</span><small>BACK OFFICE</small></a><div class="header-actions"><a href={galleryURL} target="_blank" rel="noreferrer">View gallery <ArrowUpRight size={15}/></a>{#if authenticated}<button onclick={signOut} disabled={busy||extracting}><LogOut size={15}/> Sign out</button>{/if}</div></header>
{#if restoring}
 <main class="login-layout" aria-live="polite">Restoring session…</main>
{:else if !authenticated}
 <main class="login-layout"><div class="login-intro"><div class="eyebrow">BEHIND THE COLLECTION</div><h1>Good moments.<br/><em>In good hands.</em></h1><p>Your workspace for keeping the archive growing.<br/>Add sets, bring in your sheet, and keep every preview in focus.</p><div class="login-decoration"><Asterisk size={90} strokeWidth={1}/><span>FOR THE FANS.<br/>BY THE COLLECTORS.</span></div></div><form class="login-card" onsubmit={login}><span class="lock-label">ADMINISTRATOR ACCESS</span><h2>Welcome back.</h2><p>Enter the administrator token configured for this server.</p><label>Administrator token<input type="password" autocomplete="current-password" bind:value={adminToken} required placeholder="Your access token"/></label>{#if loginError}<div class="alert error" role="alert">{loginError}</div>{/if}<button class="primary" disabled={authenticating}>{authenticating?'Checking access…':'Open the workspace'} <ArrowRight size={17}/></button><p class="login-note">The admin token is exchanged for a 24-hour session. Refreshing this tab keeps you signed in; signing out clears the session.</p></form></main>
{:else}
 <main class="workspace"><div class="page-heading"><div><div class="eyebrow">COLLECTION WORKSPACE</div><h1>The back office<span>.</span></h1><p>A little care for every moment in the archive.</p></div><div class="heading-actions"><button class="primary" onclick={()=>reviewImport()} disabled={busy||extracting||pendingScanIds.length>0}><FileSpreadsheet size={17}/>{extracting?'Fetching images…':sheetPreviewing?'Reading sheet…':'Scan Google Sheet & import'}</button><button class="primary secondary" onclick={newSet} disabled={busy||extracting}><Plus size={17}/> Add image set</button></div></div>
 <div class="stats-grid"><div><span>IMAGE SETS</span><strong>{facets.total}</strong><small>Records in your collection</small></div><div><span>ARTISTS & GROUPS</span><strong>{facets.groups.length}</strong><small>A growing roster of favorites</small></div><div><span>SETS WITH 2–5 IMAGES</span><strong>{facets.complete||0}<i><Check size={19}/></i></strong><small>Stored locally in the archive</small></div><div><span>NEEDS MORE IMAGES</span><strong>{missing}</strong><small>Sets with fewer than 2 images</small></div></div>
 <nav class="tabs" aria-label="Workspace sections"><button class:active={tab==='catalog'} onclick={()=>tab='catalog'}><ImageIcon size={15}/> Image sets <span>{facets.total}</span></button><button class:active={tab==='import'} onclick={()=>tab='import'}><FileSpreadsheet size={15}/> Import & previews</button></nav>
 {#if gankPaused}<div class="alert error" role="status"><CircleAlert size={17}/>Gank ingestion paused until {new Date(ingestion.pausedUntil).toLocaleString()}. Uploads and browsing remain available.</div>{/if}
 {#if error}<div class="alert error" role="alert"><CircleAlert size={17}/>{error}<button aria-label="Dismiss error" onclick={()=>error=''}><X size={14}/></button></div>{/if}{#if message}<div class="alert success" role="status"><Check size={17}/>{message}<button aria-label="Dismiss message" onclick={()=>message=''}><X size={14}/></button></div>{/if}
 {#if tab==='catalog'}
 <section class="catalog-panel"><div class="panel-toolbar"><label class="search"><Search size={16}/><input aria-label="Search sets" placeholder="Search by artist, title, or source…" bind:value={q} oninput={search} disabled={reloadingPage}/></label><label class="group-filter">Artist<select aria-label="Filter by group" bind:value={group} onchange={()=>{page=1;load();}} disabled={reloadingPage}><option value="">All artists</option>{#each facets.groups as f}<option value={f.name}>{f.name}</option>{/each}</select></label><button class="icon-btn" onclick={load} aria-label="Refresh catalog" disabled={busy||extracting}><RefreshCw size={16}/></button><button class="primary page-reload" onclick={reloadPageImages} disabled={busy||extracting||loading||!sets.length} title={selectedCount?`Reload public images for ${selectedCount} selected sets`:'Reload public images for all sets shown on this page'}><RefreshCw size={15} class={reloadingPage?'spin':''}/> Reload page images ({selectedCount||sets.length})</button></div>
 {#if reloadingPage}<div class="page-reload-progress" role="status" aria-live="polite"><span>{reloadCurrent||`Reloading ${reloadScope}`} · {reloadProgress.done}/{reloadProgress.total} attempted · {reloadProgress.reloaded} refreshed · {reloadProgress.failed} unavailable</span><progress value={reloadProgress.done} max={reloadProgress.total||1}></progress><button class="text-btn" onclick={()=>reloadStopRequested=true} disabled={reloadStopRequested}>{reloadStopRequested?'Stopping after current set…':'Stop after current set'}</button></div>{/if}
 <div class="table-wrap"><table><thead><tr><th class="select-set-heading"><span class="sr-only">Select sets to reload</span></th><th>SET_ID</th><th>IMAGE SET</th><th>ARTIST</th><th>DATE</th><th>PREVIEW</th><th class="right">MANAGE</th></tr></thead><tbody>{#if loading}<tr><td colspan="7" class="table-status">Loading the collection…</td></tr>{:else if !sets.length}<tr><td colspan="7" class="table-status">No sets match your filters.</td></tr>{:else}{#each sets as set}<tr><td class="select-set-cell"><input type="checkbox" aria-label={`Select ${set.title} (set #${set.id}) for image reload`} checked={selectedSetIds.includes(set.id)} onchange={e=>toggleSetSelection(set.id,e.currentTarget.checked)} disabled={busy||extracting||loading}/></td><td class="set-id">{set.id}</td><td><div class="set-cell">{#if set.images.length}<img src={set.images[0].url} alt="" loading="lazy"/>{:else}<span class="thumb-placeholder"><ImageIcon size={17}/></span>{/if}<div><button class="set-title" onclick={()=>edit(set)} disabled={busy||extracting}>{set.title}</button><small>{set.source||'Source unlisted'} · {set.origin}</small></div></div></td><td><span class="artist-tag">{set.group}</span></td><td class="date">{set.date||'Unknown'}</td><td><span class="status-tag {set.images.length>=2?'ready':'pending'}" title={set.imageError}>{set.images.length}/5 {set.images.length>=2?'ready':'needs 2+'}</span>{#if set.importWarning}<small class="warning-label" title={set.importWarning}>Date needs review</small>{/if}</td><td><div class="row-actions"><button onclick={()=>reloadImages(set)} disabled={busy||extracting} aria-label={`Reload images for ${set.title}`} title="Reload images from public sources"><RefreshCw size={14}/></button><button onclick={()=>edit(set)} disabled={busy||extracting} aria-label={`Edit ${set.title}`} title="Edit set"><Pencil size={14}/></button>{#if set.example.startsWith('https://')}<a href={set.example} target="_blank" rel="noreferrer" aria-label={`Open example for ${set.title}`} title="Open example post"><ArrowUpRight size={15}/></a>{/if}</div></td></tr>{/each}{/if}</tbody></table></div>
 <div class="pagination"><span>{total} matching sets</span><div><button disabled={page===1||loading||busy||extracting} aria-label="Previous page" onclick={()=>{page--;load();}}><ChevronLeft size={16}/></button><span>{page} / {Math.max(1,Math.ceil(total/40))}</span><button disabled={page*40>=total||loading||busy||extracting} aria-label="Next page" onclick={()=>{page++;load();}}><ChevronRight size={16}/></button></div></div></section>
 {:else}
 <section class="import-card sheet-credentials"><span class="section-number">GOOGLE SHEET WRITEBACK</span><FileSpreadsheet size={27} strokeWidth={1.3}/><h2>Connect your Sheet.</h2><p>{sheetSyncEnabled?'Sheet sync is active. New sets and metadata edits will write to Google Sheet.':sheetSyncRestartRequired?'Credential saved in SQLite. Restart the backend Pod to activate Sheet sync.':sheetSyncConfigured?'Credential is stored, but Sheet sync could not start. Check backend logs and the administrator token.':'Upload your Google service account JSON key. It is stored encrypted in SQLite and loaded when the backend Pod restarts.'}</p><form onsubmit={uploadSheetCredentials}><label class="upload-label"><Upload size={17}/><span>{credentialFile?credentialFile.name:'Choose service account JSON from your PC'}</span><input aria-label="Choose Google service account JSON" type="file" accept=".json,application/json" onchange={e=>credentialFile=e.currentTarget.files?.[0]||null}/></label><button class="primary" disabled={busy||extracting||!credentialFile}><Upload size={16}/>{busy?'Saving…':'Save credential'}</button></form></section><div class="import-grid"><section class="import-card"><span class="section-number">01 / METADATA</span><FileSpreadsheet size={27} strokeWidth={1.3}/><h2>Bring your sheet.</h2><p>Scan your Google Sheet or CSV, review the changes, then confirm to import and fetch up to 5 images for new or changed examples.</p><form onsubmit={importSheet}><label>Google Sheet URL<input type="url" bind:value={sheetUrl} required disabled={busy||extracting||pendingScanIds.length>0}/></label><label class="upload-label"><Upload size={17}/><span>{csvFile?csvFile.name:'Or choose an exported CSV'}</span><input aria-label="Upload CSV" type="file" accept=".csv,text/csv" onchange={e=>csvFile=e.currentTarget.files?.[0]||null}/></label>{#if csvFile}<button type="button" class="text-btn" onclick={()=>csvFile=null}><X size={12}/> Use live Sheet instead</button>{/if}<button class="primary" disabled={busy||extracting||pendingScanIds.length>0}>{busy?'Reading…':csvFile?'Import CSV':'Preview Google Sheet import'}<ArrowRight size={16}/></button></form><div class="template-note"><strong>Template columns</strong><code>Date, Name, GROUP, Source, Example, REMARK</code><span>Date is the date added: YYMMDD or YYYY-MM-DD. The date of the set is stored at the start of Name. Optional ID supports stable identity when renaming rows. Blank/0 dates stay unknown. Every source column is preserved.</span><span>Without ID, identity uses group + raw date + name. Renaming this tuple creates a new entry. Import is additive; rows absent from the new Sheet remain.</span><a href="/template.csv" download>Download CSV template <ArrowUpRight size={13}/></a></div></section>
 <section class="import-card"><span class="section-number">02 / PREVIEW IMAGES</span><ImageIcon size={27} strokeWidth={1.3}/><h2>Put faces to sets.</h2><p>Read up to 5 public images from each example post and store them in SQLite. Missing or deleted posts keep their metadata.</p>{#if report}<div class="report"><div><strong>{report.created}</strong><span>NEW</span></div><div><strong>{report.updated}</strong><span>UPDATED</span></div><div><strong>{report.scanCandidates.length}</strong><span>NEW PREVIEWS</span></div></div>{/if}
 {#if pendingScanIds.length}<button class="primary" onclick={resumeImported} disabled={busy||extracting}><RefreshCw size={16}/>Continue imported previews ({pendingScanIds.length})</button>{/if}
 <button class="primary secondary" onclick={extractMissing} disabled={busy||extracting||missing===0||gankPaused}><RefreshCw size={16} class={extracting?'spin':''}/>{extracting?'Extracting previews…':`Extract ${report?.candidates.length??missing} incomplete sets`}</button>
 {#if extracting||progress.total}<div class="extraction-progress"><div><span>{extracting?current:'Last extraction'}</span><span>{progress.done} / {progress.total}</span></div><progress value={progress.done} max={progress.total||1}></progress><p>{progress.ready} ready · {progress.failed} unavailable</p>{#if extracting}<button class="text-btn" onclick={()=>stopRequested=true} disabled={stopRequested}>{stopRequested?'Stopping after current set…':'Stop after current set'}</button>{/if}</div>{/if}
 <div class="template-note"><strong>Gank request pacing</strong><span>At least {ingestion.intervalSeconds} seconds between requests, shared across Gank subdomains. There is no local daily request cap. Cached posts last 7 days. Confirmed Sheet imports target 5 images for new or changed examples. Keep this tab open while fetching. Bulk extraction targets 2 images per set; individual retries can fill up to 5. A 429 or 403 from the source stops the batch and saves a cooldown across restarts.</span><span>Only publicly accessible raster images are ingested. A failure stays visible and can be retried here or corrected with a manual image upload.</span></div></section></div>
 {#if logs.length}<section class="log-panel"><h3>Extraction results <span>Most recent first</span></h3>{#each logs as log}<div class="log-row"><span class="status-tag {log.state}">{log.state}</span><strong>set_id: {log.id} · {log.title}</strong><span>{log.detail||'Preview stored in SQLite'}</span></div>{/each}</section>{/if}
 {/if}
 <footer><Asterisk size={15}/><span>BIAS / COLLECTION WORKSPACE</span><span>Every moment has a place.</span></footer>
 </main>
{/if}

{#if editor}<div class="modal-backdrop" role="presentation" onclick={()=>{if(!busy)editor=false;}}></div><dialog use:modal onpaste={pasteImages} oncancel={e=>{e.preventDefault();if(!busy)editor=false;}} aria-label={editID?'Edit image set':'Add image set'}><div class="editor-heading"><div><span class="eyebrow">{editID?`ARCHIVE / ${String(editID).padStart(4,'0')}`:'A NEW MOMENT'}</span><h2>{editID?'Edit image set.':'Grow the collection.'}</h2></div><button class="icon-btn" aria-label="Close editor" disabled={busy} onclick={()=>editor=false}><X size={20}/></button></div><form onsubmit={save}>{#if editOrigin==='sheet'}<p class="form-help">{sheetSyncEnabled?'Metadata changes to this imported set will update its Google Sheet row.':'Google Sheet sync is off; metadata changes will be saved in the app only.'} Card appearance and images stay in the app.</p>{:else}<p class="form-help">{editID?"This manual set is saved in the app only.":sheetSyncEnabled?"Saving will add a row to Google Sheet in the selected group, ordered by set date.":"Google Sheet sync is off; this set will be saved in the app only."}</p>{/if}{#if editID}<label>set_id<input value={editID} readonly aria-label="set_id" aria-describedby="set-image-help"/><small id="set-image-help">{editImages.length}/5 images stored. A complete set needs at least 2.</small></label>{#if editImages.length}<div class="stored-images" aria-label="Stored images">{#each editImages as image}<div><img src={image.url} alt="Stored preview"/><button type="button" disabled={busy} onclick={()=>rotateStoredImage(image.id)} aria-label={`Rotate image ${image.id} clockwise`}>Rotate 90° ↻</button></div>{/each}</div>{/if}{/if}{#if editID}<label>Set title<input bind:value={form.title} required maxlength="500" placeholder="260915 Karina — Hamilton"/></label>{:else}<div class="form-row"><label>Set date<input type="date" bind:value={form.setDate} required/></label><label>Name<input bind:value={form.name} required maxlength="490" placeholder="Sana"/></label></div>{/if}<div class="form-row"><label>Group / artist<input bind:value={form.group} list="artist-names" required maxlength="150" placeholder="aespa"/><datalist id="artist-names">{#each facets.groups as f}<option value={f.name}></option>{/each}</datalist></label>{#if editID}<label>Date added (Sheet Date)<input type="date" bind:value={form.date}/></label>{/if}</div><label>Source<input bind:value={form.source} list="source-names" maxlength="300" placeholder="Photographer or collection source"/><datalist id="source-names">{#each facets.sources as f}<option value={f.name}></option>{/each}</datalist></label><label>Example post, service or image URL<input type="text" bind:value={form.example} maxlength="2000" placeholder="https://ganknow.com/services/..."/><small>Public HTTPS links only. Changing this clears previews linked to the old example.</small></label><label>Notes<textarea bind:value={form.notes} maxlength="10000" rows="3" placeholder="Anything worth keeping with this set" disabled={!!checkedRibbonNotes}></textarea>{#if checkedRibbonNotes}<small>Checked ribbons will replace Notes with: {checkedRibbonNotes}</small>{/if}</label><fieldset class="style-fields"><legend>Card appearance</legend><div class="card-color-control"><label for="card-color">Card color</label><input id="card-color" type="color" value={form.style.color||'#5fd2ef'} onchange={e=>form.style.color=e.currentTarget.value} disabled={busy}/><span>{form.style.color||'Default blue'}</span><button type="button" class="text-btn" disabled={busy||!form.style.color} onclick={()=>form.style.color=''}>Use default</button></div><label class="style-check"><input type="checkbox" bind:checked={form.style.wishlist} disabled={busy}/><span>Wish list ribbon <small>Green corner ribbon</small></span></label><label class="style-check"><input type="checkbox" bind:checked={form.style.new} disabled={busy}/><span>New ribbon <small>Shown only while this is among the 10 most recently added sets</small></span></label><label class="style-check"><input type="checkbox" bind:checked={form.style.noTrade} disabled={busy}/><span>No trade ribbon <small>Violet corner ribbon</small></span></label><label class="style-check"><input type="checkbox" bind:checked={form.style.selective} disabled={busy}/><span>Selective ribbon <small>Yellow corner ribbon</small></span></label></fieldset><label class="upload-label"><Upload size={18}/><span>{files.length?`${files.length} image${files.length>1?'s':''} selected`:'Add images (2–5 per set)'}</span><input type="file" accept="image/jpeg,image/png,image/gif,image/webp" multiple aria-label="Preview images" onchange={e=>files=Array.from(e.currentTarget.files||[])}/></label><p class="form-help">Each set needs 2–5 distinct images. Selected images are resized to 1200 pixels before upload. New uploads are added to existing images. Paste images with Ctrl+V while this editor is open. Without uploads, a public example link can supply images. Sets with fewer than 2 stay marked incomplete.</p>{#if error}<div class="alert error" role="alert">{error}</div>{/if}<div class="editor-footer"><button type="button" class="text-btn" disabled={busy} onclick={()=>editor=false}>Cancel</button><button class="primary" disabled={busy||extracting}>{#if busy}<LoaderCircle size={16} class="spin"/>{:else}<Check size={16}/>{/if}{busy?'Saving…':'Save image set'}</button></div></form></dialog>{/if}

{#if importPreview}
 <dialog class="import-review" use:modal oncancel={e=>{e.preventDefault();if(!busy)importPreview=null;}} aria-labelledby="import-review-title">
  <div class="editor-heading"><div><span class="eyebrow">REVIEW BEFORE SAVING</span><h2 id="import-review-title">Review import results.</h2></div><button class="icon-btn" aria-label="Close import preview" disabled={busy} onclick={()=>importPreview=null}><X size={20}/></button></div>
  <p class="review-note">No changes have been saved. Choose what happens to stored images when an existing row’s Example link changes.</p>
  <fieldset class="image-policy"><legend>Existing images on changed rows</legend><label><input type="radio" name="image-policy" checked={importPreview.imagePolicy==='keep'} disabled={busy} onchange={()=>reviewImagePolicy('keep')}/><span><strong>Keep existing images</strong><small>Update the row without deleting its stored images. Check that they still match the row. New rows and rows without images can still be fetched.</small></span></label><label><input type="radio" name="image-policy" checked={importPreview.imagePolicy==='replace'} disabled={busy} onchange={()=>reviewImagePolicy('replace')}/><span><strong>Replace from new Example</strong><small>Delete stored images for changed Example links, then fetch from a public HTTPS link. A failed fetch leaves that row without its old images.</small></span></label></fieldset>
  <div class="report review-counts"><div><strong>{importPreview.report.created}</strong><span>NEW SETS</span></div><div><strong>{importPreview.report.changed}</strong><span>CHANGED</span></div><div><strong>{importPreview.report.unchanged}</strong><span>UNCHANGED</span></div><div><strong>{importPreview.report.scanCandidates.length}</strong><span>FETCH IMAGES</span></div></div>
  <p class="review-source">{importPreview.sheetUrl}</p>
  {#if importPreview.report.changes.length}
   <div class="review-table"><table><thead><tr><th>CHANGE</th><th>IMAGE SET</th><th>ARTIST / DATE</th><th>DETAILS</th></tr></thead><tbody>{#each importPreview.report.changes as change}<tr><td><span class="status-tag {change.kind==='new'?'ready':'pending'}">{change.kind==='new'?'New':'Update'}</span></td><td>{change.after.title}{#if change.before}<small class="previous-value">set_id: {change.id}</small>{/if}{#if change.before&&change.before.title!==change.after.title}<small class="previous-value">Was: {change.before.title}</small>{/if}</td><td>{change.after.group}<small class="previous-value">{change.after.date||'Unknown date'}</small></td><td><span class="review-example">{change.after.example||'No example link'}</span>{#if change.before&&change.before.example!==change.after.example}<small class="preview-warning">{importPreview.imagePolicy==='replace'?(change.after.example.startsWith('https://')?'Example changed: stored images will be deleted and a fetch attempted from the new link.':'Example changed: stored images will be deleted; no public HTTPS link is available to fetch replacements.'):'Example changed: stored images will be kept.'}</small>{/if}{#if change.after.importWarning}<small class="preview-warning">{change.after.importWarning}</small>{/if}<details><summary>View all fields</summary>{#each ['title','group','date','source','example','notes'] as field}<div class="review-field"><strong>{field}</strong>{#if change.before&&change.before[field]!==change.after[field]}<span class="previous-value">Before: {change.before[field]||'—'}</span>{/if}<span>{change.after[field]||'—'}</span></div>{/each}</details></td></tr>{/each}</tbody></table></div>
  {:else}<p class="review-note">No new sets or metadata changes found. Existing images will be retained.</p>{/if}
  {#if error}<div class="alert error" role="alert">{error}</div>{/if}
  <div class="editor-footer"><button class="text-btn" disabled={busy} onclick={()=>importPreview=null}>Cancel</button><button class="primary" disabled={busy} onclick={confirmImport}>{#if busy}<LoaderCircle size={16} class="spin"/>Updating…{:else}<Check size={16}/>OK — Update{/if}</button></div>
 </dialog>
{/if}
