import { useEffect, useState } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "@/lib/auth";
import { useTheme } from "@/lib/theme";
import { Button } from "@/components/ui";
import {
  FileText,
  Server,
  CheckSquare,
  BookText,
  LogOut,
  Hexagon,
  Moon,
  Sun,
  Menu,
  X,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import { cn } from "@/lib/api";

const nav = [
  { to: "/files", label: "Files", icon: FileText, hint: "Resources" },
  { to: "/servers", label: "Servers", icon: Server, hint: "Host vitals" },
  { to: "/tasks", label: "Tasks", icon: CheckSquare, hint: "Board" },
  { to: "/journal", label: "Journal", icon: BookText, hint: "Log" },
];

function ThemeToggle() {
  const { theme, toggle } = useTheme();
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={toggle}
      aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
      title={theme === "dark" ? "Light mode" : "Dark mode"}
    >
      {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
    </Button>
  );
}

function SidebarContent({
  collapsed = false,
  onToggleCollapse,
  onClose,
}: {
  collapsed?: boolean;
  onToggleCollapse?: () => void;
  onClose?: () => void;
}) {
  const { logout } = useAuth();
  const navigate = useNavigate();

  async function onLogout() {
    await logout();
    navigate("/login");
  }

  return (
    <div className="flex h-full flex-col p-4">
      <div
        className={cn(
          "mb-6 flex items-center px-1",
          collapsed ? "flex-col gap-3" : "justify-between",
        )}
      >
        <div className="flex items-center gap-2">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Hexagon size={18} />
          </span>
          {!collapsed && (
            <span className="text-lg font-semibold tracking-tight">Nexus</span>
          )}
        </div>
        <div className={cn("flex items-center gap-1", collapsed && "flex-col")}>
          <ThemeToggle />
          {onToggleCollapse && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onToggleCollapse}
              aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              className="hidden md:inline-flex"
            >
              {collapsed ? (
                <PanelLeftOpen size={16} />
              ) : (
                <PanelLeftClose size={16} />
              )}
            </Button>
          )}
          {onClose && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onClose}
              aria-label="Close navigation"
              className="md:hidden"
            >
              <X size={16} />
            </Button>
          )}
        </div>
      </div>

      {!collapsed && (
        <p className="mb-2 px-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          Workbench
        </p>
      )}
      <nav className="flex-1 space-y-1">
        {nav.map(({ to, label, icon: Icon, hint }) => (
          <NavLink
            key={to}
            to={to}
            title={collapsed ? label : undefined}
            className={({ isActive }) =>
              cn(
                "group relative flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                collapsed && "justify-center px-0",
                isActive
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground",
              )
            }
          >
            {({ isActive }) => (
              <>
                <span
                  className={cn(
                    "absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full bg-primary transition-opacity",
                    isActive ? "opacity-100" : "opacity-0",
                  )}
                />
                <Icon size={17} className="shrink-0" />
                {!collapsed && (
                  <>
                    <span className="flex-1">{label}</span>
                    <span className="text-[11px] text-muted-foreground/70">
                      {hint}
                    </span>
                  </>
                )}
              </>
            )}
          </NavLink>
        ))}
      </nav>

      <div className="mt-4 border-t border-border pt-4">
        {collapsed ? (
          <div className="flex flex-col items-center gap-3">
            <span
              className="flex h-8 w-8 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground"
              title="Owner — single-user session"
            >
              O
            </span>
            <Button
              variant="outline"
              size="icon-sm"
              onClick={onLogout}
              aria-label="Log out"
              title="Log out"
            >
              <LogOut size={15} />
            </Button>
          </div>
        ) : (
          <>
            <div className="mb-3 flex items-center gap-2 px-2">
              <span className="flex h-8 w-8 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
                O
              </span>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">Owner</p>
                <p className="truncate text-xs text-muted-foreground">
                  Single-user session
                </p>
              </div>
            </div>
            <Button variant="outline" className="w-full" onClick={onLogout}>
              <LogOut size={15} /> Log out
            </Button>
          </>
        )}
      </div>
    </div>
  );
}

const COLLAPSE_KEY = "nexus-sidebar-collapsed";

export default function Shell() {
  const [open, setOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") return false;
    return window.localStorage.getItem(COLLAPSE_KEY) === "1";
  });
  const location = useLocation();

  // Close the mobile drawer whenever the route changes.
  useEffect(() => {
    setOpen(false);
  }, [location.pathname]);

  function toggleCollapsed() {
    setCollapsed((c) => {
      const next = !c;
      window.localStorage.setItem(COLLAPSE_KEY, next ? "1" : "0");
      return next;
    });
  }

  return (
    <div className="flex min-h-screen bg-background">
      {/* Desktop sidebar */}
      <aside
        className={cn(
          "sticky top-0 hidden h-screen shrink-0 border-r border-border bg-card transition-[width] duration-200 md:block",
          collapsed ? "w-16" : "w-60",
        )}
      >
        <SidebarContent
          collapsed={collapsed}
          onToggleCollapse={toggleCollapsed}
        />
      </aside>

      {/* Mobile drawer */}
      {open && (
        <div className="fixed inset-0 z-40 md:hidden">
          <div
            className="absolute inset-0 bg-foreground/40 backdrop-blur-sm"
            onClick={() => setOpen(false)}
          />
          <aside className="absolute left-0 top-0 h-full w-64 animate-scale-in border-r border-border bg-card shadow-pop">
            <SidebarContent onClose={() => setOpen(false)} />
          </aside>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        {/* Mobile top bar */}
        <header className="sticky top-0 z-30 flex items-center gap-3 border-b border-border bg-card/80 px-4 py-3 backdrop-blur md:hidden">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setOpen(true)}
            aria-label="Open navigation"
          >
            <Menu size={18} />
          </Button>
          <div className="flex items-center gap-2">
            <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
              <Hexagon size={15} />
            </span>
            <span className="font-semibold">Nexus</span>
          </div>
          <div className="ml-auto">
            <ThemeToggle />
          </div>
        </header>

        <main className="flex-1 overflow-auto">
          <div className="mx-auto max-w-6xl animate-fade-in p-5 sm:p-8">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
