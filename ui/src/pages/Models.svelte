<script lang="ts">
  import { onMount } from 'svelte';
  import { get, post, put, del } from '../lib/api';

  // OpenAPI /apiv1 returns snake_case for models
  type ModelDTO = {
    name: string;
    input_price_micros: number;
    output_price_micros: number;
    currency: string; // "IRR"
    updated_at?: string;
  };
  type ListResp = { items?: ModelDTO[] };

  let models: ModelDTO[] = [];
  let loading = false;
  let refreshing = false;
  let error = '';

  // Create form state
  let createOpen = false;
  let c_name = '';
  let c_input = 0;
  let c_output = 0;

  // Toasts
  type Toast = { id: number; kind: 'success'|'error'; text: string };
  let toasts: Toast[] = [];
  let nextToastId = 1;
  function toast(text: string, kind: Toast['kind']='success', ttl=3500) {
    const t = { id: nextToastId++, kind, text };
    toasts = [...toasts, t];
    setTimeout(() => toasts = toasts.filter(x => x.id !== t.id), ttl);
  }
  function closeToast(id: number) { toasts = toasts.filter(x => x.id !== id); }

  async function load() {
    loading = true; error = '';
    try {
      const resp = await get<ListResp>('/api/v1/models');
      models = Array.isArray(resp?.items) ? resp.items! : [];
    } catch (e:any) {
      error = e?.message || 'Failed to load models';
      models = [];
      toast(error, 'error');
    } finally {
      loading = false;
    }
  }

  async function refresh() {
    refreshing = true;
    try { await load(); } finally { refreshing = false; }
  }

  async function createModel() {
    if (!c_name.trim()) { toast('Name is required', 'error'); return; }
    try {
      await post('/api/v1/models', {
        name: c_name.trim(),
        input_price_micros: Number(c_input)||0,
        output_price_micros: Number(c_output)||0,
        currency: 'IRR',
      });
      toast('Model created', 'success');
      c_name=''; c_input=0; c_output=0; createOpen=false;
      await load();
    } catch(e:any) {
      toast(e?.message || 'Create failed', 'error');
    }
  }

  // Per-row edit state
  let editKey: string|undefined;
  let e_input = 0;
  let e_output = 0;
  function startEdit(m: ModelDTO) {
    editKey = m.name;
    e_input = m.input_price_micros;
    e_output = m.output_price_micros;
  }
  function cancelEdit() { editKey = undefined; }

  async function saveEdit(name: string) {
    try {
      await put(`/api/v1/models/${encodeURIComponent(name)}`, {
        input_price_micros: Number(e_input)||0,
        output_price_micros: Number(e_output)||0,
        currency: 'IRR',
      });
      toast('Model updated', 'success');
      editKey = undefined;
      await load();
    } catch(e:any) {
      toast(e?.message || 'Update failed', 'error');
    }
  }

  async function remove(name: string) {
    if (!confirm(`Delete model "${name}"?`)) return;
    try {
      await del(`/api/v1/models/${encodeURIComponent(name)}`);
      toast('Model deleted', 'success');
      await load();
    } catch(e:any) {
      toast(e?.message || 'Delete failed', 'error');
    }
  }

  onMount(load);
</script>

