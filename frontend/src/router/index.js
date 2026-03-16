import { createRouter, createWebHistory } from "vue-router";

import HomePage from "../views/HomePage.vue";
import ProfilePage from "../views/ProfilePage.vue";
import { gameRoutes } from "../games";

const routes = [
  {
    path: "/",
    name: "home",
    component: HomePage,
  },
  {
    path: "/profile",
    name: "profile",
    component: ProfilePage,
  },
  ...gameRoutes,
  {
    // Legacy route kept for existing links; will likely become `/games/:gameId/rooms/:roomId` later.
    path: "/rooms/:roomId",
    redirect: (to) => ({
      path: `/games/name-that-tune/rooms/${encodeURIComponent(to.params.roomId)}`,
      query: to.query,
    }),
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 };
  },
});

export default router;
