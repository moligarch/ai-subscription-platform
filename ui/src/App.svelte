<script lang="ts">
  import { onMount } from 'svelte';
  import { getApiKey, clearApiKey } from './lib/session';

  // Page Components
  import Login from './pages/Login.svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import Users from './pages/Users.svelte';
  import UserDetail from './pages/UserDetail.svelte';
  import Plans from './pages/Plans.svelte';

  let route = '';
  let currentComponent: any = Login; // Default component

  // A helper to map routes to components
  const routes = {
    '/dashboard': Dashboard,
    '/users': Users,
    '/plans': Plans,
    '/login': Login,
  };

  function logout() {
    clearApiKey();
    location.hash = '#/login';
  }

  // This is our new, more robust routing function
  function updateRoute() {
    const hash = location.hash || '#/';

    // --- Route Guard Logic ---
    const isLoggedIn = !!getApiKey();

    if (isLoggedIn && (hash === '#/' || hash === '#/login')) {
      // If logged in and at root or login page, go to dashboard
      location.hash = '#/dashboard';
      return; // The hash change will re-trigger this function
    }

    if (!isLoggedIn && hash !== '#/login') {
      // If not logged in and trying to access a protected page, force login
      location.hash = '#/login';
      return; // The hash change will re-trigger this function
    }
    
    route = hash.slice(1);
  }

  onMount(() => {
    // Initial route check
    updateRoute();
    // Listen for future route changes
    window.addEventListener('hashchange', updateRoute);
  });

  // This $: block is a "reactive statement" in Svelte.
  // It automatically re-runs whenever 'route' changes.
  $: {
    if (route.startsWith('/users/')) {
      currentComponent = UserDetail;
    } else {
      // Find the component that matches the start of the route
      const baseRoute = Object.keys(routes).find(r => route.startsWith(r));
      currentComponent = baseRoute ? routes[baseRoute as keyof typeof routes] : Login;
    }
  }
</script>

<div class="min-h-screen bg-gray-50">
  <header class="bg-white border-b shadow-sm">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-3 flex items-center justify-between">
      <h1 class="text-xl font-semibold text-gray-800">Telegram AI Admin</h1>
      <nav class="space-x-4">
        {#if getApiKey()}
          <a href="#/dashboard" class="text-sm font-medium text-gray-600 hover:text-blue-600">Dashboard</a>
          <a href="#/users" class="text-sm font-medium text-gray-600 hover:text-blue-600">Users</a>
          <a href="#/plans" class="text-sm font-medium text-gray-600 hover:text-blue-600">Plans</a>
          <button on:click={logout} class="text-sm font-medium text-red-600 hover:text-red-800">Logout</button>
        {/if}
      </nav>
    </div>
  </header>

  <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <svelte:component this={currentComponent} id={route.split('/')[2]} />
  </main>
</div>