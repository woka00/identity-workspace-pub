export type CaretGeometry = { x: number; y: number; height: number };

let caretMeasureContext: CanvasRenderingContext2D | null | undefined;

function adjacentCaretGlyph(value: string, position: number) {
  const previousGlyphs = Array.from(value.slice(0, position));
  const previous = previousGlyphs[previousGlyphs.length - 1] ?? "";
  const next = Array.from(value.slice(position))[0] ?? "";
  if (previous && !/\s/u.test(previous)) return previous;
  if (next && !/\s/u.test(next)) return next;
  return previous || next || "x";
}

function glyphCaretHeight(control: HTMLInputElement | HTMLTextAreaElement, style: CSSStyleDeclaration, position: number) {
  if (control.dataset.caretHeight !== "glyph" || typeof document === "undefined") return null;
  if (caretMeasureContext === undefined) caretMeasureContext = document.createElement("canvas").getContext("2d");
  if (!caretMeasureContext) return null;

  const fontSize = Number.parseFloat(style.fontSize);
  if (!Number.isFinite(fontSize) || fontSize <= 0) return null;
  caretMeasureContext.font = `${style.fontStyle} ${style.fontVariant} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
  const metrics = caretMeasureContext.measureText(adjacentCaretGlyph(control.value, position));
  const measuredHeight = metrics.actualBoundingBoxAscent + metrics.actualBoundingBoxDescent;
  if (!Number.isFinite(measuredHeight) || measuredHeight <= 0) return null;
  return Math.min(fontSize * .9, Math.max(fontSize * .68, measuredHeight * 1.15));
}

const mirroredTextProperties = [
  "direction",
  "font-family",
  "font-size",
  "font-style",
  "font-variant",
  "font-weight",
  "font-stretch",
  "font-feature-settings",
  "font-kerning",
  "font-variation-settings",
  "line-height",
  "letter-spacing",
  "word-spacing",
  "text-align",
  "text-indent",
  "text-transform",
  "word-break",
  "overflow-wrap",
  "tab-size",
  "padding-top",
  "padding-right",
  "padding-bottom",
  "padding-left",
  "border-top-width",
  "border-right-width",
  "border-bottom-width",
  "border-left-width",
  "border-top-style",
  "border-right-style",
  "border-bottom-style",
  "border-left-style",
] as const;

export function measureTextControlCaret(control: HTMLInputElement | HTMLTextAreaElement): CaretGeometry | null {
  const position = control.selectionStart;
  if (position === null || position !== control.selectionEnd) return null;

  const bounds = control.getBoundingClientRect();
  const scaleX = bounds.width / control.offsetWidth;
  const scaleY = bounds.height / control.offsetHeight;
  if (!bounds.width || !bounds.height || !Number.isFinite(scaleX) || !Number.isFinite(scaleY)) return null;
  const style = getComputedStyle(control);
  const mirror = document.createElement("div");
  const marker = document.createElement("span");
  mirror.setAttribute("aria-hidden", "true");
  Object.assign(mirror.style, {
    position: "fixed",
    top: `${bounds.top}px`,
    left: `${bounds.left}px`,
    boxSizing: "border-box",
    width: `${control.offsetWidth}px`,
    height: `${control.offsetHeight}px`,
    overflow: "hidden",
    visibility: "hidden",
    pointerEvents: "none",
    whiteSpace: control instanceof HTMLTextAreaElement ? "pre-wrap" : "pre",
    transform: `scale(${scaleX}, ${scaleY})`,
    transformOrigin: "top left",
  });
  mirroredTextProperties.forEach((property) => mirror.style.setProperty(property, style.getPropertyValue(property)));
  if (control instanceof HTMLInputElement && control.type === "password") {
    mirror.style.setProperty("-webkit-text-security", "disc");
  }

  mirror.append(document.createTextNode(control.value.slice(0, position)));
  const remainder = control.value.slice(position);
  marker.textContent = remainder || "\u200b";
  mirror.append(marker);
  document.body.append(mirror);
  mirror.scrollLeft = control.scrollLeft;
  mirror.scrollTop = control.scrollTop;

  const mirrorRect = mirror.getBoundingClientRect();
  const markerRect = marker.getClientRects()[0];
  const borderLeft = (Number.parseFloat(style.borderLeftWidth) || 0) * scaleX;
  const borderRight = (Number.parseFloat(style.borderRightWidth) || 0) * scaleX;
  const borderTop = (Number.parseFloat(style.borderTopWidth) || 0) * scaleY;
  const borderBottom = (Number.parseFloat(style.borderBottomWidth) || 0) * scaleY;
  const paddingLeft = (Number.parseFloat(style.paddingLeft) || 0) * scaleX;
  const paddingTop = (Number.parseFloat(style.paddingTop) || 0) * scaleY;
  const paddingBottom = (Number.parseFloat(style.paddingBottom) || 0) * scaleY;
  const lineHeight = Number.parseFloat(style.lineHeight);
  const fontSize = Number.parseFloat(style.fontSize);
  const lineBoxHeight = markerRect
    ? (Number.isFinite(lineHeight) ? lineHeight * scaleY : markerRect.height)
    : (Number.isFinite(lineHeight) ? lineHeight * scaleY : null);
  // Editor carets are one em high and centered inside the line box. Using the
  // full line-height makes the caret look top-heavy next to the visible glyphs.
  const fontHeight = Number.isFinite(fontSize) && fontSize > 0 ? fontSize * scaleY : null;
  const measuredGlyphHeight = glyphCaretHeight(control, style, position);
  const desiredCaretHeight = measuredGlyphHeight === null ? fontHeight : measuredGlyphHeight * scaleY;
  const caretHeight = lineBoxHeight === null
    ? desiredCaretHeight
    : Math.min(lineBoxHeight, desiredCaretHeight ?? lineBoxHeight);
  const lineLeading = lineBoxHeight !== null && caretHeight !== null
    ? Math.max(0, (lineBoxHeight - caretHeight) / 2)
    : 0;
  // Browsers vertically center the line box inside a single-line input,
  // while a regular div mirror starts it at padding-top.
  const inputCaretY = control instanceof HTMLInputElement && caretHeight !== null
    ? bounds.top + borderTop + paddingTop + Math.max(
        0,
        (bounds.height - borderTop - borderBottom - paddingTop - paddingBottom - caretHeight) / 2,
      )
    : null;
  const geometry = markerRect && caretHeight !== null ? {
    // Mobile WebKit can rebase fixed elements when the software keyboard opens.
    // Measure inside the mirror, then anchor that local position to the real
    // control instead of assuming the mirror kept the requested viewport origin.
    x: bounds.left + markerRect.left - mirrorRect.left,
    // Inline marker rects are already positioned within the textarea line box.
    y: inputCaretY ?? bounds.top + markerRect.top - mirrorRect.top + lineLeading,
    height: caretHeight,
  } : null;
  mirror.remove();

  // Empty controls can expose no client rect for the zero-width marker on
  // initial focus. Use the real line-box origin so the custom caret is visible.
  const emptyGeometry = !control.value && position === 0 && caretHeight !== null
    ? {
        x: bounds.left + borderLeft + paddingLeft - control.scrollLeft,
        y: inputCaretY ?? bounds.top + borderTop + paddingTop + lineLeading - control.scrollTop,
        height: caretHeight,
      }
    : null;
  if (!geometry || !Number.isFinite(geometry.height) || geometry.height <= 0) {
    return emptyGeometry;
  }
  const tolerance = 1;
  if (
    geometry.x < bounds.left + borderLeft - tolerance ||
    geometry.x > bounds.right - borderRight + tolerance ||
    geometry.y + geometry.height < bounds.top + borderTop ||
    geometry.y > bounds.bottom - borderBottom
  ) return emptyGeometry;
  return geometry;
}

// Work on cloned ranges only: measurement must never edit the DOM or native selection.
export function measureContentEditableCaret(editor: HTMLElement, overlay: HTMLElement): CaretGeometry | null {
  const selection = window.getSelection();
  if (!selection?.isCollapsed || !selection.rangeCount) return null;
  const range = selection.getRangeAt(0).cloneRange();
  if (!editor.contains(range.startContainer)) return null;

  let element = range.startContainer instanceof HTMLElement ? range.startContainer : range.startContainer.parentElement;
  if (!element || element.closest('[contenteditable="false"]')) return null;
  let rect = range.getClientRects()[0];
  let useRightEdge = false;

  // Empty lines and element-boundary selections may have no collapsed range box.
  // Resolve the actual adjacent DOM node, including the <br> in an empty checklist.
  if (!rect?.height && range.startContainer instanceof HTMLElement) {
    const container = range.startContainer;
    const atEnd = range.startOffset === container.childNodes.length;
    let node: Node | undefined = container.childNodes[range.startOffset] ?? container.lastChild ?? undefined;
    while (node instanceof HTMLElement && node.tagName !== "BR" && node.childNodes.length) {
      if (node.contentEditable === "false") break;
      node = (atEnd ? node.lastChild : node.firstChild) ?? undefined;
    }
    if (node) {
      element = node instanceof HTMLElement ? node : node.parentElement;
      if (node instanceof HTMLElement && (node.tagName === "BR" || node.contentEditable === "false")) {
        range.selectNode(node);
        // Native navigation can stop immediately before/after an atomic checkbox.
        useRightEdge = atEnd && node.contentEditable === "false";
      } else {
        range.selectNodeContents(node);
        range.collapse(!atEnd);
      }
      rect = range.getClientRects()[0];
    }
  }

  if (!element) return null;
  const bounds = overlay.getBoundingClientRect();
  const scaleX = bounds.width / overlay.clientWidth;
  const scaleY = bounds.height / overlay.clientHeight;
  if (!Number.isFinite(scaleX) || !Number.isFinite(scaleY) || scaleX <= 0 || scaleY <= 0) return null;
  const style = getComputedStyle(element);
  const lineHeight = Number.parseFloat(style.lineHeight);

  if (rect?.height) {
    const textHeight = rect.height / scaleY;
    const height = Number.isFinite(lineHeight) ? lineHeight : textHeight;
    return {
      x: ((useRightEdge ? rect.right : rect.left) - bounds.left) / scaleX,
      y: (rect.top - bounds.top) / scaleY - (height - textHeight) / 2,
      height,
    };
  }

  // A truly empty block has no glyph to measure. Use its real content box.
  // For other unmeasurable positions let the browser display its native caret.
  if (element.textContent || element.querySelector('[contenteditable="false"]') || !Number.isFinite(lineHeight)) return null;
  const block = element.getBoundingClientRect();
  return {
    x: (block.left - bounds.left) / scaleX + element.clientLeft + Number.parseFloat(style.paddingLeft) - element.scrollLeft,
    y: (block.top - bounds.top) / scaleY + element.clientTop + Number.parseFloat(style.paddingTop) - element.scrollTop,
    height: lineHeight,
  };
}
