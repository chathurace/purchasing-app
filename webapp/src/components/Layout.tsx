import { useState } from "react";
import type { MouseEvent } from "react";
import { Link, Outlet, useLocation } from "react-router-dom";
import {
  AppShell,
  Header,
  Sidebar,
  Drawer,
  Box,
  Menu,
  MenuItem,
  useMediaQuery,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChevronDown, LogOut } from "@wso2/oxygen-ui-icons-react";
import { useAuth } from "../auth/AuthContext";
import { useMe } from "../hooks/useMe";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useIsAdmin } from "../hooks/useIsAdmin";
import { useCanManageVendors } from "../hooks/useCanManageVendors";
import { useCanManageBusinessUnits } from "../hooks/useCanManageBusinessUnits";
import { useCanViewAuditLog } from "../hooks/useCanViewAuditLog";
import { getNavGroups } from "../lib/nav/navModel";
import type { NavGroup } from "../lib/nav/navModel";

// ── Application shell (Oxygen UI retheme, docs/plans/19) ────────────────────
//
// Replaces the old top-bar/horizontal-nav identity with Oxygen's side-nav app
// shell: a collapsible left Sidebar, a slim Header (brand + user menu), and a
// content area rendering the routed <Outlet/>. Modelled on finops-app's
// components/shell/Shell.tsx. Login and the auth callback render outside this
// (they are separate top-level routes), matching the previous behaviour.
//
// Desktop vs mobile is hand-tracked (not delegated to Oxygen's useAppShell):
// `desktopCollapsed` toggles the full-width sidebar to a 64px icon rail on
// desktop; `mobileOpen` shows the same nav as a dismissible overlay Drawer on
// narrow viewports (nothing permanently eating content width).
const MOBILE_QUERY = "(max-width:899.95px)"; // MUI's `md` breakpoint (900px)
const SIDEBAR_WIDTH = 250; // Sidebar's own expanded-width default

function resolveActiveId(pathname: string, groups: NavGroup[]): string | undefined {
  const items = groups.flatMap((g) => g.items);
  const exact = items.find((i) => i.to === pathname);
  if (exact) return exact.id;
  // Prefix match for nested routes (e.g. /requests/:id highlights Purchase
  // requests). Excludes the root ('/') so Home doesn't swallow every route.
  const prefixMatches = items
    .filter((i) => i.to !== "/" && pathname.startsWith(`${i.to}/`))
    .sort((a, b) => b.to.length - a.to.length);
  return prefixMatches[0]?.id;
}

function SidebarNav({
  groups,
  collapsed,
  activeId,
  onSelect,
}: {
  groups: NavGroup[];
  collapsed: boolean;
  activeId: string | undefined;
  onSelect: () => void;
}) {
  return (
    <Sidebar collapsed={collapsed} activeItem={activeId} onSelect={onSelect}>
      <Sidebar.Nav>
        {groups.map((group) => (
          <Sidebar.Category key={group.id}>
            {group.label && <Sidebar.CategoryLabel>{group.label}</Sidebar.CategoryLabel>}
            {group.items.map((item) => {
              const Icon = item.icon;
              return (
                <Sidebar.Item key={item.id} id={item.id} link={<Link to={item.to} />}>
                  <Sidebar.ItemIcon>
                    <Icon size={20} />
                  </Sidebar.ItemIcon>
                  <Sidebar.ItemLabel>{item.label}</Sidebar.ItemLabel>
                </Sidebar.Item>
              );
            })}
          </Sidebar.Category>
        ))}
      </Sidebar.Nav>
    </Sidebar>
  );
}

