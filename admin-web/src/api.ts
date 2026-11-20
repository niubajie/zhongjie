export interface Health {
  status: string
}

const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? 'http://127.0.0.1:8080'

async function parseJson<T>(response: Response): Promise<T> {
  const body = (await response.json()) as T
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`)
  }
  return body
}

export async function fetchHealth(): Promise<Health> {
  const response = await fetch(`${apiBase}/healthz`, { headers: { Accept: 'application/json' } })
  return parseJson<Health>(response)
}
