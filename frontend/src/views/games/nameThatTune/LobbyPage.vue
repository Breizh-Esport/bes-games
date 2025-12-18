<template>
  <main class="page">
    <section class="hero">
      <h1 class="title">Name That Tune</h1>
      <p class="subtitle muted">Create or join rooms, then play with playlists and the buzzer.</p>
      <div class="hero-actions">
        <RouterLink class="btn btn-ghost" to="/">Back to games</RouterLink>
        <RouterLink class="btn btn-ghost" to="/profile">Profile</RouterLink>
        <RouterLink
          class="btn btn-ghost"
          to="/games/name-that-tune/settings/playlists"
          :class="{ disabled: !auth.isAuthenticated.value }"
        >
          Game settings
        </RouterLink>
      </div>
    </section>

    <section class="card">
      <h2 class="h2">Login</h2>
      <p class="muted">
        Login/registration will be done through an OIDC provider. For now, this is a placeholder: set a
        subject string to simulate being authenticated.
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
          <button class="btn" @click="onLogin" :disabled="!loginSubTrimmed">Set sub</button>
          <button class="btn btn-ghost" @click="onLogout" :disabled="!auth.isAuthenticated.value">Clear</button>
        </div>
      </div>

      <div class="row">
        <div class="badge" :class="auth.isAuthenticated.value ? 'badge-ok' : 'badge-warn'">
          <span class="badge-label">Auth</span>
          <span class="badge-value">
            {{ auth.isAuthenticated.value ? `Authenticated (${auth.state.sub})` : 'Anonymous' }}
          </span>
        </div>

        <div class="right">
          <RouterLink class="btn btn-ghost" to="/profile" :class="{ disabled: !auth.isAuthenticated.value }">
            Go to profile
          </RouterLink>
        </div>
      </div>
    </section>

    <section class="grid">
      <div class="card">
        <h2 class="h2">Create a room</h2>
        <p class="muted">You must be authenticated to create a room.</p>

        <div class="row">
          <div class="col">
            <label class="label" for="roomName">Room name</label>
            <input
              id="roomName"
              v-model="createRoomName"
              class="input"
              type="text"
              placeholder="Friday Night Blindtest"
              autocomplete="off"
            />
          </div>
          <div class="actions">
            <button class="btn" @click="onCreateRoom" :disabled="!auth.isAuthenticated.value || creatingRoom">
              {{ creatingRoom ? 'Creating...' : 'Create' }}
            </button>
          </div>
        </div>

        <p v-if="createRoomError" class="error">{{ createRoomError }}</p>
      </div>

      <div class="card">
        <div class="row row-space">
          <div>
            <h2 class="h2">Available rooms</h2>
            <p class="muted">Anyone can join rooms anonymously. Rooms show online player count (best-effort).</p>
          </div>

          <div class="actions">
            <button class="btn btn-ghost" @click="refreshRooms" :disabled="loadingRooms">
              {{ loadingRooms ? 'Refreshing...' : 'Refresh' }}
            </button>
          </div>
        </div>

        <p v-if="roomsError" class="error">{{ roomsError }}</p>

        <div v-if="loadingRooms" class="muted">Loading rooms...</div>

        <template v-else>
          <ul class="room-list" v-if="rooms.length">
            <li v-for="room in rooms" :key="room.roomId" class="room-item">
              <div class="room-meta">
                <div class="room-name">{{ room.name }}</div>
                <div class="room-stats">
                  <span class="pill">{{ room.onlinePlayers }} online</span>
                  <span class="pill pill-dim">updated {{ formatRelative(room.updatedAt) }}</span>
                </div>
                <div class="room-id muted">Room ID: {{ room.roomId }}</div>
              </div>

              <div class="room-actions">
                <button class="btn btn-ghost" @click="prefillJoinRoom(room.roomId)">Join</button>
                <RouterLink class="btn btn-ghost" :to="roomLink(room.roomId)">Open</RouterLink>
              </div>
            </li>
          </ul>

          <div v-else class="muted">No rooms yet. Create one (requires auth).</div>
        </template>
      </div>
    </section>

    <section class="card">
      <h2 class="h2">Join a room</h2>
      <p class="muted">Join anonymously, or as authenticated user if you set a sub above.</p>

      <div class="row">
        <div class="col">
          <label class="label" for="joinRoomId">Room ID</label>
          <input
            id="joinRoomId"
            v-model="joinRoomId"
            class="input"
            type="text"
            placeholder="room_..."
            autocomplete="off"
          />
        </div>

        <div class="col">
          <label class="label" for="nickname">Nickname (optional)</label>
          <input
            id="nickname"
            v-model="joinNickname"
            class="input"
            type="text"
            placeholder="Anonymous"
            autocomplete="off"
          />
        </div>

        <div class="col">
          <label class="label" for="pictureUrl">Profile picture URL (optional)</label>
          <input
            id="pictureUrl"
            v-model="joinPictureUrl"
            class="input"
            type="url"
            placeholder="https://example.com/avatar.png"
            autocomplete="off"
          />
        </div>

        <div class="actions">
          <button class="btn" @click="onJoinRoom" :disabled="!joinRoomIdTrimmed || joiningRoom">
            {{ joiningRoom ? 'Joining...' : 'Join' }}
          </button>
        </div>
      </div>

      <p v-if="joinError" class="error">{{ joinError }}</p>

      <div v-if="joinResult" class="result">
        <div class="result-line">
          Joined room <strong>{{ joinResult.roomId }}</strong> as player <strong>{{ joinResult.playerId }}</strong>
        </div>
        <div class="result-actions">
          <RouterLink class="btn" :to="roomLink(joinResult.roomId)">Go to room</RouterLink>
          <button class="btn btn-ghost" @click="clearJoinResult">Dismiss</button>
        </div>
      </div>
    </section>

    <footer class="footer muted">
      <div>
        Backend API base:
        <code class="code">{{ apiBase }}</code>
      </div>
      <div class="muted">
        Tip: set <code class="code">VITE_API_BASE_URL</code> for the frontend to point to your Go server.
      </div>
    </footer>
  </main>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { api, getApiBaseUrl } from '../../../lib/api'
