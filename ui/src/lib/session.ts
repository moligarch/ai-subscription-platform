// ui/src/lib/session.ts
// Session flag for client-side route guarding (cookie does the real auth)
const AUTH_FLAG = 'ADMIN_AUTH';

export function setAuthenticated(v: boolean) {
  if (v) sessionStorage.setItem(AUTH_FLAG, '1');
  else sessionStorage.removeItem(AUTH_FLAG);
}

export function isAuthenticated(): boolean {
  return sessionStorage.getItem(AUTH_FLAG) === '1';
}

export function clearAuthentication() {
  sessionStorage.removeItem(AUTH_FLAG);
}
