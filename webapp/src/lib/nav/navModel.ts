import type { ComponentType } from "react";
import {
  Home,
  ShoppingCart,
  ClipboardList,
  ClipboardCheck,
  FileText,
  FileSignature,
  PackageCheck,
  ReceiptText,
  Store,
  Building2,
  Users,
  ScrollText,
  Settings,
  Activity,
} from "@wso2/oxygen-ui-icons-react";

// ── Sidebar navigation model (Oxygen UI retheme, docs/plans/19) ─────────────
//
// A pure function from the viewer's permission flags to the list of nav groups
// the sidebar renders — the single source of truth for the app's information
// architecture, gated exactly as the retired top-nav `Layout` gated it:
//   - Home, My requests            → everyone
//   - Approvals                    → everyone
//   - Purchase requests            → procurement
//   - Quotations, Contracts        → procurement
//   - GRNs, Invoices               → procurement
//
// Users without a procurement/procurement_admin/admin role (the `procurement`
// flag, false for plain staff and for legal/security/compliance approvers) therefore see
// only Home, My requests, and Approvals.
//   - Vendors                      → canManageVendors (admin / procurement_admin)
//   - Business units, Settings     → canManageBusinessUnits (admin / procurement_admin)
//   - Users                        → admin
//   - Audit log                    → canViewAuditLog (admin / procurement_admin)
//   - Analytics/*                  → canViewAnalytics (admin / procurement_admin)
//
// Master-data / admin destinations are collected under a cosmetic "Admin" group
// header, and the BPM analytics views under an "Analytics" one directly above it;
// each group is appended only when it would contain at least one visible item.

export interface NavPermissions {
  procurement: boolean;
  isAdmin: boolean;
  canManageVendors: boolean;
  canManageBusinessUnits: boolean;
  canViewAuditLog: boolean;
  canViewAnalytics: boolean;
}

export interface NavDestination {
  /** Stable id, also used as the Oxygen `Sidebar.Item` id for active-item matching. */
  id: string;
  to: string;
  label: string;
  icon: ComponentType<{ size?: number }>;
}

export interface NavGroup {
  id: string;
  /** Cosmetic category header. Omitted for the top-level (ungrouped) items. */
  label?: string;
  items: NavDestination[];
}

export function getNavGroups(perms: NavPermissions): NavGroup[] {
  const {
    procurement,
    isAdmin,
    canManageVendors,
    canManageBusinessUnits,
    canViewAuditLog,
    canViewAnalytics,
  } = perms;

  const mainItems: NavDestination[] = [
    { id: "home", to: "/", label: "Home", icon: Home },
    { id: "my-requests", to: "/my-requests", label: "My requests", icon: ShoppingCart },
    { id: "approvals", to: "/approvals", label: "Approvals", icon: ClipboardCheck },
    ...(procurement
      ? [
          { id: "requests", to: "/requests", label: "Purchase requests", icon: ClipboardList },
          { id: "quotations", to: "/quotations", label: "Quotations", icon: FileText },
          { id: "contracts", to: "/contracts", label: "Contracts", icon: FileSignature },
        ]
      : []),
    ...(procurement
      ? [
          { id: "grns", to: "/grns", label: "GRNs", icon: PackageCheck },
          { id: "invoices", to: "/invoices", label: "Invoices", icon: ReceiptText },
        ]
      : []),
  ];

  // Analytics subsections. Purchase requests is the first; more will join it, so
  // this is a group rather than a single top-level item.
  const analyticsItems: NavDestination[] = canViewAnalytics
    ? [
        {
          id: "analytics-prs",
          to: "/analytics/purchase-requests",
          label: "Purchase requests",
          icon: Activity,
        },
      ]
    : [];

  const adminItems: NavDestination[] = [
    ...(canManageVendors ? [{ id: "vendors", to: "/vendors", label: "Vendors", icon: Store }] : []),
    ...(canManageBusinessUnits
      ? [{ id: "business-units", to: "/business-units", label: "Business units", icon: Building2 }]
      : []),
    ...(isAdmin ? [{ id: "users", to: "/users", label: "Users", icon: Users }] : []),
    ...(canViewAuditLog ? [{ id: "audit", to: "/audit", label: "Audit log", icon: ScrollText }] : []),
    ...(canManageBusinessUnits
      ? [{ id: "settings", to: "/settings", label: "Settings", icon: Settings }]
      : []),
  ];

  const groups: NavGroup[] = [{ id: "main", items: mainItems }];
  if (analyticsItems.length > 0) {
    groups.push({ id: "analytics", label: "Analytics", items: analyticsItems });
  }
  if (adminItems.length > 0) {
    groups.push({ id: "admin", label: "Admin", items: adminItems });
  }
  return groups;
}
