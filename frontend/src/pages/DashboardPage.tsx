import { Box, Card, CardContent, Grid, Typography, Chip } from "@mui/material";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getVersion } from "../api/client";
import { getHostStatusStats } from "../api/hosts";
import { useFleetEvents } from "../api/events";

// Landing dashboard. Shows backend connectivity plus live fleet health, which
// updates in real time as the monitor pushes host.status events over WebSocket.
export function DashboardPage() {
  const qc = useQueryClient();
  const { data: version, isError } = useQuery({ queryKey: ["version"], queryFn: getVersion });
  const { data: stats } = useQuery({
    queryKey: ["host-status-stats"],
    queryFn: getHostStatusStats,
    refetchInterval: 30000,
  });

  // Refresh the counts whenever the monitor reports a status change.
  useFleetEvents((e) => {
    if (e.type === "host.status") {
      void qc.invalidateQueries({ queryKey: ["host-status-stats"] });
    }
  });

  const online = stats?.online ?? 0;
  const offline = stats?.offline ?? 0;
  const unknown = stats?.unknown ?? 0;

  const cards: Array<[string, number | string, "success" | "error" | "warning" | "default"]> = [
    ["Hosts Online", online, "success"],
    ["Hosts Offline", offline, "error"],
    ["Status Unknown", unknown, "warning"],
  ];

  return (
    <Box>
      <Typography variant="h5" gutterBottom>
        Fleet Overview
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="text.secondary" variant="overline">
                Backend
              </Typography>
              <Box sx={{ mt: 1 }}>
                {isError ? (
                  <Chip label="unreachable" color="error" size="small" />
                ) : (
                  <Chip
                    label={version ? `connected · ${version.version}` : "connecting…"}
                    color={version ? "success" : "default"}
                    size="small"
                  />
                )}
              </Box>
            </CardContent>
          </Card>
        </Grid>
        {cards.map(([label, value, color]) => (
          <Grid item xs={12} sm={6} md={3} key={label}>
            <Card>
              <CardContent>
                <Typography color="text.secondary" variant="overline">
                  {label}
                </Typography>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 0.5 }}>
                  <Typography variant="h4">{value}</Typography>
                  <Chip size="small" label={color === "success" ? "live" : ""} color={color}
                    sx={{ visibility: color === "success" ? "visible" : "hidden" }} />
                </Box>
              </CardContent>
            </Card>
          </Grid>
        ))}
      </Grid>
    </Box>
  );
}
