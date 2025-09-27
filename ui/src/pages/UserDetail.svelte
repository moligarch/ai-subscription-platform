<script lang="ts">
  import { onMount } from 'svelte';
  import { get, ApiError } from '../lib/api';

  export let id: string;

  type RawSub = Record<string, any>;

  let data:
    | {
        user: {
          id: string;
          telegram_id: number;
          username: string;
          full_name: string;
          phone_number?: string;
          registered_at: string;
        };
        subscriptions: Array<{
          id: string;
          plan_id: string;
          status: string;
          start_at: string | null;
          expires_at: string | null;
          remaining_credits: number;
        }>;
      }
    | null = null;

  let loading = false;
  let error = '';
  let notFound = false;

  function normUser(u: any) {
    if (!u) return null;
    return {
      id: u.id ?? u.ID ?? '',
      telegram_id: u.telegram_id ?? u.TelegramID ?? 0,
      username: u.username ?? u.Username ?? '',
      full_name: u.full_name ?? u.FullName ?? '',
      phone_number: u.phone_number ?? u.PhoneNumber ?? '',
      registered_at:
        u.registered_at ?? u.RegisteredAt ?? u.registeredAt ?? ''
    };
  }

  function normSub(s: RawSub) {
    return {
      id: s.id ?? s.ID ?? '',
      plan_id: s.plan_id ?? s.PlanID ?? s.planId ?? '',
      status: String(s.status ?? s.Status ?? '').toLowerCase(),
      start_at: s.start_at ?? s.StartAt ?? null,
      expires_at: s.expires_at ?? s.ExpiresAt ?? null,
      remaining_credits:
        Number(
          s.remaining_credits ?? s.RemainingCredits ?? s.remainingCredits ?? 0
        ) || 0
    };
  }

  onMount(async () => {
    loading = true;
    error = '';
    notFound = false;

    try {
      // Single call — this endpoint returns both user and subscriptions
      const raw = await get<any>(`/api/v1/users/${id}`);

      // Accept both {user, subscriptions} and {data:{user, subscriptions}}
      const envelope = raw?.data ?? raw;
      const userObj = envelope?.user ?? envelope ?? null;
      const subsArr = Array.isArray(envelope?.subscriptions)
        ? envelope.subscriptions
        : [];

      const user = normUser(userObj);
      const subs = subsArr.map(normSub);

      if (!user) {
        notFound = true;
        return;
      }

      data = { user, subscriptions: subs };
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
    <p class="font-semibold">Error</p>
    <p class="text-sm mt-1">{error}</p>
  </div>
{:else if notFound}
  <div class="bg-yellow-50 border-l-4 border-yellow-400 text-yellow-700 p-4 rounded-md shadow">
    <p class="font-semibold">User Not Found</p>
    <p class="text-sm mt-1">We couldn't find a user with ID <code class="px-1 bg-yellow-100 rounded">{id}</code>.</p>
  </div>
{:else if data}
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
    <!-- User Card -->
    <div class="lg:col-span-1 bg-white p-6 rounded-lg shadow">
      <h3 class="text-lg font-semibold text-gray-800 mb-4">Profile</h3>
      <dl class="space-y-3">
        <div class="flex justify-between">
          <dt class="text-gray-500">ID</dt>
          <dd class="font-mono text-gray-800">{data.user.id}</dd>
        </div>
        <div class="flex justify-between">
          <dt class="text-gray-500">Telegram ID</dt>
          <dd class="text-gray-800">{data.user.telegram_id}</dd>
        </div>
        <div class="flex justify-between">
          <dt class="text-gray-500">Username</dt>
          <dd class="text-gray-800">@{data.user.username}</dd>
        </div>
        <div class="flex justify-between">
          <dt class="text-gray-500">Full name</dt>
          <dd class="text-gray-800">{data.user.full_name}</dd>
        </div>
        <div class="flex justify-between">
          <dt class="text-gray-500">Phone</dt>
          <dd class="text-gray-800">{data.user.phone_number || '—'}</dd>
        </div>
        <div class="flex justify-between">
          <dt class="text-gray-500">Registered</dt>
          <dd class="text-gray-800">
            {#if data.user.registered_at}
              {new Date(data.user.registered_at).toLocaleString()}
            {:else}
              —
            {/if}
          </dd>
        </div>
      </dl>
    </div>

    <!-- Subscriptions -->
    <div class="lg:col-span-2 bg-white p-6 rounded-lg shadow">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-gray-800">Subscriptions</h3>
      </div>

      {#if data.subscriptions.length > 0}
        <ul class="divide-y divide-gray-100">
          {#each data.subscriptions as sub}
            <li class="py-4">
              <div class="flex items-center justify-between">
                <div>
                  <div class="text-sm text-gray-500">Subscription ID</div>
                  <div class="font-mono text-gray-800">{sub.id}</div>
                </div>
                <div class="text-right">
                  <div class="text-sm text-gray-500">Plan ID</div>
                  <div class="font-mono text-gray-800">{sub.plan_id}</div>
                </div>
              </div>

              <div class="text-sm text-gray-600 space-x-4 mt-1">
                <span>
                  Status:
                  <span
                    class="font-medium"
                    class:text-green-700={sub.status === 'active'}
                    class:text-yellow-700={sub.status === 'reserved'}
                    class:text-red-700={sub.status === 'finished' || sub.status === 'cancelled'}
                  >
                    {sub.status}
                  </span>
                </span>

                <span>
                  Credits:
                  <span class="font-medium">{sub.remaining_credits.toLocaleString()}</span>
                </span>

                <span>
                  Expires:
                  <span class="font-medium">
                    {#if sub.expires_at}{new Date(sub.expires_at).toLocaleDateString()}{:else}—{/if}
                  </span>
                </span>

                <span>
                  Started:
                  <span class="font-medium">
                    {#if sub.start_at}{new Date(sub.start_at).toLocaleDateString()}{:else}—{/if}
                  </span>
                </span>
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
