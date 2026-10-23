<script>
 import { onMount } from 'svelte';
 import { Activity, Users, Clock3, Globe2, RefreshCw, ArrowUpRight, ChartNoAxesCombined, CircleAlert } from '@lucide/svelte';
 import UsageChart from './UsageChart.svelte';

 let { api } = $props();
 let days=$state('30'), report=$state(null), loading=$state(true), error=$state(''), updatedAt=$state('');
 let activeRequest, sequence=0;
 const number=new Intl.NumberFormat();
 const regions=new Intl.DisplayNames(['en'],{type:'region'});
 const shortDate=new Intl.DateTimeFormat('en',{month:'short',day:'numeric',timeZone:'UTC'});
 const countryName=code=>code?regions.of(code)||code:'Unknown';
 const dailyLabels=$derived(report?.daily.map(day=>shortDate.format(new Date(`${day.date}T00:00:00Z`)))||[]);
 const topCountries=$derived(report?.countries.slice(0,8)||[]);
 const knownCountries=$derived(report?.countries.filter(country=>country.country).length||0);
 function duration(seconds){const rounded=Math.round(seconds);return rounded>=3600?`${Math.floor(rounded/3600)}h ${String(Math.floor(rounded%3600/60)).padStart(2,'0')}m`:`${Math.floor(rounded/60)}m ${String(rounded%60).padStart(2,'0')}s`;}

 async function load(){
  activeRequest?.abort();
  const controller=new AbortController();activeRequest=controller;
  const request=++sequence;loading=true;error='';
  try{
   const result=await api(`/admin/analytics?days=${days}`,{signal:controller.signal});
   if(request!==sequence)return;
   report=result;updatedAt=new Date().toLocaleTimeString([],{hour:'2-digit',minute:'2-digit',second:'2-digit'});
  }catch(e){if(e.name!=='AbortError'&&request===sequence)error=e.message;}
  finally{if(request===sequence)loading=false;}
 }
 function changeRange(){report=null;load();}
 onMount(()=>{
  load();
  const refresh=setInterval(()=>{if(!document.hidden&&!loading)load();},20000);
  return ()=>{sequence++;activeRequest?.abort();clearInterval(refresh);};
 });
</script>

