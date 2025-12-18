<template>
  <div>
    <div class="row row-space">
      <div>
        <h2 class="h2">Playlists</h2>
        <p class="muted">Create playlists and add YouTube tracks (title + URL).</p>
      </div>

      <div class="actions">
        <button class="btn btn-ghost" @click="loadPlaylists" :disabled="loading">
          {{ loading ? 'Refreshing...' : 'Refresh' }}
        </button>
      </div>
    </div>

    <section class="card-inner" v-if="!auth.isAuthenticated.value">
      <h3 class="h3">Not authenticated</h3>
      <p class="muted">
        Playlists are tied to your account. Set a <code class="code">sub</code> on the Home page or in the lobby to
        simulate being logged in.
      </p>
      <RouterLink class="btn" to="/games/name-that-tune">Go to lobby</RouterLink>
    </section>

    <template v-else>
      <p v-if="error" class="error">{{ error }}</p>

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
          <button class="btn" @click="createPlaylist" :disabled="creating || !newPlaylistNameTrimmed">
            {{ creating ? 'Creating...' : 'Create playlist' }}
          </button>
        </div>
      </div>

      <div v-if="playlists.length" class="playlist-list">
        <details v-for="pl in playlists" :key="pl.id" class="playlist">
          <summary class="playlist-summary">
            <div class="playlist-title">
              <div class="playlist-name">{{ pl.name }}</div>
              <div class="muted small">{{ pl.items?.length || 0 }} tracks • updated {{ formatRelative(pl.updatedAt) }}</div>
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
                  {{ renamingBusy ? 'Saving...' : 'Save' }}
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
                  placeholder="https://www.youtube.com/watch?v=dQw4w9WgXcQ"
                />
              </div>
              <div class="actions">
                <button class="btn" @click="addItem(pl)" :disabled="addingItemId === pl.id">
                  {{ addingItemId === pl.id ? 'Adding...' : 'Add track' }}
                </button>
              </div>
            </div>

            <p v-if="addItemErrorId === pl.id" class="error">{{ addItemError }}</p>
          </div>
        </details>
      </div>

      <div v-else class="muted">No playlists yet.</div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../../../../lib/api'
import { useAuth } from '../../../../stores/auth'

const auth = useAuth()
const gameId = 'name-that-tune'

const playlists = ref([])
const loading = ref(false)
const error = ref('')

const newPlaylistName = ref('')
const creating = ref(false)

const renamingId = ref('')
const renameName = ref('')
const renamingBusy = ref(false)

const addTrackTitle = reactive({})
const addTrackUrl = reactive({})
const addingItemId = ref('')
const addItemErrorId = ref('')
const addItemError = ref('')

const newPlaylistNameTrimmed = computed(() => (newPlaylistName.value || '').trim())
const renameTrimmed = computed(() => (renameName.value || '').trim())

async function loadPlaylists() {
  if (!auth.isAuthenticated.value) return
  loading.value = true
  error.value = ''
  try {
    const res = await api.listPlaylists(gameId)
    const pls = res?.playlists || []
    playlists.value = Array.isArray(pls) ? pls : []
  } catch (e) {
    error.value = e?.message || 'Failed to load playlists'
  } finally {
    loading.value = false
  }
}

async function createPlaylist() {
  if (!newPlaylistNameTrimmed.value) return
  creating.value = true
  error.value = ''
  try {
    await api.createPlaylist(gameId, { name: newPlaylistNameTrimmed.value })
    newPlaylistName.value = ''
    await loadPlaylists()
  } catch (e) {
    error.value = e?.message || 'Failed to create playlist'
  } finally {
    creating.value = false
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
  error.value = ''
  try {
    await api.patchPlaylist(gameId, pl.id, { name: renameTrimmed.value })
    renamingId.value = ''
    renameName.value = ''
    await loadPlaylists()
  } catch (e) {
    error.value = e?.message || 'Failed to rename playlist'
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
    if (!title || !youtubeUrl) throw new Error('Title and YouTube URL are required')
    await api.addPlaylistItem(gameId, pl.id, { title, youtubeUrl })
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

onMounted(() => {
  loadPlaylists()
})
</script>

<style scoped>
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

.muted {
  opacity: 0.8;
}
.small {
  font-size: 0.9rem;
}

.h2 {
  margin: 0 0 8px;
}
.h3 {
  margin: 0 0 8px;
}

.error {
  margin-top: 10px;
  color: #ffb3b3;
}

.code {
  padding: 2px 6px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.card-inner {
  margin-top: 12px;
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  border-radius: 12px;
  padding: 12px;
  background: rgba(255, 255, 255, 0.03);
}

.playlist-list {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.playlist {
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  border-radius: 12px;
  padding: 10px 12px;
  background: rgba(255, 255, 255, 0.03);
}

.playlist-summary {
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  gap: 12px;
  list-style: none;
}

.playlist-title {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.playlist-name {
  font-weight: 700;
}

.playlist-body {
  margin-top: 12px;
}

.divider {
  margin: 14px 0;
  height: 1px;
  background: rgba(255, 255, 255, 0.12);
}

.tracks {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.track {
  padding: 10px 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
}

.track-title {
  font-weight: 650;
}

.link {
  color: inherit;
}
</style>
