import { useState } from "react";
import { useAuth } from "@/lib/auth";
import { Button, Card, Input } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { Hexagon, Lock } from "lucide-react";

export default function Login() {
  const { login } = useAuth();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await login(password);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError("Incorrect password.");
      } else if (err instanceof ApiError && err.status === 429) {
        setError("Too many failed attempts. Try again later.");
      } else {
        setError("Could not sign in. Try again.");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden p-6">
      {/* Soft brand glow backdrop */}
      <div
        aria-hidden
        className="pointer-events-none absolute left-1/2 top-1/3 h-[32rem] w-[32rem] -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary/10 blur-3xl"
      />
      <Card className="w-full max-w-sm animate-scale-in p-7">
        <div className="mb-6 flex flex-col items-center text-center">
          <span className="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-subtle">
            <Hexagon size={24} />
          </span>
          <h1 className="text-xl font-semibold tracking-tight">Welcome to Nexus</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Enter the Owner password to continue.
          </p>
        </div>
        <form onSubmit={onSubmit} className="space-y-3">
          <div className="relative">
            <Lock
              size={15}
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              type="password"
              placeholder="Password"
              className="pl-9"
              value={password}
              autoFocus
              aria-label="Owner password"
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <Button type="submit" className="w-full" loading={busy}>
            {busy ? "Signing in…" : "Sign in"}
          </Button>
        </form>
      </Card>
    </div>
  );
}
