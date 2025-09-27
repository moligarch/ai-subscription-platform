<script lang="ts">
  import { onMount } from 'svelte';
  import { get, type ApiError } from '../lib/api';

  let stats: any = null;
  let error = '';
  let loading = true;

  function goLogin() {
    location.hash = '#/';
  }

  function totalActiveSubs(): number {
    const m = (stats && stats.active_subs_by_plan) || {};
    // Typescript annotation stays in script, not in template
    return Object.values(m).reduce((a: number, b: unknown) => a + Number((b as any) || 0), 0);
  }

  onMount(async () => {
    loading = true;
    error = '';
    try {
      stats = await get('/api/v1/stats');
    } catch (e: any) {
      console.error(e);
      if ((e as ApiError)?.status === 401 || (e?.message || '').includes('unauthorized')) {
        return goLogin();
      }
      error = e.message || 'Failed to load dashboard stats';
    } finally {
      loading = false;
    }
  });
</script>

<h2 class="text-2xl font-bold mb-4">Dashboard</h2>

{#if loading}
  <div>Loading…</div>
{:else if error}
  <div class="text-red-600">{error}</div>
  <button class="mt-3 px-3 py-2 rounded bg-blue-600 text-white" on:click={goLogin}>Go to login</button>
{:else}
  <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Total Users</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">{stats.total_users?.toLocaleString()}</div>
    </div>

    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Active Subscriptions</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">
        {totalActiveSubs().toLocaleString()}
      </div>
    </div>

    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Total Credits Remaining</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">{stats.total_remaining_credits?.toLocaleString()}</div>
    </div>

    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Monthly Revenue (IRR)</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">{stats.revenue_irr?.month?.toLocaleString()}</div>
    </div>
  </div>
{/if}
