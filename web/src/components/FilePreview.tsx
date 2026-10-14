import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { AlertTriangle, Check, ChevronLeft, ChevronRight, Download, Eye, FileAudio, FileQuestion, FileText, Maximize2, Minimize2, Pencil, RefreshCw, Save, WrapText, X, ZoomIn, ZoomOut } from "lucide-react";
import { api, ApiError, cn } from "@/lib/api";
import { Button, Badge, Spinner } from "@/components/ui";
import { useToast } from "@/lib/toast";

const FileTextEditor = lazy(() => import("./FileTextEditor"));

export type PreviewResource = {
  id: number;
  name: string;
  path: string;
  size: number;
  is_dir: boolean;
  state: "present" | "missing";
};
type Preview = {
  type: "text" | "image" | "video" | "audio" | "pdf" | "unsupported";
  mime: string;
  size: number;
  modified_at: string;
  editable: boolean;
  reason?: string;
};
type TextContent = { content: string; revision: string; encoding: string };

function formatSize(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export default function FilePreview({
  resource,
  siblings,
  onNavigate,
  onClose,
  onSaved,
}: {
  resource: PreviewResource;
  siblings: PreviewResource[];
  onNavigate: (resource: PreviewResource) => void;
  onClose: () => void;
  onSaved: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [text, setText] = useState<TextContent | null>(null);
  const [editorInitial, setEditorInitial] = useState("");
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [reload, setReload] = useState(0);
  const [editorVersion, setEditorVersion] = useState(0);
  const [wrap, setWrap] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [zoom, setZoom] = useState(1);
  const imageContainer = useRef<HTMLDivElement>(null);
  const [imageSize, setImageSize] = useState({ width: 0, height: 0 });
  const [imageViewport, setImageViewport] = useState({ width: 0, height: 0 });
  const [cursor, setCursor] = useState({ line: 1, column: 1 });
  const toast = useToast();
  const dirty = text !== null && draft !== text.content;
  const candidates = siblings.filter((r) => !r.is_dir && r.state === "present");
  const index = candidates.findIndex((r) => r.id === resource.id);
  const base = `/api/files/resources/${resource.id}`;

  useEffect(() => {
    const element = dialog.current!;
    const trigger = document.activeElement as HTMLElement | null;
    element.showModal();
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      element.close();
      document.body.style.overflow = previous;
      if (trigger?.isConnected) trigger.focus();
    };
  }, []);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setPreview(null);
    setText(null);
    setEditing(false);
    setError("");
    setConflict(false);
    setZoom(1);
    setImageSize({ width: 0, height: 0 });
    setCursor({ line: 1, column: 1 });
    async function load() {
      try {
        const p = await api.get<Preview>(`${base}/preview`);
        if (!active) return;
        setPreview(p);
        if (p.type === "text") {
          const content = await api.get<TextContent>(`${base}/content`);
          if (!active) return;
          setText(content);
          setDraft(content.content);
          setEditorInitial(content.content.replace(/^\uFEFF/, ""));
        }
      } catch (err) {
        if (active) setError((err as Error).message);
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    return () => { active = false; };
  }, [base, reload]);

  useEffect(() => {
    const element = imageContainer.current;
    if (!element) return;
    const observer = new ResizeObserver(() => {
      // The image surface has 24px padding on each edge.
      setImageViewport({ width: Math.max(0, element.clientWidth - 48), height: Math.max(0, element.clientHeight - 48) });
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [preview?.type, loading]);

  useEffect(() => {
    if (!dirty) return;
    const beforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => window.removeEventListener("beforeunload", beforeUnload);
  }, [dirty]);

  function canLeave() {
    if (savingRef.current) return false;
    return !dirty || window.confirm("Discard unsaved changes to this file?");
  }
  function close() {
    if (canLeave()) onClose();
  }
  function navigate(offset: number) {
    const next = candidates[index + offset];
    if (next && canLeave()) onNavigate(next);
  }
  function reloadFile() {
    if (canLeave()) setReload((n) => n + 1);
  }
  async function save() {
    if (!text || !editing || !dirty || savingRef.current || conflict) return;
    savingRef.current = true;
    setSaving(true);
    setError("");
    try {
      const updated = await api.put<TextContent>(`${base}/content`, { content: draft, revision: text.revision });
      // The editor is read-only during the write, so its in-flight snapshot
      // cannot hide keystrokes made while waiting for the server.
      setText(updated);
      setDraft(updated.content);
      setPreview((p) => p ? { ...p, size: new TextEncoder().encode(updated.content).byteLength } : p);
      toast.success("File saved");
      onSaved();
    } catch (err) {
      const changed = err instanceof ApiError && err.status === 409;
      setConflict(changed);
      setError(changed ? "This file changed on disk. Your edits are still here. Download your draft, then reload the latest version before saving." : (err as Error).message);
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }
  function downloadDraft() {
    const url = URL.createObjectURL(new Blob([draft], { type: "text/plain;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = resource.name;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  const onCursor = useCallback((line: number, column: number) => setCursor({ line, column }), []);
  // Keep a UTF-8 BOM outside the editable document; round-trip it on saving.
  const bom = text?.content.startsWith("\uFEFF") ? "\uFEFF" : "";
  const textBody = text?.content.slice(bom.length) ?? "";
  const lineEnding = textBody.includes("\r\n")
    ? (/(?<!\r)\n/.test(textBody) || /\r(?!\n)/.test(textBody) ? "Mixed → LF" : "CRLF")
    : textBody.includes("\r") ? (textBody.includes("\n") ? "Mixed → LF" : "CR") : "LF";
  const toolbarButton = "h-9 px-3";
  const fit = imageSize.width > 0 && imageSize.height > 0
    ? Math.min(imageViewport.width / imageSize.width, imageViewport.height / imageSize.height)
    : 0;
  const imageWidth = imageSize.width * fit * zoom;
  const imageHeight = imageSize.height * fit * zoom;

  return createPortal(
    <dialog
      ref={dialog}
      aria-labelledby="file-preview-title"
      aria-describedby="file-preview-path"
      className={cn(
        "fixed m-auto flex h-[90dvh] w-[calc(100%-2rem)] max-w-6xl flex-col overflow-hidden rounded-xl border border-border bg-card p-0 text-foreground shadow-pop backdrop:bg-foreground/50 backdrop:backdrop-blur-sm",
        fullscreen && "h-dvh max-h-none w-screen max-w-none rounded-none",
      )}
      onCancel={(e) => { e.preventDefault(); close(); }}
      onClick={(e) => {
        if (e.target !== e.currentTarget) return;
        const box = e.currentTarget.getBoundingClientRect();
        if (e.clientX < box.left || e.clientX > box.right || e.clientY < box.top || e.clientY > box.bottom) close();
      }}
      onKeyDownCapture={(e) => {
        if (e.key === "Tab") {
          const focusable = [...e.currentTarget.querySelectorAll<HTMLElement>(
            'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), iframe, [tabindex]:not([tabindex="-1"]), [contenteditable="true"]',
          )].filter((element) => element.getClientRects().length > 0);
          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          if ((!e.shiftKey && document.activeElement === last) || (e.shiftKey && document.activeElement === first)) {
            e.preventDefault();
            (e.shiftKey ? last : first)?.focus();
          }
        }
        if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === "s") {
          e.preventDefault();
          e.stopPropagation();
          void save();
        }
      }}
    >
      <header className="flex shrink-0 items-center gap-3 border-b border-border px-4 py-3">
        <span className="hidden h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-accent text-accent-foreground sm:flex">
          <FileText size={20} aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h2 id="file-preview-title" className="truncate text-base font-semibold">{resource.name}</h2>
            {dirty && <Badge variant="warning" className="text-foreground">Unsaved</Badge>}
          </div>
          <p id="file-preview-path" className="truncate font-mono text-xs text-muted-foreground" title={resource.path}>{resource.path}</p>
        </div>
        <Button size="icon" variant="ghost" aria-label={fullscreen ? "Exit fullscreen" : "Fullscreen preview"} title={fullscreen ? "Exit fullscreen" : "Fullscreen"} onClick={() => setFullscreen((v) => !v)}>
          {fullscreen ? <Minimize2 size={18} /> : <Maximize2 size={18} />}
        </Button>
        <Button size="icon" variant="ghost" aria-label="Close preview" title="Close (Esc)" disabled={saving} onClick={close}><X size={20} /></Button>
      </header>

      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border bg-muted/30 px-4 py-2">
        <div className="mr-auto flex items-center gap-1 rounded-md border border-border bg-card p-0.5">
          <Button variant={!editing ? "subtle" : "ghost"} size="sm" aria-pressed={!editing} disabled={saving} onClick={() => {
            if (!editing || canLeave()) {
              if (text) setDraft(text.content);
              setEditorInitial(textBody);
              setEditorVersion((v) => v + 1);
              setEditing(false);
              if (conflict) setReload((v) => v + 1);
              else setError("");
            }
          }}><Eye size={14} aria-hidden /> Preview</Button>
          {preview?.editable && text && (
            <Button variant={editing ? "subtle" : "ghost"} size="sm" aria-pressed={editing} disabled={loading || saving} onClick={() => setEditing(true)}><Pencil size={14} aria-hidden /> Edit</Button>
          )}
        </div>
        {preview?.type === "text" && (
          <Button variant="ghost" className={toolbarButton} aria-label="Toggle line wrapping" aria-pressed={wrap} onClick={() => setWrap((v) => !v)} title="Toggle line wrapping"><WrapText size={16} aria-hidden /><span className="hidden sm:inline">Wrap</span></Button>
        )}
        {preview?.type === "image" && (
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="icon" aria-label="Zoom out" disabled={zoom <= 0.25} onClick={() => setZoom((v) => Math.max(0.25, v - 0.25))}><ZoomOut size={16} /></Button>
            <Button variant="ghost" size="sm" title="Fit image" onClick={() => setZoom(1)}>{Math.round(zoom * 100)}%</Button>
            <Button variant="ghost" size="icon" aria-label="Zoom in" disabled={zoom >= 3} onClick={() => setZoom((v) => Math.min(3, v + 0.25))}><ZoomIn size={16} /></Button>
          </div>
        )}
        <Button variant="ghost" size="icon" aria-label="Reload file" title="Reload from disk" disabled={saving || loading} onClick={reloadFile}><RefreshCw size={16} /></Button>
        <a href={`${base}/download`} download={resource.name} className="inline-flex h-9 items-center gap-2 rounded-md px-3 text-sm font-medium transition-colors hover:bg-muted" aria-label={`Download ${resource.name}`}><Download size={16} aria-hidden /><span className="hidden sm:inline">Download</span></a>
        {editing && (
          <Button className={cn(toolbarButton, "bg-accent-foreground text-primary-foreground hover:bg-accent-foreground/90")} loading={saving} disabled={!dirty || conflict} title="Save (Ctrl/Cmd+S)" onClick={() => void save()}><Save size={15} aria-hidden /> Save</Button>
        )}
      </div>

      {error && (
        <div role="alert" className="flex shrink-0 flex-wrap items-center gap-3 border-b border-destructive/30 bg-destructive/5 px-4 py-3 text-sm">
          <AlertTriangle size={18} className="shrink-0 text-destructive" aria-hidden />
          <p className="min-w-0 flex-1">{error}</p>
          {text && dirty && <Button variant="outline" size="sm" onClick={downloadDraft}><Download size={14} aria-hidden /> Download draft</Button>}
          <Button variant="outline" size="sm" disabled={saving || loading} onClick={reloadFile}>Reload</Button>
        </div>
      )}

      <main className="min-h-0 flex-1 overflow-hidden bg-background" aria-busy={loading}>
        {loading ? (
          <div role="status" className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground"><Spinner /> Loading preview…</div>
        ) : preview?.type === "text" && text ? (
          <Suspense fallback={<div role="status" className="flex h-full items-center justify-center gap-2"><Spinner /> Loading editor…</div>}>
            <FileTextEditor
              key={`${resource.id}-${reload}-${editorVersion}`}
              initialContent={editorInitial}
              name={resource.name}
              editable={editing && !saving}
              wrap={wrap}
              onChange={(content) => {
                // CodeMirror normalizes its internal document to LF. Convert
                // the outgoing draft explicitly, including text pasted with
                // different newlines; do not rely on DOM input behavior.
                const normalized = content.replace(/\r\n|\r/g, "\n");
                setDraft(bom + (lineEnding === "CRLF" ? normalized.replace(/\n/g, "\r\n") : lineEnding === "CR" ? normalized.replace(/\n/g, "\r") : normalized));
              }}
              onCursor={onCursor}
            />
          </Suspense>
        ) : preview?.type === "image" ? (
          <div ref={imageContainer} className="h-full overflow-auto p-6">
            <div className="flex items-center justify-center" style={{ minWidth: "100%", width: imageWidth, minHeight: "100%", height: Math.max(imageViewport.height, imageHeight) }}>
              <img key={`${base}-${reload}`} src={`${base}/content?reload=${reload}`} alt={resource.name} className="block shrink-0 object-contain" style={fit > 0 ? { width: imageWidth, height: imageHeight, maxWidth: "none" } : { maxWidth: "100%" }} onLoad={(e) => setImageSize({ width: e.currentTarget.naturalWidth, height: e.currentTarget.naturalHeight })} onError={() => setError("The image could not be displayed. Try downloading it or reloading.")} />
            </div>
          </div>
        ) : preview?.type === "video" ? (
          <div className="flex h-full items-center justify-center bg-muted/40 p-4">
            <video key={`${base}-${reload}`} controls preload="metadata" className="max-h-full w-full" aria-label={resource.name} src={`${base}/content?reload=${reload}`} onError={() => setError("This video format is not supported by your browser. Download it to play locally.")} />
          </div>
        ) : preview?.type === "audio" ? (
          <div className="flex h-full flex-col items-center justify-center gap-6 p-6">
            <span className="flex h-24 w-24 items-center justify-center rounded-2xl bg-accent text-accent-foreground"><FileAudio size={40} aria-hidden /></span>
            <p className="max-w-full truncate font-medium">{resource.name}</p>
            <audio key={`${base}-${reload}`} controls preload="metadata" className="w-full max-w-lg" aria-label={resource.name} src={`${base}/content?reload=${reload}`} onError={() => setError("This audio format is not supported by your browser. Download it to play locally.")} />
          </div>
        ) : preview?.type === "pdf" ? (
          <iframe key={`${base}-${reload}`} title={`PDF preview: ${resource.name}`} className="h-full w-full border-0 bg-muted" src={`${base}/content?reload=${reload}`} />
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
            <FileQuestion size={40} className="text-muted-foreground" aria-hidden />
            <h3 className="font-medium">Preview unavailable</h3>
            <p className="max-w-md text-sm text-muted-foreground">{preview?.reason || "This file could not be opened. Reload or download it to open locally."}</p>
            <a href={`${base}/download`} download={resource.name} className="mt-2 inline-flex h-9 items-center gap-2 rounded-md bg-accent-foreground px-4 text-sm font-medium text-primary-foreground"><Download size={15} aria-hidden /> Download file</a>
          </div>
        )}
      </main>

      <footer className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-border bg-card px-4 py-2 text-xs text-muted-foreground">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span>{formatSize(preview?.size ?? resource.size)}</span>
          {text && <>
            <span>UTF-8{bom ? " BOM" : ""}</span>
            <span>{lineEnding}</span>
            <span className="tabular-nums">Ln {cursor.line}, Col {cursor.column}</span>
            <span role="status" className="flex items-center gap-1">
              {saving ? "Saving…" : dirty ? "Unsaved changes" : editing ? <><Check size={12} aria-hidden /> Saved</> : "Read-only preview"}
            </span>
          </>}
          {!text && preview && <span>{preview.mime}</span>}
          {preview?.reason && preview.type === "text" && <span>{preview.reason}</span>}
        </div>
        <div className="flex items-center gap-1">
          {editing && <span className="mr-2 hidden sm:inline">Ctrl / ⌘ S to save</span>}
          {index >= 0 && <>
            <Button variant="ghost" size="icon-sm" aria-label="Previous file" disabled={index === 0 || saving} onClick={() => navigate(-1)}><ChevronLeft size={16} /></Button>
            <span className="min-w-12 text-center tabular-nums">{index + 1} / {candidates.length}</span>
            <Button variant="ghost" size="icon-sm" aria-label="Next file" disabled={index === candidates.length - 1 || saving} onClick={() => navigate(1)}><ChevronRight size={16} /></Button>
          </>}
        </div>
      </footer>
    </dialog>,
    document.body,
  );
}
