const {test,expect}=require('@playwright/test');
const office=process.env.E2E_BACKOFFICE_URL||'http://localhost:5184';
const groups=[
 {name:'TripleS',kind:'group',members:[{id:1,name:'Jiyeon',group:'TripleS',aliases:['지연'],source:'https://www.triples-official.jp/'}]},
 {name:'MEOVV',kind:'group',members:[{id:2,name:'Anna',group:'MEOVV',aliases:['안나'],source:'https://example.com/profile'}]},
];
const rows=[
 {fileId:11,original:'240404 지연2',path:'/Albums/240404 지연2',createdAt:'2026-10-24T01:00:00Z',setDate:'2024-04-04',rawName:'지연2',romanized:'Jiyeon2',title:'240404 Jiyeon2',group:'TripleS',memberId:1,method:'dictionary',status:'new',warning:''},
 {fileId:12,original:'261013 안나',path:'/Albums/261013 안나',createdAt:'2026-10-24T01:00:00Z',setDate:'2026-10-13',rawName:'안나',romanized:'Anna',title:'261013 Anna',group:'MEOVV',memberId:2,method:'dictionary',status:'duplicate',warning:''},
 {fileId:13,original:'261013 모르는이름',path:'/Albums/261013 모르는이름',createdAt:'2026-10-24T01:00:00Z',setDate:'2026-10-13',rawName:'모르는이름',romanized:'Moreuneunireum',title:'261013 Moreuneunireum',group:'',memberId:0,method:'koroman',status:'review',warning:'Choose an artist manually'},
];

async function setup(page,{sheet=true}={}){
 const calls=[];
 await page.route('**/api/**',async route=>{
  const req=route.request(),path=new URL(req.url()).pathname;
  const replies={
   '/api/admin/session':{token:'test-session'},'/api/admin/status':{authenticated:true},
   '/api/sets':{items:[],total:0},'/api/facets':{total:0,ready:0,complete:0,groups:[],sources:[{name:'studio',count:1},{name:'loveshakedata',count:2}]},'/api/admin/ingestion':{},
   '/api/admin/artists':{groups},
   '/api/admin/terabox/accounts':{accounts:[{id:1,label:'First drive',root:'/Albums',source:'studio'},{id:2,label:'Second drive',root:'/Albums',source:'studio2'}],jevConfigured:true,sheetSyncEnabled:sheet},
  };
  if(path==='/api/admin/terabox/preview'){
   calls.push({type:'preview',body:req.postDataJSON()});
   return route.fulfill({json:{id:'preview-token',accountId:req.postDataJSON().accountId,rows,complete:true,scanned:20,warnings:[]}});
  }
  if(path==='/api/admin/terabox/commit'){
   calls.push({type:'commit',body:req.postDataJSON()});
   return route.fulfill({json:{results:req.postDataJSON().rows.map(row=>({fileId:row.fileId,setId:100,status:'saved'}))}});
  }
  if(path==='/api/admin/sets'&&req.method()==='POST'){calls.push({type:'unexpected-save'});return route.fulfill({status:500,json:{error:'Preview must not save'}});}
  if(Object.hasOwn(replies,path))return route.fulfill({json:replies[path]});
  return route.fulfill({status:404,json:{error:'Unexpected API request'}});
 });
 await page.goto(office);
 await page.getByLabel('Administrator token').fill('test-admin-token');
 await page.getByRole('button',{name:'Open the workspace'}).click();
 await page.getByRole('button',{name:'Update from TeraBox',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'Update from TeraBox'});
 await expect(dialog.getByRole('button',{name:'Scan last 3 days'})).toBeEnabled();
 return {dialog,calls};
}

test('private drive import switches accounts and waits for edited preview confirmation',async({page},info)=>{
 const {dialog,calls}=await setup(page);
 await dialog.getByLabel('TeraBox account',{exact:true}).selectOption('2');
 await dialog.getByRole('button',{name:'Scan last 3 days'}).click();
 await expect(dialog.getByText('KOROMAN: Jiyeon2')).toBeVisible();
 expect(calls).toEqual([{type:'preview',body:{accountId:2}}]);
 await expect(dialog.getByLabel('Import 240404 지연2',{exact:true})).toBeChecked();
 await expect(dialog.getByLabel('Import 261013 안나',{exact:true})).toBeDisabled();
 await expect(dialog.getByLabel('Import 261013 모르는이름',{exact:true})).not.toBeChecked();
 await dialog.getByLabel('Final title',{exact:true}).first().fill('240404 Jiyeon 2 premium');
 await dialog.screenshot({path:`test-results/terabox-preview-${info.project.name}.png`});
 await dialog.getByRole('button',{name:'Confirm to Sheet & DB'}).click();
 await expect(dialog.getByRole('status')).toContainText('Saved 1 sets');
 expect(calls).toHaveLength(2);
 expect(calls[1]).toEqual({type:'commit',body:{previewId:'preview-token',rows:[{fileId:11,title:'240404 Jiyeon 2 premium',group:'TripleS',setDate:'2024-04-04',example:'',notes:''}]}});
 await expect(dialog.getByRole('button',{name:'Confirm to Sheet & DB'})).toBeDisabled();
});

