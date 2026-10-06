<script>
 import { onMount } from 'svelte';
 import CardCarousel from './CardCarousel.svelte';
 import { Asterisk, ArrowUpRight, ArrowRight, Search, X, SlidersHorizontal, ChevronLeft, ChevronRight, Image as ImageIcon, LayoutGrid, CalendarDays, CircleHelp } from '@lucide/svelte';
 import { displayDate, groupSets, queryString } from './catalog.js';
 const initial = new URLSearchParams(window.location.search);
 let q=$state(initial.get('q')||''), group=$state(initial.get('group')||''), from=$state(initial.get('from')||''), to=$state(initial.get('to')||''), sort=$state(initial.get('sort')||'newest');
 let by=$state('none'), page=$state(1), items=$state([]), total=$state(0), facets=$state({groups:[],total:0,ready:0}), featured=$state([]);
 let loading=$state(true), error=$state(''), mounted=$state(false), selected=$state(null), imageIndex=$state(0), about=$state(false), filtersOpen=$state(false);
 let activeRequest, timer;
 const sections=$derived(groupSets(items,by));
 const pages=$derived(Math.max(1,Math.ceil(total/24)));
 const filtered=$derived(Boolean(q||group||from||to));
 const backoffice=import.meta.env.VITE_BACKOFFICE_URL || 'http://localhost:5174';
 async function json(url,signal) { const response=await fetch(url,{signal});const result=await response.json();if(!response.ok)throw new Error(result.error||'Unable to load the archive');return result; }
 async function load(filters) {
  activeRequest?.abort();const controller=new AbortController();activeRequest=controller;loading=true;error='';
  const query=queryString(filters);window.history.replaceState(null,'',query?`?${query}`:window.location.pathname);
  try { const result=await json(`/api/sets?${query}`,controller.signal);items=result.items;total=result.total; }
  catch(e) {if(e.name!=='AbortError')error=e.message;}
  finally {if(activeRequest===controller)loading=false;}
 }
 $effect(()=>{const filters={q,group,from,to,sort,page,limit:24};if(mounted){clearTimeout(timer);timer=setTimeout(()=>load(filters),200);}return()=>clearTimeout(timer);});
 onMount(()=>{mounted=true;Promise.all([json('/api/facets'),json('/api/sets?limit=200')]).then(([f,sets])=>{facets=f;featured=sets.items.filter(x=>x.images.length).slice(0,3);}).catch(e=>error=e.message);return()=>activeRequest?.abort();});
 function reset(){q='';group='';from='';to='';page=1;sort='newest';}
 function chooseGroup(name){group=name;page=1;}
 function open(set,index=0){selected=set;imageIndex=index;}
 function close(){selected=null;about=false;}
 function keydown(e){if(e.key==='Escape')close();if(selected?.images.length>1&&e.key==='ArrowRight')imageIndex=(imageIndex+1)%selected.images.length;if(selected?.images.length>1&&e.key==='ArrowLeft')imageIndex=(imageIndex-1+selected.images.length)%selected.images.length;}
 function modal(node){node.showModal();return {destroy(){node.close();}};}
</script>

<svelte:window onkeydown={keydown}/>
<svelte:head><title>BIAS — Your bias. In focus.</title></svelte:head>

<header class="topbar">
 <a class="brand" href="/" aria-label="BIAS home"><span class="brand-mark"><Asterisk size={27} strokeWidth={2.6}/></span>BIAS<span class="brand-dot">®</span></a>
 <nav aria-label="Main navigation"><button class:active={by==='none'} onclick={()=>{by='none';reset();}}>Discover</button><button class:active={by==='group'} onclick={()=>by='group'}>Collections</button><button onclick={()=>about=true}>About the archive</button></nav>
 <a class="office-link" href={backoffice} target="_blank" rel="noreferrer">Back office <ArrowUpRight size={15}/></a>
</header>

