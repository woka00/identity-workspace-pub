import { useEffect, useRef } from "react";
import { measureTextControlCaret } from "../infrastructure/browser/caretGeometry";

const BLINK_IDLE_MS = 500;
const NON_TEXT_INPUT_TYPES = new Set([
  "button", "checkbox", "color", "date", "datetime-local", "file", "hidden",
  "image", "month", "radio", "range", "reset", "submit", "time", "week",
]);

function textControl(element: EventTarget | null): HTMLInputElement | HTMLTextAreaElement | null {
  if (element instanceof HTMLTextAreaElement) return element.disabled || element.readOnly ? null : element;
  if (!(element instanceof HTMLInputElement) || NON_TEXT_INPUT_TYPES.has(element.type)) return null;
  return element.disabled || element.readOnly ? null : element;
}

function hasRunningAncestorAnimation(control: HTMLElement) {
  for (let element: Element | null = control; element; element = element.parentElement) {
    if (element.getAnimations().some((animation) => animation.pending || animation.playState === "running")) {
      return true;
    }
  }
  return false;
}

function visibleCaretColor(caretColor: string, textColor: string) {
  const normalized = caretColor.replace(/\s+/g, "").toLowerCase();
  const transparentAlpha = (/^(?:rgba|hsla)\(/.test(normalized) && /,0(?:\.0+)?%?\)$/.test(normalized))
    || /\/0(?:\.0+)?%?\)$/.test(normalized);
  return normalized && normalized !== "auto" && normalized !== "transparent" && !transparentAlpha
    ? caretColor
    : textColor;
}

export default function AnimatedTextCaret() {
  const originRef = useRef<HTMLSpanElement>(null);
  const caretRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    const origin = originRef.current;
    const caret = caretRef.current;
    if (!origin || !caret) return;

    let active: HTMLInputElement | HTMLTextAreaElement | null = null;
    let frame = 0;
    let blinkTimer = 0;
    let visible = false;
    let composing = false;
    let snap = true;
    let retryFrames = 0;
    let disposed = false;
    const resizeObserver = new ResizeObserver(() => layout());

    const clearActive = () => {
      if (active) delete active.dataset.animatedCaret;
      resizeObserver.disconnect();
      active = null;
    };
    const hide = () => {
      visible = false;
      caret.style.visibility = "hidden";
      delete caret.dataset.blinking;
      window.clearTimeout(blinkTimer);
      if (active) delete active.dataset.animatedCaret;
    };
    const resetBlink = () => {
      window.clearTimeout(blinkTimer);
      delete caret.dataset.blinking;
      blinkTimer = window.setTimeout(() => {
        if (visible) caret.dataset.blinking = "true";
      }, BLINK_IDLE_MS);
    };
    const update = () => {
      frame = 0;
      if (!active || document.activeElement !== active || document.hidden || composing) {
        hide();
        return;
      }
      const trackingAnimation = hasRunningAncestorAnimation(active);
      const geometry = measureTextControlCaret(active);
      if (!geometry) {
        hide();
        if (trackingAnimation || retryFrames > 0) {
          retryFrames = Math.max(0, retryFrames - 1);
          schedule();
        }
        return;
      }
      const style = getComputedStyle(active);
      // Convert viewport geometry through the overlay's real fixed origin;
      // mobile WebKit can move that origin when the software keyboard opens.
      const originBounds = origin.getBoundingClientRect();
      caret.style.color = visibleCaretColor(style.caretColor, style.color);
      caret.style.transitionProperty = snap || !visible || trackingAnimation ? "none" : "transform";
      caret.style.transform = `translate3d(${geometry.x - originBounds.left}px, ${geometry.y - originBounds.top}px, 0)`;
      caret.style.height = `${geometry.height}px`;
      caret.style.visibility = "visible";
      active.dataset.animatedCaret = "true";
      if (!visible) resetBlink();
      visible = true;
      snap = false;
      if (trackingAnimation || retryFrames > 0) {
        retryFrames = Math.max(0, retryFrames - 1);
        schedule();
      }
    };
    const schedule = () => {
      if (!disposed && !frame) frame = requestAnimationFrame(update);
    };
    const activity = (event?: Event) => {
      const control = textControl(event?.target ?? document.activeElement);
      if (control && control !== active) {
        clearActive();
        active = control;
        resizeObserver.observe(control);
        snap = true;
        retryFrames = 2;
      }
      if (!active || document.activeElement !== active) return;
      resetBlink();
      schedule();
    };
    const layout = () => {
      snap = true;
      schedule();
    };
    const viewportLayout = () => schedule();
    const focusOut = (event: FocusEvent) => {
      if (event.target !== active) return;
      hide();
      clearActive();
    };
    const compositionStart = (event: CompositionEvent) => {
      if (event.target !== active) return;
      composing = true;
      hide();
    };
    const compositionEnd = (event: CompositionEvent) => {
      if (event.target !== active) return;
      composing = false;
      activity(event);
    };
    const visibility = () => document.hidden ? hide() : activity();
    const animatedLayout = (event: Event) => {
      if (active && event.target instanceof Element && event.target.contains(active)) layout();
    };
    const activityEvents = ["beforeinput", "input", "keydown", "keyup", "pointerdown", "pointerup", "click", "select"] as const;
    const animationEvents = ["animationstart", "animationend", "animationcancel", "transitionrun", "transitionend", "transitioncancel"] as const;
    document.addEventListener("focusin", activity);
    document.addEventListener("focusout", focusOut);
    activityEvents.forEach((event) => document.addEventListener(event, activity, true));
    document.addEventListener("selectionchange", activity);
    document.addEventListener("scroll", layout, true);
    document.addEventListener("compositionstart", compositionStart, true);
    document.addEventListener("compositionend", compositionEnd, true);
    animationEvents.forEach((event) => document.addEventListener(event, animatedLayout, true));
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("resize", layout);
    window.addEventListener("blur", hide);
    window.addEventListener("focus", activity);
    window.visualViewport?.addEventListener("resize", viewportLayout);
    window.visualViewport?.addEventListener("scroll", viewportLayout);
    activity();

    return () => {
      disposed = true;
      cancelAnimationFrame(frame);
      hide();
      clearActive();
      document.removeEventListener("focusin", activity);
      document.removeEventListener("focusout", focusOut);
      activityEvents.forEach((event) => document.removeEventListener(event, activity, true));
      document.removeEventListener("selectionchange", activity);
      document.removeEventListener("scroll", layout, true);
      document.removeEventListener("compositionstart", compositionStart, true);
      document.removeEventListener("compositionend", compositionEnd, true);
      animationEvents.forEach((event) => document.removeEventListener(event, animatedLayout, true));
      document.removeEventListener("visibilitychange", visibility);
      window.removeEventListener("resize", layout);
      window.removeEventListener("blur", hide);
      window.removeEventListener("focus", activity);
      window.visualViewport?.removeEventListener("resize", viewportLayout);
      window.visualViewport?.removeEventListener("scroll", viewportLayout);
    };
  }, []);

  return <span ref={originRef} className="animatedTextCaretOrigin" aria-hidden="true"><span ref={caretRef} className="animatedTextCaret"><span className="animatedTextCaretLine" /></span></span>;
}