export function Layout() {
  const location = useLocation();
  const procurement = useProcurementAccess();
  const isAdmin = useIsAdmin();
  const canManageVendors = useCanManageVendors();
  const canManageBusinessUnits = useCanManageBusinessUnits();
  const canViewAuditLog = useCanViewAuditLog();
  const isMobile = useMediaQuery(MOBILE_QUERY);
  const [desktopCollapsed, setDesktopCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  const groups = getNavGroups({
    procurement,
    isAdmin,
    canManageVendors,
    canManageBusinessUnits,
    canViewAuditLog,
  });
  const activeId = resolveActiveId(location.pathname, groups);
  const closeMobileDrawer = () => setMobileOpen(false);

  return (
    <>
      <AppShell>
        <AppShell.Navbar>
          <Header>
            <Header.Toggle
              collapsed={isMobile ? !mobileOpen : desktopCollapsed}
              onToggle={() =>
                isMobile
                  ? setMobileOpen((open) => !open)
                  : setDesktopCollapsed((collapsed) => !collapsed)
              }
            />
            <Header.Brand>
              <Link
                to="/"
                style={{ display: "flex", alignItems: "center", textDecoration: "none", color: "inherit" }}
              >
                <Header.BrandTitle>Purchasing</Header.BrandTitle>
              </Link>
            </Header.Brand>
            <Header.Spacer />
            <Header.Actions>
              <UserMenu />
            </Header.Actions>
          </Header>
        </AppShell.Navbar>

        {!isMobile && (
          <AppShell.Sidebar>
            <SidebarNav groups={groups} collapsed={desktopCollapsed} activeId={activeId} onSelect={() => {}} />
          </AppShell.Sidebar>
        )}

        <AppShell.Main>
          {/* AppShell wraps Main's content in a row-flex Box internally; a
              flex-row item doesn't stretch horizontally by default, so this
              full-width wrapper keeps page content filling the content area.
              A little left padding keeps page content from butting against the
              sidebar/nav edge. */}
          <Box sx={{ width: "100%", pl: { xs: 2, md: 3 } }}>
            <Outlet />
          </Box>
        </AppShell.Main>
      </AppShell>

      {isMobile && (
        <Drawer
          anchor="left"
          variant="temporary"
          open={mobileOpen}
          onClose={closeMobileDrawer}
          keepMounted
          slotProps={{ paper: { sx: { width: SIDEBAR_WIDTH } } }}
        >
          <SidebarNav groups={groups} collapsed={false} activeId={activeId} onSelect={closeMobileDrawer} />
        </Drawer>
      )}
    </>
  );
}

// UserMenu — a compact avatar + email trigger in the Header that opens a Menu
// with a single Sign out action. Modelled on finops-app's UserMenu.
function UserMenu() {
  const { logout } = useAuth();
  const { data: me } = useMe();
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const open = Boolean(anchorEl);
  const theme = useTheme();

  const email = me?.email ?? "";
  const initial = (email.trim()[0] ?? "?").toUpperCase();

  const handleOpen = (e: MouseEvent<HTMLElement>) => setAnchorEl(e.currentTarget);
  const handleClose = () => setAnchorEl(null);
  const handleSignOut = () => {
    handleClose();
    logout();
  };

  return (
    <Box sx={{ display: "flex", alignItems: "center", ml: "auto", gap: "4px" }}>
      <Box
        component="button"
        type="button"
        onClick={handleOpen}
        aria-haspopup="true"
        aria-expanded={open}
        aria-controls={open ? "user-menu" : undefined}
        aria-label="Account menu"
        sx={{
          display: "flex",
          alignItems: "center",
          gap: "6px",
          padding: "4px 8px 4px 4px",
          borderRadius: "20px",
          bgcolor: "action.hover",
          border: "none",
          cursor: "pointer",
          font: "inherit",
        }}
      >
        <Box
          sx={{
            width: "24px",
            height: "24px",
            borderRadius: "50%",
            bgcolor: "primary.main",
            color: "common.white",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontSize: "11px",
            fontWeight: 700,
            flexShrink: 0,
          }}
        >
          {initial}
        </Box>
        <Box
          component="span"
          sx={{
            fontSize: "13px",
            fontWeight: 500,
            color: "text.primary",
            maxWidth: "180px",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {email}
        </Box>
        <ChevronDown size={18} color={theme.palette.text.secondary} />
      </Box>
      <Menu id="user-menu" anchorEl={anchorEl} open={open} onClose={handleClose}>
        <MenuItem onClick={handleSignOut}>
          <LogOut size={16} style={{ marginRight: 8 }} />
          Sign out
        </MenuItem>
      </Menu>
    </Box>
  );
}
