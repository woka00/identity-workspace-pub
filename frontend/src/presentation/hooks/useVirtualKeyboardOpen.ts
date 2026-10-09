import { useEffect, useState } from "react";

export function useVirtualKeyboardOpen() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const viewport = window.visualViewport;
    if (!viewport) return;
    let maximumHeight = viewport.height;
    let blurTimer: number | undefined;

    const editableFocused = () => {
      const active = document.activeElement;
      return active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement || (active instanceof HTMLElement && active.isContentEditable);
    };
    const sync = () => {
      maximumHeight = Math.max(maximumHeight, viewport.height);
      const lostHeight = Math.max(maximumHeight, window.innerHeight) - viewport.height;
      setOpen(editableFocused() && lostHeight > 120);
    };
    const onFocusOut = () => {
      if (blurTimer !== undefined) window.clearTimeout(blurTimer);
      blurTimer = window.setTimeout(sync, 0);
    };

    sync();
    viewport.addEventListener("resize", sync);
    viewport.addEventListener("scroll", sync);
    window.addEventListener("resize", sync);
    document.addEventListener("focusin", sync);
    document.addEventListener("focusout", onFocusOut);
    return () => {
      if (blurTimer !== undefined) window.clearTimeout(blurTimer);
      viewport.removeEventListener("resize", sync);
      viewport.removeEventListener("scroll", sync);
      window.removeEventListener("resize", sync);
      document.removeEventListener("focusin", sync);
      document.removeEventListener("focusout", onFocusOut);
    };
  }, []);

  return open;
}
