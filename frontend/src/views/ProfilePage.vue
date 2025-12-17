<template>
  <main class="page">
    <header class="header">
      <div class="brand">
        <h1 class="title">Profile</h1>
        <p class="subtitle muted">Update your nickname and avatar, manage playlists, delete account</p>
      </div>

      <nav class="nav">
        <RouterLink class="link" to="/">Home</RouterLink>
        <RouterLink class="link router-link-active" to="/profile">Profile</RouterLink>
      </nav>
    </header>

    <section class="card" v-if="!auth.isAuthenticated.value">
      <h2 class="h2">Not authenticated</h2>
      <p class="muted">
        Profile features require authentication (OIDC). For now, go back to Home and set a simulated
        subject (<code class="code">sub</code>).
      </p>
      <RouterLink class="btn" to="/">Go to Home</RouterLink>
    </section>

    <template v-else>
      <section class="card">
        <div class="row row-space">
          <div>
            <h2 class="h2">Your profile</h2>
            <p class="muted">This is stored server-side (in-memory for now).</p>
          </div>

          <div class="actions">
            <button class="btn btn-ghost" @click="refreshAll" :disabled="loadingAny">
              {{ loadingAny ? 'Refreshing…' : 'Refresh' }}
            </button>
          </div>
        </div>

        <p v-if="profileError" class="error">{{ profileError }}</p>

        <div class="row">
          <div class="avatar">
            <img v-if="picturePreview" :src="picturePreview" alt="avatar" />
            <div v-else class="avatar-fallback">{{ initials }}</div>
          </div>

          <div class="col">
            <label class="label" for="nickname">Nickname</label>
            <input id="nickname" v-model="nickname" class="input" type="text" autocomplete="off" />
          </div>

          <div class="col">
            <label class="label" for="pictureUrl">Profile picture URL</label>
            <input
              id="pictureUrl"
              v-model="pictureUrl"
              class="input"
              type="url"
              placeholder="https://…"
              autocomplete="off"
            />
          </div>

          <div class="actions">
            <button class="btn" @click="saveProfile" :disabled="savingProfile">
              {{ savingProfile ? 'Saving…' : 'Save profile' }}
            </button>
          </div>
        </div>

        <div class="row">
          <div class="badge badge-ok">
            <span class="badge-label">sub</span>
            <span class="badge-value">{{ auth.state.sub }}</span>
          </div>
          <button class="btn btn-ghost" @click="auth.logout()">Logout</button>
        </div>
      </section>

      <section class="card">
        <div class="row row-space">
          <div>
            <h2 class="h2">Playlists</h2>
            <p class="muted">Create playlists and add YouTube tracks (title + URL).</p>
          </div>
        </div>

        <p v-if="playlistError" class="error">{{ playlistError }}</p>

        <div class="row">
          <div class="col">
            <label class="label" for="newPlaylistName">New playlist name</label>
            <input
              id="newPlaylistName"
              v-model="newPlaylistName"
              class="input"
              type="text"
              placeholder="My blindtest set"
              autocomplete="off"
            />
          </div>
          <div class="actions">
            <button class="btn" @click="createPlaylist" :disabled="creatingPlaylist || !newPlaylistNameTrimmed">
              {{ creatingPlaylist ? 'Creating…' : 'Create playlist' }}
            </button>
          </div>
        </div>

        <div v-if="playlists.length" class="playlist-list">
          <details v-for="pl in playlists" :key="pl.id" class="playlist">
            <summary class="playlist-summary">
              <div class="playlist-title">
                <div class="playlist-name">{{ pl.name }}</div>
                <div class="muted small">
                  {{ pl.items?.length || 0 }} tracks • updated {{ formatRelative(pl.updatedAt) }}
                </div>
              </div>

              <div class="playlist-actions">
                <button class="btn btn-ghost" @click.prevent="beginRename(pl)">Rename</button>
              </div>
            </summary>

            <div class="playlist-body">
              <div v-if="renamingId === pl.id" class="row">
                <div class="col">
                  <label class="label">New name</label>
                  <input v-model="renameName" class="input" type="text" autocomplete="off" />
                </div>
                <div class="actions">
                  <button class="btn" @click="applyRename(pl)" :disabled="renamingBusy || !renameTrimmed">
                    {{ renamingBusy ? 'Saving…' : 'Save' }}
                  </button>
                  <button class="btn btn-ghost" @click="cancelRename" :disabled="renamingBusy">Cancel</button>
                </div>
              </div>

              <div class="tracks" v-if="pl.items && pl.items.length">
                <div v-for="it in pl.items" :key="it.id" class="track">
                  <div class="track-main">
                    <div class="track-title">{{ it.title }}</div>
                    <div class="muted small">
                      <a class="link" :href="it.youTubeURL || it.youtubeUrl" target="_blank" rel="noreferrer">
                        {{ it.youTubeID || it.youtubeId || (it.youTubeURL || it.youtubeUrl) }}
                      </a>
                      • added {{ formatRelative(it.addedAt) }}
                    </div>
                  </div>
                </div>
              </div>
              <div v-else class="muted">No tracks yet.</div>

              <div class="divider"></div>

              <div class="row">
                <div class="col">
                  <label class="label">Track title</label>
                  <input v-model="addTrackTitle[pl.id]" class="input" type="text" placeholder="Song name" />
                </div>
                <div class="col">
                  <label class="label">YouTube URL</label>
                  <input
                    v-model="addTrackUrl[pl.id]"
                    class="input"
                    type="url"
                    placeholder="https://www.youtube.com/watch?v=…"
                  />
                </div>
                <div class="actions">
                  <button class="btn" @click="addItem(pl)" :disabled="addingItemId === pl.id">
                    {{ addingItemId === pl.id ? 'Adding…' : 'Add track' }}
                  </button>
                </div>
              </div>

              <p v-if="addItemErrorId === pl.id" class="error">{{ addItemError }}</p>
            </div>
          </details>
        </div>

        <div v-else class="muted">No playlists yet.</div>
      </section>

      <section class="card danger">
        <h2 class="h2">Delete account</h2>
        <p class="muted">
          This will delete your account profile on the backend (in-memory for now). In a real setup, this
          should also revoke tokens / delete server-side data.
        </p>

        <p v-if="deleteError" class="error">{{ deleteError }}</p>

        <div class="row">
          <div class="col">
            <label class="label" for="confirm">Type <code class="code">DELETE</code> to confirm</label>
            <input id="confirm" v-model="deleteConfirm" class="input" type="text" autocomplete="off" />
          </div>
          <div class="actions">
            <button class="btn btn-danger" @click="deleteAccount" :disabled="deleting || deleteConfirm !== 'DELETE'">
              {{ deleting ? 'Deleting…' : 'Delete my account' }}
            </button>
          </div>
        </div>
      </section>
    </template>
  </main>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { api } from '../lib/api'
