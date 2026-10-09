import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  captureBarcodeVideoRegion,
  createBarcodeFrameDecoder,
} from "../../infrastructure/browser/barcodeScanner";
import {
  acquireBarcodeCamera,
  releaseBarcodeCamera,
  stopBarcodeCamera,
} from "../../infrastructure/browser/barcodeCamera";

type ScannerStatus = "starting" | "scanning" | "unsupported" | "denied" | "unavailable" | "error";

function scannerError(error: unknown): { status: ScannerStatus; message: string } {
  if (error instanceof DOMException) {
    if (error.name === "NotAllowedError" || error.name === "SecurityError") {
      return { status: "denied", message: "Разрешите доступ к камере в настройках браузера или используйте обычную фотографию." };
    }
    if (error.name === "NotFoundError" || error.name === "OverconstrainedError") {
      return { status: "unavailable", message: "Задняя камера недоступна. Можно выбрать фотографию штрихкода." };
    }
    if (error.name === "NotReadableError" || error.name === "AbortError") {
      return { status: "unavailable", message: "Камеру использует другое приложение. Закройте его или выберите фотографию." };
    }
  }
  return { status: "error", message: "Не удалось запустить камеру. Используйте фотографию штрихкода." };
}

export default function BarcodeScannerSheet({ onDetected, onClose, onFallback }: {
  onDetected: (code: string) => void;
  onClose: () => void;
  onFallback: () => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const frameRef = useRef<HTMLDivElement>(null);
  const detectedRef = useRef(onDetected);
  const closeRef = useRef(onClose);
  const [status, setStatus] = useState<ScannerStatus>("starting");
  const [message, setMessage] = useState("Запускаем заднюю камеру…");

  useEffect(() => { detectedRef.current = onDetected; }, [onDetected]);
  useEffect(() => { closeRef.current = onClose; }, [onClose]);

  useEffect(() => {
    let cancelled = false;
    let timer = 0;
    let stream: MediaStream | null = null;

    const releaseCamera = () => {
      window.clearTimeout(timer);
      const activeStream = stream;
      stream = null;
      if (videoRef.current) {
        videoRef.current.pause();
        videoRef.current.srcObject = null;
      }
      if (activeStream) releaseBarcodeCamera(activeStream);
    };

    const closeWhenHidden = () => {
      if (document.visibilityState !== "hidden") return;
      cancelled = true;
      stream = null;
      if (videoRef.current) {
        videoRef.current.pause();
        videoRef.current.srcObject = null;
      }
      stopBarcodeCamera();
      closeRef.current();
    };

    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      cancelled = true;
      releaseCamera();
      closeRef.current();
    };

    document.addEventListener("visibilitychange", closeWhenHidden);
    window.addEventListener("keydown", closeOnEscape);

    async function start() {
      if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
        setStatus("unsupported");
        setMessage("Live-камера недоступна в этом режиме. Используйте обычную фотографию штрихкода.");
        return;
      }
      try {
        const decoderPromise = createBarcodeFrameDecoder();
        stream = await acquireBarcodeCamera();
        if (cancelled) {
          releaseCamera();
          return;
        }
        const video = videoRef.current;
        if (!video) throw new Error("scanner video is unavailable");
        const cameraTrack = stream.getVideoTracks()[0];
        if (cameraTrack?.getCapabilities) {
          const capabilities = cameraTrack.getCapabilities() as MediaTrackCapabilities & { focusMode?: string[] };
          if (capabilities.focusMode?.includes("continuous")) {
            const advanced = { focusMode: "continuous" } as MediaTrackConstraintSet;
            await cameraTrack.applyConstraints({ advanced: [advanced] }).catch(() => undefined);
          }
        }
        video.srcObject = stream;
        await video.play();
        const decoder = await decoderPromise;
        if (cancelled) return;
        setStatus("scanning");
        setMessage("Поместите все полосы и свободные края внутрь рамки");

        const scan = async () => {
          if (cancelled) return;
          const frame = frameRef.current;
          const activeVideo = videoRef.current;
          if (frame && activeVideo && activeVideo.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
            const canvas = captureBarcodeVideoRegion(activeVideo, frame.getBoundingClientRect());
            const code = canvas ? await decoder.decode(canvas) : "";
            if (cancelled) return;
            if (code) {
              cancelled = true;
              releaseCamera();
              if ("vibrate" in navigator) navigator.vibrate(35);
              detectedRef.current(code);
              return;
            }
          }
          timer = window.setTimeout(() => void scan(), 260);
        };
        void scan();
      } catch (error) {
        releaseCamera();
        if (cancelled) return;
        const next = scannerError(error);
        setStatus(next.status);
        setMessage(next.message);
      }
    }

    void start();
    return () => {
      cancelled = true;
      releaseCamera();
      document.removeEventListener("visibilitychange", closeWhenHidden);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, []);

  const cameraFailed = status === "unsupported" || status === "denied" || status === "unavailable" || status === "error";
  return createPortal(
    <section className={`barcodeScannerSheet${cameraFailed ? " isUnavailable" : ""}`} role="dialog" aria-modal="true" aria-labelledby="barcode-scanner-title">
      <video ref={videoRef} className="barcodeScannerVideo" autoPlay muted playsInline aria-hidden="true" />
      <header className="barcodeScannerHeader">
        <div><strong id="barcode-scanner-title">Сканирование штрихкода</strong><span>EAN · UPC · DATABAR</span></div>
        <button type="button" onClick={onClose} aria-label="Закрыть сканер">×</button>
      </header>
      {!cameraFailed && <div ref={frameRef} className={`barcodeScannerFrame${status === "scanning" ? " isScanning" : ""}`} aria-hidden="true"><i /></div>}
      {cameraFailed && <div className="barcodeScannerUnavailable" aria-hidden="true"><span /><strong>Камера недоступна</strong></div>}
      <footer className="barcodeScannerFooter">
        <p className={`barcodeScannerStatus status-${status}`} aria-live="polite"><i aria-hidden="true" />{message}</p>
        <button type="button" className="barcodeScannerFallback" onClick={onFallback}>Сфотографировать вместо сканирования</button>
      </footer>
    </section>,
    document.body,
  );
}
