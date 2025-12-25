<template>
    <main class="page">
        <header class="header">
            <div class="brand">
                <div class="row row-center row-gap-sm">
                    <RouterLink class="btn btn-ghost" to="/">Home</RouterLink>
                    <RouterLink class="btn btn-ghost" to="/profile">
                        Profile
                    </RouterLink>
                </div>

                <h1 class="title">
                    Room
                    <span class="muted" v-if="roomId">({{ roomId }})</span>
                </h1>

                <p class="subtitle muted" v-if="snapshot">
                    {{ snapshot.name }} -
                    {{ isOwner ? "Owner view" : "Player view" }}
                </p>
                <p class="subtitle muted" v-else>Loading...</p>
            </div>

            <div class="right">
                <div
                    class="badge"
                    :class="
                        auth.isAuthenticated.value ? 'badge-ok' : 'badge-warn'
                    "
                >
                    <span class="badge-label">Auth</span>
                    <span class="badge-value">{{
                        auth.isAuthenticated.value
                            ? "Authenticated"
                            : "Anonymous"
                    }}</span>
                </div>
                <div class="badge badge-dim">
                    <span class="badge-label">WS</span>
                    <span class="badge-value">{{ wsStatus }}</span>
                </div>
            </div>
        </header>

        <section class="card" v-if="error">
            <h2 class="h2">Error</h2>
            <p class="error">{{ error }}</p>
            <div class="row">
                <button class="btn" @click="reloadAll" :disabled="loadingAny">
                    Retry
                </button>
            </div>
        </section>

        <section class="card" v-else>
            <div class="row row-space">
                <div>
                    <h2 class="h2">Players</h2>
                    <p class="muted">
                        Live roster (name, picture, score). Owner can kick and
                        change score.
                    </p>
                </div>

                <div class="actions">
                    <button
                        class="btn btn-ghost"
                        @click="reloadAll"
                        :disabled="loadingAny"
                    >
                        {{ loadingAny ? "Refreshing..." : "Refresh" }}
                    </button>
                    <button
                        class="btn btn-ghost"
                        @click="copyRoomId"
                        :disabled="!roomId"
                    >
                        Copy room id
                    </button>
                </div>
            </div>

            <div v-if="!snapshot" class="muted">Loading room...</div>

            <div v-else class="roster">
                <div v-if="!snapshot.players?.length" class="muted">
                    No players yet.
                </div>

                <div
                    v-for="p in snapshot.players"
                    :key="p.playerId"
                    class="player"
                >
                    <div class="avatar">
                        <img
                            v-if="p.pictureUrl"
                            :src="p.pictureUrl"
                            alt="avatar"
                        />
                        <div v-else class="avatar-fallback">
                            {{ initialsOf(p.nickname) }}
                        </div>
                    </div>

                    <div class="player-main">
                        <div class="player-name">
                            <span class="name">{{ p.nickname }}</span>
                            <span
                                class="pill"
                                :class="p.connected ? 'pill-ok' : 'pill-dim'"
                                >{{ p.connected ? "online" : "offline" }}</span
                            >
                            <span
                                v-if="p.playerId === playerId"
                                class="pill pill-me"
                                >you</span
                            >
                            <span
                                v-if="p.sub && p.sub === snapshot.ownerSub"
                                class="pill pill-owner"
                                >owner</span
                            >
                        </div>
                        <div class="muted small">
                            <span
                                >score: <strong>{{ p.score }}</strong></span
                            >
                            <span class="dot">-</span>
                            <span class="muted">id: {{ p.playerId }}</span>
                            <span
                                v-if="isOwner && cooldownMsForPlayer(p) > 0"
                                class="dot"
                                >-</span
                            >
                            <span
                                v-if="isOwner && cooldownMsForPlayer(p) > 0"
                                class="muted"
                                >cooldown
                                {{ formatMs(cooldownMsForPlayer(p)) }}</span
                            >
                        </div>
                    </div>

                    <div
                        class="player-actions"
                        v-if="isOwner && p.sub !== snapshot.ownerSub"
                    >
                        <button
                            class="btn btn-ghost"
                            @click="scoreDelta(p.playerId, -1)"
                            :disabled="busyOwner"
                        >
                            -1
                        </button>
                        <button
                            class="btn btn-ghost"
                            @click="scoreDelta(p.playerId, +1)"
                            :disabled="busyOwner"
                        >
                            +1
                        </button>
                        <button
                            class="btn btn-ghost"
                            @click="scoreSetPrompt(p.playerId, p.score)"
                            :disabled="busyOwner"
                        >
                            Set...
                        </button>
                        <button
                            class="btn btn-danger"
                            @click="kick(p.playerId)"
                            :disabled="busyOwner"
                        >
                            Kick
                        </button>
                    </div>
                </div>
            </div>
        </section>

        <section class="card" v-if="showAudioControls">
            <div class="row row-space">
                <div>
                    <h2 class="h2">Audio</h2>
                    <p class="muted">Shared volume setting for all players.</p>
                </div>
            </div>

            <div v-if="!snapshot" class="muted">Loading...</div>

            <template v-else>
                <div class="row">
                    <div class="col">
                        <div class="muted small">Volume</div>
                        <div class="strong">{{ volume }}%</div>
                    </div>

                    <div class="col">
                        <label class="label" for="volume">Volume</label>
                        <input
                            id="volume"
                            class="input"
                            type="range"
                            min="0"
                            max="100"
                            v-model.number="volume"
                        />
                    </div>
                </div>
            </template>
        </section>

        <section class="card" v-if="isOwner">
            <div class="row row-space">
                <div>
                    <h2 class="h2">Playback</h2>
                    <p class="muted">
                        Owner loads a playlist and controls playback.
                    </p>
                </div>
            </div>

            <div v-if="!snapshot" class="muted">Loading...</div>

            <template v-else>
                <p v-if="ownerError" class="error">{{ ownerError }}</p>

                <div class="row">
                    <div class="col">
                        <label class="label" for="playlistSelect"
                            >Playlist</label
                        >
                        <select
                            id="playlistSelect"
                            class="input"
                            v-model="selectedPlaylistId"
                        >
                            <option value="">Select...</option>
                            <option
                                v-for="pl in myPlaylists"
                                :key="pl.id"
                                :value="pl.id"
                            >
                                {{ pl.name }} ({{ pl.items?.length || 0 }})
                            </option>
                        </select>
                    </div>
                </div>

                <div
                    class="playlist-track-list"
                    v-if="snapshot.playlist?.items?.length"
                >
                    <div
                        v-for="(track, index) in snapshot.playlist.items"
                        :key="track.id"
                        class="playlist-track"
                        :class="{
                            'is-current':
                                index === snapshot.playback?.trackIndex,
                            'is-selectable': true,
                        }"
                        @click="selectTrack(index)"
                    >
                        <div class="playlist-track-thumb">
                            <img
                                v-if="track.thumbnailUrl || track.thumbnailURL"
                                :src="track.thumbnailUrl || track.thumbnailURL"
                                alt="thumbnail"
                            />
                            <div v-else class="playlist-track-thumb-fallback">
                                note
                            </div>
                        </div>
                        <div class="playlist-track-main">
                            <div class="playlist-track-title">
                                {{ track.title }}
                            </div>
                            <div class="muted small">#{{ index + 1 }}</div>
                        </div>
                    </div>
                </div>
                <div v-else class="muted">No playlist loaded.</div>

                <div class="divider"></div>

                <div class="playback-controls">
                    <div class="playback-buttons">
                        <button
                            class="icon-btn"
                            @click="prevTrack"
                            :disabled="busyOwner || !canControlPlayback"
                            aria-label="Previous track"
                        >
                            <svg viewBox="0 0 24 24" aria-hidden="true">
                                <path d="M6 6h2v12H6V6zm3 6 9-6v12l-9-6z" />
                            </svg>
                        </button>
                        <button
                            class="icon-btn icon-main"
                            @click="togglePause"
                            :disabled="
                                busyOwner ||
                                !canControlPlayback ||
                                (snapshot.playback?.paused && !canStartPlayback)
                            "
                            aria-label="Play or pause"
                        >
                            <svg
                                v-if="snapshot.playback?.paused"
                                viewBox="0 0 24 24"
                                aria-hidden="true"
                            >
                                <path d="M8 5v14l11-7z" />
                            </svg>
                            <svg v-else viewBox="0 0 24 24" aria-hidden="true">
                                <path d="M6 5h5v14H6V5zm7 0h5v14h-5V5z" />
                            </svg>
                        </button>
                        <button
                            class="icon-btn"
                            @click="nextTrack"
                            :disabled="busyOwner || !canControlPlayback"
                            aria-label="Next track"
                        >
                            <svg viewBox="0 0 24 24" aria-hidden="true">
                                <path d="M16 6h2v12h-2V6zM7 6l9 6-9 6V6z" />
                            </svg>
                        </button>
                        <button
                            class="icon-btn"
                            @click="restartTrack"
                            :disabled="busyOwner || !canControlPlayback"
                            aria-label="Restart track"
                        >
                            <svg viewBox="0 0 24 24" aria-hidden="true">
                                <path
                                    d="M12 5a7 7 0 1 1-6.32 4H3l3.5-3.5L10 9H7.82A5 5 0 1 0 12 7V5z"
                                />
                            </svg>
                        </button>
                    </div>

                    <div class="playback-progress">
                        <div
                            class="progress-bar"
                            :class="durationKnown ? '' : 'progress-unknown'"
                            @click="seekFromBar"
                        >
                            <div
                                class="progress-fill"
                                :style="{ width: `${progressPercent}%` }"
                            ></div>
                        </div>
                        <div class="playback-timer">
                            {{ positionLabel }} / {{ durationLabel }}
                        </div>
                    </div>
                </div>

                <div
                    v-if="waitingForReady"
                    class="playback-waiting muted small"
                >
                    Waiting for
                    {{ waitingForReadyPlayers.length || "players" }} to preload.
                    We'll start as soon as everyone is ready.
                </div>

                <div
                    v-else-if="waitingForBuffer"
                    class="playback-waiting muted small"
                >
                    Waiting for
                    {{ bufferingPlayers.length || "players" }} to buffer. We'll
                    resume as soon as everyone is ready.
                </div>

                <div v-if="isOwner" class="muted small">
                    Ready:
                    {{
                        (snapshot?.players || []).length -
                        waitingForReadyPlayers.length
                    }}/{{ (snapshot?.players || []).length }}
                </div>

                <div class="hint muted small">
                    Playback is synchronized via YouTube with buffered starts
                    and periodic drift correction.
                </div>
            </template>
        </section>

        <section class="card" v-if="shouldShowBuzzer">
            <div class="row row-space">
                <div>
                    <h2 class="h2">Buzzer</h2>
                    <p class="muted">Tap to buzz in.</p>
                </div>
            </div>

            <div v-if="!snapshot" class="muted">Loading...</div>

            <template v-else>
                <p v-if="playerError" class="error">{{ playerError }}</p>

                <div class="row">
                    <div class="col">
                        <div class="muted small">Status</div>
                        <div class="strong">
                            <span v-if="currentPlayerCooldownMs > 0"
                                >Cooldown:
                                {{ formatMs(currentPlayerCooldownMs) }}</span
                            >
                            <span v-else-if="!isPlaybackLive"
                                >Waiting for playback</span
                            >
                            <span v-else>Ready</span>
                        </div>
                    </div>
                    <div class="actions">
                        <button
                            class="btn btn-buzz"
                            @click="buzz"
                            :disabled="buzzerDisabled"
                        >
                            <span v-if="buzzing">Buzzing...</span>
                            <span v-else-if="currentPlayerCooldownMs > 0"
                                >Wait
                                {{ formatMs(currentPlayerCooldownMs) }}</span
                            >
                            <span v-else-if="!isPlaybackLive">Not playing</span>
                            <span v-else>BUZZ</span>
                        </button>
                    </div>
                </div>

                <div v-if="lastBuzz" class="result">
                    <div class="result-line">
                        Last buzz:
                        <strong>{{
                            lastBuzz.player?.nickname ||
                            lastBuzz.player?.Nickname ||
                            "unknown"
                        }}</strong>
                        <span class="muted small">
                            - {{ formatRelative(lastBuzz.ts) }}
                        </span>
                    </div>
                </div>

                <div class="hint muted small">
                    Stay connected; if you disconnect you will be prompted to
                    rejoin.
                </div>
            </template>
        </section>

        <div v-if="buzzModal" class="modal-backdrop">
            <div class="modal-card">
                <h3 class="h3">Buzz!</h3>
                <div class="buzz-modal-player">
                    <div class="avatar">
                        <img
                            v-if="buzzModal.player?.pictureUrl"
                            :src="buzzModal.player.pictureUrl"
                            alt="avatar"
                        />
                        <div v-else class="avatar-fallback">
                            {{ initialsOf(buzzModal.player?.nickname || "") }}
                        </div>
                    </div>
                    <div>
                        <div class="strong">
                            {{
                                buzzModal.player?.nickname ||
                                buzzModal.playerId ||
                                "Unknown"
                            }}
                        </div>
                        <div class="muted small">
                            Buzzed {{ formatRelative(buzzModal.ts) }}
                        </div>
                    </div>
                </div>

                <template v-if="isOwner">
                    <div class="actions">
                        <button
                            class="btn"
                            @click="resolveBuzz(true)"
                            :disabled="resolvingBuzz"
                        >
                            {{ resolvingBuzz ? "Working..." : "Correct" }}
                        </button>
                        <button
                            class="btn btn-ghost"
                            @click="resolveBuzz(false)"
                            :disabled="resolvingBuzz"
                        >
                            Wrong
                        </button>
                    </div>
                </template>
                <template v-else>
                    <p class="muted">
                        Waiting for the host to validate the answer.
                    </p>
                </template>
            </div>
        </div>

        <div class="yt-player" :id="playerContainerId" aria-hidden="true"></div>

        <div v-if="showJoinModal" class="modal-backdrop">
            <div class="modal-card">
                <h3 class="h3">
                    {{ roomClosedReason ? "Room closed" : "Reconnect to play" }}
                </h3>
                <p class="muted" v-if="roomClosedReason">
                    {{ closeReasonMessage }}
                </p>
                <div class="actions" v-if="roomClosedReason">
                    <RouterLink class="btn btn-ghost" to="/">Leave</RouterLink>
                </div>
                <template v-else>
                    <template v-if="wasKicked">
                        <p class="muted">You were kicked by the room owner.</p>
                        <div class="actions">
                            <RouterLink class="btn btn-ghost" to="/"
                                >Leave</RouterLink
                            >
                        </div>
                    </template>
                    <template v-else>
                        <p class="muted" v-if="canSkipNickname">
                            Enter the room password to reconnect.
                        </p>
                        <p class="muted" v-else>
                            Enter a nickname to join this room.
                        </p>
                        <div class="row">
                            <template v-if="!canSkipNickname">
                                <div class="col">
                                    <label class="label" for="modalNick"
                                        >Nickname</label
                                    >
                                    <input
                                        id="modalNick"
                                        v-model="joinNick"
                                        class="input"
                                        type="text"
                                        placeholder="Anonymous"
                                    />
                                </div>
                                <div class="col">
                                    <label class="label" for="modalPic"
                                        >Picture URL</label
                                    >
                                    <input
                                        id="modalPic"
                                        v-model="joinPic"
                                        class="input"
                                        type="url"
                                        placeholder="https://..."
                                    />
                                </div>
                            </template>
                            <div class="col" v-if="requiresRoomPassword">
                                <label class="label" for="modalPassword"
                                    >Room password</label
                                >
                                <input
                                    id="modalPassword"
                                    v-model="joinPassword"
                                    class="input"
                                    type="password"
                                    placeholder="Password"
                                    autocomplete="off"
                                />
                            </div>
                            <div class="actions">
                                <button
                                    class="btn"
                                    @click="joinFromModal"
                                    :disabled="
                                        joining ||
                                        (!canSkipNickname &&
                                            !joinNickTrimmed) ||
                                        (requiresRoomPassword && !joinPassword)
                                    "
                                >
                                    {{ joining ? "Joining..." : "Join room" }}
                                </button>
                                <RouterLink class="btn btn-ghost" to="/"
                                    >Leave</RouterLink
                                >
                            </div>
                        </div>
                        <p v-if="joinError" class="error">{{ joinError }}</p>
                    </template>
                </template>
            </div>
        </div>
    </main>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { api, roomWebSocketUrl } from "../../../lib/api";
