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
            <span className="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-indigo-700 text-white shadow-sm">
              <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={1.75}
                strokeLinecap="round"
                strokeLinejoin="round"
                className="h-6 w-6"
                aria-hidden="true"
              >
                <path d="M2.5 3h2.2l1 3m0 0 1.9 8.2A1.6 1.6 0 0 0 10.2 15.5h6.9a1.6 1.6 0 0 0 1.56-1.24l1.34-6.16a.8.8 0 0 0-.78-.98H5.7" />
                <circle cx="9.5" cy="19.5" r="1.3" />
                <circle cx="17" cy="19.5" r="1.3" />
              </svg>
            </span>
            <h1 className="text-xl font-semibold tracking-tight text-slate-900">WSO2 Purchasing App</h1>
            <p className="mt-1 text-sm text-slate-500">Sign in with your company account.</p>
          </div>
          {loginError && (
            <p className="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700">
              {loginError}
            </p>
          )}
          <button onClick={login} className="btn-primary w-full py-2.5">
            Sign in
          </button>
        </div>
        <p className="mt-6 text-center text-xs text-slate-400">
          Internal purchasing management · WSO2
        </p>
      </div>
    </div>
  );
}
