import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { Box, Chip, Stack, Typography } from "@mui/material";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { useAuthStore } from "../store/auth";

type Status = "connecting" | "connected" | "closed" | "error";

// Live browser SSH terminal. Connects to the backend WebSocket gateway, which is
// the sole SSH client. Terminal output arrives as binary frames; keystrokes are
// sent as binary; resize is sent as a JSON control message. The browser never
// holds SSH keys or certificates.
export function TerminalPage() {
  const { hostId } = useParams<{ hostId: string }>();
  const accessToken = useAuthStore((s) => s.accessToken);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [status, setStatus] = useState<Status>("connecting");

  useEffect(() => {
    if (!containerRef.current || !hostId || !accessToken) return;

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "monospace",
      fontSize: 13,
      scrollback: 100000,
      theme: { background: "#0d1117" },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(containerRef.current);
    fit.fit();

    const proto = window.location.protocol === "https:" ? "wss" : "ws";
    const url = `${proto}://${window.location.host}/api/v1/terminal/${hostId}?token=${encodeURIComponent(accessToken)}`;
    const ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";

    const sendResize = () => {
      fit.fit();
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
      }
    };

    ws.onopen = () => {
      setStatus("connected");
      sendResize();
    };
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === "error") term.write(`\r\n\x1b[31m${msg.data}\x1b[0m\r\n`);
        } catch {
          term.write(ev.data);
        }
      } else {
        term.write(new Uint8Array(ev.data));
      }
    };
    ws.onclose = () => setStatus("closed");
    ws.onerror = () => setStatus("error");

    const dataSub = term.onData((d) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(new TextEncoder().encode(d));
    });
    window.addEventListener("resize", sendResize);

    return () => {
      window.removeEventListener("resize", sendResize);
      dataSub.dispose();
      ws.close();
      term.dispose();
    };
  }, [hostId, accessToken]);

  const color =
    status === "connected" ? "success" : status === "connecting" ? "warning" : "error";

  return (
    <Box>
      <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 1 }}>
        <Typography variant="h6">Terminal</Typography>
        <Chip size="small" label={status} color={color} />
      </Stack>
      <Box
        ref={containerRef}
        sx={{ height: "calc(100vh - 160px)", bgcolor: "#0d1117", p: 1, borderRadius: 1 }}
      />
    </Box>
  );
}
