import { useEffect, useRef, useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useMe } from "../hooks/useMe";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useIsAdmin } from "../hooks/useIsAdmin";
import { useIsApprover } from "../hooks/useIsApprover";
import { useCanManageVendors } from "../hooks/useCanManageVendors";
import { useCanManageBudgetUnits } from "../hooks/useCanManageBudgetUnits";

const linkCls = ({ isActive }: { isActive: boolean }) =>
  `rounded-lg px-3 py-1.5 text-sm font-medium transition ${
    isActive
      ? "bg-indigo-50 text-indigo-700 shadow-sm ring-1 ring-inset ring-indigo-100"
      : "text-slate-600 hover:bg-slate-100 hover:text-slate-900"
  }`;

function initials(email?: string): string {
  if (!email) return "?";
  const name = email.split("@")[0];
  const parts = name.split(/[.\-_]/).filter(Boolean);
  const letters = (parts.length >= 2 ? parts[0][0] + parts[1][0] : name.slice(0, 2)) || "?";
  return letters.toUpperCase();
}

export function Layout() {
  const { logout } = useAuth();
  const { data: me } = useMe();
  const procurement = useProcurementAccess();
  const isAdmin = useIsAdmin();
  const isApprover = useIsApprover();
  const canManageVendors = useCanManageVendors();
  const canManageBudgetUnits = useCanManageBudgetUnits();
  // Requests and Approvals are visible to everyone (staff-only users included);
  // the Approvals page just lists whatever is awaiting the caller. Approvers
  // (named approvers, or budget/legal/security card actors) and procurement
  // additionally see the Quotations and Contracts tabs, with a read-only,
  // PR-scoped view of the latter two.
  const canApprove = procurement || isApprover;

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-slate-200/70 bg-white/80 backdrop-blur-md">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-2.5">
          <div className="flex min-w-0 items-center gap-6">
            <span className="flex items-center gap-2.5">
              <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-indigo-500 to-indigo-700 text-sm font-bold text-white shadow-sm">
                P
              </span>
              <span className="text-[15px] font-semibold tracking-tight text-slate-900">
                Purchasing
              </span>
            </span>
            <nav className="flex items-center gap-1">
              <NavLink to="/requests" className={linkCls}>
                Requests
              </NavLink>
              <NavLink to="/approvals" className={linkCls}>
                Approvals
              </NavLink>
              {canApprove && (
                <NavLink to="/quotations" className={linkCls}>
                  Quotations
                </NavLink>
              )}
              {canApprove && (
                <NavLink to="/contracts" className={linkCls}>
                  Contracts
                </NavLink>
              )}
              {procurement && (
                <>
                  <NavLink to="/grns" className={linkCls}>
                    GRNs
                  </NavLink>
                  <NavLink to="/invoices" className={linkCls}>
                    Invoices
                  </NavLink>
                </>
              )}
              {canManageVendors && (
                <NavLink to="/vendors" className={linkCls}>
                  Vendors
                </NavLink>
              )}
              {canManageBudgetUnits && (
                <NavLink to="/budget-units" className={linkCls}>
                  Budget units
                </NavLink>
              )}
              {isAdmin && (
                <NavLink to="/users" className={linkCls}>
                  Users
                </NavLink>
              )}
              {canManageBudgetUnits && (
                <NavLink to="/settings" className={linkCls}>
                  Settings
                </NavLink>
              )}
            </nav>
          </div>
          <div className="flex shrink-0 items-center">
            {me && <UserMenu email={me.email} onSignOut={logout} />}
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-8">
        <Outlet />
      </main>
    </div>
  );
}

// UserMenu is the compact account control in the top bar: a user-icon button
// that opens a dropdown with the signed-in email and a sign-out action.
function UserMenu({ email, onSignOut }: { email: string; onSignOut: () => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDocClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDocClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDocClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="Account menu"
        onClick={() => setOpen((v) => !v)}
        className={`flex h-9 w-9 items-center justify-center rounded-full text-xs font-semibold ring-1 ring-inset transition ${
          open
            ? "bg-indigo-600 text-white ring-indigo-600"
            : "bg-indigo-100 text-indigo-700 ring-indigo-200 hover:bg-indigo-200"
        }`}
      >
        {initials(email)}
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 z-40 mt-2 w-64 origin-top-right animate-fade-in overflow-hidden rounded-xl border border-slate-200 bg-white shadow-card"
        >
          <div className="flex items-center gap-3 border-b border-slate-100 px-4 py-3">
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-xs font-semibold text-indigo-700 ring-1 ring-inset ring-indigo-200">
              {initials(email)}
            </span>
            <div className="min-w-0">
              <p className="text-xs text-slate-400">Signed in as</p>
              <p className="truncate text-sm font-medium text-slate-900" title={email}>
                {email}
              </p>
            </div>
          </div>
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onSignOut();
            }}
            className="flex w-full items-center gap-2.5 px-4 py-2.5 text-left text-sm font-medium text-slate-700 transition hover:bg-slate-50"
          >
            <svg
              className="h-4 w-4 text-slate-400"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden
            >
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
              <path d="M16 17l5-5-5-5M21 12H9" />
            </svg>
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}
