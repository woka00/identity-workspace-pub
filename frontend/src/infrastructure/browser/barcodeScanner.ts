type NativeBarcodeDetectorInstance = {
  detect(source: CanvasImageSource): Promise<Array<{ rawValue?: string; format?: string }>>;
};
type NativeBarcodeDetector = {
  new (options?: { formats?: string[] }): NativeBarcodeDetectorInstance;
  getSupportedFormats?: () => Promise<string[]>;
};

type BarcodeImageVariant = { topRatio: number; heightRatio: number; rotation: 0 | 90 };
type BarcodeCanvasMode = "contrast" | "binary";

const nativeBarcodeFormats = ["ean_13", "ean_8", "upc_a", "upc_e"];

export interface BarcodeFrameDecoder {
  decode(canvas: HTMLCanvasElement): Promise<string>;
}

export function normalizeBarcodeValue(raw: string) {
  // Keep one stable, printable key across native and ZXing decoders.
  const value = raw.trim()
    .replace(/^]C1/, "")
    .replace(/\x1d/g, "|")
    .replace(/\s+/gu, " ")
    .replace(/[\x00-\x1c\x1e-\x1f\x7f-\x9f]/g, (character) => `\\x${character.charCodeAt(0).toString(16).padStart(2, "0")}`);
  return value.length > 0 && Array.from(value).length <= 512 ? value : "";
}

function firstDetectedBarcode(results: Array<{ rawValue?: string; format?: string }>) {
  for (const result of results) {
    if (result.format?.toLocaleLowerCase() === "code_128") continue;
    const value = normalizeBarcodeValue(result.rawValue ?? "");
    if (value) return value;
  }
  return "";
}

function otsuThreshold(histogram: Uint32Array, pixelCount: number) {
  let total = 0;
  for (let value = 0; value < histogram.length; value += 1) total += value * histogram[value];
  let backgroundWeight = 0;
  let backgroundTotal = 0;
  let maximumVariance = -1;
  let threshold = 128;
  for (let value = 0; value < histogram.length; value += 1) {
    backgroundWeight += histogram[value];
    if (backgroundWeight === 0) continue;
    const foregroundWeight = pixelCount - backgroundWeight;
    if (foregroundWeight === 0) break;
    backgroundTotal += value * histogram[value];
    const backgroundMean = backgroundTotal / backgroundWeight;
    const foregroundMean = (total - backgroundTotal) / foregroundWeight;
    const variance = backgroundWeight * foregroundWeight * (backgroundMean - foregroundMean) ** 2;
    if (variance > maximumVariance) {
      maximumVariance = variance;
      threshold = value;
    }
  }
  return threshold;
}

function enhancedBarcodeCanvas(source: HTMLCanvasElement, mode: BarcodeCanvasMode) {
  const scale = Math.max(1, Math.min(4, 1400 / Math.max(1, source.width)));
  const contentWidth = Math.max(1, Math.round(source.width * scale));
  const contentHeight = Math.max(1, Math.round(source.height * scale));
  const horizontalPadding = Math.max(24, Math.round(contentWidth * 0.035));
  const verticalPadding = Math.max(8, Math.round(contentHeight * 0.04));
  const canvas = document.createElement("canvas");
  canvas.width = contentWidth + horizontalPadding * 2;
  canvas.height = contentHeight + verticalPadding * 2;
  const context = canvas.getContext("2d", { willReadFrequently: true });
  if (!context) throw new Error("barcode canvas is unavailable");
  context.fillStyle = "#fff";
  context.fillRect(0, 0, canvas.width, canvas.height);
  context.imageSmoothingEnabled = false;
  context.drawImage(source, 0, 0, source.width, source.height, horizontalPadding, verticalPadding, contentWidth, contentHeight);
  const pixels = context.getImageData(0, 0, canvas.width, canvas.height);
  const histogram = new Uint32Array(256);
  for (let index = 0; index < pixels.data.length; index += 4) {
    const luminance = Math.round(pixels.data[index] * 0.299 + pixels.data[index + 1] * 0.587 + pixels.data[index + 2] * 0.114);
    histogram[luminance] += 1;
  }
  const threshold = otsuThreshold(histogram, pixels.data.length / 4);
  for (let index = 0; index < pixels.data.length; index += 4) {
    const luminance = pixels.data[index] * 0.299 + pixels.data[index + 1] * 0.587 + pixels.data[index + 2] * 0.114;
    const output = mode === "binary"
      ? (luminance <= threshold ? 0 : 255)
      : Math.max(0, Math.min(255, (luminance - threshold) * 1.8 + 128));
    pixels.data[index] = output;
    pixels.data[index + 1] = output;
    pixels.data[index + 2] = output;
  }
  context.putImageData(pixels, 0, 0);
  return canvas;
}

