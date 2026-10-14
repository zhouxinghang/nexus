import { Component, Suspense, type ReactNode } from "react";
import { useLocation } from "react-router-dom";
import { cn } from "@/lib/api";
import { Button, Spinner } from "@/components/ui";

export function PageLoading({ fullScreen = false }: { fullScreen?: boolean }) {
  return (
    <div
      role="status"
      aria-label="Loading page"
      className={cn(
        "flex min-h-56 items-center justify-center gap-2 text-sm text-muted-foreground",
        fullScreen && "min-h-screen",
      )}
    >
      <Spinner className="motion-reduce:animate-none" />
      Loading page…
    </div>
  );
}

type BoundaryProps = { children: ReactNode; fullScreen: boolean; resetKey: string };

class PageBoundary extends Component<BoundaryProps, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidUpdate(previous: BoundaryProps) {
    if (previous.resetKey !== this.props.resetKey && this.state.failed) {
      this.setState({ failed: false });
    }
  }

  render() {
    if (this.state.failed) {
      return (
        <div
          role="alert"
          className={cn(
            "flex min-h-56 flex-col items-center justify-center gap-3 text-center text-sm",
            this.props.fullScreen && "min-h-screen",
          )}
        >
          <p>Could not load this page. Check your connection and reload.</p>
          <Button variant="outline" onClick={() => window.location.reload()}>Reload page</Button>
        </div>
      );
    }
    return <Suspense fallback={<PageLoading fullScreen={this.props.fullScreen} />}>{this.props.children}</Suspense>;
  }
}

// Keeping this boundary inside Shell leaves navigation available while the
// current tool loads or fails. Changing routes resets errors, not Shell state.
export function PageContent({ children, fullScreen = false }: { children: ReactNode; fullScreen?: boolean }) {
  const location = useLocation();
  return <PageBoundary fullScreen={fullScreen} resetKey={location.pathname}>{children}</PageBoundary>;
}
