import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui";
import { FileText, Server, CheckSquare, BookText, LogOut } from "lucide-react";
import { cn } from "@/lib/api";

const nav = [
  { to: "/files", label: "Files", icon: FileText },
  { to: "/servers", label: "Servers", icon: Server },
  { to: "/tasks", label: "Tasks", icon: CheckSquare },
  { to: "/journal", label: "Journal", icon: BookText },
];

export default function Shell() {
  const { logout } = useAuth();
  const navigate = useNavigate();

  async function onLogout() {
    await logout();
    navigate("/login");
  }

  return (
    <div className="flex min-h-screen">
      <aside className="flex w-56 flex-col border-r border-border bg-card p-4">
        <div className="mb-6 px-2 text-lg font-semibold">Nexus</div>
        <nav className="flex-1 space-y-1">
          {nav.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition",
                  isActive ? "bg-primary text-primary-foreground" : "hover:bg-muted",
                )
              }
            >
              <Icon size={16} />
              {label}
            </NavLink>
          ))}
        </nav>
        <Button variant="outline" className="mt-4" onClick={onLogout}>
          <LogOut size={16} /> Log out
        </Button>
      </aside>
      <main className="flex-1 overflow-auto p-8">
        <Outlet />
      </main>
    </div>
  );
}
