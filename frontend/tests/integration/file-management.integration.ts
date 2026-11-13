import { expect, test, type Page } from '@playwright/test';
import { readFile } from 'node:fs/promises';

test('real C++ API file management through the browser', async ({ page }, testInfo) => {
  const username = `browser_${Date.now()}`;
  const password = 'browser-integration-password-123';
  const a = '浏览器目录 A';
  const renamedA = '浏览器目录 A 已重命名';
  const b = '浏览器目录 B';
  const child = '子目录';
  const filename = 'browser-original.txt';
  const renamedFile = 'browser-renamed.txt';
  const content = Buffer.from('真实浏览器与 C++ 文件传输验收\n'.repeat(2000));
  const calls: Array<{ method: string; path: string; status: number }> = [];
  const pageErrors: string[] = [];
  page.on('pageerror', error => pageErrors.push(error.message));
  page.on('response', response => {
    const path = new URL(response.url()).pathname;
    if (path.startsWith('/api/') || path === '/register' || path === '/login') {
      calls.push({ method: response.request().method(), path, status: response.status() });
    }
  });

  async function mutation(method: string, path: RegExp, action: () => Promise<unknown>, status = 200) {
    const [response] = await Promise.all([
      page.waitForResponse(response => response.request().method() === method && path.test(new URL(response.url()).pathname), { timeout: 15_000 }),
      action(),
    ]);
    expect(response.status(), `${method} ${response.url()}`).toBe(status);
    return status === 204 ? undefined : response.json();
  }

  async function manage() {
    await page.getByRole('button', { name: '管理文件', exact: true }).click();
    await expect(page.locator('.listing-page')).toBeVisible();
  }

  async function root() {
    await page.locator('.space-row').click();
    await manage();
    await expect(page.locator('.listing-heading h1')).toHaveText('我的空间');
  }

  async function createFolder(name: string) {
    await page.locator('.quick-actions').getByRole('button', { name: '文件夹', exact: true }).click();
    const input = page.getByRole('textbox', { name: '名称', exact: true });
    await input.fill(name);
    const result = await mutation('POST', /^\/api\/folders$/, () => input.press('Enter'));
    await expect(page.getByRole('button', { name: `更多操作：${name}`, exact: true })).toBeVisible();
    return result.id as number;
  }

  async function menu(name: string, action: string) {
    await page.getByRole('button', { name: `更多操作：${name}`, exact: true }).click();
    await page.getByRole('menuitem', { name: action, exact: true }).click();
  }

  async function openFolder(name: string) {
    await page.locator('.listing-page').getByRole('button', { name, exact: true }).click();
    await manage();
    await expect(page.locator('.listing-heading h1')).toHaveText(name);
  }

  async function upload(name: string, data: Buffer) {
    const result = await mutation('POST', /^\/api\/files$/, async () => {
      const chooserPromise = page.waitForEvent('filechooser');
      await page.locator('.quick-actions').getByRole('button', { name: '上传', exact: true }).click();
      const chooser = await chooserPromise;
      await chooser.setFiles({ name, mimeType: 'text/plain', buffer: data });
    });
    await expect(page.getByRole('button', { name: `更多操作：${name}`, exact: true })).toBeVisible();
    return result.id as number;
  }

  async function download(name: string, data: Buffer) {
    const downloadPromise = page.waitForEvent('download');
    await menu(name, '下载');
    const result = await downloadPromise;
    expect(result.suggestedFilename()).toBe(name);
    const path = testInfo.outputPath(`download-${name}`);
    await result.saveAs(path);
    expect(await readFile(path)).toEqual(data);
  }

  async function trashList() {
    const responsePromise = page.waitForResponse(response =>
      response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/trash');
    await page.getByRole('button', { name: '回收站', exact: true }).click();
    const response = await responsePromise;
    expect(response.status()).toBe(200);
    await expect(page.locator('.listing-heading h1')).toHaveText('回收站');
    return response.json();
  }

  function trashRow(name: string) {
    return page.locator('.trash-row').filter({ has: page.locator('.row-name', { hasText: name }) });
  }

  async function remove(name: string, path: RegExp) {
    page.once('dialog', dialog => dialog.accept());
    await mutation('DELETE', path, () => menu(name, '移到回收站'), 204);
  }

  await test.step('register and log in against the C++ API', async () => {
    await page.goto('/');
    await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    await page.getByRole('button', { name: '注册账号', exact: true }).click();
    await page.getByLabel('用户名', { exact: true }).fill(username);
    await page.getByLabel('密码', { exact: true }).fill(password);
    await mutation('POST', /^\/register$/, () => page.getByRole('button', { name: '注册并进入', exact: true }).click());
    await expect(page.locator('.space-row')).toBeVisible();
    await expect(page.locator('.offline-preview-badge')).toHaveCount(0);
    await page.getByRole('button', { name: '退出登录', exact: true }).click();
    await page.getByRole('button', { name: '返回登录', exact: true }).click();
    await page.getByLabel('用户名', { exact: true }).fill(username);
    await page.getByLabel('密码', { exact: true }).fill(password);
    await mutation('POST', /^\/login$/, () => page.getByRole('button', { name: '登录', exact: true }).click());
    await expect(page.locator('.space-row')).toBeVisible();
    await manage();
  });

  const { aId, bId, fileId } = await test.step('create folders and upload, reload and download real bytes', async () => {
    const aId = await createFolder(a);
    const bId = await createFolder(b);
    const fileId = await upload(filename, content);
    await page.reload();
    await manage();
    await expect(page.getByRole('button', { name: `更多操作：${filename}`, exact: true })).toBeVisible();
    await download(filename, content);
    return { aId, bId, fileId };
  });

  await test.step('rename and move a file through the UI', async () => {
    await menu(filename, '重命名');
    const input = page.getByRole('textbox', { name: '名称', exact: true });
    await input.fill(renamedFile);
    await mutation('PATCH', new RegExp(`^/api/files/${fileId}$`), () => input.press('Enter'));
    await menu(renamedFile, '移动');
    const dialog = page.getByRole('dialog', { name: '移动 1 项', exact: true });
    await dialog.getByRole('button', { name: a, exact: true }).click();
    await mutation('PATCH', new RegExp(`^/api/files/${fileId}/move$`), () => dialog.getByRole('button', { name: '移动', exact: true }).click());
    await expect(dialog).toHaveCount(0);
    await openFolder(a);
    await download(renamedFile, content);
    await createFolder(child);
  });

  await test.step('handle a name conflict and move a directory with subtree guards', async () => {
    await root();
    await menu(a, '重命名');
    const input = page.getByRole('textbox', { name: '名称', exact: true });
    await input.fill(b);
    await mutation('PATCH', new RegExp(`^/api/folders/${aId}$`), () => input.press('Enter'), 409);
    await expect(page.locator('.inline-error')).toBeVisible();
    await expect(input).toBeEnabled();
    await input.fill(renamedA);
    await mutation('PATCH', new RegExp(`^/api/folders/${aId}$`), () => input.press('Enter'));
    await menu(renamedA, '移动');
    const dialog = page.getByRole('dialog', { name: '移动 1 项', exact: true });
    await expect(dialog.getByRole('button', { name: renamedA, exact: true })).toBeDisabled();
    const row = dialog.locator('.move-folder-row').filter({ has: page.getByRole('button', { name: renamedA, exact: true }) });
    await row.getByRole('button', { name: '展开', exact: true }).click();
    await expect(dialog.getByRole('button', { name: child, exact: true })).toBeDisabled();
    await dialog.getByRole('button', { name: b, exact: true }).click();
    await mutation('PATCH', new RegExp(`^/api/folders/${aId}/move$`), () => dialog.getByRole('button', { name: '移动', exact: true }).click());
    await expect(dialog).toHaveCount(0);
    await openFolder(b);
    await openFolder(renamedA);
    await expect(page.getByRole('button', { name: `更多操作：${renamedFile}`, exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath('real-api-file-management.png'), fullPage: true });
  });

  await test.step('search, trash, restore and permanently delete a file', async () => {
    await page.getByRole('button', { name: '搜索', exact: true }).click();
    await page.getByPlaceholder('搜索文件或文件夹').fill(renamedFile);
    await expect(page.getByRole('button', { name: `更多操作：${renamedFile}`, exact: true })).toBeVisible();
    await remove(renamedFile, new RegExp(`^/api/files/${fileId}$`));
    let list = await trashList();
    let entry = list.items.find((item: { type: string; resource_id: number }) => item.type === 'file' && item.resource_id === fileId);
    expect(entry).toBeTruthy();
    await mutation('POST', new RegExp(`^/api/trash/${entry.id}/restore$`), () => trashRow(renamedFile).getByRole('button', { name: '恢复', exact: true }).click(), 204);
    await expect(trashRow(renamedFile)).toHaveCount(0);
    await page.getByRole('button', { name: '搜索', exact: true }).click();
    await expect(page.getByRole('button', { name: `更多操作：${renamedFile}`, exact: true })).toBeVisible();
    await remove(renamedFile, new RegExp(`^/api/files/${fileId}$`));
    list = await trashList();
    entry = list.items.find((item: { type: string; resource_id: number }) => item.type === 'file' && item.resource_id === fileId);
    page.once('dialog', dialog => dialog.accept());
    await mutation('DELETE', new RegExp(`^/api/trash/${entry.id}$`), () => trashRow(renamedFile).getByRole('button', { name: '永久删除', exact: true }).click(), 204);
    await expect(trashRow(renamedFile)).toHaveCount(0);
  });

  await test.step('recursively restore and permanently delete a directory tree', async () => {
    await root();
    await openFolder(b);
    await openFolder(renamedA);
    const nestedContent = Buffer.from('目录树恢复后的真实文件内容');
    await upload('nested.txt', nestedContent);
    await root();
    await remove(b, new RegExp(`^/api/folders/${bId}$`));
    let list = await trashList();
    let entry = list.items.find((item: { type: string; resource_id: number }) => item.type === 'folder' && item.resource_id === bId);
    await mutation('POST', new RegExp(`^/api/trash/${entry.id}/restore$`), () => trashRow(b).getByRole('button', { name: '恢复', exact: true }).click(), 204);
    await page.getByRole('button', { name: '文件', exact: true }).click();
    await root();
    await openFolder(b);
    await openFolder(renamedA);
    await download('nested.txt', nestedContent);
    await root();
    await remove(b, new RegExp(`^/api/folders/${bId}$`));
    list = await trashList();
    entry = list.items.find((item: { type: string; resource_id: number }) => item.type === 'folder' && item.resource_id === bId);
    page.once('dialog', dialog => dialog.accept());
    await mutation('DELETE', new RegExp(`^/api/trash/${entry.id}$`), () => trashRow(b).getByRole('button', { name: '永久删除', exact: true }).click(), 204);
    await page.getByRole('button', { name: '文件', exact: true }).click();
    await root();
    await page.reload();
    await manage();
    await expect(page.locator('.file-list-row')).toHaveCount(0);
  });

  expect(pageErrors).toEqual([]);
  expect(calls.filter(call => call.status >= 500)).toEqual([]);
  await testInfo.attach('real-api-requests.json', { body: JSON.stringify(calls, null, 2), contentType: 'application/json' });
});
