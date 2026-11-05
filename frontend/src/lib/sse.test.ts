import { describe, expect, it } from 'vitest'
import { parseSSE } from './sse'

describe('parseSSE', () => {
  it('returns complete events and keeps the unfinished tail', () => {
    const { payloads, rest } = parseSSE('data: {"type":"token","content":"Hel"}\n\ndata: {"type":"tok')
    expect(payloads).toEqual([{ type: 'token', content: 'Hel' }])
    expect(rest).toBe('data: {"type":"tok')
  })

  it('joins an event split across chunks', () => {
    const first = parseSSE('data: {"type":"token",')
    expect(first.payloads).toEqual([])
    const second = parseSSE(first.rest + '"content":"lo"}\n\n')
    expect(second.payloads).toEqual([{ type: 'token', content: 'lo' }])
    expect(second.rest).toBe('')
  })

  it('skips heartbeat comments', () => {
    const { payloads } = parseSSE(': ping\n\ndata: {"type":"status","content":"Searching"}\n\n: ping\n\n')
    expect(payloads).toEqual([{ type: 'status', content: 'Searching' }])
  })

  it('reads several events in one chunk, in order', () => {
    const { payloads } = parseSSE(
      'data: {"type":"token","content":"a"}\n\ndata: {"type":"token","content":"b"}\n\ndata: {"type":"done","conversation_id":"c1"}\n\n',
    )
    expect(payloads.map((p) => p.type)).toEqual(['token', 'token', 'done'])
  })

  it('throws on a malformed event instead of dropping it silently', () => {
    expect(() => parseSSE('data: {not json}\n\n')).toThrow(SyntaxError)
  })
})
