import { CssBaseline, ThemeProvider } from "@mui/material";
import { Route, Routes, Navigate } from "react-router-dom";
import { useMemo } from "react";
import { buildTheme } from "./theme";
import { useUIStore } from "./store/ui";
import { AppLayout } from "./components/AppLayout";
import { DashboardPage } from "./pages/DashboardPage";
import { PlaceholderPage } from "./pages/PlaceholderPage";

// Root component. Routing is intentionally broad now; each milestone replaces a
// PlaceholderPage with its real implementation. Permission-gating is layered in
// at M2 via a ProtectedRoute wrapper.
export function App() {
  const mode = useUIStore((s) => s.mode);
  const theme = useMemo(() => buildTheme(mode), [mode]);

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Routes>
        <Route element={<AppLayout />}>
          <Route index element={<DashboardPage />} />
          <Route path="hosts" element={<PlaceholderPage title="Host Inventory" />} />
          <Route path="terminals" element={<PlaceholderPage title="Terminals" />} />
          <Route path="sessions" element={<PlaceholderPage title="Session Replay" />} />
          <Route path="approvals" element={<PlaceholderPage title="Approval Queue" />} />
          <Route path="audit" element={<PlaceholderPage title="Audit Logs" />} />
          <Route path="users" element={<PlaceholderPage title="User Management" />} />
          <Route path="roles" element={<PlaceholderPage title="Role Management" />} />
          <Route path="groups" element={<PlaceholderPage title="Group Management" />} />
          <Route path="enrollment" element={<PlaceholderPage title="Host Enrollment" />} />
          <Route path="certificates" element={<PlaceholderPage title="Certificate Management" />} />
          <Route path="settings" element={<PlaceholderPage title="System Settings" />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ThemeProvider>
  );
}
