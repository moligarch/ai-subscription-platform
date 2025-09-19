const KEY = 'ADMIN_API_KEY';

export function setApiKey(k: string) {
  sessionStorage.setItem(KEY, k);
}

export function getApiKey(): string | null {
  return sessionStorage.getItem(KEY);
}

export function clearApiKey() {
  sessionStorage.removeItem(KEY);
}
