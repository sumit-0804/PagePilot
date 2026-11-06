import { describe, expect, it } from 'vitest'
import { isFileDrag, MAX_UPLOAD_BYTES, validateUpload } from './upload'

function fakeFile(name: string, size = 10): File {
  const file = new File(['x'], name)
  Object.defineProperty(file, 'size', { value: size })
  return file
}

describe('validateUpload', () => {
  it('accepts supported types regardless of case', () => {
    expect(validateUpload(fakeFile('Notes.PDF'))).toBeNull()
    expect(validateUpload(fakeFile('readme.md'))).toBeNull()
  })

  it('rejects unsupported types', () => {
    expect(validateUpload(fakeFile('setup.exe'))).toMatch(/isn’t supported/)
  })

  it('rejects files over 50 MB', () => {
    expect(validateUpload(fakeFile('big.pdf', MAX_UPLOAD_BYTES + 1))).toMatch(/larger than 50 MB/)
  })
})

describe('isFileDrag', () => {
  it('is true only when the drag carries files', () => {
    expect(isFileDrag({ types: ['Files'] })).toBe(true)
    expect(isFileDrag({ types: ['text/plain', 'text/html'] })).toBe(false)
    expect(isFileDrag(null)).toBe(false)
  })
})