async function createNativeBarcodeDetector() {
  const detectorConstructor = (window as Window & { BarcodeDetector?: NativeBarcodeDetector }).BarcodeDetector;
  if (!detectorConstructor) return null;
  let formats = nativeBarcodeFormats;
  if (detectorConstructor.getSupportedFormats) {
    try {
      const supported = await detectorConstructor.getSupportedFormats();
      formats = nativeBarcodeFormats.filter((format) => supported.includes(format));
    } catch {
      // Construct the detector with the conservative fallback below.
    }
  }
  try {
    return new detectorConstructor({ formats: formats.length > 0 ? formats : ["ean_13", "ean_8", "upc_a", "upc_e"] });
  } catch {
    try {
      return new detectorConstructor({ formats: ["ean_13", "ean_8", "upc_a", "upc_e"] });
    } catch {
      return null;
    }
  }
}

async function createZXingBarcodeReader() {
  const [{ BrowserMultiFormatReader }, { BarcodeFormat, DecodeHintType }] = await Promise.all([
    import("@zxing/browser"),
    import("@zxing/library"),
  ]);
  const hints = new Map<any, any>();
  hints.set(DecodeHintType.TRY_HARDER, true);
  hints.set(DecodeHintType.POSSIBLE_FORMATS, [
    BarcodeFormat.EAN_13, BarcodeFormat.EAN_8, BarcodeFormat.UPC_A, BarcodeFormat.UPC_E,
    BarcodeFormat.RSS_14, BarcodeFormat.RSS_EXPANDED,
  ]);
  const reader = new BrowserMultiFormatReader(hints);
  return {
    decode(canvas: HTMLCanvasElement) {
      const result = reader.decodeFromCanvas(canvas);
      if (result.getBarcodeFormat() === BarcodeFormat.CODE_128) return "";
      return normalizeBarcodeValue(result.getText());
    },
  };
}

export async function createBarcodeFrameDecoder(): Promise<BarcodeFrameDecoder> {
  let detector = await createNativeBarcodeDetector();
  const reader = await createZXingBarcodeReader();
  let frameNumber = 0;

  return {
    async decode(canvas) {
      if (!canvas.width || !canvas.height) return "";
      if (detector) {
        try {
          const detected = await detector.detect(canvas);
          const value = firstDetectedBarcode(detected);
          if (value) return value;
        } catch {
          detector = null;
        }
      }
      const phase = frameNumber++ % 3;
      const variants = phase === 0
        ? [canvas]
        : [canvas, enhancedBarcodeCanvas(canvas, phase === 1 ? "contrast" : "binary")];
      for (const variant of variants) {
        try {
          const value = reader.decode(variant);
          if (value) return value;
        } catch {
          // A live frame without a readable barcode is expected.
        }
      }
      return "";
    },
  };
}

export function captureBarcodeVideoRegion(video: HTMLVideoElement, region: DOMRectReadOnly) {
  const bounds = video.getBoundingClientRect();
  const sourceWidth = video.videoWidth;
  const sourceHeight = video.videoHeight;
  if (!sourceWidth || !sourceHeight || !bounds.width || !bounds.height) return null;

  // The preview uses object-fit: cover. Recreate that transform in reverse so
  // the canvas contains exactly the pixels visible inside the scanner frame.
  const coverScale = Math.max(bounds.width / sourceWidth, bounds.height / sourceHeight);
  const renderedWidth = sourceWidth * coverScale;
  const renderedHeight = sourceHeight * coverScale;
  const hiddenX = (renderedWidth - bounds.width) / 2;
  const hiddenY = (renderedHeight - bounds.height) / 2;
  const rawX = (region.left - bounds.left + hiddenX) / coverScale;
  const rawY = (region.top - bounds.top + hiddenY) / coverScale;
  const rawWidth = region.width / coverScale;
  const rawHeight = region.height / coverScale;
  const sourceX = Math.max(0, Math.min(sourceWidth - 1, rawX));
  const sourceY = Math.max(0, Math.min(sourceHeight - 1, rawY));
  const cropWidth = Math.max(1, Math.min(sourceWidth - sourceX, rawWidth));
  const cropHeight = Math.max(1, Math.min(sourceHeight - sourceY, rawHeight));
  const outputScale = Math.min(1, 1600 / Math.max(cropWidth, cropHeight));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(cropWidth * outputScale));
  canvas.height = Math.max(1, Math.round(cropHeight * outputScale));
  const context = canvas.getContext("2d", { willReadFrequently: true });
  if (!context) return null;
  context.imageSmoothingEnabled = true;
  context.drawImage(video, sourceX, sourceY, cropWidth, cropHeight, 0, 0, canvas.width, canvas.height);
  return canvas;
}

