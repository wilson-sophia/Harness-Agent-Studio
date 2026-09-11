import type {
  HarnessConnection,
  PipelineForm,
  PipelineResponse,
  ProjectAnalysisResponse,
} from './types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'

export async function analyzeRepository(
  form: PipelineForm,
  connection: HarnessConnection,
): Promise<ProjectAnalysisResponse> {
  const response = await fetch(`${apiBaseUrl}/api/projects/analyze`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      repoUrl: form.repoUrl,
      branch: form.branch,
      mode: form.mode,
      harnessAccountId: connection.accountId,
      hasHarnessToken: connection.apiToken.trim().length > 0,
    }),
  })

  return parseResponse<ProjectAnalysisResponse>(response)
}

export async function generatePipeline(
  form: PipelineForm,
  connection: HarnessConnection,
): Promise<PipelineResponse> {
  const response = await fetch(`${apiBaseUrl}/api/pipelines/generate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      ...form,
      harnessAccountId: connection.accountId,
      hasHarnessToken: connection.apiToken.trim().length > 0,
    }),
  })

  return parseResponse<PipelineResponse>(response)
}

async function parseResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`)
  }

  return (await response.json()) as T
}
