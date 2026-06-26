import { Box, Card, CardContent, Grid, Typography, Chip } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { getVersion } from "../api/client";

// Landing dashboard. In M0 it confirms backend connectivity; later milestones
// replace the cards with live fleet metrics (hosts online, active sessions, etc).
export function DashboardPage() {
  const { data, isError } = useQuery({ queryKey: ["version"], queryFn: getVersion });

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
                    label={data ? `connected · ${data.version}` : "connecting…"}
                    color={data ? "success" : "default"}
                    size="small"
                  />
                )}
              </Box>
            </CardContent>
          </Card>
        </Grid>
        {[
          ["Hosts Online", "—"],
          ["Active Sessions", "—"],
          ["Pending Approvals", "—"],
        ].map(([label, value]) => (
          <Grid item xs={12} sm={6} md={3} key={label}>
            <Card>
              <CardContent>
                <Typography color="text.secondary" variant="overline">
                  {label}
                </Typography>
                <Typography variant="h4">{value}</Typography>
              </CardContent>
            </Card>
          </Grid>
        ))}
      </Grid>
    </Box>
  );
}
