import { useEffect, useRef, type RefObject } from "react";
import { measureContentEditableCaret } from "../../infrastructure/browser/caretGeometry";

// Delay before starting a fresh expand cycle after the last user action.
const BLINK_IDLE_MS = 500;

export function useAnimatedCaret(editorRef: RefObject<HTMLElement>) {
  const overlayRef = useRef<HTMLDivElement>(null);
  const caretRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    const editor = editorRef.current;
    const overlay = overlayRef.current;
    const caret = caretRef.current;
    if (!editor || !overlay || !caret) return;

    let frame = 0;
    let blinkTimer = 0;
    let visible = false;
    let composing = false;
    let snap = true;
    let disposed = false;

    const resetBlink = () => {
      window.clearTimeout(blinkTimer);
      delete caret.dataset.blinking;
      blinkTimer = window.setTimeout(() => {
        if (visible) caret.dataset.blinking = "true";
      }, BLINK_IDLE_MS);
    };
    const hide = () => {
      visible = false;
      caret.style.visibility = "hidden";
      delete editor.dataset.animatedCaret;
      delete caret.dataset.blinking;
      window.clearTimeout(blinkTimer);
    };
    const update = () => {
      frame = 0;
      if (document.activeElement !== editor || document.hidden || composing) {
        hide();
        return;
      }
      const geometry = measureContentEditableCaret(editor, overlay);
      if (!geometry) {
        hide();
        return;
      }
      const { x, y, height } = geometry;
      // Scroll/layout changes snap to the text; only actual caret movement glides.
      caret.style.transitionProperty = snap || !visible ? "none" : "transform";
      caret.style.transform = `translate3d(${x}px, ${y}px, 0)`;
      caret.style.height = `${height}px`;
      caret.style.visibility = "visible";
      editor.dataset.animatedCaret = "true";
      if (!visible) resetBlink();
      visible = true;
      snap = false;
    };
    const schedule = () => {
      if (!disposed && !frame) frame = requestAnimationFrame(update);
    };
    const activity = () => {
      if (document.activeElement !== editor) return;
      resetBlink();
      schedule();
    };
    const layout = () => {
      snap = true;
      schedule();
    };
    const compositionStart = () => {
      composing = true;
      // Keep the browser's IME caret/candidate anchor for the composition session.
      hide();
    };
    const compositionEnd = () => {
      composing = false;
      activity();
    };
    const focus = () => {
      snap = true;
      activity();
    };
    const visibility = () => document.hidden ? hide() : focus();
    const activityEvents = ["beforeinput", "input", "keydown", "keyup", "pointerdown", "pointerup", "click"] as const;
    activityEvents.forEach((event) => editor.addEventListener(event, activity));
    editor.addEventListener("focus", focus);
    editor.addEventListener("blur", hide);
    editor.addEventListener("compositionstart", compositionStart);
    editor.addEventListener("compositionend", compositionEnd);
    document.addEventListener("selectionchange", activity);
    document.addEventListener("scroll", layout, true);
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("resize", layout);
    window.addEventListener("blur", hide);
    window.addEventListener("focus", focus);
    window.visualViewport?.addEventListener("resize", layout);
    window.visualViewport?.addEventListener("scroll", layout);
    const resizeObserver = new ResizeObserver(layout);
    resizeObserver.observe(editor);
    resizeObserver.observe(overlay);
    const mutationObserver = new MutationObserver((records) => {
      if (records.some((record) => record.type === "attributes")) layout();
      else activity();
    });
    mutationObserver.observe(editor, { childList: true, characterData: true, subtree: true, attributes: true, attributeFilter: ["style", "class", "dir"] });
    // Ancestor styles can change fonts/line-height without resizing a capped editor.
    for (let ancestor = editor.parentElement; ancestor; ancestor = ancestor.parentElement) {
      mutationObserver.observe(ancestor, { attributes: true, attributeFilter: ["style", "class", "dir"] });
    }
    document.fonts.addEventListener("loadingdone", layout);
    void document.fonts.ready.then(() => { if (!disposed) layout(); });
    schedule();

    return () => {
      disposed = true;
      cancelAnimationFrame(frame);
      hide();
      activityEvents.forEach((event) => editor.removeEventListener(event, activity));
      editor.removeEventListener("focus", focus);
      editor.removeEventListener("blur", hide);
      editor.removeEventListener("compositionstart", compositionStart);
      editor.removeEventListener("compositionend", compositionEnd);
      document.removeEventListener("selectionchange", activity);
      document.removeEventListener("scroll", layout, true);
      document.removeEventListener("visibilitychange", visibility);
      window.removeEventListener("resize", layout);
      window.removeEventListener("blur", hide);
      window.removeEventListener("focus", focus);
      window.visualViewport?.removeEventListener("resize", layout);
      window.visualViewport?.removeEventListener("scroll", layout);
      document.fonts.removeEventListener("loadingdone", layout);
      resizeObserver.disconnect();
      mutationObserver.disconnect();
    };
  }, [editorRef]);

  return { overlayRef, caretRef };
}
