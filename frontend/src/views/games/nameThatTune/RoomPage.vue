<template>
    <main class="page">
        <header class="header">
            <div class="brand">
                <div class="row row-center row-gap-sm">
                    <RouterLink class="btn btn-ghost" to="/"
                        >ÔåÉ Home</RouterLink
                    >
                    <RouterLink class="btn btn-ghost" to="/profile"
                        >Profile</RouterLink
                    >
                </div>

                <h1 class="title">
                    Room
                    <span class="muted" v-if="roomId">({{ roomId }})</span>
                </h1>

                <p class="subtitle muted" v-if="snapshot">
                    {{ snapshot.name }} ÔÇó
                    {{ isOwner ? "Owner view" : "Player view" }}
                </p>
                <p class="subtitle muted" v-else>LoadingÔÇª</p>
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
                        {{ loadingAny ? "RefreshingÔÇª" : "Refresh" }}
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

            <div v-if="!snapshot" class="muted">Loading roomÔÇª</div>

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
                            <span class="dot">ÔÇó</span>
                            <span class="muted">id: {{ p.playerId }}</span>
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
                            SetÔÇª
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


        <section class="card">
            <div class="row row-space">
                <div>
                    <h2 class="h2">Playback</h2>
                    <p class="muted">
                        Owner loads a playlist and controls playback. Players cannot see track details.
                    </p>
                </div>
            </div>

            <div v-if="!snapshot" class="muted">Loading...</div>

            <template v-else>
                <div class="row">
                    <div class="col">
                        <div class="muted small">Audio</div>
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

                    <div class="col" v-if="isOwner">
                        <div class="muted small">Role</div>
                        <div class="strong">Owner</div>
                    </div>
                </div>

                <div class="divider"></div>

                <div class="row" v-if="isOwner">
                    <div class="col">
                        <div class="muted small">Loaded playlist</div>
                        <div class="strong">
                            {{ snapshot.playlist?.name || "None" }}
                            <span v-if="snapshot.playlist" class="muted small"
                                >({{
                                    snapshot.playlist.items?.length || 0
                                }}
                                tracks)</span
                            >
                        </div>
                    </div>

                    <div class="col">
                        <div class="muted small">Now playing</div>
                        <div class="strong">
                            <span v-if="snapshot.playback?.track">{{
                                snapshot.playback.track.title
                            }}</span>
                            <span v-else class="muted">No track</span>
                        </div>
                    </div>

                    <div class="col">
                        <div class="muted small">State</div>
                        <div class="strong">
                            {{
                                snapshot.playback?.paused ? "Paused" : "Playing"
                            }}
                            <span class="muted small"
                                - pos
                                {{
                                    Math.floor(
                                        (snapshot.playback?.positionMs || 0) /
                                            1000,
                                    )
                                }}s</span
                            >
                        </div>
                    </div>
                </div>
                <div v-else class="muted">
                    Playback details are hidden for players until the owner is present.
                </div>

                <div class="divider"></div>

                <div class="grid">
                    <div class="card-inner" v-if="isOwner">
                        <h3 class="h3">Controls</h3>

                        <p v-if="ownerError" class="error">{{ ownerError }}</p>

                        <div class="row">
                            <div class="col">
                                <label class="label" for="playlistSelect"
                                    >Load playlist</label
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
                                        {{ pl.name }} ({{
                                            pl.items?.length || 0
                                        }})
                                    </option>
                                </select>
                            </div>
                            <div class="actions">
                                <button
                                    class="btn"
                                    @click="loadSelectedPlaylist"
                                    :disabled="busyOwner || !selectedPlaylistId"
                                >
                                    Load
                                </button>
                                <button
                                    class="btn btn-ghost"
                                    @click="refreshMyPlaylists"
                                    :disabled="busyOwner"
                                >
                                    Refresh playlists
                                </button>
                            </div>
                        </div>

                        <div class="divider"></div>

                        <div class="row">
                            <div class="actions">
                                <button
                                    class="btn"
                                    @click="togglePause"
                                    :disabled="busyOwner || !canControlPlayback"
                                >
                                    {{
                                        snapshot.playback?.paused
                                            ? "Play"
                                            : "Pause"
                                    }}
                                </button>
                                <button
                                    class="btn btn-ghost"
                                    @click="prevTrack"
                                    :disabled="busyOwner || !canControlPlayback"
                                >
                                    Prev
                                </button>
                                <button
                                    class="btn btn-ghost"
                                    @click="nextTrack"
                                    :disabled="busyOwner || !canControlPlayback"
                                >
                                    Next
                                </button>
                                <button
                                    class="btn btn-ghost"
                                    @click="restartTrack"
                                    :disabled="busyOwner || !canControlPlayback"
                                >
                                    Restart
                                </button>
                            </div>
                        </div>

                        <div class="row">
                            <div class="col">
                                <label class="label" for="seek"
                                    >Seek (seconds)</label
                                >
                                <input
                                    id="seek"
                                    class="input"
                                    type="number"
                                    min="0"
                                    step="1"
                                    v-model.number="seekSeconds"
                                    :disabled="busyOwner || !canControlPlayback"
                                />
                            </div>
                            <div class="actions">
                                <button
                                    class="btn btn-ghost"
                                    @click="seekTo"
                                    :disabled="busyOwner || !canControlPlayback"
                                >
                                    Seek
                                </button>
                            </div>
                        </div>

                        <div class="hint muted small">
                            Note: actual audio playback is not implemented yet
                            (YouTube embed/player). These controls broadcast
                            state to all clients via WebSocket.
                        </div>
                    </div>

                    <div class="card-inner" v-if="!isOwner">
                        <h3 class="h3">Player controls</h3>
                        <p v-if="playerError" class="error">
                            {{ playerError }}
                        </p>

                        <div class="row">
                            <div class="col">
                                <div class="muted small">Buzzer</div>
                                <div class="strong">Tap to buzz in.</div>
                            </div>
                            <div class="actions">
                                <button
                                    class="btn btn-buzz"
                                    @click="buzz"
                                    :disabled="!currentPlayerConnected || buzzing || roomClosedReason"
                                >
                                    {{ buzzing ? "Buzzing..." : "BUZZ" }}
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
                                <span class="muted small"
                                    >? {{ formatRelative(lastBuzz.ts) }}</span
                                >
                            </div>
                        </div>

                        <div class="hint muted small">
                            Stay connected; if you disconnect you will be prompted to rejoin.
                        </div>
                    </div>
                </div>
            </template>
        </section>

        <div v-if="showJoinModal" class="modal-backdrop">
            <div class="modal-card">
                <h3 class="h3">
                    {{ roomClosedReason ? "Room closed" : "Reconnect to play" }}
                </h3>
                <p class="muted" v-if="roomClosedReason">
                    {{ closeReasonMessage }}
                </p>
                <template v-else>
                    <p class="muted">
                        Enter a nickname and picture to join this room.
                    </p>
                    <div class="row">
                        <div class="col">
                            <label class="label" for="modalNick">Nickname</label>
                            <input
                                id="modalNick"
                                v-model="joinNick"
                                class="input"
                                type="text"
                                placeholder="Anonymous"
                            />
                        </div>
                        <div class="col">
                            <label class="label" for="modalPic">Picture URL</label>
                            <input
                                id="modalPic"
                                v-model="joinPic"
                                class="input"
                                type="url"
                                placeholder="https://..."
                            />
                        </div>
                        <div class="actions">
                            <button class="btn" @click="joinFromModal" :disabled="joining">
                                {{ joining ? "Joining..." : "Join room" }}
                            </button>
                            <RouterLink class="btn btn-ghost" to="/">Leave</RouterLink>
                        </div>
                    </div>
                    <p v-if="joinError" class="error">{{ joinError }}</p>
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

