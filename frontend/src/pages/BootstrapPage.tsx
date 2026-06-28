import { useEffect, useState } from "react";
import {
  Alert, Box, Button, Card, CardContent, CircularProgress, Stack, TextField,
  Typography,
} from "@mui/material";
import TerminalIcon from "@mui/icons-material/Terminal";
import { useNavigate } from "react-router-dom";
import { bootstrapInit, bootstrapStatus } from "../api/auth";
import { useAppName, useDocumentTitle } from "../api/branding";

// First-run wizard. Creates the initial Super Administrator while the platform
// has no accounts; once any user exists the backend reports the wizard closed
// and we redirect to the login screen.
export function BootstrapPage() {
  const navigate = useNavigate();
  const appName = useAppName();
  useDocumentTitle();
  const [checking, setChecking] = useState(true);
  const [available, setAvailable] = useState(false);
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    bootstrapStatus()
      .then((s) => {
        if (!active) return;
        setAvailable(s.bootstrapAvailable);
        if (!s.bootstrapAvailable) navigate("/login", { replace: true });
      })
      .catch(() => active && setError("Could not check setup status."))
      .finally(() => active && setChecking(false));
    return () => {
      active = false;
    };
  }, [navigate]);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await bootstrapInit({ username, email, displayName, password });
      navigate("/login", { replace: true });
    } catch (err) {
      setError(extractError(err));
    } finally {
      setSubmitting(false);
    }
  };

  if (checking) {
    return (
      <Box
        sx={{
          minHeight: "100vh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <CircularProgress />
      </Box>
    );
  }

  if (!available) return null;

  return (
    <Box
      sx={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        p: 2,
      }}
    >
      <Card sx={{ width: 420, maxWidth: "100%" }}>
        <CardContent sx={{ p: 4 }}>
          <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 3 }}>
            <TerminalIcon color="primary" />
            <Typography variant="h6" sx={{ fontWeight: 600 }}>
              {appName}
            </Typography>
          </Stack>
          <Typography variant="h5" gutterBottom>
            Create Super Administrator
          </Typography>
          <Typography color="text.secondary" variant="body2" sx={{ mb: 2 }}>
            This is a one-time setup for the first account on this platform.
          </Typography>
          <Box component="form" onSubmit={onSubmit}>
            <Stack spacing={2}>
              {error && <Alert severity="error">{error}</Alert>}
              <TextField
                label="Username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoFocus
                fullWidth
                required
              />
              <TextField
                label="Display name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                fullWidth
              />
              <TextField
                label="Email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                fullWidth
              />
              <TextField
                label="Password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                fullWidth
                required
              />
              <Button
                type="submit"
                variant="contained"
                size="large"
                disabled={submitting}
              >
                {submitting ? "Creating…" : "Create administrator"}
              </Button>
            </Stack>
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
}

interface ApiErrorShape {
  response?: { data?: { error?: string } };
}

function extractError(err: unknown): string {
  const e = err as ApiErrorShape;
  return e.response?.data?.error ?? "Setup failed. Please try again.";
}
