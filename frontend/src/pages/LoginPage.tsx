import { useEffect, useState } from "react";
import {
  Alert, Box, Button, Card, CardContent, Stack, TextField, Typography,
} from "@mui/material";
import TerminalIcon from "@mui/icons-material/Terminal";
import { useNavigate } from "react-router-dom";
import { useAuthStore } from "../store/auth";
import { bootstrapStatus } from "../api/auth";

// Credentials login form. On success the auth store holds the access token and
// principal; we then route to the dashboard. On first run (no users yet) we send
// the operator to the bootstrap wizard instead of showing a sign-in form.
export function LoginPage() {
  const navigate = useNavigate();
  const login = useAuthStore((s) => s.login);
  const verifyMfa = useAuthStore((s) => s.verifyMfa);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [challenge, setChallenge] = useState<string | null>(null);
  const [code, setCode] = useState("");

  useEffect(() => {
    let active = true;
    bootstrapStatus()
      .then((s) => {
        if (active && s.bootstrapAvailable) navigate("/bootstrap", { replace: true });
      })
      .catch(() => {/* backend unreachable — stay on the sign-in form */});
    return () => {
      active = false;
    };
  }, [navigate]);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      if (challenge) {
        await verifyMfa(challenge, code);
        navigate("/", { replace: true });
        return;
      }
      const res = await login(username, password);
      if (res.mfaRequired && res.challenge) {
        setChallenge(res.challenge);
      } else {
        navigate("/", { replace: true });
      }
    } catch (err) {
      setError(extractError(err));
    } finally {
      setSubmitting(false);
    }
  };

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
      <Card sx={{ width: 380, maxWidth: "100%" }}>
        <CardContent sx={{ p: 4 }}>
          <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 3 }}>
            <TerminalIcon color="primary" />
            <Typography variant="h6" sx={{ fontWeight: 600 }}>
              Fleet Terminal
            </Typography>
          </Stack>
          <Typography variant="h5" gutterBottom>
            {challenge ? "Two-factor verification" : "Sign in"}
          </Typography>
          <Box component="form" onSubmit={onSubmit} sx={{ mt: 2 }}>
            <Stack spacing={2}>
              {error && <Alert severity="error">{error}</Alert>}
              {challenge ? (
                <TextField
                  label="Authenticator code"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  autoFocus
                  fullWidth
                  required
                  inputProps={{ inputMode: "numeric", pattern: "[0-9]*", maxLength: 6 }}
                  helperText="Enter the 6-digit code from your authenticator app"
                />
              ) : (
                <>
                  <TextField
                    label="Username"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    autoFocus
                    fullWidth
                    required
                  />
                  <TextField
                    label="Password"
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    fullWidth
                    required
                  />
                </>
              )}
              <Button
                type="submit"
                variant="contained"
                size="large"
                disabled={submitting}
              >
                {submitting ? "Please wait…" : challenge ? "Verify" : "Sign in"}
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
  return e.response?.data?.error ?? "Sign in failed. Please try again.";
}