import { useAuth } from "../../../stores/auth";

const props = defineProps({
    roomId: { type: String, required: true },
    gameId: { type: String, default: "name-that-tune" },
    // optional hint; real mode is derived from snapshot.ownerSub vs auth.sub
    mode: { type: String, default: "" },
});

const auth = useAuth();

const loadingAny = ref(false);
const error = ref("");
const nowTick = ref(Date.now());

// Room state
const snapshot = ref(null);

// Local join state (client-only)
const playerId = ref("");
const joining = ref(false);
const leaving = ref(false);
const NICK_STORAGE_KEY = "ntt.nickname";
const PIC_STORAGE_KEY = "ntt.pictureUrl";

function loadStoredNickname() {
    try {
        return localStorage.getItem(NICK_STORAGE_KEY) || "";
    } catch {
        return "";
    }
}

function loadStoredPictureUrl() {
    try {
        return localStorage.getItem(PIC_STORAGE_KEY) || "";
    } catch {
        return "";
    }
}

function saveStoredProfile(nickname, pictureUrl) {
    try {
        if (nickname) localStorage.setItem(NICK_STORAGE_KEY, nickname);
        else localStorage.removeItem(NICK_STORAGE_KEY);
        if (pictureUrl) localStorage.setItem(PIC_STORAGE_KEY, pictureUrl);
        else localStorage.removeItem(PIC_STORAGE_KEY);
    } catch {
        // ignore
    }
}

