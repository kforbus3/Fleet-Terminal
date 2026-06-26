import { useEffect, useRef, useState } from "react";
import {
  Box, Chip, Drawer, IconButton, Paper, Stack, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Typography, Divider, CircularProgress,
} from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { useQuery } from "@tanstack/react-query";
import { getRecording, listSessions, type SSHSession } from "../api/sessions";

// One parsed asciicast v2 output frame: absolute time offset (seconds) + bytes.
interface CastFrame {
  time: number;
  data: string;
}

interface ParsedCast {
  width: number;
  height: number;
  frames: CastFrame[];
}

// Parse an asciicast v2 stream: a JSON header line followed by one JSON event
// array ([time, type, data]) per line. Only "o" (output) events are replayed.
function parseCast(cast: string): ParsedCast {
  const lines = cast.split("\n").filter((l) => l.trim() !== "");
  const result: ParsedCast = { width: 80, height: 24, frames: [] };
  if (lines.length === 0) return result;
  try {
    const header = JSON.parse(lines[0]) as { width?: number; height?: number };
    if (header.width) result.width = header.width;
    if (header.height) result.height = header.height;
  } catch {
    // No valid header; fall through and treat all lines as events.
  }
  for (const line of lines.slice(1)) {
    try {
      const [time, type, data] = JSON.parse(line) as [number, string, string];
      if (type === "o") result.frames.push({ time, data });
    } catch {
      // Skip malformed event lines.
    }
  }
  return result;
}

function ReplayTerminal({ sessionId }: { sessionId: string }) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const { data, isLoading, isError } = useQuery({
    queryKey: ["recording", sessionId],
    queryFn: () => getRecording(sessionId),
    retry: false,
  });

  useEffect(() => {
    if (!data || !containerRef.current) return;
    const parsed = parseCast(data.cast);
    const term = new Terminal({
      cols: parsed.width, rows: parsed.height,
      fontSize: 12, convertEol: true, disableStdin: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(containerRef.current);
    try { fit.fit(); } catch { /* container may not be laid out yet */ }

    // Schedule each output frame at its asciicast timestamp (seconds -> ms).
    const timers: ReturnType<typeof setTimeout>[] = [];
    for (const frame of parsed.frames) {
      timers.push(setTimeout(() => term.write(frame.data), frame.time * 1000));
    }

    return () => {
      timers.forEach(clearTimeout);
      term.dispose();
    };
  }, [data]);

  if (isLoading) {
    return <Box sx={{ p: 3, textAlign: "center" }}><CircularProgress size={24} /></Box>;
  }
  if (isError || !data) {
    return (
      <Typography color="text.secondary" sx={{ p: 2 }}>
        No recording is available for this session.
      </Typography>
    );
  }
  return (
    <Box>
      <Typography variant="caption" color="text.secondary">
        {data.recording.format} · {(data.recording.sizeBytes / 1024).toFixed(1)} KiB ·
        {" "}{Math.round(data.recording.durationMs / 1000)}s
      </Typography>
      <Box ref={containerRef} sx={{ mt: 1, height: 480, bgcolor: "#000", p: 1, borderRadius: 1 }} />
    </Box>
  );
}

// Recorded SSH session browser. Selecting a row opens a replay drawer that
// streams the asciicast recording back through an xterm.js terminal, honoring
// the original frame timing.
export function SessionsPage() {
  const { data: sessions = [], isLoading } = useQuery({ queryKey: ["sessions"], queryFn: listSessions });
  const [active, setActive] = useState<SSHSession | null>(null);

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 2 }}>Session Replay</Typography>

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Started</TableCell>
              <TableCell>User</TableCell>
              <TableCell>Host</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Bytes (in/out)</TableCell>
              <TableCell>Client IP</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {sessions.map((s) => (
              <TableRow key={s.id} hover sx={{ cursor: "pointer" }} onClick={() => setActive(s)}>
                <TableCell>{new Date(s.startedAt).toLocaleString()}</TableCell>
                <TableCell>{s.username}</TableCell>
                <TableCell>{s.hostname}</TableCell>
                <TableCell><Chip label={s.status} size="small" /></TableCell>
                <TableCell>{s.bytesIn} / {s.bytesOut}</TableCell>
                <TableCell>{s.clientIp}</TableCell>
              </TableRow>
            ))}
            {!isLoading && sessions.length === 0 && (
              <TableRow><TableCell colSpan={6}>
                <Typography color="text.secondary">No recorded sessions.</Typography>
              </TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Drawer anchor="right" open={active !== null} onClose={() => setActive(null)}
        PaperProps={{ sx: { width: { xs: "100%", md: 760 }, p: 2 } }}>
        {active && (
          <Box>
            <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
              <Typography variant="h6" sx={{ flexGrow: 1 }}>
                {active.username}@{active.hostname}
              </Typography>
              <IconButton onClick={() => setActive(null)}><CloseIcon /></IconButton>
            </Stack>
            <Stack direction="row" spacing={2} sx={{ mb: 1 }}>
              <Typography variant="caption" color="text.secondary">
                Started {new Date(active.startedAt).toLocaleString()}
              </Typography>
              {active.endedAt && (
                <Typography variant="caption" color="text.secondary">
                  Ended {new Date(active.endedAt).toLocaleString()}
                </Typography>
              )}
              {active.exitCode !== undefined && (
                <Typography variant="caption" color="text.secondary">
                  Exit {active.exitCode}
                </Typography>
              )}
            </Stack>
            <Divider sx={{ mb: 2 }} />
            <ReplayTerminal sessionId={active.id} />
          </Box>
        )}
      </Drawer>
    </Box>
  );
}
