<script lang="ts">
  import { loginWithApiKey } from '../lib/api';
  import { setAuthenticated } from '../lib/session';

  let apiKey = '';
  let error = '';
  let loading = false;

  async function onSubmit() {
    error = '';
    if (!apiKey.trim()) {
      error = 'Admin API key is required.';
      return;
    }
    loading = true;
    try {
      await loginWithApiKey(apiKey.trim());           // sets HttpOnly cookie
      setAuthenticated(true);                         // steer router
      window.location.assign('/#/dashboard');         // go to dashboard
    } catch (e: any) {
      error = e?.message || 'Login failed';
    } finally {
      loading = false;
    }
  }
</script>

<form class="max-w-md mx-auto mt-10 bg-white p-8 rounded-lg shadow-md"
      on:submit|preventDefault={onSubmit}>
  <h2 class="text-2xl font-bold mb-6 text-center text-gray-700">Admin Panel Login</h2>
  <div class="mb-4">
    <label for="apiKey" class="block text-gray-600 mb-2">Admin API Key</label>
    <input
      id="apiKey"
      type="password"
      class="w-full border rounded-md px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
      bind:value={apiKey}
      placeholder="Enter your admin API key"
      autocomplete="current-password"
    />
  </div>
  {#if error}
    <div class="bg-red-100 text-red-700 text-sm p-3 rounded-md mb-4">{error}</div>
  {/if}
  <div class="flex items-center justify-between">
    <button type="submit"
      class="bg-blue-600 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded-md focus:outline-none focus:shadow-outline transition-colors duration-200"
      disabled={loading}>
      {loading ? 'Logging in…' : 'Login'}
    </button>
  </div>
</form>
