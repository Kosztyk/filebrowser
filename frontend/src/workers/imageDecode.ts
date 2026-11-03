import UTIF from "utif";
const workerSelf = globalThis as unknown as {
  onmessage: ((event: MessageEvent<ArrayBuffer>) => void) | null;
  postMessage: (data: unknown, transfer?: Transferable[]) => void;
};
workerSelf.onmessage = (event) => {
  try {
    const buffer = event.data;
    if (buffer.byteLength > 32 * 1024 * 1024)
      throw new Error("Encoded image exceeds the 32 MiB preview limit");
    const pages = UTIF.decode(buffer);
    if (!pages.length || pages.length > 64)
      throw new Error("Unsupported image page count");
    const image = pages[0];
    const width = Number(image.t256?.[0]);
    const height = Number(image.t257?.[0]);
    if (
      !Number.isSafeInteger(width) ||
      !Number.isSafeInteger(height) ||
      width <= 0 ||
      height <= 0 ||
      width * height > 32_000_000
    )
      throw new Error("Image exceeds the 32 million pixel preview limit");
    UTIF.decodeImage(buffer, image);
    const rgba = UTIF.toRGBA8(image) as Uint8Array;
    if (rgba.length !== width * height * 4)
      throw new Error("Invalid decoded image length");
    workerSelf.postMessage({ width, height, rgba }, [
      rgba.buffer as ArrayBuffer,
    ]);
  } catch (e) {
    workerSelf.postMessage({ error: String(e) });
  }
};