test('preview allows manual artist choice and cannot confirm without Sheet sync',async({page})=>{
 const {dialog,calls}=await setup(page,{sheet:false});
 await dialog.getByRole('button',{name:'Scan last 3 days'}).click();
 const unresolved=dialog.locator('.tera-row').filter({has:page.getByLabel('Import 261013 모르는이름',{exact:true})});
 await unresolved.getByLabel('Artist suggestion').selectOption('2');
 await expect(unresolved.getByLabel('Group / solo')).toHaveValue('MEOVV');
 await expect(unresolved.getByLabel('Final title')).toHaveValue('261013 Anna');
 await unresolved.getByRole('checkbox').check();
 await expect(dialog.getByRole('button',{name:'Confirm to Sheet & DB'})).toBeDisabled();
 expect(calls.filter(c=>c.type==='commit')).toHaveLength(0);
});

test('expired private session reports an error without writing any sets',async({page})=>{
 const {dialog,calls}=await setup(page);
 await page.route('**/api/admin/terabox/preview',route=>route.fulfill({status:502,json:{error:'TeraBox session expired; reconnect the account'}}));
 await dialog.getByRole('button',{name:'Scan last 3 days'}).click();
 await expect(dialog.getByRole('alert')).toContainText('session expired');
 expect(calls).toHaveLength(0);
 await expect(dialog.getByRole('button',{name:'Confirm to Sheet & DB'})).toHaveCount(0);
});

test('HTML 502 from the gateway shows a useful error and does not create sets',async({page})=>{
 const {dialog,calls}=await setup(page);
 await page.route('**/api/admin/terabox/preview',route=>route.fulfill({status:502,contentType:'text/html',body:'<!DOCTYPE html><html>gateway diagnostics</html>'}));
 await dialog.getByRole('button',{name:'Scan last 3 days'}).click();
 await expect(dialog.getByRole('alert')).toContainText('Gateway request failed (HTTP 502)');
 await expect(dialog.getByRole('alert')).not.toContainText('Unexpected token');
 expect(calls).toHaveLength(0);
 await expect(dialog.getByRole('button',{name:'Scan last 3 days'})).toBeEnabled();
});

test('account API host saves with its session and follows account switching',async({page})=>{
 const {dialog}=await setup(page);
 let saved;
 await page.route('**/api/admin/terabox/accounts/1',async route=>{
  saved=route.request().postDataJSON();
  return route.fulfill({json:{saved:true}});
 });
 await page.route('**/api/admin/terabox/accounts',route=>route.fulfill({json:{accounts:[
  {id:1,label:'First drive',root:'/Albums',source:'studio',host:'dm.terabox.com'},
  {id:2,label:'Second drive',root:'/',host:'www.terabox.com'},
 ],jevConfigured:true,sheetSyncEnabled:true}}));
 await expect(dialog.getByLabel('API host')).toHaveValue('www.terabox.com');
 await dialog.getByLabel('API host').selectOption('dm.terabox.com');
 await dialog.getByLabel('Session Cookie').fill('ndus=test-cookie');
 await dialog.getByLabel('jsToken',{exact:true}).fill('test-js');
 await dialog.getByRole('button',{name:'Save account session'}).click();
 await expect(dialog.getByRole('status')).toContainText('Session saved securely');
 expect(saved).toMatchObject({host:'dm.terabox.com',cookie:'ndus=test-cookie',jsToken:'test-js',bdstoken:''});
 await expect(dialog.getByLabel('Session Cookie')).toHaveValue('');
 await dialog.getByLabel('TeraBox account',{exact:true}).selectOption('2');
 await expect(dialog.getByLabel('API host')).toHaveValue('www.terabox.com');
 await dialog.getByLabel('TeraBox account',{exact:true}).selectOption('1');
 await expect(dialog.getByLabel('API host')).toHaveValue('dm.terabox.com');
});

