<script lang="ts">
  import { onMount } from 'svelte';

  let stats: any = null;
  let error = '';
  let loading = true;

  onMount(async () => {
    try {
      // --- MOCK API CALL ---
      // In a real application, this would be: stats = await get('/api/v1/stats');
      // Here, we simulate the network request with a 1-second delay.
      await new Promise(resolve => setTimeout(resolve, 1000));

      // This is our static, mock data that matches the API specification.
      stats = {
        total_users: 152,
        active_subs_by_plan: {
          "plan_id_1": 25,
          "plan_id_2": 51
        },
        total_remaining_credits: 1234567,
        revenue_irr: {
          week: 1500000,
          month: 7850000,
          year: 92400000
        }
      };
      // To test the error state, you could uncomment the line below:
      // throw new Error("Failed to connect to the server.");

    } catch (e: any) {
      console.error(e);
      error = e.message || 'Failed to load dashboard stats';
    } finally {
      loading = false;
    }
  });
</script>

<h2 class="text-2xl font-semibold mb-6 text-gray-800">Dashboard</h2>

{#if loading}
  <div class="text-center p-6 bg-white rounded-lg shadow">
    <p class="text-gray-600">Loading statistics...</p>
  </div>
{:else if error}
  <div class="bg-red-100 border-l-4 border-red-500 text-red-700 p-4 rounded-md shadow" role="alert">
    <p class="font-bold">Error</p>
    <p>{error}</p>
  </div>
{:else if stats}
  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Total Users</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">{stats.total_users}</div>
    </div>
    
    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Active Subscriptions</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">{Object.values(stats.active_subs_by_plan).reduce((a, b) => a + b, 0)}</div>
    </div>

    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Total Credits Remaining</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">{stats.total_remaining_credits.toLocaleString()}</div>
    </div>

    <div class="bg-white p-6 rounded-lg shadow hover:shadow-lg transition-shadow">
      <div class="text-sm font-medium text-gray-500">Monthly Revenue (IRR)</div>
      <div class="text-3xl font-bold text-gray-800 mt-2">{stats.revenue_irr.month.toLocaleString()}</div>
    </div>
  </div>
{/if}