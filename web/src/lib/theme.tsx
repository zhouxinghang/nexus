import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

type Theme = "light" | "dark";

type ThemeState = {
  theme: Theme;
  toggle: () => void;
  setTheme: (t: Theme) => void;
};

const ThemeContext = createContext<ThemeState | null>(null);
const STORAGE_KEY = "nexus-theme";

function initialTheme(): Theme {
  if (typeof window === "undefined") return "light";
  const saved = window.localStorage.getItem(STORAGE_KEY);
  if (saved === "light" || saved === "dark") return saved;
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function apply(theme: Theme) {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.style.colorScheme = theme;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(initialTheme);

  useEffect(() => {
    apply(theme);
    window.localStorage.setItem(STORAGE_KEY, theme);
  }, [theme]);

  function setTheme(t: Theme) {
    setThemeState(t);
  }
  function toggle() {
    setThemeState((t) => (t === "dark" ? "light" : "dark"));
  }

  return (
    <ThemeContext.Provider value={{ theme, toggle, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within ThemeProvider");
  return ctx;
}
