const { ipcRenderer } = require('electron');

// Only this packaged local document uses the preload; expose nothing on window.
window.addEventListener('DOMContentLoaded', () => {
  for (const action of ['minimize', 'maximize', 'close']) {
    document.getElementById(action).addEventListener('click', () => {
      ipcRenderer.send('window-controls:action', action);
    });
  }
});

ipcRenderer.on('window-controls:state', (_event, maximized) => {
  document.body.classList.toggle('maximized', maximized);
  const button = document.getElementById('maximize');
  const label = maximized ? 'Восстановить размер' : 'Развернуть';
  button.title = label;
  button.setAttribute('aria-label', label);
});
