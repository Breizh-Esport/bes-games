import { createRouter, createWebHistory } from "vue-router";

import HomePage from "../views/HomePage.vue";
import ProfilePage from "../views/ProfilePage.vue";

// Games
import NameThatTuneLobbyPage from "../views/games/nameThatTune/LobbyPage.vue";
import NameThatTuneRoomPage from "../views/games/nameThatTune/RoomPage.vue";
import NameThatTuneSettingsLayout from "../views/games/nameThatTune/SettingsLayout.vue";
import NameThatTunePlaylistsPage from "../views/games/nameThatTune/settings/PlaylistsPage.vue";

const routes = [
  {
    path: "/",
    name: "home",
    component: HomePage,
  },
  {
    path: "/games/name-that-tune",
    name: "games.nameThatTune.lobby",
    component: NameThatTuneLobbyPage,
  },
  {
    path: "/games/name-that-tune/settings",
    component: NameThatTuneSettingsLayout,
    children: [
      {
        path: "",
        redirect: "/games/name-that-tune/settings/playlists",
      },
      {
        path: "playlists",
        name: "games.nameThatTune.settings.playlists",
        component: NameThatTunePlaylistsPage,
      },
    ],
  },
  {
    path: "/profile",
    name: "profile",
    component: ProfilePage,
  },
  {
    path: "/games/name-that-tune/rooms/:roomId",
    name: "games.nameThatTune.room",
    component: NameThatTuneRoomPage,
    props: (route) => ({
      roomId: route.params.roomId,
      gameId: "name-that-tune",
      mode: route.query.mode,
    }),
  },
  {
    // Legacy route kept for existing links; will likely become `/games/:gameId/rooms/:roomId` later.
    path: "/rooms/:roomId",
    name: "room",
    component: NameThatTuneRoomPage,
    props: (route) => ({
      roomId: route.params.roomId,
      mode: route.query.mode,
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
