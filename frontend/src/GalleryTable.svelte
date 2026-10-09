<script>
 import { ArrowUpRight, Image as ImageIcon } from '@lucide/svelte';
 import { displayDate } from './catalog.js';
 import { sourceHue } from './source-colors.js';
 let { items, onopen } = $props();
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex (A focusable scroll region lets keyboard users reach columns outside the viewport.) -->
<div class="sheet-scroll" tabindex="0" role="region" aria-label="Image sets table — scroll horizontally for more columns">
 <table class="gallery-table">
  <caption class="visually-hidden">Image sets — select a title to open its images and details</caption>
  <thead>
   <tr class="sheet-letters" aria-hidden="true">{#each ['A','B','C','D','E','F','G','H'] as letter}<td>{letter}</td>{/each}</tr>
   <tr><th scope="col">SET_ID</th><th scope="col">Date</th><th scope="col">Image set</th><th scope="col">Artist</th><th scope="col">Shop / source</th><th scope="col">Images</th><th scope="col">Example</th><th scope="col">Notes</th></tr>
  </thead>
  <tbody>{#each items as set (set.id)}
   <tr>
    <th scope="row">{set.id}</th>
    <td class="sheet-date">{displayDate(set.date)}</td>
    <td><button class="sheet-title" onclick={()=>onopen(set)} aria-label={`View ${set.title}`}>
     {#if set.images.length}<img src={set.images[0].url} alt="" loading="lazy"/>{:else}<span class="sheet-placeholder"><ImageIcon size={19}/></span>{/if}
     <span>{set.title}</span><ArrowUpRight size={14}/>
    </button></td>
    <td>{set.group || '—'}</td>
    <td><span class="source-badge" class:unlisted={!set.source.trim()} style={`--shop-hue: ${sourceHue(set.source)??0}`}>{set.source || 'Source unlisted'}</span></td>
    <td class="sheet-count">{set.images.length}</td>
    <td>{#if set.example.startsWith('https://')}<a class="sheet-example" href={set.example} target="_blank" rel="noreferrer" aria-label={`Example for ${set.title}`}>Open post <ArrowUpRight size={13}/></a>{:else}<span class="muted">{set.example || '—'}</span>{/if}</td>
    <td class="sheet-notes">{set.notes || '—'}</td>
   </tr>
  {/each}</tbody>
 </table>
</div>