const storedNickname = ref(loadStoredNickname());
const storedPictureUrl = ref(loadStoredPictureUrl());
const hasStoredNickname = computed(() => !!(storedNickname.value || "").trim());

const joinNick = ref(storedNickname.value);
const joinPic = ref(storedPictureUrl.value);
const joinNickTrimmed = computed(() => (joinNick.value || "").trim());
const joinPassword = ref("");
const joinError = ref("");
const roomClosedReason = ref("");
const wasKicked = ref(false);

// Owner controls / playlists
const myPlaylists = ref([]);
const selectedPlaylistId = ref("");
const suppressPlaylistLoad = ref(false);
const busyOwner = ref(false);
const ownerError = ref("");

// Player controls
const volume = ref(65);
const buzzing = ref(false);
const playerError = ref("");
const buzzModal = ref(null);
const resolvingBuzz = ref(false);
const cooldownByPlayer = ref({});
const playerDurationSec = ref(0);

// YouTube playback
const playerContainerId = computed(() => `yt-player-${props.roomId}`);
const ytPlayer = ref(null);
const ytReady = ref(false);
const currentVideoId = ref("");
let ytLoadPromise = null;
let syncTimer = null;
let nowTimer = null;
let startTimer = null;
let scheduledStartAt = null;
let lastBufferingReport = null;
let lastPlaybackStamp = "";
let lastPlaybackPaused = null;
let preloadReadyTimer = null;
let lastReadyReport = "";
const PRELOAD_READY_FRACTION = 0.2;
const PRELOAD_READY_MAX_WAIT_MS = 2000;
let preloadStartAtMs = 0;
let bufferingDelayTimer = null;
const BUFFER_REPORT_GRACE_MS = 1000;

// Buzzer events
const lastBuzz = ref(null);

// WS
const wsStatus = ref("disconnected");
const lastRealtimeError = ref("");
let ws = null;
const ownerActions = new Set([
    "kick",
    "score.add",
    "score.set",
    "playlist.load",
    "playback.set",
    "playback.pause",
    "playback.seek",
    "buzz.resolve",
]);
const playerActions = new Set(["buzz", "playback.buffer", "playback.ready"]);

const PLAYER_STORAGE_PREFIX = "ntt.player.";
const PLAYER_TOKEN_PREFIX = "ntt.playerToken.";
const OWNER_TOKEN_PREFIX = "ntt.ownerToken.";
function storageKey() {
    return `${PLAYER_STORAGE_PREFIX}${props.roomId}`;
}
function tokenStorageKey(prefix) {
    return `${prefix}${props.roomId}`;
}
function loadStoredPlayerId() {
    try {
        return sessionStorage.getItem(storageKey()) || "";
    } catch {
        return "";
    }
}
function setPlayerId(id) {
    const v = id || "";
    playerId.value = v;
    try {
        if (v) sessionStorage.setItem(storageKey(), v);
        else sessionStorage.removeItem(storageKey());
    } catch {
        // ignore
    }
    return v;
}

const playerToken = ref("");
const ownerToken = ref("");
function loadStoredToken(prefix) {
    try {
        return sessionStorage.getItem(tokenStorageKey(prefix)) || "";
    } catch {
        return "";
    }
}
function setPlayerToken(token) {
    const v = token || "";
    playerToken.value = v;
    try {
        if (v) sessionStorage.setItem(tokenStorageKey(PLAYER_TOKEN_PREFIX), v);
        else sessionStorage.removeItem(tokenStorageKey(PLAYER_TOKEN_PREFIX));
    } catch {
        // ignore
    }
    return v;
}
function setOwnerToken(token) {
    const v = token || "";
    ownerToken.value = v;
    try {
        if (v) sessionStorage.setItem(tokenStorageKey(OWNER_TOKEN_PREFIX), v);
        else sessionStorage.removeItem(tokenStorageKey(OWNER_TOKEN_PREFIX));
    } catch {
        // ignore
    }
    return v;
}

setPlayerId(loadStoredPlayerId());
setPlayerToken(loadStoredToken(PLAYER_TOKEN_PREFIX));
setOwnerToken(loadStoredToken(OWNER_TOKEN_PREFIX));

const canControlPlayback = computed(
    () =>
        !!snapshot.value?.playlist?.playlistId &&
        (snapshot.value?.playlist?.items?.length || 0) > 0 &&
        !roomClosedReason.value,
);
const playbackStartAtMs = computed(() => {
    const raw = snapshot.value?.playback?.startAt;
    if (!raw) return null;
    const parsed = Date.parse(raw);
    return Number.isFinite(parsed) ? parsed : null;
});
const playbackPositionMs = computed(() => {
    const base = snapshot.value?.playback?.positionMs || 0;
    if (snapshot.value?.playback?.paused) return base;
    const startAt = playbackStartAtMs.value;
    const updatedAt = Date.parse(snapshot.value?.playback?.updatedAt);
    const reference = Number.isFinite(startAt) ? startAt : updatedAt;
    if (!Number.isFinite(reference)) return base;
    const delta = Math.max(0, nowTick.value - reference);
    return base + delta;
});
const durationKnown = computed(() => {
    const fromSnapshot = snapshot.value?.playback?.track?.durationSec || 0;
    const fromPlayer = playerDurationSec.value || 0;
    return Math.max(fromSnapshot, fromPlayer) > 0;
});
const effectiveDurationMs = computed(() => {
    const durationSec = Math.max(
        snapshot.value?.playback?.track?.durationSec || 0,
        playerDurationSec.value || 0,
    );
    if (durationSec > 0) return durationSec * 1000;
    return 180000;
});
const progressPercent = computed(() => {
    const durationMs = effectiveDurationMs.value || 1;
    const positionMs = playbackPositionMs.value || 0;
    const pct = Math.min(1, Math.max(0, positionMs / durationMs));
    return Math.round(pct * 100);
});
const positionLabel = computed(() =>
    formatDuration(Math.floor((playbackPositionMs.value || 0) / 1000)),
);
const durationLabel = computed(() => {
    const durationSec = Math.max(
        snapshot.value?.playback?.track?.durationSec || 0,
        playerDurationSec.value || 0,
    );
    return durationSec > 0 ? formatDuration(durationSec) : "--:--";
});
const isOwner = computed(() => {
    const sub = auth.state.sub;
    const ownerSub = snapshot.value?.ownerSub;
    if (!sub || !ownerSub) return false;
    return sub === ownerSub;
});
const canSkipNickname = computed(
    () => auth.isAuthenticated.value && isOwner.value,
);

const currentPlayer = computed(() => {
    if (!snapshot.value?.players) return null;
    if (playerId.value) {
        const match = snapshot.value.players.find(
            (p) => p.playerId === playerId.value,
        );
        if (match) return match;
    }
    const sub = auth.state.sub;
    if (sub) {
        const match = snapshot.value.players.find((p) => p.sub === sub);
        if (match) return match;
    }
    const nick = (storedNickname.value || "").trim();
    if (nick) {
        const match = snapshot.value.players.find((p) => p.nickname === nick);
        if (match) return match;
    }
    return null;
});

