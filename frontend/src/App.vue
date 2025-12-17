<script setup>
import { RouterView, RouterLink } from 'vue-router'
</script>

<template>
  <div class="app">
    <header class="app-header">
      <RouterLink class="brand" to="/">bes-blind</RouterLink>
      <nav class="nav">
        <RouterLink class="link" to="/">Home</RouterLink>
        <RouterLink class="link" to="/profile">Profile</RouterLink>
      </nav>
    </header>

    <RouterView />
  </div>
</template>

<style scoped>
.app {
  min-height: 100vh;
}

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  background: var(--color-background, rgba(0, 0, 0, 0.2));
}

.brand {
  text-decoration: none;
  font-weight: 800;
  letter-spacing: 0.02em;
  color: inherit;
}

.nav {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.link {
  text-decoration: none;
  opacity: 0.9;
  color: inherit;
}

.link.router-link-active {
  opacity: 1;
  font-weight: 650;
}
</style>