test('TeraBox preview reuses Finder and commits only the chosen row example',async({page},info)=>{
 const {dialog,calls}=await setup(page);
 const searches=[];
 const url='https://ganknow.com/services/terabox-example';
 await page.route('**/api/admin/example-candidates',async route=>{
  searches.push(route.request().postDataJSON());
  return route.fulfill({json:{candidates:[{url,title:'Jiyeon sample',source:'studio',status:'active',matchExplanation:'Date and name match',excerpt:'Public sample'}],recommendedUrl:url,confidence:0.98}});
 });
 await dialog.getByRole('button',{name:'Scan last 3 days'}).click();
 const row=dialog.locator('.tera-row').filter({has:page.getByLabel('Import 240404 지연2',{exact:true})});
 await row.locator('summary').filter({hasText:'Find an example link'}).click();
 await expect(row.getByLabel('Gank seller nickname')).toHaveValue('studio');
 await expect(row.getByLabel('Search date (YYMMDD)')).toHaveValue('240404');
 await expect(row.getByLabel('Search name preview')).toHaveValue('Jiyeon2');
 await row.getByLabel('Search name preview').fill('Jiyeon');
 await row.getByLabel('File count (optional)').fill('2075');
 await row.getByRole('button',{name:'Find example links',exact:true}).click();
 await expect(row.getByText('TypeSafe recommendation',{exact:false})).toBeVisible();
 await expect(row.getByLabel('Example URL (optional)')).toHaveValue('');
 expect(calls.filter(c=>c.type==='commit')).toHaveLength(0);
 await row.getByRole('button',{name:'Use this link'}).click();
 await expect(row.getByLabel('Example URL (optional)')).toHaveValue(url);
 const other=dialog.locator('.tera-row').filter({has:page.getByLabel('Import 261013 모르는이름',{exact:true})});
 await expect(other.getByLabel('Example URL (optional)')).toHaveValue('');
 expect(searches).toEqual([{seller:'studio',setDate:'2024-04-04',name:'Jiyeon',group:'TripleS',count:2075}]);
 await expect(row.getByLabel('Final title')).toHaveValue('240404 Jiyeon2');
 await dialog.screenshot({path:`test-results/terabox-finder-${info.project.name}.png`});
 await dialog.getByRole('button',{name:'Confirm to Sheet & DB'}).click();
 await expect(dialog.getByRole('status')).toContainText('Saved 1 sets');
 expect(calls.find(c=>c.type==='commit').body.rows).toEqual([{fileId:11,title:'240404 Jiyeon2',group:'TripleS',setDate:'2024-04-04',example:url,notes:''}]);
});

test('TeraBox Finder failure keeps manual example input usable',async({page})=>{
 const {dialog,calls}=await setup(page);
 await page.route('**/api/admin/example-candidates',route=>route.fulfill({status:502,json:{error:'Finder unavailable'}}));
 await dialog.getByRole('button',{name:'Scan last 3 days'}).click();
 const row=dialog.locator('.tera-row').first();
 await row.locator('summary').filter({hasText:'Find an example link'}).click();
 await row.getByRole('button',{name:'Find example links',exact:true}).click();
 await expect(row.getByRole('alert')).toHaveText('Finder unavailable');
 await row.getByLabel('Example URL (optional)').fill('https://ganknow.com/services/manual');
 await expect(row.getByLabel('Example URL (optional)')).toHaveValue('https://ganknow.com/services/manual');
 expect(calls.filter(c=>c.type==='commit')).toHaveLength(0);
});

test('TeraBox Finder seller dropdown uses gallery sources independently per row',async({page})=>{
 const {dialog}=await setup(page);
 await dialog.getByRole('button',{name:'Scan last 3 days'}).click();
 const rows=dialog.locator('.tera-row');
 const first=rows.first();
 const other=rows.last();
 await first.locator('summary').filter({hasText:'Find an example link'}).click();
 await other.locator('summary').filter({hasText:'Find an example link'}).click();
 const seller=first.getByRole('combobox',{name:'Gank seller nickname'});
 const otherSeller=other.getByRole('combobox',{name:'Gank seller nickname'});
 await expect(seller).toHaveValue('studio',{timeout:2000});
 expect(await seller.evaluate(el=>Array.from(el.list.options,o=>o.value))).toEqual(['studio','loveshakedata']);
 expect(await seller.getAttribute('list')).not.toBe(await otherSeller.getAttribute('list'));
 let query;
 await page.route('**/api/admin/example-candidates',route=>{query=route.request().postDataJSON();return route.fulfill({json:{candidates:[]}});});
 await seller.fill('loveshakedata');
 await first.getByRole('button',{name:'Find example links',exact:true}).click();
 await expect(first.getByText('No matching links found.',{exact:false})).toBeVisible();
 expect(query.seller).toBe('loveshakedata');
 await expect(otherSeller).toHaveValue('studio');
});
