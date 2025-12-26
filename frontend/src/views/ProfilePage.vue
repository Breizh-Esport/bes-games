<template>
    <main class="page">
        <header class="header">
            <div class="brand">
                <h1 class="title">Profile</h1>
                <p class="subtitle muted">
                    Account settings and game-specific settings
                </p>
            </div>
        </header>

        <section class="card" v-if="!auth.isAuthenticated.value">
            <h2 class="h2">Sign in required</h2>
            <p class="muted">
                Profile and game settings require authentication.
            </p>
            <div class="actions">
                <button class="btn" @click="auth.login()">Sign in</button>
                <RouterLink class="btn btn-ghost" to="/"
                    >Go to games</RouterLink
                >
            </div>
        </section>

        <template v-else>
            <section class="card">
                <div class="row row-space">
                    <div>
                        <h2 class="h2">Account</h2>
                        <p class="muted">
                            Nickname and avatar are stored server-side.
                        </p>
                    </div>

                    <div class="actions">
                        <button
                            class="btn btn-ghost"
                            @click="refreshProfile"
                            :disabled="loading"
                        >
                            {{ loading ? "Refreshing..." : "Refresh" }}
                        </button>
                    </div>
                </div>

                <p v-if="profileError" class="error">{{ profileError }}</p>

                <div class="row">
                    <div class="avatar">
                        <img
                            v-if="picturePreview"
                            :src="picturePreview"
                            alt="avatar"
                        />
                        <div v-else class="avatar-fallback">{{ initials }}</div>
                    </div>

                    <div class="col">
                        <label class="label" for="nickname">Nickname</label>
                        <input
                            id="nickname"
                            v-model="nickname"
                            class="input"
                            type="text"
                            autocomplete="off"
                        />
                    </div>

                    <div class="col">
                        <label class="label" for="pictureUrl"
                            >Profile picture URL</label
                        >
                        <input
                            id="pictureUrl"
                            v-model="pictureUrl"
                            class="input"
                            type="url"
                            placeholder="https://example.com/avatar.png"
                            autocomplete="off"
                        />
                    </div>

                    <div class="actions">
                        <button
                            class="btn"
                            @click="saveProfile"
                            :disabled="saving"
                        >
                            {{ saving ? "Saving..." : "Save profile" }}
                        </button>
                    </div>
                </div>

                <div class="row">
                    <div class="badge badge-ok">
                        <span class="badge-label">sub</span>
                        <span class="badge-value">{{ auth.state.sub }}</span>
                    </div>
                    <button class="btn btn-ghost" @click="auth.logout()">
                        Logout
                    </button>
                </div>
            </section>

            <section class="card">
                <div class="row row-space">
                    <div>
                        <h2 class="h2">Game settings</h2>
                        <p class="muted">
                            Each game can have its own settings and data.
                        </p>
                    </div>
                </div>

                <div class="games-grid">
                    <article class="game-tile">
                        <div>
                            <div class="game-title">Name That Tune</div>
                            <div class="muted small">
                                Playlists and future game-specific settings.
                            </div>
                        </div>
                        <div class="actions">
                            <RouterLink
                                class="btn"
                                to="/games/name-that-tune/settings/playlists"
                                >Open</RouterLink
                            >
                            <RouterLink
                                class="btn btn-ghost"
                                to="/games/name-that-tune"
                                >Lobby</RouterLink
                            >
                        </div>
                    </article>
                </div>
            </section>

            <section class="card danger">
                <h2 class="h2">Delete account</h2>
                <p class="muted">
                    This will delete your user profile on the backend, and also
                    remove your game data (playlists, rooms, etc.).
                </p>

                <p v-if="deleteError" class="error">{{ deleteError }}</p>

                <div class="row">
                    <div class="col">
                        <label class="label" for="confirm"
                            >Type <code class="code">DELETE</code> to
                            confirm</label
                        >
                        <input
                            id="confirm"
                            v-model="deleteConfirm"
                            class="input"
                            type="text"
                            autocomplete="off"
                        />
                    </div>
                    <div class="actions">
                        <button
                            class="btn btn-danger"
                            @click="deleteAccount"
                            :disabled="deleting || deleteConfirm !== 'DELETE'"
                        >
                            {{ deleting ? "Deleting..." : "Delete my account" }}
                        </button>
                    </div>
                </div>
            </section>
        </template>
    </main>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { api } from "../lib/api";