const currentPlayerConnected = computed(
    () => !!currentPlayer.value && currentPlayer.value.connected,
);

const requiresRoomPassword = computed(() => !!snapshot.value?.hasPassword);
const shouldShowBuzzer = computed(() => {
    if (isOwner.value) return false;
    const ownerSub = snapshot.value?.ownerSub;
    if (!ownerSub) return true;
    return (auth.state.sub || "") !== ownerSub;
});
const showAudioControls = computed(
    () => shouldShowBuzzer.value || isOwner.value,
);
const currentPlayerCooldownMs = computed(() =>
    cooldownMsForPlayer(currentPlayer.value),
);
const bufferingPlayers = computed(
    () => snapshot.value?.playback?.bufferingPlayers || [],
);
const waitingForBuffer = computed(
    () => !!snapshot.value?.playback?.waitingForBuffer,
);
const waitingForReady = computed(
    () => !!snapshot.value?.playback?.waitingForReady,
);
const waitingForReadyPlayers = computed(
    () => snapshot.value?.playback?.waitingForReadyPlayers || [],
);
const canStartPlayback = computed(
    () => waitingForReadyPlayers.value.length === 0,
);
const isPlaybackLive = computed(() => {
    if (!snapshot.value?.playback?.track) return false;
    if (snapshot.value?.playback?.paused) return false;
    const startAt = playbackStartAtMs.value;
    if (Number.isFinite(startAt) && startAt > nowTick.value) return false;
    return true;
});
const buzzerDisabled = computed(
    () =>
        !currentPlayerConnected.value ||
        buzzing.value ||
        !!roomClosedReason.value ||
        currentPlayerCooldownMs.value > 0 ||
        !isPlaybackLive.value,
);

const showJoinModal = computed(() => {
    if (roomClosedReason.value) return true;
    if (!snapshot.value) return false;
    if (wasKicked.value) return true;
    if (currentPlayerConnected.value) return false;
    if (canSkipNickname.value) return requiresRoomPassword.value;
    if (requiresRoomPassword.value) return true;
    return !hasStoredNickname.value;
});

const closeReasonMessage = computed(() => {
    switch (roomClosedReason.value) {
        case "owner_timeout":
            return "The owner left and did not return within 10 minutes. The room was closed.";
        case "owner_left_empty":
            return "The owner left the room and nobody was connected, so the room was closed.";
        case "":
            return "";
        default:
            return "The room was closed.";
    }
});

async function reloadAll() {
    loadingAny.value = true;
    error.value = "";
    try {
        snapshot.value = await api.getRoom(props.gameId, props.roomId);
        roomClosedReason.value = "";
        syncPlayerFromSnapshot();
        // if owner, pull playlists for load action
        if (
            auth.isAuthenticated.value &&
            snapshot.value?.ownerSub === auth.state.sub
        ) {
            await refreshMyPlaylists();
        }
    } catch (e) {
        if (e?.status === 404) {
            roomClosedReason.value =
                roomClosedReason.value || "owner_left_empty";
        }
        error.value = e?.message || "Failed to load room";
    } finally {
        loadingAny.value = false;
    }
}

function syncPlayerFromSnapshot() {
    if (!snapshot.value?.players) return;
    if (playerId.value) {
        const exists = snapshot.value.players.some(
            (p) => p.playerId === playerId.value,
        );
        if (!exists) {
            if (!leaving.value) {
                wasKicked.value = true;
            }
            setPlayerId("");
        }
        return;
    }
    const sub = auth.state.sub;
    if (sub) {
        const match = snapshot.value.players.find((p) => p.sub === sub);
        if (match) {
            setPlayerId(match.playerId);
        }
    }
}

function getTrackVideoId(track) {
    if (!track) return "";
    return (
        track.youTubeID ||
        track.youtubeId ||
        track.youTubeId ||
        track.youtubeID ||
        ""
    );
}

function cooldownMsForPlayer(player) {
    if (!player?.playerId) return 0;
    const untilISO = cooldownByPlayer.value[player.playerId];
    if (!untilISO) return 0;
    const until = Date.parse(untilISO);
    if (!Number.isFinite(until)) return 0;
    return Math.max(0, until - nowTick.value);
}

function getDesiredPlayback() {
    const track = snapshot.value?.playback?.track;
    const videoId = getTrackVideoId(track);
    if (!videoId) return null;
    const paused = !!snapshot.value?.playback?.paused;
    const base = snapshot.value?.playback?.positionMs || 0;
    const startAtMs = playbackStartAtMs.value;
    const updatedAt = Date.parse(snapshot.value?.playback?.updatedAt);
    const reference = Number.isFinite(startAtMs) ? startAtMs : updatedAt;
    const delta =
        !paused && Number.isFinite(reference)
            ? Math.max(0, nowTick.value - reference)
            : 0;
    return {
        videoId,
        paused,
        targetMs: base + delta,
        startAtMs: Number.isFinite(startAtMs) ? startAtMs : null,
    };
}

function loadYouTubeAPI() {
    if (ytLoadPromise) return ytLoadPromise;
    ytLoadPromise = new Promise((resolve) => {
        if (window.YT && window.YT.Player) {
            resolve(window.YT);
            return;
        }

        const existing = document.querySelector("script[data-yt-iframe]");
        if (!existing) {
            const script = document.createElement("script");
            script.src = "https://www.youtube.com/iframe_api";
            script.async = true;
            script.defer = true;
            script.dataset.ytIframe = "true";
            document.head.appendChild(script);
        }

        const previous = window.onYouTubeIframeAPIReady;
        window.onYouTubeIframeAPIReady = () => {
            if (typeof previous === "function") previous();
            resolve(window.YT);
        };
    });
    return ytLoadPromise;
}

async function setupYouTubePlayer() {
    const container = document.getElementById(playerContainerId.value);
    if (!container) return;
    const YT = await loadYouTubeAPI();
    ytReady.value = false;
    ytPlayer.value = new YT.Player(container, {
        height: "0",
        width: "0",
        playerVars: {
            autoplay: 0,
            controls: 0,
            disablekb: 1,
            iv_load_policy: 3,
            modestbranding: 1,
            playsinline: 1,
            vq: "tiny",
        },
        events: {
            onReady: () => {
                ytReady.value = true;
                applyPlaybackQualityLow();
                syncPlayerToSnapshot();
            },
            onStateChange: handleYTStateChange,
        },
    });
}

function destroyYouTubePlayer() {
    if (!ytPlayer.value) return;
    try {
        ytPlayer.value.destroy();
    } catch {
        // ignore
    }
    ytPlayer.value = null;
    ytReady.value = false;
    currentVideoId.value = "";
    clearScheduledStart();
}

function clearScheduledStart() {
    if (startTimer) {
        clearTimeout(startTimer);
    }
    startTimer = null;
    scheduledStartAt = null;
}

function startPreloadReadyPolling() {
    if (preloadReadyTimer) {
        clearTimeout(preloadReadyTimer);
    }
    if (!preloadStartAtMs) {
        preloadStartAtMs = Date.now();
    }
    preloadReadyTimer = setTimeout(() => {
        if (!ytReady.value || !ytPlayer.value) return;
        const desired = getDesiredPlayback();
        if (!desired || desired.videoId !== currentVideoId.value) {
            startPreloadReadyPolling();
            return;
        }
        const state = ytPlayer.value.getPlayerState
            ? ytPlayer.value.getPlayerState()
            : null;
        const YT = window.YT;
        const stateReady =
            state === YT?.PlayerState?.CUED ||
            state === YT?.PlayerState?.UNSTARTED ||
            state === YT?.PlayerState?.PAUSED ||
            state === YT?.PlayerState?.PLAYING;
        if (!stateReady) {
            startPreloadReadyPolling();
            return;
        }
        const fraction =
            typeof ytPlayer.value.getVideoLoadedFraction === "function"
                ? ytPlayer.value.getVideoLoadedFraction()
                : 0;
        if (Number.isFinite(fraction) && fraction >= PRELOAD_READY_FRACTION) {
            reportPlaybackReady(true);
            preloadReadyTimer = null;
            preloadStartAtMs = 0;
            return;
        }
        if (
            Date.now() - preloadStartAtMs >= PRELOAD_READY_MAX_WAIT_MS &&
            stateReady
        ) {
            reportPlaybackReady(true);
            preloadReadyTimer = null;
            preloadStartAtMs = 0;
            return;
        }
        startPreloadReadyPolling();
    }, 250);
}

