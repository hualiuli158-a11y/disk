import { expect, test, type Page } from '@playwright/test';

async function installClock(page: Page) {
  await page.evaluate(() => {
    (window as any).feedbackModuleUrls = Object.fromEntries(performance.getEntriesByType('resource').map(entry => [new URL(entry.name).pathname, entry.name]));
  });
  await page.clock.install();
}

async function showError(page: Page, message: string) {
  await page.evaluate(async message => {
    const url = (window as any).feedbackModuleUrls['/src/stores/workspaceStore.ts'];
    const { useWorkspaceStore } = await import(url);
    useWorkspaceStore.getState().setError(message);
  }, message);
}

test('errors expire and repeated identical errors receive a fresh countdown', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await installClock(page);
  await showError(page, '操作失败，请重试');
  await expect(page.locator('.inline-error')).toBeVisible();
  await page.clock.runFor(4000);
  await showError(page, '操作失败，请重试');
  await page.clock.runFor(2000);
  await expect(page.locator('.inline-error')).toBeVisible();
  await page.clock.runFor(3100);
  await expect(page.locator('.inline-error')).toHaveCount(0);
  await showError(page, '第二次错误');
  await page.locator('.inline-error').hover();
  await page.clock.runFor(6000);
  await expect(page.locator('.inline-error')).toBeVisible();
  await page.mouse.move(0, 0);
  await page.clock.runFor(5100);
  await expect(page.locator('.inline-error')).toHaveCount(0);
});

test('upload stays visible while active and disappears five seconds after completion', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await page.evaluate(async () => {
    const url = (window as any).feedbackModuleUrls?.['/src/services/api.ts'] || performance.getEntriesByType('resource').map(entry => entry.name).find(url => new URL(url).pathname === '/src/services/api.ts')!;
    const { api } = await import(url);
    const upload = api.uploadWithProgress;
    api.uploadWithProgress = async (...args: Parameters<typeof upload>) => {
      await new Promise<void>(resolve => { (window as any).finishUpload = resolve; });
      return upload(...args);
    };
  });
  await installClock(page);
  await page.locator('input[type=file]').setInputFiles({ name: '自动消失.txt', mimeType: 'text/plain', buffer: Buffer.from('content') });
  await expect(page.getByText('正在上传文件', { exact: true })).toBeVisible();
  await page.clock.runFor(10000);
  await expect(page.locator('.upload-status')).toBeVisible();
  await page.evaluate(() => (window as any).finishUpload());
  await page.clock.runFor(500);
  await expect(page.getByText('上传完成', { exact: true })).toBeVisible();
  await page.clock.runFor(4000);
  await expect(page.locator('.upload-status')).toBeVisible();
  await page.clock.runFor(1500);
  await expect(page.locator('.upload-status')).toHaveCount(0);
  await expect(page.getByRole('treeitem', { name: '自动消失.txt', exact: true })).toBeVisible();
});

test('a new upload cancels the previous completion countdown; failures also expire', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await installClock(page);
  await page.locator('input[type=file]').setInputFiles({ name: '完成.txt', mimeType: 'text/plain', buffer: Buffer.from('content') });
  await page.clock.runFor(500);
  await expect(page.getByText('上传完成', { exact: true })).toBeVisible();
  await page.evaluate(async () => {
    const url = (window as any).feedbackModuleUrls?.['/src/services/api.ts'] || performance.getEntriesByType('resource').map(entry => entry.name).find(url => new URL(url).pathname === '/src/services/api.ts')!;
    const { api, ApiError } = await import(url);
    api.uploadWithProgress = async () => {
      await new Promise<void>(resolve => { (window as any).failUpload = resolve; });
      throw new ApiError('上传失败', 503);
    };
  });
  await page.locator('input[type=file]').setInputFiles({ name: '失败.txt', mimeType: 'text/plain', buffer: Buffer.from('content') });
  await page.clock.runFor(6000);
  await expect(page.getByText('正在上传文件', { exact: true })).toBeVisible();
  await page.evaluate(() => (window as any).failUpload());
  await expect(page.locator('.upload-status-heading strong')).toHaveText('上传失败');
  await page.clock.runFor(5100);
  await expect(page.locator('.upload-status')).toHaveCount(0);
});