import { useAuth } from '../stores/auth'

const router = useRouter()
const auth = useAuth()

const loadingAny = ref(false)

// Profile form
const nickname = ref('')
const pictureUrl = ref('')
const profileError = ref('')
const savingProfile = ref(false)

// Playlists
const playlists = ref([])
const playlistError = ref('')
const newPlaylistName = ref('')
const creatingPlaylist = ref(false)

// Rename
const renamingId = ref('')
const renameName = ref('')
const renamingBusy = ref(false)

// Add item per playlist (keyed by playlist id)
const addTrackTitle = reactive({})
const addTrackUrl = reactive({})
const addingItemId = ref('')
const addItemErrorId = ref('')
const addItemError = ref('')

// Delete account
const deleteConfirm = ref('')
const deleting = ref(false)
const deleteError = ref('')

const newPlaylistNameTrimmed = computed(() => (newPlaylistName.value || '').trim())
const renameTrimmed = computed(() => (renameName.value || '').trim())

const picturePreview = computed(() => {
  const u = (pictureUrl.value || '').trim()
  return u || ''
})

const initials = computed(() => {
  const n = (nickname.value || '').trim()
  if (!n) return 'P'
  const parts = n.split(/\s+/).filter(Boolean)
  const a = parts[0]?.[0] || 'P'
  const b = parts.length > 1 ? parts[1]?.[0] : ''
  return (a + b).toUpperCase()
})

async function loadProfile() {
  profileError.value = ''
  try {
    const me = await api.getMe()
    nickname.value = me?.Nickname || me?.nickname || 'Player'
    pictureUrl.value = me?.PictureURL || me?.pictureURL || me?.pictureUrl || ''
  } catch (e) {
    profileError.value = e?.message || 'Failed to load profile'
  }
}

async function saveProfile() {
  savingProfile.value = true
  profileError.value = ''
  try {
    await api.putMe({
      nickname: nickname.value,
      pictureUrl: pictureUrl.value,
    })
  } catch (e) {
    profileError.value = e?.message || 'Failed to save profile'
  } finally {
    savingProfile.value = false
  }
}

async function loadPlaylists() {
  playlistError.value = ''
  try {
    const res = await api.listMyPlaylists()
    const pls = res?.playlists || []
    playlists.value = Array.isArray(pls) ? pls : []
  } catch (e) {
    playlistError.value = e?.message || 'Failed to load playlists'
  }
}

