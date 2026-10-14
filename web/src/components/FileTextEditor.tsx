import { useEffect, useRef } from "react";
import { basicSetup } from "codemirror";
import { Compartment, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { HighlightStyle, LanguageDescription, syntaxHighlighting } from "@codemirror/language";
import { languages } from "@codemirror/language-data";
import { tags } from "@lezer/highlight";

const highlighting = syntaxHighlighting(HighlightStyle.define([
  { tag: [tags.keyword, tags.heading], color: "hsl(var(--accent-foreground))", fontWeight: "600" },
  { tag: [tags.string, tags.regexp], color: "color-mix(in srgb, hsl(var(--success)) 70%, hsl(var(--foreground)))" },
  { tag: [tags.number, tags.bool, tags.typeName], color: "color-mix(in srgb, hsl(var(--warning)) 65%, hsl(var(--foreground)))" },
  { tag: tags.comment, color: "hsl(var(--muted-foreground))", fontStyle: "italic" },
  { tag: tags.strong, fontWeight: "600" },
  { tag: tags.emphasis, fontStyle: "italic" },
  { tag: tags.link, color: "hsl(var(--accent-foreground))", textDecoration: "underline" },
  { tag: [tags.propertyName, tags.attributeName], color: "hsl(var(--accent-foreground))" },
  { tag: tags.invalid, color: "hsl(var(--destructive))" },
]));

// A real, locally bundled editor: virtualized lines, search, history, selection
// and lazy language support. Tab retains its normal focus-navigation behavior.
export default function FileTextEditor({
  initialContent,
  name,
  editable,
  wrap,
  onChange,
  onCursor,
}: {
  initialContent: string;
  name: string;
  editable: boolean;
  wrap: boolean;
  onChange: (content: string) => void;
  onCursor: (line: number, column: number) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView>();
  const mode = useRef(new Compartment());
  const wrapping = useRef(new Compartment());
  const callbacks = useRef({ onChange, onCursor });
  callbacks.current = { onChange, onCursor };

  useEffect(() => {
    if (!container.current) return;
    const language = new Compartment();
    const editor = new EditorView({
      parent: container.current,
      state: EditorState.create({
        doc: initialContent,
        extensions: [
          basicSetup,
          highlighting,
          mode.current.of([
            EditorState.readOnly.of(!editable),
            EditorView.editable.of(editable),
            EditorView.contentAttributes.of({
              "aria-label": editable ? `Edit ${name}` : `Preview ${name}`,
              "aria-readonly": String(!editable),
              tabindex: "0",
              spellcheck: "false",
            }),
          ]),
          wrapping.current.of(wrap ? EditorView.lineWrapping : []),
          language.of([]),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) callbacks.current.onChange(update.state.sliceDoc());
            if (update.selectionSet || update.docChanged) {
              const head = update.state.selection.main.head;
              const line = update.state.doc.lineAt(head);
              callbacks.current.onCursor(line.number, head - line.from + 1);
            }
          }),
          EditorView.theme({
            "&": { height: "100%", color: "hsl(var(--foreground))", backgroundColor: "hsl(var(--background))" },
            ".cm-scroller": { overflow: "auto", fontFamily: "'JetBrains Mono', ui-monospace, monospace", fontSize: "14px", lineHeight: "1.7" },
            "@media (max-width: 640px)": { ".cm-scroller": { fontSize: "16px" } },
            ".cm-content": { padding: "16px 0", caretColor: "hsl(var(--primary))" },
            ".cm-line": { padding: "0 16px" },
            ".cm-gutters": { backgroundColor: "hsl(var(--muted) / .4)", color: "hsl(var(--muted-foreground))", borderRight: "1px solid hsl(var(--border))" },
            ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "hsl(var(--accent) / .45)" },
            ".cm-cursor": { borderLeftColor: "hsl(var(--primary))" },
            "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": { backgroundColor: "hsl(var(--primary) / .2)" },
            "&.cm-focused": { outline: "2px solid hsl(var(--ring) / .5)", outlineOffset: "-2px" },
            ".cm-panels, .cm-tooltip": { backgroundColor: "hsl(var(--card))", color: "hsl(var(--foreground))", borderColor: "hsl(var(--border))" },
            ".cm-search input, .cm-search button": { color: "hsl(var(--foreground))", background: "hsl(var(--muted))", border: "1px solid hsl(var(--border))" },
          }),
        ],
      }),
    });
    view.current = editor;
    let active = true;
    const match = LanguageDescription.matchFilename(languages, name);
    if (match) {
      match.load().then((support) => {
        if (active) editor.dispatch({ effects: language.reconfigure(support) });
      }).catch(() => {
        // Source remains editable if optional syntax support fails to load.
      });
    }
    return () => {
      active = false;
      editor.destroy();
      view.current = undefined;
    };
    // A parent key resets the document only on open/reload, never on keystrokes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialContent, name]);

  useEffect(() => {
    view.current?.dispatch({
      effects: mode.current.reconfigure([
        EditorState.readOnly.of(!editable),
        EditorView.editable.of(editable),
        EditorView.contentAttributes.of({
          "aria-label": editable ? `Edit ${name}` : `Preview ${name}`,
          "aria-readonly": String(!editable),
          tabindex: "0",
          spellcheck: "false",
        }),
      ]),
    });
    if (editable) view.current?.focus();
  }, [editable, name]);

  useEffect(() => {
    view.current?.dispatch({ effects: wrapping.current.reconfigure(wrap ? EditorView.lineWrapping : []) });
  }, [wrap]);

  return <div ref={container} className="h-full min-h-0" data-testid="file-text-editor" />;
}
