const { WebContentsView, ipcMain, dialog } = require('electron');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { isControlsSender } = require('./policy.cjs');

function attachWindowControls(window, workspaceView) {
  // Keep native window actions in a local view; the remote site receives no IPC bridge.
  const url = pathToFileURL(path.join(__dirname, 'controls/index.html')).href;
  const view = new WebContentsView({
    webPreferences: {
      preload: path.join(__dirname, 'controls/preload.cjs'),
      partition: 'window-controls',
      nodeIntegration: false, contextIsolation: true, sandbox: true, webSecurity: true,
    },
  });
  const contents = view.webContents;
  const topInset = process.platform === 'linux' ? 40 : 0;
  view.setBackgroundColor(topInset ? '#f8f8f6' : '#00000000');
  window.contentView.addChildView(view);
  const layout = () => {
    const { width, height } = window.getContentBounds();
    // Reserve real space so fixed headers and dialogs cannot slip under the controls.
    workspaceView.setBounds({ x: 0, y: topInset, width, height: Math.max(0, height - topInset) });
    view.setBounds(topInset
      ? { x: 0, y: 0, width, height: topInset }
      : { x: Math.max(0, width - 128), y: 0, width: 120, height: 32 });
  };
  const updateState = () => {
    if (!contents.isDestroyed()) contents.send('window-controls:state', window.isMaximized());
  };
  const onAction = (event, action) => {
    if (window.isDestroyed() || !isControlsSender(event, contents, url)) return;
    if (action === 'minimize') window.minimize();
    if (action === 'maximize') {
      if (window.isMaximized()) window.unmaximize(); else window.maximize();
    }
    if (action === 'close') window.close();
  };
  ipcMain.on('window-controls:action', onAction);
  contents.on('will-navigate', event => event.preventDefault());
  contents.on('will-redirect', event => event.preventDefault());
  contents.setWindowOpenHandler(() => ({ action: 'deny' }));
  contents.session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
  contents.session.setPermissionCheckHandler(() => false);
  contents.on('did-finish-load', updateState);
  window.on('resize', layout);
  window.on('maximize', updateState);
  window.on('unmaximize', updateState);
  window.on('closed', () => {
    ipcMain.removeListener('window-controls:action', onAction);
    if (!contents.isDestroyed()) contents.close();
  });
  layout();
  void contents.loadURL(url).catch(() => {
    if (!window.isDestroyed()) {
      dialog.showErrorBox('Ошибка интерфейса', 'Не удалось загрузить кнопки окна. Переустановите приложение.');
      window.close();
    }
  });
  return contents;
}

module.exports = { attachWindowControls };
