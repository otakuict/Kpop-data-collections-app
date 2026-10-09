const { test, expect } = require('@playwright/test');

async function dismissGuide(page, hide = true) {
 await page.getByRole('checkbox', { name: /Don't show it again/ }).setChecked(hide);
 await page.getByRole('button', { name: 'Close short guide' }).click();
}

test('the animated tour advances, remembers opt-out, and can be reopened', async ({ page }) => {
 await page.goto('/');
 await expect(page.getByRole('heading', { name: 'Choose your view' })).toBeVisible();
 await page.getByRole('button', { name: 'Next', exact: true }).click();
 await expect(page.getByRole('heading', { name: 'Find your favorites' })).toBeVisible();
 await page.getByRole('button', { name: 'Next', exact: true }).click();
 await expect(page.getByRole('heading', { name: 'Explore each set' })).toBeVisible();
 await page.getByRole('checkbox', { name: /Don't show it again/ }).check();
 await page.getByRole('button', { name: 'Get started' }).click();
 await page.reload();
 await expect(page.getByRole('button', { name: 'Switch to table view' })).toBeVisible();
 await expect(page.getByRole('dialog')).toHaveCount(0);
 await page.getByRole('button', { name: 'Open short guide' }).click();
 await expect(page.getByRole('checkbox', { name: /Don't show it again/ })).toBeChecked();
 await dismissGuide(page, false);
 await page.reload();
 await expect(page.getByRole('dialog')).toBeVisible();
 await expect(page.getByRole('checkbox', { name: /Don't show it again/ })).not.toBeChecked();
});

test('table shares filters and details, and switching views retains the loaded sets', async ({ page }) => {
 const setRequests = [];
 page.on('request', request => { if (request.url().includes('/api/sets?')) setRequests.push(request.url()); });
 await page.goto('/'); await dismissGuide(page);
 await page.getByRole('button', { name: 'aespa', exact: true }).click();
 await expect(page.locator('.gallery-card')).toHaveCount(3);
 const titles = await page.locator('.card-title').allTextContents();
 const before = setRequests.length;
 await page.getByRole('button', { name: 'Switch to table view' }).click();
 await expect(page.locator('.gallery-table tbody tr')).toHaveCount(3);
 await expect(page).toHaveURL(/group=aespa.*view=table/);
 expect(setRequests.length).toBe(before);
 await expect(page.getByRole('columnheader', { name: 'Shop / source' })).toBeVisible();
 await page.getByRole('button', { name: `View ${titles[0]}`, exact: true }).click();
 await expect(page.getByRole('dialog')).toBeVisible();
 await page.getByRole('button', { name: 'Close dialog' }).click();
 await page.getByRole('button', { name: 'Switch to card view' }).click();
 await expect(page.locator('.card-title')).toHaveText(titles);
 expect(setRequests.length).toBe(before);
 await page.getByRole('button', { name: 'Switch to table view' }).click();
 await page.reload();
 await expect(page.locator('.gallery-table tbody tr')).toHaveCount(3);
});