function reportPlaybackReady(ready) {
    if (!currentPlayerConnected.value || !playerId.value) return;
    const updatedAt = snapshot.value?.playback?.updatedAt;
    if (!updatedAt) return;
    const token = `${ready}:${updatedAt}`;
    if (lastReadyReport === token) return;
    const sent = sendRoomCommand("playback.ready", {
        playerId: playerId.value,
        ready,
        playbackUpdatedAt: updatedAt,
    });
    if (!sent) return;
    lastReadyReport = token;
}

function scheduleStart(startAtMs, targetSec) {
    if (!Number.isFinite(startAtMs)) return;
    if (scheduledStartAt === startAtMs) return;
    clearScheduledStart();
    const delay = Math.max(0, startAtMs - Date.now());
    scheduledStartAt = startAtMs;
    startTimer = setTimeout(() => {
        if (!ytPlayer.value) return;
        try {
            ytPlayer.value.seekTo(targetSec, true);
        } catch {
            // ignore
        }
        try {
            ytPlayer.value.playVideo();
        } catch {
            // ignore
        }
    }, delay);
}

function applyPlaybackQualityLow() {
    if (!ytPlayer.value?.setPlaybackQuality) return;
    try {
        ytPlayer.value.setPlaybackQuality("tiny");
    } catch {
        // ignore
    }
}

function updatePlayerDuration() {
    if (!ytReady.value || !ytPlayer.value?.getDuration) return;
    const duration = ytPlayer.value.getDuration();
    if (Number.isFinite(duration) && duration > 0) {
        playerDurationSec.value = duration;
    }
}

function reportPlaybackBuffering(buffering) {
    if (!currentPlayerConnected.value || !playerId.value) return;
    if (lastBufferingReport === buffering) return;
    const sent = sendRoomCommand("playback.buffer", {
        playerId: playerId.value,
        buffering,
    });
    if (!sent) return;
    lastBufferingReport = buffering;
}

function handleYTStateChange(evt) {
    const state = evt?.data;
    const YT = window.YT;
    if (!YT?.PlayerState) return;
    if (state === YT.PlayerState.BUFFERING) {
        if (!isPlaybackLive.value) return;
        const startAt = playbackStartAtMs.value;
        const delay =
            Number.isFinite(startAt) &&
            nowTick.value < startAt + BUFFER_REPORT_GRACE_MS
                ? startAt + BUFFER_REPORT_GRACE_MS - nowTick.value
                : 0;
        if (delay > 0) {
            if (bufferingDelayTimer) {
                clearTimeout(bufferingDelayTimer);
            }
            bufferingDelayTimer = setTimeout(() => {
                if (!ytPlayer.value?.getPlayerState) return;
                const current = ytPlayer.value.getPlayerState();
                if (current === YT.PlayerState.BUFFERING) {
                    reportPlaybackBuffering(true);
                }
            }, delay);
            return;
        }
        reportPlaybackBuffering(true);
        return;
    }
    if (bufferingDelayTimer) {
        clearTimeout(bufferingDelayTimer);
        bufferingDelayTimer = null;
    }
    if (state === YT.PlayerState.CUED) {
        if (!isPlaybackLive.value) return;
        reportPlaybackBuffering(false);
        return;
    }
    if (state === YT.PlayerState.PLAYING) {
        reportPlaybackBuffering(false);
    }
}

function syncPlayerToSnapshot() {
    if (!ytReady.value || !ytPlayer.value) return;
    const desired = getDesiredPlayback();
    const playbackStamp = snapshot.value?.playback?.updatedAt || "";
    const playbackPaused = snapshot.value?.playback?.paused ?? null;
    if (
        playbackStamp !== lastPlaybackStamp ||
        playbackPaused !== lastPlaybackPaused
    ) {
        lastPlaybackStamp = playbackStamp;
        lastPlaybackPaused = playbackPaused;
        lastBufferingReport = null;
        lastReadyReport = "";
    }
    if (!desired) {
        if (currentVideoId.value) {
            try {
                ytPlayer.value.stopVideo();
            } catch {
                // ignore
            }
        }
        currentVideoId.value = "";
        clearScheduledStart();
        reportPlaybackBuffering(false);
        return;
    }

    const targetSec = Math.max(0, desired.targetMs / 1000);
    if (currentVideoId.value !== desired.videoId) {
        currentVideoId.value = desired.videoId;
        clearScheduledStart();
        lastBufferingReport = null;
        playerDurationSec.value = 0;
        preloadStartAtMs = 0;
        ytPlayer.value.cueVideoById({
            videoId: desired.videoId,
            startSeconds: targetSec,
        });
        applyPlaybackQualityLow();
        if (typeof ytPlayer.value.setVolume === "function") {
            ytPlayer.value.setVolume(volume.value);
            ytPlayer.value.unMute();
        }
        const waitingForStart =
            !desired.paused &&
            Number.isFinite(desired.startAtMs) &&
            desired.startAtMs > nowTick.value;
        if (!desired.paused && !waitingForStart) {
            ytPlayer.value.seekTo(targetSec, true);
            ytPlayer.value.playVideo();
        } else {
            ytPlayer.value.pauseVideo();
            if (waitingForStart) {
                scheduleStart(desired.startAtMs, targetSec);
            }
        }
        updatePlayerDuration();
        startPreloadReadyPolling();
        const state = ytPlayer.value.getPlayerState
            ? ytPlayer.value.getPlayerState()
            : null;
        if (state === window.YT?.PlayerState?.BUFFERING) {
            if (!desired.paused && !waitingForStart) {
                reportPlaybackBuffering(true);
            }
        } else if (state !== null) {
            reportPlaybackBuffering(false);
        }
        return;
    }

    const state = ytPlayer.value.getPlayerState
        ? ytPlayer.value.getPlayerState()
        : null;
    const waitingForStart =
        !desired.paused &&
        Number.isFinite(desired.startAtMs) &&
        desired.startAtMs > nowTick.value;
    if (desired.paused) {
        clearScheduledStart();
        if (state !== window.YT?.PlayerState?.PAUSED) {
            ytPlayer.value.pauseVideo();
        }
        startPreloadReadyPolling();
        if (ytPlayer.value.setPlaybackRate) {
            ytPlayer.value.setPlaybackRate(1);
        }
    } else if (waitingForStart) {
        if (state !== window.YT?.PlayerState?.PAUSED) {
            ytPlayer.value.pauseVideo();
        }
        scheduleStart(desired.startAtMs, targetSec);
        startPreloadReadyPolling();
        if (ytPlayer.value.setPlaybackRate) {
            ytPlayer.value.setPlaybackRate(1);
        }
    } else if (state !== window.YT?.PlayerState?.PLAYING) {
        clearScheduledStart();
        ytPlayer.value.playVideo();
    }

    if (typeof ytPlayer.value.setVolume === "function") {
        ytPlayer.value.setVolume(volume.value);
        ytPlayer.value.unMute();
    }
    applyPlaybackQualityLow();
    updatePlayerDuration();

    const currentSec = ytPlayer.value.getCurrentTime
        ? ytPlayer.value.getCurrentTime()
        : null;
    if (Number.isFinite(currentSec)) {
        const drift = currentSec - targetSec;
        const absDrift = Math.abs(drift);
        if (absDrift > 0.2) {
            ytPlayer.value.seekTo(targetSec, true);
        } else if (absDrift > 0.05 && ytPlayer.value.setPlaybackRate) {
            const rate = drift > 0 ? 0.95 : 1.05;
            ytPlayer.value.setPlaybackRate(rate);
        } else if (ytPlayer.value.setPlaybackRate) {
            ytPlayer.value.setPlaybackRate(1);
        }
    }

    if (state === window.YT?.PlayerState?.BUFFERING) {
        if (!desired.paused && !waitingForStart) {
            reportPlaybackBuffering(true);
        }
    } else if (state !== null) {
        reportPlaybackBuffering(false);
    }
}

async function refreshMyPlaylists() {
    ownerError.value = "";
    try {
        const res = await api.listPlaylists(props.gameId);
        myPlaylists.value = Array.isArray(res?.playlists) ? res.playlists : [];
    } catch (e) {
        ownerError.value = e?.message || "Failed to load playlists";
    }
}

