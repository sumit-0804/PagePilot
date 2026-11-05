/** One `data:` payload from the chat stream, parsed from JSON. */
export type SSEPayload = { type?: string } & Record<string, unknown>

/**
 * Splits buffered SSE text into complete events. Returns their JSON payloads and the unfinished
 * tail to keep for the next chunk. Comment lines (":" heartbeats) and events without data are skipped.
 */
export function parseSSE(buffer: string): { payloads: SSEPayload[]; rest: string } {
  const events = buffer.split('\n\n')
  const rest = events.pop() ?? ''
  const payloads: SSEPayload[] = []
  for (const event of events) {
    const data = event.split('\n').find((line) => line.startsWith('data:'))
    if (data) payloads.push(JSON.parse(data.slice(5)) as SSEPayload)
  }
  return { payloads, rest }
}
