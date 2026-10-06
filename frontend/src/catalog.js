export function displayDate(date) {
 if (!date) return 'Date unknown';
 return new Intl.DateTimeFormat('en-GB', { day:'numeric', month:'short', year:'numeric', timeZone:'UTC' }).format(new Date(`${date}T00:00:00Z`)).replace("Sept", "Sep");
}
export function queryString(filters) {
 const params = new URLSearchParams();
 for (const [key,value] of Object.entries(filters)) if (value !== '' && value != null) params.set(key,String(value));
 return params.toString();
}
export function groupSets(items,by) {
 if (by === 'none') return [{ label:'',items }];
 const groups = new Map();
 for (const item of items) {
  const label = by === 'group' ? item.group : displayDate(item.date);
  if (!groups.has(label)) groups.set(label,[]);
  groups.get(label).push(item);
 }
 return [...groups].map(([label,items])=>({label,items}));
}
