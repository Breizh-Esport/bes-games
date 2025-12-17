// Tiny reactive auth store placeholder.
//
// This is NOT real OIDC. It's a minimal client-side shim so you can build the
// multi-page app flow now and swap in a proper OIDC client later.
//
// Current behavior:
// - "Authenticated" == a non-empty `sub` stored in localStorage
// - You can set/clear the `sub` to simulate login/logout
//
// Backend expectation (temporary):
// - Authenticated requests send `X-User-Sub: <sub>` (handled in `src/lib/api.js`)
//
// Replace this with a real OIDC integration later (e.g. oidc-client-ts) and keep
// the public surface area the same if possible.

import { reactive, readonly, computed } from 'vue'
import { getUserSub, setUserSub, clearUserSub } from '../lib/api'

const state = reactive({
  sub: getUserSub(),
})

const isAuthenticated = computed(() => !!state.sub)

function syncFromStorage() {
  state.sub = getUserSub()
}

// Best-effort cross-tab sync
if (typeof window !== 'undefined' && window.addEventListener) {
  window.addEventListener('storage', (e) => {
    // localStorage key lives in lib/api.js; we can't import it directly without exporting it.
    // Just resync on any localStorage change.
    if (e.storageArea === localStorage) syncFromStorage()
  })
}

export function useAuth() {
  return {
    state: readonly(state),
    isAuthenticated,

    // Placeholder "login": set an arbitrary subject.
    // For a real OIDC integration this would be a redirect to the provider.
    loginWithSub(sub) {
      const v = setUserSub(sub)
      state.sub = v
      return v
    },

    // Placeholder "logout"
    logout() {
      clearUserSub()
      state.sub = ''
    },

    // Accessors
    getSub() {
      return state.sub
    },

    // Useful for dev/test pages
    setSub(sub) {
      return this.loginWithSub(sub)
    },

    // Re-read localStorage
    refresh() {
      syncFromStorage()
    },
  }
}