<section class="usage-dashboard" aria-label="Gallery usage">
 <div class="page-heading usage-heading">
  <div><div class="eyebrow">A CLOSER LOOK AT YOUR AUDIENCE</div><h1>Usage dashboard<span>.</span></h1><p>See who visits the gallery and how long they spend exploring.</p></div>
  <div class="usage-controls"><label>Date range<select aria-label="Date range" bind:value={days} onchange={changeRange}><option value="7">Last 7 days</option><option value="30">Last 30 days</option><option value="90">Last 90 days</option></select></label><button class="icon-btn" aria-label="Refresh usage" onclick={load} disabled={loading}><RefreshCw size={17} class={loading?'spin':''}/></button></div>
 </div>

 {#if error}<div class="alert error" role="alert"><CircleAlert size={17}/><span>{error}{report?' Showing the last successful update.':''}</span><button class="text-btn" onclick={load} disabled={loading}>Retry</button></div>{/if}
 {#if !report}
  {#if loading}<div class="usage-loading" role="status"><RefreshCw size={20} class="spin"/>Loading gallery usage…</div>{/if}
 {:else}
  <div class="usage-meta"><span><span class="usage-live-dot"></span>Gallery activity</span><span>{report.from} — {report.to} · UTC</span><span>{loading?'Refreshing…':`Updated ${updatedAt}`} · Auto-refresh 20s</span></div>
  <div class="usage-metrics">
   <article class="usage-metric"><div><span>ONLINE NOW</span><Activity size={18}/></div><strong data-testid="usage-online">{number.format(report.online)}<span class="usage-live-dot"></span></strong><p>Seen within the last 60 seconds</p></article>
   <article class="usage-metric"><div><span>PEOPLE</span><Users size={18}/></div><strong data-testid="usage-visitors">{number.format(report.visitors)}</strong><p>Unique browsers in this period</p></article>
   <article class="usage-metric"><div><span>VISITS</span><ArrowUpRight size={18}/></div><strong data-testid="usage-visits">{number.format(report.visits)}</strong><p>Includes return visits after inactivity</p></article>
   <article class="usage-metric"><div><span>AVG. TIME PER VISIT</span><Clock3 size={18}/></div><strong class="usage-duration" data-testid="usage-average">{duration(report.averageSeconds)}</strong><p>Approximate time observed</p></article>
  </div>

  {#if !report.visits}<div class="usage-empty"><ChartNoAxesCombined size={28}/><div><h2>No visits recorded yet.</h2><p>Open the gallery to start collecting activity. Historical data begins with this dashboard.</p></div></div>{/if}

  <div class="usage-trends">
   <article class="usage-panel"><div class="usage-panel-heading"><div><h2><Users size={17}/>Visitors over time</h2><p>Unique browsers each day</p></div><span class="usage-legend"><i></i>People</span></div><UsageChart labels={dailyLabels} values={report.daily.map(day=>day.visitors)} label="Daily visitors"/></article>
   <article class="usage-panel"><div class="usage-panel-heading"><div><h2><Clock3 size={17}/>Time spent</h2><p>Average observed time per visit, by day</p></div><span class="usage-legend violet"><i></i>Seconds</span></div><UsageChart labels={dailyLabels} values={report.daily.map(day=>day.averageSeconds)} label="Average visit duration" color="#b0a0f5" seconds/></article>
  </div>

  <article class="usage-panel usage-countries"><div class="usage-panel-heading"><div><h2><Globe2 size={17}/>Where visitors come from</h2><p>Approximate country, based on the visitor’s network</p></div><span class="usage-country-count">{knownCountries} {knownCountries===1?'country':'countries'}</span></div>
   {#if report.countries.length}
    <div class="usage-country-grid"><UsageChart type="bar" labels={topCountries.map(country=>countryName(country.country))} values={topCountries.map(country=>country.visitors)} label="Visitors by country" color="#79d7bb"/>
     <div class="usage-country-table"><table><caption class="sr-only">Visitors and visits by country</caption><thead><tr><th scope="col">Country</th><th scope="col">People</th><th scope="col">Visits</th><th scope="col">Avg. time</th></tr></thead><tbody>{#each report.countries as country}<tr><td><span class="usage-country-code" aria-hidden="true">{country.country||'—'}</span>{countryName(country.country)}</td><td>{number.format(country.visitors)}</td><td>{number.format(country.visits)}</td><td>{duration(country.averageSeconds)}</td></tr>{/each}</tbody></table></div>
    </div>
   {:else}<p class="usage-no-country">No country data for this period.</p>{/if}
   {#if !report.countries.length||report.countries.some(country=>!country.country)}<p class="usage-country-note">Unknown means location information was unavailable. Country reporting needs a trusted geolocation proxy.</p>{/if}
  </article>

  <details class="usage-panel usage-daily"><summary>View daily numbers <span>UTC</span></summary><div class="usage-country-table"><table><caption class="sr-only">Daily gallery usage</caption><thead><tr><th scope="col">Date</th><th scope="col">People</th><th scope="col">Visits</th><th scope="col">Avg. time</th><th scope="col">Total time</th></tr></thead><tbody>{#each report.daily as day}<tr><td>{day.date}</td><td>{number.format(day.visitors)}</td><td>{number.format(day.visits)}</td><td>{duration(day.averageSeconds)}</td><td>{duration(day.totalSeconds)}</td></tr>{/each}</tbody></table></div></details>
  <p class="usage-footnote"><Clock3 size={14}/><span>Time is estimated from the gallery’s 20-second heartbeat and stops at the last heartbeat. A 60-second gap starts a new visit. People are browser cookies; clearing cookies or changing devices can count a person again.</span></p>
 {/if}
</section>

<style>
 .usage-dashboard{min-width:0}
.usage-heading{margin-bottom:24px}
.usage-controls{display:flex;align-items:flex-end;gap:10px;flex-shrink:0}
.usage-controls label{margin:0;font-size:10px;color:var(--muted)}
.usage-controls select{display:block;margin-top:7px;min-height:38px;padding:8px 30px 8px 12px;border:1px solid #314d6b;border-radius:6px;background:#14263b;color:#edf4fb;font-size:12px}
.usage-controls .icon-btn{width:38px;height:38px}
.usage-meta{display:flex;flex-wrap:wrap;gap:10px 20px;align-items:center;font-size:10px;color:var(--muted);margin-bottom:18px}
.usage-meta>span:first-child{display:flex;align-items:center;gap:7px;color:#a6e7ce}
.usage-meta>span:last-child{margin-left:auto}
.usage-live-dot{display:inline-block;width:7px;height:7px;border-radius:50%;background:#79d7bb;box-shadow:0 0 0 4px #79d7bb12;flex-shrink:0}
.usage-metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:16px;margin-bottom:24px}
.usage-metric{background:linear-gradient(135deg,#14283a,#111e2f);border:1px solid var(--line);border-radius:10px;padding:22px;min-width:0}
.usage-metric>div{display:flex;justify-content:space-between;gap:8px;color:#88a5c0}
.usage-metric>div>span{font-size:9px;letter-spacing:1px;line-height:1.7}
.usage-metric>div :global(svg){color:#5fd2ef}
.usage-metric strong{display:flex;gap:14px;align-items:center;font-family:Manrope,sans-serif;font-size:38px;font-weight:550;letter-spacing:-1.6px;font-variant-numeric:tabular-nums;margin:16px 0 9px;white-space:nowrap}
.usage-metric strong.usage-duration{font-size:30px;letter-spacing:-1px;line-height:1.8}
.usage-metric p{font-size:10px;line-height:1.6;color:var(--muted);margin:0}
.usage-trends{display:grid;grid-template-columns:1.3fr 1fr;gap:20px;margin-bottom:20px}
.usage-panel{padding:24px;background:#111e2f;border:1px solid var(--line);border-radius:10px;min-width:0}
.usage-panel-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:26px}
.usage-panel h2{display:flex;align-items:center;gap:9px;font-size:14px;font-weight:550;margin:0 0 8px;letter-spacing:-.2px}
.usage-panel h2 :global(svg){color:#91b1cd}
.usage-panel-heading p{font-size:11px;color:var(--muted);line-height:1.6;margin:0}
.usage-legend{display:flex;align-items:center;gap:7px;color:var(--muted);font-size:10px;white-space:nowrap;padding-top:4px}
.usage-legend i{width:7px;height:7px;border-radius:50%;background:#5fd2ef}
.usage-legend.violet i{background:#b0a0f5}
.usage-country-count{font-size:10px;color:#a4e0cc;background:#1b3a3a;padding:6px 9px;border-radius:5px;white-space:nowrap}
.usage-country-grid{display:grid;grid-template-columns:1fr 1fr;gap:35px;align-items:center}
.usage-country-table{overflow:auto;max-height:320px;scrollbar-color:#436584 #111e2f}
.usage-country-table table{min-width:350px}
.usage-country-table th{padding:12px 10px;letter-spacing:.8px;font-size:9px;white-space:nowrap}
.usage-country-table th:first-child{width:auto}
.usage-country-table td{padding:14px 10px;font-size:11px;white-space:nowrap;font-variant-numeric:tabular-nums}
.usage-country-table td:first-child{color:#d5e5f4}
.usage-country-code{display:inline-flex;width:30px;height:24px;align-items:center;justify-content:center;margin-right:10px;font-size:9px;color:#9fbcd5;background:#1b3047;border:1px solid #2b455f;border-radius:4px}
.usage-country-note,.usage-footnote{font-size:11px;line-height:1.8;color:var(--muted)}
.usage-country-note{border-top:1px solid var(--line);padding-top:16px;margin:20px 0 0}
.usage-no-country{padding:25px 0;color:var(--muted);font-size:12px}
.usage-daily{margin-top:20px;padding:0;overflow:hidden}
.usage-daily summary{cursor:pointer;padding:19px 24px;font-size:12px;color:#c6d8e8}
.usage-daily summary>span{float:right;font-size:10px;color:var(--muted)}
.usage-daily .usage-country-table{max-height:350px}
.usage-daily td:first-child,.usage-daily th:first-child{padding-left:24px}
.usage-footnote{display:flex;align-items:flex-start;gap:9px;margin:22px 0 0;max-width:920px}
.usage-footnote :global(svg){margin-top:3px}
.usage-empty{display:flex;align-items:center;gap:17px;background:#122d3a;border:1px solid #2a5265;border-radius:9px;padding:21px 24px;margin-bottom:24px;color:#8dd9ec}
.usage-empty h2{font-size:14px;margin:0 0 6px;color:#deedf8}
.usage-empty p{font-size:11px;line-height:1.7;margin:0;color:#a6bfd3}
.usage-loading{display:flex;gap:12px;align-items:center;justify-content:center;min-height:280px;color:var(--muted);font-size:13px}

 @media(max-width:1100px){.usage-metrics{gap:12px}
.usage-metric{padding:18px}
.usage-metric strong{font-size:32px}
.usage-metric strong.usage-duration{font-size:26px}
.usage-trends{grid-template-columns:1fr 1fr}
.usage-country-grid{gap:18px}
}

 @media(max-width:850px){.usage-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}
.usage-country-grid{grid-template-columns:1fr}
.usage-country-table{max-height:280px}
}

 @media(max-width:650px){.usage-heading{flex-direction:column;gap:18px}
.usage-heading h1{font-size:30px}
.usage-heading p{font-size:11px;line-height:1.7}
.usage-controls{width:100%}
.usage-controls label{flex:1}
.usage-controls select{width:100%;font-size:16px}
.usage-meta{font-size:9px;gap:9px 15px}
.usage-meta>span:last-child{margin-left:0;width:100%}
.usage-metrics{gap:10px}
.usage-metric{padding:16px}
.usage-metric>div>span{font-size:8px;letter-spacing:.6px}
.usage-metric strong{font-size:32px}
.usage-metric strong.usage-duration{font-size:25px;line-height:1.8}
.usage-metric p{font-size:10px;min-height:32px}
.usage-trends{grid-template-columns:1fr;gap:16px;margin-bottom:16px}
.usage-panel{padding:20px 16px}
.usage-panel-heading{gap:10px}
.usage-panel h2{font-size:13px}
.usage-panel-heading p{font-size:10px}
.usage-country-count{font-size:9px}
.usage-country-grid{gap:24px}
.usage-country-table td{padding:12px 8px;font-size:10px}
.usage-daily{padding:0}
.usage-daily summary{padding:18px 16px}
.usage-empty{padding:18px 16px;align-items:flex-start}
.usage-footnote,.usage-country-note{font-size:10px}
}

</style>
