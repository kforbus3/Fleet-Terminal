import { create } from "zustand";
import { setAccessToken } from "../api/client";
import * as authApi from "../api/auth";
import type { User } from "../api/auth";

interface AuthState {
  user: User | null;
  permissions: string[];
  accessToken: string | null;
  isSuperAdmin: boolean;
  loaded: boolean;
  // login returns {mfaRequired:true, challenge} when a second factor is needed;
  // otherwise the session is established.
  login: (username: string, password: string) => Promise<{ mfaRequired?: boolean; challenge?: string }>;
  verifyMfa: (challenge: string, code: string) => Promise<void>;
  logout: () => Promise<void>;
  loadMe: () => Promise<void>;
  has: (perm: string) => boolean;
}

// Authentication store. The access token lives in memory only (never persisted);
// session continuity across reloads relies on the HttpOnly refresh cookie +
// loadMe(). Super Admins and the Admin.All wildcard satisfy every permission.
export const useAuthStore = create<AuthState>()((set, get) => ({
  user: null,
  permissions: [],
  accessToken: null,
  isSuperAdmin: false,
  loaded: false,

  login: async (username, password) => {
    const res = await authApi.login(username, password);
    if (res.mfaRequired) {
      return { mfaRequired: true, challenge: res.challenge };
    }
    setAccessToken(res.accessToken ?? null);
    set({ user: res.user ?? null, accessToken: res.accessToken ?? null });
    await get().loadMe();
    return {};
  },

  verifyMfa: async (challenge, code) => {
    const res = await authApi.mfaVerify(challenge, code);
    setAccessToken(res.accessToken ?? null);
    set({ user: res.user ?? null, accessToken: res.accessToken ?? null });
    await get().loadMe();
  },

  logout: async () => {
    try {
      await authApi.logout();
    } finally {
      setAccessToken(null);
      set({
        user: null,
        permissions: [],
        accessToken: null,
        isSuperAdmin: false,
        loaded: true,
      });
    }
  },

  loadMe: async () => {
    try {
      const res = await authApi.me();
      set({
        user: res.user,
        permissions: res.permissions,
        isSuperAdmin: res.isSuperAdmin,
        loaded: true,
      });
    } catch {
      setAccessToken(null);
      set({
        user: null,
        permissions: [],
        accessToken: null,
        isSuperAdmin: false,
        loaded: true,
      });
    }
  },

  has: (perm) => {
    const { isSuperAdmin, permissions } = get();
    if (isSuperAdmin) return true;
    return permissions.includes("Admin.All") || permissions.includes(perm);
  },
}));
