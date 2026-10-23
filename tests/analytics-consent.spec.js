const { test, expect } = require('@playwright/test');

test.beforeEach(async({page})=>{
 await page.addInitScript(()=>localStorage.setItem('bias.quick-guide.hidden.v1','1'));
});

test('visitors can browse and reject analytics without sending a heartbeat',async({page,context},testInfo)=>{
 await page.clock.install();
 const beats=[];
 page.on('request',request=>{if(request.url().endsWith('/api/presence')&&request.method()==='POST')beats.push(request);});
 await page.goto('/');
 const banner=page.getByRole('region',{name:'Analytics cookies'});
 await expect(banner).toBeVisible();
 await expect(page.locator('.gallery-card').first()).toBeVisible();
 await page.screenshot({path:`test-results/analytics-consent-${testInfo.project.name}.png`});
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
 await page.clock.fastForward(40000);
 expect(beats).toHaveLength(0);
 await banner.getByRole('button',{name:'Reject analytics',exact:true}).click();
 await expect(banner).toHaveCount(0);
 await page.reload();
 await expect(banner).toHaveCount(0);
 await page.clock.fastForward(40000);
 expect(beats).toHaveLength(0);
 expect((await context.cookies()).some(cookie=>cookie.name==='gallery_visitor')).toBeFalsy();
});

test('acceptance enables heartbeats, persists on refresh, and withdrawal clears the visitor cookie',async({page,context})=>{
 await page.clock.install();
 const beats=[];
 page.on('request',request=>{if(request.url().endsWith('/api/presence')&&request.method()==='POST')beats.push(request);});
 await page.goto('/');
 await page.getByRole('button',{name:'Accept analytics',exact:true}).click();
 await expect.poll(()=>beats.length).toBe(1);
 expect(beats[0].headers()['x-gallery-analytics-consent']).toBe('accepted');
 await expect.poll(async()=>(await context.cookies()).some(cookie=>cookie.name==='gallery_visitor')).toBeTruthy();
 await expect(page.getByRole('status',{name:/consenting visitors online now/})).toBeVisible();
 await page.clock.fastForward(20000);
 await expect.poll(()=>beats.length).toBe(2);
 await page.reload();
 await expect(page.getByRole('region',{name:'Analytics cookies'})).toHaveCount(0);
 await expect.poll(()=>beats.length).toBe(3);
 await page.getByRole('button',{name:'Cookie settings',exact:true}).click();
 const settings=page.getByRole('dialog',{name:'Privacy & cookies'});
 await expect(settings).toContainText('90 days');
 await settings.getByRole('button',{name:'Reject analytics',exact:true}).click();
 await expect(settings).toHaveCount(0);
 await expect.poll(async()=>(await context.cookies()).some(cookie=>cookie.name==='gallery_visitor')).toBeFalsy();
 const stopped=beats.length;
 await page.clock.fastForward(60000);
 expect(beats).toHaveLength(stopped);
});

test('rejecting in another tab stops analytics in an already open gallery',async({page,context})=>{
 await page.goto('/');
 await page.getByRole('button',{name:'Accept analytics',exact:true}).click();
 await expect.poll(async()=>(await context.cookies()).some(cookie=>cookie.name==='gallery_visitor')).toBeTruthy();
 const second=await context.newPage();
 await second.goto('/');
 await second.getByRole('button',{name:'Cookie settings',exact:true}).click();
 await second.getByRole('dialog',{name:'Privacy & cookies'}).getByRole('button',{name:'Reject analytics',exact:true}).click();
 await page.getByRole('button',{name:'Cookie settings',exact:true}).click();
 await expect(page.getByRole('dialog',{name:'Privacy & cookies'})).toContainText('Analytics is off');
 await expect.poll(async()=>(await context.cookies()).some(cookie=>cookie.name==='gallery_visitor')).toBeFalsy();
});

test('acceptance waits for pending cookie removal before creating a visitor cookie',async({page,context})=>{
 await page.clock.install();
 let release;
 const removal=new Promise(resolve=>{release=resolve;});
 let clearing=false, beats=0;
 await page.route('**/api/presence',async route=>{
  if(route.request().method()==='DELETE'){clearing=true;await removal;}
  if(route.request().method()==='POST')beats++;
  await route.continue();
 });
 try{
  await page.goto('/');
  await expect.poll(()=>clearing).toBeTruthy();
  await page.getByRole('button',{name:'Accept analytics',exact:true}).click();
  await page.clock.fastForward(20000);
  expect(beats).toBe(0);
  release();
  await expect.poll(()=>beats).toBe(1);
  await expect.poll(async()=>(await context.cookies()).some(cookie=>cookie.name==='gallery_visitor')).toBeTruthy();
 }finally{release();}
});
