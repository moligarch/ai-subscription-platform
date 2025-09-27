<script lang="ts">
  import { onMount } from "svelte";

  type VerifyResp = { result: "ok" | "fail"; reason?: string; bot?: string };

  let statusText = "در حال بررسی پرداخت...";
  let ok = false;
  let bot = "";
  let reason = "";

  function parseParams(): URLSearchParams {
    // Prefer hash query: #/payment-result?Authority=...&Status=OK
    const h = window.location.hash || "";
    const qi = h.indexOf("?");
    if (qi >= 0) return new URLSearchParams(h.substring(qi + 1));

    // Fallback to normal querystring: ?Authority=...&Status=...
    return new URLSearchParams(window.location.search || "");
  }

  async function verify() {
    const params = parseParams();
    const authority = params.get("Authority") || "";
    const status = params.get("Status") || "";

    try {
      const res = await fetch("/api/v1/payment/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ authority, status })
      });
      const data = (await res.json()) as VerifyResp;

      ok = data.result === "ok";
      bot = data.bot ?? "";
      if (ok) {
        statusText = "✅ پرداخت شما تایید شد. پلن شما فعال شد.";
      } else {
        reason = data.reason || "پرداخت تایید نشد.";
        statusText = "❌ پرداخت ناموفق";
      }
    } catch (e) {
      ok = false;
      reason = "خطای شبکه";
      statusText = "❌ پرداخت ناموفق";
    }
  }

  function openTelegram() {
    const u = bot ? `https://t.me/${bot}` : "https://t.me";
    window.location.href = u;
  }

  onMount(verify);
</script>

<style>
  :global(body) {
    font-family: system-ui, -apple-system, Segoe UI, Roboto, Ubuntu, Helvetica, Arial, sans-serif;
  }
  .card {
    max-width: 560px; margin: 40px auto; border:1px solid #e7ecf3; border-radius: 16px;
    padding: 24px; box-shadow: 0 4px 18px rgba(0,0,0,.05); background:#fff; color: #0b1320;
  }
  .btn { display:inline-block; margin-top:12px; padding:10px 14px; border-radius:10px; border:1px solid #1b74e4; cursor:pointer; }
  .btn:hover { background:#f5faff }
  .small { color:#475569; margin-top:6px }
  .reason { color:#9a1c1f; margin-top:8px }
</style>

<div class="card" dir="rtl">
  <h2>{statusText}</h2>
  {#if !ok && reason}<p class="reason">علت: {reason}</p>{/if}
  <button class="btn" on:click={openTelegram}>بازگشت به تلگرام</button>
  {#if bot}<p class="small">در صورت عدم باز شدن، در تلگرام <b>@{bot}</b> را جستجو کنید.</p>{/if}
</div>
