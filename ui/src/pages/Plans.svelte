<script lang="ts">
  import { onMount } from 'svelte';
  import { get, post, put, del, ApiError } from '../lib/api';

  type Plan = {
    id: string;
    name: string;
    duration_days: number;
    credits: number;
    price_irr: number;
    supported_models: string[]; // backend shape
  };

  // --- State ---
  let plans: Plan[] = [];
  let loading = true;
  let saving = false;
  let error = '';

  // Form state
  let showForm = false;
  let editId: string | null = null;
  const defaultFormState = {
    name: '',
    duration_days: 30,
    credits: 100000,
    price_irr: 50000,
    supported_models: 'gpt-4o, gemini-1.5-pro'
  };
  let form: {
    name: string;
    duration_days: number;
    credits: number;
    price_irr: number;
    supported_models: string;
  } = { ...defaultFormState };

  // --- Helpers ---
  function toSupportedModelsArray(s: string) {
    return s
      .split(',')
      .map(x => x.trim())
      .filter(Boolean);
  }

  function fromSupportedModelsArray(arr: string[]) {
    return arr.join(', ');
  }

  // Normalize backend payloads
  function normalizePlan(raw: any): Plan {
    return {
      id: raw.id ?? raw.ID ?? '',
      name: raw.name ?? raw.Name ?? '',
      duration_days: raw.duration_days ?? raw.DurationDays ?? 0,
      credits: raw.credits ?? raw.Credits ?? 0,
      price_irr: raw.price_irr ?? raw.PriceIRR ?? 0,
      supported_models: raw.supported_models ?? raw.supportedModels ?? raw.SupportedModels ?? []
    };
  }

  // --- Notifications (simple inline notifications shown below the table) ---
  type Notification = { id: string; message: string; type: 'error' | 'info' };
  let notifications: Notification[] = [];

  function addNotification(message: string, type: 'error' | 'info' = 'error', ttl = 8000) {
    const id = cryptoRandomId();
    notifications = [...notifications, { id, message, type }];
    if (ttl > 0) {
      setTimeout(() => dismissNotification(id), ttl);
    }
  }

  function dismissNotification(id: string) {
    notifications = notifications.filter(n => n.id !== id);
  }

  // small helper for ids (uses crypto if available)
  function cryptoRandomId() {
    try {
      return (crypto as any).randomUUID?.() ?? Math.random().toString(36).slice(2, 10);
    } catch {
      return Math.random().toString(36).slice(2, 10);
    }
  }

  // --- CRUD operations ---
  async function load() {
    loading = true;
    error = '';
    try {
      const raw = await get<any>('/api/v1/plans');
      const payload = raw?.data ?? raw;
      const list = Array.isArray(payload) ? payload : payload?.data ?? payload?.plans ?? payload?.plans_list ?? [];
      plans = (Array.isArray(list) ? list : []).map(normalizePlan);
    } catch (e: any) {
      console.error(e);
      error = e?.message ?? 'Failed to load plans';
      plans = [];
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    editId = null;
    form = { ...defaultFormState };
    showForm = true;
  }

  function startEdit(plan: Plan) {
    editId = plan.id;
    form = {
      name: plan.name,
      duration_days: plan.duration_days,
      credits: plan.credits,
      price_irr: plan.price_irr,
      supported_models: fromSupportedModelsArray(plan.supported_models)
    };
    showForm = true;
  }

  async function submit() {
    saving = true;
    error = '';
    try {
      const payload = {
        name: form.name,
        duration_days: Number(form.duration_days),
        credits: Number(form.credits),
        price_irr: Number(form.price_irr),
        supported_models: toSupportedModelsArray(form.supported_models)
      };

      if (editId) {
        // update
        const raw = await put<any>(`/api/v1/plans/${editId}`, payload);
        const updated = normalizePlan(raw?.data ?? raw);
        plans = plans.map(p => (p.id === editId ? updated : p));
      } else {
        // create
        const raw = await post<any>('/api/v1/plans', payload);
        const created = normalizePlan(raw?.data ?? raw);
        plans = [created, ...plans];
      }

      showForm = false;
      editId = null;
    } catch (e: any) {
      console.error(e);
      if ((e as ApiError)?.status) {
        error = `Server error: ${ (e as ApiError).status } ${e?.message ?? ''}`;
      } else {
        error = e?.message ?? 'Failed to save plan';
      }
    } finally {
      saving = false;
    }
  }

  async function remove(id: string) {
    if (!confirm('Are you sure you want to delete this plan?')) return;
    try {
      // attempt delete
      await del(`/api/v1/plans/${id}`);
      // on success remove from local array
      plans = plans.filter(p => p.id !== id);
      addNotification('Plan deleted successfully', 'info', 4000);
    } catch (e: any) {
      console.error(e);
      // Prefer server message if provided, otherwise fall back to a generic message
      const msg =
        (e && (e.message ?? (e.error ?? (e?.data?.message ?? undefined)))) ||
        'Plan could not be deleted. It may have active subscribers or server rejected the request.';
      addNotification(msg, 'error', 10000);
    }
  }

  onMount(load);
</script>

<div>
  <div class="flex justify-between items-center mb-6">
    <h2 class="text-2xl font-semibold text-gray-800">Subscription Plans</h2>
    <button class="bg-blue-600 hover:bg-blue-700 text-white font-bold px-4 py-2 rounded-md" on:click={startCreate}>
      New Plan
    </button>
  </div>

  {#if saving}
    <div class="text-center p-4 bg-yellow-50 rounded-md mb-4">Saving...</div>
  {/if}

  {#if showForm}
    <div class="bg-white p-6 rounded-lg shadow mb-6 border border-gray-200">
      <h3 class="text-lg font-semibold mb-4">{editId ? 'Edit Plan' : 'Create New Plan'}</h3>
      {#if error}
        <div class="mb-3 text-sm text-red-700">{error}</div>
      {/if}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label for="plan-name" class="block text-sm font-medium text-gray-700 mb-1">Plan Name</label>
          <input id="plan-name" placeholder="e.g., Pro Plan" class="border p-2 w-full rounded-md" bind:value={form.name} />
        </div>
        <div>
          <label for="plan-duration" class="block text-sm font-medium text-gray-700 mb-1">Duration (days)</label>
          <input id="plan-duration" type="number" class="border p-2 w-full rounded-md" bind:value={form.duration_days} />
        </div>
        <div>
          <label for="plan-credits" class="block text-sm font-medium text-gray-700 mb-1">Credits</label>
          <input id="plan-credits" type="number" class="border p-2 w-full rounded-md" bind:value={form.credits} />
        </div>
        <div>
          <label for="plan-price" class="block text-sm font-medium text-gray-700 mb-1">Price (IRR)</label>
          <input id="plan-price" type="number" class="border p-2 w-full rounded-md" bind:value={form.price_irr} />
        </div>
        <div class="md:col-span-2">
          <label for="plan-models" class="block text-sm font-medium text-gray-700 mb-1">Supported Models</label>
          <input id="plan-models" placeholder="e.g., gpt-4o, gemini-1.5-pro" class="border p-2 w-full rounded-md" bind:value={form.supported_models} />
        </div>
      </div>
      <div class="mt-4">
        <button class="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-md mr-2" on:click={submit} disabled={saving}>
          Save
        </button>
        <button class="px-4 py-2 border rounded-md" on:click={() => { showForm = false; error = ''; }}>
          Cancel
        </button>
      </div>
    </div>
  {/if}

  {#if loading}
    <div class="text-center p-6 bg-white rounded-lg shadow"><p class="text-gray-600">Loading plans...</p></div>
  {:else if error}
    <div class="bg-red-100 border-l-4 border-red-500 text-red-700 p-4 rounded-md shadow"><p>{error}</p></div>
  {:else}
    <div class="bg-white rounded-lg shadow overflow-x-auto">
      <table class="min-w-full bg-white">
        <thead class="bg-gray-50">
          <tr>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Duration</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Credits</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Price (IRR)</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Models</th>
            <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200">
          {#each plans as p (p.id)}
            <tr class="hover:bg-gray-50">
              <td class="px-6 py-4 whitespace-nowrap font-medium">{p.name}</td>
              <td class="px-6 py-4 whitespace-nowrap">{p.duration_days} days</td>
              <td class="px-6 py-4 whitespace-nowrap">{p.credits.toLocaleString()}</td>
              <td class="px-6 py-4 whitespace-nowrap">{p.price_irr.toLocaleString?.() ?? p.price_irr}</td>
              <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-600">{(p.supported_models || []).join(', ')}</td>
              <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium space-x-4">
                <button class="text-blue-600 hover:text-blue-900" on:click={() => startEdit(p)}>Edit</button>
                <button class="text-red-600 hover:text-red-900" on:click={() => remove(p.id)}>Delete</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <!-- Notifications area (appears under the table) -->
    {#if notifications.length > 0}
      <div class="mt-4 space-y-2">
        {#each notifications as n (n.id)}
          <div class="flex items-start justify-between p-3 rounded-md shadow-sm"
              class:bg-red-50={n.type === 'error'}
              class:border-red-200={n.type === 'error'}
              class:bg-green-50={n.type === 'info'}
              class:border-green-200={n.type === 'info'}
              style="border:1px solid rgba(0,0,0,0.05);">
            <div class="text-sm text-gray-800">
              <strong class="mr-2">{n.type === 'error' ? 'Error' : 'Info'}:</strong>
              <span>{n.message}</span>
            </div>
            <div>
              <button class="text-xs px-2 py-1 rounded hover:bg-gray-100" on:click={() => dismissNotification(n.id)}>
                Dismiss
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
