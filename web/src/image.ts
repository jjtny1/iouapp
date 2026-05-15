// Receipt parsing runs through the Anthropic vision API, which accepts JPEG,
// PNG, GIF, and WebP — but not HEIC, the default format for iPhone photos.
// This module converts HEIC/HEIF to JPEG in the browser and downscales large
// photos so uploads stay well within the parser's size limits.

const MAX_EDGE = 1600;
const JPEG_QUALITY = 0.9;
const HEIC_EXT = /\.(heic|heif)$/i;

// prepareReceiptImage normalizes a user-picked file into a JPEG suitable for
// upload: HEIC photos are converted, oversized images are downscaled. Files
// that are already a reasonably sized JPEG are returned untouched.
export async function prepareReceiptImage(file: File): Promise<File> {
  let blob: Blob = file;
  let name = file.name;

  if (isHeic(file)) {
    // heic-to pulls in a large libheif WASM bundle; load it on demand so it
    // only reaches users who actually upload an iPhone HEIC photo.
    const { heicTo } = await import("heic-to");
    blob = await heicTo({
      blob: file,
      type: "image/jpeg",
      quality: JPEG_QUALITY,
    });
    name = name.replace(HEIC_EXT, ".jpg");
  }

  const downscaled = await downscale(blob);
  if (downscaled) {
    blob = downscaled;
    name = name.replace(/\.[^.]+$/, ".jpg");
  }

  if (blob === file) return file;
  return new File([blob], name, { type: "image/jpeg" });
}

// isHeic detects HEIC/HEIF by MIME type, with an extension fallback since some
// browsers report an empty type for HEIC files.
function isHeic(file: File): boolean {
  return /image\/hei[cf]/i.test(file.type) || HEIC_EXT.test(file.name);
}

// downscale re-encodes the image as JPEG with its longest edge capped at
// MAX_EDGE. It returns null when no change is needed (the image already fits
// and is a JPEG), letting the caller keep the original file.
async function downscale(blob: Blob): Promise<Blob | null> {
  const url = URL.createObjectURL(blob);
  try {
    const img = await loadImage(url);
    const scale = Math.min(1, MAX_EDGE / Math.max(img.width, img.height));
    if (scale === 1 && blob.type === "image/jpeg") return null;

    const canvas = document.createElement("canvas");
    canvas.width = Math.round(img.width * scale);
    canvas.height = Math.round(img.height * scale);
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);

    return await new Promise<Blob | null>((resolve) =>
      canvas.toBlob((b) => resolve(b), "image/jpeg", JPEG_QUALITY),
    );
  } finally {
    URL.revokeObjectURL(url);
  }
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error("could not read the selected image"));
    img.src = src;
  });
}
