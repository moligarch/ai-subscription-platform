<script lang="ts">
  import { onMount } from 'svelte';
  import { isAuthenticated, clearAuthentication } from './lib/session';
  import { logout as apiLogout } from './lib/api';

  // Pages
  import Login from './pages/Login.svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import Users from './pages/Users.svelte';
  import UserDetail from './pages/UserDetail.svelte';
  import Plans from './pages/Plans.svelte';
  import Models from './pages/Models.svelte';


  let route = '';
  let currentComponent: any = Login;

  const routes = {
    '/dashboard': Dashboard,
    '/users': Users,
    '/plans': Plans,
    '/models': Models,
    '/login': Login,
  };

  async function logout() {
    try { await apiLogout(); } catch {}
    clearAuthentication();
    location.hash = '#/login';
  }

  function updateRoute() {
    const hash = location.hash || '#/';
    const authed = isAuthenticated();

    // Guard
    if (authed && (hash === '#/' || hash === '#/login')) {
      location.hash = '#/dashboard';
      return;
    }
    if (!authed && hash !== '#/login') {
      location.hash = '#/login';
      return;
    }
    route = hash.slice(1);
  }

  onMount(() => {
    updateRoute();
    window.addEventListener('hashchange', updateRoute);
  });

  // choose component
  $: {
    if (route.startsWith('/users/')) {
      currentComponent = UserDetail;
    } else {
      const base = Object.keys(routes).find(r => route.startsWith(r));
      currentComponent = base ? routes[base as keyof typeof routes] : Login;
    }
  }
</script>

<div class="min-h-screen bg-gray-50">
  <header class="bg-white border-b shadow-sm">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-3 flex items-center justify-between">
      <h1 class="text-xl font-semibold text-gray-800">Telegram AI Admin</h1>
      <nav class="space-x-4">
        {#if isAuthenticated()}
          <a href="#/dashboard" class="text-sm font-medium text-gray-600 hover:text-blue-600">Dashboard</a>
          <a href="#/users" class="text-sm font-medium text-gray-600 hover:text-blue-600">Users</a>
          <a href="#/plans" class="text-sm font-medium text-gray-600 hover:text-blue-600">Plans</a>
          <a href="#/models" class="text-sm font-medium text-gray-600 hover:text-blue-600">Models</a>
          <button on:click={logout} class="text-sm font-medium text-red-600 hover:text-red-800">Logout</button>
        {/if}
      </nav>
    </div>
  </header>

  <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <svelte:component this={currentComponent} id={route.split('/')[2]} />
  </main>
</div>
