import { Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "@/lib/auth";
import Login from "@/pages/Login";
import Shell from "@/pages/Shell";
import Files from "@/pages/Files";
import Servers from "@/pages/Servers";
import Tasks from "@/pages/Tasks";
import Journal from "@/pages/Journal";
import Bookmarks from "@/pages/Bookmarks";
import Overview from "@/pages/Overview";

function Protected({ children }: { children: React.ReactNode }) {
  const { authenticated, loading } = useAuth();
  if (loading) {
    return <div className="flex min-h-screen items-center justify-center text-sm">Loading…</div>;
  }
  if (!authenticated) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function LoginRoute() {
  const { authenticated, loading } = useAuth();
  if (loading) return null;
  if (authenticated) return <Navigate to="/" replace />;
  return <Login />;
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginRoute />} />
        <Route
          element={
            <Protected>
              <Shell />
            </Protected>
          }
        >
          <Route index element={<Overview />} />
          <Route path="/files" element={<Files />} />
          <Route path="/servers" element={<Servers />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/journal" element={<Journal />} />
          <Route path="/bookmarks" element={<Bookmarks />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AuthProvider>
  );
}