async function loadSelectedPlaylist() {
    if (!selectedPlaylistId.value) return;
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await ensureRoomCommand("playlist.load", {
            playlistId: selectedPlaylistId.value,
        });
        // Do not apply REST response to local room state.
        // Room state (playlist, playback, players, scores) is driven by WebSocket snapshots.
    } catch (e) {
        ownerError.value = e?.message || "Failed to load playlist";
    } finally {
        busyOwner.value = false;
    }
}

async function togglePause() {
    busyOwner.value = true;
    ownerError.value = "";
    try {
        const paused = !snapshot.value?.playback?.paused;
        if (paused) {
            const trackIndex = snapshot.value?.playback?.trackIndex ?? 0;
            const positionMs = Math.floor(playbackPositionMs.value || 0);
            await ensureRoomCommand("playback.set", {
                trackIndex,
                paused: true,
                positionMs,
            });
        } else {
            await ensureRoomCommand("playback.pause", { paused });
        }
        // Do not apply REST response; wait for WS `room.snapshot`.
    } catch (e) {
        ownerError.value = e?.message || "Failed to toggle pause";
    } finally {
        busyOwner.value = false;
    }
}

async function setTrackIndex(
    trackIndex,
    { paused = undefined, positionMs = undefined } = {},
) {
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await ensureRoomCommand("playback.set", {
            trackIndex,
            paused,
            positionMs,
        });
        // Do not apply REST response; wait for WS `room.snapshot`.
    } catch (e) {
        ownerError.value = e?.message || "Failed to set playback";
    } finally {
        busyOwner.value = false;
    }
}

function prevTrack() {
    const idx = snapshot.value?.playback?.trackIndex ?? 0;
    const next = Math.max(0, idx - 1);
    setTrackIndex(next, { positionMs: 0 });
}

function nextTrack() {
    const idx = snapshot.value?.playback?.trackIndex ?? 0;
    const max = (snapshot.value?.playlist?.items?.length || 1) - 1;
    const next = Math.min(max, idx + 1);
    setTrackIndex(next, { positionMs: 0 });
}

function restartTrack() {
    const idx = snapshot.value?.playback?.trackIndex ?? 0;
    setTrackIndex(idx, { positionMs: 0 });
}

function selectTrack(index) {
    if (busyOwner.value || !canControlPlayback.value) return;
    setTrackIndex(index, { paused: true, positionMs: 0 });
}

async function seekToMs(positionMs) {
    if (!Number.isFinite(positionMs) || positionMs < 0) return;
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await ensureRoomCommand("playback.seek", {
            positionMs: Math.floor(positionMs),
        });
        // Do not apply REST response; wait for WS `room.snapshot`.
    } catch (e) {
        ownerError.value = e?.message || "Failed to seek";
    } finally {
        busyOwner.value = false;
    }
}

function seekFromBar(event) {
    if (busyOwner.value || !canControlPlayback.value) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const x = Math.min(Math.max(event.clientX - rect.left, 0), rect.width);
    const ratio = rect.width ? x / rect.width : 0;
    const targetMs = Math.floor(effectiveDurationMs.value * ratio);
    seekToMs(targetMs);
}

// Owner: roster controls
async function kick(pid) {
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await ensureRoomCommand("kick", { playerId: pid });
        // Do not apply REST response; roster updates come via WS `room.snapshot`.
    } catch (e) {
        ownerError.value = e?.message || "Failed to kick player";
    } finally {
        busyOwner.value = false;
    }
}

async function scoreDelta(pid, delta) {
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await ensureRoomCommand("score.add", { playerId: pid, delta });
        // Do not apply REST response; score updates come via WS `room.snapshot`.
    } catch (e) {
        ownerError.value = e?.message || "Failed to update score";
    } finally {
        busyOwner.value = false;
    }
}

async function scoreSetPrompt(pid, current) {
    const raw = window.prompt("Set score:", String(current ?? 0));
    if (raw == null) return;
    const n = Number(raw);
    if (!Number.isFinite(n)) return;
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await ensureRoomCommand("score.set", {
            playerId: pid,
            score: Math.trunc(n),
        });
        // Do not apply REST response; score updates come via WS `room.snapshot`.
    } catch (e) {
        ownerError.value = e?.message || "Failed to set score";
    } finally {
        busyOwner.value = false;
    }
}

// Player: join/leave + buzzer
async function joinRoomWithProfile({
    nickname,
    pictureUrl,
    password,
    persist = false,
}) {
    if (!props.roomId || roomClosedReason.value) return;
    if (
        currentPlayerConnected.value &&
        playerToken.value &&
        (!isOwner.value || ownerToken.value)
    ) {
        return;
    }
    if (wasKicked.value) return;
    const safeNickname = (nickname || "").trim();
    joining.value = true;
    joinError.value = "";
    playerError.value = "";
    try {
        const res = await api.joinRoom(props.gameId, props.roomId, {
            nickname: safeNickname || undefined,
            pictureUrl: pictureUrl || undefined,
            password: password || undefined,
        });
        setPlayerId(res?.PlayerID || res?.playerId || "");
        setPlayerToken(res?.PlayerToken || res?.playerToken || "");
        if (res?.OwnerToken || res?.ownerToken) {
            setOwnerToken(res?.OwnerToken || res?.ownerToken || "");
        } else {
            setOwnerToken("");
        }
        wasKicked.value = false;
        roomClosedReason.value = "";
        if (persist) {
            saveStoredProfile(safeNickname || "", pictureUrl || "");
            storedNickname.value = safeNickname || "";
            storedPictureUrl.value = pictureUrl || "";
        }
        if (res?.snapshot) {
            snapshot.value = res.snapshot;
        }
        joinPassword.value = "";
        syncPlayerFromSnapshot();
    } catch (e) {
        joinError.value = e?.message || "Failed to join room";
    } finally {
        joining.value = false;
    }
}

async function joinFromModal() {
    const nickname = joinNickTrimmed.value;
    if (!canSkipNickname.value && !nickname) return;
    await joinRoomWithProfile({
        nickname: canSkipNickname.value ? "" : nickname,
        pictureUrl: canSkipNickname.value ? "" : joinPic.value,
        password: joinPassword.value,
        persist: !canSkipNickname.value,
    });
}

async function maybeAutoJoin() {
    if (!snapshot.value) return;
    if (roomClosedReason.value) return;
    if (
        currentPlayerConnected.value &&
        playerToken.value &&
        (!isOwner.value || ownerToken.value)
    ) {
        return;
    }
    if (requiresRoomPassword.value) return;
    if (joining.value) return;
    if (canSkipNickname.value) {
        await joinRoomWithProfile({
            nickname: "",
            pictureUrl: "",
            persist: false,
        });
        return;
    }
    if (!hasStoredNickname.value) return;
    await joinRoomWithProfile({
        nickname: storedNickname.value,
        pictureUrl: storedPictureUrl.value,
        persist: false,
    });
}

async function leave({ silent = false } = {}) {
    if (!playerId.value) return;
    leaving.value = !silent;
    playerError.value = "";
    try {
        await api.leaveRoom(props.gameId, props.roomId, {
            playerId: playerId.value,
        });
        setPlayerId("");
        setPlayerToken("");
        setOwnerToken("");
    } catch (e) {
        if (!silent) {
            playerError.value = e?.message || "Failed to leave room";
        }
    } finally {
        if (!silent) {
            leaving.value = false;
        }
    }
}

async function buzz() {
    if (!currentPlayerConnected.value) {
        playerError.value = "Join the room to buzz.";
        return;
    }
    buzzing.value = true;
    playerError.value = "";
    try {
        await ensureRoomCommand("buzz", {
            playerId: playerId.value,
        });
    } catch (e) {
        playerError.value = e?.message || "Failed to buzz";
    } finally {
        buzzing.value = false;
    }
}

async function resolveBuzz(correct) {
    if (!buzzModal.value?.playerId) return;
    resolvingBuzz.value = true;
    ownerError.value = "";
    try {
        await ensureRoomCommand("buzz.resolve", {
            playerId: buzzModal.value.playerId,
            correct,
        });
        buzzModal.value = null;
    } catch (e) {
        ownerError.value = e?.message || "Failed to resolve buzz";
    } finally {
        resolvingBuzz.value = false;
    }
}

