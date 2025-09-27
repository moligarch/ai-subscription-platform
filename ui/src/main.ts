// ui/src/main.ts
import App from './App.svelte';
import PaymentResult from './pages/PaymentResult.svelte';
import './styles/index.css';

function isPaymentResult(): boolean {
  const h = window.location.hash || '';
  if (h.startsWith('#/payment-result')) return true;
  const sp = new URLSearchParams(window.location.search);
  return !!sp.get('Authority') && !!sp.get('Status');
}

let app: any = null;
let showingPayment = false;

function mount() {
  const target = document.getElementById('app')!;
  const wantPayment = isPaymentResult();

  if (app) {
    // If the “type” (payment vs app) didn’t change, do nothing
    if (wantPayment === showingPayment) return;
    app.$destroy();
    target.innerHTML = '';
    app = null;
  }

  showingPayment = wantPayment;
  app = wantPayment ? new PaymentResult({ target }) : new App({ target });
}

// Only re-mount when the “kind of page” changes
window.addEventListener('hashchange', mount);
window.addEventListener('popstate', mount);

mount();
export default app;
