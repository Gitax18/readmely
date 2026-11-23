/**
 * A minimal ZIP writer (entries stored, not compressed), so a download can keep the folders a
 * preset needs (.cursor/rules/...) without a dependency. Paths are checked: nothing absolute, no
 * "..", no backslashes, so an unzipped pack can only land inside the folder it is unzipped into.
 */

const TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

export function crc32(data: Uint8Array): number {
  let c = 0xffffffff;
  for (const byte of data) c = (TABLE[(c ^ byte) & 0xff] as number) ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

export function safeZipPath(p: string): boolean {
  return (
    p.length > 0 &&
    p.length <= 200 &&
    !p.startsWith("/") &&
    !p.includes("\\") &&
    !/[\u0000-\u001f]/.test(p) &&
    p.split("/").every((seg) => seg !== "" && seg !== "." && seg !== "..")
  );
}

export interface ZipEntry {
  path: string;
  content: string;
}

export function makeZip(entries: ZipEntry[], now: Date = new Date()): Uint8Array<ArrayBuffer> {
  const enc = new TextEncoder();
  const time = (now.getHours() << 11) | (now.getMinutes() << 5) | (now.getSeconds() >> 1);
  const date = ((now.getFullYear() - 1980) << 9) | ((now.getMonth() + 1) << 5) | now.getDate();
  const parts: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  let offset = 0;

  const u16 = (v: DataView, o: number, n: number) => v.setUint16(o, n, true);
  const u32 = (v: DataView, o: number, n: number) => v.setUint32(o, n, true);

  for (const e of entries) {
    if (!safeZipPath(e.path)) throw new Error("unsafe path in archive");
    const name = enc.encode(e.path);
    const data = enc.encode(e.content);
    const crc = crc32(data);

    const local = new Uint8Array(30 + name.length);
    const lv = new DataView(local.buffer);
    u32(lv, 0, 0x04034b50);
    u16(lv, 4, 20);
    u16(lv, 6, 0x0800); // names are UTF-8
    u16(lv, 8, 0); // stored
    u16(lv, 10, time);
    u16(lv, 12, date);
    u32(lv, 14, crc);
    u32(lv, 18, data.length);
    u32(lv, 22, data.length);
    u16(lv, 26, name.length);
    local.set(name, 30);
    parts.push(local, data);

    const cen = new Uint8Array(46 + name.length);
    const cv = new DataView(cen.buffer);
    u32(cv, 0, 0x02014b50);
    u16(cv, 4, 20);
    u16(cv, 6, 20);
    u16(cv, 8, 0x0800);
    u16(cv, 10, 0);
    u16(cv, 12, time);
    u16(cv, 14, date);
    u32(cv, 16, crc);
    u32(cv, 20, data.length);
    u32(cv, 24, data.length);
    u16(cv, 28, name.length);
    u32(cv, 42, offset);
    cen.set(name, 46);
    central.push(cen);
    offset += local.length + data.length;
  }

  const centralSize = central.reduce((n, c) => n + c.length, 0);
  const end = new Uint8Array(22);
  const ev = new DataView(end.buffer);
  u32(ev, 0, 0x06054b50);
  u16(ev, 8, entries.length);
  u16(ev, 10, entries.length);
  u32(ev, 12, centralSize);
  u32(ev, 16, offset);

  const all = [...parts, ...central, end];
  const out = new Uint8Array(new ArrayBuffer(all.reduce((n, p) => n + p.length, 0)));
  let at = 0;
  for (const p of all) {
    out.set(p, at);
    at += p.length;
  }
  return out;
}
