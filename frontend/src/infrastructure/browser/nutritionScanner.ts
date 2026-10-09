export type ScannedNutrition = {
  calories?: number;
  protein?: number;
  fat?: number;
  carbohydrate?: number;
  recognizedText: string;
};

type NutritionKey = "calories" | "protein" | "fat" | "carbohydrate";

const labels: Record<NutritionKey, RegExp> = {
  calories: /(?:энергетическ\S*\s+(?:ценност\S*|цен\S*)|калори\S*|энергия|energy|calories?|kcal|ккал)/iu,
  protein: /(?:белк(?:и|ов|а)?|протеин\S*|protein(?:s)?)/iu,
  fat: /(?:жир(?:ы|ов|а)?|fat(?:s)?|lipid(?:s)?)/iu,
  carbohydrate: /(?:углевод(?:ы|ов|а)?|carbohydrate(?:s)?|carbs?)/iu,
};

const allLabels = /(?:энергетическ\S*\s+(?:ценност\S*|цен\S*)|калори\S*|энергия|energy|calories?|kcal|ккал|белк(?:и|ов|а)?|протеин\S*|protein(?:s)?|жир(?:ы|ов|а)?|fat(?:s)?|lipid(?:s)?|углевод(?:ы|ов|а)?|carbohydrate(?:s)?|carbs?)/giu;
const numberPattern = /\d{1,4}(?:[.,]\d{1,2})?/g;

function parseNumber(value: string) {
  return Number(value.replace(",", "."));
}

function validValue(key: NutritionKey, value: number) {
  return Number.isFinite(value) && value >= 0 && value <= (key === "calories" ? 1000 : 100);
}

function numbersIn(value: string) {
  return Array.from(value.matchAll(numberPattern), (match) => ({ value: parseNumber(match[0]), index: match.index ?? 0 }));
}

function explicitCalories(line: string) {
  const values = Array.from(line.matchAll(/(\d{1,4}(?:[.,]\d{1,2})?)\s*(?:kcal|ккал)/giu), (match) => parseNumber(match[1]));
  return values.find((value) => validValue("calories", value));
}

function valueAfterLabel(line: string, key: NutritionKey) {
  const label = labels[key].exec(line);
  if (!label?.index && label?.index !== 0) return undefined;
  const segmentStart = label.index + label[0].length;
  const nextLabel = allLabels.exec(line.slice(segmentStart));
  allLabels.lastIndex = 0;
  const segment = line.slice(segmentStart, nextLabel?.index === undefined ? undefined : segmentStart + nextLabel.index);
  if (key === "calories") {
    const kcal = explicitCalories(line);
    if (kcal !== undefined) return kcal;
    // Kilojoules are energy, but not calories. Never convert or copy them into
    // the kcal field; mixed "kJ / kcal" rows are handled by explicitCalories.
    if (/(?:kj|кдж)/iu.test(line)) return undefined;
  }
  const values = numbersIn(segment)
    .filter(({ index }) => !/%/.test(segment.slice(index, index + 12)))
    .map(({ value }) => value);
  return values.find((value) => validValue(key, value));
}

/** Parse both ordinary sentences and the row/column layouts commonly printed on packaging. */
export function parseNutritionText(rawText: string): ScannedNutrition {
  const text = rawText
    .replace(/[|¦]/g, " ")
    .replace(/[—–]/g, "-")
    .replace(/\u00a0/g, " ");
  const lines = text.split(/\r?\n/).map((line) => line.replace(/\s+/g, " ").trim()).filter(Boolean);
  const found: Partial<Record<NutritionKey, number>> = {};

  for (const line of lines) {
    (Object.keys(labels) as NutritionKey[]).forEach((key) => {
      if (found[key] !== undefined || !labels[key].test(line)) return;
      if (key === "fat" && /(?:насыщ|трансжир|saturat|trans\s*fat)/iu.test(line) && !/(?:всего|total)\s+(?:жир|fat)/iu.test(line)) return;
      const value = valueAfterLabel(line, key);
      if (value !== undefined) found[key] = value;
    });
  }

  // Some compact labels put all headings on one row and all values below it.
  for (let index = 0; index < lines.length - 1; index += 1) {
    const heading = lines[index];
    const ordered = (Object.keys(labels) as NutritionKey[])
      .map((key) => ({ key, index: heading.search(labels[key]) }))
      .filter((item) => item.index >= 0)
      .sort((left, right) => left.index - right.index);
    if (ordered.length < 2) continue;
    const values = numbersIn(lines[index + 1]).map(({ value }) => value);
    if (values.length < ordered.length) continue;
    ordered.forEach(({ key }, valueIndex) => {
      if (key === "calories" && /(?:kj|кдж)/iu.test(lines[index + 1]) && !/(?:kcal|ккал)/iu.test(lines[index + 1])) return;
      if (found[key] === undefined && validValue(key, values[valueIndex])) found[key] = values[valueIndex];
    });
  }

  // Energy is often written as "840 kJ / 200 kcal" without another label.
  if (found.calories === undefined) {
    for (const line of lines) {
      const value = explicitCalories(line);
      if (value !== undefined) {
        found.calories = value;
        break;
      }
    }
  }

  return {
    calories: found.calories,
    protein: found.protein,
    fat: found.fat,
    carbohydrate: found.carbohydrate,
    recognizedText: text.trim(),
  };
}

