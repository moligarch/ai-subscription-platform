<script lang="ts">
  import { onMount } from 'svelte';
  import { get, post, del } from '../lib/api';

  type PlanServer = {
    ID: string;
    Name: string;
    DurationDays: number;
    Credits: number;
    PriceIRR: number;
    SupportedModels: string[];
    CreatedAt: string;
  };
  type PlansListResponse = { data: PlanServer[] };

  type PlanUI = {
    id: string;
    name: string;
    duration_days: number;
    credits: number;
    price_irr: number;
    supported_models: string[];
    created_at: string;
  };

  type GenResp = {
    batch_id: string;
    codes: { code: string; expires_at?: string|null }[];
  };

  let plans: PlanUI[] = [];
  let loading = false;
  let refreshing = false;
  let error = '';

  // create form
  let showCreate = false;
  let creating = false;
  let name = '';
  let duration_days: number = 30;
  let credits: number = 0;
  let price_irr: number = 0;
  let supported_models_str = '';

  // toast
  type Toast = { id: number; kind: 'success'|'error'; text: string };
  let toasts: Toast[] = [];
  let nextToastId = 1;
  function toast(text: string, kind: Toast['kind']='success', ttl=3500) {
    const t = { id: nextToastId++, kind, text };
    toasts = [...toasts, t];
    setTimeout(() => toasts = toasts.filter(x => x.id !== t.id), ttl);
  }
  function closeToast(id: number) { toasts = toasts.filter(x => x.id !== id); }

  function mapServerToUI(p: PlanServer): PlanUI {
    return {
      id: p.ID,
      name: p.Name,
      duration_days: p.DurationDays,
      credits: p.Credits,
      price_irr: p.PriceIRR,
      supported_models: p.SupportedModels || [],
      created_at: p.CreatedAt
    };
  }
  function parseModels(s: string): string[] {
    return s.split(',').map(x => x.trim()).filter(Boolean);
  }
  const fmtIRR = (n:number)=> new Intl.NumberFormat('fa-IR').format(Number.isFinite(n)?n:0)+' ریال';

  async function loadPlans() {
    loading = true; error='';
    try {
      const res = await get<PlansListResponse>('/api/v1/plans');
      plans = Array.isArray(res?.data) ? res.data.map(mapServerToUI) : [];
    } catch (e:any) {
      error = e?.message || 'Failed to load plans';
      plans = []; toast(error, 'error');
    } finally {
      loading = false;
    }
  }
  async function refresh() { refreshing=true; try{await loadPlans();}finally{refreshing=false;} }

  async function createPlan() {
    creating = true; error='';
    try {
      await post('/api/v1/plans', {
        name, duration_days, credits, price_irr,
        supported_models: parseModels(supported_models_str),
      });
      toast('Plan created','success');
      name=''; duration_days=30; credits=0; price_irr=0; supported_models_str='';
      showCreate = false;
      await loadPlans();
    } catch (e:any) {
      toast(e?.message || 'Failed to create plan','error');
    } finally {
      creating = false;
    }
  }
  async function deletePlan(id: string) {
    if (!confirm('Delete this plan?')) return;
    try { await del(`/api/v1/plans/${encodeURIComponent(id)}`); toast('Plan deleted'); await loadPlans(); }
    catch(e:any){ toast(e?.message || 'Failed to delete plan','error'); }
  }

  // ---------- Activation Codes ----------
  let genOpenFor: string|undefined;
  let gen_count = 1;
  let gen_expires = ''; // ISO date (optional)
  let gen_loading = false;
  let gen_result: GenResp|null = null;

  function openGen(planId: string) {
    genOpenFor = planId;
    gen_count = 1;
    gen_expires = '';
    gen_result = null;
  }
  function closeGen() { genOpenFor = undefined; gen_result = null; }

  async function generateCodes(planId: string) {
    gen_loading = true; gen_result = null;
    try {
      const payload: any = { plan_id: planId, count: Number(gen_count)||1 };
      if (gen_expires.trim()) payload.expires_at = new Date(gen_expires).toISOString();
      const resp = await post<GenResp>('/api/v1/activation-codes/generate', payload);
      gen_result = resp;
      toast(`Generated ${resp?.codes?.length || 0} codes`, 'success');
    } catch (e:any) {
      toast(e?.message || 'Failed to generate codes','error');
    } finally {
      gen_loading = false;
    }
  }

  function copyCodes() {
    if (!gen_result?.codes?.length) return;
    const txt = gen_result.codes.map(c => c.code).join('\n');
    navigator.clipboard.writeText(txt).then(()=>toast('Codes copied'),'error');
  }
  function downloadCSV() {
    if (!gen_result?.codes?.length) return;
    const rows = [['code','expires_at'], ...gen_result.codes.map(c => [c.code, c.expires_at ?? ''])];
    const csv = rows.map(r => r.map(v => `"${String(v).replace(/"/g,'""')}"`).join(',')).join('\n');
    const blob = new Blob([csv], {type:'text/csv;charset=utf-8;'});
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `activation-codes-${gen_result.batch_id}.csv`;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  onMount(loadPlans);
</script>

<!-- toasts -->
<div class="fixed top-4 right-4 z-50 space-y-2">
  {#each toasts as t (t.id)}
    <div class="rounded-md shadow px-4 py-3 text-sm text-white flex items-start gap-3"
      style="background:{t.kind==='success' ? '#16a34a' : '#dc2626'}">
      <div class="font-semibold">{t.kind==='success'?'Success':'Error'}</div>
      <div class="opacity-95">{t.text}</div>
      <button class="ml-auto opacity-70 hover:opacity-100" on:click={() => closeToast(t.id)}>✕</button>
    </div>
  {/each}
</div>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h2 class="text-2xl font-bold text-gray-800">Plans</h2>
    <div class="flex items-center gap-2">
      <button class="text-sm text-blue-600 hover:text-blue-800" on:click={refresh} disabled={refreshing||loading}>
        {refreshing?'Refreshing…':'Refresh'}
      </button>
      <button class="bg-blue-600 hover:bg-blue-700 text-white text-sm px-4 py-2 rounded"
        on:click={() => showCreate = !showCreate}>{showCreate? 'Cancel':'New Plan'}</button>
    </div>
  </div>

  {#if showCreate}
    <div class="bg-white rounded-lg shadow p-4">
      <h3 class="text-lg font-semibold text-gray-700 mb-3">Create New Plan</h3>
      <form class="grid md:grid-cols-3 gap-4" on:submit|preventDefault={createPlan}>
        <div>
          <label for="plan-name" class="text-sm text-gray-600">Name</label>
          <input id="plan-name" class="w-full border rounded px-3 py-2" bind:value={name} />
        </div>
        <div>
          <label for="plan-duration" class="text-sm text-gray-600">Duration (days)</label>
          <input id="plan-duration" type="number" class="w-full border rounded px-3 py-2" bind:value={duration_days} />
        </div>
        <div>
          <label for="plan-credits" class="text-sm text-gray-600">Credits</label>
          <input id="plan-credits" type="number" class="w-full border rounded px-3 py-2" bind:value={credits} />
        </div>
        <div>
          <label for="plan-price" class="text-sm text-gray-600">Price (IRR)</label>
          <input id="plan-price" type="number" class="w-full border rounded px-3 py-2" bind:value={price_irr} />
        </div>
        <div class="md:col-span-2">
          <label for="plan-models" class="text-sm text-gray-600">Supported Models (comma separated)</label>
          <input id="plan-models" class="w-full border rounded px-3 py-2"
            bind:value={supported_models_str} placeholder="gpt-4o, gpt-4o-mini" />
        </div>
        <div class="md:col-span-3">
          <button type="submit" class="bg-blue-600 hover:bg-blue-700 text-white text-sm px-4 py-2 rounded disabled:opacity-60"
            disabled={creating}>{creating? 'Creating…':'Create Plan'}</button>
        </div>
      </form>
    </div>
  {/if}

  <div class="bg-white rounded-lg shadow p-4">
    <div class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead class="bg-gray-50 text-gray-600">
          <tr>
            <th class="text-left px-3 py-2">Name</th>
            <th class="text-left px-3 py-2">Duration</th>
            <th class="text-right px-3 py-2">Credits</th>
            <th class="text-right px-3 py-2">Price</th>
            <th class="text-left px-3 py-2">Models</th>
            <th class="text-right px-3 py-2">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#if loading}
            <tr><td class="px-3 py-3 text-gray-500" colspan="6">Loading…</td></tr>
          {:else if plans.length === 0}
            <tr><td class="px-3 py-3 text-gray-500" colspan="6">No plans found</td></tr>
          {:else}
            {#each plans as p}
              <tr class="border-t align-top">
                <td class="px-3 py-2 font-medium text-gray-800">{p.name}</td>
                <td class="px-3 py-2 text-gray-700">{p.duration_days} days</td>
                <td class="px-3 py-2 text-right text-gray-700">{p.credits.toLocaleString()}</td>
                <td class="px-3 py-2 text-right text-gray-700">{fmtIRR(p.price_irr)}</td>
                <td class="px-3 py-2 text-gray-700">
                  {#if p.supported_models?.length}
                    <span class="inline-flex flex-wrap gap-1">
                      {#each p.supported_models as m}
                        <span class="px-2 py-0.5 bg-gray-100 rounded text-gray-700">{m}</span>
                      {/each}
                    </span>
                  {:else}
                    <span class="text-gray-400">—</span>
                  {/if}
                </td>
                <td class="px-3 py-2 text-right space-x-3">
                  <button class="text-green-700 hover:text-green-900" on:click={() => openGen(p.id)}>Generate Codes</button>
                  <button class="text-red-600 hover:text-red-800" on:click={() => deletePlan(p.id)}>Delete</button>
                </td>
              </tr>

              {#if genOpenFor === p.id}
                <tr class="border-b">
                  <td class="px-3 pb-3" colspan="6">
                    <div class="rounded-md border p-3 bg-gray-50">
                      <div class="text-sm font-semibold mb-2">Generate Activation Codes for <span class="text-blue-700">{p.name}</span></div>
                      <div class="grid md:grid-cols-3 gap-3 items-end">
                        <div>
                          <label for="gen-count" class="text-sm text-gray-600">Count</label>
                          <input id="gen-count" type="number" min="1" class="w-full border rounded px-3 py-2"
                            bind:value={gen_count} />
                        </div>
                        <div>
                          <label for="gen-exp" class="text-sm text-gray-600">
                            Expires at (optional, local date/time)
                          </label>
                          <input id="gen-exp" type="datetime-local" class="w-full border rounded px-3 py-2"
                            bind:value={gen_expires} />
                        </div>
                        <div class="flex gap-2">
                          <button class="bg-green-600 hover:bg-green-700 text-white text-sm px-4 py-2 rounded disabled:opacity-60"
                            on:click={() => generateCodes(p.id)} disabled={gen_loading}>
                            {gen_loading ? 'Generating…' : 'Generate'}
                          </button>
                          <button class="text-gray-600 hover:text-gray-800 text-sm" on:click={closeGen}>Close</button>
                        </div>
                      </div>

                      {#if gen_result}
                        <div class="mt-3">
                          <div class="flex items-center justify-between">
                            <div class="text-sm text-gray-700">Batch: <code>{gen_result.batch_id}</code></div>
                            <div class="space-x-2">
                              <button class="text-sm text-blue-600 hover:text-blue-800" on:click={copyCodes}>Copy</button>
                              <button class="text-sm text-blue-600 hover:text-blue-800" on:click={downloadCSV}>Download CSV</button>
                            </div>
                          </div>
                          <div class="overflow-x-auto mt-2">
                            <table class="min-w-full text-xs">
                              <thead class="bg-gray-100">
                                <tr><th class="text-left px-2 py-1">Code</th><th class="text-left px-2 py-1">Expires</th></tr>
                              </thead>
                              <tbody>
                                {#each gen_result.codes as c}
                                  <tr class="border-t">
                                    <td class="px-2 py-1 font-mono">{c.code}</td>
                                    <td class="px-2 py-1">{c.expires_at ? new Date(c.expires_at).toLocaleString() : '—'}</td>
                                  </tr>
                                {/each}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </div>
</div>

<style>
  button[disabled]{opacity:.6;cursor:not-allowed;}
</style>
