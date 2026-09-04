// Follow CPA's resolved DOM theme, not a separate saved preference.
(() => {
  const root = document.documentElement;
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  let host;
  try {
    if (window.parent !== window) host = window.parent.document.documentElement;
  } catch (_) {
    // Cross-origin embedding cannot expose the host DOM; use system preference.
  }
  const sync = () => {
    const dark = host ? host.getAttribute('data-theme') === 'dark' : system.matches;
    root.dataset.theme = dark ? 'dark' : 'light';
    root.style.colorScheme = dark ? 'dark' : 'light';
  };
  sync();
  if (host) new MutationObserver(sync).observe(host, {attributes: true, attributeFilter: ['data-theme']});
  else system.addEventListener('change', sync);
})();
