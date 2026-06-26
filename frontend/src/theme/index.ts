import { createTheme, type Theme } from "@mui/material/styles";

// Two themes mirror the dark/light requirement; the active mode is persisted
// by the UI store. Colors aim for an operations-console aesthetic.
export function buildTheme(mode: "light" | "dark"): Theme {
  return createTheme({
    palette: {
      mode,
      primary: { main: mode === "dark" ? "#5b9dff" : "#1565c0" },
      secondary: { main: "#26a69a" },
      background:
        mode === "dark"
          ? { default: "#0d1117", paper: "#161b22" }
          : { default: "#f5f7fa", paper: "#ffffff" },
    },
    shape: { borderRadius: 8 },
    typography: {
      fontFamily: "Roboto, system-ui, sans-serif",
      fontSize: 13,
    },
    components: {
      MuiButton: { defaultProps: { disableElevation: true } },
    },
  });
}