// Room state
const snapshot = ref(null);

// Local join state (client-only)
const playerId = ref("");
const joining = ref(false);
const leaving = ref(false);
const joinNick = ref("");
const joinPic = ref("");
const joinError = ref("");
const roomClosedReason = ref("");

// Owner controls / playlists
const myPlaylists = ref([]);
const selectedPlaylistId = ref("");
const busyOwner = ref(false);
const ownerError = ref("");

// Player controls
const volume = ref(65);
const buzzing = ref(false);
const playerError = ref("");

// Buzzer events
const lastBuzz = ref(null);

// WS
const wsStatus = ref("disconnected");
let ws = null;

// simple seek UI
const seekSeconds = ref(0);

const PLAYER_STORAGE_PREFIX = "ntt.player.";
function storageKey() {
    return `${PLAYER_STORAGE_PREFIX}${props.roomId}`;
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

setPlayerId(loadStoredPlayerId());

const canControlPlayback = computed(
    () =>
        !!snapshot.value?.playlist?.playlistId &&
        (snapshot.value?.playlist?.items?.length || 0) > 0 &&
        !roomClosedReason.value,
);
const isOwner = computed(() => {
    const sub = auth.state.sub;
    const ownerSub = snapshot.value?.ownerSub;
    if (!sub || !ownerSub) return false;
    return sub === ownerSub;
});

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
    return null;
});

