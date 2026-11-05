/* Shared lifecycle only; product interactions stay in app.js. */
(() => {
  const sync = () => { document.body.dataset.pageHidden = String(document.hidden); };
  document.addEventListener('visibilitychange', sync); sync();
})();
