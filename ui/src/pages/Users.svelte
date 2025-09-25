<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api'; // Use the same API helper

  // --- State Management ---
  let users: any[] = [];
  let total = 0;
  let limit = 10; // Page size
  let offset = 0;
  let loading = true;
  let error = '';

  // --- Data Fetching ---
  async function load() {
    loading = true;
    error = '';
    try {
      // --- REAL API CALL ---
      const res = await get<{ data: any[]; total: number; limit: number; offset: number }>(
        `/api/v1/users?limit=${limit}&offset=${offset}`
      );

      if (!res || !res.data) {
        users = [];
        total = 0;
        return;
      }

      users = res.data;
      total = res.total;
      limit = res.limit;
      offset = res.offset;

    } catch (e: any) {
      console.error(e);
      error = e.message || 'Failed to load users';
    } finally {
      loading = false;
    }
  }

  function nextPage() {
    if (offset + limit < total) {
      offset += limit;
      load();
    }
  }

  function prevPage() {
    if (offset - limit >= 0) {
      offset -= limit;
      load();
    }
  }

  function openUser(id: string) {
    location.hash = `#/users/${id}`;
  }

  onMount(load);
</script>

<div class="flex justify-between items-center mb-6">
  <h2 class="text-2xl font-semibold text-gray-800">Users</h2>
</div>

{#if loading}
  <div class="text-center p-6 bg-white rounded-lg shadow">
    <p class="text-gray-600">Loading user list...</p>
  </div>
{:else if error}
  <div class="bg-red-100 border-l-4 border-red-500 text-red-700 p-4 rounded-md shadow" role="alert">
    <p class="font-bold">Error</p>
    <p>{error}</p>
  </div>
{:else if users.length == 0}
  <div class="p-6 bg-white rounded-lg shadow text-gray-600 text-center">
    No users found.
  </div>
{:else}
  <div class="bg-white rounded-lg shadow overflow-x-auto">
    <table class="min-w-full bg-white">
      <thead class="bg-gray-50">
        <tr>
          <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Full Name</th>
          <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Username</th>
          <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Phone</th>
          <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-200">
        {#each users as u (u.id)}
          <tr class="hover:bg-gray-50">
            <td class="px-6 py-4 whitespace-nowrap">{u.full_name ?? '-'}</td>
            <td class="px-6 py-4 whitespace-nowrap text-gray-500">@{u.username ?? '-'}</td>
            <td class="px-6 py-4 whitespace-nowrap text-gray-500">{u.phone_number ?? '-'}</td>
            <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
              <button class="text-blue-600 hover:text-blue-900" on:click={() => openUser(u.id)}>
                View Details
              </button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="mt-6 flex justify-between items-center">
    <span class="text-sm text-gray-600">
      Showing {offset + 1} - {Math.min(offset + limit, total)} of {total}
    </span>
    <div class="inline-flex">
      <button 
        class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-l-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed" 
        on:click={prevPage}
        disabled={offset === 0}
      >
        Previous
      </button>
      <button 
        class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border-t border-b border-r border-gray-300 rounded-r-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
        on:click={nextPage}
        disabled={offset + limit >= total}
      >
        Next
      </button>
    </div>
  </div>
{/if}
