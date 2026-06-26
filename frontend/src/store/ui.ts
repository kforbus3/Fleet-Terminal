import { create } from "zustand";
import { persist } from "zustand/middleware";

type Mode = "light" | "dark";

interface UIState {
  mode: Mode;
  toggleMode: () => void;
  setMode: (m: Mode) => void;
}

// Persisted UI preferences (theme mode). Survives reloads via localStorage.
export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      mode: "dark",
      toggleMode: () => set((s) => ({ mode: s.mode === "dark" ? "light" : "dark" })),
      setMode: (m) => set({ mode: m }),
    }),
    { name: "fleet-ui" },
  ),
);
