// Cindy's image byte decoding and exact normalized-stroke rasterization.
// WinOne's existing draft/attachment transaction replaces Cindy's composer store.
import {mediaHost} from './host';
import {drawStrokesOnCanvas,type AnnotationStroke} from './lightboxAnnotations';

function cleanImageMime(mime: string): string {
  return mime.split(';')[0].trim();
}

const IMAGE_MAX_BYTES = 100 * 1024 * 1024;

function assertDataUrlWithinLimit(estimatedBytes: number): void {
  if (estimatedBytes > IMAGE_MAX_BYTES) {
    throw new Error('图片过大');
  }
}

export function xdtFileUrlToPath(url: string): string | null {
  if (!url.startsWith('xdt-file://')) return null;
  try {
    return new URL(url).searchParams.get('path');
  } catch {
    return null;
  }
}

export async function loadImageSourceBase64(
  src: string,
): Promise<{ base64: string; mimeType: string }> {
  if (src.startsWith('xdt-image://') || src.startsWith('cindy-media://')) {
    return mediaHost.readCachedImageAsBase64({ url: src });
  }
  const base64Match = /^data:(image\/[a-z0-9+.-]+);base64,(.+)$/i.exec(src);
  if (base64Match) {
    assertDataUrlWithinLimit((base64Match[2].length * 3) / 4);
    return { base64: base64Match[2], mimeType: base64Match[1] };
  }
  if (/^data:image\//i.test(src)) {
    // 其余 data: 形式(非 base64 的 URL-encoded、或带 MIME 参数如
    // `;charset=utf-8` 的 base64):fetch 归一化(data: URL 同源),
    // blob.type 可能保留参数,统一剥掉。解码大小 ≤ URL 长度,先保守限流。
    assertDataUrlWithinLimit(src.length);
    return mediaHost.readImageBytes({ url: src });
  }
  if (
    xdtFileUrlToPath(src) !== null ||
    /^https?:\/\//.test(src) ||
    src.startsWith('cindy-remote-media://')
  ) {
    const res = await mediaHost.readImageBytes({ url: src });
    return { base64: res.base64, mimeType: cleanImageMime(res.mimeType) };
  }
  throw new Error(`unsupported image source: ${src.slice(0, 32)}`);
}

export function isImageBytesReachable(src: string): boolean {
  return (
    src.startsWith('xdt-image://') ||
    src.startsWith('cindy-media://') ||
    xdtFileUrlToPath(src) !== null ||
    /^data:image\//i.test(src) ||
    /^https?:\/\//.test(src) ||
    src.startsWith('cindy-remote-media://')
  );
}

export async function burnInAnnotations(
  source: { base64: string; mimeType: string },
  strokes: readonly AnnotationStroke[],
  outMimeOverride?: 'image/png' | 'image/jpeg',
): Promise<{ blob: Blob; mimeType: string }> {
  const image = new Image();
  image.src = `data:${source.mimeType};base64,${source.base64}`;
  await image.decode();

  const canvas = document.createElement('canvas');
  canvas.width = image.naturalWidth;
  canvas.height = image.naturalHeight;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('canvas unavailable');
  ctx.drawImage(image, 0, 0, image.naturalWidth, image.naturalHeight);
  drawStrokesOnCanvas(ctx, strokes, image.naturalWidth, image.naturalHeight);

  const outMime =
    outMimeOverride ?? (source.mimeType === 'image/jpeg' ? 'image/jpeg' : 'image/png');
  const blob = await new Promise<Blob | null>((resolve) =>
    canvas.toBlob(resolve, outMime, 0.92),
  );
  if (!blob) throw new Error('encode failed');
  return { blob, mimeType: outMime };
}

export function blobToBase64Payload(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result as string;
      const idx = result.indexOf(',');
      resolve(idx >= 0 ? result.slice(idx + 1) : result);
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

export function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}
