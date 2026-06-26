import { Box, Paper, Typography } from "@mui/material";

// Temporary page shown for routes whose feature is delivered in a later
// milestone. Keeps navigation coherent while the app is built out.
export function PlaceholderPage({ title }: { title: string }) {
  return (
    <Box>
      <Typography variant="h5" gutterBottom>
        {title}
      </Typography>
      <Paper variant="outlined" sx={{ p: 4 }}>
        <Typography color="text.secondary">
          This module is under construction.
        </Typography>
      </Paper>
    </Box>
  );
}
