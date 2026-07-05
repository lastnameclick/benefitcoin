// Generates the PWA icon set (brand-teal gradient square + a white coin
// mark) as plain PNGs, using only Node's built-in zlib — no image library
// dependency. Re-run with `node scripts/gen-icons.mjs` after a rebrand.
import { deflateSync, crc32 } from "node:zlib";
import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const outDir = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "public", "icons");

// Matches the .brand-mark gradient in src/styles.css.
const GRADIENT_FROM = [0x0e, 0x7c, 0x66];
const GRADIENT_TO = [0x14, 0xa8, 0x88];

const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

function chunk(type, data) {
  const typeBuf = Buffer.from(type, "ascii");
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length, 0);
  const crcBuf = Buffer.alloc(4);
  crcBuf.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])) >>> 0, 0);
  return Buffer.concat([len, typeBuf, data, crcBuf]);
}

function lerp(a, b, t) {
  return Math.round(a + (b - a) * t);
}

// Renders a size×size RGBA square: a diagonal brand gradient background with
// a centered white "coin" circle. `coinScale` keeps the circle within a
// maskable icon's safe zone when needed.
function renderPng(size, coinScale) {
  const pixels = Buffer.alloc(size * size * 4);
  const cx = size / 2;
  const cy = size / 2;
  const r = size * coinScale;
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const t = (x + y) / (2 * size); // diagonal gradient position, 0..1
      const dx = x - cx + 0.5;
      const dy = y - cy + 0.5;
      const inCoin = dx * dx + dy * dy <= r * r;
      const i = (y * size + x) * 4;
      if (inCoin) {
        pixels[i] = 0xff;
        pixels[i + 1] = 0xff;
        pixels[i + 2] = 0xff;
      } else {
        pixels[i] = lerp(GRADIENT_FROM[0], GRADIENT_TO[0], t);
        pixels[i + 1] = lerp(GRADIENT_FROM[1], GRADIENT_TO[1], t);
        pixels[i + 2] = lerp(GRADIENT_FROM[2], GRADIENT_TO[2], t);
      }
      pixels[i + 3] = 0xff;
    }
  }

  // Each scanline needs a leading filter-type byte (0 = none).
  const raw = Buffer.alloc(size * (1 + size * 4));
  for (let y = 0; y < size; y++) {
    const srcStart = y * size * 4;
    const dstStart = y * (1 + size * 4);
    raw[dstStart] = 0;
    pixels.copy(raw, dstStart + 1, srcStart, srcStart + size * 4);
  }

  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(size, 0);
  ihdr.writeUInt32BE(size, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 6; // color type: RGBA
  ihdr[10] = 0; // compression
  ihdr[11] = 0; // filter
  ihdr[12] = 0; // interlace

  return Buffer.concat([
    PNG_SIGNATURE,
    chunk("IHDR", ihdr),
    chunk("IDAT", deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

const targets = [
  { name: "icon-192.png", size: 192, coinScale: 0.34 },
  { name: "icon-512.png", size: 512, coinScale: 0.34 },
  { name: "icon-maskable-512.png", size: 512, coinScale: 0.26 }, // stays inside the 80% safe zone
  { name: "apple-touch-icon.png", size: 180, coinScale: 0.34 },
];

for (const t of targets) {
  writeFileSync(path.join(outDir, t.name), renderPng(t.size, t.coinScale));
  console.log("wrote", t.name);
}
