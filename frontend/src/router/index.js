import { createRouter, createWebHistory } from 'vue-router'

// Pages (created/edited elsewhere)
import HomePage from '../views/HomePage.vue'
import ProfilePage from '../views/ProfilePage.vue'
import RoomPage from '../views/RoomPage.vue'

const routes = [
  {
    path: '/',
    name: 'home',
    component: HomePage,
  },
  {
    path: '/profile',
    name: 'profile',
    component: ProfilePage,
  },
  {
    // One room page; it renders either "owner" or "player" view based on:
    // - route query (?mode=owner|player), or
    // - room ownership from snapshot (recommended)
    path: '/rooms/:roomId',
    name: 'room',
    component: RoomPage,
    props: (route) => ({
      roomId: route.params.roomId,
      mode: route.query.mode,
    }),
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

export default router
