import Hls from "hls.js";

/** Attaches hls.js, or sets video.src when MSE is unavailable. Returns null in the native case. */
export function bindHls(
  video: HTMLVideoElement,
  url: string,
  config: ConstructorParameters<typeof Hls>[0],
  onFatal: (details: string) => void,
): Hls | null {
  if (!Hls.isSupported()) {
    video.src = url;
    return null;
  }
  const hls = new Hls({ enableWorker: true, lowLatencyMode: false, ...config });
  hls.loadSource(url);
  hls.attachMedia(video);
  hls.on(Hls.Events.ERROR, (_, data) => {
    if (!data.fatal) return;
    if (data.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad();
    else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) hls.recoverMediaError();
    else onFatal(data.details);
  });
  return hls;
}
