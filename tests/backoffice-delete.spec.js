const { test, expect } = require('@playwright/test');

const office = process.env.E2E_BACKOFFICE_URL || 'http://localhost:5184';
const token = process.env.E2E_ADMIN_TOKEN || 'e2e-only-administrator-token-24chars';
const headers = { Authorization: `Bearer ${token}` };

async function signIn(page) {
 await page.goto(office);
 await page.getByLabel('Administrator token').fill(token);
 await page.getByRole('button', { name: 'Open the workspace' }).click();
 await expect(page.getByRole('heading', { name: 'The back office.' })).toBeVisible();
}

test('deleting a set requires confirmation, permits cancellation, and refreshes the catalog', async ({ page }, testInfo) => {
 const title = `Delete confirmation ${testInfo.project.name} ${Date.now()}`;
 const created = await page.request.post(`${office}/api/admin/sets`, { headers, data: { title, group: 'DELETE TEST' } });
 expect(created.ok()).toBeTruthy();
 const set = await created.json();
 const url = `${office}/api/admin/sets/${set.id}`;
 const deletions = [];
 page.on('request', request => { if (request.method() === 'DELETE') deletions.push(request.url()); });
 try {
  await signIn(page);
  await page.getByLabel('Search sets', { exact: true }).fill(title);
  const row = page.getByRole('row').filter({ hasText: title });
  const remove = row.getByRole('button', { name: `Delete ${title}`, exact: true });
  await expect(remove).toBeVisible();
  const total = page.locator('.stats-grid > div').first().locator('strong');
  const totalBefore = Number(await total.innerText());
  await remove.click();
  const modal = page.getByRole('dialog', { name: 'Delete image set?' });
  await expect(modal).toContainText(title);
  await expect(modal).toContainText(`set_id: ${set.id}`);
  await expect(modal).toContainText('cannot be undone');
  await page.screenshot({ path: `test-results/delete-confirmation-${testInfo.project.name}.png` });
  await modal.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(modal).toHaveCount(0);
  await expect(remove).toBeFocused();
  await remove.click();
  await page.keyboard.press('Escape');
  await expect(modal).toHaveCount(0);
  expect(deletions).toEqual([]);
  expect((await page.request.get(`${office}/api/sets/${set.id}`)).status()).toBe(200);

  await remove.click();
  await modal.getByRole('button', { name: 'Delete image set', exact: true }).click();
  await expect(modal).toHaveCount(0);
  await expect(row).toHaveCount(0);
  await expect(page.getByRole('status')).toContainText(`Deleted “${title}”.`);
  await expect(page.locator('.pagination')).toContainText('0 matching sets');
  await expect(total).toHaveText(String(totalBefore - 1));
  expect(deletions).toEqual([url]);
  expect((await page.request.get(`${office}/api/sets/${set.id}`)).status()).toBe(404);
 } finally {
  await page.request.delete(url, { headers });
 }
});

test('a failed deletion stays open for retry and blocks duplicate submissions while pending', async ({ page }, testInfo) => {
 const title = `Retry deletion ${testInfo.project.name} ${Date.now()}`;
 const created = await page.request.post(`${office}/api/admin/sets`, { headers, data: { title, group: 'DELETE TEST' } });
 expect(created.ok()).toBeTruthy();
 const set = await created.json();
 const url = `${office}/api/admin/sets/${set.id}`;
 let release;
 try {
  await signIn(page);
  await page.getByLabel('Search sets', { exact: true }).fill(title);
  const row = page.getByRole('row').filter({ hasText: title });
  await row.getByRole('button', { name: `Delete ${title}`, exact: true }).click();
  const modal = page.getByRole('dialog', { name: 'Delete image set?' });
  await page.route(url, route => route.fulfill({ status: 500, json: { error: 'Database operation failed' } }));
  await modal.getByRole('button', { name: 'Delete image set', exact: true }).click();
  await expect(modal.getByRole('alert')).toHaveText('Database operation failed');
  expect((await page.request.get(`${office}/api/sets/${set.id}`)).status()).toBe(200);
  await page.unroute(url);

  const pending = new Promise(resolve => { release = resolve; });
  await page.route(url, async route => { await pending; await route.continue(); });
  await modal.getByRole('button', { name: 'Delete image set', exact: true }).click();
  await expect(modal.getByRole('button', { name: 'Deleting…', exact: true })).toBeDisabled();
  await expect(modal.getByRole('button', { name: 'Cancel', exact: true })).toBeDisabled();
  await expect(modal.getByRole('button', { name: 'Close delete confirmation' })).toBeDisabled();
  await page.keyboard.press('Escape');
  await expect(modal).toBeVisible();
  release();
  await expect(modal).toHaveCount(0);
  await expect(row).toHaveCount(0);
 } finally {
  release?.();
  await page.unroute(url);
  await page.request.delete(url, { headers });
 }
});

test('deleting the only set on the last page returns to the previous page', async ({ page }, testInfo) => {
 const prefix = `Delete last page ${testInfo.project.name} ${Date.now()}`;
 const created = [];
 try {
  for (let index = 0; index < 41; index++) {
   const response = await page.request.post(`${office}/api/admin/sets`, { headers, data: { title: `${prefix} ${index}`, group: 'PAGINATION TEST' } });
   expect(response.ok()).toBeTruthy();
   created.push(await response.json());
  }
  await signIn(page);
  await page.getByLabel('Search sets', { exact: true }).fill(prefix);
  await expect(page.locator('.pagination')).toContainText('41 matching sets');
  await page.getByRole('button', { name: 'Next page', exact: true }).click();
  await expect(page.locator('.pagination')).toContainText('2 / 2');
  const row = page.locator('tbody tr');
  await expect(row).toHaveCount(1);
  const title = await row.locator('.set-title').innerText();
  await row.getByRole('button', { name: `Delete ${title}`, exact: true }).click();
  await page.getByRole('dialog', { name: 'Delete image set?' }).getByRole('button', { name: 'Delete image set', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('.pagination')).toContainText('40 matching sets');
  await expect(page.locator('.pagination')).toContainText('1 / 1');
  await expect(page.locator('tbody tr')).toHaveCount(40);
 } finally {
  await Promise.all(created.map(set => page.request.delete(`${office}/api/admin/sets/${set.id}`, { headers })));
 }
});