<!-- toasts -->
<div class="fixed top-4 right-4 z-50 space-y-2">
  {#each toasts as t (t.id)}
    <div class="rounded-md shadow px-4 py-3 text-sm text-white flex items-start gap-3"
      style="background:{t.kind==='success' ? '#16a34a' : '#dc2626'}">
      <div class="font-semibold">{t.kind==='success' ? 'Success' : 'Error'}</div>
      <div class="opacity-95">{t.text}</div>
      <button class="ml-auto opacity-70 hover:opacity-100" on:click={() => closeToast(t.id)}>✕</button>
    </div>
  {/each}
</div>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h2 class="text-2xl font-bold text-gray-800">Models</h2>
    <div class="flex items-center gap-2">
      <button class="text-sm text-blue-600 hover:text-blue-800" on:click={refresh} disabled={refreshing||loading}>
        {refreshing ? 'Refreshing…' : 'Refresh'}
      </button>
      <button class="bg-blue-600 hover:bg-blue-700 text-white text-sm px-4 py-2 rounded"
        on:click={() => createOpen = !createOpen}>{createOpen?'Cancel':'New Model'}</button>
    </div>
  </div>

  {#if createOpen}
    <div class="bg-white rounded-lg shadow p-4">
      <h3 class="text-lg font-semibold text-gray-700 mb-3">Create Model</h3>
      <form class="grid md:grid-cols-3 gap-4" on:submit|preventDefault={createModel}>
        <div>
          <label for="m-name" class="text-sm text-gray-600">Name</label>
          <input id="m-name" class="w-full border rounded px-3 py-2" bind:value={c_name} placeholder="gpt-4o-mini" />
        </div>
        <div>
          <label for="m-inp" class="text-sm text-gray-600">Input price (micros)</label>
          <input id="m-inp" type="number" class="w-full border rounded px-3 py-2" bind:value={c_input} />
        </div>
        <div>
          <label for="m-out" class="text-sm text-gray-600">Output price (micros)</label>
          <input id="m-out" type="number" class="w-full border rounded px-3 py-2" bind:value={c_output} />
        </div>
        <div class="md:col-span-3">
          <button type="submit" class="bg-blue-600 hover:bg-blue-700 text-white text-sm px-4 py-2 rounded">
            Create
          </button>
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
            <th class="text-right px-3 py-2">Input (micros)</th>
            <th class="text-right px-3 py-2">Output (micros)</th>
            <th class="text-left px-3 py-2">Currency</th>
            <th class="text-left px-3 py-2">Updated</th>
            <th class="text-right px-3 py-2">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#if loading}
            <tr><td class="px-3 py-3 text-gray-500" colspan="6">Loading…</td></tr>
          {:else if !models.length}
            <tr><td class="px-3 py-3 text-gray-500" colspan="6">No models</td></tr>
          {:else}
            {#each models as m}
              <tr class="border-t">
                <td class="px-3 py-2 font-medium text-gray-800">{m.name}</td>
                {#if editKey === m.name}
                  <td class="px-3 py-2 text-right"><input type="number" class="w-36 border rounded px-2 py-1 text-right" bind:value={e_input} /></td>
                  <td class="px-3 py-2 text-right"><input type="number" class="w-36 border rounded px-2 py-1 text-right" bind:value={e_output} /></td>
                  <td class="px-3 py-2">IRR</td>
                  <td class="px-3 py-2 text-gray-600">{m.updated_at ? new Date(m.updated_at).toLocaleString() : '—'}</td>
                  <td class="px-3 py-2 text-right space-x-2">
                    <button class="text-blue-600 hover:text-blue-800" on:click={() => saveEdit(m.name)}>Save</button>
                    <button class="text-gray-600 hover:text-gray-800" on:click={cancelEdit}>Cancel</button>
                  </td>
                {:else}
                  <td class="px-3 py-2 text-right text-gray-700">{m.input_price_micros.toLocaleString()}</td>
                  <td class="px-3 py-2 text-right text-gray-700">{m.output_price_micros.toLocaleString()}</td>
                  <td class="px-3 py-2 text-gray-700">{m.currency}</td>
                  <td class="px-3 py-2 text-gray-600">{m.updated_at ? new Date(m.updated_at).toLocaleString() : '—'}</td>
                  <td class="px-3 py-2 text-right space-x-2">
                    <button class="text-blue-600 hover:text-blue-800" on:click={() => startEdit(m)}>Edit</button>
                    <button class="text-red-600 hover:text-red-800" on:click={() => remove(m.name)}>Delete</button>
                  </td>
                {/if}
              </tr>
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
