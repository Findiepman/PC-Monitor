export type Theme = 'light' | 'dark';

const KEY = 'dashd:theme';
const EVENT = 'dashd:theme';

function stored(): Theme | null {
  try {
    const v = localStorage.getItem(KEY);
    return v === 'light' || v === 'dark' ? v : null;
  } catch {
    return null;
  }
}

export function currentTheme(): Theme {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
}

function apply(t: Theme) {
  document.documentElement.dataset.theme = t;
  window.dispatchEvent(new Event(EVENT));
}

/** Saved choice first, else the system setting (which keeps being followed until a choice is saved). */
export function initTheme() {
  const mq = matchMedia('(prefers-color-scheme: dark)');
  apply(stored() ?? (mq.matches ? 'dark' : 'light'));
  mq.addEventListener('change', () => {
    if (!stored()) apply(mq.matches ? 'dark' : 'light');
  });
}

export function setTheme(t: Theme) {
  try {
    localStorage.setItem(KEY, t);
  } catch {
    // Storage unavailable; the choice just won't persist.
  }
  apply(t);
}

export function onThemeChange(fn: () => void) {
  window.addEventListener(EVENT, fn);
  return () => window.removeEventListener(EVENT, fn);
}
