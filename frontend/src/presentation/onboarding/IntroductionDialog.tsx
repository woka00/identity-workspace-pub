import { useEffect, useRef } from "react";
import type { ReactNode } from "react";

export default function IntroductionDialog({ titleID, children, onClose }: { titleID: string; children: ReactNode; onClose?: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const previousOverflow = document.body.style.overflow;
    dialog?.showModal();
    const heading = dialog?.querySelector<HTMLElement>("h1");
    heading?.setAttribute("tabindex", "-1");
    heading?.focus({ preventScroll: true });
    document.body.style.overflow = "hidden";
    return () => {
      dialog?.close();
      document.body.style.overflow = previousOverflow;
      previousFocus?.focus();
    };
  }, []);

  return <dialog ref={ref} className="introductionDialog" aria-labelledby={titleID} onCancel={(event) => { event.preventDefault(); event.stopPropagation(); onClose?.(); }} onKeyDown={(event) => event.stopPropagation()}>
    {onClose && <button type="button" className="introductionClose" aria-label="Закрыть" onClick={onClose}>×</button>}
    {children}
  </dialog>;
}
