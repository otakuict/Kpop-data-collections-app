const { test, expect } = require('@playwright/test');
const office = process.env.E2E_BACKOFFICE_URL || 'http://localhost:5184';
const candidates = [
 { url:'https://ganknow.com/post/a', title:'261024 Sana 200p', excerpt:'Matching preview', source:'Post', status:'active', matchExplanation:'exact date and name' },
 { url:'https://ganknow.com/services/1-b', title:'261024 Sana other set', excerpt:'Another result', source:'Shop', status:'active', matchExplanation:'count differs' },
];

async function openEditor(page) {
 await page.route('**/api/**', async route => {
  const path = new URL(route.request().url()).pathname;
  const replies = {
   '/api/admin/session':{token:'test-session'},
   '/api/admin/status':{authenticated:true},
   '/api/sets':{items:[],total:0},
   '/api/facets':{total:0,ready:0,complete:0,groups:[],sources:[]},
   '/api/admin/ingestion':{},
  };
  if (Object.hasOwn(replies,path)) return route.fulfill({json:replies[path]});
  return route.fulfill({status:404,json:{error:'Unexpected API call'}});
 });
 await page.goto(office);
 await page.getByLabel('Administrator token').fill('test-admin-token');
 await page.getByRole('button',{name:'Open the workspace'}).click();
 await page.getByRole('button',{name:'Add image set',exact:true}).click();
 const editor=page.getByRole('dialog',{name:'Add image set',exact:true});
 await editor.getByLabel('Set date').fill('2026-10-24');
 await editor.getByLabel('Name',{exact:true}).fill('Sana');
 await editor.getByLabel('Group / artist').fill('TWICE');
 await editor.getByLabel('Source',{exact:true}).fill('studio');
 await expect(editor.getByLabel('Gank seller nickname')).toHaveValue('studio');
 return editor;
}

test('finder recommends but waits for selection and Save before creating a set',async({page},testInfo)=>{
 const editor=await openEditor(page);
 const saved=[];
 await page.route('**/api/admin/sets',async route=>{
  saved.push(route.request().postDataJSON());
  await route.fulfill({json:{id:1,title:'261024 Sana',date:'2026-10-24',origin:'manual',images:[{},{}]}});
 });
 await page.route('**/api/admin/example-candidates',async route=>{
  expect(route.request().postDataJSON()).toEqual({seller:'studio',setDate:'2026-10-24',name:'Sana',group:'TWICE',count:200});
  await route.fulfill({json:{candidates,recommendedUrl:candidates[0].url,confidence:0.85}});
 });
 await editor.getByLabel('File count (optional)').fill('200');
 await editor.getByRole('button',{name:'Find example links',exact:true}).click();
 await expect(editor.getByText('TypeSafe recommendation · confidence 85%')).toBeVisible();
 const example=editor.getByLabel('Example post, service or image URL');
 await expect(example).toHaveValue('');
 expect(saved).toEqual([]);
 // An operator can choose the alternative to the AI recommendation.
 await editor.getByRole('article').nth(1).getByRole('button',{name:'Use this link'}).click();
 await expect(example).toHaveValue(candidates[1].url);
 expect(saved).toEqual([]);
 await expect(editor.getByRole('button',{name:'Selected link',exact:true})).toHaveAttribute('aria-pressed','true');
 await editor.locator('.example-finder').screenshot({path:`test-results/finder-${testInfo.project.name}.png`});
 await editor.getByRole('button',{name:'Save image set',exact:true}).click();
 await expect(editor).toHaveCount(0);
 expect(saved).toHaveLength(1);
 expect(saved[0].example).toBe(candidates[1].url);
});

