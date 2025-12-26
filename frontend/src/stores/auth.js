// Minimal OIDC-aware auth store (server-managed session cookie).
//
// The backend handles the OIDC flow and issues an HTTP-only session cookie.
// The frontend only triggers login/logout and checks /api/me to detect state.

import { reactive, readonly, computed } from "vue";
import { api, getApiBaseUrl } from "../lib/api";

const state = reactive({
  sub: "",
  profile: null,
  status: "unknown",
});

const isAuthenticated = computed(() => state.status === "authenticated");

async function refresh() {
  try {
    const profile = await api.getMe();
    state.profile = profile || null;
    state.sub = profile?.sub || "";
    state.status = "authenticated";
  } catch {
    state.profile = null;
    state.sub = "";
    state.status = "anonymous";
  }
}

function login(returnTo) {
  const base = getApiBaseUrl();
  const target = returnTo || (typeof window !== "undefined" ? window.location.href : "/");
  window.location.href = `${base}/auth/login?returnTo=${encodeURIComponent(target)}`;
}

function logout(returnTo) {
  const base = getApiBaseUrl();
  const target = returnTo || (typeof window !== "undefined" ? window.location.href : "/");
  window.location.href = `${base}/auth/logout?returnTo=${encodeURIComponent(target)}`;
}

export function useAuth() {
  return {
    state: readonly(state),
    isAuthenticated,
    refresh,
    login,
    logout,
    getSub() {
      return state.sub;
    },
  };
}
