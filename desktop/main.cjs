const { app, BaseWindow, WebContentsView, Menu, dialog, session, shell } = require('electron');
const path = require('node:path');
const { serverURL, isAppURL, navigationKind } = require('./policy.cjs');
const { attachWindowControls } = require('./window-controls.cjs');

app.setName('identity workspace');
app.enableSandbox();
const origin = serverURL(process.env.IDENTITY_WORKSPACE_URL || 'http://localhost:8080');
let mainWindow;
let externalPromptOpen = false;
const preferences = { nodeIntegration: false, contextIsolation: true, sandbox: true, webSecurity: true };

async function openExternal(url) {
  if (navigationKind(url, origin) === 'blocked' || externalPromptOpen) return;
  externalPromptOpen = true;
  try {
    const { response } = await dialog.showMessageBox({
      type: 'question', title: 'Внешняя ссылка',
      message: `Открыть ${new URL(url).origin} в браузере?`,
      buttons: ['Отмена', 'Открыть'], defaultId: 0, cancelId: 0,
    });
    if (response === 1) await shell.openExternal(url);
  } catch {
    dialog.showErrorBox('Не удалось открыть браузер', 'Откройте сайт в браузере вручную.');
  } finally {
    externalPromptOpen = false;
  }
}

function showWorkspace() {
  const workspaceOrigin = origin;
  const workspaceSession = session.fromPartition('persist:workspace');
  const grants = new Set();
  workspaceSession.setPermissionCheckHandler((contents, permission, requestingOrigin, details) =>
    Boolean(contents && isAppURL(contents.getURL(), workspaceOrigin) &&
      details.isMainFrame && isAppURL(requestingOrigin, workspaceOrigin) && grants.has(permission) &&
      (permission !== 'media' || details.mediaType === 'video')));
  workspaceSession.setPermissionRequestHandler(async (contents, permission, callback, details) => {
    const names = { notifications: 'уведомления', media: 'доступ к камере' };
    if (!contents || !isAppURL(contents.getURL(), workspaceOrigin) || !details.isMainFrame ||
        !isAppURL(details.requestingUrl, workspaceOrigin) || !names[permission] ||
        (permission === 'media' && (!details.mediaTypes?.length || details.mediaTypes.some((type) => type !== 'video')))) {
      callback(false);
      return;
    }
    try {
      const { response } = await dialog.showMessageBox({
        type: 'question', title: 'Разрешение', message: `Разрешить ${names[permission]} для ${workspaceOrigin}?`,
        buttons: ['Запретить', 'Разрешить'], defaultId: 0, cancelId: 0,
      });
      const allowed = response === 1 && !contents.isDestroyed() && isAppURL(contents.getURL(), workspaceOrigin);
      if (allowed) grants.add(permission);
      callback(allowed);
    } catch { callback(false); }
  });
  const window = new BaseWindow({
    width: 1200, height: 850, minWidth: 390, minHeight: 560,
    frame: false,
    title: 'identity workspace', backgroundColor: '#12161d',
    icon: app.isPackaged ? path.join(process.resourcesPath, 'icon.png') :
      path.join(__dirname, 'assets/icon.png'),
  });
  const workspaceView = new WebContentsView({ webPreferences: { ...preferences, session: workspaceSession } });
  const contents = workspaceView.webContents;
  window.contentView.addChildView(workspaceView);
  mainWindow = window;
  const controlContents = attachWindowControls(window, workspaceView);
  if (process.platform !== 'darwin') window.removeMenu();
  // Keep common shortcuts available after removing the application menu (including macOS).
  const handleShortcut = (event, input) => {
    if (input.type !== 'keyDown' || input.alt || !(process.platform === 'darwin' ? input.meta : input.control)) return;
    const actions = {
      a: () => contents.selectAll(), c: () => contents.copy(), x: () => contents.cut(),
      v: () => contents.paste(), z: () => input.shift ? contents.redo() : contents.undo(),
      r: () => contents.reload(), q: () => app.quit(), w: () => window.close(),
    };
    const action = actions[input.key.toLowerCase()];
    if (action) { event.preventDefault(); action(); }
  };
  contents.on('before-input-event', handleShortcut);
  controlContents.on('before-input-event', handleShortcut);
  window.on('closed', () => {
    if (!contents.isDestroyed()) contents.close();
    if (mainWindow === window) mainWindow = null;
  });
  const guardNavigation = (event) => {
    const target = event.url;
    const kind = navigationKind(target, workspaceOrigin);
    if (kind === 'internal') return;
    event.preventDefault();
    if (kind === 'external') void openExternal(target);
  };
  contents.on('will-navigate', guardNavigation);
  contents.on('will-redirect', guardNavigation);
  contents.setWindowOpenHandler(({ url: target }) => {
    if (navigationKind(target, workspaceOrigin) === 'internal') void contents.loadURL(target).catch(() => {});
    else void openExternal(target);
    return { action: 'deny' };
  });
  contents.on('page-title-updated', (event) => {
    event.preventDefault();
    const current = contents.getURL();
    window.setTitle(isAppURL(current, workspaceOrigin) ? 'identity workspace' :
      `identity workspace — ${new URL(current).hostname}`);
  });
  let errorVisible = false;
  contents.on('did-fail-load', async (_event, code, _description, _url, isMainFrame) => {
    if (!isMainFrame || code === -3 || errorVisible || window.isDestroyed()) return;
    errorVisible = true;
    try {
      const { response } = await dialog.showMessageBox(window, {
        type: 'warning', title: 'Сервер недоступен',
        message: 'Не удалось открыть приложение.',
        detail: `Проверьте соединение с интернетом. Сервер ${workspaceOrigin} должен быть доступен.`,
        buttons: ['Повторить', 'Закрыть'], cancelId: 1,
      });
      errorVisible = false;
      if (window.isDestroyed()) return;
      if (response === 0) void contents.loadURL(workspaceOrigin).catch(() => {});
      if (response === 1) window.close();
    } catch {
      if (!window.isDestroyed()) window.close();
    } finally { errorVisible = false; }
  });
  void contents.loadURL(workspaceOrigin).catch(() => {});
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    const window = mainWindow;
    if (window) { if (window.isMinimized()) window.restore(); window.show(); window.focus(); }
  });
  app.whenReady().then(() => {
    Menu.setApplicationMenu(null);
    showWorkspace();
    app.on('activate', () => { if (BaseWindow.getAllWindows().length === 0) showWorkspace(); });
  }).catch(() => { dialog.showErrorBox('Ошибка запуска', 'Не удалось запустить identity workspace.'); app.quit(); });
  app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit(); });
}
