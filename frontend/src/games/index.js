import NameThatTuneLobbyPage from "../views/games/nameThatTune/LobbyPage.vue";
import NameThatTuneRoomPage from "../views/games/nameThatTune/RoomPage.vue";
import NameThatTuneSettingsLayout from "../views/games/nameThatTune/SettingsLayout.vue";
import NameThatTunePlaylistsPage from "../views/games/nameThatTune/settings/PlaylistsPage.vue";

const installedGames = [
  {
    id: "name-that-tune",
    name: "Name That Tune",
    description:
      "Guess songs as fast as you can. Rooms, playlists, buzzer, and synchronized playback state.",
    lobbyPath: "/games/name-that-tune",
    settingsPath: "/games/name-that-tune/settings/playlists",
    roomPath(roomId) {
      return `/games/name-that-tune/rooms/${encodeURIComponent(roomId)}`;
    },
  },
];

const installedGameById = new Map(installedGames.map((game) => [game.id, game]));

const gameRoutes = [
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
    path: "/games/name-that-tune/rooms/:roomId",
    name: "games.nameThatTune.room",
    component: NameThatTuneRoomPage,
    props: (route) => ({
      roomId: route.params.roomId,
      gameId: "name-that-tune",
      mode: route.query.mode,
    }),
  },
];

function findInstalledGame(id) {
  return installedGameById.get(id) || null;
}

function supportsGame(id) {
  return installedGameById.has(id);
}

function gameLobbyPath(id) {
  return findInstalledGame(id)?.lobbyPath || null;
}

function gameSettingsPath(id) {
  return findInstalledGame(id)?.settingsPath || null;
}

export {
  findInstalledGame,
  gameLobbyPath,
  gameRoutes,
  gameSettingsPath,
  installedGames,
  supportsGame,
};
