const { test, expect } = require('@playwright/test');
const office=process.env.E2E_BACKOFFICE_URL||'http://localhost:5184';
const token=process.env.E2E_ADMIN_TOKEN||'e2e-only-administrator-token-24chars';

async function signIn(page){await page.goto(office);await page.getByLabel('Administrator token').fill(token);await page.getByRole('button',{name:'Open the workspace'}).click();await expect(page.getByRole('heading',{name:'The back office.'})).toBeVisible();}

test('gallery filters real seed records, groups dates, opens details, and handles empty results',async({page},testInfo)=>{
 const errors=[];page.on('pageerror',e=>errors.push(e.message));await page.goto('/');
 await expect(page.locator('.gallery-card')).toHaveCount(24);await expect(page.getByRole('heading',{name:/Your bias/})).toBeVisible();
 await page.screenshot({path:`test-results/gallery-${testInfo.project.name}.png`,fullPage:true});
 await page.getByRole('button',{name:'aespa',exact:true}).click();await expect(page.locator('.gallery-card')).toHaveCount(3);
 if(testInfo.project.name==='mobile')await page.getByRole('button',{name:'Filters',exact:true}).click();
 await page.getByLabel('From date').fill('2026-09-15');await page.getByLabel('To date').fill('2026-09-15');await expect(page.locator('.gallery-card')).toHaveCount(1);
 await page.getByLabel('Group by').selectOption('date');await expect(page.locator('.section-label')).toContainText('15 Sep 2026');
 await page.getByRole('button',{name:'View 260915 Karina kdata',exact:true}).click();await expect(page.getByRole('dialog')).toBeVisible();await expect(page.getByRole('link',{name:'Open example post'})).toHaveAttribute('href',/ganknow/);await page.getByRole('button',{name:'Close dialog'}).click();
 await page.getByLabel('Search image sets').fill('this-does-not-exist');await expect(page.getByRole('heading',{name:'No moments found'})).toBeVisible();await page.getByRole('button',{name:'Reset filters'}).click();await expect(page.locator('.gallery-card')).toHaveCount(24);
 expect(errors).toEqual([]);
});

