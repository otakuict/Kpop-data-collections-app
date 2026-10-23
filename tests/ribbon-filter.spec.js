const { test, expect } = require('@playwright/test');

const sets = [
 { title: 'New aespa', group: 'aespa', style: { new: true } },
 { title: 'Selective aespa', group: 'aespa', style: { selective: true } },
 { title: 'Both ribbons', group: 'aespa', style: { new: true, selective: true } },
 { title: 'New IVE', group: 'IVE', style: { new: true } },
 { title: 'Trade ribbons', group: 'aespa', style: { wishlist: true, noTrade: true } },
 { title: 'No ribbons', group: 'aespa', style: {} },
].map((set, index) => ({ ...set, id: index + 1, date: '2026-10-07', source: 'Fixture', example: '', notes: '', images: [], imageState: 'missing' }));

test.beforeEach(async ({ page }) => {
 await page.addInitScript(() => localStorage.setItem('bias.quick-guide.hidden.v1', '1'));
 await page.route('**/api/facets', route => route.fulfill({ json: {
  total: sets.length, ready: 0, groups: [{ name: 'aespa', count: 5 }, { name: 'IVE', count: 1 }],
 } }));
 await page.route('**/api/presence', route => route.fulfill({ json: { online: 1 } }));
 await page.route('**/api/sets?*', route => {
  const query = new URL(route.request().url()).searchParams;
  const matching = sets.filter(set => (!query.get('ribbon') || set.style[query.get('ribbon')]) &&
   (!query.get('group') || set.group === query.get('group')) &&
   (!query.get('q') || set.title.toLowerCase().includes(query.get('q').toLowerCase())));
  return route.fulfill({ json: { items: matching, total: matching.length } });
 });
});

test('the collection heading is removed and existing ribbons filter alongside artists', async ({ page }, testInfo) => {
 await page.goto('/');
 await expect(page.getByRole('region', { name: 'Archive introduction' })).toBeVisible();
 await expect(page.getByRole('heading', { name: 'The collection.', exact: true })).toHaveCount(0);
 await expect(page.getByText('BROWSE THE ARCHIVE', { exact: true })).toHaveCount(0);
 const ribbon = page.getByLabel('Filter by ribbon type', { exact: true });
 await ribbon.selectOption('new');
 await expect(page.locator('.card-title')).toHaveText(['New aespa', 'Both ribbons', 'New IVE']);
 await expect(page).toHaveURL(/ribbon=new/);
 await page.locator('.collection-content').screenshot({ path: `test-results/ribbon-filter-${testInfo.project.name}.png` });
 await page.getByRole('button', { name: 'aespa', exact: true }).click();
 await expect(page.locator('.card-title')).toHaveText(['New aespa', 'Both ribbons']);
 await ribbon.selectOption('selective');
 await expect(page.locator('.card-title')).toHaveText(['Selective aespa', 'Both ribbons']);
 await page.getByRole('button', { name: 'Switch to table view' }).click();
 await expect(page.locator('.gallery-table tbody tr')).toHaveCount(2);
 await page.reload();
 await expect(ribbon).toHaveValue('selective');
 await expect(page.locator('.gallery-table tbody tr')).toHaveCount(2);
 await page.getByRole('button', { name: 'All sets', exact: true }).click();
 await expect(ribbon).toHaveValue('selective');
 await ribbon.selectOption('wishlist');
 await expect(page.locator('.gallery-table tbody tr')).toHaveCount(1);
 await expect(page.locator('.gallery-table tbody')).toContainText('Trade ribbons');
 await ribbon.selectOption('noTrade');
 await expect(page.locator('.gallery-table tbody tr')).toHaveCount(1);
 await ribbon.selectOption('');
 await expect(page.locator('.gallery-table tbody tr')).toHaveCount(sets.length);
 await expect(page).not.toHaveURL(/ribbon=/);
});

test('reset filters clears the selected ribbon and artist', async ({ page }) => {
 await page.goto('/?group=IVE&ribbon=selective');
 await expect(page.getByRole('heading', { name: 'No moments found' })).toBeVisible();
 await page.getByRole('button', { name: 'Reset filters', exact: true }).click();
 await expect(page.locator('.gallery-card')).toHaveCount(sets.length);
 await expect(page.getByLabel('Filter by ribbon type', { exact: true })).toHaveValue('');
 await expect(page).not.toHaveURL(/ribbon=|group=/);
});
