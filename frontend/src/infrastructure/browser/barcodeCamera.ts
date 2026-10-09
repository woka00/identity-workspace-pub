let cachedStream: MediaStream | null = null;
let pendingStream: Promise<MediaStream> | null = null;
let activeConsumers = 0;
let lifecycleInstalled = false;
let requestGeneration = 0;

const cameraConstraints: MediaStreamConstraints = {
  audio: false,
  video: {
    facingMode: { ideal: "environment" },
    width: { ideal: 2160 },
    height: { ideal: 3840 },
    aspectRatio: { ideal: 9 / 16 },
  },
};

function liveVideoTracks(stream: MediaStream | null) {
  return stream?.getVideoTracks().filter((track) => track.readyState === "live") ?? [];
}

function installLifecycleCleanup() {
  if (lifecycleInstalled) return;
  lifecycleInstalled = true;
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") stopBarcodeCamera();
  });
  window.addEventListener("pagehide", stopBarcodeCamera);
}

/** Coalesce concurrent mounts while the scanner is opening. */
export async function acquireBarcodeCamera() {
  installLifecycleCleanup();
  const cachedTracks = liveVideoTracks(cachedStream);
  if (cachedStream && cachedTracks.length > 0) {
    activeConsumers += 1;
    cachedTracks.forEach((track) => { track.enabled = true; });
    return cachedStream;
  }

  if (!pendingStream) {
    const generation = requestGeneration;
    pendingStream = navigator.mediaDevices.getUserMedia(cameraConstraints)
      .then((stream) => {
        if (generation !== requestGeneration) {
          stream.getTracks().forEach((track) => track.stop());
          throw new DOMException("Camera request was cancelled", "AbortError");
        }
        cachedStream = stream;
        stream.getVideoTracks().forEach((track) => {
          track.addEventListener("ended", () => {
            if (cachedStream === stream && liveVideoTracks(stream).length === 0) cachedStream = null;
          }, { once: true });
        });
        return stream;
      })
      .finally(() => { pendingStream = null; });
  }

  const stream = await pendingStream;
  activeConsumers += 1;
  liveVideoTracks(stream).forEach((track) => { track.enabled = true; });
  return stream;
}

/** Fully release the capture device as soon as the scanner closes. */
export function releaseBarcodeCamera(stream: MediaStream) {
  activeConsumers = Math.max(0, activeConsumers - 1);
  if (activeConsumers > 0) return;
  stream.getTracks().forEach((track) => track.stop());
  if (stream === cachedStream) cachedStream = null;
}

/** Fully release the device whenever the PWA leaves the foreground. */
export function stopBarcodeCamera() {
  requestGeneration += 1;
  activeConsumers = 0;
  cachedStream?.getTracks().forEach((track) => track.stop());
  cachedStream = null;
}
