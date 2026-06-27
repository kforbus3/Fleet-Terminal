import {
  AppBar, Box, CssBaseline, Drawer, IconButton, List, ListItemButton,
  ListItemIcon, ListItemText, Toolbar, Typography, Tooltip,
} from "@mui/material";
import DnsIcon from "@mui/icons-material/Dns";
import TerminalIcon from "@mui/icons-material/Terminal";
import DashboardIcon from "@mui/icons-material/Dashboard";
import HistoryIcon from "@mui/icons-material/History";
import ApprovalIcon from "@mui/icons-material/HowToReg";
import GavelIcon from "@mui/icons-material/Gavel";
import PeopleIcon from "@mui/icons-material/People";
import SecurityIcon from "@mui/icons-material/Security";
import GroupWorkIcon from "@mui/icons-material/GroupWork";
import CloudUploadIcon from "@mui/icons-material/CloudUpload";
import VpnKeyIcon from "@mui/icons-material/VpnKey";
import SettingsIcon from "@mui/icons-material/Settings";
import ShieldIcon from "@mui/icons-material/Shield";
import DarkModeIcon from "@mui/icons-material/DarkMode";
import LightModeIcon from "@mui/icons-material/LightMode";
import { Link as RouterLink, Outlet, useLocation } from "react-router-dom";
import { useUIStore } from "../store/ui";

const DRAWER_WIDTH = 232;

const NAV = [
  { to: "/", label: "Dashboard", icon: <DashboardIcon /> },
  { to: "/hosts", label: "Hosts", icon: <DnsIcon /> },
  { to: "/terminals", label: "Terminals", icon: <TerminalIcon /> },
  { to: "/sessions", label: "Session Replay", icon: <HistoryIcon /> },
  { to: "/approvals", label: "Approvals", icon: <ApprovalIcon /> },
  { to: "/audit", label: "Audit", icon: <GavelIcon /> },
  { to: "/users", label: "Users", icon: <PeopleIcon /> },
  { to: "/roles", label: "Roles", icon: <SecurityIcon /> },
  { to: "/groups", label: "Groups", icon: <GroupWorkIcon /> },
  { to: "/enrollment", label: "Enrollment", icon: <CloudUploadIcon /> },
  { to: "/certificates", label: "Certificates", icon: <VpnKeyIcon /> },
  { to: "/security", label: "Security", icon: <ShieldIcon /> },
  { to: "/settings", label: "Settings", icon: <SettingsIcon /> },
];

// Application chrome: top bar + persistent navigation drawer. The routed page
// renders into <Outlet/>.
export function AppLayout() {
  const { pathname } = useLocation();
  const mode = useUIStore((s) => s.mode);
  const toggleMode = useUIStore((s) => s.toggleMode);

  return (
    <Box sx={{ display: "flex" }}>
      <CssBaseline />
      <AppBar position="fixed" sx={{ zIndex: (t) => t.zIndex.drawer + 1 }}>
        <Toolbar variant="dense">
          <TerminalIcon sx={{ mr: 1 }} />
          <Typography variant="h6" sx={{ flexGrow: 1, fontWeight: 600 }}>
            Fleet Terminal
          </Typography>
          <Tooltip title="Toggle theme">
            <IconButton color="inherit" onClick={toggleMode}>
              {mode === "dark" ? <LightModeIcon /> : <DarkModeIcon />}
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>
      <Drawer
        variant="permanent"
        sx={{
          width: DRAWER_WIDTH,
          flexShrink: 0,
          [`& .MuiDrawer-paper`]: { width: DRAWER_WIDTH, boxSizing: "border-box" },
        }}
      >
        <Toolbar variant="dense" />
        <Box sx={{ overflow: "auto" }}>
          <List dense>
            {NAV.map((item) => {
              const selected =
                item.to === "/" ? pathname === "/" : pathname.startsWith(item.to);
              return (
                <ListItemButton
                  key={item.to}
                  component={RouterLink}
                  to={item.to}
                  selected={selected}
                >
                  <ListItemIcon sx={{ minWidth: 36 }}>{item.icon}</ListItemIcon>
                  <ListItemText primary={item.label} />
                </ListItemButton>
              );
            })}
          </List>
        </Box>
      </Drawer>
      <Box component="main" sx={{ flexGrow: 1, p: 3, mt: 6 }}>
        <Outlet />
      </Box>
    </Box>
  );
}
