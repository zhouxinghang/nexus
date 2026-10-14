import { lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "@/lib/auth";
import { PageContent, PageLoading } from "@/components/PageContent";
import Login from "@/pages/Login";

const Shell = lazy(() => import("@/pages/Shell"));
const Files = lazy(() => import("@/pages/Files"));
const Servers = lazy(() => import("@/pages/Servers"));
const Tasks = lazy(() => import("@/pages/Tasks"));
const Journal = lazy(() => import("@/pages/Journal"));
const Bookmarks = lazy(() => import("@/pages/Bookmarks"));
const Overview = lazy(() => import("@/pages/Overview"));

function Protected({ children }: { children: React.ReactNode }) {
  const { authenticated, loading } = useAuth();
  if (loading) {
    return <PageLoading fullScreen />;
  }
  if (!authenticated) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function LoginRoute() {
  const { authenticated, loading } = useAuth();
  if (loading) return <PageLoading fullScreen />;
  if (authenticated) return <Navigate to="/" replace />;
  return <Login />;
}

export default function App() {
  return (
    <AuthProvider>
      <PageContent fullScreen>
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
      </PageContent>
    </AuthProvider>
  );
}
