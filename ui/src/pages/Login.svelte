<script lang="ts">
  import { setApiKey } from '../lib/session';

  let apiKey = '';
  let error = '';

  function login() {
    // 1. Basic validation
    if (!apiKey.trim()) {
      error = 'Admin API key is required.';
      return;
    }

    // 2. Mock API Call Simulation
    // In a real application, you would make a fetch() request here to validate the key.
    // For now, we will accept any non-empty key as valid.
    error = ''; // Clear previous errors
    
    // 3. Store the key in the session and redirect on success
    setApiKey(apiKey.trim());
    location.hash = '#/dashboard';
  }
</script>

<div class="max-w-md mx-auto mt-10 bg-white p-8 rounded-lg shadow-md">
  <h2 class="text-2xl font-bold mb-6 text-center text-gray-700">Admin Panel Login</h2>
  
  <div class="mb-4">
    <label for="apiKey" class="block text-gray-600 mb-2">Admin API Key</label>
    <input 
      id="apiKey"
      type="password" 
      class="w-full border rounded-md px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500" 
      bind:value={apiKey} 
      on:keydown={(e) => e.key === 'Enter' && login()}
      placeholder="Enter your admin API key" 
    />
  </div>

  {#if error}
    <div class="bg-red-100 text-red-700 text-sm p-3 rounded-md mb-4">{error}</div>
  {/if}
  
  <div class="flex items-center justify-between">
    <button 
      class="bg-blue-600 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded-md focus:outline-none focus:shadow-outline transition-colors duration-200" 
      on:click={login}
    >
      Login
    </button>
    <button
      type="button"
      class="inline-block align-baseline font-bold text-sm text-blue-500 hover:text-blue-800"
    >
      API Key Docs
    </button>
  </div>
</div>