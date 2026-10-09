const { test, expect } = require('@playwright/test');
test.beforeEach(async ({ page }) => {
 await page.addInitScript(() => localStorage.setItem('bias.quick-guide.hidden.v1', '1'));
});


const sets = Array.from({ length: 50 }, (_, index) => ({
 id: index + 1, title: `Scroll set ${index + 1}`, group: index < 2 ? 'aespa' : 'IVE',
 date: '2026-10-07', source: 'Fixture', example: '',
 images: [1, 2].map(id => ({ id: index * 2 + id, url: `/api/images/${index * 2 + id}` })),
}));

async function mockArchive(page, respond) {
 await page.route('**/api/facets', route => route.fulfill({ json: {
  total: 50, ready: 50, groups: [{ name: 'aespa', count: 2 }, { name: 'IVE', count: 48 }],
 } }));
 await page.route('**/api/images/*', route => route.fulfill({ contentType: 'image/svg+xml',
  body: '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="125"><rect width="100" height="125" fill="#637a41"/></svg>',
 }));
 await page.route('**/api/sets?*', async route => {
  const params = new URL(route.request().url()).searchParams;
  if (params.has('page') && await respond?.(route, params)) return;
  const matching = sets.filter(set => (!params.get('group') || set.group === params.get('group')) &&
   (!params.get('q') || set.title.includes(params.get('q'))));
  const number = Number(params.get('page') || 1), limit = Number(params.get('limit') || 24);
  await route.fulfill({ json: { items: matching.slice((number - 1) * limit, number * limit), total: matching.length } });
 });
}

async function reachEnd(page) {
 await page.locator('.collection-content').evaluate(node => window.scrollTo({ top: node.offsetTop + node.offsetHeight, behavior: 'instant' }));
}

test('scrolling appends sets, preserves carousel state, and stops at the final batch', async ({ page }) => {
 const requests = [], errors = [];
 page.on('pageerror', error => errors.push(error.message));
 await mockArchive(page, async (_, params) => { requests.push(Number(params.get('page'))); });
 await page.goto('/');
 await expect(page.locator('.gallery-card')).toHaveCount(24);
 await expect(page.getByRole('button', { name: /^(Next|Previous) page$/ })).toHaveCount(0);
 const first = page.locator('.gallery-card').first();
 await first.getByRole('slider').focus();await page.keyboard.press('End');
 await reachEnd(page);await expect(page.locator('.gallery-card')).toHaveCount(48);
 await expect(first.getByLabel('Image position')).toHaveText('2 / 2');
 await expect(first.locator('.card-title')).toHaveText('Scroll set 1');
 await reachEnd(page);await expect(page.locator('.gallery-card')).toHaveCount(50);
 await expect(page.getByText('All sets loaded', { exact: true })).toBeVisible();
 await expect(page.locator('.card-number').last()).toHaveText('050');
 await expect(page).not.toHaveURL(/[?&]page=/);
 expect(requests).toEqual([1, 2, 3]);expect(errors).toEqual([]);
});

test('a failed next batch keeps visible sets and retries the same batch', async ({ page }) => {
 let secondRequests = 0;
 await mockArchive(page, async (route, params) => {
  if (params.get('page') !== '2') return false;
  secondRequests++;
  if (secondRequests !== 1) return false;
  await route.fulfill({ status: 503, json: { error: 'Temporary archive failure' } });return true;
 });
 await page.goto('/');await expect(page.locator('.gallery-card')).toHaveCount(24);
 await reachEnd(page);await expect(page.getByRole('alert')).toContainText('Temporary archive failure');
 await expect(page.locator('.gallery-card')).toHaveCount(24);
 await page.getByRole('button', { name: 'Retry loading more' }).click();
 await expect(page.locator('.gallery-card')).toHaveCount(48);expect(secondRequests).toBe(2);
});

test('changing a filter discards an in-flight next batch and resets the list', async ({ page }) => {
 let release, started;
 const pending = new Promise(resolve => { started = resolve; });
 const held = new Promise(resolve => { release = resolve; });
 await mockArchive(page, async (route, params) => {
  if (params.get('page') !== '2' || params.get('group')) return false;
  started();await held;
  await route.fulfill({ json: { items: sets.slice(24, 48), total: 50 } }).catch(() => {});return true;
 });
 await page.goto('/');await expect(page.locator('.gallery-card')).toHaveCount(24);
 await reachEnd(page);await pending;
 await page.getByRole('button', { name: 'aespa', exact: true }).click();
 await expect(page.locator('.gallery-card')).toHaveCount(2);
 release();await expect(page.locator('.gallery-card')).toHaveCount(2);
 await expect(page.locator('.card-title')).toHaveText(['Scroll set 1', 'Scroll set 2']);
 await expect(page).toHaveURL(/group=aespa/);
 await expect(page).not.toHaveURL(/[?&]page=/);
});

test('changing sort at the end returns to the collection start without loading every batch', async ({ page }) => {
 await mockArchive(page);
 await page.goto('/');await expect(page.locator('.gallery-card')).toHaveCount(24);
 await reachEnd(page);await expect(page.locator('.gallery-card')).toHaveCount(48);
 await reachEnd(page);await expect(page.locator('.gallery-card')).toHaveCount(50);
 await page.getByLabel('Sort sets').selectOption('oldest');
 await expect(page.locator('.gallery-card')).toHaveCount(24);
 await expect(page.locator('.collection-heading')).toBeInViewport();
 await expect(page).toHaveURL(/sort=oldest/);
});
