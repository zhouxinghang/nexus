import { readdir, readFile, writeFile } from "node:fs/promises";
import { gzipSync } from "node:zlib";

// Precompression keeps the native Host process from spending CPU compressing
// the same immutable asset on every request. No runtime/package dependency.
const output = new URL("../../server/internal/web/dist/", import.meta.url);
let originals = 0;
let compressed = 0;
let count = 0;

async function compressDirectory(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const file = new URL(entry.name + (entry.isDirectory() ? "/" : ""), directory);
    if (entry.isDirectory()) {
      await compressDirectory(file);
    } else if (/\.(html|js|css|svg)$/.test(entry.name)) {
      const data = await readFile(file);
      const gzip = gzipSync(data, { level: 9 });
      await writeFile(new URL(`${entry.name}.gz`, directory), gzip);
      originals += data.length;
      compressed += gzip.length;
      count++;
    }
  }
}

await compressDirectory(output);
console.log(`Precompressed ${count} text assets: ${(originals / 1024).toFixed(1)} KiB → ${(compressed / 1024).toFixed(1)} KiB gzip`);