export async function loadBarcodeImage(source: string) {
  const image = new Image();
  image.decoding = "async";
  image.src = source;
  await new Promise<void>((resolve, reject) => {
    image.onload = () => resolve();
    image.onerror = () => reject(new Error("barcode image could not be decoded"));
  });
  if (typeof image.decode === "function") {
    await image.decode().catch(() => undefined);
  }
  return image;
}

function barcodeImageVariants(): BarcodeImageVariant[] {
  const crops = [
    { topRatio: 0, heightRatio: 1 },
    { topRatio: 0, heightRatio: 0.68 },
    { topRatio: 0.16, heightRatio: 0.68 },
    { topRatio: 0.32, heightRatio: 0.68 },
  ];
  return [0, 90].flatMap((rotation) => crops.map((crop) => ({ ...crop, rotation: rotation as 0 | 90 })));
}

function renderBarcodeCanvas(image: HTMLImageElement, variant: BarcodeImageVariant) {
  const sourceWidth = image.naturalWidth || image.width;
  const sourceHeight = image.naturalHeight || image.height;
  if (!sourceWidth || !sourceHeight) throw new Error("barcode image has no dimensions");

  // Large phone photos add work without adding useful barcode detail. Keep a
  // generous resolution while making the slow TRY_HARDER pass predictable.
  const scale = Math.min(1, 2000 / Math.max(sourceWidth, sourceHeight));
  const width = Math.max(1, Math.round(sourceWidth * scale));
  const cropTop = sourceHeight * variant.topRatio;
  const cropHeight = Math.max(1, Math.round(sourceHeight * variant.heightRatio));
  const outputHeight = Math.max(1, Math.round(cropHeight * scale));
  const rotated = variant.rotation === 90;
  const canvas = document.createElement("canvas");
  canvas.width = rotated ? outputHeight : width;
  canvas.height = rotated ? width : outputHeight;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("barcode canvas is unavailable");
  context.imageSmoothingEnabled = true;
  if (rotated) {
    context.translate(canvas.width, 0);
    context.rotate(Math.PI / 2);
  }
  context.drawImage(image, 0, cropTop, sourceWidth, cropHeight, 0, 0, width, outputHeight);
  if (rotated) context.setTransform(1, 0, 0, 1, 0, 0);

  return canvas;
}

async function decodeBarcodeFromImageOnce(image: HTMLImageElement) {
  const variants = barcodeImageVariants();
  const detector = await createNativeBarcodeDetector();
  if (detector) {
    try {
      for (const variant of variants) {
        const canvas = renderBarcodeCanvas(image, variant);
        const detected = await detector.detect(canvas);
        const value = firstDetectedBarcode(detected);
        if (value) return value;
      }
    } catch {
      // Continue with ZXing on browsers with an incomplete BarcodeDetector.
    }
  }

  const reader = await createZXingBarcodeReader();
  for (const mode of [null, "contrast", "binary"] as const) {
    for (const variant of variants) {
      try {
        const rendered = renderBarcodeCanvas(image, variant);
        const canvas = mode ? enhancedBarcodeCanvas(rendered, mode) : rendered;
        const value = reader.decode(canvas);
        if (value) return value;
      } catch {
        // Try the next crop, contrast pass, or orientation.
      }
    }
  }
  throw new Error("barcode could not be recognized");
}

export async function decodeBarcodeFromImage(image: HTMLImageElement) {
  let lastError: unknown;
  for (let attempt = 0; attempt < 2; attempt += 1) {
    try {
      return await decodeBarcodeFromImageOnce(image);
    } catch (error) {
      lastError = error;
      // Some mobile BarcodeDetector implementations intermittently return no
      // result for the same decoded photo. Recreate both detectors once before
      // asking the user to take another picture.
    }
  }
  throw lastError instanceof Error ? lastError : new Error("barcode could not be recognized");
}
