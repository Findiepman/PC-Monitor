// Sets the theme before first paint (saved choice, else the system setting).
// A separate file because dashd's CSP (default-src 'self') blocks inline scripts.
(function () {
  var t = null;
  try {
    t = localStorage.getItem('dashd:theme');
  } catch (e) {}
  if (t !== 'light' && t !== 'dark') t = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  document.documentElement.dataset.theme = t;
})();
