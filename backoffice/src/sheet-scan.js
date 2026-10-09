function importOptions(sheetUrl, csvFile) {
 if (!csvFile) return {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({sheetUrl})};
 const body=new FormData();
 body.append('sheetUrl',sheetUrl);
 body.append('csv',csvFile);
 return {method:'POST',body};
}

export function previewSheet(api, sheetUrl, csvFile) {
 return api('/admin/import/preview',importOptions(sheetUrl,csvFile));
}

export async function commitSheet(api, preview, options={}) {
 const csv=new File([preview.csv],'reviewed-sheet.csv',{type:'text/csv'});
 const report=await api('/admin/import',importOptions(preview.sheetUrl,csv));
 options.onImport?.(report);
 const batch=await extractSets(api,report.scanCandidates,{...options,target:5});
 return {report,batch};
}

export async function extractSets(api, ids, {target=2,shouldStop=()=>false,onProgress=()=>{},onCurrent=()=>{}}={}) {
 const batch={done:0,total:ids.length,ready:0,failed:0,results:[],remaining:[...ids],stopped:false,pause:null};
 onProgress({...batch});
 for (const id of ids) {
  if (shouldStop()) {batch.stopped=true;break;}
  onCurrent(id);
  try {
   const set=await api(`/admin/sets/${id}/extract?target=${target}`,{method:'POST'});
   const ready=set.images.length>=2;
   batch[ready?'ready':'failed']++;
   batch.results.push({id,title:set.title,state:set.imageState,detail:set.imageError,images:set.images.length});
  } catch(error) {
   batch.results.push({id,title:`Set #${id}`,state:'failed',detail:error.message});
   if (error.pausedUntil || shouldStop()) {batch.pause=error;batch.stopped=true;onProgress({...batch});break;}
   batch.failed++;
  }
  batch.done++;
  batch.remaining.shift();
  onProgress({...batch});
 }
 return batch;
}
