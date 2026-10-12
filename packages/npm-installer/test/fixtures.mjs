import { crc32 } from 'node:zlib'
import { gzipSync } from 'node:zlib'

export function tarFixture(name, type = '0', link = '') {
  const header = Buffer.alloc(512)
  header.write(name, 0, 100)
  for (const [offset, width, value] of [[100, 8, 0o644], [108, 8, 0], [116, 8, 0], [124, 12, 0], [136, 12, 0]]) {
    header.write(value.toString(8).padStart(width - 1, '0') + '\0', offset, width)
  }
  header.fill(32, 148, 156)
  header.write(type, 156, 1)
  header.write(link, 157, 100)
  header.write('ustar\0', 257, 6)
  header.write('00', 263, 2)
  const sum = [...header].reduce((total, byte) => total + byte, 0)
  header.write(sum.toString(8).padStart(6, '0') + '\0 ', 148, 8)
  return gzipSync(Buffer.concat([header, Buffer.alloc(1024)]))
}

export function zipFixture(entries) {
  const local = []
  const central = []
  let offset = 0
  for (const { name, body = Buffer.alloc(0), mode = 0o100644 } of entries) {
    const filename = Buffer.from(name)
    const bytes = Buffer.from(body)
    const header = Buffer.alloc(30)
    header.writeUInt32LE(0x04034b50, 0)
    header.writeUInt16LE(20, 4)
    header.writeUInt32LE(crc32(bytes), 14)
    header.writeUInt32LE(bytes.length, 18)
    header.writeUInt32LE(bytes.length, 22)
    header.writeUInt16LE(filename.length, 26)
    local.push(header, filename, bytes)
    const directory = Buffer.alloc(46)
    directory.writeUInt32LE(0x02014b50, 0)
    directory.writeUInt16LE(0x0314, 4)
    directory.writeUInt16LE(20, 6)
    directory.writeUInt32LE(crc32(bytes), 16)
    directory.writeUInt32LE(bytes.length, 20)
    directory.writeUInt32LE(bytes.length, 24)
    directory.writeUInt16LE(filename.length, 28)
    directory.writeUInt32LE((mode << 16) >>> 0, 38)
    directory.writeUInt32LE(offset, 42)
    central.push(directory, filename)
    offset += header.length + filename.length + bytes.length
  }
  const index = Buffer.concat(central)
  const end = Buffer.alloc(22)
  end.writeUInt32LE(0x06054b50, 0)
  end.writeUInt16LE(entries.length, 8)
  end.writeUInt16LE(entries.length, 10)
  end.writeUInt32LE(index.length, 12)
  end.writeUInt32LE(offset, 16)
  return Buffer.concat([...local, index, end])
}
