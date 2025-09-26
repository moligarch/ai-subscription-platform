import App from './App.svelte';
import PaymentResult from './pages/PaymentResult.svelte';
import './styles/index.css';

function shouldShowPaymentResult(): boolean {
  const h = window.location.hash || '';
  if (h.startsWith('#/payment-result')) return true;

  // Fallback: some gateways ignore fragments and redirect to /?Authority=...&Status=...
  const sp = new URLSearchParams(window.location.search);
  return !!sp.get('Authority') && !!sp.get('Status');
}

let app: App | PaymentResult | null = null;

function mount() {
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  const target = document.getElementById('app')!;
  if (shouldShowPaymentResult()) {
    app = new PaymentResult({ target });
  } else {
    app = new App({ target });
  }
}

window.addEventListener('hashchange', () => {
  // naive re-mount on hash change (sufficient for this small app)
  mount();
});
window.addEventListener('popstate', () => {
  mount();
});

mount();
export default app;