// WS connection
function realtimeErrorFor(action) {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        return "Realtime connection required. Please wait for the room connection to finish.";
    }
    if (ownerActions.has(action) && !ownerToken.value) {
        return "Join the room to control playback.";
    }
    if (playerActions.has(action) && !playerToken.value) {
        return "Join the room to use the buzzer.";
    }
    return "Realtime connection required.";
}

function sendRoomCommand(action, payload) {
    lastRealtimeError.value = "";
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        lastRealtimeError.value = realtimeErrorFor(action);
        return false;
    }
    const command = { action, ...payload };
    if (ownerActions.has(action)) {
        if (!ownerToken.value) {
            lastRealtimeError.value = realtimeErrorFor(action);
            return false;
        }
        command.ownerToken = ownerToken.value;
    }
    if (playerActions.has(action)) {
        if (!playerToken.value) {
            lastRealtimeError.value = realtimeErrorFor(action);
            return false;
        }
        command.playerToken = playerToken.value;
    }
    try {
        ws.send(
            JSON.stringify({
                type: "room.command",
                roomId: props.roomId,
                payload: command,
            }),
        );
        return true;
    } catch {
        lastRealtimeError.value = realtimeErrorFor(action);
        return false;
    }
}

async function ensureRoomCommand(action, payload) {
    if (wsStatus.value === "disconnected" || wsStatus.value === "error") {
        connectWS();
    }
    if (wsStatus.value !== "connected") {
        const timeoutMs = 1200;
        const start = Date.now();
        while (
            wsStatus.value !== "connected" &&
            Date.now() - start < timeoutMs
        ) {
            await new Promise((resolve) => setTimeout(resolve, 50));
        }
    }
    const sent = sendRoomCommand(action, payload);
    if (!sent) {
        throw new Error(
            lastRealtimeError.value || "Realtime connection required.",
        );
    }
}

function connectWS() {
    disconnectWS();

    const url = roomWebSocketUrl(props.gameId, props.roomId);
    wsStatus.value = "connecting";

    ws = new WebSocket(url);

    ws.onopen = () => {
        wsStatus.value = "connected";
    };

    ws.onclose = () => {
        wsStatus.value = "disconnected";
        ws = null;
    };

    ws.onerror = () => {
        wsStatus.value = "error";
    };

    ws.onmessage = (evt) => {
        try {
            const msg = JSON.parse(evt.data);
            // Backend event shape: { type, roomId, ts, payload }
            if (msg?.type === "room.snapshot") {
                snapshot.value = msg.payload;
                roomClosedReason.value = "";
                syncPlayerFromSnapshot();
                if (snapshot.value?.players) {
                    const active = new Set(
                        snapshot.value.players.map((p) => p.playerId),
                    );
                    const next = {};
                    Object.entries(cooldownByPlayer.value).forEach(
                        ([pid, until]) => {
                            if (active.has(pid)) {
                                next[pid] = until;
                            }
                        },
                    );
                    cooldownByPlayer.value = next;
                }
                return;
            }

            if (msg?.type === "room.command.error") {
                const action = msg?.payload?.action || "";
                const message =
                    msg?.payload?.message || "Command failed. Try again.";
                if (ownerActions.has(action)) {
                    ownerError.value = message;
                } else {
                    playerError.value = message;
                }
                return;
            }

            if (msg?.type === "playback.preload") {
                lastReadyReport = "";
                preloadStartAtMs = 0;
                syncPlayerToSnapshot();
                startPreloadReadyPolling();
                return;
            }

            if (msg?.type === "room.closed") {
                roomClosedReason.value = msg?.payload?.reason || "closed";
                return;
            }

            // Buzzer event: { type: "buzzer", payload: { player: ... } }
            if (msg?.type === "buzzer") {
                lastBuzz.value = {
                    ts: msg.ts || new Date().toISOString(),
                    player: msg.payload?.player,
                };
                buzzModal.value = {
                    ts: msg.ts || new Date().toISOString(),
                    player: msg.payload?.player,
                    playerId:
                        msg.payload?.player?.playerId ||
                        msg.payload?.playerId ||
                        "",
                };
                return;
            }

            if (msg?.type === "buzzer.resolved") {
                buzzModal.value = null;
                if (msg.payload?.playerId) {
                    const next = { ...cooldownByPlayer.value };
                    delete next[msg.payload.playerId];
                    cooldownByPlayer.value = next;
                }
                return;
            }

            if (msg?.type === "buzzer.cooldown") {
                const pid = msg.payload?.playerId;
                const until = msg.payload?.until;
                if (pid && until) {
                    cooldownByPlayer.value = {
                        ...cooldownByPlayer.value,
                        [pid]: until,
                    };
                }
                return;
            }
        } catch {
            // ignore bad frames
        }
    };
}

function disconnectWS() {
    if (!ws) return;
    try {
        ws.close();
    } catch {
        // ignore
    }
    ws = null;
    wsStatus.value = "disconnected";
}

// Misc
function initialsOf(name) {
    const n = (name || "").trim();
    if (!n) return "A";
    const parts = n.split(/\s+/).filter(Boolean);
    const a = parts[0]?.[0] || "A";
    const b = parts.length > 1 ? parts[1]?.[0] : "";
    return (a + b).toUpperCase();
}

function formatDuration(totalSec) {
    if (!Number.isFinite(totalSec) || totalSec < 0) return "--:--";
    const sec = Math.floor(totalSec);
    const min = Math.floor(sec / 60);
    const rem = sec % 60;
    return `${min}:${String(rem).padStart(2, "0")}`;
}

function formatMs(ms) {
    if (!Number.isFinite(ms) || ms <= 0) return "0s";
    const sec = Math.ceil(ms / 1000);
    return `${sec}s`;
}

function formatRelative(isoOrDate) {
    const d = new Date(isoOrDate);
    if (Number.isNaN(d.getTime())) return "unknown";
    const diff = Date.now() - d.getTime();
    const sec = Math.round(diff / 1000);
    if (sec < 10) return "just now";
    if (sec < 60) return `${sec}s ago`;
    const min = Math.round(sec / 60);
    if (min < 60) return `${min}m ago`;
    const hr = Math.round(min / 60);
    if (hr < 48) return `${hr}h ago`;
    const day = Math.round(hr / 24);
    return `${day}d ago`;
}

async function copyRoomId() {
    if (!props.roomId) return;
    try {
        await navigator.clipboard.writeText(props.roomId);
    } catch {
        // ignore
    }
}

onMounted(async () => {
    roomClosedReason.value = "";
    wasKicked.value = false;
    setPlayerId(loadStoredPlayerId());
    nowTimer = setInterval(() => {
        nowTick.value = Date.now();
    }, 1000);
    await setupYouTubePlayer();
    await reloadAll();
    await maybeAutoJoin();
    connectWS();
    syncTimer = setInterval(() => {
        syncPlayerToSnapshot();
    }, 1000);
});

watch(
    [snapshot, currentPlayerConnected, roomClosedReason, hasStoredNickname],
    () => {
        maybeAutoJoin();
    },
    { immediate: true },
);

watch(
    () => [
        snapshot.value?.playback?.track?.youTubeID,
        snapshot.value?.playback?.track?.youtubeId,
        snapshot.value?.playback?.paused,
        snapshot.value?.playback?.positionMs,
        snapshot.value?.playback?.updatedAt,
        snapshot.value?.playback?.startAt,
    ],
    () => {
        syncPlayerToSnapshot();
    },
    { immediate: true },
);

watch(
    () => volume.value,
    (next) => {
        if (!ytReady.value || !ytPlayer.value) return;
        if (typeof ytPlayer.value.setVolume === "function") {
            ytPlayer.value.setVolume(next);
            ytPlayer.value.unMute();
        }
    },
    { immediate: true },
);

watch(
    () => snapshot.value?.playlist?.playlistId,
    (next) => {
        if (!next) {
            if (!selectedPlaylistId.value) return;
            suppressPlaylistLoad.value = true;
            selectedPlaylistId.value = "";
        } else if (next === selectedPlaylistId.value) {
            return;
        } else {
            suppressPlaylistLoad.value = true;
            selectedPlaylistId.value = next;
        }
        setTimeout(() => {
            suppressPlaylistLoad.value = false;
        }, 0);
    },
);

watch(
    () => selectedPlaylistId.value,
    async (next) => {
        if (!next || suppressPlaylistLoad.value) return;
        if (next === snapshot.value?.playlist?.playlistId) return;
        await loadSelectedPlaylist();
    },
);