import { useAuth } from "../stores/auth";

const router = useRouter();
const auth = useAuth();

const loading = ref(false);

const nickname = ref("");
const pictureUrl = ref("");
const profileError = ref("");
const saving = ref(false);

const deleteConfirm = ref("");
const deleting = ref(false);
const deleteError = ref("");

const picturePreview = computed(() => (pictureUrl.value || "").trim() || "");

const initials = computed(() => {
    const n = (nickname.value || "").trim();
    if (!n) return "P";
    const parts = n.split(/\s+/).filter(Boolean);
    const a = parts[0]?.[0] || "P";
    const b = parts.length > 1 ? parts[1]?.[0] : "";
    return (a + b).toUpperCase();
});

async function loadProfile() {
    profileError.value = "";
    const me = await api.getMe();
    nickname.value = me?.Nickname || me?.nickname || "Player";
    pictureUrl.value = me?.PictureURL || me?.pictureURL || me?.pictureUrl || "";
}

async function refreshProfile() {
    loading.value = true;
    profileError.value = "";
    try {
        await loadProfile();
    } catch (e) {
        profileError.value = e?.message || "Failed to load profile";
    } finally {
        loading.value = false;
    }
}

async function saveProfile() {
    saving.value = true;
    profileError.value = "";
    try {
        await api.putMe({
            nickname: nickname.value,
            pictureUrl: pictureUrl.value,
        });
    } catch (e) {
        profileError.value = e?.message || "Failed to save profile";
    } finally {
        saving.value = false;
    }
}

async function deleteAccount() {
    if (deleteConfirm.value !== "DELETE") return;
    deleting.value = true;
    deleteError.value = "";
    try {
        await api.deleteMe();
        auth.logout();
        await router.push("/");
    } catch (e) {
        deleteError.value = e?.message || "Failed to delete account";
    } finally {
        deleting.value = false;
        deleteConfirm.value = "";
    }
}

onMounted(async () => {
    if (!auth.isAuthenticated.value) return;
    await refreshProfile();
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
    opacity: 0.8;
}

.card {
    background: var(--color-background-soft, rgba(255, 255, 255, 0.06));
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    border-radius: 12px;
    padding: 16px;
    margin-top: 16px;
}

.h2 {
    margin: 0 0 8px;
}

.muted {
    opacity: 0.8;
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

.btn-danger {
    background: rgba(255, 60, 60, 0.2);
    border-color: rgba(255, 60, 60, 0.25);
}

.avatar {
    width: 64px;
    height: 64px;
    border-radius: 14px;
    overflow: hidden;
    border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
    background: rgba(255, 255, 255, 0.06);
    display: flex;
    align-items: center;
    justify-content: center;
}

.avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.avatar-fallback {
    font-weight: 800;
    font-size: 1.1rem;
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
.badge-label {
    opacity: 0.8;
    font-size: 0.9rem;
}
.badge-value {
    font-weight: 600;
}

.code {
    padding: 2px 6px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.08);
    border: 1px solid rgba(255, 255, 255, 0.12);
}

.error {
    margin-top: 10px;
    color: #ffb3b3;
}

.danger {
    border-color: rgba(255, 60, 60, 0.28);
}

.games-grid {
    margin-top: 10px;
    display: grid;
    grid-template-columns: 1fr;
    gap: 12px;
}

.game-tile {
    padding: 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    display: flex;
    justify-content: space-between;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
}

.game-title {
    font-weight: 750;
}
</style>
