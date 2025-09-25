<script lang="ts">
  import { onMount } from 'svelte';
  import { get, ApiError } from '../lib/api';

  export let id: string;

  type RawSub = Record<string, any>;

  let data: {
    user: {
      id: string;
      telegram_id: number;
      username: string;
      full_name: string;
      phone_number?: string;
      registered_at: string;
    };
    subscriptions: {
      id: string;
      plan_id: string;
      plan_name?: string;
      status: string;
      start_at?: string;
      expires_at?: string;
      remaining_credits: number;
    }[];
  } | null = null;

  let error = '';
  let notFound = false;
  let loading = true;

  function normalizeSub(s: RawSub) {
    return {
      id: s.id ?? s.ID ?? s.ID?.toString() ?? '',
      plan_id: s.plan_id ?? s.PlanID ?? s.PlanID ?? '',
      plan_name: s.plan_name ?? s.PlanName ?? s.PlanName ?? '',
      status: s.status ?? s.Status ?? '',
      start_at: s.start_at ?? s.StartAt ?? s.StartAt ?? null,
      expires_at: s.expires_at ?? s.ExpiresAt ?? s.ExpiresAt ?? null,
      remaining_credits: s.remaining_credits ?? s.RemainingCredits ?? 0
    };
  }

  onMount(async () => {
    loading = true;
    error = '';
    notFound = false;

    try {
      // Fetch raw response
      const raw = await get<any>(`/api/v1/users/${id}`);

      // Debug: inspect shape in browser console
      console.debug('User detail raw response:', raw);

      // Support both shapes:
      // - backend returns { user: {...}, subscriptions: [...] }
      // - OR helper get() wraps it as { data: { user:..., subscriptions:... } }
      const payload = raw?.data ?? raw;

      if (!payload) {
        // Nothing useful returned
        throw new Error('Empty payload from /api/v1/users');
      }

      // If API sometimes returns user object at top-level (not nested), accommodate that:
      const userObj = payload.user ?? payload;
      const rawSubs = payload.subscriptions ?? payload.Subscriptions ?? payload.subs ?? [];

      const subs = (Array.isArray(rawSubs) ? rawSubs : [])
        .map(normalizeSub);

      // Ensure `data.user` exists and has expected fields
      if (!userObj || !userObj.id) {
        // maybe the endpoint returned an array or different shape
        throw new Error('Unexpected user payload shape');
      }

      data = {
        user: {
          id: userObj.id,
          telegram_id: userObj.telegram_id ?? userObj.telegramId ?? userObj.TelegramID ?? 0,
          username: userObj.username ?? userObj.UserName ?? '',
          full_name: userObj.full_name ?? userObj.FullName ?? userObj.fullName ?? '',
          phone_number: userObj.phone_number ?? userObj.PhoneNumber ?? userObj.phone ?? '',
          registered_at: userObj.registered_at ?? userObj.RegisteredAt ?? userObj.registered_at ?? ''
        },
        subscriptions: subs
      };
    } catch (e: any) {
      console.error('User detail load error', e);

      if ((e as ApiError)?.status === 404) {
        notFound = true;
      } else {
        error = e?.message ?? String(e) ?? `Failed to load user ${id}`;
      }
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
{:else if notFound}
  <div class="p-6 bg-white rounded-lg shadow text-gray-600 text-center">
    User not found.
  </div>
{:else if data}
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
    <div class="lg:col-span-1 bg-white p-6 rounded-lg shadow">
      <h3 class="text-lg font-semibold border-b pb-2 mb-4">Profile</h3>
      <div class="space-y-3 text-sm">
        <div><strong>Full Name:</strong> {data.user.full_name}</div>
        <div><strong>Username:</strong> @{data.user.username}</div>
        <div><strong>Phone:</strong> {data.user.phone_number ?? '-'}</div>
        <div><strong>User ID:</strong> <code class="text-xs bg-gray-100 p-1 rounded">{data.user.id}</code></div>
        <div><strong>Telegram ID:</strong> <code class="text-xs bg-gray-100 p-1 rounded">{data.user.telegram_id}</code></div>
        <div><strong>Registered:</strong> {new Date(data.user.registered_at).toLocaleDateString()}</div>
      </div>
    </div>

    <div class="lg:col-span-2 bg-white p-6 rounded-lg shadow">
      <h3 class="text-lg font-semibold border-b pb-2 mb-4">Subscription History</h3>
      {#if data.subscriptions && data.subscriptions.length > 0}
        <ul class="space-y-4">
          {#each data.subscriptions as sub (sub.id)}
            <li class="p-4 rounded-md border" class:border-green-300={sub.status === 'active'} class:bg-green-50={sub.status === 'active'}>
              <div class="font-bold">{sub.plan_name ?? sub.plan_id}</div>
              <div class="text-sm text-gray-600 space-x-4 mt-1">
                <span>Status: <span class="font-medium" class:text-green-700={sub.status === 'active'}>{sub.status}</span></span>
                <span>Credits: <span class="font-medium">{sub.remaining_credits.toLocaleString()}</span></span>
                <span>Expires: <span class="font-medium">{new Date(sub.expires_at).toLocaleDateString()}</span></span>
                <span>Plan ID: <span class="font-medium">{sub.plan_id.toLocaleString()}</span></span>
              </div>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="text-sm text-gray-500">This user has no subscription history.</p>
      {/if}
    </div>
  </div>
{/if}