const currentPlayerConnected = computed(
    () => !!currentPlayer.value && currentPlayer.value.connected,
);

const showJoinModal = computed(() => {
    if (roomClosedReason.value) return true;
    if (!snapshot.value) return false;
    return !currentPlayerConnected.value;
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
        if (auth.isAuthenticated.value) {
            await refreshMyPlaylists();
        }
    } catch (e) {
        if (e?.status === 404) {
            roomClosedReason.value = roomClosedReason.value || "owner_left_empty";
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
        await api.loadPlaylist(props.gameId, props.roomId, {
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
        await api.pause(props.gameId, props.roomId, { paused });
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
        await api.setPlayback(props.gameId, props.roomId, {
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

async function seekTo() {
    const sec = Number(seekSeconds.value || 0);
    if (!Number.isFinite(sec) || sec < 0) return;
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await api.seek(props.gameId, props.roomId, {
            positionMs: Math.floor(sec * 1000),
        });
        // Do not apply REST response; wait for WS `room.snapshot`.
    } catch (e) {
        ownerError.value = e?.message || "Failed to seek";
    } finally {
        busyOwner.value = false;
    }
}

// Owner: roster controls
async function kick(pid) {
    busyOwner.value = true;
    ownerError.value = "";
    try {
        await api.kick(props.gameId, props.roomId, { playerId: pid });
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
        await api.addScore(props.gameId, props.roomId, {
            playerId: pid,
            delta,
        });
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
        await api.setScore(props.gameId, props.roomId, {
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
async function joinFromModal() {
    if (!props.roomId || roomClosedReason.value) return;
    joining.value = true;
    joinError.value = "";
    playerError.value = "";
    try {
        const res = await api.joinRoom(props.gameId, props.roomId, {
            nickname: joinNick.value || undefined,
            pictureUrl: joinPic.value || undefined,
        });
        setPlayerId(res?.PlayerID || res?.playerId || "");
        roomClosedReason.value = "";
        if (res?.snapshot) {
            snapshot.value = res.snapshot;
        }
        syncPlayerFromSnapshot();
    } catch (e) {
        joinError.value = e?.message || "Failed to join room";
    } finally {
        joining.value = false;
    }
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
        await api.buzz(props.gameId, props.roomId, {
            playerId: playerId.value,
        });
    } catch (e) {
        playerError.value = e?.message || "Failed to buzz";
    } finally {
        buzzing.value = false;
    }
}

// WS connection
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
    setPlayerId(loadStoredPlayerId());
    await reloadAll();
    connectWS();
});

// reconnect WS when roomId changes
watch(
    () => props.roomId,
    async () => {
        roomClosedReason.value = "";
        joinError.value = "";
        snapshot.value = null;
        setPlayerId(loadStoredPlayerId());
        await reloadAll();
        connectWS();
    },
);

onBeforeUnmount(() => {
    leave({ silent: true });
    disconnectWS();
});
</script></script>

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
