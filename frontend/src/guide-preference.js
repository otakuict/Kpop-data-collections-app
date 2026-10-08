const hiddenKey = 'bias.quick-guide.hidden.v1';

export function shouldShowGuide(getStorage = () => window.localStorage) {
 try { return getStorage().getItem(hiddenKey) !== '1'; }
 catch { return true; }
}

export function saveGuidePreference(hidden, getStorage = () => window.localStorage) {
 try {
  const storage = getStorage();
  if (hidden) storage.setItem(hiddenKey, '1');
  else storage.removeItem(hiddenKey);
 } catch { /* The guide can still be dismissed when browser storage is unavailable. */ }
}