test('finder handles AI fallback, no results, and connection errors',async({page})=>{
 const editor=await openEditor(page);
 const button=editor.getByRole('button',{name:'Find example links',exact:true});
 let response={candidates,recommendedUrl:'',warning:'TypeSafe recommendation is unavailable. You can still choose any result manually.'};
 let status=200;
 await page.route('**/api/admin/example-candidates',route=>route.fulfill({status,json:response}));
 await button.click();
 await expect(editor.getByRole('status')).toContainText('TypeSafe recommendation is unavailable');
 await expect(editor.getByRole('button',{name:'Use this link'})).toHaveCount(2);
 response={candidates:[],recommendedUrl:''};
 await button.click();
 await expect(editor.getByText(/No matching links found/)).toBeVisible();
 response={error:'Finder search failed'};status=502;
 await button.click();
 await expect(editor.getByRole('alert')).toHaveText('Finder search failed');
 await expect(editor.getByLabel('Example post, service or image URL')).toBeEditable();
});

test('changing filters cancels a pending search and clears old suggestions',async({page})=>{
 const editor=await openEditor(page);
 let release;
 const pending=new Promise(resolve=>{release=resolve;});
 await page.route('**/api/admin/example-candidates',async route=>{
  await pending;
  await route.fulfill({json:{candidates,recommendedUrl:candidates[0].url,confidence:0.9}}).catch(()=>{});
 });
 await editor.getByRole('button',{name:'Find example links',exact:true}).click();
 await expect(editor.getByRole('button',{name:'Finding links and asking TypeSafe…'})).toBeDisabled();
 await editor.getByLabel('Name',{exact:true}).fill('Karina');
 release();
 await expect(editor.getByRole('button',{name:'Find example links',exact:true})).toBeEnabled();
 await expect(editor.getByRole('article')).toHaveCount(0);
 await expect(editor.getByLabel('Example post, service or image URL')).toHaveValue('');
});

test('editable search preview splits date and name like Finder without changing saved metadata',async({page},testInfo)=>{
 const editor=await openEditor(page);
 await editor.getByLabel('Name',{exact:true}).fill('261018 sullyoon');
 const date=editor.getByLabel('Search date (YYMMDD)',{exact:true});
 const name=editor.getByLabel('Search name preview',{exact:true});
 await expect(date).toHaveValue('261018');
 await expect(name).toHaveValue('sullyoon');
 const requests=[];
 await page.route('**/api/admin/example-candidates',async route=>{
  requests.push(route.request().postDataJSON());
  await route.fulfill({json:{candidates,recommendedUrl:''}});
 });
 const search=editor.getByRole('button',{name:'Find example links',exact:true});
 await search.click();
 await expect(editor.getByRole('article')).toHaveCount(2);
 expect(requests[0]).toMatchObject({setDate:'2026-10-18',name:'sullyoon'});
 await date.fill('261019');
 await name.fill('sullyoon nol festival');
 await expect(editor.getByRole('article')).toHaveCount(0);
 await search.click();
 await expect(editor.getByRole('article')).toHaveCount(2);
 expect(requests[1]).toMatchObject({setDate:'2026-10-19',name:'sullyoon nol festival'});
 await name.evaluate(input=>{
  const clipboard=new DataTransfer();clipboard.setData('text/plain','261018\n  sullyoon');
  input.dispatchEvent(new ClipboardEvent('paste',{clipboardData:clipboard,bubbles:true,cancelable:true}));
 });
 await expect(date).toHaveValue('261018');
 await expect(name).toHaveValue('sullyoon');
 await expect(editor.getByLabel('Set date')).toHaveValue('2026-10-24');
 await expect(editor.getByLabel('Name',{exact:true})).toHaveValue('261018 sullyoon');
 await search.click();
 await expect(editor.getByRole('article')).toHaveCount(2);
 await editor.locator('.example-finder').screenshot({path:`test-results/finder-preview-${testInfo.project.name}.png`});
 const saved=[];
 await page.route('**/api/admin/sets',async route=>{
  saved.push(route.request().postDataJSON());
  await route.fulfill({json:{id:1,title:'261018 sullyoon',date:'2026-10-24',origin:'manual',images:[{},{}]}});
 });
 await editor.getByRole('button',{name:'Save image set',exact:true}).click();
 await expect(editor).toHaveCount(0);
 expect(saved[0]).toMatchObject({setDate:'2026-10-24',name:'261018 sullyoon'});
});
