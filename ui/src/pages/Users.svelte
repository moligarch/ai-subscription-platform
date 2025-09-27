<script lang="ts">
  import { onMount } from 'svelte';
  import { get, type ApiError } from '../lib/api';

  // --- State ---
  let users: any[] = [];
  let total = 0;
  let limit = 10;
  let offset = 0;
  let loading = true;
  let error = '';

  function goLogin() {
    location.hash = '#/';
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const res = await get<{ data: any[]; total: number; limit: number; offset: number }>(
        `/api/v1/users?limit=${limit}&offset=${offset}`
      );
      users = res.data || [];
      total = res.total || 0;
    } catch (e: any) {
      console.error(e);
      if ((e as ApiError)?.status === 401 || (e?.message || '').includes('unauthorized')) {
        return goLogin();
      }
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

<h2 class="text-2xl font-bold mb-4">Users</h2>

{#if loading}
  <div>Loading…</div>
{:else if error}
  <div class="text-red-600">{error}</div>
  <button class="mt-3 px-3 py-2 rounded bg-blue-600 text-white" on:click={goLogin}>Go to login</button>
{:else}
  <div class="bg-white p-4 rounded-lg shadow">
    <table class="w-full">
      <thead>
        <tr class="text-left text-gray-600">
          <th class="py-2">ID</th>
          <th class="py-2">Telegram</th>
          <th class="py-2">Username</th>
          <th class="py-2">Registered</th>
          <th class="py-2">Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each users as u}
          <tr class="border-t">
            <td class="py-2 text-sm">{u.id}</td>
            <td class="py-2 text-sm">{u.telegram_id}</td>
            <td class="py-2 text-sm">{u.username}</td>
            <td class="py-2 text-sm">{u.registered_at ? new Date(u.registered_at).toLocaleString() : '-'}</td>
            <td class="py-2">
              <button class="text-blue-600 hover:underline" on:click={() => openUser(u.id)}>View</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>

    <div class="flex items-center justify-between mt-4">
      <div class="text-sm text-gray-600">
        Showing {Math.min(offset + 1, total)}–{Math.min(offset + users.length, total)} of {total}
      </div>
      <div class="space-x-2">
        <button class="px-3 py-2 rounded bg-gray-200 hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed"
          on:click={prevPage} disabled={offset === 0}>
          Prev
        </button>
        <button class="px-3 py-2 rounded bg-gray-200 hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed"
          on:click={nextPage} disabled={offset + limit >= total}>
          Next
        </button>
      </div>
    </div>
  </div>
{/if}