async function createPlaylist() {
  if (!newPlaylistNameTrimmed.value) return
  creatingPlaylist.value = true
  playlistError.value = ''
  try {
    await api.createMyPlaylist({ name: newPlaylistNameTrimmed.value })
    newPlaylistName.value = ''
    await loadPlaylists()
  } catch (e) {
    playlistError.value = e?.message || 'Failed to create playlist'
  } finally {
    creatingPlaylist.value = false
  }
}

function beginRename(pl) {
  renamingId.value = pl.id
  renameName.value = pl.name
}

function cancelRename() {
  renamingId.value = ''
  renameName.value = ''
}

async function applyRename(pl) {
  if (!renameTrimmed.value) return
  renamingBusy.value = true
  playlistError.value = ''
  try {
    await api.patchMyPlaylist(pl.id, { name: renameTrimmed.value })
    renamingId.value = ''
    renameName.value = ''
    await loadPlaylists()
  } catch (e) {
    playlistError.value = e?.message || 'Failed to rename playlist'
  } finally {
    renamingBusy.value = false
  }
}

async function addItem(pl) {
  addItemErrorId.value = ''
  addItemError.value = ''
  addingItemId.value = pl.id

  const title = (addTrackTitle[pl.id] || '').trim()
  const youtubeUrl = (addTrackUrl[pl.id] || '').trim()

  try {
    if (!title || !youtubeUrl) {
      throw new Error('Title and YouTube URL are required')
    }
    await api.addMyPlaylistItem(pl.id, { title, youtubeUrl })
    addTrackTitle[pl.id] = ''
    addTrackUrl[pl.id] = ''
    await loadPlaylists()
  } catch (e) {
    addItemErrorId.value = pl.id
    addItemError.value = e?.message || 'Failed to add track'
  } finally {
    addingItemId.value = ''
  }
}

async function deleteAccount() {
  if (deleteConfirm.value !== 'DELETE') return
  deleting.value = true
  deleteError.value = ''
  try {
    await api.deleteMe()
    // Also clear client-side auth (temporary shim)
    auth.logout()
    await router.push('/')
  } catch (e) {
    deleteError.value = e?.message || 'Failed to delete account'
  } finally {
    deleting.value = false
    deleteConfirm.value = ''
  }
}

async function refreshAll() {
  loadingAny.value = true
  try {
    await Promise.all([loadProfile(), loadPlaylists()])
  } finally {
    loadingAny.value = false
  }
}

function formatRelative(isoOrDate) {
  const d = new Date(isoOrDate)
  if (Number.isNaN(d.getTime())) return 'unknown'
  const diff = Date.now() - d.getTime()
  const sec = Math.round(diff / 1000)
  if (sec < 10) return 'just now'
  if (sec < 60) return `${sec}s ago`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.round(min / 60)
  if (hr < 48) return `${hr}h ago`
  const day = Math.round(hr / 24)
  return `${day}d ago`
}

onMounted(async () => {
  if (!auth.isAuthenticated.value) return
  await refreshAll()
})
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
}
.link.router-link-active {
  font-weight: 600;
  opacity: 1;
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
  background: rgba(255, 80, 80, 0.22);
  border-color: rgba(255, 80, 80, 0.35);
}
.btn-danger:hover {
  background: rgba(255, 80, 80, 0.32);
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

.error {
  margin-top: 10px;
  color: #ffb3b3;
}

.code {
  padding: 2px 6px;
  border-radius: 8px;
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  background: rgba(255, 255, 255, 0.04);
}

.avatar {
  width: 64px;
  height: 64px;
  border-radius: 16px;
  overflow: hidden;
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  background: rgba(255, 255, 255, 0.04);
  display: grid;
  place-items: center;
}
.avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.avatar-fallback {
  font-weight: 700;
  opacity: 0.9;
}

.playlist-list {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.playlist {
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  border-radius: 12px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.03);
}

.playlist-summary {
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 12px;
  list-style: none;
}
.playlist-summary::-webkit-details-marker {
  display: none;
}

.playlist-title {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.playlist-name {
  font-weight: 650;
}

.playlist-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.playlist-body {
  padding: 12px;
  border-top: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
}

.tracks {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 10px;
}

.track {
  padding: 10px;
  border-radius: 12px;
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  background: rgba(255, 255, 255, 0.02);
}
.track-title {
  font-weight: 600;
}
.track-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.divider {
  margin: 14px 0;
  height: 1px;
  background: var(--color-border, rgba(255, 255, 255, 0.12));
}

.danger {
  border-color: rgba(255, 80, 80, 0.35);
  background: rgba(255, 80, 80, 0.06);
}
</style>