<main>
 <section class="hero" aria-label="Archive introduction">
  <div class="hero-copy"><div class="eyebrow"><span class="tiny-dot"></span> THE K-POP IMAGE ARCHIVE <span class="edition">VOL. 001</span></div>
   <h1>OTAKU ICETEA.<br/><em>Collection.</em><span class="headline-star">✳</span></h1>
   <p>Every stage. Every era. A little closer.<br/>Explore a growing collection of K-pop image sets.</p>
   <a href="#collection" class="explore-link">Explore the archive <ArrowRight size={18}/></a>
   <div class="hero-stats"><div><strong>{facets.total || '—'}</strong><span>IMAGE SETS</span></div><div><strong>{facets.groups.length || '—'}</strong><span>GROUPS & ARTISTS</span></div><span class="stats-note">Collected with care.<br/>Made to be discovered.</span></div>
  </div>
  <div class="hero-art" aria-label="Featured images from the archive">
   <div class="orbit"></div><span class="art-label">A MOMENT, ARCHIVED.</span>
   {#each featured.slice(0,2) as set,i}<button class="polaroid polaroid-{i}" onclick={()=>open(set)} aria-label={`View ${set.title}`}><img src={set.images[0].url} alt={set.title}/><span>{set.group} <ArrowUpRight size={14}/></span></button>{/each}
   {#if featured.length===0}<div class="art-placeholder"><Asterisk size={100}/><span>YOUR NEXT FAVORITE MOMENT.</span></div>{/if}
   <div class="art-stamp">FOR THE<br/><strong>FANS.</strong><Asterisk size={19}/></div><div class="art-caption">FROM THE STAGE TO YOUR COLLECTION <span>↗</span></div>
  </div>
 </section>

 <section class="archive" id="collection">
  <aside class:mobile-open={filtersOpen}>
   <div class="aside-heading"><span>REFINE YOUR VIEW</span><SlidersHorizontal size={14}/></div>
   <label class="search-box"><Search size={17}/><input aria-label="Search image sets" placeholder="Search the archive..." bind:value={q} oninput={()=>page=1}/>{#if q}<button aria-label="Clear search" onclick={()=>{q='';page=1;}}><X size={13}/></button>{/if}</label>
   <div class="filter-label">GROUPS & ARTISTS</div>
   <div class="group-list"><button class:chosen={!group} onclick={()=>chooseGroup('')}><span>All artists</span><span class="count">{facets.total}</span></button>{#each facets.groups as f}<button class:chosen={group===f.name} onclick={()=>chooseGroup(f.name)}><span>{f.name}</span><span class="count">{f.count}</span></button>{/each}</div>
   <div class="date-filter"><div class="filter-label"><CalendarDays size={13}/> DATE OF SET</div><label>From<input aria-label="From date" type="date" bind:value={from} oninput={()=>page=1}/></label><label>To<input aria-label="To date" type="date" bind:value={to} oninput={()=>page=1}/></label></div>
   {#if filtered}<button class="reset" onclick={reset}><X size={13}/> Clear all filters</button>{/if}
   <div class="aside-note"><Asterisk size={22}/><p>A collection of moments.<br/>Find the ones that stay.</p></div>
  </aside>
  <div class="collection-content">
   <div class="collection-heading"><div><div class="eyebrow small">BROWSE THE ARCHIVE</div><h2>{group || 'The collection'}<span class="title-dot">.</span></h2><p>{loading?'Finding your next favorite…':`${total} image sets to discover`}</p></div><button class="mobile-filter" onclick={()=>filtersOpen=!filtersOpen}><SlidersHorizontal size={16}/> Filters</button><span class="collection-icon"><LayoutGrid size={23}/></span></div>
   <div class="collection-toolbar"><div class="quick-groups">{#each ['', 'LE SSERAFIM', 'aespa', 'IVE', 'TWICE'] as name}<button class:selected={group===name} onclick={()=>chooseGroup(name)}>{name||'All sets'}</button>{/each}</div><div class="view-controls"><label>Group by<select aria-label="Group by" bind:value={by}><option value="none">None</option><option value="group">Artist</option><option value="date">Date</option></select></label><label>Sort<select aria-label="Sort sets" bind:value={sort} onchange={()=>page=1}><option value="newest">Newest first</option><option value="oldest">Oldest first</option><option value="name">Name A–Z</option></select></label></div></div>
   {#if error}<div class="status error" role="alert"><CircleHelp size={25}/><h3>Couldn't load the archive</h3><p>{error}</p><button onclick={()=>load({q,group,from,to,sort,page,limit:24})}>Try again</button></div>
   {:else if loading}<div class="gallery-grid" aria-label="Loading image sets">{#each Array(6) as _}<div class="skeleton-card"><div></div><span></span><span></span></div>{/each}</div>
   {:else if !items.length}<div class="status"><Search size={30}/><h3>No moments found</h3><p>Try another artist, name, or date range.</p><button onclick={reset}>Reset filters <ArrowRight size={16}/></button></div>
   {:else}{#each sections as section}{#if section.label}<h3 class="section-label">{section.label}<span>{section.items.length} SETS ON THIS PAGE</span></h3>{/if}<div class="gallery-grid">{#each section.items as set,i}<article class="gallery-card"><CardCarousel {set} number={String((page-1)*24+i+1).padStart(3,'0')} onopen={open}/><div class="card-meta"><div class="card-date">{displayDate(set.date)}<span class="group-tag">{set.group}</span></div><button class="card-title" onclick={()=>open(set)}>{set.title}</button><div class="card-source">{set.source || 'Source unlisted'} <span>IMAGE SET ↗</span></div></div></article>{/each}</div>{/each}
   <div class="pagination"><span>Showing {(page-1)*24+1}–{Math.min(page*24,total)} of {total} sets</span><div><button aria-label="Previous page" disabled={page===1} onclick={()=>{page--;document.getElementById('collection').scrollIntoView({behavior:'smooth'});}}><ChevronLeft size={17}/></button><span>{page} <span class="muted">/ {pages}</span></span><button aria-label="Next page" disabled={page===pages} onclick={()=>{page++;document.getElementById('collection').scrollIntoView({behavior:'smooth'});}}><ChevronRight size={17}/></button></div></div>
   {/if}
  </div>
 </section>
 <footer><a class="brand" href="/">BIAS<span class="brand-dot">®</span></a><p>A little archive. A lot of love.</p><span>K-POP IMAGE ARCHIVE <Asterisk size={15}/></span></footer>
</main>

{#if selected||about}<div class="modal-backdrop" role="presentation" onclick={close}></div><dialog use:modal oncancel={close} class:about={about} aria-label={about?'About the archive':selected.title}><button class="modal-close" aria-label="Close dialog" onclick={close}><X size={20}/></button>
 {#if about}<div class="about-content"><span class="brand-mark"><Asterisk size={25}/></span><h2>Moments worth<br/><em>keeping.</em></h2><p>BIAS brings your K-pop collection together. Browse image sets by artist and date, and open each set for its source and preview.</p><p>The starting collection comes from the supplied Google Sheet. Previews are extracted from public example posts and stored in the archive. Some posts are missing or deleted; their records remain available.</p><a href="https://docs.google.com/spreadsheets/d/1z45mKRtLvTZth8b-uhqkMyjzxVtHI8b7YVcySQVeEkg/edit?gid=0" target="_blank" rel="noreferrer">View the source sheet <ArrowUpRight size={16}/></a></div>
 {:else}<div class="detail-image">{#if selected.images.length}<img src={selected.images[imageIndex].url} alt={selected.title}/>{#if selected.images.length>1}<div class="detail-pagination"><button aria-label="Previous image" onclick={()=>imageIndex=(imageIndex-1+selected.images.length)%selected.images.length}><ChevronLeft size={18}/></button><span>{imageIndex+1} / {selected.images.length}</span><button aria-label="Next image" onclick={()=>imageIndex=(imageIndex+1)%selected.images.length}><ChevronRight size={18}/></button></div>{/if}{:else}<div class="no-image"><ImageIcon size={40} strokeWidth={1}/><span>PREVIEW UNAVAILABLE</span><small>{selected.imageState==='missing'?'No example link in the source sheet.':'The original preview could not be retrieved.'}</small></div>{/if}</div><div class="detail-copy"><div class="eyebrow">{selected.group}</div><h2>{selected.title}</h2><dl><div><dt>Date of set</dt><dd>{displayDate(selected.date)}</dd></div><div><dt>Source</dt><dd>{selected.source||'Unlisted'}</dd></div><div><dt>Preview images</dt><dd>{selected.images.length}</dd></div></dl>{#if selected.notes}<p class="notes">{selected.notes}</p>{/if}{#if selected.importWarning}<p class="detail-warning">{selected.importWarning}</p>{/if}{#if selected.example.startsWith('https://')}<a class="source-link" href={selected.example} target="_blank" rel="noreferrer">Open example post <ArrowUpRight size={16}/></a>{:else if selected.example}<p class="notes">Source note: {selected.example}</p>{/if}<span class="detail-id">ARCHIVE / {String(selected.id).padStart(4,'0')}</span></div>{/if}
</dialog>{/if}
