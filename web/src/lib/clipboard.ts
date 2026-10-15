// Call directly from a click/keyboard handler so the fallback retains the
// browser's user activation. No permissions are requested or security flags
// changed; browsers that refuse both paths return false for manual copying.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (window.isSecureContext && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Permission, embedding and focus policies can reject the modern API.
  }
  return copyUsingSelection(text);
}

function copyUsingSelection(text: string): boolean {
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const selection = document.getSelection();
  const ranges = selection
    ? Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index).cloneRange())
    : [];
  const control = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement ? active : null;
  const start = control?.selectionStart;
  const end = control?.selectionEnd;
  const direction = control?.selectionDirection;
  const buffer = document.createElement("textarea");
  buffer.value = text;
  buffer.readOnly = true;
  buffer.tabIndex = -1;
  buffer.setAttribute("aria-label", "Copy buffer");
  // A selectable, offscreen control avoids layout/scroll jumps and the mobile
  // keyboard. display:none/hidden controls cannot be copied using selection.
  buffer.style.cssText = "position:fixed;left:-10000px;top:0;width:1px;height:1px;font-size:16px;";
  try {
    document.body.appendChild(buffer);
    buffer.focus({ preventScroll: true });
    buffer.select();
    buffer.setSelectionRange(0, buffer.value.length);
    // Deprecated but still useful for user-initiated copying on ordinary HTTP.
    // A false result is a failure, never grounds for a success toast.
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    buffer.remove();
    if (active?.isConnected) active.focus({ preventScroll: true });
    if (selection) {
      selection.removeAllRanges();
      for (const range of ranges) selection.addRange(range);
    }
    if (control?.isConnected && start != null && end != null) {
      control.setSelectionRange(start, end, direction ?? undefined);
    }
  }
}