// reconnect WS when roomId changes
watch(
    () => props.roomId,
    async () => {
        roomClosedReason.value = "";
        wasKicked.value = false;
        joinError.value = "";
        joinPassword.value = "";
        snapshot.value = null;
        setPlayerId(loadStoredPlayerId());
        destroyYouTubePlayer();
        await setupYouTubePlayer();
        await reloadAll();
        connectWS();
    },
);

onBeforeUnmount(() => {
    leave({ silent: true });
    disconnectWS();
    destroyYouTubePlayer();
    if (syncTimer) clearInterval(syncTimer);
    if (nowTimer) clearInterval(nowTimer);
    clearScheduledStart();
    if (preloadReadyTimer) clearTimeout(preloadReadyTimer);
    if (bufferingDelayTimer) clearTimeout(bufferingDelayTimer);
});
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
    margin: 10px 0 0;
    line-height: 1.1;
}

.brand .subtitle {
    margin: 6px 0 0;
}

.right {
    display: flex;
    gap: 10px;
    align-items: center;
    flex-wrap: wrap;
}

.card {
    background: var(--color-background-soft, rgba(255, 255, 255, 0.06));
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    border-radius: 12px;
    padding: 16px;
    margin-top: 16px;
}

.card-inner {
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    border-radius: 12px;
    padding: 12px;
}

.h2 {
    margin: 0 0 8px;
}

.h3 {
    margin: 0 0 10px;
}

.muted {
    opacity: 0.8;
}

.strong {
    font-weight: 650;
}

.small {
    font-size: 0.9rem;
}

.row {
    display: flex;
    gap: 12px;
    align-items: flex-end;
    flex-wrap: wrap;
    margin-top: 12px;
}

.row-gap-sm {
    gap: 8px;
}

.row-center {
    align-items: center;
}

.row-space {
    justify-content: space-between;
    align-items: flex-start;
}

.col {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 220px;
    flex: 1 1 220px;
}

.actions {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
}

.label {
    font-size: 0.9rem;
    opacity: 0.9;
}

.input {
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: var(--color-background, rgba(0, 0, 0, 0.2));
    color: inherit;
    outline: none;
}

.btn {
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: rgba(100, 140, 255, 0.22);
    color: inherit;
    cursor: pointer;
    text-decoration: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
}

.btn:hover {
    background: rgba(100, 140, 255, 0.32);
}

.btn:disabled,
.btn.disabled {
    opacity: 0.5;
    cursor: not-allowed;
    pointer-events: none;
}

.btn-ghost {
    background: transparent;
}

.btn-danger {
    background: rgba(255, 80, 80, 0.22);
    border-color: rgba(255, 80, 80, 0.35);
}

.btn-danger:hover {
    background: rgba(255, 80, 80, 0.32);
}

.btn-buzz {
    font-weight: 800;
    letter-spacing: 0.03em;
    padding: 14px 18px;
    background: rgba(255, 180, 70, 0.22);
    border-color: rgba(255, 180, 70, 0.35);
}

.btn-buzz:hover {
    background: rgba(255, 180, 70, 0.32);
}

.badge {
    display: inline-flex;
    gap: 10px;
    align-items: center;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    border-radius: 999px;
    padding: 8px 10px;
}

.badge-ok {
    background: rgba(80, 200, 120, 0.12);
}

.badge-warn {
    background: rgba(255, 180, 70, 0.12);
}

.badge-dim {
    opacity: 0.85;
}

.badge-label {
    opacity: 0.8;
    font-size: 0.9rem;
}

.badge-value {
    font-weight: 650;
}

.error {
    margin-top: 10px;
    color: #ffb3b3;
}

.roster {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
}

.player {
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 12px;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    border-radius: 12px;
    background: rgba(255, 255, 255, 0.02);
}

.avatar {
    width: 44px;
    height: 44px;
    border-radius: 12px;
    overflow: hidden;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: rgba(255, 255, 255, 0.04);
    display: grid;
    place-items: center;
    flex: 0 0 auto;
}

.avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.avatar-fallback {
    font-weight: 800;
    opacity: 0.9;
}

.player-main {
    flex: 1 1 auto;
    min-width: 220px;
}

.player-name {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
}

.name {
    font-weight: 700;
}

.player-actions {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
}

.pill {
    font-size: 0.85rem;
    padding: 4px 8px;
    border-radius: 999px;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: rgba(255, 255, 255, 0.04);
}

.pill-ok {
    background: rgba(80, 200, 120, 0.12);
    border-color: rgba(80, 200, 120, 0.35);
}

.pill-dim {
    opacity: 0.75;
}

.pill-me {
    background: rgba(100, 140, 255, 0.12);
    border-color: rgba(100, 140, 255, 0.35);
}

.pill-owner {
    background: rgba(220, 120, 255, 0.12);
    border-color: rgba(220, 120, 255, 0.35);
}

.dot {
    margin: 0 6px;
    opacity: 0.5;
}

.divider {
    margin: 14px 0;
    height: 1px;
    background: var(--color-border, rgba(255, 255, 255, 0.12));
}

.playlist-track-list {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-height: 260px;
    overflow-y: auto;
    padding-right: 4px;
}

.playlist-track {
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
}

.playlist-track.is-selectable {
    cursor: pointer;
}

.playlist-track.is-current {
    border-color: rgba(100, 140, 255, 0.45);
    background: rgba(100, 140, 255, 0.12);
}

.playlist-track-thumb {
    width: 56px;
    height: 56px;
    border-radius: 10px;
    overflow: hidden;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.06);
    display: grid;
    place-items: center;
    flex: 0 0 auto;
}

.playlist-track-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.playlist-track-thumb-fallback {
    font-size: 1.2rem;
    opacity: 0.75;
}

.playlist-track-main {
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.playlist-track-title {
    font-weight: 650;
}

.playback-controls {
    display: flex;
    flex-direction: column;
    gap: 12px;
}

.playback-waiting {
    margin-top: 8px;
}

.playback-buttons {
    display: flex;
    gap: 10px;
    align-items: center;
    flex-wrap: wrap;
}

.icon-btn {
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: rgba(100, 140, 255, 0.12);
    color: inherit;
    border-radius: 12px;
    padding: 8px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: background 0.2s ease;
}

.icon-btn svg {
    width: 20px;
    height: 20px;
    fill: currentColor;
}

.icon-btn:hover {
    background: rgba(100, 140, 255, 0.22);
}

.icon-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    pointer-events: none;
}

.icon-main {
    padding: 10px;
    border-radius: 14px;
    background: rgba(255, 180, 70, 0.2);
    border-color: rgba(255, 180, 70, 0.35);
}

.playback-progress {
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.progress-bar {
    height: 10px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.08);
    overflow: hidden;
    cursor: pointer;
    border: 1px solid rgba(255, 255, 255, 0.12);
}

.progress-bar.progress-unknown {
    opacity: 0.6;
}

.progress-fill {
    height: 100%;
    background: rgba(100, 140, 255, 0.7);
    width: 0%;
    transition: width 0.2s ease;
}

.playback-timer {
    font-size: 0.9rem;
    opacity: 0.8;
    display: flex;
    justify-content: flex-end;
}

.grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 12px;
}

@media (min-width: 980px) {
    .grid {
        grid-template-columns: 1fr 1fr;
        align-items: start;
    }
}

.result {
    margin-top: 12px;
    padding: 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 180, 70, 0.35);
    background: rgba(255, 180, 70, 0.12);
}

.result-line {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: baseline;
}

.buzz-modal-player {
    display: flex;
    gap: 12px;
    align-items: center;
    margin: 12px 0;
}

.yt-player {
    position: fixed;
    width: 0;
    height: 0;
    overflow: hidden;
    pointer-events: none;
    opacity: 0;
    z-index: -1;
}

.yt-player iframe {
    width: 0;
    height: 0;
    pointer-events: none;
}

.hint {
    margin-top: 10px;
}

.code {
    padding: 2px 6px;
    border-radius: 8px;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: rgba(255, 255, 255, 0.04);
}

.modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
    z-index: 20;
}

.modal-card {
    background: var(--color-background, rgba(20, 20, 24, 0.95));
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.16));
    border-radius: 14px;
    padding: 20px;
    width: min(760px, 100%);
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.35);
}

.info-blur {
    background: rgba(255, 255, 255, 0.03);
    padding: 12px;
    border-radius: 12px;
}
</style>
