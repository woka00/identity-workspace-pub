import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { acquireBarcodeCamera, releaseBarcodeCamera } from "../../infrastructure/browser/barcodeCamera";
import { recognizeNutrition, type ScannedNutrition } from "../../infrastructure/browser/nutritionScanner";

type ScannerStatus = "starting" | "ready" | "recognizing" | "unavailable" | "error";

export default function NutritionScannerSheet({ onDetected, onClose }: {
  onDetected: (nutrition: ScannedNutrition) => void;
  onClose: () => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const [status, setStatus] = useState<ScannerStatus>("starting");
  const [message, setMessage] = useState("Запускаем заднюю камеру…");

  function releaseCamera() {
    const stream = streamRef.current;
    streamRef.current = null;
    if (videoRef.current) {
      videoRef.current.pause();
      videoRef.current.srcObject = null;
    }
    if (stream) releaseBarcodeCamera(stream);
  }

  useEffect(() => {
    let cancelled = false;
    async function start() {
      if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
        setStatus("unavailable");
        setMessage("Камера недоступна. Выберите готовую фотографию этикетки.");
        return;
      }
      try {
        const stream = await acquireBarcodeCamera();
        if (cancelled) {
          releaseBarcodeCamera(stream);
          return;
        }
        streamRef.current = stream;
        const video = videoRef.current;
        if (!video) throw new Error("nutrition scanner video is unavailable");
        video.srcObject = stream;
        await video.play();
        setStatus("ready");
        setMessage("Держите таблицу ровно, без бликов, и нажмите кнопку");
      } catch {
        setStatus("unavailable");
        setMessage("Не удалось открыть камеру. Выберите готовую фотографию этикетки.");
      }
    }
    void start();
    return () => {
      cancelled = true;
      releaseCamera();
    };
  }, []);

  async function analyze(source: File | HTMLCanvasElement) {
    releaseCamera();
    setStatus("recognizing");
    setMessage("Подготавливаем распознавание…");
    try {
      const result = await recognizeNutrition(source, (progress) => {
        setMessage(`Распознаём текст… ${Math.max(1, Math.round(progress * 100))}%`);
      });
      const count = [result.calories, result.protein, result.fat, result.carbohydrate].filter((value) => value !== undefined).length;
      if (count === 0) {
        setStatus("error");
        setMessage("КБЖУ не найдено. Снимите этикетку ближе, ровно и при хорошем свете.");
        return;
      }
      onDetected(result);
    } catch {
      setStatus("error");
      setMessage("Не удалось распознать снимок. Проверьте интернет для первой загрузки OCR и попробуйте ещё раз.");
    }
  }

  function capture() {
    const video = videoRef.current;
    if (!video?.videoWidth || !video.videoHeight) return;
    const canvas = document.createElement("canvas");
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    const context = canvas.getContext("2d");
    if (!context) return;
    context.drawImage(video, 0, 0);
    void analyze(canvas);
  }

  const cameraAvailable = status === "ready";
  const busy = status === "recognizing";
  return createPortal(
    <section className={`nutritionScannerSheet${cameraAvailable ? "" : " isUnavailable"}`} role="dialog" aria-modal="true" aria-labelledby="nutrition-scanner-title">
      <video ref={videoRef} className="nutritionScannerVideo" autoPlay muted playsInline aria-hidden="true" />
      <header className="barcodeScannerHeader">
        <div><strong id="nutrition-scanner-title">Сканирование КБЖУ</strong><span>ТАБЛИЦА ИЛИ ТЕКСТ · РУССКИЙ И ENGLISH</span></div>
        <button type="button" onClick={onClose} aria-label="Закрыть сканер">×</button>
      </header>
      {cameraAvailable && <div className="nutritionScannerFrame" aria-hidden="true"><span>Поместите пищевую ценность в рамку</span></div>}
      {!cameraAvailable && <div className="nutritionScannerUnavailable" aria-hidden="true"><strong>{status === "recognizing" ? "Читаем этикетку" : "Нужна фотография"}</strong></div>}
      <footer className="nutritionScannerFooter">
        <p className={`barcodeScannerStatus status-${status}`} aria-live="polite"><i aria-hidden="true" />{message}</p>
        {cameraAvailable && <button type="button" className="nutritionScannerCapture" onClick={capture}><span aria-hidden="true" /></button>}
        <button type="button" className="barcodeScannerFallback" disabled={busy} onClick={() => fileRef.current?.click()}>Выбрать фото этикетки</button>
        <input ref={fileRef} className="foodBarcodeFile" type="file" accept="image/*" onChange={(event) => {
          const file = event.target.files?.[0];
          if (file && file.type.startsWith("image/") && file.size <= 15 * 1024 * 1024) {
            void analyze(file);
          } else if (file) {
            setStatus(streamRef.current ? "ready" : "error");
            setMessage("Выберите изображение размером не больше 15 МБ.");
          }
          event.currentTarget.value = "";
        }} />
      </footer>
    </section>,
    document.body,
  );
}
