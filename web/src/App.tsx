import { Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "@/lib/auth";
import Login from "@/pages/Login";
import Shell from "@/pages/Shell";
import Files from "@/pages/Files";
import Servers from "@/pages/Servers";
import Tasks from "@/pages/Tasks";
import Journal from "@/pages/Journal";
import Bookmarks from "@/pages/Bookmarks";

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
  if (authenticated) return <Navigate to="/files" replace />;
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
          <Route index element={<Navigate to="/files" replace />} />
          <Route path="/files" element={<Files />} />
          <Route path="/servers" element={<Servers />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/journal" element={<Journal />} />
          <Route path="/bookmarks" element={<Bookmarks />} />
        </Route>
        <Route path="*" element={<Navigate to="/files" replace />} />
      </Routes>
    </AuthProvider>
  );
}
