const { test, expect } = require('@playwright/test');

const office = process.env.E2E_BACKOFFICE_URL || 'http://localhost:5184';
const token = process.env.E2E_ADMIN_TOKEN || 'e2e-only-administrator-token-24chars';

function report(days) {
 return {
  days, from: new Date(Date.UTC(2026, 9, 24 - days)).toISOString().slice(0, 10), to: '2026-10-23', online: 3,
  visitors: 12, visits: 18, totalSeconds: 2160, averageSeconds: 120,
  daily: Array.from({ length: days }, (_, index) => ({ date: new Date(Date.UTC(2026, 9, 24 - days + index)).toISOString().slice(0, 10), visitors: index % 5, visits: index % 7, totalSeconds: 240, averageSeconds: 120 })),
  countries: [{ country: 'TH', visitors: 10, visits: 16, totalSeconds: 2040, averageSeconds: 127.5 }, { country: '', visitors: 2, visits: 2, totalSeconds: 120, averageSeconds: 60 }],
 };
}

async function signIn(page, hash = '') {
 await page.goto(office + '/' + hash);
 await page.getByLabel('Administrator token').fill(token);
 await page.getByRole('button', { name: 'Open the workspace' }).click();
}

test('sidebar opens usage charts, changes range, and preserves the dashboard on refresh', async ({ page }, testInfo) => {
 const errors = [];
 page.on('pageerror', error => errors.push(error.message));
 const requestedDays = [];
 await page.route('**/api/admin/analytics?*', route => {
  expect(route.request().headers().authorization).toMatch(/^Bearer /);
  const days = Number(new URL(route.request().url()).searchParams.get('days'));
  requestedDays.push(days);
  return route.fulfill({ json: report(days) });
 });
 await signIn(page);
 await page.getByRole('link', { name: 'Usage dashboard' }).click();
 await expect(page.getByRole('heading', { name: 'Usage dashboard.' })).toBeVisible();
 await expect(page.getByTestId('usage-visitors')).toHaveText('12');
 await expect(page.getByTestId('usage-average')).toHaveText('2m 00s');
 await expect(page.getByRole('cell', { name: 'Thailand', exact: true })).toBeVisible();
 await expect(page.getByRole('cell', { name: 'Unknown', exact: true })).toBeVisible();
 await expect(page.locator('canvas')).toHaveCount(3);
 await expect(page.getByRole('img', { name: 'Daily visitors' })).toBeVisible();
 await page.getByLabel('Date range').selectOption('7');
 await expect.poll(() => requestedDays.at(-1)).toBe(7);
 await page.getByLabel('Date range').selectOption('90');
 await expect.poll(() => requestedDays.at(-1)).toBe(90);
 await page.screenshot({ path: `test-results/usage-dashboard-${testInfo.project.name}.png`, fullPage: true });
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
 await page.reload();
 await expect(page.getByRole('heading', { name: 'Usage dashboard.' })).toBeVisible();
 await page.getByRole('link', { name: 'Image sets', exact: true }).click();
 await expect(page.getByRole('heading', { name: 'The back office.' })).toBeVisible();
 await expect(page.locator('canvas')).toHaveCount(0);
 expect(errors).toEqual([]);
});

test('dashboard has honest empty and error states with retry', async ({ page }) => {
 let fail = true;
 await page.route('**/api/admin/analytics?*', route => fail
  ? route.fulfill({ status: 500, json: { error: 'Analytics temporarily unavailable' } })
  : route.fulfill({ json: { ...report(30), online: 0, visitors: 0, visits: 0, averageSeconds: 0, totalSeconds: 0, countries: [], daily: report(30).daily.map(day => ({ ...day, visitors: 0, visits: 0, totalSeconds: 0, averageSeconds: 0 })) } }));
 await signIn(page, '#dashboard');
 await expect(page.getByRole('alert')).toContainText('Analytics temporarily unavailable');
 fail = false;
 await page.getByRole('button', { name: 'Retry', exact: true }).click();
 await expect(page.getByText('No visits recorded yet.')).toBeVisible();
 await expect(page.getByTestId('usage-visitors')).toHaveText('0');
 await expect(page.getByText('No country data for this period.')).toBeVisible();
});

test('real gallery heartbeats feed the authenticated dashboard without counting the back office', async ({ page }) => {
 await signIn(page, '#dashboard');
 await expect(page.getByTestId('usage-visitors')).toBeVisible();
 const headers = { Authorization: `Bearer ${token}` };
 const beforeResponse = await page.request.get(`${office}/api/admin/analytics?days=7`, { headers });
 expect(beforeResponse.ok()).toBeTruthy();
 const before = await beforeResponse.json();
 for (let index = 0; index < 2; index++) {
  const heartbeat = await page.request.post(`${office}/api/presence`, { headers: { 'X-Gallery-Analytics-Consent': 'accepted' } });
  expect(heartbeat.ok()).toBeTruthy();
 }
 const after = await (await page.request.get(`${office}/api/admin/analytics?days=7`, { headers })).json();
 expect(after.visitors).toBe(before.visitors + 1);
 expect(after.visits).toBe(before.visits + 1);
 await page.getByRole('button', { name: 'Refresh usage' }).click();
 await expect(page.getByTestId('usage-visitors')).toHaveText(String(after.visitors));
 await page.getByRole('button', { name: 'Sign out' }).click();
 await expect(page.getByLabel('Administrator token')).toBeVisible();
 await expect(page.getByRole('navigation', { name: 'Back office navigation' })).toHaveCount(0);
});

test('usage refreshes every 20 seconds and stops polling when leaving the dashboard', async ({ page }) => {
 await page.clock.install();
 let requests = 0;
 await page.route('**/api/admin/analytics?*', route => {
  requests++;
  return route.fulfill({ json: { ...report(30), online: requests } });
 });
 await signIn(page, '#dashboard');
 await expect(page.getByTestId('usage-online')).toHaveText('1');
 await page.clock.fastForward(20000);
 await expect(page.getByTestId('usage-online')).toHaveText('2');
 await page.getByRole('link', { name: 'Image sets', exact: true }).click();
 await expect(page.getByRole('heading', { name: 'The back office.' })).toBeVisible();
 await page.clock.fastForward(40000);
 expect(requests).toBe(2);
});
