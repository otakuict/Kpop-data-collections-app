// Same whitespace normalization and YYMMDD/name split as finder's handleCluePaste.
export function splitFinderClue(value) {
 const normalized=value.trim().replace(/\s+/g,' ');
 const match=normalized.match(/^(\d{6})\s+(.+)$/);
 return match?{date:match[1],name:match[2]}:null;
}

export function finderSearchPreview(name,setDate) {
 return splitFinderClue(name)||{date:setDate.slice(2).replaceAll('-',''),name:name.trim().replace(/\s+/g,' ')};
}

export function finderSearchDate(date) {
 return date?`20${date.slice(0,2)}-${date.slice(2,4)}-${date.slice(4,6)}`:'';
}
