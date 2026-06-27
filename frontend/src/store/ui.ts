import { create } from "zustand";
import { persist } from "zustand/middleware";

type Mode = "light" | "dark";

interface UIState {
  mode: Mode;
  toggleMode: () => void;
  setMode: (m: Mode) => void;
  sidebarOpen: boolean;
  toggleSidebar: () => void;
  setSidebarOpen: (open: boolean) => void;
}

// Persisted UI preferences (theme mode, sidebar visibility). Survives reloads
// via localStorage.
export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      mode: "dark",
      toggleMode: () => set((s) => ({ mode: s.mode === "dark" ? "light" : "dark" })),
      setMode: (m) => set({ mode: m }),
      sidebarOpen: true,
      toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
      setSidebarOpen: (open) => set({ sidebarOpen: open }),
    }),
    { name: "fleet-ui" },
  ),
);
