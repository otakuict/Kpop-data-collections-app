const { defineConfig, devices } = require('@playwright/test');
module.exports=defineConfig({
 testDir:'./tests',fullyParallel:false,workers:1,timeout:45000,
 use:{baseURL:process.env.E2E_GALLERY_URL||'http://localhost:5183',trace:'retain-on-failure',launchOptions:process.env.PLAYWRIGHT_CHROME_PATH?{executablePath:process.env.PLAYWRIGHT_CHROME_PATH}:{}},
 projects:[{name:'desktop',use:{...devices['Desktop Chrome']}},{name:'mobile',use:{...devices['iPhone 13'],defaultBrowserType:'chromium'}}],
 reporter:'list'
});
