import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { userManager } from "../auth/userManager";

export function CallbackPage() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const handled = useRef(false);

  useEffect(() => {
    if (handled.current) return; // guard against StrictMode double-mount
    handled.current = true;
    userManager
      .signinRedirectCallback()
      .then(() => navigate("/requests", { replace: true }))
      .catch((e) => setError(e instanceof Error ? e.message : "Sign-in failed"));
  }, [navigate]);

  if (error) {
    return (
      <div className="p-8">
        <p className="text-red-600">Sign-in failed: {error}</p>
        <a className="text-indigo-600 underline" href="/login">
          Back to login
        </a>
      </div>
    );
  }
  return <div className="p-8 text-gray-500">Signing you in…</div>;
}