async function imageCanvas(source: File | HTMLCanvasElement) {
  if (source instanceof HTMLCanvasElement) return source;
  let image: ImageBitmap | HTMLImageElement;
  let objectURL = "";
  try {
    image = await createImageBitmap(source, { imageOrientation: "from-image" });
  } catch {
    objectURL = URL.createObjectURL(source);
    image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const element = new Image();
      element.onload = () => resolve(element);
      element.onerror = () => reject(new Error("nutrition image could not be decoded"));
      element.src = objectURL;
    });
  }
  const scale = Math.min(1, 2400 / Math.max(image.width, image.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(image.width * scale));
  canvas.height = Math.max(1, Math.round(image.height * scale));
  const context = canvas.getContext("2d", { willReadFrequently: true });
  if (!context) throw new Error("nutrition canvas is unavailable");
  context.drawImage(image, 0, 0, canvas.width, canvas.height);
  if (typeof ImageBitmap !== "undefined" && image instanceof ImageBitmap) image.close();
  if (objectURL) URL.revokeObjectURL(objectURL);

  const pixels = context.getImageData(0, 0, canvas.width, canvas.height);
  for (let index = 0; index < pixels.data.length; index += 4) {
    const grey = pixels.data[index] * .299 + pixels.data[index + 1] * .587 + pixels.data[index + 2] * .114;
    const contrasted = Math.max(0, Math.min(255, (grey - 128) * 1.38 + 128));
    pixels.data[index] = contrasted;
    pixels.data[index + 1] = contrasted;
    pixels.data[index + 2] = contrasted;
  }
  context.putImageData(pixels, 0, 0);
  return canvas;
}

export async function recognizeNutrition(
  source: File | HTMLCanvasElement,
  onProgress?: (progress: number) => void,
) {
  const [{ createWorker, OEM, PSM }, canvas] = await Promise.all([
    import("tesseract.js"),
    imageCanvas(source),
  ]);
  const worker = await createWorker(["rus", "eng"], OEM.LSTM_ONLY, {
    workerPath: "/tesseract/worker.min.js",
    corePath: "/tesseract/core",
    langPath: "https://tessdata.projectnaptha.com/4.0.0",
    logger: (message) => {
      if (message.status === "recognizing text") onProgress?.(message.progress);
    },
  });
  try {
    await worker.setParameters({ tessedit_pageseg_mode: PSM.AUTO, preserve_interword_spaces: "1", user_defined_dpi: "300" });
    const first = await worker.recognize(canvas, { rotateAuto: true });
    let parsed = parseNutritionText(first.data.text);
    if ([parsed.calories, parsed.protein, parsed.fat, parsed.carbohydrate].filter((value) => value !== undefined).length >= 4) return parsed;

    await worker.setParameters({ tessedit_pageseg_mode: PSM.SPARSE_TEXT, preserve_interword_spaces: "1" });
    const second = await worker.recognize(canvas, { rotateAuto: true });
    const combined = parseNutritionText(`${first.data.text}\n${second.data.text}`);
    parsed = combined;
    return parsed;
  } finally {
    await worker.terminate();
  }
}
