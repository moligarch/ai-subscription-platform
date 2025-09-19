<script lang="ts">
  import { onMount } from 'svelte';

  /**
   * The user's ID, passed in as a prop from the App.svelte router.
   * `export let` is how you define props in Svelte.
   */
  export let id: string;

  // --- State Management ---
  let data: any = null;
  let error = '';
  let loading = true;

  // --- Mock Data Fetching ---
  onMount(async () => {
    try {
      // --- MOCK API CALL ---
      // Simulates fetching `/api/v1/users/{id}`
      await new Promise(resolve => setTimeout(resolve, 600));

      // This is our static, mock data for a single user.
      // We use the `id` prop to make the data feel dynamic.
      data = {
        user: {
          id: id,
          telegram_id: 987654321,
          username: `user_${id.slice(-4)}`,
          full_name: `User Fullname ${id.slice(-4)}`,
          phone_number: `+989123456${id.slice(-4)}`,
          registered_at: new Date(Date.now() - 1000 * 3600 * 24 * 30).toISOString(),
        },
        subscriptions: [
          {
            id: 'sub-uuid-1',
            plan_id: 'plan-uuid-pro',
            plan_name: 'Pro Plan', // Added for display purposes
            status: 'active',
            start_at: new Date(Date.now() - 1000 * 3600 * 24 * 15).toISOString(),
            expires_at: new Date(Date.now() + 1000 * 3600 * 24 * 15).toISOString(),
            remaining_credits: 45000,
          },
          {
            id: 'sub-uuid-2',
            plan_id: 'plan-uuid-std',
            plan_name: 'Standard Plan', // Added for display purposes
            status: 'finished',
            start_at: new Date(Date.now() - 1000 * 3600 * 24 * 45).toISOString(),
            expires_at: new Date(Date.now() - 1000 * 3600 * 24 * 15).toISOString(),
            remaining_credits: 0,
          }
        ]
      };

    } catch (e: any) {
      console.error(e);
      error = e.message || `Failed to load user ${id}`;
    } finally {
      loading = false;
    }
  });
</script>

<div class="mb-6">
  <a href="#/users" class="text-blue-600 hover:underline">&larr; Back to All Users</a>
  <h2 class="text-2xl font-semibold text-gray-800 mt-2">User Details</h2>
</div>

{#if loading}
  <div class="text-center p-6 bg-white rounded-lg shadow">
    <p class="text-gray-600">Loading user details...</p>
  </div>
{:else if error}
  <div class="bg-red-100 border-l-4 border-red-500 text-red-700 p-4 rounded-md shadow" role="alert">
    <p class="font-bold">Error</p>
    <p>{error}</p>
  </div>
{:else if data}
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
    <div class="lg:col-span-1 bg-white p-6 rounded-lg shadow">
      <h3 class="text-lg font-semibold border-b pb-2 mb-4">Profile</h3>
      <div class="space-y-3 text-sm">
        <div><strong>Full Name:</strong> {data.user.full_name}</div>
        <div><strong>Username:</strong> @{data.user.username}</div>
        <div><strong>Phone:</strong> {data.user.phone ?? '-'}</div>
        <div><strong>User ID:</strong> <code class="text-xs bg-gray-100 p-1 rounded">{data.user.id}</code></div>
        <div><strong>Telegram ID:</strong> <code class="text-xs bg-gray-100 p-1 rounded">{data.user.telegram_id}</code></div>
        <div><strong>Registered:</strong> {new Date(data.user.registered_at).toLocaleDateString()}</div>
      </div>
    </div>

    <div class="lg:col-span-2 bg-white p-6 rounded-lg shadow">
      <h3 class="text-lg font-semibold border-b pb-2 mb-4">Subscription History</h3>
      <ul class="space-y-4">
        {#each data.subscriptions as sub (sub.id)}
          <li class="p-4 rounded-md border" class:border-green-300={sub.status === 'active'} class:bg-green-50={sub.status === 'active'}>
            <div class="font-bold">{sub.plan_name}</div>
            <div class="text-sm text-gray-600 space-x-4 mt-1">
              <span>Status: <span class="font-medium" class:text-green-700={sub.status === 'active'}>{sub.status}</span></span>
              <span>Credits: <span class="font-medium">{sub.remaining_credits.toLocaleString()}</span></span>
              <span>Expires: <span class="font-medium">{new Date(sub.expires_at).toLocaleDateString()}</span></span>
            </div>
          </li>
        {:else}
          <p class="text-sm text-gray-500">This user has no subscription history.</p>
        {/each}
      </ul>
    </div>
  </div>
{/if}