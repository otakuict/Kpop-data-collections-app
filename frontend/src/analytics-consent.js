export const consentKey = 'bias.analytics-consent.v1';
export const consentLifetime = 180 * 24 * 60 * 60 * 1000;

export function readAnalyticsConsent(getStorage = () => window.localStorage, now = Date.now()) {
 try {
  const saved = JSON.parse(getStorage().getItem(consentKey));
  if (!saved || !['accepted', 'rejected'].includes(saved.choice)) return null;
  if (!Number.isFinite(saved.expiresAt) || saved.expiresAt <= now || saved.expiresAt > now + consentLifetime) return null;
  return saved.choice;
 } catch { return null; }
}

export function saveAnalyticsConsent(choice, getStorage = () => window.localStorage, now = Date.now()) {
 try {
  getStorage().setItem(consentKey, JSON.stringify({choice, expiresAt: now + consentLifetime}));
 } catch { /* An explicit choice still applies to the current page when storage is blocked. */ }
}