import { useAuth } from '../../../stores/auth'

const router = useRouter()
const auth = useAuth()

const apiBase = getApiBaseUrl()
const gameId = 'name-that-tune'

// Login placeholder
const loginSub = ref(auth.state.sub || '')
const loginSubTrimmed = computed(() => (loginSub.value || '').trim())

function onLogin() {
  if (!loginSubTrimmed.value) return
  auth.loginWithSub(loginSubTrimmed.value)
}

function onLogout() {
  auth.logout()
  loginSub.value = ''
}

function roomLink(roomId) {
  return `/games/name-that-tune/rooms/${encodeURIComponent(roomId)}`
}

// Rooms
const rooms = ref([])
const loadingRooms = ref(false)
const roomsError = ref('')

async function refreshRooms() {
  loadingRooms.value = true
  roomsError.value = ''
  try {
    const res = await api.listRooms(gameId)
    rooms.value = Array.isArray(res?.rooms) ? res.rooms : []
  } catch (e) {
    roomsError.value = e?.message || 'Failed to load rooms'
  } finally {
    loadingRooms.value = false
  }
}

// Create room
const createRoomName = ref('')
const creatingRoom = ref(false)
const createRoomError = ref('')

async function onCreateRoom() {
  if (!auth.isAuthenticated.value) {
    createRoomError.value = 'You must be authenticated to create a room.'
    return
  }
  creatingRoom.value = true
  createRoomError.value = ''
  try {
    const res = await api.createRoom(gameId, { name: createRoomName.value })
    const roomId = res?.RoomID || res?.roomID || res?.roomId
    if (!roomId) throw new Error('Backend did not return a roomId')
    await refreshRooms()
    await router.push(roomLink(roomId))
  } catch (e) {
    createRoomError.value = e?.message || 'Failed to create room'
  } finally {
    creatingRoom.value = false
  }
}

// Join room
const joinRoomId = ref('')
const joinRoomIdTrimmed = computed(() => (joinRoomId.value || '').trim())
const joinNickname = ref('')
const joinPictureUrl = ref('')
const joiningRoom = ref(false)
const joinError = ref('')
const joinResult = ref(null)

function prefillJoinRoom(roomId) {
  joinRoomId.value = roomId
}

function clearJoinResult() {
  joinResult.value = null
}

async function onJoinRoom() {
  const roomId = joinRoomIdTrimmed.value
  if (!roomId) return

  joiningRoom.value = true
  joinError.value = ''
  joinResult.value = null

  try {
    const res = await api.joinRoom(gameId, roomId, {
      nickname: joinNickname.value || undefined,
      pictureUrl: joinPictureUrl.value || undefined,
    })

    const playerId = res?.PlayerID || res?.playerID || res?.playerId
    const actualRoomId = res?.Snapshot?.roomId || res?.snapshot?.roomId || roomId

    if (!playerId) throw new Error('Backend did not return a playerId')

    joinResult.value = { roomId: actualRoomId, playerId }
    await router.push(roomLink(actualRoomId))
  } catch (e) {
    joinError.value = e?.message || 'Failed to join room'
  } finally {
    joiningRoom.value = false
  }
}

// Helpers
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
  refreshRooms()
})
</script>

<style scoped>
.page {
  max-width: 1100px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}

.hero {
  margin-top: 8px;
  padding: 16px;
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.03);
}

.title {
  margin: 0;
  line-height: 1.1;
}

.subtitle {
  margin: 8px 0 0;
}

.hero-actions {
  margin-top: 12px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
}

@media (min-width: 980px) {
  .grid {
    grid-template-columns: 1fr 1fr;
    align-items: start;
  }
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

.right {
  margin-left: auto;
  display: flex;
  gap: 8px;
}

.error {
  margin-top: 10px;
  color: #ffb3b3;
}

.room-list {
  list-style: none;
  padding: 0;
  margin: 12px 0 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.room-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding: 12px;
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  border-radius: 12px;
}

.room-name {
  font-weight: 650;
}

.room-stats {
  margin-top: 6px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--color-border, rgba(255, 255, 255, 0.12));
  border-radius: 999px;
  padding: 4px 8px;
  font-size: 0.9rem;
}

.pill-dim {
  opacity: 0.8;
}

.room-id {
  margin-top: 6px;
  font-size: 0.95rem;
}

.room-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.result {
  margin-top: 12px;
  padding: 12px;
  border-radius: 12px;
  border: 1px solid rgba(80, 200, 120, 0.18);
  background: rgba(80, 200, 120, 0.08);
}

.result-line {
  font-weight: 550;
}

.result-actions {
  margin-top: 10px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.footer {
  margin-top: 20px;
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: space-between;
}

.code {
  padding: 2px 6px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.12);
}
</style>
