<template>
  <main class="page">
    <header class="header">
      <div class="brand">
        <h1 class="title">Name That Tune</h1>
        <p class="subtitle muted">Game settings</p>
      </div>

      <nav class="nav">
        <RouterLink class="link" to="/">Games</RouterLink>
        <RouterLink class="link" to="/games/name-that-tune">Lobby</RouterLink>
        <RouterLink class="link" to="/profile">Profile</RouterLink>
      </nav>
    </header>

    <section class="layout">
      <aside class="menu card">
        <h2 class="h2">Settings</h2>
        <RouterLink class="menu-item" to="/games/name-that-tune/settings/playlists">Playlists</RouterLink>
        <div class="menu-item menu-item-disabled">More coming soon</div>
      </aside>

      <section class="content card">
        <RouterView />
      </section>
    </section>
  </main>
</template>

<script setup>
import { RouterLink, RouterView } from 'vue-router'
</script>

<style scoped>
.page {
  max-width: 1100px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}

.header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.brand .title {
  margin: 0;
  line-height: 1.1;
}
.brand .subtitle {
  margin: 6px 0 0;
  opacity: 0.8;
}

.nav {
  display: flex;
  gap: 12px;
}
.link {
  text-decoration: none;
  opacity: 0.9;
  color: inherit;
}
.link.router-link-active {
  font-weight: 650;
  opacity: 1;
}

.layout {
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
}

@media (min-width: 900px) {
  .layout {
    grid-template-columns: 260px 1fr;
    align-items: start;
  }
}

.card {
  background: var(--color-background-soft, rgba(255, 255, 255, 0.06));
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  border-radius: 12px;
  padding: 16px;
}

.h2 {
  margin: 0 0 10px;
}

.muted {
  opacity: 0.8;
}

.menu-item {
  display: block;
  padding: 10px 12px;
  border-radius: 10px;
  text-decoration: none;
  color: inherit;
  border: 1px solid rgba(255, 255, 255, 0.06);
}
.menu-item.router-link-active {
  border-color: rgba(100, 140, 255, 0.35);
  background: rgba(100, 140, 255, 0.14);
  font-weight: 650;
}

.menu-item + .menu-item {
  margin-top: 10px;
}

.menu-item-disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