test('back-office rejects unauthorized writes and adds a real uploaded preview visible in gallery',async({page},testInfo)=>{
 const rejected=await page.request.post(`${office}/api/admin/sets`,{data:{title:'Unauthorized',group:'TEST'}});expect(rejected.status()).toBe(401);
 await signIn(page);await page.getByRole('button',{name:'Add image set'}).click();
 const title=`E2E moment ${testInfo.project.name} ${Date.now()}`;
 await page.getByLabel('Set title').fill(title);await page.getByLabel('Group / artist').fill('TEST ARCHIVE');await page.getByLabel('Date of set').fill('2026-10-06');await page.getByLabel('Source', {exact:true}).fill('Browser acceptance test');
 const preview=await page.request.get(`${office}/api/images/1`);expect(preview.ok()).toBeTruthy();
 const second=await page.request.get(`${office}/api/images/2`);expect(second.ok()).toBeTruthy();
 await page.getByLabel('Preview images',{exact:true}).setInputFiles([{name:'preview.jpg',mimeType:'image/jpeg',buffer:await preview.body()},{name:'second.jpg',mimeType:'image/jpeg',buffer:await second.body()}]);
 await page.getByRole('button',{name:'Save image set',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(0);await page.getByLabel('Search sets',{exact:true}).fill(title);
 const row=page.getByRole('row').filter({hasText:title});await expect(row).toContainText('2/5 ready');await expect(page.getByRole('columnheader',{name:'SET_ID',exact:true})).toBeVisible();await row.getByRole('button',{name:`Edit ${title}`,exact:true}).click();await expect(page.getByLabel('set_id',{exact:true})).toHaveValue(/^[0-9]+$/);await page.getByRole('button',{name:'Close editor'}).click();
 await page.screenshot({path:`test-results/backoffice-${testInfo.project.name}.png`,fullPage:true});
 await page.goto(`/?q=${encodeURIComponent(title)}`);await expect(page.locator('.gallery-card')).toHaveCount(1);await page.locator('.gallery-card').getByRole('button',{name:`View ${title}`,exact:true}).click();await expect(page.locator('.detail-image>img')).toBeVisible();
});

test('CSV import is idempotent and retains manual entries',async({page})=>{
 await signIn(page);await page.getByRole('button',{name:'Import & previews'}).click();await page.getByLabel('Upload CSV',{exact:true}).setInputFiles('data/source.csv');await page.getByRole('button',{name:'Import CSV',exact:true}).click();
 await expect(page.getByRole('status')).toContainText('Imported 165 rows: 0 new, 165 updated.');
 await page.getByRole('button',{name:'Image sets',exact:false}).click();await page.getByLabel('Search sets',{exact:true}).fill('E2E moment');await expect(page.getByRole('row').filter({hasText:'E2E moment'}).first()).toContainText('2/5 ready');
});

test('card carousel uses a bottom drag bar and keeps page scrolling independent',async({page},testInfo)=>{
 const album={id:9001,title:'Carousel acceptance',group:'TEST',date:'2026-10-06',source:'Fixture',example:'',images:[1,2,3].map(id=>({id,url:`/api/images/${id}`}))};
 await page.route('**/api/sets?*',route=>route.fulfill({json:{items:[album],total:1,page:1,limit:24}}));
 await page.goto('/');const card=page.locator('.gallery-card');await expect(card).toHaveCount(1);
 const bar=card.getByRole('slider',{name:'Choose image'});
 await expect(bar).toBeVisible();await expect(card.getByLabel('Image position')).toHaveText('1 / 3');
 await expect(card.getByRole('button',{name:/^(Next|Previous) image$/})).toHaveCount(0);
 await bar.scrollIntoViewIfNeeded();const bounds=await bar.boundingBox(),image=await card.locator('.card-image').boundingBox();
 expect(Math.abs(bounds.x+bounds.width/2-image.x-image.width/2)).toBeLessThan(1);
 expect(bounds.y-image.y).toBeGreaterThan(image.height*.65);
 const left=bounds.x+12,right=bounds.x+bounds.width-12,y=bounds.y+bounds.height/2;
 if(testInfo.project.name==='mobile'){
  const session=await page.context().newCDPSession(page);
  await session.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:left,y}]});
  for(let step=1;step<=6;step++){
   await session.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:left+(right-left)*step/6,y}]});
   await page.evaluate(()=>new Promise(requestAnimationFrame));
  }
  await session.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
 }else{
  await page.mouse.move(left,y);await page.mouse.down();
  await page.mouse.move(left+(right-left)*.3,y);
  // While held, images follow the pointer between image boundaries.
  await expect.poll(()=>card.locator('.carousel-track').evaluate(track=>{
   const offset=-new DOMMatrixReadOnly(getComputedStyle(track).transform).m41/track.clientWidth;
   return Math.abs(offset-.6);
  })).toBeLessThan(.05);
  // Pointer capture keeps dragging active beyond the bar itself.
  await page.mouse.move(right+40,y-45);await page.mouse.up();
 }
 await expect(card.getByLabel('Image position')).toHaveText('3 / 3');
 await page.mouse.move(right,y);await page.mouse.down();await page.mouse.move(left-40,y);await page.mouse.up();
 await expect(card.getByLabel('Image position')).toHaveText('1 / 3');
 await card.locator('.carousel-slide').first().hover();const scrollBefore=await page.evaluate(()=>scrollY);
 const delta=scrollBefore>0?-140:140;await page.mouse.wheel(0,delta);
 await expect.poll(async()=>((await page.evaluate(()=>scrollY))-scrollBefore)*Math.sign(delta)).toBeGreaterThan(0);
 await expect(card.getByLabel('Image position')).toHaveText('1 / 3');
 await bar.focus();await page.keyboard.press('End');await expect(bar).toHaveAttribute('aria-valuenow','3');
 await page.keyboard.press('Home');await page.keyboard.press('ArrowRight');await expect(card.getByLabel('Image position')).toHaveText('2 / 3');
 await card.getByRole('button',{name:'View Carousel acceptance, image 2',exact:true}).click();
 await expect(page.getByRole('dialog')).toBeVisible();await expect(page.locator('.detail-image>img')).toHaveAttribute('src','/api/images/2');
});
