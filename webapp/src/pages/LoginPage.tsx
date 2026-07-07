import { Navigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

export function LoginPage() {
  const { user, loading, loginError, login } = useAuth();
  if (loading) return <div className="p-8 text-slate-500">Loading…</div>;
  if (user) return <Navigate to="/requests" replace />;

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="app-card animate-fade-in p-8 shadow-card">
          <div className="mb-6 flex flex-col items-center text-center">
            <span className="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-indigo-700 text-lg font-bold text-white shadow-sm">
              P
            </span>
            <h1 className="text-xl font-semibold tracking-tight text-slate-900">Purchasing</h1>
            <p className="mt-1 text-sm text-slate-500">Sign in with your company account.</p>
          </div>
          {loginError && (
            <p className="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700">
              {loginError}
            </p>
          )}
          <button onClick={login} className="btn-primary w-full py-2.5">
            Sign in with SSO
          </button>
        </div>
        <p className="mt-6 text-center text-xs text-slate-400">
          Internal purchasing management · WSO2
        </p>
      </div>
    </div>
  );
}
