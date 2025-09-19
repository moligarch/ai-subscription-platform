<script lang="ts">
  import { onMount } from 'svelte';
  import { v4 as uuidv4 } from 'uuid'; // For frontend mock purposes only

  // --- State Management ---
  let plans: any[] = [];
  let loading = true;
  let error = '';

  // Form State
  let showForm = false;
  let editId: string | null = null;
  const defaultFormState = {
    name: '',
    duration_days: 30,
    credits: 100000,
    price_irr: 50000,
    supported_models: 'gpt-4o, gemini-1.5-pro'
  };
  let form = { ...defaultFormState };

  // --- Mock Data & API Logic ---
  async function load() {
    loading = true;
    error = '';
    try {
      await new Promise(resolve => setTimeout(resolve, 500));
      if (plans.length === 0) {
          plans = [
          { id: 'd1b2c3d4-e5f6-7890-1234-567890abcdef', name: 'Pro', duration_days: 30, credits: 100000, price_irr: 50000, supported_models: ['gpt-4o', 'gemini-1.5-pro'] },
          { id: 'd2b2c3d4-e5f6-7890-1234-567890abcdef', name: 'Standard', duration_days: 30, credits: 20000, price_irr: 10000, supported_models: ['gpt-4o-mini', 'gemini-1.5-flash'] },
        ];
      }
    } catch (e:any) {
      error = e.message || 'Failed to load plans';
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    editId = null;
    form = { ...defaultFormState };
    showForm = true;
  }

  function startEdit(plan: any) {
    editId = plan.id;
    form = {
      name: plan.name,
      duration_days: plan.duration_days,
      credits: plan.credits,
      price_irr: plan.price_irr,
      supported_models: (plan.supported_models || []).join(', ')
    };
    showForm = true;
  }

  async function submit() {
    await new Promise(resolve => setTimeout(resolve, 400));
    
    const payload = { 
      ...form, 
      supported_models: form.supported_models.split(',').map((s:string) => s.trim()) 
    };

    if (editId) {
      // MOCK UPDATE: Find and replace the plan in the array
      plans = plans.map(p => p.id === editId ? { ...p, ...payload } : p);
    } else {
      // MOCK CREATE: Add a new plan.
      // The UUID is generated here ONLY for the mock UI to track the new row.
      // The real backend will generate the official ID.
      plans = [...plans, { id: uuidv4(), ...payload }];
    }

    showForm = false;
    editId = null;
  }

  async function remove(id: string) {
    if (!confirm('Are you sure you want to delete this plan?')) return;
    
    await new Promise(resolve => setTimeout(resolve, 400));
    plans = plans.filter(p => p.id !== id);
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

  {#if showForm}
    <div class="bg-white p-6 rounded-lg shadow mb-6 border border-gray-200">
      <h3 class="text-lg font-semibold mb-4">{editId ? 'Edit Plan' : 'Create New Plan'}</h3>
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
        <button class="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-md mr-2" on:click={submit}>Save</button>
        <button class="px-4 py-2 border rounded-md" on:click={() => { showForm = false; }}>Cancel</button>
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
            <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200">
          {#each plans as p (p.id)}
            <tr class="hover:bg-gray-50">
              <td class="px-6 py-4 whitespace-nowrap font-medium">{p.name}</td>
              <td class="px-6 py-4 whitespace-nowrap">{p.duration_days} days</td>
              <td class="px-6 py-4 whitespace-nowrap">{p.credits.toLocaleString()}</td>
              <td class="px-6 py-4 whitespace-nowrap">{p.price_irr.toLocaleString()}</td>
              <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium space-x-4">
                <button class="text-blue-600 hover:text-blue-900" on:click={() => startEdit(p)}>Edit</button>
                <button class="text-red-600 hover:text-red-900" on:click={() => remove(p.id)}>Delete</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>