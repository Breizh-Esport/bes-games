<template>
    <main class="page">
        <header class="header">
            <div class="brand">
                <h1 class="title">bes-games</h1>
                <p class="subtitle muted">Choose a game to play.</p>
            </div>
        </header>

        <section class="grid">
            <article v-for="g in games" :key="g.id" class="card game-card">
                <div class="game-meta">
                    <h2 class="h2">{{ g.name }}</h2>
                    <p class="muted">{{ g.description }}</p>
                </div>
                <div class="game-actions">
                    <RouterLink class="btn" :to="gameLink(g.id)"
                        >Open</RouterLink
                    >
                </div>
            </article>

            <div v-if="loadingGames" class="muted">Loading games...</div>
            <p v-if="gamesError" class="error">{{ gamesError }}</p>
        </section>

        <section class="card">
            <h2 class="h2">Authentication (temporary)</h2>
            <p class="muted">
                Authentication is currently a placeholder. Set an arbitrary
                <code class="code">sub</code> to simulate being logged in (sent
                as <code class="code">X-User-Sub</code> to the backend).
            </p>

            <div class="row">
                <div class="col">
                    <label class="label" for="sub">User subject (sub)</label>
                    <input
                        id="sub"
                        v-model="loginSub"
                        class="input"
                        type="text"
                        placeholder="e.g. oidc|john.doe"
                        autocomplete="off"
                    />
                </div>

                <div class="actions">
                    <button
                        class="btn"
                        @click="onLogin"
                        :disabled="!loginSubTrimmed"
                    >
                        Set sub
                    </button>
                    <button
                        class="btn btn-ghost"
                        @click="onLogout"
                        :disabled="!auth.isAuthenticated.value"
                    >
                        Clear
                    </button>
                </div>
            </div>

            <div class="row">
                <div
                    class="badge"
                    :class="
                        auth.isAuthenticated.value ? 'badge-ok' : 'badge-warn'
                    "
                >
                    <span class="badge-label">Auth</span>
                    <span class="badge-value">
                        {{
                            auth.isAuthenticated.value
                                ? `Authenticated (${auth.state.sub})`
                                : "Anonymous"
                        }}
                    </span>
                </div>
            </div>
        </section>

        <footer class="footer muted">
            <div>
                Backend API base:
                <code class="code">{{ apiBase }}</code>
            </div>
            <div class="muted">
                Tip: set <code class="code">VITE_API_BASE_URL</code> for the
                frontend to point to your Go server.
            </div>
        </footer>
    </main>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { api, getApiBaseUrl } from "../lib/api";
import { useAuth } from "../stores/auth";

const auth = useAuth();

const apiBase = getApiBaseUrl();

// Login placeholder
const loginSub = ref(auth.state.sub || "");
const loginSubTrimmed = computed(() => (loginSub.value || "").trim());

function onLogin() {
    if (!loginSubTrimmed.value) return;
    auth.loginWithSub(loginSubTrimmed.value);
}

function onLogout() {
    auth.logout();
    loginSub.value = "";
}

function gameLink(id) {
    return `/games/${encodeURIComponent(id)}`;
}

const fallbackGames = [
    {
        id: "name-that-tune",
        name: "Name That Tune",
        description:
            "Guess songs as fast as you can. Rooms, playlists, buzzer, and synchronized playback state.",
    },
];

const games = ref(fallbackGames);
const loadingGames = ref(false);
const gamesError = ref("");

onMounted(() => {
    (async () => {
        loadingGames.value = true;
        gamesError.value = "";
        try {
            const res = await api.listGames();
            const gs = res?.games;
            games.value = Array.isArray(gs) && gs.length ? gs : fallbackGames;
        } catch (e) {
            games.value = fallbackGames;
            gamesError.value =
                e?.message || "Failed to load games (using fallback list)";
        } finally {
            loadingGames.value = false;
        }
    })();
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
    margin: 0;
    line-height: 1.1;
}
.brand .subtitle {
    margin: 6px 0 0;
}

.grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: 14px;
}

.card {
    background: var(--color-background-soft, rgba(255, 255, 255, 0.06));
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    border-radius: 12px;
    padding: 16px;
    margin-top: 16px;
}

.game-card {
    margin-top: 0;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    min-height: 150px;
}

.game-meta .h2 {
    margin: 0 0 8px;
}

.game-actions {
    margin-top: 12px;
    display: flex;
    justify-content: flex-end;
}

.h2 {
    margin: 0 0 8px;
}

.muted {
    opacity: 0.8;
}

.row {
    display: flex;
    gap: 12px;
    align-items: flex-end;
    flex-wrap: wrap;
    margin-top: 12px;
}

.col {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 220px;
    flex: 1 1 220px;
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

.actions {
    display: flex;
    gap: 8px;
    align-items: center;
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
.badge-label {
    opacity: 0.8;
    font-size: 0.9rem;
}
.badge-value {
    font-weight: 600;
}

.error {
    margin-top: 10px;
    color: #ffb3b3;
}

.footer {
    margin-top: 18px;
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.code {
    padding: 2px 6px;
    border-radius: 8px;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: rgba(255, 255, 255, 0.04);
}
</style>
